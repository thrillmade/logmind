package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thrillmade/logmind/internal/skill"
)

// fixedNow pins last-refined for the rewrite assertions in the CLI
// layer just as it does in the skill package tests.
var fixedNow = time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)

// scaffoldSkillForSync mirrors the helper in internal/skill/sync_test.go
// but lives here because cross-package test helpers in Go require
// either exporting them or duplicating. Duplication keeps the test
// surface stable for snapshot tooling.
func scaffoldSkillForSync(t *testing.T, root, name string) {
	t.Helper()
	if _, err := skill.ScaffoldBasic(root, name, "desc"); err != nil {
		t.Fatalf("scaffold %s: %v", name, err)
	}
	if err := skill.WriteProvenanceSkeleton(skill.MDPath(root, name), name); err != nil {
		t.Fatalf("provenance %s: %v", name, err)
	}
}

// writeReviewForSync drops a SPEC-format review file under
// docs/reviews/.
func writeReviewForSync(t *testing.T, root, filename, sha string, citations []string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir reviews: %v", err)
	}
	body := "# clud-bug review — PR #1\n"
	body += "<!-- protocol-version: 0.1.0 -->\n"
	body += "<!-- written-by: clud-bug[bot] -->\n"
	body += "<!-- review-sha: " + sha + " -->\n\n"
	body += "**Summary:** N · N · N\n\n"
	if len(citations) > 0 {
		body += "**Skills cited:**\n"
		for _, c := range citations {
			body += "- " + c + "\n"
		}
		body += "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(body), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
}

// TestRunSync_NoReviewsDir: missing docs/reviews/ → ok line with zero
// counters.
func TestRunSync_NoReviewsDir(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	if err := runSync(dir, false, fixedNow, &stdout, &stderr); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "no skills updated") {
		t.Errorf("expected zero-work line; got:\n%s", got)
	}
	if !strings.Contains(got, "ok sync: 0 skill(s) updated") {
		t.Errorf("expected ok line; got:\n%s", got)
	}
}

// TestRunSync_AppliesCitations: one PR, one skill — happy-path
// integration that exercises CLI → skill.Sync → on-disk rewrite.
func TestRunSync_AppliesCitations(t *testing.T) {
	dir := t.TempDir()
	scaffoldSkillForSync(t, dir, "cli-skill")
	writeReviewForSync(t, dir, "PR-1.md", strings.Repeat("a", 40), []string{
		"cli-skill (4 findings)",
	})

	var stdout, stderr bytes.Buffer
	if err := runSync(dir, false, fixedNow, &stdout, &stderr); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "1 skill(s) updated") {
		t.Errorf("missing updated count; stdout:\n%s", got)
	}
	if !strings.Contains(got, "4 citation(s) added") {
		t.Errorf("missing citation count; stdout:\n%s", got)
	}
	if !strings.Contains(got, "cli-skill: 0 → 4") {
		t.Errorf("missing per-skill diff; stdout:\n%s", got)
	}

	body, _ := os.ReadFile(filepath.Join(skill.SkillDir(dir, "cli-skill"), "PROVENANCE.md"))
	if !strings.Contains(string(body), "cited-by-clud-bug: 4") {
		t.Errorf("counter not bumped; body:\n%s", body)
	}
}

