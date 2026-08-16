// init.go — `logmind init` subcommand.
//
// Scaffolds a fresh logmind install into the current repository:
//
//   - docs/decisions.md, docs/decisions-archive.md, docs/file-structure.md,
//     docs/timeline.md (seed contents from embedded templates)
//   - .logmind/config.yml (verbatim copy of config.yml.template)
//   - AGENTS.md slim-or-full block (slim is the SPEC §1.1 default)
//   - per-agent stubs for every enabled agent (claude + cursor by default)
//   - .gitignore + .gitattributes logmind blocks
//   - git config merge drivers (when inside a git repo)
//   - .git/hooks/post-merge, post-rewrite, commit-msg
//   - .github/workflows/<all 4 templates>
//   - first decision log entry
//
// Refresh mode: when called against a repo where logmind is already
// initialised (.logmind/config.yml + docs/decisions.md exist), init
// reruns only the idempotent refresh steps — workflow template
// updates, AGENTS.md marker refresh, .gitattributes block, git config
// drivers, and hooks. docs/ and .logmind/ are left untouched.
//
// Flags mirror the Python CLI's init command:
//
//	--no-git              Skip git operations entirely
//	--agents <list>       Comma-separated agent enable list
//	--all-agents          Enable every agent in the registry
//	--github-actions/--no-github-actions  Install workflow templates (default on)
//	--spec                Scaffold docs/spec.md (if absent) and point
//	                       context.spec_file at it (if unset). Idempotent;
//	                       works in both fresh-install and refresh mode.
//	--refresh             Force-re-render the workflow files logmind owns,
//	                       even at a matching template version. This is the
//	                       command the shipped workflows' branch-drift
//	                       warning names; see workflowForceRender.
//
// Run `logmind install-hook` separately to set up the local pre-commit
// hook; that is a real top-level subcommand, not an init flag.
package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/agents"
	"github.com/thrillmade/logmind/internal/atomicio"
	"github.com/thrillmade/logmind/internal/claudehook"
	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/gitattr"
	"github.com/thrillmade/logmind/internal/gitcli"
	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/inserter"
	"github.com/thrillmade/logmind/internal/templates"
	"github.com/thrillmade/logmind/internal/timeline"
	"github.com/thrillmade/logmind/internal/tree"
	"github.com/thrillmade/logmind/internal/version"
)

type initFlags struct {
	noGit         bool
	agentsList    string
	allAgents     bool
	githubActions bool
	spec          bool
	refresh       bool
}

func newInitCmd() *cobra.Command {
	f := &initFlags{githubActions: true}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize logmind in the current project",
		Long: "Initialize logmind in the current project. Creates docs/,\n" +
			"template files, AGENTS.md block, per-agent stubs, GitHub\n" +
			"workflows, .gitignore + .gitattributes blocks, hooks, and\n" +
			"the first decision log entry. Idempotent: re-running on an\n" +
			"already-initialised repo only refreshes templates + hooks.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd, f)
		},
	}
	cmd.Flags().BoolVar(&f.noGit, "no-git", false, "Skip git operations (don't commit initialization).")
	cmd.Flags().StringVar(&f.agentsList, "agents", "", "Comma-separated list of agents to configure (e.g., claude,cursor,windsurf).")
	cmd.Flags().BoolVar(&f.allAgents, "all-agents", false, "Configure all supported AI agents.")
	cmd.Flags().BoolVar(&f.githubActions, "github-actions", true, "Install logmind GitHub Actions (decision aggregator, link checker). Default on.")
	cmd.Flags().BoolVar(&f.spec, "spec", false,
		"Scaffold docs/spec.md (only if absent) and set context.spec_file: docs/spec.md in .logmind/config.yml (only if unset). Idempotent; works in both fresh-install and refresh mode.")
	cmd.Flags().BoolVar(&f.refresh, "refresh", false,
		"Re-render the GitHub workflow files logmind owns even when their template version already matches — the fix for a scaffolded `on: push:` branch filter that went stale (e.g. the default branch was renamed). DISCARDS any edits you made to those files: they are rewritten from the bundled template, and each discard is named in the output. Never touches a workflow that carries no logmind template marker, and still refuses to downgrade one carrying a newer marker.")
	return cmd
}

