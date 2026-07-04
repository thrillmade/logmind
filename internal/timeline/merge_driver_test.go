//go:build integration

// Package timeline — merge_driver_test.go
//
// Multi-branch self-heal regression tests (B3 carry-forward of the
// Python v0.6.16 release). The whole dogfood loop depends on the
// merge driver + post-merge hook cooperatively producing a clean
// final tree when independent branches both run `logmind log` and
// merge back to main sequentially. Without these tests, every
// concurrent-PR cycle hits a check-derived-docs failure and the user
// has to `logmind rebase` per branch — the v0.6.15 tokenomics-agent
// regression that v0.6.16 surfaces here.
//
// The tests use REAL subprocess `git` + REAL subprocess `logmind`
// calls — no mocks. The merge driver shells out to `logmind timeline
// --write %A`; mocking it would defeat the purpose. The cost is that
// these tests need the logmind binary on PATH, so they live under a
// `//go:build integration` tag — `go test ./...` skips them, and
// `go test -tags=integration ./internal/timeline/...` runs them.
// CI installs the binary (`go install ./cmd/logmind`) before invoking
// the tagged run.
//
// Test surface (mirrors tests/test_merge_driver.py at v0.6.16):
//
//   - test_merge_driver_self_heals_two_concurrent_branches
//     Two independent branches, sequential merge to main. Asserts no
//     conflict markers + both decision-branches files present +
//     timeline shows 3 entries (init + A + B).
//
//   - test_merge_driver_self_heals_three_concurrent_branches
//     N=3 generalisation. Sequential merges; the post-merge regen is
//     amended into each merge commit (matches the realistic flow:
//     v0.6.7 leaves regen unstaged, and `git merge` refuses on dirty
//     tree, so users either amend or push-as-is).
//
//   - test_merge_driver_self_heals_squash_merge
//     The GitHub-default PR-merge pattern: agent A is squash-merged,
//     agent B's branch forked off pre-squash main. v0.6.13 (orphan
//     skip) + v0.6.16 (HEAD-vs-origin skip) MUST compose without
//     silently dropping entries.
//
// Why the integration tag and not e2e: existing CI workflows already
// build the Go binary and run tagged tests for cross-binary parity
// checks. `integration` matches that convention; `e2e` is reserved
// for browser-driven flows that don't exist in this repo.
package timeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// logmindBin returns the resolved path to a logmind binary on PATH, or
// "" + skip if none is available. The merge driver invokes `logmind
// timeline --write %A` via shell; without a real binary on PATH every
// test in this file can only spuriously fail.
//
// Memoised because the resolution is identical across all tests in
// the file; `exec.LookPath` is cheap but doing it once is clearer.
var logmindOnPath = func() string {
	bin, err := exec.LookPath("logmind")
	if err != nil {
		return ""
	}
	return bin
}()

// skipIfNoLogmind keeps the boilerplate at one line in every test.
func skipIfNoLogmind(t *testing.T) {
	t.Helper()
	if logmindOnPath == "" {
		t.Skip("`logmind` not on PATH; merge driver can't shell out — skipping integration test. " +
			"Build + install with `go install ./cmd/logmind` (or the Python equivalent) and re-run with `-tags=integration`.")
	}
}

// setupLogmindRepo creates a temp dir, inits a git repo with branch
// `main`, runs `logmind init --no-skill-install`, disables auto_push
// (no remote in tests), and commits the scaffolding into a clean
// initial state. Returns the repo root.
//
// Mirrors Python `_setup_logmind_repo` in tests/test_merge_driver.py.
func setupLogmindRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	runGit(t, repo, "init", "-q", "-b", "main")
	runGit(t, repo, "config", "user.email", "test@test.com")
	runGit(t, repo, "config", "user.name", "test")
	runGit(t, repo, "config", "commit.gpgsign", "false")

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test repo\n"), 0o644); err != nil {
		t.Fatalf("seed README.md: %v", err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-qm", "initial")

	// logmind init scaffolds docs/, .gitattributes, config, hooks,
	// merge-driver git config. `--skill-install no` skips the npx
	// skills.sh install which would fail without network in CI.
	cmd := exec.Command(logmindOnPath, "init", "--skill-install", "no")
	cmd.Dir = repo
	cmd.Env = testEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("logmind init failed: %v\noutput:\n%s", err, out)
	}

	// Disable auto_push — there's no remote in the test repo and
	// `logmind log`'s push step would otherwise fail at the end.
	cfg := filepath.Join(repo, ".logmind", "config.yml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	patched := strings.Replace(string(data), "auto_push: true", "auto_push: false", 1)
	if err := os.WriteFile(cfg, []byte(patched), 0o644); err != nil {
		t.Fatalf("rewrite config.yml: %v", err)
	}

	// Stage everything logmind init produced (docs/, AGENTS.md,
	// .logmind/, .gitattributes, hook files) and fold into the
	// initial-commit baseline. Subsequent tests start from a clean
	// tree with the merge driver already configured.
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-qm", "logmind init scaffolding")

	return repo
}

