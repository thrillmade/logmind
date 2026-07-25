// refresh.go — the shared, idempotent "bring this repo back to spec"
// remediation sequence, used by both `logmind init` (refresh mode) and
// `logmind doctor --fix`.
//
// It re-installs exactly the on-disk artifacts that `logmind doctor`
// probes for drift: the workflow templates, the AGENTS.md marker block,
// the .gitattributes logmind block, the per-clone merge-driver git config,
// and the three managed git hooks (post-merge / post-rewrite / commit-msg).
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
	"path/filepath"

	"github.com/thrillmade/logmind/internal/claudehook"
	"github.com/thrillmade/logmind/internal/gitattr"
	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/inserter"
)

// refreshResult tallies what applyRefresh actually changed. It drives both
// init's "✓/↻" lines and doctor --fix's quiet `ok` summary.
type refreshResult struct {
	WorkflowsCreated   []string // rel paths
	WorkflowsRefreshed []string // rel paths
	AgentsMDMsg        string   // "" when no change
	GitattrChanged     bool
	MergeDriverSet     bool     // a merge-driver git config key was (re)written
	HooksRefreshed     []string // subset of {"post-merge","post-rewrite","commit-msg"}
	ClaudeHookChanged  bool     // .claude/settings.json PreToolUse guard was created/refreshed
}

// Changed reports whether applyRefresh wrote anything.
func (r refreshResult) Changed() bool {
	return len(r.WorkflowsCreated) > 0 || len(r.WorkflowsRefreshed) > 0 ||
		r.AgentsMDMsg != "" || r.GitattrChanged || r.MergeDriverSet ||
		len(r.HooksRefreshed) > 0 || r.ClaudeHookChanged
}

// refreshOpts gates the write surfaces that need a repo/CI context.
type refreshOpts struct {
	githubActions      bool // install/refresh .github/workflows/*
	git                bool // configure merge drivers + install git hooks (no-op outside a git repo)
	claudeAgentEnabled bool // install/refresh .claude/settings.json PreToolUse guard (Layer 1)
	// derivedDocsIntegrationPoint gates installing/refreshing L2a's
	// pre-commit hook (hooks.InstallPreCommit) — resolved from
	// derived_docs.mode == "integration-point" (default "driver"; see
	// internal/cli/derived.go's integrationPointMode). Only meaningful when
	// git is also true (same as the other 3 hooks).
	derivedDocsIntegrationPoint bool
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
		created, refreshed, err := installWorkflowTemplates(cwd, true)
		res.WorkflowsCreated = created
		res.WorkflowsRefreshed = refreshed
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
		// L2a (pin-preservation) — install ONLY on explicit opt-in
		// (derived_docs.mode: integration-point). Unlike the three hooks
		// above, this one is config-gated: a driver-mode repo (the default)
		// shouldn't have the hook silently installed/reinstalled on the
		// next `doctor --fix` / `init` refresh.
		if opts.derivedDocsIntegrationPoint {
			if c, _ := hooks.InstallPreCommit(cwd); c {
				res.HooksRefreshed = append(res.HooksRefreshed, "pre-commit")
			}
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
