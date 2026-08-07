// refresh.go — the shared, idempotent "bring this repo back to spec"
// remediation sequence, used by both `logmind init` (refresh mode) and
// `logmind doctor --fix`.
//
// It re-installs exactly the on-disk artifacts that `logmind doctor`
// probes for drift: the workflow templates, the AGENTS.md marker block,
// the .gitattributes logmind block, the per-clone merge-driver git config,
// and the four managed git hooks (post-merge / post-rewrite / commit-msg /
// pre-commit).
//
// It deliberately does NOT:
//   - write or rewrite docs/ decision content, docs/timeline.md, or
//     docs/file-structure.md (those are content / derived docs, not
//     install state);
//   - touch .logmind/config.yml;
//   - clobber a foreign (markerless) git hook — hooks.Install* refuse to
//     overwrite a hook that lacks the logmind marker, so a hand-written
//     hook is left alone and surfaces as residual drift instead;
//   - manage .github/dependabot.yml (doctor does not probe it; init keeps
//     its own dependabot UX).
//
// v2.0.0 addition: the Claude Code harness's PreToolUse guard entry in
// .claude/settings.json (Layer 1 of commit enforcement; see
// internal/claudehook). Unlike the git hooks, this write is gated on
// opts.claudeAgentEnabled rather than opts.git — .claude/settings.json is
// repo content, not git-clone state, so it should install even under
// --no-git, and should NOT install when the claude agent is disabled.
package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/thrillmade/logmind/internal/claudehook"
	"github.com/thrillmade/logmind/internal/gitattr"
	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/inserter"
)

// refreshResult tallies what applyRefresh actually changed. It drives both
// init's "✓/↻" lines and doctor --fix's quiet `ok` summary.
type refreshResult struct {
	WorkflowsCreated   []string            // rel paths
	WorkflowsRefreshed []string            // rel paths
	WorkflowsDeclined  []templateDowngrade // refused downgrades (#286) — MUST be reported
	AgentsMDMsg        string              // "" when no change
	GitattrChanged     bool
	MergeDriverSet     bool     // a merge-driver git config key was (re)written
	HooksRefreshed     []string // subset of {"post-merge","post-rewrite","commit-msg"}
	ClaudeHookChanged  bool     // .claude/settings.json PreToolUse guard was created/refreshed
}

// refreshOpts gates the write surfaces that need a repo/CI context.
type refreshOpts struct {
	githubActions      bool // install/refresh .github/workflows/*
	git                bool // configure merge drivers + install git hooks (no-op outside a git repo)
	claudeAgentEnabled bool // install/refresh .claude/settings.json PreToolUse guard (Layer 1)
}

// applyRefresh runs every idempotent installer that brings a drifted repo
// back to spec, reporting what changed. It never writes docs/ content or a
// foreign hook (the underlying installers enforce the latter).
//
// All steps run best-effort; the FIRST hard write error (from the
// content-bearing installers: workflows, AGENTS.md, .gitattributes) is
// returned without aborting the remaining steps, so a caller can choose to
// warn-and-continue (init) or fail (doctor --fix). Merge-driver and hook
// installers are merge-time optimizations that swallow their own per-item
// errors, matching the existing init behavior.
func applyRefresh(cwd string, opts refreshOpts) (refreshResult, error) {
	var res refreshResult
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if opts.githubActions {
		created, refreshed, declined, err := installWorkflowTemplates(cwd, true)
		res.WorkflowsCreated = created
		res.WorkflowsRefreshed = refreshed
		res.WorkflowsDeclined = declined
		note(err)
	}

	// AGENTS.md marker block + .gitattributes block are repo-content, not
	// git-state, so they run regardless of opts.git.
	msg, err := inserter.EnsureAgentsMD(cwd)
	if err == nil {
		res.AgentsMDMsg = msg
	}
	note(err)

	changed, err := gitattr.EnsureBlock(filepath.Join(cwd, ".gitattributes"))
	if err == nil && changed {
		res.GitattrChanged = true
	}
	note(err)

	if opts.git {
		res.MergeDriverSet = gitattr.ConfigureMergeDrivers(cwd)
		if c, _ := hooks.InstallPostMerge(cwd); c {
			res.HooksRefreshed = append(res.HooksRefreshed, "post-merge")
		}
		if c, _ := hooks.InstallPostRewrite(cwd); c {
			res.HooksRefreshed = append(res.HooksRefreshed, "post-rewrite")
		}
		if c, _ := hooks.InstallCommitMsg(cwd); c {
			res.HooksRefreshed = append(res.HooksRefreshed, "commit-msg")
		}
		// L2a (pin-preservation) — unconditional, alongside the three hooks
		// above. The v2.0.0 B6 `derived_docs.mode` adoption gate that used
		// to install this hook only on explicit opt-in is gone.
		if c, _ := hooks.InstallPreCommit(cwd); c {
			res.HooksRefreshed = append(res.HooksRefreshed, "pre-commit")
		}
	}

	if opts.claudeAgentEnabled {
		// Error deliberately swallowed, matching the git-hook installers
		// above: a malformed (e.g. JSONC-style / trailing-comma)
		// .claude/settings.json is user content EnsurePreToolUseGuard
		// refuses to touch — feeding that refusal into firstErr would turn
		// `doctor --fix` into a persistent exit-1 (suppressing the summary
		// line, the branch-summary backfill, and the residual re-probe)
		// over a file --fix can't repair anyway. Instead it degrades like
		// a foreign git hook does: the doctor probe classifies an
		// unparseable settings.json as "missing" (benign), and the guard
		// installs the moment the user fixes their JSON.
		if changed, _ := claudehook.EnsurePreToolUseGuard(cwd); changed {
			res.ClaudeHookChanged = true
		}
	}

	return res, firstErr
}

// reportTemplateDowngrades writes one stderr line per refused workflow
// refresh. Called by BOTH refresh surfaces (init refresh mode, doctor
// --fix) — the refusal is the whole point of #286, so it can never be
// left to a caller's discretion.
//
// Shape follows SPEC §3.4's rule for the analogous fail-open case ("Failing
// open MUST NOT be silent... MUST say so on stderr, naming what it looked
// for and what it found"): name the file, both markers, and the direction.
func reportTemplateDowngrades(stderr io.Writer, declined []templateDowngrade) {
	for _, d := range declined {
		fmt.Fprintf(stderr,
			"note: %s left unchanged — installed template %s is NEWER than the %s this binary bundles; "+
				"refusing to downgrade. Upgrade logmind to move it forward.\n",
			d.Path, d.Installed, d.Bundled)
	}
}
