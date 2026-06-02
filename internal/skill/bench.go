package skill

import (
	"fmt"
	"regexp"
	"strings"
)

// Default thresholds used by `logmind skill bench`. Mirror Python's
// _LOGMIND_SKILL_TIGHT_BYTES / _LOGMIND_SKILL_BUDGET_BYTES at v0.6.16.
//
// `tight`: well-trimmed (~500 tokens at 4 bytes/token).
// `budget`: past this, splitting helps (~1500 tokens).
const (
	DefaultBenchTarget = 2000
	DefaultBenchBudget = 6000
)

// headerRE locates markdown headers (## My Section). Mirrors Python's
// _HEADER_RE — same multi-line + group structure.
var (
	headerRE      = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*$`)
	htmlCommentRE = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// BenchSection mirrors the per-section dict Python emits.
type BenchSection struct {
	Name  string `json:"name"`
	Bytes int    `json:"bytes"`
	Pct   int    `json:"pct"`
}

// BenchResult is the bench measurement for one SKILL.md body.
// Field tags map exactly to Python's JSON output keys
// (`json.dumps(result, indent=2)` in skill_bench's --json branch) — the
// snapshot tests pin the JSON shape byte-for-byte.
type BenchResult struct {
	Bytes       int            `json:"bytes"`
	EstTokens   int            `json:"est_tokens"`
	Status      string         `json:"status"`
	Target      int            `json:"target"`
	Budget      int            `json:"budget"`
	Sections    []BenchSection `json:"sections"`
	Suggestions []string       `json:"suggestions"`
}

// BenchSkill measures per-call token cost of a SKILL.md body.
//
// Mirrors Python's bench_skill — same byte counts, same status bucket,
// same suggestion heuristics. The `target` and `budget` parameters are
// honored by the status bucketer + suggestion generator (PR #99 review
// fix — prior versions silently ignored caller overrides).
func BenchSkill(content string, target, budget int) BenchResult {
	if target <= 0 {
		target = DefaultBenchTarget
	}
	if budget <= 0 {
		budget = DefaultBenchBudget
	}
	total := len(content)
	sectionsRaw := splitSections(content)
	sections := make([]BenchSection, 0, len(sectionsRaw))
	for _, s := range sectionsRaw {
		pct := 0
		if total > 0 {
			pct = (s.Bytes * 100) / total
		}
		sections = append(sections, BenchSection{
			Name:  s.Name,
			Bytes: s.Bytes,
			Pct:   pct,
		})
	}
	suggestions := trimSuggestions(content, sectionsRaw, total, target, budget)
	if suggestions == nil {
		// Python emits `[]`, not `null`, when there are no suggestions.
		// Keep parity with json.dumps so the `--json` snapshot tests
		// match byte-for-byte.
		suggestions = []string{}
	}
	return BenchResult{
		Bytes:       total,
		EstTokens:   total / 4, // English-text approximation Python uses.
		Status:      benchStatus(total, target, budget),
		Target:      target,
		Budget:      budget,
		Sections:    sections,
		Suggestions: suggestions,
	}
}

// sectionInfo is the internal (name, bytes) tuple Python returns from
// _split_into_sections.
type sectionInfo struct {
	Name  string
	Bytes int
}

// splitSections mirrors Python's _split_into_sections. The frontmatter
// `---...---` block becomes its own section; the leading body (before
// the first `##`) is `intro`; subsequent `##` headers each open a new
// section.
func splitSections(content string) []sectionInfo {
	var out []sectionInfo

	rest := content
	if strings.HasPrefix(content, "---") {
		// end is the index of `\n---` in content[4:], or -1 if absent.
		idx := strings.Index(content[4:], "\n---")
		if idx != -1 {
			// content[:end+4] in Python where `end` was absolute.
			// Here idx is relative, so the absolute end is 4 + idx.
			absEnd := 4 + idx
			frontmatter := content[:absEnd+4]
			out = append(out, sectionInfo{"frontmatter", len(frontmatter)})
			rest = strings.TrimLeft(content[absEnd+4:], "\n")
		}
	}

	// Find every ## section header (level 2). We mirror Python's
	// "(start_position, level, title)" tuples but only retain level==2
	// — same filter Python applies.
	type hdr struct {
		Start int
		Title string
	}
	var headers []hdr
	for _, m := range headerRE.FindAllStringSubmatchIndex(rest, -1) {
		// m = [matchStart, matchEnd, g1Start, g1End, g2Start, g2End]
		level := m[3] - m[2]
		if level != 2 {
			continue
		}
		title := rest[m[4]:m[5]]
		// Python's `.+?` is non-greedy and the `\s*$` trims trailing
		// whitespace. Regexp here already gives us the trimmed title.
		title = strings.TrimRight(title, " \t")
		headers = append(headers, hdr{Start: m[0], Title: title})
	}

	if len(headers) == 0 {
		// Whole body is one chunk.
		body := strings.TrimSpace(rest)
		if body != "" {
			out = append(out, sectionInfo{"body", len(body)})
		}
		return out
	}

	// Intro = anything before the first ## header.
	intro := strings.TrimSpace(rest[:headers[0].Start])
	if intro != "" {
		out = append(out, sectionInfo{"intro", len(intro)})
	}

	for i, h := range headers {
		end := len(rest)
		if i+1 < len(headers) {
			end = headers[i+1].Start
		}
		sectionBody := strings.TrimSpace(rest[h.Start:end])
		out = append(out, sectionInfo{h.Title, len(sectionBody)})
	}
	return out
}

// benchStatus buckets the size into a status label. Mirrors Python's
// _bench_status — caller-supplied thresholds, with the
// LogmindByteCap-floor for `over-budget`.
func benchStatus(size, target, budget int) string {
	switch {
	case size <= target:
		return "tight"
	case size <= budget:
		return "typical"
	case size <= LogmindByteCap:
		return "verbose"
	default:
		return "over-budget"
	}
}

// trimSuggestions mirrors Python's _trim_suggestions. The three
// heuristics fire in order; the generic-fallback branch only triggers
// when none of the specific suggestions did.
func trimSuggestions(content string, sections []sectionInfo, total, target, budget int) []string {
	var out []string
	if total <= budget {
		return out
	}

	// Heuristic 1: any single section > 30% of total is a candidate.
	for _, s := range sections {
		if s.Name == "frontmatter" || s.Name == "intro" || s.Name == "body" {
			continue
		}
		if total <= 0 {
			continue
		}
		// Python: size / total > 0.30 → float comparison. We compare
		// via multiplication to keep Go integer-safe without losing
		// precision at the threshold.
		if s.Bytes*100 > total*30 {
			pct := s.Bytes * 100 / total
			out = append(out, fmt.Sprintf(
				"Section '%s' is %d bytes (%d%% of total) — "+
					"consider linking out to docs OR moving to its own skill.",
				s.Name, s.Bytes, pct,
			))
		}
	}

	// Heuristic 2: HTML comments — agents load them, but they don't
	// render in the visible prompt.
	commentBytes := 0
	for _, m := range htmlCommentRE.FindAllString(content, -1) {
		commentBytes += len(m)
	}
	if commentBytes >= 200 {
		out = append(out, fmt.Sprintf(
			"%d bytes of HTML comments — agents load them too. "+
				"Move authoring notes to a sibling NOTES.md if they're not for the agent.",
			commentBytes,
		))
	}

	// Heuristic 3: over the hard cap → split.
	if total > LogmindByteCap {
		out = append(out, fmt.Sprintf(
			"Total exceeds %d-byte cap — split into multiple focused skills.",
			LogmindByteCap,
		))
	} else if len(out) == 0 {
		// Generic fallback when verbose but no specific culprit
		// section.
		out = append(out, fmt.Sprintf(
			"Total is %d bytes (target: %d, budget: %d). "+
				"Tighten the largest sections or move detailed examples behind links.",
			total, target, budget,
		))
	}

	return out
}
