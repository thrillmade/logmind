// Package linkcheck verifies that every relative markdown link in
// the project's documentation resolves to an existing file, and that
// no `docs/*.md` file is orphaned (linked to by nothing).
//
// This is a port of src/logmind/actions/link_check.py — every regex,
// every order-of-operations choice, every output string is kept
// byte-identical to v0.6.14 so the Go CLI's `check-links` subcommand
// emits the same report the Python action does.
//
// Important parity points (see commentary in link_check.py):
//
//   - Fenced code blocks (``` ... ```) AND inline-code spans (`...`)
//     are stripped before the link scan. The cost of NOT stripping
//     them is well-documented in PR #83 — markdown that demonstrates
//     `[text](path)` syntax can't be discussed without breaking CI.
//
//   - Code spans use 1+ backticks for the delimiter; the regex
//     requires matching lengths (CommonMark §6.1) but does NOT use
//     re.DOTALL — an unmatched backtick must not eat real broken
//     links across paragraphs.
//
//   - Anchor-only links (`[x](#section)`) and pure external links
//     (`http://`, `https://`, `mailto:`, `ftp://`, `//`) are skipped.
//
//   - Allowlist entries with trailing `/` match directory prefixes;
//     entries without are treated as exact match OR ancestor.
package linkcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// LinkPattern matches a markdown inline link `[text](target)` where
// target has no whitespace and no closing paren. Byte-identical to
// LINK_PATTERN in link_check.py.
var LinkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)

// DefaultAllowOrphans matches DEFAULT_ALLOW_ORPHANS in the Python
// source. `docs/decisions-branches/` is a directory prefix
// (trailing `/`) — any `.md` under it is exempt.
var DefaultAllowOrphans = []string{
	"docs/decisions.md",
	"docs/decisions-archive.md",
	"docs/file-structure.md",
	"docs/decisions-branches/",
}

// DefaultRoots matches DEFAULT_ROOTS in the Python source — the
// files/dirs that the link checker walks by default.
var DefaultRoots = []string{"README.md", "AGENTS.md", "CLAUDE.md", "docs"}

// externalPrefixes mirrors _EXTERNAL_PREFIXES — these target prefixes
// are skipped entirely (external links aren't filesystem-resolvable).
var externalPrefixes = []string{"http://", "https://", "mailto:", "ftp://", "//", "#"}

// fencedCodeBlockRE strips multi-line ``` ... ``` blocks. The Python
// uses re.DOTALL | re.MULTILINE — Go uses (?ms) for the same effect.
var fencedCodeBlockRE = regexp.MustCompile("(?ms)^```.*?^```")

// Inline-code-span stripping: the Python source uses a regex with a
// backreference to enforce matching delimiter length:
//
//	r"(`+)(?:(?!\1)[^\n])+?\1"
//
// Go's regexp (RE2) does NOT support backreferences, so the Go port
// implements the equivalent semantic in stripInlineCodeSpans below
// (manual byte-walk: find a run of N backticks, scan to the first
// matching run of EXACTLY N backticks on the same line, replace the
// span with whitespace). No package-level regex variable is needed
// — the manual walker is the source of truth.

// Check runs the link integrity check against repoRoot.
//
// Returns sorted slices of broken-link reports and orphan paths. The
// broken-list format matches the Python `"<source>: missing -> <target>"`
// string layout; the orphan list is repo-relative POSIX paths.
//
// Empty `roots` falls back to DefaultRoots; empty `allowOrphans`
// falls back to DefaultAllowOrphans. Callers wanting "no allowlist"
// should pass a non-nil empty slice — matching the Python
// `allow_orphans=()` behaviour.
func Check(repoRoot string, roots, allowOrphans []string) (broken, orphans []string, err error) {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, nil, err
	}
	// Resolve symlinks once so .resolve()-vs-Path math in the Python
	// version (which uses Path.resolve()) is mirrored consistently.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}

	if roots == nil {
		roots = DefaultRoots
	}
	if allowOrphans == nil {
		allowOrphans = DefaultAllowOrphans
	}

	absRoots := make([]string, len(roots))
	for i, r := range roots {
		absRoots[i] = filepath.Join(abs, r)
	}

	mdFiles := collectMDFiles(absRoots)

	// incoming[abs-path] = set of source files that link to it. Used
	// to identify orphans (markdown files under docs/ that no other
	// tracked file links to).
	incoming := make(map[string]map[string]struct{}, len(mdFiles))
	for _, p := range mdFiles {
		incoming[p] = make(map[string]struct{})
	}

	for _, source := range mdFiles {
		for _, link := range extractLinks(source) {
			target := link.target
			if isExternal(target) {
				continue
			}
			stripped := stripAnchor(target)
			if stripped == "" {
				// Pure-anchor link (`#section`); skip.
				continue
			}
			// Resolve relative to the source file's directory.
			joined := filepath.Join(filepath.Dir(source), stripped)
			// Mirror Path.resolve(): follow symlinks; absolutise.
			resolved, errResolve := filepath.Abs(joined)
			if errResolve == nil {
				if real, e := filepath.EvalSymlinks(resolved); e == nil {
					resolved = real
				}
			}
			if _, statErr := os.Stat(resolved); statErr != nil {
				rel, err := filepath.Rel(abs, source)
				if err != nil {
					rel = source
				}
				rel = filepath.ToSlash(rel)
				broken = append(broken, fmt.Sprintf("%s: missing -> %s", rel, target))
				continue
			}
			if filepath.Ext(resolved) == ".md" {
				if _, tracked := incoming[resolved]; tracked {
					incoming[resolved][source] = struct{}{}
				}
			}
		}
	}

	for md, sources := range incoming {
		rel, err := filepath.Rel(abs, md)
		if err != nil {
			continue
		}
		relPosix := filepath.ToSlash(rel)
		parts := strings.Split(relPosix, "/")
		if len(parts) == 0 || parts[0] != "docs" {
			continue
		}
		if isAllowedOrphan(relPosix, allowOrphans) {
			continue
		}
		if len(sources) == 0 {
			orphans = append(orphans, relPosix)
		}
	}

	sort.Strings(broken)
	sort.Strings(orphans)
	return broken, orphans, nil
}