func runInit(cmd *cobra.Command, f *initFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	docsPath := filepath.Join(cwd, "docs")
	logmindDir := filepath.Join(cwd, ".logmind")
	configPath := filepath.Join(logmindDir, "config.yml")

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Initializing logmind...")
	fmt.Fprintln(out)

	// Parse agents list once, up front — both the refresh path and the
	// fresh-install path below need it to decide whether the Claude Code
	// PreToolUse guard (Layer 1 of commit enforcement; internal/claudehook)
	// gets installed.
	enabled := enabledAgentList(f.agentsList, f.allAgents)
	claudeAgentEnabled := slices.Contains(enabled, "claude")

	alreadyInit := pathExists(filepath.Join(docsPath, "decisions.md")) && pathExists(configPath)
	if alreadyInit {
		return runInitRefresh(cmd, f, cwd, docsPath, claudeAgentEnabled)
	}

	// Git repo guard — mirror Python's no-git/--no-git interaction.
	if !f.noGit && !gitcli.IsRepo(cwd) {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"Warning: Not a git repository. Initialize git first with 'git init' or use --no-git flag.")
		// Non-interactive: bail rather than prompt-and-spin. Users get a
		// deterministic exit. Python would prompt; we keep this safer for
		// scripted invocations.
		f.noGit = true
	}

	// docs/
	if err := os.MkdirAll(docsPath, 0o755); err != nil {
		return fmt.Errorf("create docs/: %w", err)
	}
	fmt.Fprintln(out, "✓ Created docs/")

	if err := writeFile(filepath.Join(docsPath, "decisions.md"), templates.DecisionsTemplate()); err != nil {
		return err
	}
	fmt.Fprintln(out, "✓ Created docs/decisions.md")

	if err := writeFile(filepath.Join(docsPath, "decisions-archive.md"), templates.DecisionsArchiveTemplate()); err != nil {
		return err
	}
	fmt.Fprintln(out, "✓ Created docs/decisions-archive.md")

	// file-structure.md + timeline.md are seeded here but get rewritten
	// AFTER the first decision is logged. Python emits the placeholder
	// "✓ Created" line in this position so output parity needs the
	// echo here even though the actual canonical body lands later. The
	// tree-walk uses the depth-2 default that matches Python v0.5.0+.
	if _, err := tree.WriteFileStructure(filepath.Join(docsPath, "file-structure.md"), cwd, 2); err != nil {
		_ = writeFile(filepath.Join(docsPath, "file-structure.md"), templates.FileStructureTemplate())
	}
	fmt.Fprintln(out, "✓ Created docs/file-structure.md")

	if body, err := timeline.Generate(docsPath, cmd.ErrOrStderr()); err == nil {
		_ = writeFile(filepath.Join(docsPath, "timeline.md"), body)
	} else {
		_ = writeFile(filepath.Join(docsPath, "timeline.md"), "# Timeline\n")
	}
	fmt.Fprintln(out, "✓ Created docs/timeline.md")

	// .logmind/config.yml (verbatim template).
	if err := os.MkdirAll(logmindDir, 0o755); err != nil {
		return fmt.Errorf("create .logmind/: %w", err)
	}
	if err := writeFile(configPath, templates.ConfigTemplate()); err != nil {
		return err
	}
	fmt.Fprintln(out, "✓ Created .logmind/config.yml")

	// docs/spec.md + context.spec_file — opt-in (H2 of the canonical-spec-file
	// feature). specCreated gates whether it joins the commit-file list below.
	var specCreated bool
	if f.spec {
		specCreated = applyInitSpec(cmd, cwd)
	}

	// Agent files — for now, ensure AGENTS.md slim block + per-agent stubs.
	// The Python init runs insert_into_all_ai_files; we delegate to the
	// EnsureAgentsMD path which writes the slim variant by default and
	// CreateAgentFile for each enabled agent's stub.
	// A first-time init normally CREATES AGENTS.md, but a repo can already
	// carry a block written by a newer binary (a partially-migrated fleet,
	// #257) — so this surface reports the refusal too.
	//
	// unwritten counts the artifacts init WANTED and could not have. It is
	// what stops the two summary lines at the bottom from claiming a success
	// that did not happen (#306) — before it existed, an AGENTS.md or a
	// workflow that refused to be written was a discarded error and a
	// "logmind initialized successfully!".
	unwritten := 0
	msg, agentsDeclined, err := inserter.EnsureAgentsMD(cwd)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "note: AGENTS.md was NOT written —", err)
		unwritten++
	} else {
		if msg != "" {
			fmt.Fprintln(out, msg)
		}
		reportAgentsBlockRefusal(cmd.ErrOrStderr(), agentsDeclined)
	}
	for _, agentName := range enabled {
		filePath, err := inserter.CreateAgentFile(agentName, cwd)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: create agent file:", agentName, err)
			continue
		}
		if filePath == "" {
			continue
		}
		if rel, err := filepath.Rel(cwd, filePath); err == nil {
			fmt.Fprintln(out, "✓ Created", rel)
		}
	}

	// GitHub workflows. workflowsUnwritten is counted separately from the
	// `unwritten` total above because the receipt reports a workflows RATIO,
	// and an AGENTS.md that could not be written is not a workflow.
	var installedWorkflows []string
	workflowsUnwritten := 0
	var dependabotChanged bool
	if f.githubActions {
		// Fresh install: refresh=false never overwrites, so no VERSION or
		// OWNERSHIP decline can happen here — but a declineUnwritable can
		// (#306), and it must be reported and counted rather than aborting
		// the other templates.
		created, _, declined, err := installWorkflowTemplates(cwd, false)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: workflow install failed:", err)
			unwritten++
		}
		for _, wf := range created {
			fmt.Fprintln(out, "✓ Created", wf)
		}
		reportTemplateDowngrades(cmd.ErrOrStderr(), declined)
		workflowsUnwritten = unwritableCount(declined)
		unwritten += workflowsUnwritten
		installedWorkflows = created

		// .github/dependabot.yml — fresh install or merge. Keeps the
		// thrillmade/setup-logmind action ref current via Dependabot's
		// github-actions ecosystem. v1.1.0+ paired with the
		// setup-logmind action pattern in the workflow templates.
		dependabotChanged = applyDependabotInit(cmd, cwd)
	}

	// .gitignore block.
	gitignoreChanged, err := ensureGitignoreBlock(filepath.Join(cwd, ".gitignore"))
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: .gitignore update failed:", err)
	} else if gitignoreChanged {
		fmt.Fprintln(out, "✓ Added logmind block to .gitignore")
	}

	// .gitattributes block + merge driver config (when in a git repo).
	gitattrChanged, err := gitattr.EnsureBlock(filepath.Join(cwd, ".gitattributes"))
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: .gitattributes update failed:", err)
	} else if gitattrChanged {
		fmt.Fprintln(out, "✓ Added logmind block to .gitattributes")
	}
	if !f.noGit {
		_ = gitattr.ConfigureMergeDrivers(cwd)
		if _, err := hooks.InstallPostMerge(cwd); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: post-merge hook install failed:", err)
		}
		if _, err := hooks.InstallPostRewrite(cwd); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: post-rewrite hook install failed:", err)
		}
		if _, err := hooks.InstallCommitMsg(cwd); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: commit-msg hook install failed:", err)
		}
		// L2a of the derived-docs pin-preservation design (see
		// internal/cli/derived.go). Unconditional, alongside the other three
		// git hooks above — the v2.0.0 B6 `derived_docs.mode` adoption gate
		// that used to install this hook only on explicit opt-in is gone.
		if _, err := hooks.InstallPreCommit(cwd); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: pre-commit hook install failed:", err)
		}
	}

	// Layer 1 of commit enforcement: the Claude Code harness's PreToolUse
	// guard entry in .claude/settings.json. Gated on claudeAgentEnabled,
	// NOT on !f.noGit — .claude/settings.json is repo content, not git
	// state, so it installs even under --no-git or outside a git repo.
	claudeHookChanged := false
	if claudeAgentEnabled {
		changed, err := claudehook.EnsurePreToolUseGuard(cwd)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: Claude Code PreToolUse guard install failed:", err)
		} else if changed {
			claudeHookChanged = true
			fmt.Fprintln(out, "✓ Installed Claude Code guard-commit hook (.claude/settings.json)")
		}
	}

	// First decision log entry — append to docs/decisions.md.
	if err := logFirstDecision(docsPath); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: first decision log failed:", err)
	} else {
		fmt.Fprintln(out, "✓ Logged first decision: \"Initialize logmind decision tracking\"")
	}

	// Re-render file-structure.md + timeline.md AFTER all init artifacts
	// (workflows, agent files, .gitignore, .gitattributes, hooks, first
	// decision) are on disk. Mirrors Python's behaviour: the `log` call
	// inside log_first_decision triggers update_file_structure and the
	// timeline regen path. Without this re-render, the tree-walk only
	// sees docs/ and the timeline lacks the first decision row.
	_, _ = tree.WriteFileStructure(filepath.Join(docsPath, "file-structure.md"), cwd, 2)
	if body, err := timeline.Generate(docsPath, cmd.ErrOrStderr()); err == nil {
		_ = writeFile(filepath.Join(docsPath, "timeline.md"), body)
	}

	// Commit everything (best-effort; never fatal).
	if !f.noGit {
		var filesToCommit []string
		filesToCommit = append(filesToCommit,
			"docs/decisions.md",
			"docs/decisions-archive.md",
			"docs/file-structure.md",
			"docs/timeline.md",
			".logmind/config.yml",
		)
		filesToCommit = append(filesToCommit, installedWorkflows...)
		if gitignoreChanged {
			filesToCommit = append(filesToCommit, ".gitignore")
		}
		if gitattrChanged {
			filesToCommit = append(filesToCommit, ".gitattributes")
		}
		if dependabotChanged {
			filesToCommit = append(filesToCommit, ".github/dependabot.yml")
		}
		if claudeHookChanged {
			filesToCommit = append(filesToCommit, ".claude/settings.json")
		}
		if specCreated {
			filesToCommit = append(filesToCommit, "docs/spec.md")
		}
		for _, agent := range enabled {
			if rel, ok := agents.FilePath(agent, cwd); ok && pathExists(rel) {
				if relPath, err := filepath.Rel(cwd, rel); err == nil {
					filesToCommit = append(filesToCommit, relPath)
				}
			}
		}
		if pathExists(filepath.Join(cwd, "AGENTS.md")) {
			filesToCommit = append(filesToCommit, "AGENTS.md")
		}
		if err := commitInitialFiles(cwd, filesToCommit, "logmind: Initialize decision tracking"); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: Failed to commit:", err)
		} else {
			fmt.Fprintln(out, "✓ Committed changes: \"logmind: Initialize decision tracking\"")
		}
	}

	// THE SUMMARY MUST MATCH WHAT LANDED (#306). Both of these lines used to
	// be unconditional, so a run that installed one of four workflows — or
	// none — still said "successfully" and still listed `workflows` among the
	// things it had written. The happy path is byte-identical to what it has
	// always been; only a run with something unwritten reads differently, and
	// it names the count so the notes on stderr can be found.
	fmt.Fprintln(out)
	if unwritten > 0 {
		fmt.Fprintf(out, "logmind initialized, but %d artifact(s) could NOT be written — see the notes on stderr.\n", unwritten)
	} else {
		fmt.Fprintln(out, "logmind initialized successfully!")
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Start logging decisions with:")
	fmt.Fprintln(out, "  logmind log \"Your decision here\" -r \"why\" -a \"alternative\" -i \"implication\"")
	fmt.Fprintln(out)
	if unwritten > 0 {
		fmt.Fprintf(out, "ok initialized: docs/ .logmind/ workflows=%d/%d unwritten=%d @v%s\n",
			len(installedWorkflows), len(installedWorkflows)+workflowsUnwritten, unwritten, version.Version)
	} else {
		fmt.Fprintf(out, "ok initialized: docs/ .logmind/ workflows @v%s\n", version.Version)
	}
	return nil
}

