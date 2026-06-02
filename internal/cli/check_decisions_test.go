package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCheckDecisions_NotARepo asserts the early-skip path: not a git
// repo → "Not a git repository, skipping check." + exit 0.
func TestCheckDecisions_NotARepo(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runCheckDecisions(dir, 20, false, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_not_a_repo.golden", stdout.String())
}

// TestCheckDecisions_NoChanges runs against a fresh repo with no
// staged changes. Expected: "✓ 0 lines changed (below 20-line threshold)."
func TestCheckDecisions_NoChanges(t *testing.T) {
	repo := initRepo(t)
	var stdout bytes.Buffer
	if err := runCheckDecisions(repo, 20, false, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_clean.golden", stdout.String())
}

// TestCheckDecisions_OverThreshold stages 25 lines of non-docs code.
// Expected: warning + exit 1 (errSilentExit1).
func TestCheckDecisions_OverThreshold(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "source.txt", 25)
	var stdout bytes.Buffer
	err := runCheckDecisions(repo, 20, false, &stdout)
	if !errors.Is(err, errSilentExit1) {
		t.Fatalf("runCheckDecisions err = %v; want errSilentExit1", err)
	}
	checkGolden(t, "check_decisions_over_threshold.golden", stdout.String())
}

// TestCheckDecisions_OverThresholdNoFail verifies the same warning
// text but exit 0 when --no-fail is set.
func TestCheckDecisions_OverThresholdNoFail(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "source.txt", 25)
	var stdout bytes.Buffer
	if err := runCheckDecisions(repo, 20, true, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_over_threshold_nofail.golden", stdout.String())
}

// TestCheckDecisions_DocsStaged stages docs/decisions.md — must be
// recognised as documentation and short-circuit to the green path.
func TestCheckDecisions_DocsStaged(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "decisions.md"), []byte("decision\n"), 0o644); err != nil {
		t.Fatalf("write decisions.md: %v", err)
	}
	cmd := exec.Command("git", "add", "docs/decisions.md")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	var stdout bytes.Buffer
	if err := runCheckDecisions(repo, 20, false, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_docs_staged.golden", stdout.String())
}

// TestCheckDecisions_BranchDecisionStaged covers the branch-aware
// path: a staged docs/decisions-branches/<branch>.md must also be
// recognised as a decision file.
func TestCheckDecisions_BranchDecisionStaged(t *testing.T) {
	repo := initRepo(t)
	dir := filepath.Join(repo, "docs", "decisions-branches")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "feature.md"), []byte("decision\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := exec.Command("git", "add", "docs/decisions-branches/feature.md")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	var stdout bytes.Buffer
	if err := runCheckDecisions(repo, 20, false, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	// Reuses the docs-staged golden — the success message is the same
	// regardless of which decision file matched the predicate.
	checkGolden(t, "check_decisions_docs_staged.golden", stdout.String())
}

// TestCheckDecisions_CustomThreshold passes --threshold 50, stages
// 25 lines: must report "✓ 25 lines changed (below 50-line threshold)."
func TestCheckDecisions_CustomThreshold(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "source.txt", 25)
	var stdout bytes.Buffer
	if err := runCheckDecisions(repo, 50, false, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_under_threshold.golden", stdout.String())
}

// TestCheckDecisions_DocsLOCIsExempt — a 25-line change to a
// docs/*.md file MUST be excluded from the LOC count, because docs
// changes don't represent decisions-being-made.
func TestCheckDecisions_DocsLOCIsExempt(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stageLines(t, repo, "docs/random.md", 25)
	var stdout bytes.Buffer
	if err := runCheckDecisions(repo, 20, false, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_clean.golden", stdout.String())
}

// stageLines writes `count` lines of "line N\n" to `name` inside
// repo, then `git add` it.
func stageLines(t *testing.T, repo, name string, count int) {
	t.Helper()
	full := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var b bytes.Buffer
	for i := 1; i <= count; i++ {
		b.WriteString("line\n")
	}
	if err := os.WriteFile(full, b.Bytes(), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := exec.Command("git", "add", name)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add %s: %v\n%s", name, err, out)
	}
}
