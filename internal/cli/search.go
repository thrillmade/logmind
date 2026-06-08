// search.go — `logmind search` subcommand. Go port of src/logmind/cli.py's
// search command (Python `@main.command()` at cli.py:1330-1417) +
// src/logmind/core/search.py (the searcher engine) restored for v1.2.1
// after being dropped in the v1.0 Go rewrite.
//
// SPEC §3.3 / §A.3 requires this command. Behavior mirrors Python
// v0.6.16 verbatim — substring/regex search across decisions.md +
// decisions-archive.md with N context lines and >>> match <<< highlight
// markers when case-insensitive.
//
// Surface (mirrors Python click options):
//
//   - <query>             positional, required. Treated as a Go regexp;
//                         on regex-compile failure, falls back to a
//                         literal substring search (matches Python
//                         try/except re.error → re.escape pattern).
//   - --case-sensitive/-c default: false (case-insensitive search).
//   - --no-archive        skip decisions-archive.md.
//   - --no-context        suppress the ±N context-line block.
//   - --context-lines/-C N number of context lines to show (default 2).
//
// Result format (mirrors Python format_search_results):
//
//   <file> - <decision-title> (line <N>)
//   --------------------------------------
//     <ctx line>
//     <ctx line>
//   > <matched line, with >>> term <<< marker when case-insensitive>
//     <ctx line>
//     <ctx line>
//
// Multiple results separated by a blank line.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// searchFlags carries the parsed flags for `logmind search`.
type searchFlags struct {
	caseSensitive bool
	noArchive     bool
	noContext     bool
	contextLines  int
}

// searchResult mirrors Python's SearchResult dataclass in core/search.py.
// Fields named to match the Python struct field-for-field so the
// formatter ports cleanly.
type searchResult struct {
	File          string
	DecisionTitle string
	LineNumber    int
	MatchedLine   string
	ContextBefore []string
	ContextAfter  []string
}

// newSearchCmd wires the `logmind search <query>` subcommand.
//
// Help text mirrors Python's docstring so consumers see identical
// guidance via `--help`.
func newSearchCmd() *cobra.Command {
	f := &searchFlags{contextLines: 2}
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search through decision logs for a term or pattern",
		Long: `Search through decision logs for a term or pattern.

Supports regex patterns. By default, search is case-insensitive
and includes archived decisions.

Examples:
    logmind search "postgres"
    logmind search "database.*choice" -c
    logmind search "API" --no-archive`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSearch(cwd, args[0], f, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVarP(&f.caseSensitive, "case-sensitive", "c", false,
		"Perform case-sensitive search")
	cmd.Flags().BoolVar(&f.noArchive, "no-archive", false,
		"Don't search archived decisions")
	cmd.Flags().BoolVar(&f.noContext, "no-context", false,
		"Don't show context lines around matches")
	cmd.Flags().IntVarP(&f.contextLines, "context-lines", "C", 2,
		"Number of context lines to show (default: 2)")
	return cmd
}

// runSearch implements the search command. Mirrors Python's `def search()`
// at cli.py:1355-1417.
func runSearch(cwd, query string, f *searchFlags, stdout, stderr io.Writer) error {
	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		fmt.Fprintln(stdout, "Error: docs/ directory not found. Run 'logmind init' first.")
		return ErrSilent
	}

	results, err := searchDecisions(query, docsPath, f.caseSensitive, !f.noArchive, f.contextLines)
	if err != nil {
		fmt.Fprintf(stdout, "Error during search: %v\n", err)
		return ErrSilent
	}

	if len(results) == 0 {
		fmt.Fprintf(stdout, "No matches found for: %s\n", query)
		return nil
	}

	// Show result count
	matchWord := "matches"
	if len(results) == 1 {
		matchWord = "match"
	}
	fmt.Fprintf(stdout, "Found %d %s for: %s\n", len(results), matchWord, query)
	fmt.Fprintln(stdout)

	// Highlight term in matched line when case-insensitive (matches Python:
	// `highlight_term=query if not case_sensitive else None`).
	highlight := ""
	if !f.caseSensitive {
		highlight = query
	}
	formatted := formatSearchResults(results, !f.noContext, highlight)
	fmt.Fprintln(stdout, formatted)
	return nil
}

