package cli

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRebaseNotARepo: rebase outside a git repo errors out cleanly.
func TestRebaseNotARepo(t *testing.T) {
	cwd := t.TempDir()
	var stdout bytes.Buffer
	err := runRebase(cwd, "", false, false, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("err = %v; want ErrSilent", err)
	}
	want := "Error: not in a git repository.\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q; want %q", stdout.String(), want)
	}
}

// gitInit minimally initializes a git repo for rebase tests. Returns
// the path. Tests skip if git is unavailable on PATH.
func gitInit(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init", "-q", "-b", "main"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test"},
		// gc.auto=0 — see initLogTestGitRepo (log_test.go) for why.
		{"git", "config", "gc.auto", "0"},
		{"git", "commit", "--allow-empty", "-q", "-m", "initial"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", c, err, out)
		}
	}
	return dir
}

// TestRebaseRefusesOnDefaultBranch: when current branch == base, exit 1.
func TestRebaseRefusesOnDefaultBranch(t *testing.T) {
	cwd := gitInit(t)
	var stdout bytes.Buffer
	// We're on `main` and base defaults to `main` (no remote configured;
	// DefaultBranch's fallback path hits local-main check).
	err := runRebase(cwd, "", false, false, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("err = %v; want ErrSilent", err)
	}
	if !strings.Contains(stdout.String(), "refusing to rebase 'main' onto itself") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

// TestRebaseNoBaseExplicit: --base specified to a branch == current
// branch still refuses.
func TestRebaseRefusesExplicitBase(t *testing.T) {
	cwd := gitInit(t)
	var stdout bytes.Buffer
	err := runRebase(cwd, "main", false, false, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout.String(), "refusing to rebase 'main' onto itself") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

// TestRebaseFetchFailsNoOrigin: --no-fetch off but no origin → fetch errors out.
func TestRebaseFetchFailsNoOrigin(t *testing.T) {
	cwd := gitInit(t)
	// Create + checkout a feature branch so we're not on main.
	for _, c := range [][]string{
		{"git", "checkout", "-q", "-b", "feat/x"},
	} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = cwd
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", c, err, out)
		}
	}
	var stdout bytes.Buffer
	err := runRebase(cwd, "main", false, false, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("err = %v; want ErrSilent", err)
	}
	// fetch attempts origin which doesn't exist → step printed first
	// then "Error: git fetch failed."
	if !strings.Contains(stdout.String(), "→ git fetch origin main") {
		t.Errorf("missing fetch progress: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Error: git fetch failed.") {
		t.Errorf("missing fetch error: %q", stdout.String())
	}
}

// TestRebaseNoFetchNoOriginRebaseFails: --no-fetch skips fetch, rebase
// then attempts origin/main which doesn't exist → rebase step errors.
func TestRebaseNoFetchNoOriginRebaseFails(t *testing.T) {
	cwd := gitInit(t)
	for _, c := range [][]string{
		{"git", "checkout", "-q", "-b", "feat/x"},
	} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = cwd
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", c, err, out)
		}
	}
	var stdout bytes.Buffer
	err := runRebase(cwd, "main", true, true, &stdout) // no-push + no-fetch
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("err = %v; want ErrSilent", err)
	}
	if !strings.Contains(stdout.String(), "→ git rebase origin/main") {
		t.Errorf("missing rebase progress: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Error: git rebase failed.") {
		t.Errorf("missing rebase error: %q", stdout.String())
	}
}

// TestRebaseSuccess: full happy path with a same-repo "origin/main"
// substitute. We create a parallel branch that origin/main aliases to.
func TestRebaseSuccessNoPush(t *testing.T) {
	cwd := gitInit(t)
	// Setup: create a "remote" by cloning into a temp dir, then add it
	// as origin. After cloning the upstream main has 1 commit; we add
	// one more on the local feature branch.
	remote := filepath.Join(t.TempDir(), "bare.git")
	for _, c := range [][]string{
		{"git", "clone", "--bare", "-q", cwd, remote},
		{"git", "-C", cwd, "remote", "add", "origin", remote},
		{"git", "-C", cwd, "fetch", "-q", "origin"},
		{"git", "-C", cwd, "checkout", "-q", "-b", "feat/x"},
		// Add an empty commit on feat/x so rebase has work to do.
		{"git", "-C", cwd, "commit", "--allow-empty", "-q", "-m", "feat work"},
	} {
		cmd := exec.Command(c[0], c[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", c, err, out)
		}
	}
	var stdout bytes.Buffer
	if err := runRebase(cwd, "main", true, false, &stdout); err != nil {
		t.Fatalf("rebase err = %v; stdout=%q", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "✓ Rebased 'feat/x' onto origin/main (push skipped).") {
		t.Errorf("missing success message: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "ok rebased: feat/x onto origin/main (no push)") {
		t.Errorf("missing ok line: %q", stdout.String())
	}
}
