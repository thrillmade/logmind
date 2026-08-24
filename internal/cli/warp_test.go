// warp_test.go — exercises `logmind warp`: the read-only refresh of the two
// derived docs (docs/timeline.md, docs/file-structure.md) from the default
// branch, plus (v2.0.0 4b-ter) the merge-base repair step that moved here
// from the commit-path surfaces.
//
// The repair applies to EVERY repo — the v2.0.0 B6 `derived_docs.mode`
// per-repo adoption gate is gone — but it fires only where the branch has
// ACTUALLY DIVERGED (divergedDerivedDocPaths, derived.go). Those are two
// different conditions that once shared one `if`; collapsing them made warp
// repair healthy branches and silently overwrite the refresh it had just
// announced in the same command. Coverage:
//
//   - the refreshed working-tree copy matches origin/<default>'s content
//     when the branch hasn't fallen behind (read-refresh and repair converge)
//   - a healthy branch KEEPS the refresh when main has advanced past its
//     fork point — nothing diverged, so nothing is repaired or staged
//   - a non-repo cwd is a no-op, not an error
//   - never commits, even across repeated calls — the refresh is left
//     modified-but-unstaged in the working tree, which is the contract
//   - on a non-default branch, repairs an already-diverged HEAD to the
//     merge-base with the default branch (staging the fix, which is the
//     §0.4.1.2 "deliberate" signal) — regardless of any
//     `.logmind/config.yml` content, including a leftover legacy
//     `derived_docs:` section
package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/testgit"
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
	// A clone does NOT inherit origin's gc.auto/maintenance.auto — those are
	// local, per-repo config that `git clone` doesn't copy — so the clone
	// needs testgit's fix applied again, independently (see its package doc).
	testgit.CloneRepo(t, repo, "-q", origin)
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

// TestWarp_RefreshesFromOriginUncommitted branches directly off the
// freshly-fetched origin/main tip, so this branch's merge-base WITH
// origin/main IS that tip — the (unconditional) repair step then converges
// with the plain read-refresh instead of overwriting it with older
// merge-base content, keeping this assertion meaningful. (A branch that has
// instead fallen BEHIND main is covered by TestWarp_RepairsAlreadyDivergedBranch
// below, where the repair's merge-base content is what wins.)
func TestWarp_RefreshesFromOriginUncommitted(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/timeline.md", "MAIN-FRESH\n")
	runGitIn(t, repo, "fetch", "origin", "main")
	runGitIn(t, repo, "checkout", "-b", "feat/w", "origin/main")

	if err := runWarp(repo, io.Discard, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}
	if readFileStr(t, filepath.Join(repo, "docs", "timeline.md")) != "MAIN-FRESH\n" {
		t.Fatal("warp must refresh the working copy from origin/main")
	}
	// feat/w's own HEAD already IS origin/main's tip, so the repair's
	// merge-base restore is a true no-op here: nothing to stage.
	if isStaged(t, repo, "docs/timeline.md") {
		t.Fatal("warp must not stage a doc that already matches its merge-base")
	}
}

// TestWarp_DoesNotCommit: warp must leave HEAD untouched — no new commit
// appears after a refresh, even once the (unconditional) repair step runs.
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
	// feat/w2 forked BEFORE origin's advance and never touched the derived
	// docs, so there is nothing to repair — the refresh survives and leaves
	// origin's newer content in the WORKING TREE, uncommitted. That is
	// warp's documented contract ("refresh ... read-only — not committed"),
	// and it is the whole reason the command exists.
	//
	// This assertion previously demanded a CLEAN tree, which was only true
	// while the repair fired unconditionally and overwrote the refresh it
	// had just announced. Inverted here rather than deleted: a clean tree
	// now means the refresh was silently discarded, which is the regression
	// TestWarp_HealthyBranch_KeepsRefreshWhenMainAdvanced exists to catch.
	status := runGitOut(t, repo, "status", "--porcelain", "--", "docs/timeline.md")
	if status == "" {
		t.Fatal("expected docs/timeline.md to carry the uncommitted refresh; a clean tree means warp discarded it")
	}
	// Uncommitted means UNSTAGED — a leading space in the porcelain XY pair.
	// Staging is the §0.4.1.2 "deliberate repair" signal and must not be
	// asserted on a branch with nothing to repair.
	if strings.HasPrefix(status, " ") == false {
		t.Fatalf("refresh must be left unstaged; porcelain status = %q", status)
	}
}

