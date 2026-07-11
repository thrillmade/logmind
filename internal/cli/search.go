// search.go — `logmind search <query>` subcommand.
//
// SKILL.md / AGENTS.md / internal/templates/logmind-section.md document
// `logmind search "keyword"` as "full-text search across recent + archive"
// with `--case-sensitive` and `--no-archive`, but the v2 Go binary never
// shipped it. This is the v2 re-implementation, searching the SAME "recent"
// source `logmind show` prints — resolveDecisionsPath's branch-aware target —
// plus docs/decisions-archive.md unless --no-archive is passed.
//
// The query is compiled as a regular expression; on compile failure it falls
// back to a literal substring match (regexp.QuoteMeta), so a query containing
// unbalanced regex metacharacters still searches instead of erroring. Matches
// are case-insensitive by default; --case-sensitive requires an exact-case
// match.
//
// There is no existing decisions-package helper for full-text grep-with-
// context — decisions.Iter only extracts header metadata (date/title), not
// line-level content — so the line scan here is new, but file SELECTION
// reuses resolveDecisionsPath (the same helper `logmind log`/`headline`/
// `show` use) rather than hardcoding docs/decisions.md.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
		Long: `Full-text search across the decision log for a term or pattern.

Searches the current branch's decision file — the same file "logmind show"
prints (docs/decisions.md on the default branch, docs/decisions-branches/
<branch>.md on a feature branch) — plus docs/decisions-archive.md, unless
--no-archive is passed.

The query is treated as a regular expression; if it fails to compile, it
falls back to a literal substring search. Matching is case-insensitive by
default — pass --case-sensitive to require an exact-case match.

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
		"Search only the current branch's recent decisions; skip docs/decisions-archive.md.")
	return cmd
}

// runSearch implements `logmind search`.
func runSearch(cwd, query string, f *searchFlags, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		q.fail("Error: docs/ directory not found. Run 'logmind init' first.\n")
		return ErrSilent
	}

	cfg, _ := config.Load(cwd)
	recentPath, _ := resolveDecisionsPath(cwd, docsPath, cfg)
	files := []string{recentPath}
	includeArchive := !f.noArchive
	if includeArchive {
		archivePath := filepath.Join(docsPath, "decisions-archive.md")
		if pathExists(archivePath) {
			files = append(files, archivePath)
		}
	}

	pattern := compileSearchPattern(query, f.caseSensitive)

	var results []searchResult
	for _, file := range files {
		if !pathExists(file) {
			continue
		}
		hits, err := searchFile(cwd, file, pattern)
		if err != nil {
			return fmt.Errorf("search %s: %w", file, err)
		}
		results = append(results, hits...)
	}

	if quiet {
		q.ok("search matches=%d archive=%t case_sensitive=%t", len(results), includeArchive, f.caseSensitive)
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
	fmt.Fprintln(stdout, formatSearchResults(results, query))
	fmt.Fprintf(stdout, "ok search: %d %s for %q\n", len(results), matchWord, query)
	return nil
}

// compileSearchPattern builds the match regex. A query that fails to compile
// as a regex (e.g. an unbalanced group) falls back to a literal substring
// match via regexp.QuoteMeta, which cannot itself fail to compile.
func compileSearchPattern(query string, caseSensitive bool) *regexp.Regexp {
	prefix := "(?i)"
	if caseSensitive {
		prefix = ""
	}
	if pattern, err := regexp.Compile(prefix + query); err == nil {
		return pattern
	}
	return regexp.MustCompile(prefix + regexp.QuoteMeta(query))
}

// searchFile scans path line-by-line for pattern, returning one searchResult
// per matching line with ±searchContextLines of surrounding context and the
// nearest preceding "## ..." decision header as the title label.
func searchFile(cwd, path string, pattern *regexp.Regexp) ([]searchResult, error) {
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
		if !pattern.MatchString(line) {
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
//	> <matched line, with >>> term <<< markers>
//	  <context line>
//
// Results are separated by a blank line. highlightTerm is wrapped in
// >>> <<< markers wherever it (case-insensitively) appears in the matched
// line, regardless of whether the search itself was case-sensitive — the
// marker is a display aid, not a re-assertion of the match semantics.
func formatSearchResults(results []searchResult, highlightTerm string) string {
	var out []string
	var highlight *regexp.Regexp
	if highlightTerm != "" {
		highlight = regexp.MustCompile("(?i)(" + regexp.QuoteMeta(highlightTerm) + ")")
	}

	for i, r := range results {
		if i > 0 {
			out = append(out, "")
		}
		header := fmt.Sprintf("%s - %s (line %d)", r.File, r.DecisionTitle, r.LineNumber)
		out = append(out, header, strings.Repeat("-", len(header)))

		for _, ctx := range r.ContextBefore {
			out = append(out, "  "+ctx)
		}
		matched := r.MatchedLine
		if highlight != nil {
			matched = highlight.ReplaceAllString(matched, ">>> $1 <<<")
		}
		out = append(out, "> "+matched)
		for _, ctx := range r.ContextAfter {
			out = append(out, "  "+ctx)
		}
	}
	return strings.Join(out, "\n")
}