// runInitRefresh handles the idempotent re-init path. Mirrors Python's
// already_initialized branch in cli.init: refresh workflows + AGENTS.md
// marker + .gitattributes + git config + hooks, leave docs/ and
// .logmind/ alone.
func runInitRefresh(cmd *cobra.Command, f *initFlags, cwd, docsPath string, claudeAgentEnabled bool) error {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "logmind is already initialized — running in refresh mode.")
	fmt.Fprintln(out)

	// docs/spec.md + context.spec_file — --spec works in refresh mode too
	// (H2 design point): a repo that ran `logmind init` before this feature
	// existed can still opt in later without a fresh install.
	if f.spec {
		applyInitSpec(cmd, cwd)
	}

	// Workflow templates. Exactly ONE of the two paths below writes
	// .github/workflows/:
	//
	//   - default: applyRefresh's version-ordered refresh, shared verbatim
	//     with `doctor --fix`.
	//   - --refresh: a forced re-render of the files logmind owns. Kept out
	//     of applyRefresh deliberately — `doctor --fix`'s contract is "move
	//     a stale marker forward", and a repair the user asked for by name
	//     must not become something a drift-remediation pass does on its
	//     own. See workflowForceRender for why a version-ordered refresh
	//     cannot repair a stale scaffold-time render.
	res, err := applyRefresh(cwd, refreshOpts{
		githubActions:      f.githubActions && !f.refresh,
		git:                true,
		claudeAgentEnabled: claudeAgentEnabled,
	})
	var overwritten []string
	if f.githubActions && f.refresh {
		created, refreshed, declined, over, werr := installWorkflowTemplatesMode(cwd, workflowForceRender)
		res.WorkflowsCreated, res.WorkflowsRefreshed, res.WorkflowsDeclined = created, refreshed, declined
		overwritten = over
		if err == nil {
			err = werr
		}
	}
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: refresh failed:", err)
	}

	if f.githubActions {
		discarded := map[string]bool{}
		for _, wf := range overwritten {
			discarded[wf] = true
		}
		for _, wf := range res.WorkflowsCreated {
			fmt.Fprintln(out, "✓ Created", wf)
		}
		for _, wf := range res.WorkflowsRefreshed {
			if discarded[wf] {
				// DISCLOSURE, at the moment content is actually discarded.
				// logmind owns this file (its marker says so) so overwriting
				// it is defensible — but a user who had edited it otherwise
				// learns nothing at all, and "↻ Refreshed … to current
				// template" reads as a no-op when it was not one.
				fmt.Fprintln(out, "↻ Re-rendered", wf,
					"from the bundled template — previous contents replaced; run `git diff` to see what was discarded")
				continue
			}
			fmt.Fprintln(out, "↻ Refreshed", wf, "to current template")
		}
		// A refused downgrade (#286) is neither a create nor a refresh, but
		// it IS a decision the user has to know about — report it before the
		// "already current" line, which would otherwise be the only thing a
		// repo running ahead of the binary ever sees.
		reportTemplateDowngrades(cmd.ErrOrStderr(), res.WorkflowsDeclined)
		// "already current" is a CLAIM about the whole set, so it may only
		// be printed when the whole set was actually accounted for. A
		// markerless file (the user's own, never touched), a displaced
		// marker, a refused downgrade and a file that could not be written
		// are all workflows this run did not bring to the bundled template,
		// and saying "all current" over any of them is false.
		//
		// ONE OWNER FOR "WHICH FILES DID THIS RUN NOT WRITE": the declined
		// list, populated inside installWorkflowTemplatesMode. A second
		// out-of-band scan for the markerless subset (this used to be a
		// separate `unownedWorkflows(cwd)` walk) is the same fact read
		// twice — it printed its own note for a file the loop had already
		// reported, and it re-derived ownership from a second extractor,
		// which is exactly the split #299 closed.
		if len(res.WorkflowsCreated) == 0 && len(res.WorkflowsRefreshed) == 0 && len(res.WorkflowsDeclined) == 0 {
			fmt.Fprintln(out, "  All workflow templates already current.")
		}
		// Dependabot config is init-owned (doctor doesn't probe it, so
		// applyRefresh leaves it alone); keep init's own merge semantics +
		// existing-ecosystem nudge here.
		_ = applyDependabotInit(cmd, cwd)
	}

	if res.AgentsMDMsg != "" {
		fmt.Fprintln(out, res.AgentsMDMsg)
	}
	// #267's analogue of the workflow refusal above: a block this binary
	// can't move forward is silence on stdout, so stderr carries it.
	reportAgentsBlockRefusal(cmd.ErrOrStderr(), res.AgentsMDDeclined)
	if res.GitattrChanged {
		fmt.Fprintln(out, "✓ Added logmind block to .gitattributes")
	}
	for _, h := range res.HooksRefreshed {
		fmt.Fprintln(out, "✓ Refreshed .git/hooks/"+h)
	}
	if res.ClaudeHookChanged {
		fmt.Fprintln(out, "✓ Refreshed .claude/settings.json (Claude Code guard-commit hook)")
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Done. docs/ and .logmind/ left untouched.")

	// A workflow that could not be written is the command not doing its
	// job, and the summary above has already printed every file that WAS
	// written. Exiting 0 here is what would make that summary read as the
	// whole story — the same shape as a regenerator reporting success while
	// pushing nothing. The other refusal (a declined downgrade) is a
	// deliberate no-op and stays exit 0, as are the two ownership refusals.
	//
	// unwritablePaths, not a fourth open-coded `d.Err != nil` loop: the exit
	// code and the stderr note must never disagree about which files failed.
	failed := unwritablePaths(res.WorkflowsDeclined)
	if len(failed) > 0 {
		return fmt.Errorf("%d workflow file(s) could not be written: %s (see the notes above)",
			len(failed), strings.Join(failed, ", "))
	}
	return nil
}

