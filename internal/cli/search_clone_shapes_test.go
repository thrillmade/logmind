// search_clone_shapes_test.go — the regression for BLOCK 1: `logmind search`
// found the default branch's decisions only where `origin/HEAD` happened to
// be set.
//
// `searchSources` used to build the default branch's path out of
// gitcli.DefaultBranch. That resolver's fallback chain ends
// "…→ single-branch repo → that branch IS the default → 'main'", so wherever
// origin/HEAD is unset the resolved name collapsed onto the CURRENT branch (or
// onto a main.md that does not exist) and the default branch's file dropped
// silently out of the scan while sitting on disk.
//
// Every case here is a REAL git repository in one of the shapes that has no
// origin/HEAD, driven through the real `logmind search` command, asserting on
// what the command PRINTS. A test on searchSources' return value would pass its
// own mutation and still go green when the bug ships again, because the harm
// is in the output.
//
// NOTE ON FIXTURES: no case seeds a default branch through
// `init.defaultBranch`. Apple's Command Line Tools ship
// `init.defaultBranch = main` in their own gitconfig, so on macOS that knob
// answers "main" for every directory and a test written against it passes for
// the wrong reason. Branches are created by name (`git init -b`, `git checkout
// -b`) and the clone shapes are produced by real `git clone` invocations.
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// withCwd runs fn with the process working directory set to dir, restoring it
// afterwards. The sibling helper withTempCwd only ever creates a BARE temp
// dir; these cases need to cd into a git repository built by real clone
// plumbing, which withTempCwd cannot express.
func withCwd(t *testing.T, dir string, fn func()) {
	t.Helper()
	origin, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir %s: %v", dir, err)
	}
	defer func() { _ = os.Chdir(origin) }()
	fn()
}

