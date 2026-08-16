package gitcli

import (
	"os/exec"
	"testing"
)

// TestDefaultBranch_ResolvesRatherThanPrefersMain is the regression for the
// defect a fixed `main`-before-`master` preference carries: it answers
// "main" for a repository whose default branch is `master` the moment a
// stray local `main` exists — a leftover from a rename, or a branch created
// by reflex — because the old step 2 asked "does main exist?" and never
// "which of these is this repository's default?".
//
// This mattered little while the only caller was `logmind rebase` picking a
// base. It matters now: `logmind init` renders this answer into a workflow
// `on: push:` filter, and the wrong name there installs a check that
// silently never runs. Covering two names blindly (`branches: [main,
// master]`) used to hide the resolver behind an OR; rendering ONE name is
// only better if the one rendered is right.
//
// Each case names the rung it exercises. Nothing here touches ambient git
// config: `init.defaultBranch` is set REPO-LOCALLY where a case needs it,
// which overrides whatever the machine's global config says, so the
// assertions hold on a developer box whose Command Line Tools gitconfig
// already sets that key.
func TestDefaultBranch_ResolvesRatherThanPrefersMain(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, repo string)
		want  string
	}{
		{
			// THE PANEL'S CASE. Default branch is `master`, a stray local
			// `main` exists, there is no origin/HEAD. Pre-fix this returned
			// "main" and scaffolded `branches: [main]` into a `master` repo.
			name: "stray local main does not outvote the checked-out master",
			setup: func(t *testing.T, repo string) {
				runGit(t, repo, "branch", "-M", "master")
				runGit(t, repo, "branch", "main")
			},
			want: "master",
		},
		{
			// THE PANEL'S CONTROL. Same repo without the stray `main` — this
			// answered correctly before the fix too, and still must, or the
			// fix traded one wrong answer for another.
			name: "control: master alone",
			setup: func(t *testing.T, repo string) {
				runGit(t, repo, "branch", "-M", "master")
			},
			want: "master",
		},
		{
			// The common case, unchanged: `main` alone is `main`.
			name: "control: main alone",
			setup: func(t *testing.T, repo string) {
				runGit(t, repo, "branch", "-M", "main")
			},
			want: "main",
		},
		{
			// Tiebreak (a): the remote's branch set outranks local checkout
			// state. A clone of a `master` repo where somebody made a local
			// `main` and is sitting on it — origin/main does not exist, so
			// HEAD is the misleading signal and the remote is the true one.
			name: "the remote's branch set beats a stray local checkout",
			setup: func(t *testing.T, repo string) {
				runGit(t, repo, "branch", "-M", "master")
				// A remote-tracking ref for master only, no origin/HEAD.
				runGit(t, repo, "update-ref", "refs/remotes/origin/master", "HEAD")
				runGit(t, repo, "checkout", "-q", "-b", "main")
			},
			want: "master",
		},
		{
			// Tiebreak (b) again, the other way round: the same shape with
			// the roles swapped must answer "main", or the resolver is just
			// a `master`-first preference now.
			name: "checked-out main wins over a stray master",
			setup: func(t *testing.T, repo string) {
				runGit(t, repo, "branch", "-M", "main")
				runGit(t, repo, "branch", "master")
			},
			want: "main",
		},
		{
			// Tiebreak (c): both names exist, HEAD is on neither, but the
			// repo's own init.defaultBranch names one of them.
			name: "init.defaultBranch breaks the tie when HEAD cannot",
			setup: func(t *testing.T, repo string) {
				runGit(t, repo, "branch", "-M", "master")
				runGit(t, repo, "branch", "main")
				runGit(t, repo, "config", "init.defaultBranch", "master")
				runGit(t, repo, "checkout", "-q", "-b", "feat/x")
			},
			want: "master",
		},
		{
			// Tiebreak (d): both names exist, no origin, HEAD on neither,
			// and init.defaultBranch names a third branch entirely. The repo
			// has told us nothing, so the conventional order is all that is
			// left. Set repo-locally to `trunk` precisely so rung (c) cannot
			// fire off the machine's ambient config and make this vacuous.
			name: "no evidence at all falls back to the conventional order",
			setup: func(t *testing.T, repo string) {
				runGit(t, repo, "branch", "-M", "master")
				runGit(t, repo, "branch", "main")
				runGit(t, repo, "config", "init.defaultBranch", "trunk")
				runGit(t, repo, "checkout", "-q", "-b", "feat/x")
			},
			want: "main",
		},
		{
			// Step 1 is untouched: origin/HEAD, when set, is the answer and
			// nothing below it gets a vote — not even a checked-out `main`.
			name: "origin/HEAD still wins outright",
			setup: func(t *testing.T, repo string) {
				runGit(t, repo, "branch", "-M", "main")
				runGit(t, repo, "update-ref", "refs/remotes/origin/master", "HEAD")
				runGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/master")
			},
			want: "master",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := initRepo(t)
			tc.setup(t, repo)
			if got := DefaultBranch(repo); got != tc.want {
				t.Errorf("DefaultBranch = %q, want %q\nbranches: %s",
					got, tc.want, branchList(t, repo))
			}
		})
	}
}

// branchList renders every local and origin ref for a readable failure.
func branchList(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname)", "refs/heads/", "refs/remotes/")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "(could not list refs: " + err.Error() + ")"
	}
	return string(out)
}
