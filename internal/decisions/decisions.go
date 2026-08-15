// Package decisions parses logmind decision-log markdown into typed
// entries. The parser is intentionally minimal — it only extracts the
// header line (`## YYYY-MM-DD HH:MM - <title>`); body content stays in
// the source file and is dereferenced lazily by downstream readers.
//
// Mirrors src/logmind/core/parser.py (the DECISION_HEADER regex + the
// "skip malformed but keep going" error policy).
//
// The package also exposes Collect(), the multi-source aggregator that reads
// every NonBranchSources() file (decisions.md + decisions-archive.md) plus
// decisions-branches/*.md and returns a unified, source-tagged slice. The
// timeline subcommand consumes that slice directly.
//
// SplitRaw / SplitRawBytes expose the same header boundaries as byte
// ranges rather than just parsed fields — the primitive for moving an entry
// between files (the §3.2 migration of a legacy main log into
// docs/decisions-branches/main.md) without ever re-rendering it.
package decisions

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Entry is one parsed decision header.
type Entry struct {
	Date        time.Time
	Title       string
	SourcePath  string
	SourceLabel string
}

// NonBranchSource names a decision file that the SPEC §3.2 branch-file layout
// does not name after a branch. File is relative to docsPath; Label is the
// §3.2 source grammar token ("main" | "archive") a reader tags its entries
// with.
type NonBranchSource struct {
	File  string
	Label string
}

// NonBranchSources returns, in deterministic read order, every decision file
// that lives outside docs/decisions-branches/ and that EVERY read path
// (Collect, the timeline's collectMarked, `search`, `show --all`) MUST read.
//
// This is the single owner of that list. A read path that hardcodes one of
// these filenames instead of ranging over this function is the bug class this
// exists to make unrepresentable: §3.2 collapsed the layout by dropping
// docs/decisions-archive.md from four read paths independently, and a repo
// that had rotated under the old `max_recent: 20` default silently lost every
// archived decision from `search`, `show --all`, and the timeline.
//
//   - decisions.md — the pre-§3.2 main log. Still WRITTEN, but only where no
//     branch NAME exists to name a file after: a non-git directory, a
//     detached HEAD, an unborn repo, or decisions.branch_aware explicitly off
//     (resolveDecisionsPath in internal/cli/log.go routes those here). It is
//     no longer where a decision made ON the default branch goes — that is
//     docs/decisions-branches/main.md like any other branch.
//   - decisions-archive.md — the pre-§3.2 rotation overflow, written by the
//     retired `max_recent` cap. NOTHING writes it now, in any state. It is
//     read-only legacy: a repo that rotated before upgrading keeps every
//     archived decision findable. Nothing here migrates its contents into
//     main.md — rewriting a user-owned artifact is not this code's business
//     (SPEC line 1101); the read paths simply surface it where it lies.
//
// Order is the read order callers append in, so output stays deterministic.
func NonBranchSources() []NonBranchSource {
	return []NonBranchSource{
		{File: "decisions.md", Label: "main"},
		{File: "decisions-archive.md", Label: "archive"},
	}
}

// decisionHeader mirrors Python's DECISION_HEADER regex
// (core/parser.py:9). Captures:
//
//	1: date  (YYYY-MM-DD)
//	2: time  (HH:MM)
//	3: title (free-form, ends at line break)
var decisionHeader = regexp.MustCompile(`^## (\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}) - (.+)$`)

// Iter reads `path` and emits every decision header found in it.
//
// Mirrors Python's iter_decisions:
//   - missing file → no entries, no error (expected for optional branch files)
//   - header matched structurally but date/time parse fails → stderr
//     warning + skip (the entry is "loud-dropped" rather than silently
//     ignored, matching the "fail-safe → loud rather than silent" RTK
//     comment in parser.py:18)
//
// The returned slice preserves the file's on-disk order. Callers that
// need newest-first ordering sort after the fact.
//
// stderr is supplied as a writer so tests can capture warnings.
func Iter(path string, stderr io.Writer) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	if stderr == nil {
		stderr = os.Stderr
	}

	var out []Entry
	scanner := bufio.NewScanner(f)
	// Decision headers are short — but the default 64KB scanner buffer
	// is too small if some upstream tool dumped a huge log line. Match
	// Python's "read whole file" robustness with a 1 MiB max line.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		m := decisionHeader.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		dateStr := m[1] + " " + m[2]
		// `time.Parse` is the Go analog of strptime("%Y-%m-%d %H:%M").
		// On parse failure, mirror the Python warn-and-skip path.
		t, perr := time.Parse("2006-01-02 15:04", dateStr)
		if perr != nil {
			fmt.Fprintf(stderr, "  ! logmind: skipping malformed decision header at %s:%d: %v\n", path, lineno, perr)
			continue
		}
		out = append(out, Entry{
			Date:  t,
			Title: m[3],
		})
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// RawEntry pairs a parsed Entry with the exact bytes of the entry block it
// was parsed from — from its "## YYYY-MM-DD HH:MM - <title>" header line
// through (and including) everything up to the next entry's header line, or
// EOF for the last entry in the file. RawEntry.Raw is the entry exactly as
// it sits on disk, so a caller that re-emits it (`show`, the §3.2 migration
// of a legacy main log into a branch file) does so byte-for-byte — it is
// never re-rendered.
type RawEntry struct {
	Entry
	Raw string
}