// TestRunSync_DryRun: --dry-run path emits the same summary but
// leaves disk untouched.
func TestRunSync_DryRun(t *testing.T) {
	dir := t.TempDir()
	scaffoldSkillForSync(t, dir, "dry-cli-skill")
	provPath := filepath.Join(skill.SkillDir(dir, "dry-cli-skill"), "PROVENANCE.md")
	before, _ := os.ReadFile(provPath)

	writeReviewForSync(t, dir, "PR-1.md", strings.Repeat("a", 40), []string{
		"dry-cli-skill (2 findings)",
	})

	var stdout, stderr bytes.Buffer
	if err := runSync(dir, true, fixedNow, &stdout, &stderr); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "(dry-run)") {
		t.Errorf("dry-run marker missing; stdout:\n%s", got)
	}
	// Bug 2 (review #135): the machine-parseable `ok sync:` line MUST
	// carry the `(dry-run)` marker so downstream consumers can
	// distinguish dry-run preview output from real-run output.
	if !strings.Contains(got, "ok sync: (dry-run) 1 skill(s) updated") {
		t.Errorf("ok line missing (dry-run) marker; stdout:\n%s", got)
	}
	// Cross-check: in dry-run mode, the bare (non-dry-run) `ok sync:`
	// line shape MUST NOT appear — otherwise a grep for `^ok sync: \d+`
	// would still match the dry-run preview and contaminate the counter.
	if strings.Contains(got, "ok sync: 1 skill(s) updated") {
		t.Errorf("ok line should carry (dry-run) prefix in dry-run mode; stdout:\n%s", got)
	}

	after, _ := os.ReadFile(provPath)
	if string(before) != string(after) {
		t.Errorf("dry-run touched disk")
	}
}

// TestRunSync_UnknownSkillWarning: a citation targeting a missing
// skill emits a warn-line on stderr; runSync still succeeds.
func TestRunSync_UnknownSkillWarning(t *testing.T) {
	dir := t.TempDir()
	writeReviewForSync(t, dir, "PR-1.md", strings.Repeat("a", 40), []string{
		"missing-skill (1 finding)",
	})

	var stdout, stderr bytes.Buffer
	if err := runSync(dir, false, fixedNow, &stdout, &stderr); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	if !strings.Contains(stderr.String(), "missing-skill") {
		t.Errorf("expected warn for missing skill; stderr:\n%s", stderr.String())
	}
	// Even with the warn, runSync exits cleanly.
	if !strings.Contains(stdout.String(), "ok sync:") {
		t.Errorf("ok line missing; stdout:\n%s", stdout.String())
	}
}

// TestNewSyncCmd_RegisteredOnRoot: the root command tree exposes
// `sync` so `logmind sync` is reachable. The cobra test surface is
// the same one TestNewRootCmd uses for the other B-wave commands.
func TestNewSyncCmd_RegisteredOnRoot(t *testing.T) {
	root := NewRootCmd()
	found := false
	for _, c := range root.Commands() {
		if c.Use == "sync" {
			found = true
			break
		}
	}
	if !found {
		t.Error("`sync` subcommand not registered on root")
	}
}

// TestRunSync_ParentReviewsDirReadable_ButReviewsDirAbsent: when
// docs/ exists but docs/reviews/ doesn't, Sync returns the "nothing
// to do" path. Pins the os.ErrNotExist short-circuit.
func TestRunSync_DocsButNoReviewsDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := runSync(dir, false, fixedNow, &stdout, &stderr); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	if !strings.Contains(stdout.String(), "no skills updated") {
		t.Errorf("expected no-op; stdout:\n%s", stdout.String())
	}
}

// TestRunSync_PropagatesFilesystemError: when docs/reviews exists but
// is unreadable, runSync returns ErrSilent and surfaces an Error line
// to stdout. Skipped on platforms where root can read everything
// (effectively: testing under CI with elevated privileges).
func TestRunSync_FilesystemError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — chmod 000 doesn't block reads")
	}
	dir := t.TempDir()
	reviewsDir := filepath.Join(dir, "docs", "reviews")
	if err := os.MkdirAll(reviewsDir, 0o755); err != nil {
		t.Fatalf("mkdir reviews: %v", err)
	}
	// Block traversal — os.ReadDir returns EACCES.
	if err := os.Chmod(reviewsDir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(reviewsDir, 0o755) // restore so t.TempDir cleanup works

	var stdout, stderr bytes.Buffer
	err := runSync(dir, false, fixedNow, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("expected ErrSilent on read failure; got %v", err)
	}
	if !strings.Contains(stdout.String(), "Error:") {
		t.Errorf("expected Error: line on stdout; got:\n%s", stdout.String())
	}
}