// logmindLog runs `logmind log <summary> -r <reason> -a <alt> -i <impl>`
// in `repo`. Returns the captured combined output for caller assertions
// when the exit code isn't 0. Mirrors Python `_logmind_log`.
func logmindLog(t *testing.T, repo, summary, reason, alt, impl string) {
	t.Helper()
	cmd := exec.Command(logmindOnPath,
		"log", summary,
		"-r", reason,
		"-a", alt,
		"-i", impl,
	)
	cmd.Dir = repo
	cmd.Env = testEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("logmind log %q failed: %v\noutput:\n%s", summary, err, out)
	}
}

// testEnv is the environment passed to EVERY subprocess git/logmind
// call in this file. CRITICALLY includes the parent test process's
// env so things like PYENV_VERSION, PATH, HOME propagate into git
// hooks. Without this, git invokes the hook with a clean env, the
// pyenv shim resolves `logmind` to "system" Python, and we get an
// arbitrary older binary (locally observed: 0.3.4) producing
// different brief-mode output than the v0.6.16 binary the test
// expects. Mirrors Python's `{**os.environ, "LOGMIND_QUIET": "1"}`
// inheritance.
func testEnv() []string {
	return append(os.Environ(), "LOGMIND_QUIET=1")
}

// runGit shells `git <args>` in repo with `check=true` semantics
// (fatal on non-zero exit). The merge-driver tests below use it for
// every git operation except the merges themselves (which need their
// own assertion on the exit code).
func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	cmd.Env = testEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\noutput:\n%s", strings.Join(args, " "), err, out)
	}
}

// runGitMerge runs `git merge` and returns (exitCode, combined output)
// so the caller can assert on the merge's success vs failure separately
// — the merge driver MUST keep this exit code at 0 even on conflicting
// derived files. Mirrors the Python tests' explicit capture pattern.
func runGitMerge(t *testing.T, repo string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"merge"}, args...)...)
	cmd.Dir = repo
	cmd.Env = testEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), string(out)
		}
		t.Fatalf("git merge unable to run: %v", err)
	}
	return 0, string(out)
}

// amendPendingRegen folds the post-merge hook's regen output into the
// just-created merge commit. Without it, subsequent `git merge`
// operations refuse to proceed on the dirty working tree (v0.6.7
// contract leaves docs/timeline.md + docs/file-structure.md unstaged).
//
// Mirrors Python `_amend_pending_regen`.
func amendPendingRegen(t *testing.T, repo string) {
	t.Helper()
	// Best-effort add (the files may not exist if regen no-op'd) —
	// ignore exit code.
	cmd := exec.Command("git", "add", "docs/timeline.md", "docs/file-structure.md")
	cmd.Dir = repo
	cmd.Env = testEnv()
	_ = cmd.Run()

	// Diff exit code 1 = staged diff present; 0 = nothing to amend.
	diffCmd := exec.Command("git", "diff", "--cached", "--quiet")
	diffCmd.Dir = repo
	diffCmd.Env = testEnv()
	if err := diffCmd.Run(); err != nil {
		// Non-zero → staged diff present → amend.
		runGit(t, repo, "commit", "--amend", "--no-edit", "-q")
	}
}

// assertNoConflictMarkers — the merge driver's whole purpose is to
// leave NO conflict markers in derived docs. If any survive, the
// driver didn't fire (or fired wrong) and the test must fail loudly.
//
// Mirrors Python `_assert_no_conflict_markers`.
func assertNoConflictMarkers(t *testing.T, text, label string) {
	t.Helper()
	for _, marker := range []string{"<<<<<<< ", ">>>>>>> ", "======="} {
		if strings.Contains(text, marker) {
			t.Fatalf("%s: merge driver left conflict marker %q — driver did NOT auto-resolve.\nFull text:\n%s",
				label, marker, text)
		}
	}
}

// assertDecisionBranchesPresent verifies that every branch's per-branch
// decision file landed in `docs/decisions-branches/feat__<name>.md`
// after the merges. The file must be non-empty.
//
// Mirrors Python `_assert_decision_branches_present`.
func assertDecisionBranchesPresent(t *testing.T, repo string, branches []string, label string) {
	t.Helper()
	for _, branch := range branches {
		path := filepath.Join(repo, "docs", "decisions-branches", fmt.Sprintf("feat__%s.md", branch))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: docs/decisions-branches/feat__%s.md missing — merge did not bring in branch %q's decision file (err: %v)",
				label, branch, branch, err)
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			t.Fatalf("%s: decision-branches/feat__%s.md is empty", label, branch)
		}
	}
}

