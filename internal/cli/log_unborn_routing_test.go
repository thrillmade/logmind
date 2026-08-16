// log_unborn_routing_test.go — pins the two git states that get confused
// for each other in SPEC §3.2's branch-routing rule: an UNBORN repo (git
// init, no commit yet) and a DETACHED HEAD.
//
// This file exists because the confusion was written down as fact. A
// correction to internal/cli/log.go's routing comment added "unborn repo"
// to the list of states that fall back to docs/decisions.md, and the claim
// was then replicated into seven places — including skill/SKILL.md and
// internal/templates/AGENTS.md.template, both shipped to consumers and read
// by agents as ground truth. Nothing tested it, so nothing contradicted it.
package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/testgit"
)

// TestResolveDecisionsPathUnbornVsDetached pins where `logmind log` puts an
// entry in each of the two states, as a PAIR.
//
// The truth, measured rather than inferred from a neighbouring comment:
// `git symbolic-ref --short HEAD` resolves HEAD's ref WITHOUT dereferencing
// it to a commit, so it SUCCEEDS before the first commit — a fresh `git
// init` answers with the branch HEAD already points at, exit 0, even while
// `git rev-parse --verify HEAD` fails. gitcli.CurrentBranch is therefore
// non-empty on an unborn repo and the entry routes to the branch file like
// any other branch. Detached HEAD is the case that yields "", because HEAD
// there holds a raw SHA rather than a ref.
//
// The two cases must run together. Detached is the control: it is the state
// that genuinely DOES produce docs/decisions.md. Asserting "unborn does not
// write docs/decisions.md" alone would still pass if the fallback path went
// dead entirely, or if `log` silently stopped writing anything — the
// control catches both, because it demands that same file be created with
// the entry inside it. Each case also asserts its own git precondition
// first, so neither can pass vacuously against a repo that is not in the
// state the case names.
//
// Pinned on the files `logmind log` leaves on disk rather than on
// resolveDecisionsPath directly: a unit test on that helper passes its own
// mutation and still goes green if a caller re-routes around it.
func TestResolveDecisionsPathUnbornVsDetached(t *testing.T) {
	mustGit := func(t *testing.T, dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// gitOK reports the exit status of a git command as a bool, for
	// asserting preconditions (a failing command is the signal here, not an
	// error to report).
	gitOK := func(dir string, args ...string) bool {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		return cmd.Run() == nil
	}

	const (
		branchFile = "docs/decisions-branches/main.md"
		legacyFile = "docs/decisions.md"
	)

	cases := []struct {
		name string
		// setup puts the repo into the state under test. Runs with cwd
		// already at dir, before docs/ is scaffolded.
		setup func(t *testing.T, dir string)
		// precondition asserts the repo really is in that state, so a
		// change to setup that quietly loses it fails loudly here rather
		// than making the routing assertion meaningless.
		precondition func(t *testing.T, dir string)
		summary      string
		wantFile     string
		wantAbsent   string
		why          string
	}{
		{
			name: "unborn repo routes to the branch file",
			setup: func(t *testing.T, dir string) {
				// --initial-branch=main keeps this independent of the
				// machine's init.defaultBranch. No commit is made: that is
				// the whole point of the case.
				testgit.InitRepo(t, dir, "--initial-branch=main")
				mustGit(t, dir, "config", "user.email", "test@example.com")
				mustGit(t, dir, "config", "user.name", "Test")
			},
			precondition: func(t *testing.T, dir string) {
				if gitOK(dir, "rev-parse", "--verify", "HEAD") {
					t.Fatal("precondition: repo has a commit — this case must run on an UNBORN repo, so it is no longer testing what it claims")
				}
				if !gitOK(dir, "symbolic-ref", "--short", "HEAD") {
					t.Fatal("precondition: symbolic-ref failed on an unborn repo — the premise of this whole test (that it succeeds pre-commit) no longer holds; re-measure before editing the routing docs")
				}
			},
			summary:    "Unborn routing probe",
			wantFile:   branchFile,
			wantAbsent: legacyFile,
			why:        "an unborn repo HAS a branch name (symbolic-ref answers pre-commit), so §3.2 routes it to the branch file — it is NOT a docs/decisions.md fallback case",
		},
		{
			name: "detached HEAD routes to the legacy file",
			setup: func(t *testing.T, dir string) {
				testgit.InitRepo(t, dir, "--initial-branch=main")
				mustGit(t, dir, "config", "user.email", "test@example.com")
				mustGit(t, dir, "config", "user.name", "Test")
				mustGit(t, dir, "config", "commit.gpgsign", "false")
				if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
					t.Fatalf("write seed: %v", err)
				}
				mustGit(t, dir, "add", "seed.txt")
				mustGit(t, dir, "commit", "-m", "seed")
				mustGit(t, dir, "checkout", "--detach", "HEAD")
			},
			precondition: func(t *testing.T, dir string) {
				if !gitOK(dir, "rev-parse", "--verify", "HEAD") {
					t.Fatal("precondition: repo is unborn — the detached case needs a commit to detach onto")
				}
				if gitOK(dir, "symbolic-ref", "--short", "HEAD") {
					t.Fatal("precondition: symbolic-ref succeeded — HEAD is not actually detached, so this control proves nothing")
				}
			},
			summary:    "Detached routing control",
			wantFile:   legacyFile,
			wantAbsent: branchFile,
			why:        "a detached HEAD has no branch NAME to name a file after, so it is one of the three genuine docs/decisions.md fallbacks",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := withTempCwd(t, func(d string) {
				tc.setup(t, d)
				tc.precondition(t, d)
				scaffoldDocs(t)
				withFakeTTY(t, false, func() {
					root := NewRootCmd()
					root.SetArgs([]string{"log", tc.summary, "-r", "Why", "--no-commit"})
					var out bytes.Buffer
					root.SetOut(&out)
					root.SetErr(&out)
					if err := root.Execute(); err != nil {
						t.Fatalf("log: %v\n%s", err, out.String())
					}
				})
			})

			body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(tc.wantFile)))
			if err != nil {
				t.Fatalf("expected the entry in %s (%s), but the file is unreadable: %v", tc.wantFile, tc.why, err)
			}
			if !strings.Contains(string(body), tc.summary) {
				t.Fatalf("%s exists but does not contain %q (%s); body:\n%s", tc.wantFile, tc.summary, tc.why, body)
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(tc.wantAbsent))); err == nil {
				t.Fatalf("%s was written, but %s", tc.wantAbsent, tc.why)
			}
		})
	}
}
