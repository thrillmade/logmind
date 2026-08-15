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
	// A legacy docs/decisions-archive.md MUST be collected. §3.2 stopped
	// WRITING the archive; it did not make the decisions already in one stop
	// counting. A repo that rotated under the retired `max_recent: 20`
	// default would otherwise lose them all on upgrade.
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
		t.Fatalf("got %d; want 3 (the branch file + the legacy main log + the legacy archive)", len(entries))
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
	if entries[2].SourcePath != "decisions-archive.md" {
		t.Errorf("entries[2].SourcePath = %q; want decisions-archive.md", entries[2].SourcePath)
	}
	if !strings.Contains(entries[2].Title, "archived entry") {
		t.Errorf("entries[2].Title = %q; want the archived entry's title", entries[2].Title)
	}
}

// TestSplitRawBytes_PreambleAndBoundaries: the header/preface text before
// the first entry becomes `preamble`; each entry's Raw spans exactly its
// header line through the byte before the next header (or EOF for the
// last), reconstructing the original content when concatenated back
// together.
func TestSplitRawBytes_PreambleAndBoundaries(t *testing.T) {
	content := "# Decision Log\n\n" +
		"This file contains the 20 most recent decisions.\n\n" +
		"---\n" +
		"## 2026-06-01 10:00 - First\n\n" +
		"**Reasoning:** why one\n\n" +
		"---\n\n" +
		"## 2026-06-02 11:00 - Second\n\n" +
		"**Reasoning:** why two\n\n" +
		"---\n\n"

	preamble, entries := SplitRawBytes(content)
	wantPreamble := "# Decision Log\n\n" +
		"This file contains the 20 most recent decisions.\n\n" +
		"---\n"
	if preamble != wantPreamble {
		t.Fatalf("preamble = %q; want %q", preamble, wantPreamble)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries; want 2", len(entries))
	}
	if entries[0].Title != "First" || entries[1].Title != "Second" {
		t.Fatalf("titles = %q, %q", entries[0].Title, entries[1].Title)
	}
	wantEntry0 := "## 2026-06-01 10:00 - First\n\n" +
		"**Reasoning:** why one\n\n" +
		"---\n\n"
	if entries[0].Raw != wantEntry0 {
		t.Fatalf("entries[0].Raw = %q; want %q", entries[0].Raw, wantEntry0)
	}
	wantEntry1 := "## 2026-06-02 11:00 - Second\n\n" +
		"**Reasoning:** why two\n\n" +
		"---\n\n"
	if entries[1].Raw != wantEntry1 {
		t.Fatalf("entries[1].Raw = %q; want %q", entries[1].Raw, wantEntry1)
	}
	// Reassembly invariant: preamble + every entry's Raw, in order,
	// reconstructs the original content exactly.
	rebuilt := preamble
	for _, e := range entries {
		rebuilt += e.Raw
	}
	if rebuilt != content {
		t.Fatalf("preamble+entries != original content:\ngot:  %q\nwant: %q", rebuilt, content)
	}
}

// TestSplitRawBytes_NoEntries_WholeContentIsPreamble: a file with no
// decision headers at all (e.g. a freshly-scaffolded, empty decisions.md)
// returns the whole content as preamble and a nil entries slice.
func TestSplitRawBytes_NoEntries_WholeContentIsPreamble(t *testing.T) {
	content := "# Decision Log\n\nNo entries yet.\n\n---\n"
	preamble, entries := SplitRawBytes(content)
	if preamble != content {
		t.Fatalf("preamble = %q; want whole content %q", preamble, content)
	}
	if entries != nil {
		t.Fatalf("entries = %v; want nil", entries)
	}
}

// TestSplitRawBytes_IgnoresEntryBlockMarker: the §1.6.3 HTML-comment marker
// block that opens a branch decision file (`<!-- logmind-entry-start: ...
// -->`) must never be mistaken for a decision entry — decisionHeader only
// matches a literal "## " line prefix, so the marker rides along inside
// whatever text precedes the first real "## " header.
func TestSplitRawBytes_IgnoresEntryBlockMarker(t *testing.T) {
	content := "<!-- logmind-entry-start: 2026-06-01-my-branch -->\n" +
		"- **2026-06-01** — My branch\n" +
		"<!-- logmind-entry-end -->\n\n" +
		"## 2026-06-01 10:00 - First decision\n\n" +
		"**Reasoning:** why\n\n" +
		"---\n\n"
	preamble, entries := SplitRawBytes(content)
	if len(entries) != 1 {
		t.Fatalf("got %d entries; want 1 (marker block must not count)", len(entries))
	}
	if strings.Contains(preamble, "First decision") {
		t.Fatalf("preamble leaked into the decision entry: %q", preamble)
	}
	if !strings.Contains(preamble, "logmind-entry-start") {
		t.Fatalf("marker block should live in preamble; got %q", preamble)
	}
	if entries[0].Title != "First decision" {
		t.Fatalf("entries[0].Title = %q", entries[0].Title)
	}
}

// TestSplitRaw_MissingFile: matches Iter's "no file, no entries, no error"
// contract.
func TestSplitRaw_MissingFile(t *testing.T) {
	preamble, entries, err := SplitRaw("/nonexistent/path.md")
	if err != nil {
		t.Fatalf("missing file should not error; got %v", err)
	}
	if preamble != "" || entries != nil {
		t.Fatalf("got (%q, %v); want (\"\", nil)", preamble, entries)
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
		got := BranchLabelFromFilename(c.in)
		if got != c.want {
			t.Errorf("BranchLabelFromFilename(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}
