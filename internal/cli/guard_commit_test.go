package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeGuardCommitConfig writes a minimal .logmind/config.yml under repo
// so tests can exercise git.enforce_commits / git.commit_line_threshold
// without hand-rolling the full config shape.
func writeGuardCommitConfig(t *testing.T, repo, body string) {
	t.Helper()
	dir := filepath.Join(repo, ".logmind")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .logmind: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
}

// bigLines returns n newline-terminated lines, enough to trip a small
// threshold regardless of diff mode.
func bigLines(n int) string {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		b.WriteString("line\n")
	}
	return b.String()
}

// --- runGuardCommit: --layer harness ------------------------------------

func TestRunGuardCommit_Harness_AllowsSmallChange(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "small.go"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git commit -m x`, repo)

	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty (harness allow prints nothing)", stdout.String())
	}
}

func TestRunGuardCommit_Harness_BlocksOverThresholdUnstagedChange(t *testing.T) {
	repo := initRepo(t)
	// Unstaged (not `git add`ed) — this is the WorkingTreeUnion regression:
	// a compound `git add -A && git commit` hasn't staged anything yet
	// when the harness's PreToolUse hook fires.
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(bigLines(30)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git add -A && git commit -m x`, repo)

	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("exitCode = %d; want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "logmind log") {
		t.Fatalf("stderr = %q; want it to mention `logmind log`", stderr.String())
	}
}

func TestRunGuardCommit_Harness_NonBashToolAllowsWithoutInspection(t *testing.T) {
	repo := initRepo(t)
	payload := harnessJSON(t, "Read", `git commit -m x`, repo)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (non-Bash tool_name fails open)", exitCode, err)
	}
}

func TestRunGuardCommit_Harness_MalformedJSONFailsOpen(t *testing.T) {
	repo := initRepo(t)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader("not json"), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (unparseable payload fails open)", exitCode, err)
	}
}

func TestRunGuardCommit_Harness_NonCommitBashCommandAllows(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(50)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `npm test`, repo)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (not a git-commit shape)", exitCode, err)
	}
}

func TestRunGuardCommit_Harness_SkipLogmindMarkerAllows(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(50)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git commit -m "wip [skip-logmind]"`, repo)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil ([skip-logmind] extracted from -m)", exitCode, err)
	}
}

func TestRunGuardCommit_Harness_UsesPayloadCwdOverRepoRootFlag(t *testing.T) {
	repo := initRepo(t)
	other := t.TempDir() // not a git repo at all
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(50)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// payload.cwd points at the real repo; --repo-root points nowhere
	// useful. The real repo's cwd must win, so the big change is caught.
	payload := harnessJSON(t, "Bash", `git commit -m x`, repo)
	exitCode, err := runGuardCommit(other, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("exitCode = %d; want 2 (payload cwd should have been used, not the --repo-root fallback)", exitCode)
	}
}

// --- runGuardCommit: --layer git-hook ------------------------------------

func TestRunGuardCommit_GitHook_AllowsSmallStagedChange(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "small.go", 5)
	msgFile := writeMsgFile(t, repo, "a small change")

	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit(repo, "git-hook", msgFile, 20, true, false,
		nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
}

func TestRunGuardCommit_GitHook_BlocksOverThresholdStagedChange(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "big.go", 25)
	msgFile := writeMsgFile(t, repo, "a big change")

	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit(repo, "git-hook", msgFile, 20, true, false,
		nil, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0 (git-hook blocks via ErrSilent, not a special exit code)", exitCode)
	}
	if err == nil || err != ErrSilent {
		t.Fatalf("err = %v; want ErrSilent", err)
	}
	if !strings.Contains(stderr.String(), "logmind log") {
		t.Fatalf("stderr = %q; want it to mention `logmind log`", stderr.String())
	}
}

func TestRunGuardCommit_GitHook_UnstagedOnlyChangeAllows(t *testing.T) {
	// The StagedOnly half of the key regression: a large change that
	// hasn't been staged is invisible to the git-hook layer by design
	// (correct — the index IS final by the time commit-msg runs; an
	// unstaged change literally cannot end up in this commit).
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(bigLines(30)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	msgFile := writeMsgFile(t, repo, "subject")

	exitCode, err := runGuardCommit(repo, "git-hook", msgFile, 20, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (unstaged change invisible under StagedOnly)", exitCode, err)
	}
}

func TestRunGuardCommit_GitHook_MissingMsgFileFlag_PlainError(t *testing.T) {
	repo := initRepo(t)
	exitCode, err := runGuardCommit(repo, "git-hook", "", 20, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
	if err == nil {
		t.Fatalf("err = nil; want a plain error (missing --msg-file)")
	}
	if err == ErrSilent {
		t.Fatalf("err = ErrSilent; want a plain (non-ErrSilent) error for this misuse case")
	}
	if !strings.Contains(err.Error(), "--msg-file") {
		t.Fatalf("err = %v; want it to mention --msg-file", err)
	}
}

func TestRunGuardCommit_GitHook_MissingMsgFileOnDisk_PropagatesRealError(t *testing.T) {
	repo := initRepo(t)
	exitCode, err := runGuardCommit(repo, "git-hook", filepath.Join(repo, "does-not-exist.txt"), 20, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
	if err == nil || err == ErrSilent {
		t.Fatalf("err = %v; want a plain os.ReadFile error", err)
	}
}

// --- shared: --layer validation, threshold resolution, enforce_commits ---

func TestRunGuardCommit_InvalidLayer_PlainError(t *testing.T) {
	repo := initRepo(t)
	exitCode, err := runGuardCommit(repo, "bogus", "", 20, true, false,
		strings.NewReader("{}"), &bytes.Buffer{}, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
	if err == nil || err == ErrSilent {
		t.Fatalf("err = %v; want a plain error naming the bad --layer value", err)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err = %v; want it to echo the invalid value", err)
	}
}

func TestRunGuardCommit_EnforceCommitsFalse_AllowsBothLayers(t *testing.T) {
	repo := initRepo(t)
	writeGuardCommitConfig(t, repo, "git:\n  enforce_commits: false\n")

	// Harness layer: a huge unstaged change that would otherwise block.
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(100)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git commit -m x`, repo)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("harness: exitCode=%d err=%v; want 0,nil (enforce_commits:false is a full off-ramp)", exitCode, err)
	}

	// git-hook layer: stage the same huge change.
	stageLines(t, repo, "staged-big.go", 100)
	msgFile := writeMsgFile(t, repo, "subject")
	exitCode, err = runGuardCommit(repo, "git-hook", msgFile, 20, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("git-hook: exitCode=%d err=%v; want 0,nil (enforce_commits:false is a full off-ramp)", exitCode, err)
	}
}

