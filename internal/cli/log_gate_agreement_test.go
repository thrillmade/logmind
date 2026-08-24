// log_gate_agreement_test.go — the WRITER and the commit GATE must agree
// about WHERE a decision lives, and this file pins that agreement on the
// thing an operator can see: `logmind log` writes an entry, `git add`
// stages it, and `guard-commit` allows the commit that carries it.
//
// A unit test on either half alone passes its own mutation and still goes
// green while the two disagree — which is exactly how this shipped. The
// gate scoped its predicate to the literal `docs/decisions.md`; the writer
// builds its target from `filepath.Join(cwd, "docs")`, which is neither
// resolved against the git root nor spelled the way the filesystem spells
// it. Two configurations where logmind's own output was uncommittable:
//
//   - a repository that already had a `Docs/` directory on a case-folding
//     volume — `logmind log` reports success, git stages `Docs/decisions.md`,
//     and the gate exits 65 on a decision it just wrote;
//   - `logmind init` run below the git root (it exits 0 there), so the
//     entry lands at `pkg/api/docs/decisions.md` and the gate, which only
//     ever sees repo-root-relative paths, does not recognise it.
package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/guardcommit"
	"github.com/thrillmade/logmind/internal/testgit"
)

// foldsCase reports whether dir's filesystem answers to a spelling that is
// not the one on disk. The `Docs/` case below is only reachable where it
// does, so the test skips rather than asserting something the platform
// cannot produce.
func foldsCase(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatalf("mkdir probe: %v", err)
	}
	defer func() { _ = os.RemoveAll(probe) }()
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

