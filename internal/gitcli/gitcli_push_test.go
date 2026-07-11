package gitcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// bareRemote creates a `git init --bare` repo under t.TempDir() and
// returns its path. No network — a bare repo on the local filesystem is
// a valid git remote, so Push can exercise a real fast-forward push
// without touching GitHub.
func bareRemote(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping push integration test")
	}
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	return dir
}

// TestPush_SucceedsToLocalBareRemote: a real fast-forward push to a
// local bare remote lands on the remote (verified by reading the
// remote's branch tip back).
func TestPush_SucceedsToLocalBareRemote(t *testing.T) {
	repo := initRepo(t) // one commit on the default branch, HEAD born
	remote := bareRemote(t)

	branch := CurrentBranch(repo)
	runGit(t, repo, "remote", "add", "origin", remote)
	// Set upstream so a bare `git push` (zero args) has a destination —
	// this mirrors how `logmind log` calls Push(cwd) with no args.
	runGit(t, repo, "push", "-u", "origin", branch)

	// Add a second commit locally, then Push() it via the wrapper.
	writeAndCommit(t, repo, "second.txt", "second\n", "second")
	if err := Push(repo); err != nil {
		t.Fatalf("Push to local bare remote: %v", err)
	}

	// The remote's branch tip must now equal the local HEAD.
	localHead := revParse(t, repo, "HEAD")
	remoteHead := revParse(t, remote, branch)
	if localHead != remoteHead {
		t.Fatalf("remote %s = %q; want local HEAD %q (push did not land)", branch, remoteHead, localHead)
	}
}

// TestPush_NoRemoteReturnsErrorFast: a repo with NO remote configured
// makes `git push` fail fast (no configured destination). The wrapper
// must return a non-nil error quickly — never block.
func TestPush_NoRemoteReturnsErrorFast(t *testing.T) {
	repo := initRepo(t) // no `git remote add`

	start := time.Now()
	err := Push(repo)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Push with no remote returned nil; want non-nil error")
	}
	// "Fast" is a generous ceiling — the real point is it returns at all
	// (no hang). A local `git push` with no destination fails in well
	// under a second; 20s catches a genuine block without flaking on a
	// loaded CI box.
	if elapsed > 20*time.Second {
		t.Fatalf("Push took %v; expected a fast non-blocking failure", elapsed)
	}
}

// TestPush_BadRemoteReturnsErrorFast: an origin pointing at a
// nonexistent local path fails fast too. GIT_TERMINAL_PROMPT=0 +
// ssh BatchMode (set inside Push) guarantee no interactive prompt can
// stall this even for an auth-shaped URL; a bogus local path needs no
// auth and simply errors immediately.
func TestPush_BadRemoteReturnsErrorFast(t *testing.T) {
	repo := initRepo(t)
	branch := CurrentBranch(repo)
	// Point origin at a path that does not exist.
	runGit(t, repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "does-not-exist.git"))

	start := time.Now()
	err := Push(repo, "origin", branch)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Push to a nonexistent remote returned nil; want non-nil error")
	}
	if elapsed > 20*time.Second {
		t.Fatalf("Push took %v; expected a fast non-blocking failure", elapsed)
	}
}

// --- small local helpers (kept here so the push tests are self-contained) ---

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeAndCommit(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-q", "-m", msg)
}

func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse %s in %s: %v\n%s", ref, dir, err, out)
	}
	return string(out)
}
