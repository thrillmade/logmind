// search.go — `logmind search <query>` subcommand.
//
// SKILL.md / AGENTS.md / internal/templates/logmind-section.md document
// `logmind search "keyword"` as "full-text search across recent + archive"
// with `--case-sensitive` and `--no-archive`. This is the v2 implementation.
//
// MATCHING IS LITERAL SUBSTRING, not regex. "Full-text search" means the
// query is matched verbatim: `search "cost($)"` finds a line containing the
// literal text `cost($)`, `search "v1.0"` does NOT match `v1x0`, and
// `search "a|b"` matches only the literal `a|b`. Treating the query as a regex
// would silently drop hits (a valid-but-non-matching pattern) or over-match
// (metacharacters), both surprising for a keyword search. Case-insensitive by
// default; --case-sensitive requires an exact-case match.
//
// SCOPE (deterministic source order, existence-checked, deduped by path):
//
//  1. docs/decisions.md            — main's recent decisions (always)
//  2. the current feature-branch file from resolveDecisionsPath, IF it
//     differs from decisions.md (i.e. only when on a feature branch)
//  3. docs/decisions-archive.md    — unless --no-archive
//
// So a feature-branch agent searching finds main's decisions AND its own
// branch's in-flight decisions AND the archive — the union the docs promise
// ("across recent + archive"). On the default branch, (1) and (2) collapse to
// a single decisions.md entry.
//
// File SELECTION reuses resolveDecisionsPath (the same helper `logmind log`/
// `headline`/`show` use). There is no existing decisions-package helper for
// full-text grep-with-context — decisions.Iter only extracts header metadata
// (date/title), not line-level content — so the line scan here is new.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
)

// searchContextLines is the fixed number of lines of surrounding context
// shown above/below each match. Not exposed as a flag — the documented
// interface (SKILL.md / AGENTS.md) only promises --case-sensitive and
// --no-archive; a fixed, generous default keeps results readable without
// growing the flag surface beyond what's documented.
const searchContextLines = 2

// searchResult is one line-level hit within a decisions file.
type searchResult struct {
	File          string // repo-relative path the hit came from
	DecisionTitle string // nearest preceding "## ..." header, or a placeholder
	LineNumber    int    // 1-indexed
	MatchedLine   string
	ContextBefore []string
	ContextAfter  []string
}

// searchFlags carries the parsed flags for `logmind search`.
type searchFlags struct {
	caseSensitive bool
	noArchive     bool
}

// newSearchCmd wires the `logmind search <query>` subcommand.
func newSearchCmd() *cobra.Command {
	f := &searchFlags{}
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search across recent decisions and the archive",
		Long: `Full-text search across the decision log for a keyword or phrase.

Searches, in order: docs/decisions.md (main's recent decisions), the current
feature branch's decision file (docs/decisions-branches/<branch>.md, when on a
feature branch), and docs/decisions-archive.md — so a feature-branch agent
finds main's decisions, its own branch's in-flight decisions, and the archive.
Pass --no-archive to skip the archive.

The query is matched as a LITERAL substring (not a regex): "cost($)" finds the
literal text "cost($)". Matching is case-insensitive by default — pass
--case-sensitive to require an exact-case match.

Examples:
    logmind search "postgres"
    logmind search "API" --case-sensitive
    logmind search "database" --no-archive`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSearch(cwd, args[0], f, quietEnabled(cmd), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&f.caseSensitive, "case-sensitive", false,
		"Require an exact-case match (default: case-insensitive).")
	cmd.Flags().BoolVar(&f.noArchive, "no-archive", false,
		"Skip docs/decisions-archive.md (search only decisions.md + the current branch file).")
	return cmd
}

// runSearch implements `logmind search`.
func runSearch(cwd, query string, f *searchFlags, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)

	if strings.TrimSpace(query) == "" {
		q.fail("Error: empty search query.\n")
		return ErrSilent
	}

	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		q.fail("Error: docs/ directory not found. Run 'logmind init' first.\n")
		return ErrSilent
	}

	files := searchSources(cwd, docsPath, !f.noArchive)

	var results []searchResult
	for _, file := range files {
		hits, err := searchFile(cwd, file, query, f.caseSensitive)
		if err != nil {
			return fmt.Errorf("search %s: %w", file, err)
		}
		results = append(results, hits...)
	}

	if quiet {
		q.ok("search matches=%d sources=%d archive=%t case_sensitive=%t", len(results), len(files), !f.noArchive, f.caseSensitive)
		return nil
	}

	if len(results) == 0 {
		fmt.Fprintf(stdout, "No matches found for: %s\n", query)
		fmt.Fprintf(stdout, "ok search: 0 matches for %q\n", query)
		return nil
	}

	matchWord := "matches"
	if len(results) == 1 {
		matchWord = "match"
	}
	fmt.Fprintf(stdout, "Found %d %s for: %s\n\n", len(results), matchWord, query)
	fmt.Fprintln(stdout, formatSearchResults(results, query, f.caseSensitive))
	fmt.Fprintf(stdout, "ok search: %d %s for %q\n", len(results), matchWord, query)
	return nil
}

