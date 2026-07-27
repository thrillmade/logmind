// warp_test.go — exercises `logmind warp`: the read-only refresh of the two
// derived docs (docs/timeline.md, docs/file-structure.md) from the default
// branch, plus (v2.0.0 4b-ter) the merge-base repair step that moved here
// from the commit-path surfaces. Coverage:
//
//   - the refreshed working-tree copy matches origin/<default>'s content
//   - the refresh is NEVER staged (git status stays clean for the path), in
//     driver mode — the default, and every fixture in this file except
//     TestWarp_RepairsAlreadyDivergedBranch
//   - a non-repo cwd is a no-op, not an error
//   - never commits, even across repeated calls
//   - in integration-point mode, on a non-default branch, repairs an
//     already-diverged HEAD to the merge-base with the default branch
//     (staging the fix), preferring the repair over origin's raw tip when
//     the two disagree
package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initClonePair builds a real origin/repo pair on the local filesystem: origin
// is a normal (non-bare) working repo with an initial commit that seeds
// docs/timeline.md, and repo is a `git clone` of it — so repo's "origin"
// remote, tracking branch, and origin/HEAD symref are all set up exactly as
// they would be for a real GitHub clone. A non-bare origin (rather than
// gitcli_push_test.go's bareRemote) is deliberate: warp's test fixtures need
// to commit new content directly onto origin's working tree between fetches.
func initClonePair(t *testing.T) (origin, repo string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping integration test")
	}

	origin = t.TempDir()
	initLogTestGitRepo(t, origin)
	mustWriteUnder(t, origin, "docs/timeline.md", "ORIGIN-INITIAL\n")
	commitAll(t, origin, "init")

	repo = filepath.Join(t.TempDir(), "repo")
	cmd := exec.Command("git", "clone", "-q", origin, repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	runGitIn(t, repo, "config", "user.email", "test@example.com")
	runGitIn(t, repo, "config", "user.name", "Test")
	runGitIn(t, repo, "config", "commit.gpgsign", "false")
	return origin, repo
}

// commitOn writes content to relPath under dir and commits it — used to
// simulate main advancing on origin between a repo's clone and its next warp.
func commitOn(t *testing.T, dir, relPath, content string) {
	t.Helper()
	mustWriteUnder(t, dir, relPath, content)
	commitAll(t, dir, "advance "+relPath)
}

// isStaged reports whether relPath has any staged (index vs HEAD) change —
// used to assert warp never adds the refreshed docs to the index.
func isStaged(t *testing.T, dir, relPath string) bool {
	t.Helper()
	out := runGitOut(t, dir, "diff", "--cached", "--name-only")
	for _, line := range strings.Split(out, "\n") {
		if line == relPath {
			return true
		}
	}
	return false
}

func TestWarp_RefreshesFromOriginUncommitted(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/timeline.md", "MAIN-FRESH\n")
	runGitIn(t, repo, "fetch", "origin", "main") // pre-fetch so ShowFile sees it
	runGitIn(t, repo, "checkout", "-b", "feat/w")

	if err := runWarp(repo, io.Discard, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}
	if readFileStr(t, filepath.Join(repo, "docs", "timeline.md")) != "MAIN-FRESH\n" {
		t.Fatal("warp must refresh the working copy from origin/main")
	}
	if isStaged(t, repo, "docs/timeline.md") {
		t.Fatal("warp must NOT stage the refreshed docs")
	}
}

// TestWarp_DoesNotCommit: beyond not staging, warp must leave HEAD untouched
// — no new commit appears after a refresh.
func TestWarp_DoesNotCommit(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/timeline.md", "MAIN-FRESH-2\n")
	runGitIn(t, repo, "fetch", "origin", "main")
	runGitIn(t, repo, "checkout", "-b", "feat/w2")

	before := runGitOut(t, repo, "rev-parse", "HEAD")

	if err := runWarp(repo, io.Discard, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}

	after := runGitOut(t, repo, "rev-parse", "HEAD")
	if before != after {
		t.Fatalf("warp must not create a commit: HEAD moved from %q to %q", before, after)
	}
	// The working tree overall must still report the doc as modified
	// (unstaged) relative to HEAD, since HEAD did not move.
	status := runGitOut(t, repo, "status", "--porcelain", "--", "docs/timeline.md")
	if !strings.HasPrefix(status, " M") {
		t.Fatalf("expected docs/timeline.md to show as an unstaged modification; got %q", status)
	}
}

