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
//
// `docs/reviews/` is the SPEC §6.2 review-writeback path: clud-bug-app
// writes `docs/reviews/PR-<n>.md` files as append-only review telemetry
// (consumed by `logmind sync` per internal/skill/sync.go). Those files
// are never cross-linked from README/AGENTS/docs by design — every PR
// that picks up a review would otherwise fail `check-links` with an
// orphan finding. The directory itself is owned by the App + sync
// pipeline, so we exclude the whole prefix (matching the
// `docs/decisions-branches/` precedent) rather than glob just `PR-*.md`.
var DefaultAllowOrphans = []string{
	"docs/decisions.md",
	"docs/decisions-archive.md",
	"docs/file-structure.md",
	"docs/timeline-archive.md",
	"docs/decisions-branches/",
	"docs/reviews/",
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

// Finding is one orphan or broken-link entry enriched with a heuristic
// SuggestedFix string the agent (or PR-comment workflow) can act on
// without re-deriving the fix logic. The plain `[]string` slices
// returned by `Check()` stay byte-identical to the Python output for
// backward compatibility; callers that want the agent-readable report
// switch to `CheckWithReport()` instead. Added in v1.2.0 per plan §8.7
// (Layer 1 + Layer 3 self-healing).
type Finding struct {
	// Path is the repo-relative POSIX path of the markdown file with
	// the problem. For broken-link findings, this is the source file
	// containing the dead link; for orphans, this is the unlinked file.
	Path string `json:"path"`

	// Reason is a single-line human-readable description of the issue.
	// For broken links it includes the dead target; for orphans it's a
	// short "no parent doc links to it" phrase. Already tense + voice
	// stable so PR-comment renderers can drop it in verbatim.
	Reason string `json:"reason"`

	// SuggestedFix is the heuristic next-step the agent should take.
	// Designed to be the SINGLE most likely correct action — not a menu
	// of options. Lines start with `→` so PR comments and terminal
	// output share the same shape:
	//
	//   → run: logmind timeline --write docs/timeline.md
	//   → add a link from AGENTS.md (canonical entry point per logmind convention)
	//
	// Always present; falls back to a generic instruction when no
	// specific heuristic matches.
	SuggestedFix string `json:"suggestedFix"`
}

// CheckReport is the agent-readable view returned by `CheckWithReport()`.
// Mirrors the `Check()` shape (broken + orphans split) but each entry is
// a `Finding` with a `SuggestedFix` attached.
//
// JSON-tagged for the v5 workflow template's mode-B comment path: the
// shell wrapper invokes `logmind check-links --json` and parses this
// struct verbatim.
type CheckReport struct {
	Broken  []Finding `json:"broken"`
	Orphans []Finding `json:"orphans"`
}

// HasIssues returns true when either Broken or Orphans is non-empty.
// Sugar for the retry-loop predicate in `logmind log`'s self-heal
// (`if report.HasIssues() { prompt }`).
func (r CheckReport) HasIssues() bool {
	return len(r.Broken) > 0 || len(r.Orphans) > 0
}

// CheckWithReport runs the same check as `Check()` and returns a
// `CheckReport` carrying agent-readable findings. Fix suggestions are
// heuristic — see `suggestOrphanFix` / `suggestBrokenLinkFix` below for
// the resolution order.
//
// Equivalent to `Check()` for repo discovery, allowlist application,
// and error semantics. The two functions share `Check()`'s walk +
// resolve machinery so the same paths reach the same verdict.
func CheckWithReport(repoRoot string, roots, allowOrphans []string) (CheckReport, error) {
	broken, orphans, err := Check(repoRoot, roots, allowOrphans)
	if err != nil {
		return CheckReport{}, err
	}
	abs, _ := filepath.Abs(repoRoot)
	if resolved, errR := filepath.EvalSymlinks(abs); errR == nil {
		abs = resolved
	}

	report := CheckReport{
		Broken:  make([]Finding, 0, len(broken)),
		Orphans: make([]Finding, 0, len(orphans)),
	}
	for _, line := range broken {
		// Lines come from `Check()` as `"<source>: missing -> <target>"`.
		// Split into source + target so the suggestion heuristic can
		// reason about both.
		source, target := splitBrokenLine(line)
		report.Broken = append(report.Broken, Finding{
			Path:         source,
			Reason:       fmt.Sprintf("broken link → %s", target),
			SuggestedFix: suggestBrokenLinkFix(abs, source, target),
		})
	}
	for _, orphan := range orphans {
		report.Orphans = append(report.Orphans, Finding{
			Path:         orphan,
			Reason:       "no parent doc links to it",
			SuggestedFix: suggestOrphanFix(abs, orphan),
		})
	}
	return report, nil
}

// splitBrokenLine inverts the `"<source>: missing -> <target>"` format
// produced by `Check()`. Defensive: malformed inputs return the whole
// line as the source and an empty target rather than panicking — the
// Finding still surfaces to the user, just with a generic suggestion.
func splitBrokenLine(line string) (source, target string) {
	idx := strings.Index(line, ": missing -> ")
	if idx < 0 {
		return line, ""
	}
	return line[:idx], line[idx+len(": missing -> "):]
}

// suggestBrokenLinkFix picks the most likely actionable next step for a
// broken markdown link. Heuristics, in order:
//
//  1. The target is `docs/decisions-branches/<branch>.md` or
//     `docs/decisions/<branch>.md` referenced FROM `docs/timeline.md` —
//     the timeline is auto-derived and stale; re-run `logmind timeline --write`.
//  2. The target is `docs/file-structure.md` referenced FROM AGENTS.md /
//     README.md — same story, re-run `logmind file-structure --write`.
//  3. Anything else → suggest removing the dead link or restoring the
//     target file.
//
// Returns a `→`-prefixed string ready for terminal + PR-comment display.
func suggestBrokenLinkFix(repoRoot, source, target string) string {
	sourceClean := filepath.ToSlash(source)
	targetClean := filepath.ToSlash(target)

	// Timeline regen heuristic — covers the common Layer 1 case where
	// the user logs a new branch decision but forgets to run
	// `logmind timeline --write`.
	if sourceClean == "docs/timeline.md" &&
		(strings.HasPrefix(targetClean, "docs/decisions-branches/") ||
			strings.HasPrefix(targetClean, "decisions-branches/") ||
			strings.HasPrefix(targetClean, "decisions/")) {
		return "→ run: logmind timeline --write docs/timeline.md"
	}

	// File-structure regen heuristic.
	if strings.HasSuffix(targetClean, "docs/file-structure.md") ||
		targetClean == "file-structure.md" {
		return "→ run: logmind file-structure --write docs/file-structure.md"
	}

	// Generic fallback. Mentions both fix paths so the user picks the
	// right one for their intent — restoring is right when the link is
	// load-bearing; removing is right when it was a typo.
	return fmt.Sprintf("→ remove the dead link or restore %s", target)
}

// suggestOrphanFix picks the nearest parent doc that EXISTS in the repo
// and suggests adding a link from it. Walks up `docs/` then root,
// preferring well-known entry points (README, AGENTS, docs/timeline.md)
// over arbitrary siblings.
//
// orphan is a repo-relative POSIX path (e.g. `docs/install.md`).
func suggestOrphanFix(repoRoot, orphan string) string {
	// Special-cased well-known orphans that have a canonical parent
	// per logmind convention. These run first so the suggestion is
	// concrete rather than directory-walk derived.
	switch filepath.ToSlash(orphan) {
	case "docs/timeline.md":
		return "→ add a link from AGENTS.md (canonical entry point per logmind convention)"
	case "docs/file-structure.md":
		return "→ add a link from AGENTS.md (canonical entry point per logmind convention)"
	}

	// Walk up the directory tree from the orphan's parent looking for a
	// `README.md`. Prefers the closest one (e.g., `docs/install/foo.md`
	// would suggest linking from `docs/install/README.md` if it exists,
	// then `docs/README.md`, then root `README.md`).
	orphanDir := filepath.Dir(orphan)
	dir := orphanDir
	for {
		candidate := filepath.Join(dir, "README.md")
		if dir == "." || dir == "/" {
			candidate = "README.md"
		}
		if _, err := os.Stat(filepath.Join(repoRoot, candidate)); err == nil {
			return fmt.Sprintf("→ add a link from %s (parent doc by path heuristic)",
				filepath.ToSlash(candidate))
		}
		if dir == "." || dir == "" {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// AGENTS.md fallback — every logmind repo has one (init creates it),
	// so this is the safe universal suggestion when no README is in the
	// path.
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	if _, err := os.Stat(agentsPath); err == nil {
		return "→ add a link from AGENTS.md (canonical entry point per logmind convention)"
	}

	// Final fallback — pure shape, no anchor.
	return fmt.Sprintf("→ add a link to %s from a parent doc (README.md / AGENTS.md / docs/timeline.md)",
		filepath.ToSlash(orphan))
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