// declineReason says WHY installWorkflowTemplatesMode left an existing file
// alone. One list with a reason, rather than one list per reason: two lists
// that both mean "we declined to write this path" are two lists that will
// disagree about whether a path is on them.
type declineReason int

const (
	// declineDowngrade — the file carries a NEWER marker than this binary
	// bundles (#286). Ours, but ahead of us.
	declineDowngrade declineReason = iota
	// declineUnmarked — no logmind marker at all. SPEC §5.2: the artifact
	// belongs to the user and MUST NOT be overwritten.
	declineUnmarked
	// declineDisplaced — a marker exists but not on line 1 (#299). Neither
	// clearly ours nor clearly theirs, so it is refused and named rather
	// than guessed at.
	declineDisplaced
	// declineUnwritable — logmind wanted this file and could not have it
	// (#306): a symlink at the destination, an unreadable existing file, an
	// I/O failure. Not a judgement about the file's marker like the three
	// above, but it lands on the SAME list for the reason the type comment
	// gives — every one of them means "we did not write this path", and two
	// lists that mean that are two lists that will disagree.
	//
	// It is on the list rather than being an early `return err` because a
	// refusal on ONE template used to abandon the other three: with a symlink
	// at check-decisions.yml (first alphabetically), `init` exited 0, said
	// "logmind initialized successfully!", and never installed
	// check-doc-links, regen-timeline or logmind-self-update at all.
	declineUnwritable
)