// TestWarp_FetchesFromOrigin: unlike a pre-fetch-only refresh, runWarp itself
// performs the `git fetch origin <default>` — so calling it WITHOUT a manual
// fetch first still picks up content committed on origin after the clone.
func TestWarp_FetchesFromOrigin(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/timeline.md", "MAIN-VIA-WARP-FETCH\n")
	runGitIn(t, repo, "checkout", "-b", "feat/w3")
	// Deliberately NOT fetching manually — runWarp must do it.

	if err := runWarp(repo, io.Discard, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}
	if readFileStr(t, filepath.Join(repo, "docs", "timeline.md")) != "MAIN-VIA-WARP-FETCH\n" {
		t.Fatal("warp must fetch origin itself, not rely on a pre-existing fetch")
	}
}

// TestWarp_NonRepo_NoOpNoError: running warp outside a git repo prints a
// notice and returns nil rather than erroring.
func TestWarp_NonRepo_NoOpNoError(t *testing.T) {
	dir := t.TempDir()
	var out strings.Builder
	if err := runWarp(dir, &out, io.Discard); err != nil {
		t.Fatalf("runWarp outside a repo should not error: %v", err)
	}
	if !strings.Contains(out.String(), "Not a git repo") {
		t.Fatalf("expected a not-a-repo notice; got %q", out.String())
	}
}

// TestWarp_ReportsDecisionCommitsAhead: the stdout summary reports how many
// decision-touching commits origin/<default> is ahead of HEAD.
func TestWarp_ReportsDecisionCommitsAhead(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/decisions.md", "## decision\n")
	runGitIn(t, repo, "checkout", "-b", "feat/w4")

	var out strings.Builder
	if err := runWarp(repo, &out, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}
	if !strings.Contains(out.String(), "decision commit(s) ahead") {
		t.Fatalf("expected the ahead-count summary in stdout; got %q", out.String())
	}
}

