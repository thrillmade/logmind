package cli

import "github.com/thrillmade/logmind/internal/gitcli"

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