// TestWarp_FetchesFromOrigin: unlike a pre-fetch-only refresh, runWarp itself
// performs the `git fetch origin <default>` — so calling it WITHOUT a manual
// fetch first still updates refs/remotes/origin/<default> to origin's
// latest. Asserted via the ref itself rather than file content, so the
// assertion stays about the FETCH and doesn't silently double as a check on
// refresh-vs-repair precedence (which is covered directly by
// TestWarp_HealthyBranch_KeepsRefreshWhenMainAdvanced).
func TestWarp_FetchesFromOrigin(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/timeline.md", "MAIN-VIA-WARP-FETCH\n")
	runGitIn(t, repo, "checkout", "-b", "feat/w3")
	// Deliberately NOT fetching manually — runWarp must do it.

	before := strings.TrimSpace(runGitOut(t, repo, "rev-parse", "refs/remotes/origin/main"))

	if err := runWarp(repo, io.Discard, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}

	after := strings.TrimSpace(runGitOut(t, repo, "rev-parse", "refs/remotes/origin/main"))
	if before == after {
		t.Fatal("warp must fetch origin itself, not rely on a pre-existing fetch (refs/remotes/origin/main did not move)")
	}
	originTip := strings.TrimSpace(runGitOut(t, origin, "rev-parse", "HEAD"))
	if after != originTip {
		t.Fatalf("refs/remotes/origin/main = %q after warp; want origin's tip %q", after, originTip)
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
// staleness reasoning that moved it). Scenario: simulating an old binary's
// local regen (or a hand edit) landing BEFORE any guard existed, a commit
// ALREADY on a feature branch's HEAD carries a diverged copy of
// docs/timeline.md. Meanwhile origin's main ALSO advances past the branch's
// fork point (the repo's initial clone-point commit — see initClonePair).
// `logmind warp` must fetch, then land the TRUE common-ancestor (merge-base)
// content — neither the branch's stale diverged copy NOR (per the CTO
// ruling: prefer the repair over raw freshness) origin's newer tip. No
// `.logmind/config.yml` is written at all: the repair is unconditional.
func TestWarp_RepairsAlreadyDivergedBranch(t *testing.T) {
	origin, repo := initClonePair(t)
	forkContent := readFileStr(t, filepath.Join(repo, "docs", "timeline.md"))

	// origin's main advances independently of repo's clone point —
	// simulating other work landing on main after this repo forked. The
	// merge-base between this new origin tip and the feature branch below
	// is therefore the ORIGINAL clone-point commit, not this fresher one.
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

// TestWarp_RepairsRegardlessOfLegacyConfig pins the removal of the v2.0.0 B6
// `derived_docs.mode` adoption gate from warp's repair step: a leftover
// `derived_docs: {mode: driver}` section in .logmind/config.yml (from a
// repo that predates the gate's removal — the key is now unknown to
// config.Config and silently ignored) must NOT suppress the repair. There
// is no gate left to key off of, so this behaves identically to
// TestWarp_RepairsAlreadyDivergedBranch (no config at all).
func TestWarp_RepairsRegardlessOfLegacyConfig(t *testing.T) {
	origin, repo := initClonePair(t)
	if err := os.MkdirAll(filepath.Join(repo, ".logmind"), 0o755); err != nil {
		t.Fatalf("mkdir .logmind: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".logmind", "config.yml"), []byte("derived_docs:\n  mode: driver\n"), 0o644); err != nil {
		t.Fatalf("write legacy config.yml: %v", err)
	}
	commitAll(t, repo, "leftover legacy derived_docs config")
	forkContent := readFileStr(t, filepath.Join(repo, "docs", "timeline.md"))

	commitOn(t, origin, "docs/timeline.md", "MAIN-FRESH-DRIVER\n")

	runGitIn(t, repo, "checkout", "-b", "feat/driver-diverged")
	stale := "STALE ON A LEGACY-CONFIG BRANCH\n"
	mustWriteUnder(t, repo, "docs/timeline.md", stale)
	commitAll(t, repo, "pre-existing divergence despite legacy driver config")

	var out strings.Builder
	if err := runWarp(repo, &out, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}

	got := readFileStr(t, filepath.Join(repo, "docs", "timeline.md"))
	if got != forkContent {
		t.Fatalf("legacy derived_docs.mode: driver config must not suppress the repair: got %q; want the merge-base content %q", got, forkContent)
	}
	if !strings.Contains(out.String(), "repaired derived doc(s) to merge-base") {
		t.Fatalf("expected the repair note in stdout; got %q", out.String())
	}
	if !isStaged(t, repo, "docs/timeline.md") {
		t.Fatalf("expected the repaired docs/timeline.md to be staged")
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

// TestWarp_HealthyBranch_KeepsRefreshWhenMainAdvanced pins the fix for a
// regression v2.0.0's unconditional-invariant change introduced.
//
// Deleting the `derived_docs.mode` gate removed TWO conditions that shared one
// `if`: "has this repo adopted the mode" (correctly deleted) and "does this
// branch actually need repairing" (should not have been). Losing the second
// made the repair fire on every branch — including healthy ones — where it
// overwrote the read-refresh warp performs earlier in the SAME command. warp
// printed "refreshed N derived doc(s) from origin/main" and then discarded
// them one statement later.
//
// The existing TestWarp_RefreshesFromOriginUncommitted does NOT catch this:
// it branches directly off origin/main, so the merge-base IS origin's tip and
// refresh and repair agree by construction. The bug only shows when main
// advances AFTER the fork point, which is the ordinary case for any branch
// open longer than one merge.
func TestWarp_HealthyBranch_KeepsRefreshWhenMainAdvanced(t *testing.T) {
	origin, repo := initClonePair(t)
	runGitIn(t, repo, "fetch", "origin", "main")
	runGitIn(t, repo, "checkout", "-b", "feat/w", "origin/main")

	// origin advances AFTER the fork point, so merge-base != origin's tip.
	commitOn(t, origin, "docs/timeline.md", "MAIN-ADVANCED\n")

	if err := runWarp(repo, io.Discard, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}

	// feat/w never touched the derived docs, so there is nothing to repair
	// and the refresh must survive. Before the fix this read ORIGIN-INITIAL.
	if got := readFileStr(t, filepath.Join(repo, "docs", "timeline.md")); got != "MAIN-ADVANCED\n" {
		t.Fatalf("warp discarded the refresh it announced: timeline.md = %q, want %q", got, "MAIN-ADVANCED\n")
	}
	// Nothing diverged, so nothing may be staged — staging is the §0.4.1.2
	// "this is deliberate" signal, and asserting it on a healthy branch makes
	// the signal meaningless.
	if isStaged(t, repo, "docs/timeline.md") {
		t.Fatal("warp staged a derived doc on a branch with nothing to repair")
	}
}