func TestRunGuardCommit_ThresholdPrecedence(t *testing.T) {
	repo := initRepo(t)
	writeGuardCommitConfig(t, repo, "git:\n  commit_line_threshold: 5\n")
	stageLines(t, repo, "code.go", 10)
	msgFile := writeMsgFile(t, repo, "subject")

	// Config threshold (5) is below 10 changed lines → block, with no
	// --threshold flag override (flagExplicit=false, flagValue ignored).
	_, err := runGuardCommit(repo, "git-hook", msgFile, 999, false, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != ErrSilent {
		t.Fatalf("err = %v; want ErrSilent (config threshold of 5 should have applied)", err)
	}

	// Explicit --threshold flag (50) wins over the config's 5.
	exitCode, err := runGuardCommit(repo, "git-hook", msgFile, 50, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (explicit --threshold=50 should have won over config's 5)", exitCode, err)
	}
}

// --- black-box subprocess test: exit code 2, not 1 -----------------------

// TestGuardCommitBinary_Harness_ExitCode2NotJust1 builds the real binary
// and pipes a blocking harness JSON payload to
// `guard-commit --layer harness`, asserting the process exits 2 — NOT the
// generic exit-1 every other error path in this CLI uses. Exit code 2 is
// load-bearing: the Claude Code harness's PreToolUse protocol only treats
// exit 2 as "block this tool call"; exit 1 would silently let it through.
func TestGuardCommitBinary_Harness_ExitCode2NotJust1(t *testing.T) {
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

	binPath := buildGuardCommitBinary(t, goBin)

	t.Run("blocks with exit code 2", func(t *testing.T) {
		repo := initRepo(t) // fresh repo per subtest — no cross-subtest state bleed
		if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(30)), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		payload := harnessJSON(t, "Bash", `git add -A && git commit -m x`, repo)

		cmd := exec.Command(binPath, "guard-commit", "--layer", "harness")
		cmd.Stdin = strings.NewReader(payload)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()

		exitErr, ok := runErr.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected *exec.ExitError, got %T (%v); stdout=%q stderr=%q", runErr, runErr, stdout.String(), stderr.String())
		}
		if got := exitErr.ExitCode(); got != 2 {
			t.Fatalf("ExitCode() = %d; want 2 (not the generic exit 1)", got)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q; want empty on block", stdout.String())
		}
		if !strings.Contains(stderr.String(), "logmind log") {
			t.Fatalf("stderr = %q; want it to mention `logmind log`", stderr.String())
		}
	})

	t.Run("allows with exit code 0 and empty stdout", func(t *testing.T) {
		repo := initRepo(t) // fresh repo per subtest — no cross-subtest state bleed
		if err := os.WriteFile(filepath.Join(repo, "tiny.go"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		payload := harnessJSON(t, "Bash", `git commit -m x`, repo)

		cmd := exec.Command(binPath, "guard-commit", "--layer", "harness")
		cmd.Stdin = strings.NewReader(payload)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("logmind guard-commit --layer harness (allow case) failed: %v\nstdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q; want empty on allow", stdout.String())
		}
	})
}

// buildGuardCommitBinary compiles ./cmd/logmind once per test run into a
// temp dir and returns the binary path.
func buildGuardCommitBinary(t *testing.T, goBin string) string {
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

// --- shared payload/file helpers ------------------------------------------

// harnessJSON builds a minimal PreToolUse-shaped JSON payload.
func harnessJSON(t *testing.T, toolName, command, cwd string) string {
	t.Helper()
	payload := struct {
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
		Cwd string `json:"cwd"`
	}{ToolName: toolName, Cwd: cwd}
	payload.ToolInput.Command = command
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal harness payload: %v", err)
	}
	return string(data)
}

// writeMsgFile writes a commit-message file (subject on the first line)
// and returns its path, mimicking what git hands a commit-msg hook.
func writeMsgFile(t *testing.T, repo, subject string) string {
	t.Helper()
	path := filepath.Join(repo, "COMMIT_EDITMSG")
	if err := os.WriteFile(path, []byte(subject+"\n\nbody text\n"), 0o644); err != nil {
		t.Fatalf("write msg file: %v", err)
	}
	return path
}
