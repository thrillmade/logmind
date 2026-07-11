// commit_msg_enforce_test.go — a REAL `git commit` integration test for
// the v2.0.0 commit-msg hook body. BuildCommitMsgBody/InstallCommitMsg in
// hooks.go only produce and install a shell script; the actual
// allow/block DECISION lives in internal/guardcommit (exhaustively unit
// tested there and in internal/cli/guard_commit_test.go). This file's
// narrower job: prove the shell script this package emits actually wires
// a genuine `git commit` invocation through to that decision engine and
// back into git's exit-code protocol — i.e. that the wiring, not just the
// logic, works end to end.
package hooks

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommitMsgHook_RealGitCommit_EnforcesAndCarveOuts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; skipping subprocess test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping subprocess test")
	}

	binPath := buildLogmindBinaryForHookTest(t, goBin)
	binDir := filepath.Dir(binPath)

	t.Run("blocks a substantive commit with no decision log", func(t *testing.T) {
		dir := newEnforcingRepo(t, binDir)
		writeBigFile(t, dir, "app.go")
		runGitIn(t, dir, binDir, nil, "add", "app.go")

		out, err := attemptGitCommit(t, dir, binDir, nil, "substantive change, no decision log")
		if err == nil {
			t.Fatalf("expected the commit to be BLOCKED; it succeeded:\n%s", out)
		}
		if !strings.Contains(out, "logmind log") {
			t.Errorf("blocked-commit output should mention `logmind log`; got:\n%s", out)
		}
		assertCommitCount(t, dir, binDir, 1) // only newEnforcingRepo's initial commit
	})

	t.Run("allows with [skip-logmind] in the subject", func(t *testing.T) {
		dir := newEnforcingRepo(t, binDir)
		writeBigFile(t, dir, "app.go")
		runGitIn(t, dir, binDir, nil, "add", "app.go")

		out, err := attemptGitCommit(t, dir, binDir, nil, "substantive change [skip-logmind]")
		if err != nil {
			t.Fatalf("expected the commit to be ALLOWED via [skip-logmind]; got error: %v\n%s", err, out)
		}
		assertCommitCount(t, dir, binDir, 2)
	})

	t.Run("allows with LOGMIND_ALLOW_GIT_COMMIT=1", func(t *testing.T) {
		dir := newEnforcingRepo(t, binDir)
		writeBigFile(t, dir, "app.go")
		runGitIn(t, dir, binDir, nil, "add", "app.go")

		out, err := attemptGitCommit(t, dir, binDir, []string{"LOGMIND_ALLOW_GIT_COMMIT=1"}, "substantive change, explicit override")
		if err != nil {
			t.Fatalf("expected the commit to be ALLOWED via LOGMIND_ALLOW_GIT_COMMIT=1; got error: %v\n%s", err, out)
		}
		assertCommitCount(t, dir, binDir, 2)
	})

	t.Run("allows when a decision file is staged alongside the change", func(t *testing.T) {
		dir := newEnforcingRepo(t, binDir)
		writeBigFile(t, dir, "app.go")
		if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docs", "decisions.md"), []byte("# Decisions\n\n## entry\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGitIn(t, dir, binDir, nil, "add", "app.go", "docs/decisions.md")

		out, err := attemptGitCommit(t, dir, binDir, nil, "substantive change with a decision log")
		if err != nil {
			t.Fatalf("expected the commit to be ALLOWED via a staged decision file; got error: %v\n%s", err, out)
		}
		assertCommitCount(t, dir, binDir, 2)
	})
}

