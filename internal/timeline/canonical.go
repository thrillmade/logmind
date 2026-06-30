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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thrillmade/logmind/internal/decisions"
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

// --- §1.6.4 main-canonical assembly ---------------------------------------
//
// The main-canonical timeline is a DETERMINISTIC UNION of §1.6.3 entry-block
// markers — no LLM, no re-summarization. Same source tree ⇒ same bytes on
// any branch, worktree, or checkout path. It is reachable ONLY when
// `timeline.canonical: main-canonical` is set; the default branch-divergent
// path (Generate/Render) is untouched, so v0.6.14 output is byte-stable.

// marked is one timeline row: its date+slug identity, the link-free headline
// body rendered between the entry-block markers, and its source path (the
// detail-page link target, AND the deterministic collision tiebreak — both
// relative to docs/, e.g. "decisions-branches/feat__x.md" or "decisions.md").
type marked struct {
	date   time.Time
	slug   string
	body   string
	source string
}

// splitKey parses a "<YYYY-MM-DD>-<slug>" entry-block key into its date and
// slug. Returns ok=false on a malformed key.
func splitKey(key string) (date time.Time, slug string, ok bool) {
	const dateLen = len("2006-01-02") // 10
	if len(key) < dateLen+2 || key[dateLen] != '-' {
		return time.Time{}, "", false
	}
	d, err := time.Parse("2006-01-02", key[:dateLen])
	if err != nil {
		return time.Time{}, "", false
	}
	slug = key[dateLen+1:]
	if slug == "" {
		return time.Time{}, "", false
	}
	return d, slug, true
}

// HeadlineLine renders the deterministic visible line for a timeline row —
// the body that lives BETWEEN the entry-block markers — WITHOUT a detail
// link. The link is appended by renderCanonical from the row's source path,
// so it is correct relative to docs/timeline.md regardless of where the
// marker physically lives (a docs/-relative link baked into a branch file
// would resolve wrong from that file's own directory and fail check-links).
// Single-sourced here so `logmind log` (the marker writer) and the synthesized
// rows share one exact byte format.
func HeadlineLine(date time.Time, title string) string {
	return fmt.Sprintf("- **%s** — %s", date.Format("2006-01-02"), title)
}

// GenerateMainCanonical builds the §1.6.4 deterministic-union timeline from
// the same source set as the default model (decisions.md + archive +
// decisions-branches/*), rendered in entry-block format.
func GenerateMainCanonical(docsPath string, stderr io.Writer) (string, error) {
	items, err := collectMarked(docsPath, stderr)
	if err != nil {
		return "", err
	}
	return renderCanonical(items), nil
}

// GenerateFor is the single in-process dispatch point: main-canonical when
// canonical is true, else the byte-stable default brief/full path. Every
// caller (runTimeline's three sites + init's two) routes through here so the
// model selection lives in exactly one place.
func GenerateFor(docsPath string, brief, canonical bool, stderr io.Writer) (string, error) {
	if canonical {
		return GenerateMainCanonical(docsPath, stderr)
	}
	return Generate(docsPath, brief, stderr)
}

// collectMarked gathers one marked row per timeline entry from all sources.
func collectMarked(docsPath string, stderr io.Writer) ([]marked, error) {
	if stderr == nil {
		stderr = os.Stderr
	}
	var items []marked

	// (1) Branch detail pages: use their entry-block markers when present;
	// otherwise (markerless legacy file) synthesize one row from the first
	// decision header so existing repos render with zero file edits.
	branchesDir := filepath.Join(docsPath, "decisions-branches")
	branchFiles, err := decisions.ListBranchFiles(branchesDir)
	if err != nil {
		return nil, err
	}
	for _, bf := range branchFiles {
		base := filepath.Base(bf)
		rel := "decisions-branches/" + base
		data, err := os.ReadFile(bf)
		if err != nil {
			return nil, err
		}
		if blocks := extractEntryBlocks(string(data), stderr); len(blocks) > 0 {
			for _, b := range blocks {
				d, slug, ok := splitKey(b.Key)
				if !ok {
					fmt.Fprintf(stderr, "logmind: skipping entry-block with unparseable key %q in %s\n", b.Key, rel)
					continue
				}
				items = append(items, marked{date: d, slug: slug, body: b.Body, source: rel})
			}
			continue
		}
		entries, err := decisions.Iter(bf, stderr)
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			continue
		}
		e := entries[0]
		items = append(items, marked{date: e.Date, slug: Slugify(e.Title), body: HeadlineLine(e.Date, e.Title), source: rel})
	}

	// (2) Direct-to-main + archive: one synthesized row per decision header
	// (these sources carry no entry-block markers under the newspaper model).
	for _, file := range []string{"decisions.md", "decisions-archive.md"} {
		entries, err := decisions.Iter(filepath.Join(docsPath, file), stderr)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			items = append(items, marked{date: e.Date, slug: Slugify(e.Title), body: HeadlineLine(e.Date, e.Title), source: file})
		}
	}

	return dedupeAndSuffix(items), nil
}

