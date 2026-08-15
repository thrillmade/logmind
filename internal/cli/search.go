// search.go — `logmind search <query>` subcommand.
//
// SKILL.md / AGENTS.md / internal/templates/logmind-section.md document
// `logmind search "keyword"` with `--case-sensitive` and `--no-archive`.
// This is the v2 implementation. SPEC §3.2 made every decision file
// append-only and uncapped, so nothing rotates into docs/decisions-archive.md
// any more and no NEW repo grows one; `--no-archive` is therefore a no-op —
// see its flag registration below. An archive left behind by a pre-§3.2
// binary is still SEARCHED: it holds real decisions, and "a decision written
// is a decision kept" outranks the flag's original meaning.
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
//  1. docs/decisions.md          — the pre-§3.2 main log. Still written where
//     no branch NAME exists to name a file after (non-git, detached HEAD,
//     unborn repo, branch_aware off — see resolveDecisionsPath in log.go),
//     and read so an un-migrated repo's entries stay findable.
//  2. docs/decisions-archive.md  — the retired rotation overflow. Nothing
//     writes it; read so a repo that rotated under the old `max_recent: 20`
//     default keeps its archived decisions findable.
//  3. the DEFAULT branch's file (docs/decisions-branches/<default>.md),
//     scanned whatever branch is checked out.
//  4. the CURRENT branch's file from resolveDecisionsPath, IF it differs
//     from #3.
//
// #1 and #2 come from decisions.NonBranchSources(), the single owner of that
// list. #3 exists because §3.2 made main a branch: without it, an agent on a
// feature branch — which is where agents work — got zero hits for every
// decision ever logged on the default branch. Sources collapse to one entry
// wherever they resolve to the same path, so being on the default branch does
// not double its hits.
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
	"github.com/thrillmade/logmind/internal/decisions"
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
		Short: "Full-text search across the decision log",
		Long: `Full-text search across the decision log for a keyword or phrase.

Searches, in order: docs/decisions.md (the pre-v2 main log, where one still
exists), docs/decisions-archive.md (a legacy archive, where one still exists),
the DEFAULT branch's decision file, and the current branch's decision file.
The default branch's history is searched from every branch, so a decision
logged on main stays findable from a feature branch.

Decision files are append-only and uncapped, so nothing is archived any more
and --no-archive does nothing.

The query is matched as a LITERAL substring (not a regex): "cost($)" finds the
literal text "cost($)". Matching is case-insensitive by default — pass
--case-sensitive to require an exact-case match.

Examples:
    logmind search "postgres"
    logmind search "API" --case-sensitive
    logmind search "database"`,
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
	// --no-archive is accepted but ignored. SPEC §3.2 stopped rotation, so no
	// repo grows a new docs/decisions-archive.md and there is nothing for the
	// flag to exclude going forward; a LEGACY archive is searched regardless,
	// because excluding real decisions from the "was this decided before?"
	// surface is the loss this flag was never meant to cause. Kept registered
	// so existing scripts and agent skills that still pass it don't error.
	cmd.Flags().BoolVar(&f.noArchive, "no-archive", false,
		"No-op (kept for backward compatibility). Decision files are uncapped; a legacy archive is always searched.")
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

	files := searchSources(cwd, docsPath)

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
//  1. every decisions.NonBranchSources() file — docs/decisions.md (the
//     pre-§3.2 main log) then docs/decisions-archive.md (the retired
//     rotation overflow)
//  2. the DEFAULT branch's file (defaultBranchDecisionsPath) — unconditionally,
//     not only while it is checked out
//  3. the CURRENT branch's file (resolveDecisionsPath), if != #2
//
// #2 is the whole point of this function and must not be dropped back to "the
// current branch only". §3.2 moved the default branch's decisions into
// docs/decisions-branches/main.md, so scanning only the current branch made
// `search` — the primary "was this decided before?" surface — return zero for
// everything ever logged on main the moment an agent checked out a feature
// branch, which is where agents work. The default branch is resolved, never
// hardcoded to "main"; see defaultBranchDecisionsPath.
//
// Missing files are dropped silently — a repo with no legacy main log, no
// archive, or a brand-new branch with no decisions file yet, still searches
// whatever exists. Paths are deduped, so on the default branch #2 and #3
// collapse to one source and hits are never doubled.
func searchSources(cwd, docsPath string) []string {
	cfg, _ := config.Load(cwd)
	branchPath, _ := resolveDecisionsPath(cwd, docsPath, cfg)

	var candidates []string
	for _, src := range decisions.NonBranchSources() {
		candidates = append(candidates, filepath.Join(docsPath, src.File))
	}
	if defaultPath, ok := defaultBranchDecisionsPath(cwd, docsPath, cfg); ok {
		candidates = append(candidates, defaultPath)
	}
	candidates = append(candidates, branchPath)

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
