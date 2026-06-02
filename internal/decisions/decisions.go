// Package decisions parses logmind decision-log markdown into typed
// entries. The parser is intentionally minimal — it only extracts the
// header line (`## YYYY-MM-DD HH:MM - <title>`); body content stays in
// the source file and is dereferenced lazily by downstream readers.
//
// Mirrors src/logmind/core/parser.py (the DECISION_HEADER regex + the
// "skip malformed but keep going" error policy).
//
// The package also exposes Collect(), the multi-source aggregator that
// reads decisions.md + decisions-archive.md + decisions-branches/*.md
// and returns a unified, source-tagged slice. The timeline subcommand
// consumes that slice directly.
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

// decisionHeader mirrors Python's DECISION_HEADER regex
// (core/parser.py:9). Captures:
//   1: date  (YYYY-MM-DD)
//   2: time  (HH:MM)
//   3: title (free-form, ends at line break)
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
// Sources walked (read-only; never written):
//
//	docs/decisions.md                    → source_label="main"
//	docs/decisions-archive.md            → source_label="archive"
//	docs/decisions-branches/<branch>.md  → source_label="<branch>"
//
// Missing files are tolerated; callers get whatever exists.
func Collect(docsPath string, stderr io.Writer) ([]Entry, error) {
	if stderr == nil {
		stderr = os.Stderr
	}
	var out []Entry

	main := filepath.Join(docsPath, "decisions.md")
	mainEntries, err := Iter(main, stderr)
	if err != nil {
		return nil, err
	}
	for _, e := range mainEntries {
		e.SourcePath = "decisions.md"
		e.SourceLabel = "main"
		out = append(out, e)
	}

	archive := filepath.Join(docsPath, "decisions-archive.md")
	archiveEntries, err := Iter(archive, stderr)
	if err != nil {
		return nil, err
	}
	for _, e := range archiveEntries {
		e.SourcePath = "decisions-archive.md"
		e.SourceLabel = "archive"
		out = append(out, e)
	}

	branchesDir := filepath.Join(docsPath, "decisions-branches")
	branchFiles, err := listBranchFiles(branchesDir)
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

// listBranchFiles returns the sorted set of <docsPath>/decisions-branches/*.md
// paths. Mirrors Python sorted(branches_dir.glob("*.md")).
func listBranchFiles(dir string) ([]string, error) {
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
