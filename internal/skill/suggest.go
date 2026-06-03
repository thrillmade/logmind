package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Suggestion mirrors Python's per-pattern dict from
// suggest_skills_from_decisions(). JSON tags pin the
// `--json` snapshot output.
type Suggestion struct {
	Phrase           string            `json:"phrase"`
	Slug             string            `json:"slug"`
	DecisionCount    int               `json:"decision_count"`
	Evidence         []SuggestEvidence `json:"evidence"`
	DraftDescription string            `json:"draft_description"`
}

// SuggestEvidence is one entry in the evidence list.
type SuggestEvidence struct {
	File    string `json:"file"`
	Snippet string `json:"snippet"`
}

// suggestStopwords mirrors Python's _SUGGEST_STOPWORDS frozenset. Bulk
// copied to maintain pattern-detection parity.
var suggestStopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "but": {}, "if": {},
	"then": {}, "else": {}, "of": {}, "to": {}, "for": {}, "in": {}, "on": {},
	"at": {}, "by": {}, "with": {}, "as": {}, "is": {}, "are": {}, "was": {},
	"were": {}, "be": {}, "been": {}, "being": {}, "have": {}, "has": {},
	"had": {}, "do": {}, "does": {}, "did": {}, "this": {}, "that": {},
	"these": {}, "those": {}, "it": {}, "its": {}, "we": {}, "our": {},
	"us": {}, "they": {}, "them": {}, "their": {}, "i": {}, "my": {}, "me": {},
	"you": {}, "your": {}, "he": {}, "she": {}, "his": {}, "her": {},
	"use": {}, "uses": {}, "used": {}, "using": {}, "make": {}, "made": {},
	"makes": {}, "ship": {}, "shipped": {}, "ships": {}, "add": {},
	"added": {}, "adds": {}, "remove": {}, "removes": {}, "removed": {},
	"fix": {}, "fixed": {}, "fixes": {}, "build": {}, "built": {},
	"test": {}, "tests": {}, "tested": {}, "run": {}, "ran": {}, "runs": {},
	"see": {}, "saw": {}, "seen": {}, "get": {}, "got": {}, "want": {},
	"wants": {}, "need": {}, "needs": {}, "needed": {}, "should": {},
	"could": {}, "would": {}, "will": {}, "can": {}, "may": {}, "might": {},
	"must": {}, "let": {}, "lets": {},
	"code": {}, "file": {}, "files": {}, "function": {}, "functions": {},
	"method": {}, "methods": {}, "class": {}, "classes": {}, "module": {},
	"modules": {}, "library": {}, "libraries": {},
	"feature": {}, "features": {}, "change": {}, "changes": {},
	"release": {}, "releases": {}, "version": {}, "versions": {},
	"branch": {}, "branches": {}, "commit": {}, "commits": {},
	"main": {}, "now": {}, "today": {}, "all": {}, "any": {}, "some": {},
	"one": {}, "two": {}, "three": {},
	"first": {}, "second": {}, "next": {}, "last": {}, "new": {}, "old": {},
	"before": {}, "after": {},
	"decision": {}, "decisions": {}, "reasoning": {}, "reason": {},
	"alternatives": {}, "alternative": {}, "implications": {},
	"implication": {}, "summary": {}, "context": {}, "date": {}, "pr": {},
}

// interestingTokenRE mirrors Python's _INTERESTING_TOKEN_RE — same
// alternation order, same anchoring. Token kinds (in order):
//
//	1. kebab-case multi-word: api-versioning, file-structure-check
//	2. PascalCase / camelCase: PostgreSQL, AuthHandler
//	3. acronyms: API, JWT, CI, RPC
//	4. snake_case multi-word: skill_cli, my_handler
var interestingTokenRE = regexp.MustCompile(
	`\b(` +
		`[a-z]+(?:-[a-z]+)+` + // kebab-case multi-word
		`|[A-Z][a-z]+(?:[A-Z][a-z]+)+` + // PascalCase / camelCase
		`|[A-Z]{2,}` + // acronyms (API, JWT, CI)
		`|[a-z]+_[a-z]+(?:_[a-z]+)*` + // snake_case
		`)\b`,
)

// entryDateRE matches "**Date**: YYYY-MM-DD" / "Date: YYYY-MM-DD" lines.
var entryDateRE = regexp.MustCompile(`(?m)^\s*\*{0,2}Date\*{0,2}\s*:\s*(\d{4}-\d{2}-\d{2})`)

// decisionEntry mirrors Python's per-entry dict from
// _gather_recent_decisions. The `file` field is the bare filename (no
// directory prefix) — exactly what Python emits via path.name.
type decisionEntry struct {
	File   string
	Header string
	Body   string
}

