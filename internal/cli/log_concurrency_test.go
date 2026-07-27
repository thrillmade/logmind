// log_concurrency_test.go — BLOCKER regression test for concurrent
// `logmind log` invocations against the SAME decisions file.
//
// Pre-fix, two problems combined to silently lose or misattribute
// decisions:
//
//  1. writeAtomic (timeline.go) wrote to a FIXED path+".tmp" file, so
//     concurrent writers targeting the same file raced on the
//     identical tmp path — one writer's os.Rename could fire on a tmp
//     file another writer had already renamed away, producing a
//     "rename ...tmp ...: no such file or directory" crash.
//  2. runLog had no lock around its read-modify-write-commit sequence:
//     two concurrent logs both read the pre-write content, both
//     appended their own entry in memory, and the last writeAtomic
//     call won — silently DROPPING every other concurrent decision,
//     while every single invocation printed success.
//
// This test builds the real binary and spawns N real `logmind log`
// subprocesses concurrently against the same target file (the
// default branch's docs/decisions.md), then asserts:
//
//	(a) no crashes — every invocation either lands cleanly or, on a
//	    saturated host, fails LOUD on the acquire timeout and is retried
//	    sequentially (never a crash, never a silent loss)
//	(b) every decision entry survives in the final file
//	(c) no cross-attribution — each summary is paired with its own
//	    reasoning, never another invocation's
//
// It uses --no-commit --no-push so the assertions isolate the pure
// file race from git's own index-locking behavior (a separate
// concern — see TestLogConcurrent_WithCommit_AllCommitsLand below,
// which drives a smaller N through the full commit path to confirm
// the lock also serializes `git add`/`git commit`).
//
// This test MUST fail against the pre-fix tree (verified before the
// fix landed) and MUST pass once both writeAtomic's unique-tmp-file
// fix and runLog's cross-process lock are in place.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestLogConcurrent_NoCrashes_NoLostDecisions is the mandatory
// regression test: N >= 10 concurrent `logmind log` invocations
// against the same decisions.md must all succeed and all survive.
func TestLogConcurrent_NoCrashes_NoLostDecisions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess concurrency test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; skipping subprocess concurrency test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping subprocess concurrency test")
	}

	binPath := buildLogmindBinaryForConcurrencyTest(t, goBin)
	repo := newConcurrencyTestRepo(t, binPath)

	const n = 16
	extraArgs := []string{"--no-commit", "--no-push", "--no-interactive"}
	results := runConcurrentLogs(t, binPath, repo, n, extraArgs)

	// (a) no crashes; a transient acquire-timeout under load is retried
	// sequentially so the strict all-present invariant below still holds.
	retryTimedOutSequentially(t, binPath, repo, extraArgs, results)

	content := readDecisions(t, repo)
	assertAllDecisionsPresent(t, content, n)
}

// TestLogConcurrent_WithCommit_AllCommitsLand drives a smaller N
// through the FULL commit path (no --no-commit) to confirm the lock
// also serializes `git add -A && git commit` — i.e. that each commit
// reflects exactly the decision it claims to log, with none clobbered
// or interleaved by a concurrent sibling. Smaller N keeps runtime
// reasonable since each invocation now pays for a real git commit.
func TestLogConcurrent_WithCommit_AllCommitsLand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess concurrency test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; skipping subprocess concurrency test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping subprocess concurrency test")
	}

	binPath := buildLogmindBinaryForConcurrencyTest(t, goBin)
	repo := newConcurrencyTestRepo(t, binPath)

	baseline := commitCount(t, repo)

	const n = 10
	extraArgs := []string{"--no-push", "--no-interactive"}
	results := runConcurrentLogs(t, binPath, repo, n, extraArgs)

	// A transient acquire-timeout under load is retried sequentially, so
	// the strict "exactly n decisions + n commits" invariant still holds
	// while a saturated host no longer flakes. A crash is never retried.
	retryTimedOutSequentially(t, binPath, repo, extraArgs, results)

	content := readDecisions(t, repo)
	assertAllDecisionsPresent(t, content, n)

	// Every successful log should have produced its own commit — if the
	// lock didn't serialize `git add -A && git commit`, concurrent
	// commits can clobber the index / HEAD and silently produce fewer
	// commits than invocations.
	got := commitCount(t, repo) - baseline
	if got != n {
		t.Errorf("commit count after %d concurrent logs = %d; want %d (lock should serialize git add/commit)", n, got, n)
	}
}

type concurrentLogResult struct {
	summary   string
	reasoning string
	out       string
	err       error
}

// runConcurrentLogs fires n `logmind log decision-<i> -r reason-<i>
// <extraArgs...>` subprocesses against repo as close to simultaneously
// as possible (a channel barrier releases every goroutine at once)
// and waits for them all to finish.
func runConcurrentLogs(t *testing.T, binPath, repo string, n int, extraArgs []string) []concurrentLogResult {
	t.Helper()
	var wg sync.WaitGroup
	results := make([]concurrentLogResult, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			summary := fmt.Sprintf("decision-%02d", i)
			reasoning := fmt.Sprintf("reason-%02d", i)
			args := append([]string{"log", summary, "-r", reasoning}, extraArgs...)
			cmd := exec.Command(binPath, args...)
			cmd.Dir = repo
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			runErr := cmd.Run()
			results[i] = concurrentLogResult{summary: summary, reasoning: reasoning, out: out.String(), err: runErr}
		}(i)
	}
	close(start)
	wg.Wait()
	return results
}

