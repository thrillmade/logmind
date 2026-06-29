package timeline

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"Add JWT session auth", "add-jwt-session-auth"},
		{"Use Redis for caching!", "use-redis-for-caching"},
		{"  Leading / trailing  ", "leading-trailing"},
		{"C++ & Go", "c-go"},
		{"already-kebab-case", "already-kebab-case"},
		{"Multiple   spaces\tand\ntabs", "multiple-spaces-and-tabs"},
		{"café résumé", "caf-r-sum"}, // non-ASCII runs collapse to a single '-'
		{"", ""},
		{"!!!", ""}, // all punctuation → empty after trim
		{"v1.2.0 release", "v1-2-0-release"},
	}
	for _, c := range cases {
		if got := Slugify(c.title); got != c.want {
			t.Errorf("Slugify(%q) = %q; want %q", c.title, got, c.want)
		}
	}
}

func TestSlugify_TruncatesTo60Bytes(t *testing.T) {
	// Step 4: truncate to 60 bytes. Input is pure ASCII so 60 bytes = 60 chars.
	got := Slugify(strings.Repeat("a", 70))
	if len(got) != 60 {
		t.Errorf("len = %d; want 60", len(got))
	}
	// A hyphen-dense title must still truncate at ≤60 bytes.
	got2 := Slugify(strings.Repeat("ab cd ", 20)) // → "ab-cd-ab-cd-..."
	if len(got2) > 60 {
		t.Errorf("len = %d; want ≤60", len(got2))
	}
}

func TestSlugify_NeverSplitsCodepoint(t *testing.T) {
	// Defensive: truncateRunes must not split a multi-byte rune. Build a
	// raw string whose byte-60 boundary lands mid-rune and feed it directly
	// to the truncator (Slugify itself ASCII-izes before truncating).
	s := strings.Repeat("a", 59) + "é" // 'é' = 2 bytes; byte index 60 is a continuation byte
	got := truncateRunes(s, 60)
	if !utf8.ValidString(got) {
		t.Errorf("truncateRunes split a codepoint: %q", got)
	}
	if len(got) != 59 { // the 2-byte rune is dropped whole, not split
		t.Errorf("len = %d; want 59 (dropped the split rune whole)", len(got))
	}
}

func TestHasEntryBlocks(t *testing.T) {
	withMarker := "# Timeline\n\n" + entryStartPrefix + "2026-06-29-x" + entryStartSuffix + "\nbody\n" + entryEndMarker + "\n"
	if !HasEntryBlocks(withMarker) {
		t.Errorf("HasEntryBlocks = false; want true for entry-block content")
	}
	brief := "# Timeline\n\n## 2026-06\n\n- **2026-06-29** — something\n"
	if HasEntryBlocks(brief) {
		t.Errorf("HasEntryBlocks = true; want false for brief-mode content")
	}
	// A marker only after line 200 must NOT be detected (§1.6.3.3 scans the
	// first 200 lines).
	deep := strings.Repeat("filler\n", 250) + entryStartPrefix + "2026-06-29-x" + entryStartSuffix + "\n"
	if HasEntryBlocks(deep) {
		t.Errorf("HasEntryBlocks = true; want false for a marker past line 200")
	}
}

func block(key, body string) string {
	return entryStartPrefix + key + entryStartSuffix + "\n" + body + "\n" + entryEndMarker + "\n"
}

func TestExtractEntryBlocks_WellFormed(t *testing.T) {
	content := "# Timeline\n\n" +
		block("2026-06-29-add-jwt", "- **2026-06-29** — Add JWT") +
		"\n## 2026-05\n\n" +
		block("2026-05-01-init", "- **2026-05-01** — Init")
	var stderr bytes.Buffer
	got := extractEntryBlocks(content, &stderr)
	if len(got) != 2 {
		t.Fatalf("got %d blocks; want 2 (%s)", len(got), stderr.String())
	}
	if got[0].Key != "2026-06-29-add-jwt" || got[1].Key != "2026-05-01-init" {
		t.Errorf("keys = %q, %q", got[0].Key, got[1].Key)
	}
	if got[0].Body != "- **2026-06-29** — Add JWT" {
		t.Errorf("body[0] = %q (markers must be excluded, body verbatim)", got[0].Body)
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected stderr on well-formed input: %s", stderr.String())
	}
}

func TestExtractEntryBlocks_UnclosedSkippedLoudly(t *testing.T) {
	content := entryStartPrefix + "2026-06-29-x" + entryStartSuffix + "\nbody never closes\n"
	var stderr bytes.Buffer
	got := extractEntryBlocks(content, &stderr)
	if len(got) != 0 {
		t.Errorf("got %d; want 0 (unclosed must be skipped)", len(got))
	}
	if !strings.Contains(stderr.String(), "unclosed") {
		t.Errorf("expected an 'unclosed' diagnostic on stderr; got %q", stderr.String())
	}
}

func TestExtractEntryBlocks_NestedSkippedLoudly(t *testing.T) {
	content := entryStartPrefix + "2026-06-29-outer" + entryStartSuffix + "\n" +
		entryStartPrefix + "2026-06-29-inner" + entryStartSuffix + "\n" +
		"body\n" + entryEndMarker + "\n"
	var stderr bytes.Buffer
	got := extractEntryBlocks(content, &stderr)
	// The outer is malformed (nested open); the inner is well-formed and recovered.
	if len(got) != 1 || got[0].Key != "2026-06-29-inner" {
		t.Errorf("got %+v; want just the recovered inner block", got)
	}
	if !strings.Contains(stderr.String(), "nested") {
		t.Errorf("expected a 'nested' diagnostic; got %q", stderr.String())
	}
}

func TestExtractEntryBlocks_NoMarkers(t *testing.T) {
	var stderr bytes.Buffer
	got := extractEntryBlocks("# Timeline\n\njust prose, no markers\n", &stderr)
	if len(got) != 0 {
		t.Errorf("got %d; want 0", len(got))
	}
}
