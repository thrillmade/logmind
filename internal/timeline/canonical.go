// canonical.go — the main-canonical ("newspaper") timeline.
//
// This implements SPEC §1.6.3's HTML-marker entry-block format and the
// §1.6.4 deterministic-union timeline. As of v2.0.0 this is THE timeline:
// Generate below is the sole assembly path. Same source tree ⇒ same bytes
// on any branch, worktree, or checkout. See docs/plan / the protocol SPEC
// §1.6.3.
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
			out = append(out, entryBlock{Key: key, Body: joinBodyLines(lines[i+1 : j])})
			i = j + 1
		}
	}
	return out
}

// joinBodyLines joins entry-block body lines with "\n", stripping any
// trailing "\r" from each line first. A CRLF-line-ended source (e.g. a
// Windows core.autocrlf=true checkout of docs/decisions-branches/*.md) would
// otherwise leave a "\r" on every body line after the raw "\n" split, which
// (a) writes a literal CR into docs/timeline.md — breaking this file's own
// byte-determinism invariant (see the package doc comment) — and (b) can
// defeat dedupeAndSuffix's same-body carve-out, since "body\r" != "body".
func joinBodyLines(lines []string) string {
	clean := make([]string, len(lines))
	for i, l := range lines {
		clean[i] = strings.TrimSuffix(l, "\r")
	}
	return strings.Join(clean, "\n")
}

// --- §1.6.4 main-canonical assembly ---------------------------------------
//
// The main-canonical timeline is a DETERMINISTIC UNION of §1.6.3 entry-block
// markers — no LLM, no re-summarization. Same source tree ⇒ same bytes on
// any branch, worktree, or checkout path. As of v2.0.0 it is the SOLE,
// unconditional timeline assembly model.

// marked is one timeline row: its date+slug identity, the link-free headline
// body rendered between the entry-block markers, and its source path (the
// detail-page link target, AND the deterministic collision tiebreak — both
// relative to docs/, e.g. "decisions-branches/feat__x.md").
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
//
// The title is collapsed to a SINGLE line (any CR/LF/tab run → one space):
// the headline is a one-line field by contract, and an un-collapsed multi-line
// title would push a forged `<!-- logmind-entry-start: … -->` line to column 0
// of the branch file — corrupting the union and silently dropping the real
// entry. This is the single chokepoint for every write path (the marker
// writers + the synthesized rows), so sanitizing here covers them all.
func HeadlineLine(date time.Time, title string) string {
	return fmt.Sprintf("- **%s** — %s", date.Format("2006-01-02"), collapseToLine(title))
}

// collapseToLine replaces every run of whitespace (including CR/LF) with a
// single space and trims the ends, guaranteeing a single-line result.
func collapseToLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Generate builds the §1.6.4 deterministic-union timeline from the branch
// files under docsPath and returns BOTH renderings of it: `recent` (the
// RecentLimit newest entries → docs/timeline.md) and `archive` (everything
// older → docs/timeline-archive.md). This is the sole timeline generator.
//
// Returning both from one call is the mechanism §3.3 asks for: "a producer
// renders the newest 50 to one path and the remainder to the other, every
// time, from the branch files that hold the actual record." There is no
// second entry point that produces one half on its own, so the two can never
// be rendered from different reads of the sources — and neither rendering is
// ever an INPUT here, so an entry cannot change hands between them.
func Generate(docsPath string, stderr io.Writer) (recent, archive string, err error) {
	return generateAt(docsPath, RecentLimit, stderr)
}

// generateAt is Generate with the split point as a parameter. Only Generate
// (with RecentLimit) and the tests that prove the bound is a pure rendering
// choice call it: one source read, one sorted union, sliced in two.
//
// The slice is the whole of the split. Nothing reads either output file to
// decide what belongs in it, so changing `limit` is a regeneration and never
// a migration — the union is identical, only the cut moves.
func generateAt(docsPath string, limit int, stderr io.Writer) (recent, archive string, err error) {
	items, err := collectMarked(docsPath, stderr)
	if err != nil {
		return "", "", err
	}
	if limit < 0 {
		limit = 0
	}
	if limit > len(items) {
		limit = len(items)
	}
	return renderCanonical(header, emptyBody, items[:limit]),
		renderCanonical(archiveHeader, emptyArchiveBody, items[limit:]),
		nil
}

// dateOnly truncates a timestamp to midnight in its own location, so a
// synthesized/fallback row sorts at DATE granularity — exactly like a marker
// row (whose <date>-<slug> key is date-only) and as §1.6.3.2 requires (order
// by <date>, tiebreak slug). Without this, the same entry sorts at HH:MM as a
// markerless fallback but at midnight once `doctor --fix` backfills its marker,
// silently reordering same-day rows. Date-only makes markerless == backfilled.
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
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
		// FIRST entry, per SPEC line 646: "Where none exists the producer
		// MUST derive the sentence from the branch's first decision title,
		// so a summary always resolves without a model call."
		//
		// Dating from the NEWEST entry is better engineering and was ruled
		// for by the CEO: every branch file except one eventually closes, so
		// first-vs-last is immaterial for them, while `main.md` never closes
		// and its row froze at its oldest decision while the file kept
		// growing — sinking past §3.3's cut into the archive and taking
		// everything logged on main with it.
		//
		// But that contradicts a normative MUST, and logmind's job is
		// conforming to this document, not improving on it unilaterally.
		// Proposed at thrillmade/protocol#97; this line changes when the SPEC
		// does, not before.
		e := entries[0]
		d := dateOnly(e.Date)
		items = append(items, marked{date: d, slug: Slugify(e.Title), body: HeadlineLine(d, e.Title), source: rel})
	}

	// (2) The decision files that are not named after a branch —
	// docs/decisions.md and docs/decisions-archive.md. See
	// decisions.NonBranchSources() for the list and for which of them is
	// still written: a decision made on main goes in main's branch file like
	// any other, but decisions.md still receives the branchless cases, and
	// the archive receives nothing at all. Both are read here so a repository
	// that upgrades the binary before migrating its pre-§3.2 history does not
	// silently lose it from the timeline. One synthesized row per decision
	// header — neither source ever carried entry-block markers.
	for _, src := range decisions.NonBranchSources() {
		entries, err := decisions.Iter(filepath.Join(docsPath, src.File), stderr)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			d := dateOnly(e.Date)
			items = append(items, marked{date: d, slug: Slugify(e.Title), body: HeadlineLine(d, e.Title), source: src.File})
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

// renderCanonical assembles the entry-block-format timeline: the given
// header, `## YYYY-MM` month landmarks (OUTSIDE the markers, §1.6.3.2), and
// each row wrapped in its `<date>-<slug>` entry-block markers (§1.6.3.1).
//
// Both halves of the §3.3 split render through this one function, differing
// only in their preface — that is what "the same format under the same
// rules" means concretely, and it is why a reader who reaches the end of
// docs/timeline.md continues in docs/timeline-archive.md without changing
// how they read.
func renderCanonical(header, empty string, items []marked) string {
	if len(items) == 0 {
		return header + empty
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
