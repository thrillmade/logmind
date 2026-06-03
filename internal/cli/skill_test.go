package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thrillmade/logmind/internal/skill"
)

// TestSkillNew_HappyPath: scaffold + decision-log-skipped (no docs/).
// Stdout is byte-identical to the Python reference verified against
// the running build (see PR description for the diff transcript).
func TestSkillNew_HappyPath(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runSkillNew(dir, "sample", "A test skill", true, true, &stdout); err != nil {
		t.Fatalf("runSkillNew: %v", err)
	}
	got := stdout.String()
	// Replace the absolute temp path with a marker so the golden file
	// is platform-independent.
	got = strings.ReplaceAll(got, dir, "<TMP>")
	checkGolden(t, "skill_new_happy.golden", got)

	// SKILL.md exists + contains the description field.
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "sample", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(body), "description: A test skill") {
		t.Errorf("SKILL.md missing description; body:\n%s", body)
	}
}

// TestSkillNew_AlreadyExists: re-creating fails with "already exists".
func TestSkillNew_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	if _, err := skill.ScaffoldBasic(dir, "dup", "first"); err != nil {
		t.Fatalf("pre-scaffold: %v", err)
	}
	var stdout bytes.Buffer
	err := runSkillNew(dir, "dup", "second", true, true, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("expected ErrSilent; got %v", err)
	}
	got := strings.ReplaceAll(stdout.String(), dir, "<TMP>")
	checkGolden(t, "skill_new_exists.golden", got)
}

// TestSkillNew_WithDocs: docs/ present → emits the "decision-logged"
// confirmation line.
func TestSkillNew_WithDocs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	var stdout bytes.Buffer
	if err := runSkillNew(dir, "with-docs", "", false, true, &stdout); err != nil {
		t.Fatalf("runSkillNew: %v", err)
	}
	got := strings.ReplaceAll(stdout.String(), dir, "<TMP>")
	checkGolden(t, "skill_new_with_docs.golden", got)
}

// TestSkillTest_NotFound: missing skill → error + ErrSilent.
func TestSkillTest_NotFound(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	err := runSkillTest(dir, "ghost", &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("expected ErrSilent; got %v", err)
	}
	got := strings.ReplaceAll(stdout.String(), dir, "<TMP>")
	checkGolden(t, "skill_test_not_found.golden", got)
}

// TestSkillTest_Passes: well-formed SKILL.md → both checks pass +
// "ok skill: <name> validated".
func TestSkillTest_Passes(t *testing.T) {
	dir := t.TempDir()
	if _, err := skill.ScaffoldBasic(dir, "ok-skill", "Trigger one-liner"); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	var stdout bytes.Buffer
	if err := runSkillTest(dir, "ok-skill", &stdout); err != nil {
		t.Fatalf("runSkillTest: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "✓ frontmatter required fields: ok") {
		t.Errorf("missing frontmatter pass; got:\n%s", got)
	}
	if !strings.Contains(got, "ok skill: ok-skill validated") {
		t.Errorf("missing validated trailer; got:\n%s", got)
	}
}

// TestSkillTest_Fails: hand-crafted SKILL.md missing description →
// frontmatter check fails + "FAILED validation" trailer.
func TestSkillTest_Fails(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claude", "skills", "bad")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "---\nname: bad\n---\n# Bad\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	err := runSkillTest(dir, "bad", &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("expected ErrSilent; got %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "missing required field: description") {
		t.Errorf("missing frontmatter failure; got:\n%s", got)
	}
	if !strings.Contains(got, "ok skill: bad FAILED validation") {
		t.Errorf("missing FAILED trailer; got:\n%s", got)
	}
}

// TestSkillBench_Tight: scaffolded skill is small enough for "tight".
// Output is byte-identical to Python (manually compared).
func TestSkillBench_Tight(t *testing.T) {
	dir := t.TempDir()
	if _, err := skill.ScaffoldBasic(dir, "bench-target", "Trigger one-liner"); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	var stdout bytes.Buffer
	if err := runSkillBench(dir, "bench-target", false, &stdout); err != nil {
		t.Fatalf("runSkillBench: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "— tight") {
		t.Errorf("expected 'tight' status; got:\n%s", got)
	}
	if !strings.Contains(got, "ok skill: bench bench-target tight") {
		t.Errorf("missing ok trailer; got:\n%s", got)
	}
}

// TestSkillBench_JSON: --json emits a parseable BenchResult.
func TestSkillBench_JSON(t *testing.T) {
	dir := t.TempDir()
	if _, err := skill.ScaffoldBasic(dir, "json-skill", "Trigger"); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	var stdout bytes.Buffer
	if err := runSkillBench(dir, "json-skill", true, &stdout); err != nil {
		t.Fatalf("runSkillBench: %v", err)
	}
	var parsed skill.BenchResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &parsed); err != nil {
		t.Fatalf("parse json: %v\nstdout:\n%s", err, stdout.String())
	}
	if parsed.Status == "" {
		t.Errorf("status missing; got %+v", parsed)
	}
	if len(parsed.Sections) == 0 {
		t.Errorf("sections missing; got %+v", parsed)
	}
}

