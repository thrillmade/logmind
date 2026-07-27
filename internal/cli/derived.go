package cli

import (
	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/gitcli"
)

// derivedDocPaths are the two committed, purely-derived context docs governed by
// the zero-conflict invariant: on any non-default branch they MUST stay
// byte-identical to their merge-base-with-main version (the branch never edits
// them), so git's 3-way merge is conflict-free. They are regenerated only at the
// integration point (main). Repo-relative, forward-slash (git pathspec form).
var derivedDocPaths = []string{"docs/timeline.md", "docs/file-structure.md"}

// onNonDefaultBranch reports whether cwd is a git repo currently on a branch
// other than the default branch. Best-effort: false on a non-repo, detached
// HEAD, unborn branch, or unknown default — the conservative answer, since every
// caller uses `true` to ENABLE the extra invariant guard and `false` preserves
// pre-v2.0.0 behavior.
func onNonDefaultBranch(cwd string) bool {
	if !gitcli.IsRepo(cwd) {
		return false
	}
	cur := gitcli.CurrentBranch(cwd)
	if cur == "" {
		return false
	}
	def := gitcli.DefaultBranch(cwd)
	if def == "" {
		return false
	}
	return cur != def
}

// integrationPointMode resolves cwd's derived_docs.mode (default "driver")
// from .logmind/config.yml and reports whether this repo has explicitly
// opted into "integration-point" mode — the v2.0.0 B6 adoption-signal gate
// that replaced the unconditional-by-default git.pin_derived_docs. Gates
// ALL FOUR layers of the derived-docs pin-preservation design:
//
//   - L0 — the post-merge/post-rewrite hook BODIES (hooks.BuildPostMergeBody
//     / BuildPostRewriteBody) regenerate on every branch in driver mode (the
//     pre-v2.0.0 behavior) and skip non-default branches only in
//     integration-point mode. Those hooks run as standalone `sh` scripts
//     with no Go process to call this function from, so the equivalent
//     check is inlined there as a `grep` against .logmind/config.yml — same
//     contract, different runtime.
//   - L1 — logmind log's commitDecision restore (log.go).
//   - L2a — the pre-commit hook's installation (hooks.InstallPreCommit,
//     wired in init.go/refresh.go).
//   - L2b — the harness-layer restore (guard_commit.go's
//     guardCommitHarness).
//
// Why the inversion: the protocol owner's HOLD found L0/L1 applying
// UNCONDITIONALLY (keyed only on branch name, no per-repo signal) silently
// broke a driver-mode repo's L0 branch regeneration the moment any
// contributor upgraded to a v2 binary, and violated the SPEC's
// backward-compatibility rule (an old tool + the new blocking CI gate
// produced an unwinnable combination). "driver" is now the default —
// adoption of the zero-conflict invariant must be explicit per repo.
//
// config.Load failure ⇒ "driver" (false) — the SAFE default now: a config
// problem must never silently flip a repo's regen/restore behavior. An
// empty or unrecognised Mode value ALSO resolves to "driver" (exact match
// against "integration-point" only, nothing else) — config.Load never
// errors or crashes over a bad Mode string; see DerivedDocsConfig.Mode.
func integrationPointMode(cwd string) bool {
	cfg, _ := config.Load(cwd)
	return cfg.DerivedDocs.Mode == "integration-point"
}

// unstagedDerivedDocPaths filters paths (callers always pass derivedDocPaths,
// or a subset of it) down to the ones that do NOT currently have a staged
// change relative to HEAD (gitcli.IsPathStaged). Shared by commitDecision's
// L1 restore (log.go) and guardCommitHarness's L2b restore
// (guard_commit.go) — see either call site for the full seam this closes:
// `logmind warp`'s merge-base repair (runWarp, warp.go) deliberately STAGES
// the two derived docs so the fix survives into the next commit, and an
// unconditional restore-to-HEAD would silently undo it, re-committing the
// very divergence the CI gate's remediation advice ("run `logmind warp`,
// then `logmind log`") told the user to fix.
//
// The rule, in one line: unstaged means accidental → revert it; staged
// means intentional → leave it alone. That's what staging means everywhere
// else in git, so it needs no special-casing beyond this filter.
//
// L1's own call site (log.go's commitDecision) carries the full write-up of
// the accepted trade-off this relaxation makes; the short version: a user
// who hand-`git add`s a DIVERGENT derived doc now also sails past this
// filter, because "staged" can't be told apart from "staged AND correct" on
// this offline hot path. L3 (the CI check-derived-docs gate) remains the
// backstop for that case.
func unstagedDerivedDocPaths(repoRoot string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if !gitcli.IsPathStaged(repoRoot, p) {
			out = append(out, p)
		}
	}
	return out
}