// templateDowngrade records one workflow this run did NOT write, and why.
// Reported by both callers (init refresh mode, doctor --fix) — never
// silent. FOUR causes, ONE list deliberately — see declineReason:
//
//   - declineDowngrade (#286): the file on disk carries a newer
//     `# logmind-template-version:` marker than this binary bundles, so it
//     was left alone on purpose. Err is nil.
//   - declineUnmarked / declineDisplaced (#299, SPEC §5.2): the file is the
//     user's, or cannot be told apart from the user's. Err is nil.
//   - declineUnwritable (#306): a read or write that FAILED — a planted
//     symlink refused by atomicio, a permission error, a full disk. Err
//     carries it, and Err is what every surface discriminates on.
//
// A second list for any of these would be the same defect in a new shape: a
// caller that reports one and forgets the other, so a workflow that was not
// installed vanishes from the summary. There is one slice, so every surface
// that reports refusals reports all four.
type templateDowngrade struct {
	Path      string        // rel path from repo root
	Installed string        // marker found on disk, e.g. "v11"; "" when there is none
	Bundled   string        // marker this binary ships, e.g. "v4"
	Reason    declineReason // why the write was refused
	Line      int           // 1-based line the marker sat on; only meaningful for declineDisplaced
	Err       error         // the refusal itself; only set for declineUnwritable
}

// workflowInstallMode selects how installWorkflowTemplatesMode treats a
// workflow file that ALREADY exists. Creating a missing file is the same
// in all three.
type workflowInstallMode int

const (
	// workflowCreateOnly: never overwrite an existing file. Fresh `logmind
	// init`.
	workflowCreateOnly workflowInstallMode = iota
	// workflowRefreshStale: overwrite when the installed marker is OLDER
	// than the bundled one. `logmind init` in refresh mode, `doctor --fix`.
	workflowRefreshStale
	// workflowForceRender: additionally re-render files whose marker
	// MATCHES the bundled one. `logmind init --refresh`.
	//
	// This exists because a workflow can be stale at the current version:
	// the `on: push:` branch filter is rendered at scaffold time (an `on:`
	// filter takes no expression), so renaming the default branch leaves a
	// v12 file that is version-current and wired to a branch that no longer
	// exists. The regen then silently stops firing — which is exactly what
	// the shipped workflows' drift warning tells the user to run `logmind
	// init --refresh` to repair. A version-ordered refresh cannot repair
	// it, because no version moved.
	//
	// It does NOT widen ownership: a file with no `# logmind-template-version:`
	// marker is the user's and is still never touched (SPEC §5.2), and a
	// marker NEWER than the bundled one is still declined rather than
	// downgraded (#286).
	workflowForceRender
)

// installWorkflowTemplates is the boolean-shaped entry point kept for the
// two existing callers (fresh init, applyRefresh). See
// installWorkflowTemplatesMode for the real thing.
func installWorkflowTemplates(repoRoot string, refresh bool) ([]string, []string, []templateDowngrade, error) {
	mode := workflowCreateOnly
	if refresh {
		mode = workflowRefreshStale
	}
	// The fifth return is dropped deliberately: only workflowForceRender can
	// overwrite a file whose content the user may have changed, and neither
	// mode reachable here is that one, so it is always empty.
	created, refreshed, declined, _, err := installWorkflowTemplatesMode(repoRoot, mode)
	return created, refreshed, declined, err
}