// searchSources returns the deterministic, existence-checked, path-deduped
// list of decision files `search` scans:
//
//  1. docs/decisions.md (main's recent decisions)
//  2. the current feature-branch file (resolveDecisionsPath), if != #1
//  3. docs/decisions-archive.md, when includeArchive
//
// On the default branch #1 and #2 resolve to the same path and collapse to a
// single entry. Missing files are dropped silently — a repo without an archive
// (or a brand-new branch with no decisions file yet) still searches whatever
// exists.
func searchSources(cwd, docsPath string, includeArchive bool) []string {
	cfg, _ := config.Load(cwd)
	branchPath, _ := resolveDecisionsPath(cwd, docsPath, cfg)

	candidates := []string{
		filepath.Join(docsPath, "decisions.md"),
		branchPath, // == decisions.md on the default branch (deduped below)
	}
	if includeArchive {
		candidates = append(candidates, filepath.Join(docsPath, "decisions-archive.md"))
	}

	seen := make(map[string]bool, len(candidates))
	var files []string
	for _, p := range candidates {
		if seen[p] {
			continue
		}
		seen[p] = true
		if pathExists(p) {
			files = append(files, p)
		}
	}
	return files
}

// lineMatches reports whether query occurs as a literal substring of line,
// honoring case sensitivity. This is the single source of truth for match
// semantics — the highlighter uses the same case folding.
func lineMatches(line, query string, caseSensitive bool) bool {
	if caseSensitive {
		return strings.Contains(line, query)
	}
	return strings.Contains(strings.ToLower(line), strings.ToLower(query))
}

// searchFile scans path line-by-line for the literal query, returning one
// searchResult per matching line with ±searchContextLines of surrounding
// context and the nearest preceding "## ..." decision header as the title.
func searchFile(cwd, path, query string, caseSensitive bool) ([]searchResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Split on "\n"; drop the trailing empty element a trailing newline
	// produces so line numbers match what a text editor would show.
	lines := strings.Split(string(data), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	rel := relForOk(cwd, path)
	currentTitle := ""
	var results []searchResult
	for i, line := range lines {
		if strings.HasPrefix(line, "## ") {
			currentTitle = strings.TrimSpace(strings.TrimPrefix(line, "##"))
		}
		if !lineMatches(line, query, caseSensitive) {
			continue
		}
		title := currentTitle
		if title == "" {
			title = "(no decision header yet)"
		}
		start := max(0, i-searchContextLines)
		end := min(len(lines), i+searchContextLines+1)
		results = append(results, searchResult{
			File:          rel,
			DecisionTitle: title,
			LineNumber:    i + 1, // 1-indexed
			MatchedLine:   line,
			ContextBefore: append([]string{}, lines[start:i]...),
			ContextAfter:  append([]string{}, lines[i+1:end]...),
		})
	}
	return results, nil
}

// formatSearchResults renders the multi-line per-result block:
//
//	<file> - <decision-title> (line N)
//	------------------------------------
//	  <context line>
//	> <matched line, with >>> query <<< markers>
//	  <context line>
//
// Results are separated by a blank line. Highlighting wraps the actual matched
// literal substring in >>> <<< markers using the SAME case folding as the
// match test — so a query with regex-special characters (e.g. "cost($)")
// highlights correctly because nothing is ever compiled as a pattern.
func formatSearchResults(results []searchResult, query string, caseSensitive bool) string {
	var out []string
	for i, r := range results {
		if i > 0 {
			out = append(out, "")
		}
		header := fmt.Sprintf("%s - %s (line %d)", r.File, r.DecisionTitle, r.LineNumber)
		out = append(out, header, strings.Repeat("-", len(header)))

		for _, ctx := range r.ContextBefore {
			out = append(out, "  "+ctx)
		}
		out = append(out, "> "+highlightLiteral(r.MatchedLine, query, caseSensitive))
		for _, ctx := range r.ContextAfter {
			out = append(out, "  "+ctx)
		}
	}
	return strings.Join(out, "\n")
}

// highlightLiteral wraps every literal occurrence of query in line with
// ">>> " ... " <<<" markers, preserving the ORIGINAL casing of the matched
// text (not the query's). Case folding matches lineMatches so what gets
// highlighted is exactly what matched. Purely string-index based — no regex —
// so regex-special characters in query are treated literally.
func highlightLiteral(line, query string, caseSensitive bool) string {
	if query == "" {
		return line
	}
	hayIdx := line
	needle := query
	if !caseSensitive {
		hayIdx = strings.ToLower(line)
		needle = strings.ToLower(query)
	}
	// Offsets are computed in hayIdx but sliced out of the original `line`.
	// strings.ToLower can CHANGE byte length for some Unicode (e.g. İ U+0130
	// lowercases to i̇, ẞ → ß), which would desync those offsets and slice
	// `line` at the wrong bytes — misaligned markers or an out-of-range panic.
	// Highlighting is purely cosmetic (match DETECTION via lineMatches is
	// unaffected), so bail out unhighlighted on the rare length-changing fold.
	if len(hayIdx) != len(line) {
		return line
	}
	var b strings.Builder
	for {
		off := strings.Index(hayIdx, needle)
		if off < 0 {
			b.WriteString(line)
			break
		}
		b.WriteString(line[:off])
		b.WriteString(">>> ")
		b.WriteString(line[off : off+len(needle)]) // original casing from line
		b.WriteString(" <<<")
		line = line[off+len(needle):]
		hayIdx = hayIdx[off+len(needle):]
	}
	return b.String()
}
