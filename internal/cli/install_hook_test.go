package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/testgit"
)

// TestInstallHook_NotARepo asserts the "not a git repository" output
// is byte-identical to Python (cli.py:2830). The string includes
// punctuation and a trailing newline; both are pinned by the golden.
func TestInstallHook_NotARepo(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	err := runInstallHook(dir, false, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runInstallHook err = %v; want ErrSilent", err)
	}
	checkGolden(t, "install_hook_not_a_repo.golden", stdout.String())
}

// TestInstallHook_FreshInstall walks the create-from-nothing path.
// Asserts the hook file exists, has the canonical body, and is chmod
// 0755 (the exec bit is REQUIRED for git to invoke a hook).
func TestInstallHook_FreshInstall(t *testing.T) {
	repo := initRepo(t)
	var stdout bytes.Buffer
	if err := runInstallHook(repo, false, &stdout); err != nil {
		t.Fatalf("runInstallHook: %v", err)
	}
	checkGolden(t, "install_hook_fresh.golden", stdout.String())
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	body, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	// #213: the fresh body is the shebang + the hang-guarded invocation.
	want := "#!/bin/sh\n" + preCommitGuardedCall
	if string(body) != want {
		t.Fatalf("hook body = %q; want %q", body, want)
	}
	// The guard-shape invariants the deadline wrapper must preserve.
	for _, must := range []string{
		"command -v logmind",        // missing-binary no-op guard
		"logmind check-decisions &", // backgrounded under the watchdog
		"( sleep 10; kill",          // the deadline watchdog
		"wait \"$__lm_pid\"",        // capture the real exit code
		"-gt 128",                   // timeout/crash → fail open
		"check-decisions NOT RUN",   // #270: the no-op is not a SILENT one
	} {
		if !strings.Contains(string(body), must) {
			t.Errorf("pre-commit body missing %q", must)
		}
	}
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("stat hook: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("hook is not executable: mode=%v", info.Mode())
	}
}

// TestInstallHook_AlreadyInstalled exercises the idempotency path —
// running install-hook twice in a row must produce the "already
// installed" message on the second run.
func TestInstallHook_AlreadyInstalled(t *testing.T) {
	repo := initRepo(t)
	var first bytes.Buffer
	if err := runInstallHook(repo, false, &first); err != nil {
		t.Fatalf("first install: %v", err)
	}
	var second bytes.Buffer
	if err := runInstallHook(repo, false, &second); err != nil {
		t.Fatalf("second install: %v", err)
	}
	checkGolden(t, "install_hook_already.golden", second.String())
}

// TestInstallHook_ForeignNoForce asserts the conflict path: an
// existing non-logmind hook + no --force → error message + exit 1.
// Original hook must NOT be touched.
func TestInstallHook_ForeignNoForce(t *testing.T) {
	repo := initRepo(t)
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	custom := "#!/bin/sh\necho custom\n"
	if err := os.WriteFile(hookPath, []byte(custom), 0o755); err != nil {
		t.Fatalf("seed custom: %v", err)
	}
	var stdout bytes.Buffer
	err := runInstallHook(repo, false, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runInstallHook err = %v; want ErrSilent", err)
	}
	checkGolden(t, "install_hook_foreign_no_force.golden", stdout.String())
	// Original hook must be intact.
	got, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("re-read hook: %v", err)
	}
	if string(got) != custom {
		t.Fatalf("foreign hook was modified: %q", got)
	}
}

// TestInstallHook_ForeignWithForce exercises the --force append path.
// The original content must be preserved; the logmind line is
// appended after a newline normalisation (rstrip("\n") + "\n" + line).
func TestInstallHook_ForeignWithForce(t *testing.T) {
	repo := initRepo(t)
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	custom := "#!/bin/sh\necho custom\n"
	if err := os.WriteFile(hookPath, []byte(custom), 0o755); err != nil {
		t.Fatalf("seed custom: %v", err)
	}
	var stdout bytes.Buffer
	if err := runInstallHook(repo, true, &stdout); err != nil {
		t.Fatalf("force install: %v", err)
	}
	checkGolden(t, "install_hook_foreign_force.golden", stdout.String())
	got, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	// #213: the foreign content is preserved verbatim and the hang-guarded
	// block is appended after a trailing-newline normalisation.
	want := "#!/bin/sh\necho custom\n" + preCommitGuardedCall
	if string(got) != want {
		t.Fatalf("hook body = %q; want %q", got, want)
	}
}

// TestInstallHook_NoEngineOnPath_FailsOpenLoudly is the issue #270
// regression for this hook: `logmind install-hook` is opt-in — the user
// asked for the gate — so a run where nothing on PATH answers to `logmind`
// must still allow the commit (exit 0, per SPEC §3.4's mandatory fail-open)
// AND must say on stderr that it did not run. The stale-engine case needs no
// equivalent here: this body preserves check-decisions' own exit code, so an
// engine that doesn't know the subcommand exits nonzero and BLOCKS — noisy
// by construction, never a silent allow.
//
// Runs the hook script directly under an empty PATH, the hermetic way to say
// "nothing answers to logmind" — filtering the host's real PATH would depend
// on where this machine keeps git and logmind (often one directory).
func TestInstallHook_NoEngineOnPath_FailsOpenLoudly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX sh hook body; not applicable on Windows")
	}
	repo := initRepo(t)
	if err := runInstallHook(repo, false, &bytes.Buffer{}); err != nil {
		t.Fatalf("runInstallHook: %v", err)
	}

	cmd := exec.Command("/bin/sh", filepath.Join(repo, ".git", "hooks", "pre-commit"))
	cmd.Dir = repo
	cmd.Env = []string{"PATH="}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("hook exited non-zero with no engine on PATH: %v\nstderr: %s", err, stderr.String())
	}
	for _, must := range []string{"check-decisions NOT RUN", "found nothing"} {
		if !strings.Contains(stderr.String(), must) {
			t.Errorf("stderr missing %q; got: %q", must, stderr.String())
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("hook wrote to stdout: %q; the notice belongs on stderr", stdout.String())
	}
}

// initRepo creates a git repo with an initial commit at t.TempDir().
// Test-helper duplicate of the one in internal/gitcli — repeated
// here so the cli package's tests don't depend on a sibling test
// package's exported helpers.
func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	testgit.InitRepo(t, dir, "-q")
	for _, args := range [][]string{
		{"config", "user.email", "t@t.com"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// checkGolden is the same pattern internal/cli/version_test.go uses
// for the snapshot loop. Kept duplicated rather than exported so
// each test file can read inline and the snapshot mechanism stays
// stable for future waves.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `make snapshot` to create it)", path, err)
	}
	if string(want) != got {
		t.Fatalf("drift vs %s\n--- want ---\n%s\n--- got ---\n%s",
			path, want, got)
	}
}
