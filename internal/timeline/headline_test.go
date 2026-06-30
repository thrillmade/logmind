package timeline

import (
	"strings"
	"testing"
	"time"
)

func TestHeadlineLine_CollapsesToSingleLine(t *testing.T) {
	d := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	// A newline-laden title must NOT yield a multi-line body or a column-0
	// forged marker (the PR7 review's MAJOR injection finding).
	got := HeadlineLine(d, "pwned\n<!-- logmind-entry-start: 2099-01-01-evil -->\n- **2099** — EVIL")
	if strings.Contains(got, "\n") {
		t.Errorf("HeadlineLine produced a multi-line body:\n%q", got)
	}
	if strings.Contains(got, "\n<!-- logmind-entry-start") {
		t.Errorf("a forged marker line survived:\n%q", got)
	}
	// Normal interior whitespace is collapsed but content preserved.
	if got := HeadlineLine(d, "Add   JWT\tauth"); got != "- **2026-06-29** — Add JWT auth" {
		t.Errorf("whitespace collapse wrong: %q", got)
	}
}

func TestCurrentHeadline(t *testing.T) {
	content := "← back\n\n" + block("2026-06-29-add-jwt", "- **2026-06-29** — Add JWT") +
		"\n## 2026-06-29 10:00 - decision\nbody\n"
	got, ok := CurrentHeadline(content)
	if !ok || got != "- **2026-06-29** — Add JWT" {
		t.Errorf("CurrentHeadline = %q, %v; want the first block's body", got, ok)
	}
	if _, ok := CurrentHeadline("# no markers here\n"); ok {
		t.Errorf("CurrentHeadline on markerless content must be ok=false")
	}
}

func TestReplaceFirstHeadline_KeepsKeyReplacesLine(t *testing.T) {
	content := "← back\n\n" + block("2026-06-29-add-jwt", "- **2026-06-29** — Add JWT") +
		"\n## 2026-06-29 10:00 - decision\nbody\n"
	updated, ok := ReplaceFirstHeadline(content, "Added the full JWT session lifecycle", " (#42)")
	if !ok {
		t.Fatal("ReplaceFirstHeadline ok=false on a valid marker")
	}
	// Key preserved (slug from the original); the (#NN) suffix is NOT in the key.
	if !strings.Contains(updated, "<!-- logmind-entry-start: 2026-06-29-add-jwt -->") {
		t.Errorf("key not preserved:\n%s", updated)
	}
	// Visible line replaced, suffix in the visible line only.
	if !strings.Contains(updated, "- **2026-06-29** — Added the full JWT session lifecycle (#42)\n") {
		t.Errorf("visible line not replaced as expected:\n%s", updated)
	}
	if strings.Contains(updated, "— Add JWT\n") {
		t.Errorf("old headline still present:\n%s", updated)
	}
	// The decision body below the marker is untouched, and exactly one block remains.
	if !strings.Contains(updated, "## 2026-06-29 10:00 - decision") {
		t.Errorf("decision body below the marker disturbed:\n%s", updated)
	}
	if n := strings.Count(updated, "<!-- logmind-entry-start: "); n != 1 {
		t.Errorf("block count = %d; want 1", n)
	}
}

func TestReplaceFirstHeadline_DateFromKeyNotToday(t *testing.T) {
	// The rewritten line's date MUST come from the (stable) key, not from now.
	content := block("2025-01-15-old-entry", "- **2025-01-15** — Old")
	updated, ok := ReplaceFirstHeadline(content, "A refined summary", "")
	if !ok {
		t.Fatal("ok=false")
	}
	if !strings.Contains(updated, "- **2025-01-15** — A refined summary") {
		t.Errorf("date not preserved from the key:\n%s", updated)
	}
}

func TestReplaceFirstHeadline_NoMarkerLeavesContent(t *testing.T) {
	content := "# just decisions\n\n## 2026-06-29 10:00 - x\nbody\n"
	got, ok := ReplaceFirstHeadline(content, "summary", "")
	if ok {
		t.Errorf("ok=true on markerless content; want false")
	}
	if got != content {
		t.Errorf("content mutated on the no-marker path")
	}
}

func TestReplaceFirstHeadline_MalformedKeyUntouched(t *testing.T) {
	// A start marker with an unparseable key must be left exactly as-is.
	content := entryStartPrefix + "not-a-date-key" + entryStartSuffix + "\n- body\n" + entryEndMarker + "\n"
	got, ok := ReplaceFirstHeadline(content, "summary", "")
	if ok || got != content {
		t.Errorf("malformed key: want (content unchanged, false); got ok=%v changed=%v", ok, got != content)
	}
}