// SplitRaw reads path and calls SplitRawBytes on its content. Missing file →
// ("", nil, nil), matching Iter's "no file, no entries, no error" contract.
func SplitRaw(path string) (preamble string, entries []RawEntry, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, err
	}
	preamble, entries = SplitRawBytes(string(data))
	return preamble, entries, nil
}

// SplitRawBytes splits already-loaded decisions-file content into a leading
// preamble (the file's own top-of-file header block, or the whole content
// when no entry header is found) and the entries that follow it, in on-disk
// order. Every decision file is append-only (§3.2), so on-disk order is
// oldest-first.
//
// Boundaries are found by scanning line-by-line for the same decisionHeader
// pattern Iter uses (so a header this misses, Iter misses too, and vice
// versa): each entry runs from its "## ..." line through the byte
// immediately before the next such line, or EOF. This deliberately does NOT
// special-case the §1.6.3 `<!-- logmind-entry-start: ... -->` marker block
// that opens a branch decision file: decisionHeader only ever matches a
// literal "## " line prefix, so a marker comment can never be mistaken for a
// decision entry — it simply rides along inside the preamble (or, on a
// legacy file, inside whichever entry currently precedes it). Branch files
// are never rotated (SPEC §1.4: "Branch decision files have no capacity cap
// (no archive overflow)"), so this distinction mostly matters for callers
// that reuse SplitRawBytes against a branch file for other purposes.
func SplitRawBytes(content string) (preamble string, entries []RawEntry) {
	type header struct {
		offset int
		entry  Entry
	}
	var headers []header
	off := 0
	for _, line := range strings.SplitAfter(content, "\n") {
		if line == "" {
			continue
		}
		trimmed := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if m := decisionHeader.FindStringSubmatch(trimmed); m != nil {
			if t, perr := time.Parse("2006-01-02 15:04", m[1]+" "+m[2]); perr == nil {
				headers = append(headers, header{offset: off, entry: Entry{Date: t, Title: m[3]}})
			}
			// Malformed date/time → skipped, same as Iter. SplitRawBytes is a
			// structural split, not a diagnostic pass, so it doesn't repeat
			// Iter's stderr warning here — a caller that wants that warning
			// runs Iter too.
		}
		off += len(line)
	}
	if len(headers) == 0 {
		return content, nil
	}
	preamble = content[:headers[0].offset]
	entries = make([]RawEntry, 0, len(headers))
	for i, h := range headers {
		end := len(content)
		if i+1 < len(headers) {
			end = headers[i+1].offset
		}
		entries = append(entries, RawEntry{Entry: h.entry, Raw: content[h.offset:end]})
	}
	return preamble, entries
}

// branchLabelFromFilename reverses logger._sanitize_branch's escaping.
//
// Mirror of Python core/timeline.py _branch_label_from_filename:
//   - strip ".md" suffix
//   - replace "__" with "/"
//
// Imperfect (the original sanitize step also catches `\` and `:`) but
// covers the 99% case of feat__auth → feat/auth.
func branchLabelFromFilename(name string) string {
	stem := strings.TrimSuffix(name, ".md")
	return strings.ReplaceAll(stem, "__", "/")
}

// Collect walks the canonical logmind sources under docsPath and
// returns every entry, sorted newest-first.
//
// Sources walked (Collect itself never writes):
//
//	docs/decisions.md                    → source_label="main"
//	docs/decisions-archive.md            → source_label="archive"
//	docs/decisions-branches/<branch>.md  → source_label="<branch>"
//
// The first two come from NonBranchSources() — see it for why each is still
// read and which of them is still written.
//
// Missing files are tolerated; callers get whatever exists.
func Collect(docsPath string, stderr io.Writer) ([]Entry, error) {
	if stderr == nil {
		stderr = os.Stderr
	}
	var out []Entry

	for _, src := range NonBranchSources() {
		entries, err := Iter(filepath.Join(docsPath, src.File), stderr)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			e.SourcePath = src.File
			e.SourceLabel = src.Label
			out = append(out, e)
		}
	}

	branchesDir := filepath.Join(docsPath, "decisions-branches")
	branchFiles, err := ListBranchFiles(branchesDir)
	if err != nil {
		return nil, err
	}
	for _, bf := range branchFiles {
		base := filepath.Base(bf)
		entries, err := Iter(bf, stderr)
		if err != nil {
			return nil, err
		}
		label := branchLabelFromFilename(base)
		rel := "decisions-branches/" + base
		for _, e := range entries {
			e.SourcePath = rel
			e.SourceLabel = label
			out = append(out, e)
		}
	}

	// Newest-first, matching Python collect_entries (timeline.py:112).
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Date.After(out[j].Date)
	})
	return out, nil
}

// ListBranchFiles returns the sorted set of <docsPath>/decisions-branches/*.md
// paths. Mirrors Python sorted(branches_dir.glob("*.md")).
func ListBranchFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	// Sort explicitly to match Python's sorted(.glob(...)) precisely.
	sort.Strings(out)
	return out, nil
}
