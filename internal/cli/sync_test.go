package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

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
	if err := runSync(dir, SyncCLIOptions{}, fixedNow, &stdout, &stderr); err != nil {
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
	if err := runSync(dir, SyncCLIOptions{}, fixedNow, &stdout, &stderr); err != nil {
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
	if err := runSync(dir, SyncCLIOptions{DryRun: true}, fixedNow, &stdout, &stderr); err != nil {
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
	if err := runSync(dir, SyncCLIOptions{}, fixedNow, &stdout, &stderr); err != nil {
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
	if err := runSync(dir, SyncCLIOptions{}, fixedNow, &stdout, &stderr); err != nil {
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
	err := runSync(dir, SyncCLIOptions{}, fixedNow, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("expected ErrSilent on read failure; got %v", err)
	}
	if !strings.Contains(stdout.String(), "Error:") {
		t.Errorf("expected Error: line on stdout; got:\n%s", stdout.String())
	}
}

// TestRunSync_MalformedSince: a garbage --since value is a hard error —
// runSync must not silently fall back to "scan everything" (which is
// what a zero-valued/ignored duration would produce).
func TestRunSync_MalformedSince(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runSync(dir, SyncCLIOptions{Since: "not-a-duration"}, fixedNow, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("expected ErrSilent for malformed --since; got %v", err)
	}
	if !strings.Contains(stdout.String(), "Error:") {
		t.Errorf("expected Error: line on stdout; got:\n%s", stdout.String())
	}
}

// TestNewSyncCmd_RegistersNewFlags: --since, --update-provenance, and
// --write-drafts are all reachable on the cobra command tree (not just
// wired internally) — a regression guard against a flag silently
// dropping off `newSyncCmd`.
func TestNewSyncCmd_RegistersNewFlags(t *testing.T) {
	root := NewRootCmd()
	var syncCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "sync" {
			syncCmd = c
			break
		}
	}
	if syncCmd == nil {
		t.Fatal("sync subcommand not found")
	}
	for _, name := range []string{"dry-run", "since", "update-provenance", "write-drafts"} {
		if syncCmd.Flags().Lookup(name) == nil {
			t.Errorf("--%s not registered on `logmind sync`", name)
		}
	}
}

// TestRunSync_UpdateProvenance_WritesSpecFormat: --update-provenance
// composes with the legacy default path — both run in the same
// invocation, and the new PROVENANCE.md carries the SPEC §1.11.1
// marker.
func TestRunSync_UpdateProvenance_WritesSpecFormat(t *testing.T) {
	dir := t.TempDir()
	scaffoldSkillForSync(t, dir, "prov-cli-skill")

	var stdout, stderr bytes.Buffer
	opts := SyncCLIOptions{UpdateProvenance: true}
	if err := runSync(dir, opts, fixedNow, &stdout, &stderr); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "ok sync-provenance:") {
		t.Errorf("missing ok sync-provenance line; stdout:\n%s", got)
	}
	body, err := os.ReadFile(filepath.Join(skill.SkillDir(dir, "prov-cli-skill"), "PROVENANCE.md"))
	if err != nil {
		t.Fatalf("read PROVENANCE.md: %v", err)
	}
	if !strings.Contains(string(body), "<!-- maintained-by: logmind sync -->") {
		t.Errorf("PROVENANCE.md missing SPEC marker after --update-provenance; body:\n%s", body)
	}
}

// TestRunSync_WriteDrafts_WritesSkillsDerived: --write-drafts writes
// docs/skills-derived/<slug>.md from decision-log patterns, alongside
// the (unconditional) legacy sync pass.
func TestRunSync_WriteDrafts_WritesSkillsDerived(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	body := "# Decisions\n\n" +
		"## 2026-05-15 - first\n\n**Date**: 2026-05-15\n\nadopt rate-limiting everywhere.\n\n" +
		"## 2026-05-20 - second\n\n**Date**: 2026-05-20\n\nmore rate-limiting work.\n\n" +
		"## 2026-05-25 - third\n\n**Date**: 2026-05-25\n\nrate-limiting again.\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write decisions.md: %v", err)
	}

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var stdout, stderr bytes.Buffer
	opts := SyncCLIOptions{WriteDrafts: true}
	if err := runSync(dir, opts, now, &stdout, &stderr); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "ok sync-drafts:") {
		t.Errorf("missing ok sync-drafts line; stdout:\n%s", got)
	}
	draftBody, err := os.ReadFile(filepath.Join(dir, "docs", "skills-derived", "rate-limiting.md"))
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	if !strings.Contains(string(draftBody), "status: candidate") {
		t.Errorf("draft missing status: candidate; body:\n%s", draftBody)
	}
}

// TestRunSync_DryRun_ComposesWithNewFlags: --dry-run together with
// --update-provenance and --write-drafts previews everything but writes
// nothing — neither a new PROVENANCE.md nor a new docs/skills-derived/
// file appears on disk.
func TestRunSync_DryRun_ComposesWithNewFlags(t *testing.T) {
	dir := t.TempDir()
	scaffoldSkillForSync(t, dir, "dry-compose-skill")
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	body := "# Decisions\n\n" +
		"## 2026-05-15 - first\n\n**Date**: 2026-05-15\n\nadopt cache-warming everywhere.\n\n" +
		"## 2026-05-20 - second\n\n**Date**: 2026-05-20\n\nmore cache-warming work.\n\n" +
		"## 2026-05-25 - third\n\n**Date**: 2026-05-25\n\ncache-warming again.\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write decisions.md: %v", err)
	}

	// Snapshot the entire tree before the dry-run so we can assert
	// nothing changed, rather than checking only the paths we expect
	// might change (which would miss an unexpected write elsewhere).
	before := snapshotTree(t, dir)

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var stdout, stderr bytes.Buffer
	opts := SyncCLIOptions{DryRun: true, UpdateProvenance: true, WriteDrafts: true}
	if err := runSync(dir, opts, now, &stdout, &stderr); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "(dry-run)") {
		t.Errorf("expected (dry-run) marker somewhere in output; stdout:\n%s", got)
	}

	after := snapshotTree(t, dir)
	if before != after {
		t.Errorf("dry-run must not touch disk.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// snapshotTree returns a stable, sorted "path\tsize\tmtime" listing for
// every regular file under root, used to assert a dry-run left the tree
// byte-for-byte (and file-for-file) untouched.
func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s\t%d\t%s", rel, info.Size(), info.ModTime()))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