// searchDecisions is the engine that drives the file walk + pattern match.
// Direct port of Python core/search.py:search_decisions.
func searchDecisions(query, docsPath string, caseSensitive, includeArchive bool, contextLines int) ([]searchResult, error) {
	// Build file list. Mirror Python's [decisions.md, (decisions-archive.md if include_archive)].
	files := []string{filepath.Join(docsPath, "decisions.md")}
	if includeArchive {
		archive := filepath.Join(docsPath, "decisions-archive.md")
		if pathExists(archive) {
			files = append(files, archive)
		}
	}

	// Compile regex. Falls back to literal escape on compile failure
	// (matches Python's try/except re.error → re.escape).
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	pattern, err := regexp.Compile(flags + query)
	if err != nil {
		// Literal fallback — Python: re.compile(re.escape(query), flags).
		pattern = regexp.MustCompile(flags + regexp.QuoteMeta(query))
	}

	var results []searchResult
	for _, file := range files {
		if !pathExists(file) {
			continue
		}
		fileResults, err := searchFile(file, pattern, contextLines)
		if err != nil {
			return nil, err
		}
		results = append(results, fileResults...)
	}
	return results, nil
}

// searchFile searches a single file for pattern matches with N context
// lines before/after each hit. Direct port of Python core/search.py:_search_file.
func searchFile(filePath string, pattern *regexp.Regexp, contextLines int) ([]searchResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	// Python reads .readlines() which keeps the trailing \n. To match the
	// line-by-line shape, split by \n; the matched_line + context strips
	// the trailing newline (Python `.rstrip("\n")`).
	lines := strings.Split(string(data), "\n")
	// Trim a trailing empty string from a file ending in \n so we don't
	// emit a phantom blank line (Python's readlines wouldn't carry it).
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	fileName := filepath.Base(filePath)
	currentDecision := ""
	var results []searchResult

	for i, line := range lines {
		// Track current decision section (lines starting with ## ).
		if strings.HasPrefix(line, "## ") {
			// Python: line.strip("# \n") — strip leading hashes and
			// trailing whitespace/newlines. Replicate exactly.
			currentDecision = strings.Trim(line, "# \n")
		}

		// Check if line matches pattern.
		if pattern.MatchString(line) {
			// Get context lines. Python:
			//   start_idx = max(0, i - context_lines)
			//   end_idx   = min(len(lines), i + context_lines + 1)
			start := i - contextLines
			if start < 0 {
				start = 0
			}
			end := i + contextLines + 1
			if end > len(lines) {
				end = len(lines)
			}

			before := append([]string{}, lines[start:i]...)
			after := append([]string{}, lines[i+1:end]...)

			title := currentDecision
			if title == "" {
				title = "Unknown decision"
			}

			results = append(results, searchResult{
				File:          fileName,
				DecisionTitle: title,
				LineNumber:    i + 1, // 1-indexed line numbers
				MatchedLine:   line,
				ContextBefore: before,
				ContextAfter:  after,
			})
		}
	}
	return results, nil
}

// formatSearchResults renders the multi-line per-result block. Direct
// port of Python core/search.py:format_search_results.
//
// Output shape:
//
//	<file> - <decision-title> (line N)
//	------------------------------------
//	  <ctx-before>
//	> <matched-line>      (with >>> hl <<< when highlight set)
//	  <ctx-after>
//
// Results separated by blank lines.
func formatSearchResults(results []searchResult, showContext bool, highlightTerm string) string {
	if len(results) == 0 {
		return "No matches found."
	}

	var lines []string
	// Pre-compile highlight regex once (Python compiles inside the loop;
	// we hoist for clarity. Behavior is identical.)
	var highlight *regexp.Regexp
	if highlightTerm != "" {
		// Python: re.sub(re.escape(term), r">>> \1 <<<", line, IGNORECASE)
		// with capture group. Go's ReplaceAllString uses ${1} syntax.
		highlight = regexp.MustCompile("(?i)(" + regexp.QuoteMeta(highlightTerm) + ")")
	}

	for i, result := range results {
		if i > 0 {
			lines = append(lines, "") // blank line between results
		}

		header := fmt.Sprintf("%s - %s (line %d)", result.File, result.DecisionTitle, result.LineNumber)
		lines = append(lines, header)
		lines = append(lines, strings.Repeat("-", len(header)))

		if showContext {
			for _, ctx := range result.ContextBefore {
				lines = append(lines, "  "+ctx)
			}
		}

		matched := result.MatchedLine
		if highlight != nil {
			matched = highlight.ReplaceAllString(matched, ">>> $1 <<<")
		}
		lines = append(lines, "> "+matched)

		if showContext {
			for _, ctx := range result.ContextAfter {
				lines = append(lines, "  "+ctx)
			}
		}
	}
	return strings.Join(lines, "\n")
}