// gitIn runs one git command in dir, failing the test on error.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// gitOut runs one git command in dir and returns trimmed stdout, or "" if it
// failed (used for probes like `symbolic-ref`, which legitimately fails when
// origin/HEAD is unset).
func gitOut(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// seedDecisionFile writes a docs/decisions-branches/<stem>.md holding one
// decision whose text contains a unique probe token.
func seedDecisionFile(t *testing.T, repo, stem, token string) {
	t.Helper()
	mustMkdir(t, filepath.Join(repo, "docs", "decisions-branches"))
	mustWrite(t, filepath.Join(repo, "docs", "decisions-branches", stem+".md"),
		"# Decisions\n\n## 2025-01-01 09:00 - "+token+" was decided here\n\n**Reasoning:** "+token+"\n\n---\n")
}

// newSearchOriginRepo builds the shared "origin" repository: a default branch
// (named defaultBranch) carrying defaultToken, plus a feature branch carrying
// featureToken. Returns its path.
func newSearchOriginRepo(t *testing.T, defaultBranch, defaultToken, featureToken string) string {
	t.Helper()
	origin := t.TempDir()
	gitIn(t, origin, "init", "-q", "-b", defaultBranch, ".")
	gitIn(t, origin, "config", "user.email", "t@example.com")
	gitIn(t, origin, "config", "user.name", "t")
	seedDecisionFile(t, origin, defaultBranch, defaultToken)
	seedDecisionFile(t, origin, "feat__ci", featureToken)
	gitIn(t, origin, "add", "-A")
	gitIn(t, origin, "commit", "-qm", "seed")
	gitIn(t, origin, "checkout", "-q", "-b", "feat/ci")
	gitIn(t, origin, "commit", "-q", "--allow-empty", "-m", "feature work")
	gitIn(t, origin, "checkout", "-q", defaultBranch)
	return origin
}

// TestSearch_FindsDefaultBranchHistory_InEveryCloneShape is the BLOCK 1
// regression, run as the panel's control pair plus the shape both the panel
// and the template lane converged on.
//
// Each case asserts BOTH terms through the same command in the same
// repository: the DEFAULT branch's decision (the regression) and the CURRENT
// branch's own decision (the control). A zero on the first while the second
// hits is a scope bug, not a broken fixture.
func TestSearch_FindsDefaultBranchHistory_InEveryCloneShape(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	const (
		defaultToken = "ZZQDEFAULTBRANCHDECISION"
		featureToken = "ZZQFEATUREBRANCHDECISION"
	)

	cases := []struct {
		name string
		// build returns the working copy to search from, checked out on a
		// non-default branch, and the decision file the default branch's
		// history lives in.
		build func(t *testing.T) (workdir, defaultFile string)
	}{
		{
			// origin/HEAD is NOT set by a single-branch clone. This is the
			// case that returned matches=0 sources=0 with main.md on disk.
			name: "single-branch clone, no origin/HEAD",
			build: func(t *testing.T) (string, string) {
				origin := newSearchOriginRepo(t, "main", defaultToken, featureToken)
				work := filepath.Join(t.TempDir(), "clone")
				gitIn(t, t.TempDir(), "clone", "-q", "--single-branch", "--branch", "feat/ci", origin, work)
				return work, "docs/decisions-branches/main.md"
			},
		},
		{
			// The control half of the pair: a normal clone DOES set
			// origin/HEAD, and this shape passed even before the fix.
			name: "normal clone, origin/HEAD set",
			build: func(t *testing.T) (string, string) {
				origin := newSearchOriginRepo(t, "main", defaultToken, featureToken)
				work := filepath.Join(t.TempDir(), "clone")
				gitIn(t, t.TempDir(), "clone", "-q", origin, work)
				gitIn(t, work, "checkout", "-q", "feat/ci")
				return work, "docs/decisions-branches/main.md"
			},
		},
		{
			// Every locally-created repo: `git init -b trunk` + a remote, and
			// no origin/HEAD anywhere. The default is not "main", so a
			// hardcoded "main" cannot pass either.
			name: "local trunk repo with a remote and no origin/HEAD",
			build: func(t *testing.T) (string, string) {
				origin := newSearchOriginRepo(t, "trunk", defaultToken, featureToken)
				work := t.TempDir()
				gitIn(t, work, "init", "-q", "-b", "trunk", ".")
				gitIn(t, work, "config", "user.email", "t@example.com")
				gitIn(t, work, "config", "user.name", "t")
				gitIn(t, work, "remote", "add", "origin", origin)
				seedDecisionFile(t, work, "trunk", defaultToken)
				seedDecisionFile(t, work, "feat__ci", featureToken)
				gitIn(t, work, "add", "-A")
				gitIn(t, work, "commit", "-qm", "seed")
				gitIn(t, work, "checkout", "-q", "-b", "feat/ci")
				return work, "docs/decisions-branches/trunk.md"
			},
		},
		{
			// The shape CI actually sees: a bare init, one fetched ref, a
			// branch checked out off it. No origin/HEAD.
			name: "actions/checkout shape",
			build: func(t *testing.T) (string, string) {
				origin := newSearchOriginRepo(t, "main", defaultToken, featureToken)
				work := t.TempDir()
				gitIn(t, work, "init", "-q", ".")
				gitIn(t, work, "config", "user.email", "t@example.com")
				gitIn(t, work, "config", "user.name", "t")
				gitIn(t, work, "remote", "add", "origin", origin)
				gitIn(t, work, "fetch", "-q", "--depth=1", "origin", "+refs/heads/feat/ci:refs/remotes/origin/feat/ci")
				gitIn(t, work, "checkout", "-q", "-b", "feat/ci", "refs/remotes/origin/feat/ci")
				return work, "docs/decisions-branches/main.md"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work, defaultFile := tc.build(t)

			// Precondition: the file we expect to be found IS on disk, so a
			// zero below is a search-scope failure and never a missing fixture.
			if !pathExists(filepath.Join(work, filepath.FromSlash(defaultFile))) {
				t.Fatalf("fixture precondition: %s is not on disk in %s", defaultFile, work)
			}
			// Precondition: the current branch is NOT the default branch,
			// which is where the loss was observable.
			if br := gitOut(work, "rev-parse", "--abbrev-ref", "HEAD"); br != "feat/ci" {
				t.Fatalf("fixture precondition: want to be on feat/ci, on %q", br)
			}

			withCwd(t, work, func() {
				body := runSearchCmd(t, defaultToken)
				if !strings.Contains(body, defaultFile) {
					t.Errorf("origin/HEAD=%q: the default branch's decision was not found.\nwant a hit in %s\ngot:\n%s",
						gitOut(work, "symbolic-ref", "refs/remotes/origin/HEAD"), defaultFile, body)
				}
				mustNotContain(t, body, "No matches found")

				// CONTROL, same repo and same command: the current branch's own
				// decision. If this ever goes to zero the fixture broke.
				control := runSearchCmd(t, featureToken)
				mustContain(t, control, "docs/decisions-branches/feat__ci.md")
				mustNotContain(t, control, "No matches found")
			})
		})
	}
}