// assertTimelineHasDecisionCount asserts that the main-canonical timeline
// contains exactly expectedCount rows. The v2.0.0 timeline is the §1.6.4
// entry-block union: one row per source (each direct-to-main / archive
// decision, plus one headline row per branch detail page), wrapped in a
// `<!-- logmind-entry-start: … -->` marker. Counting those markers is the
// durable signal that the regen saw ALL decision sources — the same guarantee
// the old brief-mode `(N decisions)` header count gave.
func assertTimelineHasDecisionCount(t *testing.T, repo string, expectedCount int, label string) {
	t.Helper()
	timeline, err := os.ReadFile(filepath.Join(repo, "docs", "timeline.md"))
	if err != nil {
		t.Fatalf("%s: read docs/timeline.md: %v", label, err)
	}
	got := strings.Count(string(timeline), "<!-- logmind-entry-start: ")
	if got != expectedCount {
		t.Fatalf("%s: expected %d timeline rows (entry-block markers), got %d:\n%s",
			label, expectedCount, got, timeline)
	}
}

// TestMergeDriverSelfHealsTwoConcurrentBranches — THE keystone test
// for the dogfood loop. Two agents on independent branches off main,
// both run `logmind log`, both merge to main sequentially. The merge
// driver + post-merge hook MUST cooperatively produce a final main
// where both decision files are present, no conflict markers leak,
// and the timeline reflects all three entries (init + A + B).
//
// Mirrors Python `test_merge_driver_self_heals_two_concurrent_branches`.
func TestMergeDriverSelfHealsTwoConcurrentBranches(t *testing.T) {
	skipIfNoLogmind(t)
	repo := setupLogmindRepo(t)

	// Agent A's branch
	runGit(t, repo, "checkout", "-q", "-b", "feat/agent-a")
	logmindLog(t, repo,
		"Decision A: pick PostgreSQL",
		"ACID + complex joins",
		"MongoDB",
		"connection pooling required",
	)

	// Agent B's branch (OFF main, not off A)
	runGit(t, repo, "checkout", "-q", "main")
	runGit(t, repo, "checkout", "-q", "-b", "feat/agent-b")
	logmindLog(t, repo,
		"Decision B: pick Redis for cache",
		"fast session storage",
		"Memcached",
		"run Redis server",
	)

	// Merge A first (no concurrent conflict — only A's branch ever wrote
	// timeline with content).
	runGit(t, repo, "checkout", "-q", "main")
	if code, out := runGitMerge(t, repo, "feat/agent-a", "--no-ff", "-m", "merge a"); code != 0 {
		t.Fatalf("merge of agent-a failed (exit %d):\n%s", code, out)
	}

	// THIS IS THE SELF-HEAL MOMENT.
	// Branch B's timeline.md was regen'd off main BEFORE A landed — its
	// "Recent decisions" lines contain only B. Main's timeline (post-A)
	// contains only A. Textual merge would conflict; the merge driver +
	// post-merge hook must cooperate to produce a clean tree with both.
	code, out := runGitMerge(t, repo, "feat/agent-b", "--no-ff", "-m", "merge b")
	if code != 0 {
		t.Fatalf("merge driver MUST auto-resolve timeline.md on concurrent-branch merge.\n"+
			"git merge exit code: %d\n"+
			"output:\n%s\n"+
			"Likely cause: merge driver not configured per-clone (`logmind init` "+
			"didn't run `git config merge.logmind-timeline.driver`), or `logmind` "+
			"binary not on PATH for the driver shell-out. Re-run `logmind init` "+
			"and `logmind doctor` to surface the gap.",
			code, out)
	}

	// Working tree contract — both branches' decision files survive merge.
	assertDecisionBranchesPresent(t, repo, []string{"agent-a", "agent-b"}, "two-branch self-heal")

	// Timeline contract — no conflict markers + decision count = 3 (init+A+B).
	finalTimeline, err := os.ReadFile(filepath.Join(repo, "docs", "timeline.md"))
	if err != nil {
		t.Fatalf("read final timeline: %v", err)
	}
	assertNoConflictMarkers(t, string(finalTimeline), "docs/timeline.md after B merge")
	assertTimelineHasDecisionCount(t, repo, 3, "two-branch self-heal")
}