// dedupeAndSuffix collapses identical entries sharing a <date>-<slug> key
// (the same-entry carve-out: a decision present in two sources is ONE entry),
// and when genuinely distinct entries collide on that key appends a numeric
// suffix (-2, -3, …) per §1.6.3.1. Every generated key is checked against a
// GLOBAL used-key set, so a suffix can never duplicate another entry's key —
// including a literal "<slug>-2" that already exists elsewhere in the union
// (§1.6.3.1: the <date>-<slug> pair MUST be unique within the file). Within a
// collision the bare key goes to the lexicographically-smallest source path.
// Output is sorted newest-first by date, tiebreak slug DESCENDING (§1.6.3.2)
// — deterministic, independent of input order and of the checkout path.
//
// Suffix numbers are positional (rank among same-key collisions), so adding a
// collision member that sorts EARLIER renumbers the later ones: keys stay
// unique and bytes stay deterministic, but a given entry's suffix is not
// stable across unrelated additions. (Cross-regeneration suffix stability is
// a §1.6.4 ratification item — see PR7.)
func dedupeAndSuffix(items []marked) []marked {
	// Group by the bare <date>-<slug> key. groupKeys preserves first-seen
	// order for deterministic collision processing; the final sort below
	// fixes output order regardless.
	groups := map[string][]marked{}
	var groupKeys []string
	for _, it := range items {
		k := it.date.Format("2006-01-02") + "-" + it.slug
		if _, ok := groups[k]; !ok {
			groupKeys = append(groupKeys, k)
		}
		groups[k] = append(groups[k], it)
	}

	// Pass 1: collapse identical bodies within each group, and RESERVE every
	// singleton's intrinsic key so a colliding group's suffix can't duplicate
	// it (e.g. an organic "dup" collision must not generate "dup-2" when a
	// literal "dup-2" entry already exists).
	collapsed := make(map[string][]marked, len(groupKeys))
	used := map[string]bool{}
	for _, k := range groupKeys {
		seen := map[string]bool{}
		var distinct []marked
		for _, it := range groups[k] {
			if seen[it.body] {
				continue
			}
			seen[it.body] = true
			distinct = append(distinct, it)
		}
		collapsed[k] = distinct
		if len(distinct) == 1 {
			used[k] = true
		}
	}

	// Pass 2: emit singletons as-is; assign collision members the next free
	// key (checked against `used`), smallest source path first.
	var out []marked
	for _, k := range groupKeys {
		distinct := collapsed[k]
		if len(distinct) == 1 {
			out = append(out, distinct[0])
			continue
		}
		sort.SliceStable(distinct, func(i, j int) bool {
			if distinct[i].source != distinct[j].source {
				return distinct[i].source < distinct[j].source
			}
			return distinct[i].body < distinct[j].body
		})
		datePrefix := distinct[0].date.Format("2006-01-02") + "-"
		baseSlug := distinct[0].slug
		n := 1
		for i := range distinct {
			var slug string
			for {
				if n == 1 {
					slug = baseSlug
				} else {
					slug = fmt.Sprintf("%s-%d", baseSlug, n)
				}
				n++
				if !used[datePrefix+slug] {
					break
				}
			}
			used[datePrefix+slug] = true
			distinct[i].slug = slug
			out = append(out, distinct[i])
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].date.Equal(out[j].date) {
			return out[i].date.After(out[j].date)
		}
		return out[i].slug > out[j].slug
	})
	return out
}

// renderCanonical assembles the entry-block-format timeline: the shared
// header, `## YYYY-MM` month landmarks (OUTSIDE the markers, §1.6.3.2), and
// each row wrapped in its `<date>-<slug>` entry-block markers (§1.6.3.1).
func renderCanonical(items []marked) string {
	if len(items) == 0 {
		return header + emptyBody
	}
	var b strings.Builder
	b.WriteString(header)
	lastMonth := ""
	for _, it := range items {
		if month := it.date.Format("2006-01"); month != lastMonth {
			b.WriteString("\n## ")
			b.WriteString(month)
			b.WriteString("\n")
			lastMonth = month
		}
		b.WriteString("\n")
		b.WriteString(entryStartPrefix)
		b.WriteString(it.date.Format("2006-01-02") + "-" + it.slug)
		b.WriteString(entryStartSuffix)
		b.WriteString("\n")
		// Body (the headline, link-free) + the detail link computed from the
		// source path — correct relative to docs/timeline.md, where this row
		// lands, not relative to the branch file the marker may live in.
		b.WriteString(it.body)
		b.WriteString(" → [detail](")
		b.WriteString(it.source)
		b.WriteString(")\n")
		b.WriteString(entryEndMarker)
		b.WriteString("\n")
	}
	return b.String()
}

// CurrentHeadline returns the verbatim body of the FIRST entry-block in
// content (the branch's visible headline line), and ok=false when content has
// no marker. Used to show "[current: …]" in the per-log summary nudge.
func CurrentHeadline(content string) (headline string, ok bool) {
	blocks := extractEntryBlocks(content, io.Discard)
	if len(blocks) == 0 {
		return "", false
	}
	return blocks[0].Body, true
}

// ReplaceFirstHeadline rewrites the visible body of the FIRST entry-block to a
// fresh HeadlineLine built from `summary` + `prSuffix`, while KEEPING the
// existing <date>-<slug> key (the stable identity — only the sentence is
// refined) and the marker dates. Returns (newContent, true) when a marker was
// found and rewritten; (content, false) when there is no marker, an unclosed
// marker, or a malformed key — so the caller can fall back to inserting one.
func ReplaceFirstHeadline(content, summary, prSuffix string) (string, bool) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		key, ok := parseStartMarker(line)
		if !ok {
			continue
		}
		date, _, ok := splitKey(key)
		if !ok {
			return content, false // malformed key — don't touch it
		}
		for j := i + 1; j < len(lines); j++ {
			if isEndMarker(lines[j]) {
				out := make([]string, 0, len(lines))
				out = append(out, lines[:i+1]...)                       // through the start marker
				out = append(out, HeadlineLine(date, summary+prSuffix)) // the new single body line
				out = append(out, lines[j:]...)                         // from the end marker on
				return strings.Join(out, "\n"), true
			}
		}
		return content, false // unclosed marker
	}
	return content, false // no marker
}
