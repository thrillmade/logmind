package skill

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAuditSkills_Empty: missing skills/ dir → empty result.
func TestAuditSkills_Empty(t *testing.T) {
	dir := t.TempDir()
	rows := AuditSkills(dir)
	if rows != nil {
		t.Errorf("expected nil; got %v", rows)
	}
}

// TestAuditSkills_OneSkill: single SKILL.md → one row with the
// expected name + byte size; classification = active when fresh.
func TestAuditSkills_OneSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claude", "skills", "foo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "---\nname: foo\ndescription: y\n---\n# Foo\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows := AuditSkills(dir)
	if len(rows) != 1 {
		t.Fatalf("rows = %d; want 1", len(rows))
	}
	if rows[0].Name != "foo" {
		t.Errorf("name = %q; want foo", rows[0].Name)
	}
	if rows[0].Bytes != len(body) {
		t.Errorf("bytes = %d; want %d", rows[0].Bytes, len(body))
	}
	// Classify against "today" (file mtime is now) — should be active.
	if got := Classify(rows[0], time.Now()); got != "active" {
		t.Errorf("classify = %q; want active", got)
	}
}

// TestClassify_Ghost: 0 decisions + over-tight bytes → ghost.
func TestClassify_Ghost(t *testing.T) {
	row := AuditRow{
		Name:          "ghost",
		Bytes:         5000, // > auditTightCap (2000)
		DecisionCount: 0,
		LastModified:  "2026-06-02",
	}
	if got := Classify(row, time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)); got != "ghost" {
		t.Errorf("Classify ghost = %q; want ghost", got)
	}
}

// TestClassify_Aging: last_modified > 90 days ago → aging.
func TestClassify_Aging(t *testing.T) {
	row := AuditRow{
		Name:          "old",
		Bytes:         100,
		DecisionCount: 5,
		LastModified:  "2025-01-01",
	}
	if got := Classify(row, time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)); got != "aging" {
		t.Errorf("Classify aging = %q; want aging", got)
	}
}

// TestClassify_Active: small + recent + cited → active.
func TestClassify_Active(t *testing.T) {
	row := AuditRow{
		Name:          "ok",
		Bytes:         100,
		DecisionCount: 5,
		LastModified:  "2026-06-01",
	}
	if got := Classify(row, time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)); got != "active" {
		t.Errorf("Classify active = %q; want active", got)
	}
}

// TestAuditSkills_DecisionCount: ensure name occurrences in
// decisions.md flow through to the count.
func TestAuditSkills_DecisionCount(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claude", "skills", "kept")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: kept\ndescription: y\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	body := "## 2026-05-15 - intro\nWe kept the kept skill alive.\n\n## 2026-05-20 - again\nkept is good.\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write decisions: %v", err)
	}
	rows := AuditSkills(dir)
	if len(rows) != 1 {
		t.Fatalf("rows = %d; want 1", len(rows))
	}
	// "kept" appears 3 times as a whole word.
	want := 3
	if rows[0].DecisionCount != want {
		t.Errorf("DecisionCount = %d; want %d", rows[0].DecisionCount, want)
	}
}

// TestCountWholeWord pins the substring-vs-whole-word distinction so a
// regression to strings.Count doesn't slip past the parity comment.
// Per clud-bug PR #124 review.
func TestCountWholeWord(t *testing.T) {
	cases := []struct {
		Corpus string
		Name   string
		Want   int
	}{
		{"go to going", "go", 1},   // matches "go", not "going"
		{"api API APIs", "API", 1}, // matches "API" once, not "APIs"
		{"hello world hello", "hello", 2},
		{"clud-bug-collaboration cited; clud-bug-collaboration again", "clud-bug-collaboration", 2},
		{"", "anything", 0},
		{"anything", "", 0},
		{"no match here", "skill", 0},
	}
	for _, c := range cases {
		if got := countWholeWord(c.Corpus, c.Name); got != c.Want {
			t.Errorf("countWholeWord(%q, %q) = %d; want %d", c.Corpus, c.Name, got, c.Want)
		}
	}
}