// newEnforcingRepo creates a fresh git repo with this package's
// commit-msg hook installed and one initial commit — so commit COUNTING
// (not merely "does HEAD exist"), is what distinguishes blocked vs.
// allowed across subtests below. The initial commit ALSO runs through
// the freshly-installed hook (with binDir on PATH, same as every other
// git invocation in this file) — it's tiny (one line), so it's expected
// to sail through the under-threshold carve-out; if that regresses, this
// helper fails loudly instead of masking it as "the fixture doesn't hit
// the hook."
func newEnforcingRepo(t *testing.T, binDir string) string {
	t.Helper()
	dir := t.TempDir()
	runGitIn(t, dir, binDir, nil, "init", "-q")
	runGitIn(t, dir, binDir, nil, "config", "user.email", "t@t.com")
	runGitIn(t, dir, binDir, nil, "config", "user.name", "t")
	if _, err := InstallCommitMsg(dir); err != nil {
		t.Fatalf("InstallCommitMsg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, binDir, nil, "add", "README.md")
	out, err := attemptGitCommit(t, dir, binDir, nil, "init")
	if err != nil {
		t.Fatalf("fixture's initial commit unexpectedly blocked: %v\n%s", err, out)
	}
	return dir
}

// isolatedGitEnv builds the environment for every git invocation in this
// file: PATH extended with binDir (so the installed hook's
// `command -v logmind` finds the binary this test built rather than
// whatever `logmind` may already be on the host's real PATH — e.g. an
// older release without the `guard-commit` command, which is exactly the
// footgun this exists to avoid), plus isolation from the host's/CI
// runner's global or system git config (a stray global commit.gpgsign or
// core.hooksPath would make this flaky, or hang on a signing prompt).
func isolatedGitEnv(t *testing.T, binDir string) []string {
	t.Helper()
	fakeHome := t.TempDir()
	path := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	return []string{
		"HOME=" + fakeHome,
		"USERPROFILE=" + fakeHome, // Windows' HOME equivalent
		"GIT_CONFIG_NOSYSTEM=1",
		"XDG_CONFIG_HOME=" + fakeHome,
		"PATH=" + path,
	}
}

// writeBigFile writes 30 substantive lines — comfortably over the
// default commit_line_threshold of 20 regardless of diff mode. Mirrors
// internal/cli/guard_commit_test.go's bigLines helper (duplicated here;
// different package, same small-helper-duplication convention this
// codebase already uses — see buildLogmindBinaryForHookTest below).
func writeBigFile(t *testing.T, dir, name string) {
	t.Helper()
	var b bytes.Buffer
	for i := 0; i < 30; i++ {
		b.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func runGitIn(t *testing.T, dir, binDir string, extraEnv []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	env := append(os.Environ(), isolatedGitEnv(t, binDir)...)
	cmd.Env = append(env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// attemptGitCommit runs `git commit -m message` — same isolated,
// binDir-extended PATH as runGitIn, plus any extraEnv (e.g.
// LOGMIND_ALLOW_GIT_COMMIT=1). Returns combined output and the *exec.Cmd
// error — non-nil on a non-zero exit, i.e. the hook blocked the commit.
func attemptGitCommit(t *testing.T, dir, binDir string, extraEnv []string, message string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "-m", message)
	cmd.Dir = dir
	env := append(os.Environ(), isolatedGitEnv(t, binDir)...)
	cmd.Env = append(env, extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func assertCommitCount(t *testing.T, dir, binDir string, want int) {
	t.Helper()
	got := runGitIn(t, dir, binDir, nil, "rev-list", "--count", "HEAD")
	got = strings.TrimSpace(got)
	if got != fmt.Sprintf("%d", want) {
		t.Fatalf("commit count = %s; want %d", got, want)
	}
}

// buildLogmindBinaryForHookTest compiles ./cmd/logmind into a temp dir
// and returns the binary's path. Mirrors
// internal/cli/guard_commit_test.go's buildGuardCommitBinary — kept as a
// separate copy (rather than shared) per this codebase's existing
// small-test-helper-duplication convention (see e.g.
// internal/cli/install_hook_test.go's initRepo doc comment).
func buildLogmindBinaryForHookTest(t *testing.T, goBin string) string {
	t.Helper()
	repoRoot := repoRootFromCaller(t)
	binDir := t.TempDir()
	binName := "logmind"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(binDir, binName)

	build := exec.Command(goBin, "build", "-o", binPath, "./cmd/logmind")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return binPath
}