// installWorkflowTemplatesMode copies
// internal/templates/github/*.yml.template into .github/workflows/,
// treating existing files according to mode (see workflowInstallMode).
//
// Returns (created, refreshed, declined, overwritten, err). Each list is a
// slice of relative paths from cwd; declined additionally names both
// markers, or the error that stopped the write.
//
// `overwritten` is the subset of `refreshed` whose previous content was NOT
// what this binary would have written and whose marker was already current
// — i.e. a file somebody edited, or one carrying a stale scaffold-time
// render. Only workflowForceRender can produce it, and it is separate from
// `refreshed` so the caller can DISCLOSE the discard rather than print the
// same "↻ Refreshed" line it prints for a version bump.
func installWorkflowTemplatesMode(repoRoot string, mode workflowInstallMode) ([]string, []string, []templateDowngrade, []string, error) {
	workflowsDir := filepath.Join(repoRoot, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		// Nothing can be written at all — not a per-file refusal.
		return nil, nil, nil, nil, err
	}
	var created, refreshed, overwritten []string
	var declined []templateDowngrade
	defaultBranch := gitcli.DefaultBranch(repoRoot)
	for _, tmpl := range templates.ListWorkflowTemplates() {
		// `tmpl` includes the `.template` suffix; strip for the install name.
		targetName := strings.TrimSuffix(tmpl, ".template")
		target := filepath.Join(workflowsDir, targetName)
		body := renderWorkflowTemplate(templates.Workflow(tmpl), defaultBranch)
		rel := relativePath(repoRoot, target)
		// RECORD AND CONTINUE, at every failure site in this loop. Returning
		// on the first one abandons every workflow after it in the list —
		// and the callers below print their summary from the returned
		// slices, so a run that installed nothing after the third template
		// still reported the first two as the whole story. One unwritable
		// file must cost that file, not the rest of the fleet's workflows.
		// Refuse a symlink BEFORE reading, not just before writing. The
		// ownership decision below is made from the file's own marker, and
		// os.ReadFile resolves the final component — so a link pointing at
		// something outside the repository has that file's contents answer
		// "is this workflow logmind's?". Whatever the answer, it was
		// answered about the wrong file. A symlink in .github/workflows/ is
		// not a workflow logmind manages, full stop.
		if err := atomicio.RefuseSymlink(target); err != nil {
			declined = append(declined, templateDowngrade{Path: rel, Reason: declineUnwritable, Err: err})
			continue
		}
		existing, err := os.ReadFile(target)
		if errors.Is(err, fs.ErrNotExist) {
			// ENOENT-AS-ABSENT (#306), and why this branch is reached at all
			// only for a genuinely missing file. os.ReadFile FOLLOWS a
			// symlink, so a DANGLING symlink at target — one pointing at a
			// path that does not exist — reports fs.ErrNotExist exactly as an
			// absent file does; a bare os.WriteFile then follows that same
			// link and creates the rendered workflow wherever it points,
			// possibly outside the repo, while --fix reports `workflows=1` as
			// though it had installed one. The RefuseSymlink above now
			// declines that case before the read, so the two are no longer
			// confusable here.
			//
			// atomicio.WriteFile stays anyway, and not as belt-and-braces: the
			// Lstat above and the write below are two syscalls, so a link
			// planted between them would still be followed by os.WriteFile.
			// It also writes temp-file-plus-rename (never a truncated stub)
			// and makes its own parent directory.
			if err := atomicio.WriteFile(target, []byte(body), 0o644); err != nil {
				declined = append(declined, templateDowngrade{
					Path:   rel,
					Reason: declineUnwritable,
					Err:    err,
				})
				continue
			}
			created = append(created, rel)
			continue
		}
		if err != nil {
			declined = append(declined, templateDowngrade{
				Path:   rel,
				Reason: declineUnwritable,
				Err:    err,
			})
			continue
		}
		if mode == workflowCreateOnly {
			continue
		}
		// OWNERSHIP FIRST (#299), AND IT BINDS EVERY MODE INCLUDING THE
		// FORCED ONE (SPEC §5.2). Whether we may write this path at all is a
		// separate question from whether its version is behind ours, and it
		// is answered by the same extractor `doctor` reports from — so a file
		// doctor calls the user's can no longer be one --fix overwrites. The
		// `# logmind-template-version:` marker on line 1 is the only thing
		// that makes a file in .github/workflows/ logmind's to rewrite; a
		// file that has none — the user stripped it, or wrote their own
		// workflow under a name we happen to ship — belongs to them.
		// `--refresh` is a bigger hammer, not a wider claim.
		//
		// The refusal is RECORDED rather than silently skipped: "left it
		// alone because it is yours" is correct and invisible, and a run that
		// skipped one still printed "All workflow templates already current",
		// which is a claim about a set it had just declined to act on.
		installed := inserter.ExtractTemplateMarker(string(existing))
		bundledVer := inserter.ExtractTemplateMarker(body).Version
		if !installed.Writable() {
			reason := declineUnmarked
			if installed.Ownership == inserter.MarkerDisplaced {
				reason = declineDisplaced
			}
			declined = append(declined, templateDowngrade{
				Path:      rel,
				Installed: installed.Version,
				Bundled:   bundledVer,
				Reason:    reason,
				Line:      installed.Line,
			})
			continue
		}
		installedVer := installed.Version
		if bundledVer == "" {
			// The BUNDLED side lost its marker. MarkerOwned guarantees
			// installedVer is non-empty, so this is the only remaining
			// empty case, and it is ours, not the user's: with no bundled
			// version there is nothing to order against and nothing to force
			// to, so every mode leaves the file alone rather than rendering
			// over it on a comparison that cannot be made.
			continue
		}
		if installedVer != bundledVer {
			// ORDER, not inequality (#286). A released binary bundles OLDER
			// markers than dev (`brew install logmind` → v1.2.0 → v4 while
			// dev is on v11), so an inequality test makes every refresh run
			// from a release walk a repo that is deliberately ahead of it
			// BACKWARDS — silently, and reported as "Refreshed … to current
			// template". Refuse that direction and report it instead.
			//
			// Unparseable on either side falls through to the old
			// refresh-on-inequality behaviour: an unreadable marker must not
			// become a way to pin a stale template forever.
			installedN, iok := parseTemplateVersion(installedVer)
			bundledN, bok := parseTemplateVersion(bundledVer)
			if iok && bok && installedN > bundledN {
				declined = append(declined, templateDowngrade{
					Path:      rel,
					Installed: installedVer,
					Bundled:   bundledVer,
					Reason:    declineDowngrade,
				})
				continue
			}
			// The refresh branch's symlink case (#306): a NON-dangling
			// symlink at target reads fine through os.ReadFile above and
			// carries a real, stale-looking marker, so a bare os.WriteFile
			// would rewrite whatever file the link points at — a file logmind
			// did not install and does not own. The RefuseSymlink at the top
			// of the loop declines it before the read; atomicio.WriteFile is
			// what closes the window between that check and this write.
			if err := atomicio.WriteFile(target, []byte(body), 0o644); err != nil {
				declined = append(declined, templateDowngrade{
					Path:      rel,
					Installed: installedVer,
					Bundled:   bundledVer,
					Reason:    declineUnwritable,
					Err:       err,
				})
				continue
			}
			refreshed = append(refreshed, rel)
			continue
		}
		// Markers match. A version-ordered refresh stops here — there is no
		// generation to move the file to. `--refresh` does not, because the
		// scaffold-time render (the `on: push:` branch filter) can be stale
		// at the current version and nothing else can repair it.
		if mode != workflowForceRender {
			continue
		}
		if string(existing) == body {
			// Already exactly what this binary would write. Re-writing it
			// would report "↻ Refreshed" for a file nothing changed about,
			// which trains the reader to ignore the line.
			continue
		}
		// This file is version-current but its CONTENT differs, which on
		// this path means one of two things: a stale scaffold-time render
		// (the case --refresh exists for), or an edit the user made. logmind
		// owns the file either way — the marker says so — but overwriting
		// somebody's edit while printing the same "↻ Refreshed" line a
		// no-op re-render prints is how they find out by not finding out.
		// The line below is the disclosure; `--refresh`'s flag help carries
		// the same warning before the fact.
		if err := atomicio.WriteFile(target, []byte(body), 0o644); err != nil {
			declined = append(declined, templateDowngrade{Path: rel, Reason: declineUnwritable, Err: err})
			continue
		}
		refreshed = append(refreshed, rel)
		overwritten = append(overwritten, rel)
	}
	return created, refreshed, declined, overwritten, nil
}

