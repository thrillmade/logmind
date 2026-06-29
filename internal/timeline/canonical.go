// canonical.go — primitives for the main-canonical ("newspaper") timeline.
//
// These are the building blocks for SPEC §1.6.3's HTML-marker entry-block
// format and the §1.6.4 deterministic-union timeline (Slice 2). This file
// is PRIMITIVES ONLY — nothing here is wired into timeline generation yet;
// the default branch-divergent path (Generate/Render in timeline.go) is
// untouched, so output stays byte-identical until a later PR opts in via
// config. See docs/plan / the protocol SPEC §1.6.3.
package timeline

import (
	"fmt"
	"io"
	"strings"
)

// Entry-block markers (SPEC §1.6.3.1). Each marker occupies its own line,
// starting at column 0. The opening marker carries the `<date>-<slug>`
// identity; the closing marker carries only the terminator.
const (
	entryStartPrefix = "<!-- logmind-entry-start: "
	entryStartSuffix = " -->"
	entryEndMarker   = "<!-- logmind-entry-end -->"
)

// slugMaxBytes is the §1.6.3.1 truncation bound. The slug is pure
// [a-z0-9-] after step 2, so a byte bound is also a rune bound; we still
// truncate on a rune boundary to honor the spec's "UTF-8 safe" note.
const slugMaxBytes = 60

// Slugify derives a kebab-case slug from an entry title per SPEC §1.6.3.1:
//
//  1. lowercase
//  2. replace any run of chars not in [a-z0-9] with a single "-"
//  3. strip leading/trailing "-"
//  4. truncate to 60 bytes (UTF-8 safe — never split a codepoint)
//
// Steps are applied in order; per the spec, truncation (4) happens after
// trimming (3), so a truncated slug MAY end in "-".
func Slugify(title string) string {
	var b strings.Builder
	b.Grow(len(title))
	prevDash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > slugMaxBytes {
		// slug is ASCII here, but truncate on a rune boundary defensively.
		slug = truncateRunes(slug, slugMaxBytes)
	}
	return slug
}

// truncateRunes returns s truncated to at most maxBytes, never splitting a
// multi-byte rune.
func truncateRunes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	end := maxBytes
	// Back off to a rune boundary (a continuation byte is 0b10xxxxxx).
	for end > 0 && (s[end]&0xC0) == 0x80 {
		end--
	}
	return s[:end]
}

// entryBlock is one parsed §1.6.3 entry-block: its `<date>-<slug>` key and
// the verbatim body lines between (exclusive of) the markers.
type entryBlock struct {
	Key  string
	Body string
}

// HasEntryBlocks reports whether content is in entry-block format, per the
// §1.6.3.3 detection rule: any line beginning with
// `<!-- logmind-entry-start: ` within the first 200 lines indicates
// entry-block format; otherwise the file is brief mode.
func HasEntryBlocks(content string) bool {
	lines := strings.SplitN(content, "\n", 201)
	limit := len(lines)
	if limit > 200 {
		limit = 200
	}
	for _, line := range lines[:limit] {
		if strings.HasPrefix(line, entryStartPrefix) {
			return true
		}
	}
	return false
}

// parseStartMarker returns the `<date>-<slug>` key if line is a well-formed
// opening marker at column 0, else ok=false.
func parseStartMarker(line string) (key string, ok bool) {
	if !strings.HasPrefix(line, entryStartPrefix) {
		return "", false
	}
	// Allow trailing whitespace after the suffix but require the suffix.
	trimmed := strings.TrimRight(line, " \t\r")
	if !strings.HasSuffix(trimmed, entryStartSuffix) {
		return "", false
	}
	key = strings.TrimSpace(trimmed[len(entryStartPrefix) : len(trimmed)-len(entryStartSuffix)])
	if key == "" {
		return "", false
	}
	return key, true
}

// isEndMarker reports whether line is the closing marker (at column 0,
// trailing whitespace tolerated).
func isEndMarker(line string) bool {
	return strings.TrimRight(line, " \t\r") == entryEndMarker
}

// extractEntryBlocks scans content for matched §1.6.3 entry-blocks and
// returns them in document order. Per §1.6.3.3 markers are consumed as a
// stack: each opening marker opens a frame, the next closing marker closes
// it. Bodies MUST NOT nest (§1.6.3.1); an opening marker encountered before
// the current frame closes, or an unclosed frame at EOF, is malformed — the
// offending block is skipped and a diagnostic is written to stderr (mirrors
// decisions.Iter's tolerate-and-warn posture). It never panics.
func extractEntryBlocks(content string, stderr io.Writer) []entryBlock {
	var out []entryBlock
	lines := strings.Split(content, "\n")
	i := 0
	for i < len(lines) {
		key, ok := parseStartMarker(lines[i])
		if !ok {
			i++
			continue
		}
		// Find the matching close, rejecting a nested open first.
		j := i + 1
		nested := false
		for j < len(lines) && !isEndMarker(lines[j]) {
			if _, isStart := parseStartMarker(lines[j]); isStart {
				nested = true
				break
			}
			j++
		}
		switch {
		case nested:
			fmt.Fprintf(stderr, "logmind: skipping entry-block %q: nested opening marker before close (§1.6.3.1)\n", key)
			i++ // resume at the nested opener
		case j >= len(lines):
			fmt.Fprintf(stderr, "logmind: skipping entry-block %q: unclosed at end of file\n", key)
			i++
		default:
			out = append(out, entryBlock{Key: key, Body: strings.Join(lines[i+1:j], "\n")})
			i = j + 1
		}
	}
	return out
}