// SuggestFromDecisions scans recent decision-log entries for repeated
// tokens that might justify a new skill. Returns up to topN
// suggestions, each one a Suggestion struct.
//
// Mirrors Python's suggest_skills_from_decisions — same heuristic, same
// stop-word filter, same "first occurrence per entry" evidence model.
// The `now` parameter is injectable for testability; defaults to today.
func SuggestFromDecisions(repoRoot string, sinceDays, minDecisions, topN int, now time.Time) []Suggestion {
	if now.IsZero() {
		now = time.Now()
	}
	threshold := now.AddDate(0, 0, -sinceDays)
	thresholdDay := time.Date(threshold.Year(), threshold.Month(), threshold.Day(), 0, 0, 0, 0, time.UTC)

	docsDir := filepath.Join(repoRoot, "docs")
	st, err := os.Stat(docsDir)
	if err != nil || !st.IsDir() {
		return nil
	}

	entries := gatherRecentDecisions(docsDir, thresholdDay)
	if len(entries) == 0 {
		return nil
	}

	existingSkillNames := loadExistingSkillNames(repoRoot)

	type evidenceHit struct {
		EntryIndex int
		Evidence   SuggestEvidence
	}
	tokenEvidence := map[string][]evidenceHit{}

	for idx, entry := range entries {
		seenInEntry := map[string]struct{}{}
		body := entry.Body
		for _, m := range interestingTokenRE.FindAllStringSubmatchIndex(body, -1) {
			tok := body[m[2]:m[3]]
			tokLower := strings.ToLower(tok)
			if _, stop := suggestStopwords[tokLower]; stop {
				continue
			}
			if _, seen := seenInEntry[tokLower]; seen {
				continue
			}
			seenInEntry[tokLower] = struct{}{}
			snippet := excerptAround(body, m[2], 80)
			tokenEvidence[tok] = append(tokenEvidence[tok], evidenceHit{
				EntryIndex: idx,
				Evidence:   SuggestEvidence{File: entry.File, Snippet: snippet},
			})
		}
	}

	type rankedPair struct {
		Token string
		Hits  []evidenceHit
	}
	var ranked []rankedPair
	for tok, hits := range tokenEvidence {
		if len(hits) < minDecisions {
			continue
		}
		slug := kebabSlug(tok)
		if _, exists := existingSkillNames[slug]; exists {
			continue
		}
		ranked = append(ranked, rankedPair{tok, hits})
	}

	// Sort: descending by hit count, then ascending by lowercased
	// token (deterministic tiebreak — matches Python's key tuple).
	sort.Slice(ranked, func(i, j int) bool {
		if len(ranked[i].Hits) != len(ranked[j].Hits) {
			return len(ranked[i].Hits) > len(ranked[j].Hits)
		}
		return strings.ToLower(ranked[i].Token) < strings.ToLower(ranked[j].Token)
	})

	if topN > 0 && len(ranked) > topN {
		ranked = ranked[:topN]
	}

	out := make([]Suggestion, 0, len(ranked))
	for _, pair := range ranked {
		slug := kebabSlug(pair.Token)
		evCap := 5
		if len(pair.Hits) < evCap {
			evCap = len(pair.Hits)
		}
		evidence := make([]SuggestEvidence, 0, evCap)
		for _, h := range pair.Hits[:evCap] {
			evidence = append(evidence, h.Evidence)
		}
		out = append(out, Suggestion{
			Phrase:        pair.Token,
			Slug:          slug,
			DecisionCount: len(pair.Hits),
			Evidence:      evidence,
			DraftDescription: fmt.Sprintf(
				"When working on %s, follow consistent conventions across "+
					"the codebase. (TODO: replace with concrete trigger + steps.)",
				pair.Token,
			),
		})
	}
	return out
}

func loadExistingSkillNames(repoRoot string) map[string]struct{} {
	out := map[string]struct{}{}
	skillsDir := SkillsDir(repoRoot)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mdPath := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		st, err := os.Stat(mdPath)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		out[strings.ToLower(e.Name())] = struct{}{}
	}
	return out
}