// acquireTimeoutRe recognizes the fail-loud outcome of acquireRepoLock:
// under a saturated host (e.g. the whole test suite running at once) a
// concurrent `logmind log` can legitimately exceed the 15s repo-lock
// acquire timeout and REFUSE to write rather than proceed unlocked. That
// is correct behavior — the decision is declined loudly, never silently
// lost — so the tests retry such an invocation SEQUENTIALLY (alone, it
// acquires instantly) instead of treating the transient timeout as data
// loss. A non-timeout failure (a crash / panic) is a real regression.
var acquireTimeoutRe = regexp.MustCompile(`could not acquire lock|appears stuck`)

// retryTimedOutSequentially re-runs, one at a time with no contention,
// any invocation that failed LOUD on the acquire timeout, so the strict
// "all N landed" invariant still holds under load without flaking. It
// fails the test on any NON-timeout error (a crash is never retried —
// that pre-fix crash/silent-loss is exactly what this suite must catch)
// and on any retry that still fails. On success it rewrites results[i]
// as a clean success so downstream assertions see the landed decision.
func retryTimedOutSequentially(t *testing.T, binPath, repo string, extraArgs []string, results []concurrentLogResult) {
	t.Helper()
	for i, r := range results {
		if r.err == nil {
			continue
		}
		if !acquireTimeoutRe.MatchString(r.out) {
			t.Errorf("invocation %s failed with a NON-timeout error (crash/regression?): %v\noutput:\n%s", r.summary, r.err, r.out)
			continue
		}
		t.Logf("invocation %s failed loud on the acquire timeout under load; retrying it sequentially", r.summary)
		args := append([]string{"log", r.summary, "-r", r.reasoning}, extraArgs...)
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repo
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			t.Errorf("sequential retry of %s failed: %v\noutput:\n%s", r.summary, err, out.String())
			continue
		}
		results[i] = concurrentLogResult{summary: r.summary, reasoning: r.reasoning, out: out.String(), err: nil}
	}
}

// decisionEntryRe matches a `logmind log` entry header + its
// Reasoning line, capturing the summary and the reasoning so callers
// can detect cross-attribution (a summary paired with a reasoning
// that isn't its own — the signature of a corrupted/interleaved
// concurrent write).
var decisionEntryRe = regexp.MustCompile(`(?m)^## \d{4}-\d{2}-\d{2} \d{2}:\d{2} - (decision-\d{2})\n\n\*\*Reasoning:\*\* (reason-\d{2})\n`)

// assertAllDecisionsPresent checks (b) every decision-00..decision-<n-1>
// survived in content, (c) each is paired with its own reasoning (not
// another invocation's), and that there are exactly n matches overall
// (no duplicates from a corrupted write).
func assertAllDecisionsPresent(t *testing.T, content string, n int) {
	t.Helper()
	matches := decisionEntryRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]string, len(matches))
	for _, m := range matches {
		seen[m[1]] = m[2]
	}

	var missing []string
	for i := 0; i < n; i++ {
		summary := fmt.Sprintf("decision-%02d", i)
		reasoning := fmt.Sprintf("reason-%02d", i)
		got, ok := seen[summary]
		if !ok {
			missing = append(missing, summary)
			continue
		}
		if got != reasoning {
			t.Errorf("cross-attribution: %s paired with reasoning %q; want %q", summary, got, reasoning)
		}
	}
	if len(missing) > 0 {
		t.Errorf("lost %d/%d decisions to the concurrent write race: %v\n\n--- final decisions.md ---\n%s",
			len(missing), n, missing, content)
	}
	if len(matches) != n {
		t.Errorf("found %d decision entries in decisions.md; want exactly %d (duplicates or corruption?)", len(matches), n)
	}
}

func readDecisions(t *testing.T, repo string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repo, "docs", "decisions.md"))
	if err != nil {
		t.Fatalf("read decisions.md: %v", err)
	}
	return string(body)
}

func commitCount(t *testing.T, repo string) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-list --count HEAD: %v", err)
	}
	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count); err != nil {
		t.Fatalf("parse commit count %q: %v", out, err)
	}
	return count
}

// buildLogmindBinaryForConcurrencyTest compiles ./cmd/logmind into a
// temp dir. Mirrors the buildGuardCommitBinary / TestVersionBinary_Subprocess
// pattern already used elsewhere in this package.
func buildLogmindBinaryForConcurrencyTest(t *testing.T, goBin string) string {
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

// newConcurrencyTestRepo creates a fresh temp git repo on `main`,
// scaffolds logmind via a real subprocess `logmind init`, disables
// auto_push (no remote exists in the test repo), and commits the
// scaffold so the working tree is clean before the concurrent
// `logmind log` invocations begin. Mirrors
// internal/timeline/merge_driver_test.go's setupLogmindRepo pattern.
func newConcurrencyTestRepo(t *testing.T, binPath string) string {
	t.Helper()
	repo := t.TempDir()

	runGitCmd(t, repo, "init", "-q", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "test@test.com")
	runGitCmd(t, repo, "config", "user.name", "test")
	runGitCmd(t, repo, "config", "commit.gpgsign", "false")

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test repo\n"), 0o644); err != nil {
		t.Fatalf("seed README.md: %v", err)
	}
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-qm", "initial")

	cmd := exec.Command(binPath, "init", "--github-actions=false")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("logmind init: %v\n%s", err, out)
	}

	// Disable auto_push — there's no remote in the test repo and a
	// `logmind log` that lands a commit would otherwise also attempt
	// (and fail/warn on) a push.
	cfgPath := filepath.Join(repo, ".logmind", "config.yml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	patched := strings.Replace(string(data), "auto_push: true", "auto_push: false", 1)
	if err := os.WriteFile(cfgPath, []byte(patched), 0o644); err != nil {
		t.Fatalf("rewrite config.yml: %v", err)
	}

	runGitCmd(t, repo, "add", "-A")
	runGitCmd(t, repo, "commit", "-qm", "logmind init scaffolding")

	return repo
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