// TestWarp_RepairsAlreadyDivergedBranch is the v2.0.0 4b-ter proof that the
// merge-base repair capability now lives here, not on the commit-path
// surfaces (logmind log's L1, the pre-commit hook's L2a, the harness's
// L2b — see their doc comments in log.go/hooks.go/guard_commit.go for the
// staleness reasoning that moved it). Scenario: main gains the
// integration-point adoption commit; a feature branch is cut from it; then,
// simulating an old binary's local regen (or a hand edit) landing BEFORE any
// guard existed, a commit ALREADY on that branch's HEAD carries a diverged
// copy of docs/timeline.md. Meanwhile origin's main ALSO advances past the
// branch's fork point. `logmind warp` must fetch, then land the TRUE
// common-ancestor (merge-base) content — neither the branch's stale diverged
// copy NOR (per the CTO ruling: prefer the repair over raw freshness)
// origin's newer tip.
func TestWarp_RepairsAlreadyDivergedBranch(t *testing.T) {
	origin, repo := initClonePair(t)

	// Adopt integration-point mode on repo's main — this commit is the fork
	// point both origin's later advance and the feature branch below share.
	if err := os.MkdirAll(filepath.Join(repo, ".logmind"), 0o755); err != nil {
		t.Fatalf("mkdir .logmind: %v", err)
	}
	writeDerivedDocsMode(t, repo, "integration-point")
	commitAll(t, repo, "adopt integration-point mode")
	forkContent := readFileStr(t, filepath.Join(repo, "docs", "timeline.md"))

	// origin's main advances independently of repo's mode-adoption commit
	// (which never reached origin) — simulating other work landing on main
	// after this repo forked. The merge-base between this new origin tip and
	// the feature branch below is therefore the ORIGINAL clone-point commit,
	// not this fresher one.
	commitOn(t, origin, "docs/timeline.md", "MAIN-ADVANCED-FRESH\n")

	runGitIn(t, repo, "checkout", "-b", "feat/already-diverged")
	stale := "STALE — pre-existing diverged content\n"
	mustWriteUnder(t, repo, "docs/timeline.md", stale)
	commitAll(t, repo, "bad regen (pre-existing divergence, before any guard existed)")

	var out strings.Builder
	if err := runWarp(repo, &out, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}

	timelinePath := filepath.Join(repo, "docs", "timeline.md")
	got := readFileStr(t, timelinePath)
	if got != forkContent {
		t.Fatalf("warp did not repair to the merge-base content: got %q; want the fork-point content %q (must be neither the stale diverged copy %q nor origin's fresher tip)",
			got, forkContent, stale)
	}
	if !strings.Contains(out.String(), "repaired derived doc(s) to merge-base") {
		t.Fatalf("expected the repair note in stdout; got %q", out.String())
	}

	// Unlike the plain read-refresh loop (which deliberately never stages —
	// see TestWarp_RefreshesFromOriginUncommitted), the repair DOES stage the
	// corrected content: that's what lets a subsequent commit carry it.
	if !isStaged(t, repo, "docs/timeline.md") {
		t.Fatalf("expected the repaired docs/timeline.md to be staged")
	}

	// warp still never commits — HEAD must not move.
	before := runGitOut(t, repo, "rev-parse", "HEAD")
	if err := runWarp(repo, io.Discard, io.Discard); err != nil {
		t.Fatalf("second warp call: %v", err)
	}
	after := runGitOut(t, repo, "rev-parse", "HEAD")
	if before != after {
		t.Fatalf("warp must never commit: HEAD moved from %q to %q", before, after)
	}
}

// TestWarp_DriverModeSkipsRepair: the repair step is gated on
// integrationPointMode, matching L1/L2a/L2b's own gate — a driver-mode repo
// (implicit default: no `.logmind/config.yml` at all, same as every other
// warp fixture in this file) has no merge-base invariant to repair, so a
// diverged branch's HEAD content is left exactly as the plain read-refresh
// loop would leave it: origin's freshest tip, unmodified by any repair pass.
func TestWarp_DriverModeSkipsRepair(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/timeline.md", "MAIN-FRESH-DRIVER\n")

	runGitIn(t, repo, "checkout", "-b", "feat/driver-diverged")
	stale := "STALE ON A DRIVER-MODE BRANCH\n"
	mustWriteUnder(t, repo, "docs/timeline.md", stale)
	commitAll(t, repo, "driver-mode branch-local edit (never restored)")

	var out strings.Builder
	if err := runWarp(repo, &out, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}

	got := readFileStr(t, filepath.Join(repo, "docs", "timeline.md"))
	if got != "MAIN-FRESH-DRIVER\n" {
		t.Fatalf("driver mode: expected the plain read-refresh (origin's tip), got %q", got)
	}
	if strings.Contains(out.String(), "repaired derived doc(s)") {
		t.Fatalf("driver mode must never repair; unexpected repair note in stdout: %q", out.String())
	}
	if isStaged(t, repo, "docs/timeline.md") {
		t.Fatalf("driver mode must never stage the refreshed docs")
	}
}

// TestWarp_HelpRegistered smoke-tests that `logmind warp` is wired into the
// root command tree.
func TestWarp_HelpRegistered(t *testing.T) {
	root := NewRootCmd()
	cmd, _, err := root.Find([]string{"warp"})
	if err != nil {
		t.Fatalf("warp not registered on root: %v", err)
	}
	if cmd.Use != "warp" {
		t.Fatalf("unexpected command found: %q", cmd.Use)
	}
}