// FormatReport renders the human-readable report. Byte-identical to
// link_check.format_report when `broken` and `orphans` carry the
// same data.
func FormatReport(broken, orphans []string) string {
	var lines []string
	if len(broken) > 0 {
		lines = append(lines, fmt.Sprintf("Broken links (%d):", len(broken)))
		for _, b := range broken {
			lines = append(lines, "  - "+b)
		}
	}
	if len(orphans) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, fmt.Sprintf("Orphan markdown files (%d):", len(orphans)))
		for _, o := range orphans {
			lines = append(lines, "  - "+o)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "All markdown links resolve and no orphans found.")
	}
	return strings.Join(lines, "\n")
}

// --- internal helpers ----------------------------------------------------

func isExternal(target string) bool {
	for _, p := range externalPrefixes {
		if strings.HasPrefix(target, p) {
			return true
		}
	}
	return false
}

func stripAnchor(target string) string {
	if i := strings.Index(target, "#"); i >= 0 {
		return target[:i]
	}
	return target
}

// collectMDFiles mirrors _collect_md_files: walks each root; files
// ending in `.md` are added directly, directories are recursed into.
func collectMDFiles(roots []string) []string {
	seen := make(map[string]struct{})
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if filepath.Ext(root) == ".md" {
				if resolved, err := resolvePath(root); err == nil {
					seen[resolved] = struct{}{}
				}
			}
			continue
		}
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".md" {
				return nil
			}
			if resolved, err := resolvePath(path); err == nil {
				seen[resolved] = struct{}{}
			}
			return nil
		})
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	return abs, nil
}

type extractedLink struct {
	text   string
	target string
}

func extractLinks(mdPath string) []extractedLink {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return nil
	}
	cleaned := stripCodeRegions(string(data))
	matches := LinkPattern.FindAllStringSubmatch(cleaned, -1)
	out := make([]extractedLink, 0, len(matches))
	for _, m := range matches {
		out = append(out, extractedLink{text: m[1], target: m[2]})
	}
	return out
}

// stripCodeRegions returns text with fenced blocks and inline-code
// spans replaced by whitespace (preserving newlines so line numbers
// in error reports stay correct).
//
// Fenced first (multi-line, can swallow stray backticks), then
// inline spans.
func stripCodeRegions(text string) string {
	text = fencedCodeBlockRE.ReplaceAllStringFunc(text, toWhitespace)
	text = stripInlineCodeSpans(text)
	return text
}

// stripInlineCodeSpans is the manual equivalent of the Python
// `(?P<delim>`+)(?:(?!\1)[^\n])+?\1` regex. Go's stdlib regexp
// doesn't support backreferences, so we walk the string by hand.
//
// Algorithm: find a run of N>=1 backticks, then scan forward for
// the FIRST run of EXACTLY N backticks on the same line — replacing
// everything between (inclusive) with spaces. Newlines terminate
// the search without consuming, matching the Python
// "no re.DOTALL" choice. Whitespace preservation keeps line offsets
// stable for error reporting.
func stripInlineCodeSpans(text string) string {
	b := []byte(text)
	out := make([]byte, 0, len(b))
	i := 0
	for i < len(b) {
		if b[i] != '`' {
			out = append(out, b[i])
			i++
			continue
		}
		// Found start of a backtick run.
		runStart := i
		for i < len(b) && b[i] == '`' {
			i++
		}
		runLen := i - runStart
		// Search forward on the same line for a matching run.
		j := i
		matched := -1
		for j < len(b) && b[j] != '\n' {
			if b[j] != '`' {
				j++
				continue
			}
			closeStart := j
			for j < len(b) && b[j] == '`' {
				j++
			}
			closeLen := j - closeStart
			if closeLen == runLen {
				matched = j // end of the closing run
				break
			}
		}
		if matched < 0 {
			// No closing run — emit the opening backticks verbatim
			// and resume normal scanning.
			out = append(out, b[runStart:i]...)
			continue
		}
		// Replace the whole span [runStart, matched) with spaces;
		// preserve any newlines (there are none here because we
		// stopped scanning at \n, but the Python source preserves
		// them — keep the loop literal in case future relaxation
		// changes the rules).
		for k := runStart; k < matched; k++ {
			if b[k] == '\n' {
				out = append(out, '\n')
			} else {
				out = append(out, ' ')
			}
		}
		i = matched
	}
	return string(out)
}

func toWhitespace(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out[i] = '\n'
		} else {
			out[i] = ' '
		}
	}
	return string(out)
}

func isAllowedOrphan(relPath string, allowOrphans []string) bool {
	for _, entry := range allowOrphans {
		e := strings.TrimRight(entry, "/")
		if relPath == e {
			return true
		}
		if strings.HasPrefix(relPath, e+"/") {
			return true
		}
	}
	return false
}
