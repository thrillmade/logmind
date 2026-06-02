package decisions

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIterSimple(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.md")
	body := "# Decision Log\n\n" +
		"---\n" +
		"## 2026-06-01 10:30 - First decision\n" +
		"\n" +
		"Some body text.\n" +
		"\n" +
		"## 2026-06-02 14:45 - Second decision with - dashes in title\n" +
		"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	entries, err := Iter(path, &stderr)
	if err != nil {
		t.Fatalf("Iter err: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries; want 2", len(entries))
	}
	if entries[0].Title != "First decision" {
		t.Errorf("entry[0].Title = %q", entries[0].Title)
	}
	if entries[1].Title != "Second decision with - dashes in title" {
		t.Errorf("entry[1].Title = %q", entries[1].Title)
	}
	want0 := time.Date(2026, 6, 1, 10, 30, 0, 0, time.UTC)
	if !entries[0].Date.Equal(want0) {
		t.Errorf("entry[0].Date = %v; want %v", entries[0].Date, want0)
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr output: %q", stderr.String())
	}
}

func TestIterMissingFile(t *testing.T) {
	entries, err := Iter("/nonexistent/path.md", nil)
	if err != nil {
		t.Errorf("missing file should not error; got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("missing file returned %d entries", len(entries))
	}
}

func TestIterMalformedHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.md")
	body := "## 2026-13-45 25:99 - Bad date\n" +
		"## 2026-06-01 10:30 - Good entry\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	entries, err := Iter(path, &stderr)
	if err != nil {
		t.Fatalf("Iter err: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries; want 1 (malformed skipped)", len(entries))
	}
	if !strings.Contains(stderr.String(), "skipping malformed decision header") {
		t.Errorf("expected warning on stderr; got %q", stderr.String())
	}
}

func TestCollectAllSources(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "decisions-branches"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "decisions.md"),
		[]byte("## 2026-06-01 10:00 - main entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "decisions-archive.md"),
		[]byte("## 2026-05-01 09:00 - archived entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "decisions-branches", "feat__alpha.md"),
		[]byte("## 2026-06-02 11:00 - alpha branch entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	entries, err := Collect(docs, &stderr)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d; want 3", len(entries))
	}
	// Newest-first
	if entries[0].SourceLabel != "feat/alpha" {
		t.Errorf("entries[0].SourceLabel = %q; want feat/alpha", entries[0].SourceLabel)
	}
	if entries[0].SourcePath != "decisions-branches/feat__alpha.md" {
		t.Errorf("entries[0].SourcePath = %q", entries[0].SourcePath)
	}
	if entries[1].SourceLabel != "main" {
		t.Errorf("entries[1].SourceLabel = %q; want main", entries[1].SourceLabel)
	}
	if entries[2].SourceLabel != "archive" {
		t.Errorf("entries[2].SourceLabel = %q; want archive", entries[2].SourceLabel)
	}
}

func TestBranchLabelFromFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"feat__alpha.md", "feat/alpha"},
		{"chore__clud-bug-v0.6.30.md", "chore/clud-bug-v0.6.30"},
		{"virtual-kurzweil.md", "virtual-kurzweil"},
		{"foo__bar__baz.md", "foo/bar/baz"},
	}
	for _, c := range cases {
		got := branchLabelFromFilename(c.in)
		if got != c.want {
			t.Errorf("branchLabelFromFilename(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}
