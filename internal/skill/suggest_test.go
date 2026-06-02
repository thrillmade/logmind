package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKebabSlug pins the slug normalisation against Python's
// _kebab_slug. PascalCase → pascal-case, etc.
func TestKebabSlug(t *testing.T) {
	cases := []struct {
		In   string
		Want string
	}{
		{"api-versioning", "api-versioning"},
		{"PostgreSQL", "postgre-sql"},
		// Python's _kebab_slug result on "PostgreSQL":
		//   step 1 (lower→upper boundary inserts '-'): "Postgre-SQL"
		//   step 2 (lowercase + collapse '-'): "postgre-sql"
		// Note: Python uses (?<=[a-z])(?=[A-Z]) — so the rule is
		// "previous is lowercase AND next is uppercase".
		{"JWT", "jwt"},
		{"snake_case_name", "snake-case-name"},
		{"PascalCaseClass", "pascal-case-class"},
	}
	for _, c := range cases {
		if got := kebabSlug(c.In); got != c.Want {
			t.Errorf("kebabSlug(%q) = %q; want %q", c.In, got, c.Want)
		}
	}
}

// TestSuggestFromDecisions_NoDocs returns nil cleanly.
func TestSuggestFromDecisions_NoDocs(t *testing.T) {
	dir := t.TempDir()
	out := SuggestFromDecisions(dir, 30, 3, 5, time.Now())
	if out != nil {
		t.Errorf("expected nil; got %v", out)
	}
}

// TestSuggestFromDecisions_FindsPattern: three dated entries citing
// the same token should surface one suggestion.
func TestSuggestFromDecisions_FindsPattern(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	body := "# Decisions\n\n" +
		"## 2026-05-15 - first\n\n**Date**: 2026-05-15\n\n" +
		"We chose postgres-database for cache.\n\n" +
		"## 2026-05-20 - second\n\n**Date**: 2026-05-20\n\n" +
		"More on postgres-database here.\n\n" +
		"## 2026-05-25 - third\n\n**Date**: 2026-05-25\n\n" +
		"postgres-database again.\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Today set so the 30-day window catches all three entries.
	out := SuggestFromDecisions(dir, 30, 3, 5, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if len(out) != 1 {
		t.Fatalf("suggestions = %d; want 1; got %+v", len(out), out)
	}
	if out[0].Slug != "postgres-database" {
		t.Errorf("slug = %q; want postgres-database", out[0].Slug)
	}
	if out[0].DecisionCount != 3 {
		t.Errorf("DecisionCount = %d; want 3", out[0].DecisionCount)
	}
}

// TestSuggestFromDecisions_ExcludesExisting: if a skill of that slug
// already exists, the pattern is dropped.
func TestSuggestFromDecisions_ExcludesExisting(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	body := "## 2026-05-15 - x\n\n**Date**: 2026-05-15\n\nfoo-bar\n\n" +
		"## 2026-05-16 - x\n\n**Date**: 2026-05-16\n\nfoo-bar again\n\n" +
		"## 2026-05-17 - x\n\n**Date**: 2026-05-17\n\nfoo-bar thrice\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	skillDir := filepath.Join(dir, ".claude", "skills", "foo-bar")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: foo-bar\ndescription: y\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	out := SuggestFromDecisions(dir, 30, 3, 5, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	for _, s := range out {
		if s.Slug == "foo-bar" {
			t.Errorf("expected existing skill foo-bar to be excluded; got %+v", s)
		}
	}
}

// TestFormatIssueDraft pins the markdown shape against the Python
// reference. The text is load-bearing for the human-paste workflow.
func TestFormatIssueDraft(t *testing.T) {
	s := Suggestion{
		Phrase:        "api-versioning",
		Slug:          "api-versioning",
		DecisionCount: 3,
		Evidence: []SuggestEvidence{
			{File: "decisions.md", Snippet: "uses api-versioning"},
		},
		DraftDescription: "When working on api-versioning, follow consistent conventions across the codebase. (TODO: replace with concrete trigger + steps.)",
	}
	got := FormatIssueDraft(s)
	if !strings.HasPrefix(got, "## New skill proposal: api-versioning\n") {
		t.Errorf("missing heading; got prefix:\n%q", got[:80])
	}
	if !strings.Contains(got, "`api-versioning`") {
		t.Errorf("missing slug code-span")
	}
	if !strings.Contains(got, "- `decisions.md`: uses api-versioning") {
		t.Errorf("missing evidence line; got:\n%s", got)
	}
	if !strings.HasSuffix(got, "review evidence and refine before opening._") {
		t.Errorf("missing trailing footer; got tail:\n%s", got[len(got)-80:])
	}
}

// TestExcerptAround handles boundary cases. Cross-checked against
// Python's _excerpt_around output to lock parity at edge offsets.
func TestExcerptAround(t *testing.T) {
	if got := excerptAround("hello world", 0, 6); got != "hel…" {
		// idx=0, width=6 → start=0, end=3, no leading "…" (start==0),
		// trailing "…" because end < len. Same as Python.
		t.Errorf("excerptAround(0,6) = %q; want 'hel…'", got)
	}
	if got := excerptAround("hello world", 5, 10); got != "hello worl…" {
		// idx=5, width=10 → start=0, end=10 → no leading "…", trailing "…".
		t.Errorf("excerptAround(5,10) = %q; want 'hello worl…'", got)
	}
}