func TestLogWritesWhereTheCommitGateLooks(t *testing.T) {
	mustGit := func(t *testing.T, dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	stagedNames := func(t *testing.T, dir string) []string {
		t.Helper()
		cmd := exec.Command("git", "diff", "--cached", "--name-only")
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git diff --cached: %v", err)
		}
		return strings.Fields(string(out))
	}

	cases := []struct {
		name string
		// layout puts the repository into the configuration under test and
		// returns the directory `logmind log` runs in.
		layout func(t *testing.T, root string) string
		// detach routes the entry to the branchless log instead of the
		// branch file — the half the old suffix rule forgave and this one
		// does not.
		detach bool
		// wantStaged is the path git must report for the entry. Asserted
		// before the gate is asked, so a setup that quietly stopped
		// producing the configuration fails loudly instead of passing.
		wantStaged string
		// docsRel is the whole decision record, unstaged wholesale for the
		// control. `logmind init` writes a first decision of its own, so
		// unstaging only wantStaged would leave that one in the index and
		// the control would pass on the wrong file.
		docsRel string
		// wantReceipt is what `logmind log`'s own quiet receipt must name.
		// Relative to the directory the command ran in, which is the
		// project root — not the repository root git reports against.
		wantReceipt string
	}{
		{
			name:       "pre-existing Docs directory, branchless route",
			layout:     layoutCaseFoldedDocs,
			detach:     true,
			wantStaged: "Docs/decisions.md", docsRel: "Docs", wantReceipt: "Docs/decisions.md",
		},
		{
			name:       "pre-existing Docs directory, branch route",
			layout:     layoutCaseFoldedDocs,
			wantStaged: "Docs/decisions-branches/main.md", docsRel: "Docs", wantReceipt: "Docs/decisions-branches/main.md",
		},
		{
			name:       "logmind project below the git root, branchless route",
			layout:     layoutNestedProject,
			detach:     true,
			wantStaged: "pkg/api/docs/decisions.md", docsRel: "pkg/api/docs", wantReceipt: "docs/decisions.md",
		},
		{
			name:       "logmind project below the git root, branch route",
			layout:     layoutNestedProject,
			wantStaged: "pkg/api/docs/decisions-branches/main.md", docsRel: "pkg/api/docs", wantReceipt: "docs/decisions-branches/main.md",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := withTempCwd(t, func(root string) {
				testgit.InitRepo(t, root, "--initial-branch=main")
				mustGit(t, root, "config", "user.email", "test@example.com")
				mustGit(t, root, "config", "user.name", "Test")
				mustGit(t, root, "config", "commit.gpgsign", "false")
				if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
					t.Fatalf("write seed: %v", err)
				}
				mustGit(t, root, "add", "seed.txt")
				mustGit(t, root, "commit", "-m", "seed")

				project := tc.layout(t, root)
				if tc.detach {
					mustGit(t, root, "checkout", "--detach", "HEAD")
				}

				// Well over git.commit_line_threshold, so the gate has
				// something to refuse if the decision does not count.
				if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
					t.Fatalf("mkdir src: %v", err)
				}
				if err := os.WriteFile(filepath.Join(root, "src", "big.go"),
					[]byte(strings.Repeat("// a substantive line\n", 302)), 0o644); err != nil {
					t.Fatalf("write big.go: %v", err)
				}

				if err := os.Chdir(project); err != nil {
					t.Fatalf("chdir %s: %v", project, err)
				}
				// Quiet, so the receipt names the file: the path logmind
				// PRINTS must be the path git reports, or the operator is
				// told to look somewhere the entry is not. On a case-folding
				// volume `filepath.Join(cwd, "docs")` writes into `Docs/` and
				// reports `docs/` — the same split that made the gate refuse
				// it, showing up in the output instead of the exit code.
				t.Setenv("LOGMIND_QUIET", "1")
				var out bytes.Buffer
				withFakeTTY(t, false, func() {
					cmd := NewRootCmd()
					cmd.SetArgs([]string{"log", "Gate agreement probe", "-r", "Why", "--no-commit"})
					cmd.SetOut(&out)
					cmd.SetErr(&out)
					if err := cmd.Execute(); err != nil {
						t.Fatalf("log: %v\n%s", err, out.String())
					}
				})
				if want := "path=" + tc.wantReceipt + " "; !strings.Contains(out.String(), want) {
					t.Errorf("`logmind log` receipt does not carry %q; got %q", want, out.String())
				}
				if err := os.Chdir(root); err != nil {
					t.Fatalf("chdir back: %v", err)
				}
				mustGit(t, root, "add", "-A")
			})

			names := stagedNames(t, root)
			if !stagedContains(names, tc.wantStaged) {
				t.Fatalf("git staged %v; want %s among them — the configuration under test is not the one that was set up, so the gate assertion below would prove nothing",
					names, tc.wantStaged)
			}

			// CONTROL first: the same 302 lines with the decision file
			// UNSTAGED must be refused, or "allowed" below is not evidence
			// that the decision was what allowed it.
			mustGit(t, root, "reset", "-q", "--", tc.docsRel)
			if d := guardcommit.Evaluate(root, "subject", 20, guardcommit.StagedOnly); d.Allow {
				t.Fatalf("control: Decision = %+v; want Block with the decision unstaged — 302 lines of Go would land undocumented", d)
			}
			mustGit(t, root, "add", "-A")

			d := guardcommit.Evaluate(root, "subject", 20, guardcommit.StagedOnly)
			if !d.Allow || d.CarveOut != guardcommit.CarveOutDecisionRecorded {
				t.Fatalf("Decision = %+v; want Allow via CarveOutDecisionRecorded — `logmind log` wrote a well-formed entry to %s and the commit carrying it must not be refused",
					d, tc.wantStaged)
			}
		})
	}
}

// layoutCaseFoldedDocs scaffolds into a `Docs/` directory that already
// exists — the one component of the layout a user can spell, and the one
// the writer resolves through the filesystem rather than naming.
func layoutCaseFoldedDocs(t *testing.T, root string) string {
	t.Helper()
	if !foldsCase(t, root) {
		t.Skip("filesystem is case-sensitive: `Docs/` and `docs/` are two directories here, so this configuration cannot arise")
	}
	if err := os.Mkdir(filepath.Join(root, "Docs"), 0o755); err != nil {
		t.Fatalf("mkdir Docs: %v", err)
	}
	scaffoldDocs(t)
	return root
}

// layoutNestedProject runs `logmind init` below the git root — which exits
// 0 today — so the decision record hangs off pkg/api rather than the
// repository root the gate judges paths against.
func layoutNestedProject(t *testing.T, root string) string {
	t.Helper()
	project := filepath.Join(root, "pkg", "api")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir pkg/api: %v", err)
	}
	origin, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatalf("chdir %s: %v", project, err)
	}
	scaffoldDocs(t)
	if err := os.Chdir(origin); err != nil {
		t.Fatalf("chdir back: %v", err)
	}
	return project
}

func stagedContains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
