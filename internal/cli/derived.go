package cli

import (
	"github.com/thrillmade/logmind/internal/gitcli"
)

// derivedDocPaths are the committed, purely-derived context docs governed by
// the zero-conflict invariant: on any non-default branch they MUST stay
// byte-identical to their merge-base-with-main version (the branch never edits
// them), so git's 3-way merge is conflict-free. They are regenerated only at the
// integration point (main). Repo-relative, forward-slash (git pathspec form).
//
// docs/timeline-archive.md is the older half of the SPEC §3.3 rendering split
// and is governed exactly as docs/timeline.md is — §3.3 names all three:
// "A non-default branch MUST NOT modify any derived file — the history, its
// archive, or the map." Every restore path takes this list rather than naming
// files itself, so a doc added here is picked up by all of them at once.
var derivedDocPaths = []string{"docs/timeline.md", "docs/timeline-archive.md", "docs/file-structure.md"}

// onNonDefaultBranch reports whether cwd is a git repo currently on a branch
// other than the default branch. Best-effort: false on a non-repo, a
// detached HEAD (CurrentBranch == ""), an unresolved default (DefaultBranch
// == ""), or simply because the current branch already IS the default — the
// conservative answer in each case, since every caller uses `true` to ENABLE
// the extra invariant guard and `false` preserves pre-v2.0.0 behavior.
//
// An unborn repo is NOT its own case, despite reading like one at a glance:
// CurrentBranch resolves HEAD's ref before the first commit (see
// decisions.NonBranchSources), so `git init -b main` returns false because
// cur == def (both "main") — exactly as it would after a commit. Round 9
// removed this same false "no branch name" premise from decisions.go,
// AGENTS.md.template and docs/plan.md; this comment was the ninth copy
// round 10's panel found still standing.
//
// It took until gitcli.DefaultBranch step 4 for the OTHER half to hold. The
// line above used to add that `git init -b trunk` returns true, "exactly as
// it would after a commit" — and that was the one illustration in it that
// was false both ways round: DefaultBranch answered "main" for an unborn
// `trunk` repo (so: true), while after a single commit the single-branch
// step answers "trunk" (so: false). Now both sides read the same evidence
// and an unborn repo genuinely does behave as it will once committed.
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

// unstagedDerivedDocPaths filters paths (callers always pass derivedDocPaths,
// or a subset of it) down to the ones that do NOT currently have a staged
// change relative to HEAD (gitcli.IsPathStaged). Shared by commitDecision's
// L1 restore (log.go) and guardCommitHarness's L2b restore
// (guard_commit.go) — see either call site for the full seam this closes:
// `logmind warp`'s merge-base repair (runWarp, warp.go) deliberately STAGES
// the derived docs so the fix survives into the next commit, and an
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

// divergedDerivedDocPaths filters paths down to the ones whose COMMITTED
// content actually differs from the merge-base pin — i.e. the ones a repair
// has something to fix. Used by runWarp (warp.go) to decide whether to
// repair at all.
//
// Why this exists: v2.0.0's unconditional-invariant change deleted the
// `derived_docs.mode` gate, and that gate's `if` was carrying TWO conditions
// that looked like one — "has this repo adopted the mode" (correctly deleted)
// and "does this branch actually need repairing" (should never have been).
// Losing the second made warp repair every branch, healthy or not, which
// silently discarded the read-refresh warp performs in the same command: it
// reported "refreshed N derived doc(s) from origin/main" and then overwrote
// them back to the merge-base one statement later.
//
// The deeper reason it must be conditional is the §0.4.1.2 handoff. A repair
// surface signals "this modification is deliberate" by STAGING its output,
// and the commit-path surfaces (L1, L2b) leave staged docs alone on that
// basis. If repair fires on every branch, that signal is asserted
// everywhere — and a signal that always fires carries no information. Repair
// has to be rare for "deliberate" to mean anything.
//
// Compares HEAD against mergeBase rather than the working tree, because it
// is the COMMITTED copy the invariant constrains: a branch violates the pin
// by committing a change to these files, not by having one on disk. That
// also makes this correct in warp's exact situation — the read-refresh has
// already written main's tip content into the working tree by the time this
// runs, so a working-tree comparison would see the refresh itself as
// divergence and "repair" it away, reintroducing the bug this fixes.
//
// gitcli.DefaultBranchMergeBase falls back to "HEAD" when it cannot compute a
// trustworthy base (no remote-tracking ref). That degrades correctly here
// with no special case: HEAD compared against HEAD yields no divergence, so
// no repair runs — the conservative outcome when we cannot tell.
func divergedDerivedDocPaths(repoRoot, mergeBase string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		head, headOK := gitcli.ShowFile(repoRoot, "HEAD", p)
		base, baseOK := gitcli.ShowFile(repoRoot, mergeBase, p)
		// A path tracked at one ref but not the other has diverged. Both
		// missing (a repo that never scaffolded that doc) has not.
		if headOK != baseOK || (headOK && head != base) {
			out = append(out, p)
		}
	}
	return out
}
