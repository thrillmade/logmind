// doctor_hookspath_test.go — `logmind doctor --fix` must not write a hook
// to a path this repository never reads, and must not then report OK over
// the gate it did not install.
//
// Measured on the release candidate, in a repository with
// `core.hooksPath = .githooks` and no hook there:
//
//	logmind doctor        → DRIFT, "commit-msg hook is not installed"   (true)
//	logmind doctor --fix  → exit 0, wrote .git/hooks/commit-msg          (unread)
//	logmind doctor        → Stack status: OK, exit 0                     (false)
//	git commit  (30 lines, no decision)  → exit 0
//
// The last two lines are the defect. A --fix that manufactures an OK is
// worse than no --fix at all: the operator is told the gate is back.
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

func TestDoctorFix_InstallsTheHookWhereGitReadsIt(t *testing.T) {
	mustGit := func(t *testing.T, dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runDoctor := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		root := NewRootCmd()
		root.SetArgs(args)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		// Execute FIRST: a `return out.String(), root.Execute()` evaluates
		// the buffer before the command has written to it.
		err := root.Execute()
		return out.String(), err
	}

	dir := withTempCwd(t, func(dir string) {
		testgit.InitRepo(t, dir, "-q", "--initial-branch=main")
		mustGit(t, dir, "config", "user.email", "test@example.com")
		mustGit(t, dir, "config", "user.name", "Test")
		mustGit(t, dir, "config", "commit.gpgsign", "false")
		if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
			t.Fatalf("write seed: %v", err)
		}
		mustGit(t, dir, "add", "seed.txt")
		mustGit(t, dir, "commit", "-m", "seed", "--no-verify")

		if _, err := runDoctor(t, "init", "--github-actions=false"); err != nil {
			t.Fatalf("init: %v", err)
		}

		// Relocate the hooks directory and leave it EMPTY — the state where
		// the gate is genuinely gone and the naive join cannot see that.
		if err := os.MkdirAll(filepath.Join(dir, ".githooks"), 0o755); err != nil {
			t.Fatalf("mkdir .githooks: %v", err)
		}
		mustGit(t, dir, "config", "core.hooksPath", ".githooks")
		_ = os.Remove(filepath.Join(dir, ".git", "hooks", "commit-msg"))

		// CONTROL: the gate really is gone, and doctor really does say so.
		// Without this the assertion after --fix is passed by a doctor that
		// reports nothing at all.
		before, _ := runDoctor(t, "doctor", "--offline", "--exit-zero")
		if !strings.Contains(before, "Enforcement gates absent") ||
			!strings.Contains(before, "commit-msg") {
			t.Fatalf("control: doctor did not report the missing commit-msg gate before --fix:\n%s", before)
		}

		out, err := runDoctor(t, "doctor", "--fix", "--offline")
		if err != nil {
			t.Fatalf("doctor --fix: %v\n%s", err, out)
		}
	})

	// The hook is where git reads it…
	if _, err := os.Stat(filepath.Join(dir, ".githooks", "commit-msg")); err != nil {
		t.Errorf(".githooks/commit-msg was not installed: %v — `doctor --fix` wrote to a "+
			"directory this repository does not read", err)
	}
	// …and NOT where it does not.
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "commit-msg")); err == nil {
		t.Errorf("a hook was written to .git/hooks/commit-msg; core.hooksPath is .githooks, so " +
			"git never runs it — this is the write that manufactured a false OK")
	}

	// And the OK that follows is a true one: assert the report, not just
	// the file, because "OK" is what the operator actually reads.
	out, err := func() (string, error) {
		orig, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		defer func() { _ = os.Chdir(orig) }()
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline", "--exit-zero"})
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		err := root.Execute()
		return buf.String(), err
	}()
	if err != nil {
		t.Fatalf("doctor after --fix: %v\n%s", err, out)
	}
	if strings.Contains(out, "Enforcement gates absent") {
		t.Errorf("doctor still reports an absent gate after --fix installed one:\n%s", out)
	}
}