// renderWorkflowTemplate substitutes install-time placeholders:
//
//	__LOGMIND_VERSION__        → the current binary's version constant
//	__LOGMIND_DEFAULT_BRANCH__ → this repository's default branch
//
// NOTE: as of the v12/v9 template generation NO bundled template contains
// __LOGMIND_VERSION__ — the version pin it existed for went away when CI
// moved to `thrillmade/setup-logmind`, which resolves `latest` itself. The
// substitution is kept as a working extension point, but nothing exercises
// it against a real template today, and init_test.go's "never lands on
// disk" assertion for it currently cannot fail. Filed rather than removed
// here; removing it is a separate call.
//
// The default branch is substituted here, at scaffold time, because a
// workflow's `on:` trigger CANNOT take an expression — GitHub evaluates no
// context under `on:`, so `branches: [${{ ... }}]` is not a thing. The
// alternatives were both worse: hardcoding `main` breaks every repo whose
// default branch is `master`/`trunk`/anything else (the assumption this
// removes), and broadening the filter to `branches: ['**']` would put a
// SECOND `check-derived-docs` check run on every pull-request head SHA —
// one skipped by its own `if:`, and a conditionally-skipped job reports
// SUCCESS rather than skipped. Whether that is a bypassed gate or merely a
// misleading green is the repo owner's configuration, not this function's:
// `check-derived-docs` blocks by failing the run, and only a repo that has
// added it to a ruleset's required status checks (logmind's own `main`
// ruleset has not) turns that green into a merge.
//
// Everything the workflow does at RUNTIME still reads the live
// `github.event.repository.default_branch`, so a stale value here can only
// cost a trigger, never a wrong-ref write — and the PR gate compares the
// two and warns when they drift.
func renderWorkflowTemplate(text, defaultBranch string) string {
	text = strings.ReplaceAll(text, "__LOGMIND_VERSION__", version.Version)
	return strings.ReplaceAll(text, "__LOGMIND_DEFAULT_BRANCH__", defaultBranch)
}

// NOTE on the branch value: gitcli.DefaultBranch owns this fact and its
// documented 5-step resolution ends in a hard "main" fallback, so it never
// returns "". This file deliberately does NOT wrap it in a second
// empty-check — a duplicated fallback is a second copy of the same rule
// that reads as a safety net while being unreachable, and the invariant
// belongs to the resolver, not to every caller. What depends on it (an
// empty value renders `branches: []`, a filter matching no branch, which
// is a silently dead workflow rather than a merely wrong one) is pinned by
// TestRenderedWorkflow_TriggersOnTheRepositorysOwnBranch.

// extractTemplateVersion is GONE — see inserter.ExtractTemplateMarker (#299).
//
// It prefix-matched every line of the file, while internal/doctor matched an
// anchored regex against line 1 only. A workflow whose marker sat on line 2
// was markerless to the reporter and versioned to the writer, so `doctor`
// printed "markerless" and `doctor --fix` then overwrote the file — the
// component that DECIDED the rule was not the component that APPLIED it.
// Both now call one extractor, which also settles a second disagreement the
// two had: on `# logmind-template-version: v1 junk`, the prefix form yielded
// "v1 junk" as the version token and the regex form yielded "v1".

// parseTemplateVersion extracts the ORDERING key from a template marker:
// "v11" → 11, "v9-pointer" → 9. Only the numeric generation orders — a
// string compare gets this exactly backwards ("v11" < "v4" lexically),
// which is the trap #286 fell into.
//
// Deliberately NOT version.SatisfiesMin: these markers are a single
// monotonic integer, not a major.minor.patch triple, so that helper can't
// parse them at all. The variant suffix (the "-pointer" flavour tag, and
// the "-FAKE" markers the tests plant) is stripped before the compare,
// mirroring parseVersionCore's suffix handling.
//
// Returns ok=false for anything that isn't a leading "v" followed by at
// least one digit — callers fall back to the pre-#286 behaviour rather
// than treating an unreadable marker as a reason to do nothing.
//
// Delegates to inserter.ParseMarkerGeneration so the workflow-template
// guard (#286) and the AGENTS.md block guard (#267) order marker
// generations by one rule rather than two copies of it.
func parseTemplateVersion(marker string) (int, bool) {
	return inserter.ParseMarkerGeneration(marker)
}

// enabledAgentList resolves the agents flag. --all-agents wins;
// otherwise --agents (comma-separated) wins; otherwise defaults to
// agents.DefaultEnabled().
func enabledAgentList(flag string, all bool) []string {
	if all {
		return agents.Names()
	}
	if flag != "" {
		var out []string
		for _, name := range strings.Split(flag, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			out = append(out, name)
		}
		return out
	}
	return agents.DefaultEnabled()
}

// writeFile writes content to path, creating parent directories.
// Uses 0o644 perms — matches Python's open(...).write defaults.
//
// Delegates to atomicio.WriteFile (temp-file-plus-rename, refuses a
// symlink at path) rather than a bare os.WriteFile — this helper is the
// one that lands .logmind/config.yml on a fresh `logmind init`, so a
// symlink planted at that path before init runs must not turn into an
// arbitrary-write primitive.
func writeFile(path, content string) error {
	return atomicio.WriteFile(path, []byte(content), 0o644)
}

// relativePath returns target's path relative to repoRoot. Best-effort:
// returns target unchanged on failure.
func relativePath(repoRoot, target string) string {
	if rel, err := filepath.Rel(repoRoot, target); err == nil {
		return rel
	}
	return target
}

// ensureGitignoreBlock — port of src/logmind/core/gitignore.ensure_block.
// Marker-bracketed block append; idempotent.
func ensureGitignoreBlock(path string) (bool, error) {
	const startMarker = "# >>> logmind >>>"
	const endMarker = "# <<< logmind <<<"
	defaultLines := []string{".logmind/cache/", ".logmind/.lock"}

	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if strings.Contains(existing, startMarker) {
		return false, nil
	}
	var b strings.Builder
	b.WriteString(startMarker)
	for _, line := range defaultLines {
		b.WriteByte('\n')
		b.WriteString(line)
	}
	b.WriteByte('\n')
	b.WriteString(endMarker)
	b.WriteByte('\n')
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	if existing != "" && !strings.HasSuffix(existing, "\n\n") {
		existing += "\n"
	}
	// Through writeFile (atomicio) for the same reason as the workflow
	// installs above: os.ReadFile at the top of this function follows a
	// symlink, so a link planted at .gitignore reads as an ordinary file and
	// a bare os.WriteFile would append logmind's block into whatever it
	// points at. atomicio.WriteFile refuses, and makes its own parent
	// directory — so the explicit MkdirAll this replaced is no longer needed.
	if err := writeFile(path, existing+b.String()); err != nil {
		return false, err
	}
	return true, nil
}