// gatherRecentDecisions parses decision entries split by `## ` headers
// from docs/decisions.md + docs/decisions-branches/*.md, returning
// only those that fall inside the threshold window.
//
// Filtering matches Python's PR #101 review fix: filter at the
// ENTRY level via _extract_entry_date, NOT at the file level. The main
// decisions.md is appended on every log call, so its mtime is always
// today — a file-mtime filter would leak every decision ever logged.
func gatherRecentDecisions(docsDir string, threshold time.Time) []decisionEntry {
	var candidates []string
	mainPath := filepath.Join(docsDir, "decisions.md")
	if _, err := os.Stat(mainPath); err == nil {
		candidates = append(candidates, mainPath)
	}
	branchesDir := filepath.Join(docsDir, "decisions-branches")
	if st, err := os.Stat(branchesDir); err == nil && st.IsDir() {
		if entries, err := os.ReadDir(branchesDir); err == nil {
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				candidates = append(candidates, filepath.Join(branchesDir, e.Name()))
			}
		}
	}

	var entries []decisionEntry
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(data)
		rel := filepath.Base(path)
		fileMtime := time.Time{}
		if st, err := os.Stat(path); err == nil {
			fileMtime = st.ModTime()
		}

		// Python: re.split(r"\n## ", text) — splits on the literal
		// newline-followed-by-`## ` boundary. The first element is
		// everything before the first `## `; subsequent elements omit
		// the leading `## `, which Python re-prepends when re-parsing
		// via the `if i == 0 and not part.startswith("## ")` check.
		parts := strings.Split(text, "\n## ")
		for i, part := range parts {
			if strings.TrimSpace(part) == "" {
				continue
			}
			if i == 0 && !strings.HasPrefix(part, "## ") {
				continue
			}
			headerEnd := strings.Index(part, "\n")
			var header, body string
			if headerEnd != -1 {
				header = strings.TrimSpace(part[:headerEnd])
				body = strings.TrimSpace(part[headerEnd:])
			} else {
				header = strings.TrimSpace(part)
				body = ""
			}

			entryDate, hasDate := extractEntryDate(body)
			if !hasDate {
				// No date on this entry. Fall back to file mtime for
				// branch files only — main decisions.md without dates
				// is ambiguous (entries could be from months ago).
				if rel == "decisions.md" {
					continue
				}
				if fileMtime.IsZero() || fileMtime.Before(threshold) {
					continue
				}
			} else if entryDate.Before(threshold) {
				continue
			}

			entries = append(entries, decisionEntry{File: rel, Header: header, Body: body})
		}
	}
	return entries
}

func extractEntryDate(body string) (time.Time, bool) {
	m := entryDateRE.FindStringSubmatch(body)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", m[1])
	if err != nil {
		return time.Time{}, false
	}
	// Normalise to UTC midnight so .Before comparisons against the
	// threshold (also UTC midnight) line up cleanly.
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
}

// excerptAround returns a readable snippet around byte position idx.
// Mirrors Python's _excerpt_around — same width math, same ellipsis
// markers ("…" U+2026) when truncating at either edge.
func excerptAround(text string, idx, width int) string {
	start := idx - width/2
	if start < 0 {
		start = 0
	}
	end := idx + width/2
	if end > len(text) {
		end = len(text)
	}
	snippet := strings.TrimSpace(strings.ReplaceAll(text[start:end], "\n", " "))
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(text) {
		snippet = snippet + "…"
	}
	return snippet
}

// kebabSlug mirrors Python's _kebab_slug. PascalCase → pascal-case,
// snake_case → snake-case, etc.
func kebabSlug(token string) string {
	// Step 1: insert `-` between lower→upper transitions (camelCase /
	// PascalCase splitter). Python uses lookbehind/lookahead; Go's
	// regexp doesn't support lookbehind, so walk the rune slice manually.
	var b strings.Builder
	runes := []rune(token)
	for i, r := range runes {
		if i > 0 && isASCIILetter(runes[i-1]) && r >= 'A' && r <= 'Z' {
			prevLower := runes[i-1] >= 'a' && runes[i-1] <= 'z'
			if prevLower {
				b.WriteRune('-')
			}
		}
		b.WriteRune(r)
	}
	spaced := b.String()
	spaced = strings.ReplaceAll(spaced, "_", "-")
	lower := strings.ToLower(spaced)
	// Collapse repeated `-` runs.
	for strings.Contains(lower, "--") {
		lower = strings.ReplaceAll(lower, "--", "-")
	}
	return strings.Trim(lower, "-")
}

// FormatIssueDraft mirrors Python's format_suggest_issue_draft — same
// section headers, same evidence-bullet format. Used by
// `logmind skill suggest --write-drafts`.
func FormatIssueDraft(s Suggestion) string {
	var evidence strings.Builder
	for i, e := range s.Evidence {
		if i > 0 {
			evidence.WriteString("\n")
		}
		evidence.WriteString(fmt.Sprintf("- `%s`: %s", e.File, e.Snippet))
	}
	return fmt.Sprintf(
		"## New skill proposal: %s\n"+
			"\n"+
			"### Slug\n"+
			"`%s`\n"+
			"\n"+
			"### Trigger\n"+
			"When working on `%s` — pattern emerged in %d recent decisions.\n"+
			"\n"+
			"### Evidence (auto-extracted from decision log)\n"+
			"%s\n"+
			"\n"+
			"### Draft frontmatter description\n"+
			"%s\n"+
			"\n"+
			"### Review mode\n"+
			"`critical-only` (default — adjust if needed)\n"+
			"\n"+
			"### Scope\n"+
			"_(choose: cross-repo catalog vs single-repo custom)_\n"+
			"\n"+
			"### Applies to\n"+
			"_(globs — leave blank for repo-wide)_\n"+
			"\n"+
			"---\n"+
			"\n"+
			"_Generated by `logmind skill suggest`. Auto-extracted patterns "+
			"are heuristic — please review evidence and refine before opening._",
		s.Slug, s.Slug, s.Phrase, s.DecisionCount,
		evidence.String(), s.DraftDescription,
	)
}
