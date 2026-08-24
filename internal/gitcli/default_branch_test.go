package gitcli

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/testgit"
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

// TestDefaultBranch_UnbornHEADIsEvidence is the regression for step 4. A
// repo with no commits yet has no refs at all, so steps 1-3 come up empty
// and the search used to fall through to init.defaultBranch and then to the
// hard "main" — meaning `git init -b trunk && logmind init` scaffolded
// `branches: [main]` into a repo that has never had a branch by that name.
// Byte-identical, at exit 0, to what a repo actually on `main` gets. The
// workflows simply never fire. That is the README's own Quick Start on any
// repo whose default branch is neither `main` nor `master`.
//
// HEAD is the evidence steps 1-3 cannot see. `git symbolic-ref HEAD`
// SUCCEEDS before the first commit (see the CurrentBranch contract) and
// names the branch the first commit will create — which is exactly what the
// forge will call the default branch.
//
// The cases below pin the PLACE of that step as much as its existence: it
// must lose to origin/HEAD and to a single born branch, beat
// init.defaultBranch, and never fire for a HEAD that is merely checked out.
// Nothing here touches ambient git config — where a case needs
// `init.defaultBranch`, it is set REPO-LOCALLY, which overrides whatever the
// machine's global config says.
func TestDefaultBranch_UnbornHEADIsEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(t *testing.T) string
		want  string
	}{
		{
			// THE DEFECT. `git init -b trunk`, nothing else.
			name:  "unborn trunk repo resolves to trunk",
			build: func(t *testing.T) string { return unbornRepo(t, "trunk") },
			want:  "trunk",
		},
		{
			// THE CONTROL. Same shape on the conventional name: the answer
			// was already right here and must stay right, or the fix has
			// merely moved which repos get a wrong workflow trigger.
			name:  "control: unborn main repo still resolves to main",
			build: func(t *testing.T) string { return unbornRepo(t, "main") },
			want:  "main",
		},
		{
			// Step 4 sits BELOW step 1. A clone of a `main` repo, then an
			// orphan checkout, leaves origin/HEAD naming `main` while the
			// local HEAD is unborn on `trunk` and no local branch survives —
			// so this is the one shape where the two steps actually compete.
			// The remote's declared default outranks a local ref that has
			// never been pushed.
			name: "origin/HEAD outranks an unborn local HEAD",
			build: func(t *testing.T) string {
				src := initRepo(t)
				runGit(t, src, "branch", "-M", "main")
				dst := filepath.Join(t.TempDir(), "clone")
				testgit.CloneRepo(t, dst, "-q", src)
				runGit(t, dst, "checkout", "-q", "--orphan", "trunk")
				runGit(t, dst, "branch", "-q", "-D", "main")
				// Non-vacuity: if the orphan checkout stopped leaving HEAD
				// unborn, or the local `main` survived, steps 2-3 would
				// answer and this case would assert nothing about step 1.
				if got := unbornHEAD(dst); got != "trunk" {
					t.Fatalf("setup: want an unborn local HEAD at trunk, got %q — "+
						"step 1 and step 4 would not be competing here", got)
				}
				return dst
			},
			want: "main",
		},
		{
			// Step 4 sits BELOW step 3. Commits on `develop`, no origin, no
			// conventional name: the single born branch is the default, and
			// an unborn HEAD cannot arise here at all because HEAD names it.
			name: "commits on develop with no origin resolve to develop",
			build: func(t *testing.T) string {
				repo := initRepo(t)
				runGit(t, repo, "branch", "-M", "develop")
				return repo
			},
			want: "develop",
		},
		{
			// Step 4 sits ABOVE init.defaultBranch. That key is what `git
			// init` CONSULTED in order to write HEAD, and `-b` overrules it;
			// HEAD is the answer, the config is the guess it overruled. Set
			// repo-locally so the case tests the rung deliberately instead of
			// depending on what the machine's global config happens to say.
			name: "unborn HEAD beats a conflicting init.defaultBranch",
			build: func(t *testing.T) string {
				repo := unbornRepo(t, "trunk")
				runGit(t, repo, "config", "init.defaultBranch", "master")
				return repo
			},
			want: "trunk",
		},
		{
			// The UNBORN guard, without which every feature branch becomes
			// its own default and onNonDefaultBranch (internal/cli) collapses
			// to false everywhere. Two born branches, neither conventional,
			// no origin: steps 1-3 all decline, so an unguarded HEAD read
			// would answer `feat/x` here. It must not — HEAD is only evidence
			// when it names a branch that does not exist yet.
			name: "a checked-out born branch is not a default branch",
			build: func(t *testing.T) string {
				repo := initRepo(t)
				runGit(t, repo, "branch", "-M", "develop")
				runGit(t, repo, "checkout", "-q", "-b", "feat/x")
				runGit(t, repo, "config", "init.defaultBranch", "develop")
				return repo
			},
			want: "develop",
		},
		{
			// A HEAD outside refs/heads/ is not a branch, unborn or
			// otherwise — git will commit to refs/custom/x quite happily and
			// `symbolic-ref --short` shortens it to a branch-looking
			// `custom/x`. Step 4 reads the FULL ref for this reason, so the
			// search falls through to init.defaultBranch as it would for a
			// detached HEAD.
			name: "a HEAD outside refs/heads is not an unborn branch",
			build: func(t *testing.T) string {
				repo := initRepo(t)
				// -M first: initRepo takes whatever the machine's ambient
				// init.defaultBranch names, and the delete below names a ref.
				runGit(t, repo, "branch", "-M", "main")
				runGit(t, repo, "update-ref", "refs/custom/x", "HEAD")
				runGit(t, repo, "symbolic-ref", "HEAD", "refs/custom/x")
				// Plumbing, not `git branch -D`: porcelain refuses to delete
				// the branch it thinks is checked out, and the point of the
				// case is that refs/heads/ ends up empty.
				runGit(t, repo, "update-ref", "-d", "refs/heads/main")
				runGit(t, repo, "config", "init.defaultBranch", "develop")
				return repo
			},
			want: "develop",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.build(t)
			if got := DefaultBranch(repo); got != tc.want {
				t.Errorf("DefaultBranch = %q, want %q\nHEAD: %s\nbranches: %s",
					got, tc.want, headRef(t, repo), branchList(t, repo))
			}
		})
	}
}