// logFirstDecision appends an initial decision entry to
// docs/decisions.md. Mirrors src/logmind/core/logger.log_first_decision —
// a single dated header + reasoning block introducing logmind tracking.
//
// Byte-format note: the entry is appended immediately after the `---`
// header separator in the template, no intervening blank line, so the
// output matches Python's logger.log line-for-line.
func logFirstDecision(docsPath string) error {
	path := filepath.Join(docsPath, "decisions.md")
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	entry := buildFirstDecisionEntry()
	// The template ends with `---\n`. Append entry directly so the
	// `## YYYY-MM-DD ...` line sits on the next line, matching Python.
	newContent := strings.TrimRight(string(existing), "\n") + "\n" + entry + "\n"
	// atomicio via writeFile: docs/decisions.md is the decision record itself,
	// so a torn write here loses history, and a symlink at the path would send
	// the first decision somewhere logmind does not own.
	return writeFile(path, newContent)
}

// buildFirstDecisionEntry renders the same markdown shape Python's
// log_first_decision produces. Byte-identical wording to
// src/logmind/core/logger.log_first_decision — uses an ASCII dash
// `-` in the header (not em-dash), the same reasoning sentence, and
// the same alternatives + implications lines. Date format matches
// Python's datetime.now().strftime("%Y-%m-%d %H:%M").
//
// The trailing `\n---\n` separator mirrors the Python logger's
// append behaviour — terminates each entry with `---` so the next
// log call cleanly inserts a new block.
func buildFirstDecisionEntry() string {
	now := time.Now().Format("2006-01-02 15:04")
	return fmt.Sprintf(
		"## %s - Initialize logmind decision tracking\n\n"+
			"**Reasoning:** Starting structured decision logging for this project to maintain clear documentation of architectural choices and provide context for AI agents.\n\n"+
			"**Alternatives considered:** Manual decision documentation, ADR (Architecture Decision Records)\n\n"+
			"**Implications:**\n"+
			"- All significant decisions should now be logged using `logmind.log()`\n"+
			"- AI agents will have access to decision history via docs/decisions.md\n"+
			"- Git history will serve as an audit trail for all decisions\n\n"+
			"---",
		now,
	)
}

// applyInitSpec implements `logmind init --spec` (H2 of the canonical-spec-
// file feature): scaffolds docs/spec.md from the embedded template ONLY if
// it's absent (never overwrites hand-edited content), and points
// context.spec_file at it in .logmind/config.yml ONLY if that key isn't
// already set (never overrides an explicit user choice — including a user
// who deliberately configured a DIFFERENT spec path). Both halves are
// independently idempotent, and both run the same way whether called from
// the fresh-install path or runInitRefresh — --spec is designed to work in
// either mode. Returns whether docs/spec.md was freshly created (the
// fresh-install caller uses this to decide whether to stage it for commit;
// refresh mode does not commit anything itself, so its caller ignores the
// return value).
func applyInitSpec(cmd *cobra.Command, cwd string) (specCreated bool) {
	out := cmd.OutOrStdout()
	specPath := filepath.Join(cwd, "docs", "spec.md")
	if pathExists(specPath) {
		// Present already — never overwrite (it may be hand-edited).
	} else if err := writeFile(specPath, templates.SpecTemplate()); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: create docs/spec.md failed:", err)
	} else {
		specCreated = true
		fmt.Fprintln(out, "✓ Created docs/spec.md")
	}

	merged, err := config.LoadAsMap(cwd)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: load config for --spec failed:", err)
		return specCreated
	}
	if _, alreadySet := config.GetPath(merged, "context.spec_file"); alreadySet {
		return specCreated // key already present (any value) — leave it alone
	}
	config.SetPath(merged, "context.spec_file", "docs/spec.md")
	if err := config.SaveMap(configPath(cwd), merged); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: set context.spec_file failed:", err)
		return specCreated
	}
	fmt.Fprintln(out, "✓ Set context.spec_file: docs/spec.md in .logmind/config.yml")
	return specCreated
}

// applyDependabotInit calls into inserter.EnsureDependabot and prints
// the right "Created" / "Merged" / "Already current" line per the
// returned result. Returns true when a write happened (so the caller
// can stage `.github/dependabot.yml` for the init commit).
//
// We log to stdout via the same `✓ ...` / `↻ ...` glyphs used by the
// surrounding init steps so output stays visually consistent. The
// "existing ecosystem" branch surfaces a one-line nudge so the user
// can opt into the thrillmade group manually if they want it — we
// don't auto-mutate a user-owned github-actions block (Dependabot
// rejects duplicate ecosystem+directory pairs).
func applyDependabotInit(cmd *cobra.Command, cwd string) bool {
	result, err := inserter.EnsureDependabot(cwd)
	out := cmd.OutOrStdout()
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: dependabot.yml setup failed:", err)
		return false
	}
	switch result {
	case inserter.DependabotCreated:
		fmt.Fprintln(out, "✓ Created .github/dependabot.yml")
		return true
	case inserter.DependabotMerged:
		fmt.Fprintln(out, "✓ Merged thrillmade group into .github/dependabot.yml")
		return true
	case inserter.DependabotExistingEcosystem:
		// Don't auto-mutate user-owned blocks. Surface a single nudge
		// so the user can choose to opt in by hand-editing.
		fmt.Fprintln(out,
			"  .github/dependabot.yml already covers github-actions — add a `thrillmade` group under `groups:` to bundle thrillmade/* action bumps into one PR (optional).")
		return false
	default:
		return false
	}
}

// commitInitialFiles stages + commits the listed paths. Best-effort:
// returns an error to the caller but never panics. Currently uses the
// gitcli helpers; --push and --auto-commit flags from .logmind/config.yml
// are NOT yet honored here (deferred to the log-command port).
func commitInitialFiles(repoRoot string, files []string, message string) error {
	if len(files) == 0 {
		return nil
	}
	if err := gitcli.AddPaths(repoRoot, files...); err != nil {
		return err
	}
	_, stderr, err := gitcli.RunCaptured(repoRoot, "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit: %s: %w", strings.TrimSpace(stderr), err)
	}
	return nil
}
