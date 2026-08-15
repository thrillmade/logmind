package writeaudit

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestNoUnauthorizedRawWriteFile is the guard. It scans the whole module
// for direct os.WriteFile / ioutil.WriteFile calls in non-test files and
// requires every one of them to be declared in Allowlist with a reason.
//
// Three ways to go red, all deliberate:
//
//   - a raw call in a file that is not allowlisted at all → new debt;
//   - MORE raw calls in an allowlisted file than declared → debt grew
//     inside a file whose entry made it look accounted-for;
//   - FEWER than declared → the entry is stale and must be deleted, so the
//     ledger cannot rot into a blanket permission after the owning PR lands.
func TestNoUnauthorizedRawWriteFile(t *testing.T) {
	root := repoRoot(t)

	findings, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan(%s): %v", root, err)
	}

	byFile := map[string][]Finding{}
	for _, f := range findings {
		byFile[f.File] = append(byFile[f.File], f)
	}

	for file, got := range byFile {
		ex, ok := Allowlist[file]
		if !ok {
			t.Errorf(`%s calls %s directly at %s.

os.WriteFile follows symlinks: a dangling symlink planted at that path
makes os.Stat/os.ReadFile report "absent", and this write then lands
OUTSIDE the repository, through the link.

Route it through internal/atomicio.WriteFile (temp sibling + rename onto
the destination name). If write-through is genuinely correct at this site,
add an entry to Allowlist in internal/writeaudit/writeaudit.go saying why.`,
				file, got[0].Call, lines(got))
			continue
		}
		switch {
		case len(got) > ex.Count:
			t.Errorf(`%s has %d raw write call(s) at %s but the allowlist permits %d.

A new one was added to a file that already had an exception. The existing
exception does not cover it — read the recorded reason and route the new
call through internal/atomicio.WriteFile:

  %s`, file, len(got), lines(got), ex.Count, ex.Reason)
		case len(got) < ex.Count:
			t.Errorf(`%s has %d raw write call(s) at %s but the allowlist still claims %d.

Stale entry. Lower the count, or delete the entry outright if the file is
now clean. Recorded reason:

  %s`, file, len(got), lines(got), ex.Count, ex.Reason)
		}
	}

	for file, ex := range Allowlist {
		if _, still := byFile[file]; still {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); os.IsNotExist(err) {
			t.Errorf("allowlist names %s, which no longer exists — delete the entry.\n\n  %s", file, ex.Reason)
			continue
		}
		t.Errorf(`%s no longer contains any raw write call, but is still allowlisted.

The debt was paid; delete the entry so the next raw call in this file is
caught. Recorded reason:

  %s`, file, ex.Reason)
	}
}

// TestAllowlistEntriesCarryAReason keeps the allowlist honest: an entry
// with an empty or one-word reason is a rubber stamp, and the whole point
// of the list is that each exception was argued for.
func TestAllowlistEntriesCarryAReason(t *testing.T) {
	for file, ex := range Allowlist {
		if len(strings.TrimSpace(ex.Reason)) < 40 {
			t.Errorf("allowlist entry %s has no substantive reason (%q); "+
				"say why write-through is correct, or who owns the conversion", file, ex.Reason)
		}
		if ex.Count < 1 {
			t.Errorf("allowlist entry %s has Count=%d; delete the entry instead", file, ex.Count)
		}
	}
}

// TestScan_DetectsAPlantedCall is the control for the guard above. A scan
// that silently returns nothing — wrong root, parse error swallowed, walk
// skipping everything — would make TestNoUnauthorizedRawWriteFile pass
// vacuously forever. This plants a call Scan MUST find, in a temp tree, and
// checks the surrounding discrimination too: comments mentioning the call,
// _test.go files, and a same-named method on a local variable are all
// non-findings.
func TestScan_DetectsAPlantedCall(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/probe\n\ngo 1.21\n")

	write(t, dir, "guilty.go", `package probe

import "os"

// This comment says os.WriteFile and must NOT be counted.
func guilty() error { return os.WriteFile("/tmp/x", nil, 0o644) }
`)
	write(t, dir, "innocent_test.go", `package probe

import "os"

func helper() error { return os.WriteFile("/tmp/x", nil, 0o644) }
`)
	write(t, dir, "innocent.go", `package probe

// Prose about os.WriteFile following symlinks. Not a call.
type fakeOS struct{}

func (fakeOS) WriteFile(string, []byte, uint32) error { return nil }

func innocent() error {
	var os fakeOS
	return os.WriteFile("/tmp/x", nil, 0o644)
}
`)

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Scan found %d call(s) %v; want exactly 1 (guilty.go). "+
			"0 means the scanner is blind and the repo guard passes vacuously; "+
			">1 means it counts comments, tests, or shadowed identifiers.", len(got), got)
	}
	if got[0].File != "guilty.go" || got[0].Call != "os.WriteFile" {
		t.Errorf("finding = %+v; want guilty.go / os.WriteFile", got[0])
	}
	if got[0].Line != 6 {
		t.Errorf("finding line = %d; want 6 (the call, not the comment above it)", got[0].Line)
	}
}

// TestScan_DetectsIoutilAlias pins the deprecated spelling, the obvious way
// to reintroduce the bug past a ban on `os.WriteFile` alone.
func TestScan_DetectsIoutilAlias(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/probe\n\ngo 1.21\n")
	write(t, dir, "old.go", `package probe

import "io/ioutil"

func old() error { return ioutil.WriteFile("/tmp/x", nil, 0o644) }
`)
	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || got[0].Call != "ioutil.WriteFile" {
		t.Fatalf("Scan found %v; want one ioutil.WriteFile finding", got)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root, err := FindRepoRoot(wd)
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}
	// Control: the guard is worthless if it is pointed at an empty or wrong
	// tree, which would make "no findings" mean "nothing scanned".
	if _, err := os.Stat(filepath.Join(root, "internal", "atomicio", "atomicio.go")); err != nil {
		t.Fatalf("repo root %s does not look like the logmind tree "+
			"(internal/atomicio/atomicio.go missing): %v", root, err)
	}
	return root
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func lines(fs []Finding) string {
	n := make([]int, 0, len(fs))
	for _, f := range fs {
		n = append(n, f.Line)
	}
	sort.Ints(n)
	s := make([]string, 0, len(n))
	for _, ln := range n {
		s = append(s, strconv.Itoa(ln))
	}
	return strings.Join(s, ", ")
}