// TestDefaultBranch_OrphanCheckoutDoesNotWinInAnEstablishedRepo is the
// regression for a false premise step 4's own doc comment used to state:
// "Unborn is the one state where HEAD is ... the only branch this
// repository has." That is only true when refs/heads/ is EMPTY. A repo
// that already has branches and then runs `git checkout --orphan` also
// leaves HEAD unborn — on a name that is NOT the repository's only
// branch, just a new one nobody has committed to yet, sitting alongside
// branches that already exist. Pre-gate, step 4 answered with the orphan
// name anyway, so `git checkout --orphan gh-pages` in a repo with commits
// on `develop` and `feature` (no origin) silently retargeted the
// scaffolded workflow trigger at `gh-pages`.
func TestDefaultBranch_OrphanCheckoutDoesNotWinInAnEstablishedRepo(t *testing.T) {
	repo := initRepo(t)
	runGit(t, repo, "branch", "-M", "develop")
	runGit(t, repo, "branch", "feature")

	// Non-vacuity (part 1): HEAD is born, on develop, before the orphan
	// checkout — the repo is genuinely established, not already unborn.
	if got := unbornHEAD(repo); got != "" {
		t.Fatalf("setup: want HEAD born before the orphan checkout, got unborn %q", got)
	}

	runGit(t, repo, "checkout", "-q", "--orphan", "gh-pages")

	// Non-vacuity (part 2): the checkout actually left HEAD unborn on
	// gh-pages — the exact shape the gate has to tell apart from a
	// genuinely branchless repo, or this test asserts nothing about step 4.
	if got := unbornHEAD(repo); got != "gh-pages" {
		t.Fatalf("setup: want an unborn local HEAD at gh-pages after the orphan checkout, got %q — "+
			"step 4 would not be in play here", got)
	}

	if got := DefaultBranch(repo); got == "gh-pages" {
		t.Errorf("DefaultBranch = %q; an orphan checkout inside an established repo "+
			"(develop, feature already exist, no origin) must not become the default\n"+
			"branches: %s", got, branchList(t, repo))
	}
}

// unbornRepo creates a repo whose HEAD points at branch and that has no
// commits — the state `git init -b <branch>` leaves behind, and the one
// every `logmind init` in the README's Quick Start runs against.
func unbornRepo(t *testing.T, branch string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	testgit.InitRepo(t, dir, "-q", "-b", branch)
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	return dir
}

// headRef renders HEAD's own symbolic ref for a readable failure — the
// evidence step 4 reads, which branchList cannot show for an unborn repo.
func headRef(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "symbolic-ref", "HEAD")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "(detached or unreadable: " + err.Error() + ")"
	}
	return strings.TrimSpace(string(out))
}