// TestSkillAudit_Empty: bare repo → empty message + ok line.
func TestSkillAudit_Empty(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runSkillAudit(dir, time.Time{}, false, &stdout); err != nil {
		t.Fatalf("runSkillAudit: %v", err)
	}
	checkGolden(t, "skill_audit_empty.golden", stdout.String())
}

// TestSkillAudit_OneSkill: deterministic now → "active" row.
func TestSkillAudit_OneSkill(t *testing.T) {
	dir := t.TempDir()
	if _, err := skill.ScaffoldBasic(dir, "alpha", "Trigger"); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	// Touch the SKILL.md mtime to a known LOCAL noon so the date
	// rendering doesn't depend on machine timezone. Python's
	// date.fromtimestamp() and Go's time.ModTime().Format() both honor
	// the local clock; using local noon keeps the date the same as the
	// stamp's wall date everywhere.
	stamp := time.Date(2026, 6, 2, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(filepath.Join(dir, ".claude", "skills", "alpha", "SKILL.md"), stamp, stamp); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	var stdout bytes.Buffer
	if err := runSkillAudit(dir, stamp, false, &stdout); err != nil {
		t.Fatalf("runSkillAudit: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "alpha") {
		t.Errorf("missing alpha row; got:\n%s", got)
	}
	if !strings.Contains(got, "active") {
		t.Errorf("expected active status; got:\n%s", got)
	}
	if !strings.Contains(got, "2026-06-02") {
		t.Errorf("expected 2026-06-02 date; got:\n%s", got)
	}
	if !strings.Contains(got, "ok skill: audit 1 skill (1 active)") {
		t.Errorf("expected singular trailer; got:\n%s", got)
	}
}

// TestSkillAudit_JSON: --json contains status alongside the
// AuditRow fields.
func TestSkillAudit_JSON(t *testing.T) {
	dir := t.TempDir()
	if _, err := skill.ScaffoldBasic(dir, "alpha", "Trigger"); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	stamp := time.Date(2026, 6, 2, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(filepath.Join(dir, ".claude", "skills", "alpha", "SKILL.md"), stamp, stamp); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	var stdout bytes.Buffer
	if err := runSkillAudit(dir, stamp, true, &stdout); err != nil {
		t.Fatalf("runSkillAudit: %v", err)
	}
	var rows []skill.AuditRow
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &rows); err != nil {
		t.Fatalf("parse json: %v\nstdout:\n%s", err, stdout.String())
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d; want 1", len(rows))
	}
	if rows[0].Status == "" {
		t.Errorf("status missing in JSON; got %+v", rows[0])
	}
}

// TestSkillSuggest_BadSince: malformed --since prints the canonical
// error message + ErrSilent.
func TestSkillSuggest_BadSince(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	err := runSkillSuggest(context.Background(), dir, "abc", 3, 5, "", false, true, time.Time{}, nil, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("expected ErrSilent; got %v", err)
	}
	if !strings.HasPrefix(stdout.String(), "Error: --since must be of the form Nd / Nw / Nm / Ny (got 'abc')") {
		t.Errorf("missing canonical error; got:\n%s", stdout.String())
	}
}

// TestSkillSuggest_NoDocs: empty repo (no docs/) → "no patterns" path.
func TestSkillSuggest_NoDocs(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runSkillSuggest(context.Background(), dir, "30d", 3, 5, "", false, true, time.Time{}, nil, &stdout); err != nil {
		t.Fatalf("runSkillSuggest: %v", err)
	}
	checkGolden(t, "skill_suggest_no_patterns.golden", stdout.String())
}

// TestSkillSuggest_FindsPattern: dated decisions citing a token
// surface as one suggestion in the canonical human-readable layout.
func TestSkillSuggest_FindsPattern(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	body := "## 2026-05-15 - x\n\n**Date**: 2026-05-15\n\npostgres-database here.\n\n" +
		"## 2026-05-20 - y\n\n**Date**: 2026-05-20\n\npostgres-database again.\n\n" +
		"## 2026-05-25 - z\n\n**Date**: 2026-05-25\n\npostgres-database thrice.\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := runSkillSuggest(context.Background(), dir, "30d", 3, 5, "", false, true, now, nil, &stdout); err != nil {
		t.Fatalf("runSkillSuggest: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Pattern A (cited in 3 decisions):") {
		t.Errorf("missing Pattern A; got:\n%s", got)
	}
	if !strings.Contains(got, "suggested-slug: postgres-database") {
		t.Errorf("missing slug; got:\n%s", got)
	}
	if !strings.Contains(got, "ok skill: suggest 1 pattern\n") {
		t.Errorf("missing singular ok line; got:\n%s", got)
	}
}

// TestSkillSuggest_LLMFallback: engine=llm + no API key + fallback
// enabled → heuristic runs, single notice line emitted, ok trailer.
func TestSkillSuggest_LLMFallback(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "## 2026-05-15 - x\n\n**Date**: 2026-05-15\n\nfoo-bar one\n\n" +
		"## 2026-05-20 - y\n\n**Date**: 2026-05-20\n\nfoo-bar two\n\n" +
		"## 2026-05-25 - z\n\n**Date**: 2026-05-25\n\nfoo-bar three\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// engine=llm via config; no API key in env → fallback to heuristic.
	if err := os.MkdirAll(filepath.Join(dir, ".logmind"), 0o755); err != nil {
		t.Fatalf("mkdir .logmind: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".logmind", "config.yml"),
		[]byte("skill_suggest:\n  engine: llm\n  anthropic_api_key_env: BOGUS_KEY_NEVER_SET\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	// Clear the env var just in case it leaked into the test env.
	t.Setenv("BOGUS_KEY_NEVER_SET", "")
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var stdout bytes.Buffer
	if err := runSkillSuggest(context.Background(), dir, "30d", 3, 5, "", false, false, now, nil, &stdout); err != nil {
		t.Fatalf("runSkillSuggest: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "engine=llm but no API key found — falling back to heuristic engine") {
		t.Errorf("missing fallback notice; got:\n%s", got)
	}
	if !strings.Contains(got, "Pattern A (cited in 3 decisions):") {
		t.Errorf("heuristic result missing from output; got:\n%s", got)
	}
}

// TestSkillSuggest_LLMTransport: a stub LLMSuggester overrides the
// heuristic re-rank. Confirms the cli wiring threads the transport
// through to skill.SuggestLLM.
func TestSkillSuggest_LLMTransport(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "## 2026-05-15 - x\n\n**Date**: 2026-05-15\n\npostgres-database one\n\n" +
		"## 2026-05-20 - y\n\n**Date**: 2026-05-20\n\npostgres-database two\n\n" +
		"## 2026-05-25 - z\n\n**Date**: 2026-05-25\n\npostgres-database three\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Pre-stage the engine=llm config so we don't fall back to
	// heuristic right away.
	if err := os.MkdirAll(filepath.Join(dir, ".logmind"), 0o755); err != nil {
		t.Fatalf("mkdir .logmind: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".logmind", "config.yml"),
		[]byte("skill_suggest:\n  engine: llm\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	transport := &fakeLLMTransport{
		Patterns: []skill.Suggestion{
			{
				Phrase:           "postgres-database",
				Slug:             "postgres-database",
				DecisionCount:    3,
				Evidence:         []skill.SuggestEvidence{{File: "decisions.md", Snippet: "stub snippet"}},
				DraftDescription: "When postgres-database shows up, load this skill.",
			},
		},
	}
	var stdout bytes.Buffer
	err := runSkillSuggest(context.Background(), dir, "30d", 3, 5, "", false, false, now, transport, &stdout)
	if err != nil {
		t.Fatalf("runSkillSuggest: %v", err)
	}
	if transport.Calls != 1 {
		t.Errorf("expected 1 LLM call; got %d", transport.Calls)
	}
	if !strings.Contains(stdout.String(), "Pattern A (cited in 3 decisions):") {
		t.Errorf("missing Pattern A in output; got:\n%s", stdout.String())
	}
}

// TestSkillSuggest_WriteDrafts: --write-drafts dir creates one file
// per suggestion + emits the trailing notice.
func TestSkillSuggest_WriteDrafts(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "## 2026-05-15 - x\n\n**Date**: 2026-05-15\n\nfoo-bar one\n\n" +
		"## 2026-05-20 - y\n\n**Date**: 2026-05-20\n\nfoo-bar two\n\n" +
		"## 2026-05-25 - z\n\n**Date**: 2026-05-25\n\nfoo-bar three\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	outDir := filepath.Join(dir, "drafts")
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var stdout bytes.Buffer
	if err := runSkillSuggest(context.Background(), dir, "30d", 3, 5, outDir, false, true, now, nil, &stdout); err != nil {
		t.Fatalf("runSkillSuggest: %v", err)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read drafts dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("draft files = %d; want 1", len(entries))
	}
	if entries[0].Name() != "skill-proposal-foo-bar.md" {
		t.Errorf("draft name = %q; want skill-proposal-foo-bar.md", entries[0].Name())
	}
	if !strings.Contains(stdout.String(), "Pre-filled drafts written to") {
		t.Errorf("missing drafts-written notice; got:\n%s", stdout.String())
	}
}

// fakeLLMTransport stubs the LLMSuggester interface for tests.
type fakeLLMTransport struct {
	Patterns []skill.Suggestion
	Calls    int
}

func (f *fakeLLMTransport) Suggest(ctx context.Context, in skill.LLMRequest) ([]skill.Suggestion, error) {
	f.Calls++
	if len(f.Patterns) == 0 {
		return nil, fmt.Errorf("fake: %w", skill.LLMUnavailableErr)
	}
	return f.Patterns, nil
}