// TestMergeDriverSelfHealsThreeConcurrentBranches — N=3 generalisation.
// Three branches off main, three sequential merges. Validates the
// driver scales beyond pairwise — each merge regenerates timeline
// from the cumulative decisions-branches/ tree.
//
// Mirrors Python `test_merge_driver_self_heals_three_concurrent_branches`.
func TestMergeDriverSelfHealsThreeConcurrentBranches(t *testing.T) {
	skipIfNoLogmind(t)
	repo := setupLogmindRepo(t)

	type seed struct{ name, summary, alt string }
	for _, s := range []seed{
		{"a", "Decision A: use REST", "GraphQL"},
		{"b", "Decision B: use Redis", "Memcached"},
		{"c", "Decision C: use Pytest", "unittest"},
	} {
		runGit(t, repo, "checkout", "-q", "main")
		runGit(t, repo, "checkout", "-q", "-b", "feat/agent-"+s.name)
		logmindLog(t, repo, s.summary, "reasoning for "+s.name, s.alt, "implication "+s.name)
	}

	// Merge sequentially: a → b → c. After 'a', timeline has just A. After
	// 'b', driver must merge B's regen (only B) into main's (only A) → both.
	// After 'c', driver must do the same dance against the existing A+B.
	runGit(t, repo, "checkout", "-q", "main")
	for _, name := range []string{"a", "b", "c"} {
		code, out := runGitMerge(t, repo, "feat/agent-"+name, "--no-ff", "-m", "merge "+name)
		if code != 0 {
			t.Fatalf("merge of agent-%s failed — driver didn't auto-resolve (exit %d):\n%s",
				name, code, out)
		}
		// Fold the post-merge regen into the merge commit so the next
		// iteration's `git merge` doesn't refuse on dirty working tree.
		amendPendingRegen(t, repo)
	}

	// Working tree + timeline contracts.
	assertDecisionBranchesPresent(t, repo, []string{"agent-a", "agent-b", "agent-c"}, "3-branch self-heal")
	finalTimeline, err := os.ReadFile(filepath.Join(repo, "docs", "timeline.md"))
	if err != nil {
		t.Fatalf("read final timeline: %v", err)
	}
	assertNoConflictMarkers(t, string(finalTimeline), "docs/timeline.md after 3-branch merge")
	// init + A + B + C = 4
	assertTimelineHasDecisionCount(t, repo, 4, "3-branch self-heal")
}

// TestMergeDriverSelfHealsSquashMerge — squash-merge variant. Agent A
// squash-merges (the GitHub default for PR merges); agent B's branch
// was forked BEFORE the squash. When B merges to the post-squash main,
// git replays B's timeline regen against a main whose history was
// compressed — the driver must still fire on the textual conflict and
// emit a clean final tree.
//
// Composes with the v0.6.13 + v0.6.16 fixes (orphan-branch skip +
// HEAD-vs-origin skip). If those skips fire incorrectly, the regen
// would silently no-op and we'd lose decisions; the test catches that
// by asserting both decisions land in the final timeline.
//
// Mirrors Python `test_merge_driver_self_heals_squash_merge`.
func TestMergeDriverSelfHealsSquashMerge(t *testing.T) {
	skipIfNoLogmind(t)
	repo := setupLogmindRepo(t)

	// Agent A's branch
	runGit(t, repo, "checkout", "-q", "-b", "feat/agent-a")
	logmindLog(t, repo,
		"Decision A: pick Postgres",
		"ACID",
		"MongoDB",
		"pool connections",
	)

	// Agent B forks from main BEFORE A is merged (a parallel agent)
	runGit(t, repo, "checkout", "-q", "main")
	runGit(t, repo, "checkout", "-q", "-b", "feat/agent-b")
	logmindLog(t, repo,
		"Decision B: pick pytest",
		"fixtures",
		"unittest",
		"adopt fixtures pattern",
	)

	// Squash-merge A into main (replicates `gh pr merge --squash`)
	runGit(t, repo, "checkout", "-q", "main")
	if code, out := runGitMerge(t, repo, "--squash", "feat/agent-a"); code != 0 {
		t.Fatalf("squash --merge failed (exit %d):\n%s", code, out)
	}
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-qm", "squash merge a")

	// Now merge B's branch (forked off pre-squash main). Timeline conflict
	// is unavoidable textually — driver must auto-resolve.
	code, out := runGitMerge(t, repo, "feat/agent-b", "--no-ff", "-m", "merge b")
	if code != 0 {
		t.Fatalf("merge driver MUST auto-resolve timeline.md on the squash-then-merge "+
			"case — this is the common GitHub PR cycle pattern.\n"+
			"exit %d, output:\n%s", code, out)
	}

	// Working tree + timeline contracts — squash-merge variant.
	assertDecisionBranchesPresent(t, repo, []string{"agent-a", "agent-b"}, "squash + merge")
	finalTimeline, err := os.ReadFile(filepath.Join(repo, "docs", "timeline.md"))
	if err != nil {
		t.Fatalf("read final timeline: %v", err)
	}
	assertNoConflictMarkers(t, string(finalTimeline), "docs/timeline.md after squash + merge")
	assertTimelineHasDecisionCount(t, repo, 3, "squash + merge")
}
