package writeaudit

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestNoUnauthorizedRawWriteFile is the guard. It scans the whole module for
// direct calls to the creating/truncating primitives in non-test files and
// requires every one of them to be declared in Allowlist, by identity, with
// a reason.
//
// Three ways to go red, all deliberate:
//
//   - a raw call in a file that is not allowlisted at all → new debt;
//   - a raw call in an allowlisted file at an identity the entry does not
//     name → either a new call, or the judged-keep call was MOVED (e.g. into
//     a general-purpose helper, which is how a single exception becomes a
//     module-wide one);
//   - an identity the entry names that is no longer present → the entry is
//     stale and must be deleted, so the ledger cannot rot into a blanket
//     permission after the owning PR lands.
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

%s follows symlinks: a dangling symlink planted at that path makes
os.Stat/os.ReadFile report "absent", and this write then lands OUTSIDE the
repository, through the link.

Route it through internal/atomicio.WriteFile (temp sibling + rename onto the
destination name, symlink refused). If the raw call is genuinely correct at
this site, add an entry to Allowlist in internal/writeaudit/writeaudit.go
naming the site ("%s") and saying why.

SCOPE REMINDER: this guard catches os.WriteFile, ioutil.WriteFile, os.Create,
and os.OpenFile with O_CREATE/O_TRUNC, under any import spelling. It does NOT
catch writes through an *os.File you already hold (template.Execute,
json.Encoder, io.Copy, f.Write) or a shell-out. It is not a sandbox — it
catches the ACCIDENTAL reintroduction, which is the one that keeps happening.`,
				file, got[0].Call, lines(got), got[0].Call, got[0].Site())
			continue
		}
		missing, extra := diffSites(ex.Sites, sites(got))
		if len(extra) > 0 {
			t.Errorf(`%s has raw write call(s) the allowlist does not cover: %s
(all raw calls in this file: %s at line(s) %s)

Either a new one was added to a file that already had an exception, or the
allowlisted call MOVED to a different function — which is exactly what the
identity check exists to catch, because relocating a judged keep into a
general-purpose helper turns one exception into a module-wide one.

Route the new call through internal/atomicio.WriteFile, or update the entry.
Recorded reason:

  %s`, file, strings.Join(extra, ", "), strings.Join(sites(got), ", "), lines(got), ex.Reason)
		}
		if len(missing) > 0 {
			t.Errorf(`%s no longer contains allowlisted call site(s): %s

Stale entry. Remove those identities, or delete the entry outright if the
file is now clean. Recorded reason:

  %s`, file, strings.Join(missing, ", "), ex.Reason)
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
				"say why the raw call is correct, or who owns the conversion", file, ex.Reason)
		}
		if len(ex.Sites) == 0 {
			t.Errorf("allowlist entry %s names no sites; delete the entry instead", file)
		}
		for _, s := range ex.Sites {
			if !strings.Contains(s, ":") {
				t.Errorf("allowlist entry %s has malformed site %q; want \"<func>:<pkg>.<Fn>\"", file, s)
			}
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
	if got[0].Site() != "guilty:os.WriteFile" {
		t.Errorf("site = %q; want %q — the allowlist identity is the enclosing function", got[0].Site(), "guilty:os.WriteFile")
	}
}

// TestScan_DefeatsTheSpellingDodges is the HIGH-3 regression. Every case
// below was demonstrated to compile and pass the previous guard, which only
// recognised an *ast.SelectorExpr whose X was literally named "os".
//
// Each case is its own file in one probe module so a single Scan proves all
// of them at once, and each is asserted by SITE, so a scanner that finds the
// right number of calls in the wrong places still fails.
func TestScan_DefeatsTheSpellingDodges(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/probe\n\ngo 1.21\n")

	write(t, dir, "alias.go", `package probe

import w "os"

func viaAlias() error { return w.WriteFile("/tmp/x", nil, 0o644) }
`)
	write(t, dir, "dotimport.go", `package probe

import . "os"

func viaDotImport() error { return WriteFile("/tmp/x", nil, 0o644) }
`)
	write(t, dir, "openfile.go", `package probe

import "os"

func viaOpenFile() error {
	f, err := os.OpenFile("/tmp/x", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
`)
	// O_CREATE with NO O_TRUNC — the exact shape of
	// internal/cli/filelock_unix.go, which the previous guard did not flag.
	// Kept as its own case because a probe that carries both flags cannot
	// tell whether the O_CREATE arm or the O_TRUNC arm did the catching.
	write(t, dir, "createonly.go", `package probe

import "os"

func viaCreateOnly() error {
	f, err := os.OpenFile("/tmp/lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
`)
	// O_TRUNC with NO O_CREATE — truncating an existing path still follows a
	// symlink and still destroys whatever is at the far end.
	write(t, dir, "truncateonly.go", `package probe

import "os"

func viaTruncOnly() error {
	f, err := os.OpenFile("/tmp/x", os.O_TRUNC|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return f.Close()
}
`)
	write(t, dir, "create.go", `package probe

import "os"

func viaCreate() error {
	f, err := os.Create("/tmp/x")
	if err != nil {
		return err
	}
	return f.Close()
}
`)
	// os.Truncate creates nothing, so it cannot be the dangling-symlink
	// primitive — but it resolves the path and destroys whatever is at the
	// far end of a link, which is the same damage. Taken from PR #306's
	// primitive set when the two guards were unified into this one.
	write(t, dir, "truncate.go", `package probe

import "os"

func viaTruncate() error { return os.Truncate("/tmp/x", 0) }
`)
	write(t, dir, "dynamicflags.go", `package probe

import "os"

func viaComputedFlags(flags int) error {
	f, err := os.OpenFile("/tmp/x", flags, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
`)
	// NOT a finding: opening an existing file read-only neither creates
	// nor truncates, so the guard must not cry wolf about it.
	write(t, dir, "readonly.go", `package probe

import "os"

func readOnly() error {
	f, err := os.OpenFile("/tmp/x", os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	return f.Close()
}
`)

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	gotSites := map[string]string{} // site -> file
	for _, f := range got {
		gotSites[f.Site()] = f.File
	}
	want := map[string]string{
		"viaAlias:os.WriteFile":        "alias.go",
		"viaDotImport:os.WriteFile":    "dotimport.go",
		"viaOpenFile:os.OpenFile":      "openfile.go",
		"viaCreateOnly:os.OpenFile":    "createonly.go",
		"viaTruncOnly:os.OpenFile":     "truncateonly.go",
		"viaCreate:os.Create":          "create.go",
		"viaTruncate:os.Truncate":      "truncate.go",
		"viaComputedFlags:os.OpenFile": "dynamicflags.go",
	}
	for site, file := range want {
		if gotSites[site] != file {
			t.Errorf("Scan missed %s in %s; a raw write spelled this way slips past the guard. got=%v",
				site, file, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("Scan found %d finding(s) %v; want exactly %d — an extra one means "+
			"the read-only os.OpenFile was flagged, which would make the guard noise", len(got), got, len(want))
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

// TestScan_ReportsMethodReceiverInSite pins the identity format for methods,
// so an allowlist entry for a method is writable and stable.
func TestScan_ReportsMethodReceiverInSite(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/probe\n\ngo 1.21\n")
	write(t, dir, "method.go", `package probe

import "os"

type Store struct{ path string }

func (s *Store) Save(b []byte) error { return os.WriteFile(s.path, b, 0o644) }
`)
	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || got[0].Site() != "Store.Save:os.WriteFile" {
		t.Fatalf("Scan found %v; want one finding with site Store.Save:os.WriteFile", got)
	}
}

// TestScan_RelocationChangesTheSite is the HIGH-3b regression: moving an
// allowlisted call into a general-purpose helper must not keep the ledger
// green. Under a count-based allowlist this was invisible — one call before,
// one call after.
func TestScan_RelocationChangesTheSite(t *testing.T) {
	before := scanOne(t, `package probe

import "os"

func installHook(p string, b []byte) error { return os.WriteFile(p, b, 0o755) }
`)
	after := scanOne(t, `package probe

import "os"

func RawWriteFile(p string, b []byte, m os.FileMode) error { return os.WriteFile(p, b, m) }

func installHook(p string, b []byte) error { return RawWriteFile(p, b, 0o755) }
`)
	if before.Site() == after.Site() {
		t.Fatalf("relocating the call into a helper left the site unchanged (%q); "+
			"the allowlist would stay green while the raw write became callable "+
			"from anywhere in the module", before.Site())
	}
	if after.Site() != "RawWriteFile:os.WriteFile" {
		t.Errorf("after relocation site = %q; want RawWriteFile:os.WriteFile", after.Site())
	}
}

// TestScan_DetectsFunctionValueLaundering is the HIGH-1 regression. Storing
// a banned primitive in a variable and calling THAT compiles and, before
// this fix, was invisible: resolveCallee only inspected an *ast.CallExpr's
// Fun, so the assignment itself was never looked at and the subsequent call
// through the variable resolves to nothing this scanner recognises. The
// idiom is not hypothetical — internal/skill/sync.go:693 does exactly this
// shape with the SAFE primitive (`var atomicWriteFile = atomicio.WriteFile`)
// so a copy-paste with the unsafe one is one keystroke away.
//
// Both the package-level var form and the local short-variable form are
// probed, in separate files so each is asserted by site independently.
func TestScan_DetectsFunctionValueLaundering(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/probe\n\ngo 1.21\n")

	write(t, dir, "packagevar.go", `package probe

import "os"

var rawWrite = os.WriteFile

func viaPackageVar(p string, b []byte) error { return rawWrite(p, b, 0o644) }
`)
	write(t, dir, "localvar.go", `package probe

import "os"

func viaLocalVar(p string, b []byte) error {
	w := os.WriteFile
	return w(p, b, 0o644)
}
`)

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	gotSites := map[string]string{}
	for _, f := range got {
		gotSites[f.Site()] = f.File
	}
	want := map[string]string{
		"<file>:os.WriteFile":      "packagevar.go", // top-level var decl, no enclosing func
		"viaLocalVar:os.WriteFile": "localvar.go",
	}
	for site, file := range want {
		if gotSites[site] != file {
			t.Errorf("Scan missed the laundered reference %s in %s (probes rawWrite(...) and w(...) "+
				"must NOT also appear as separate findings — the call through the variable resolves to "+
				"nothing this scanner recognises, and the escape is caught at the assignment). got=%v",
				site, file, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("Scan found %d finding(s) %v; want exactly %d (one per assignment, and the calls "+
			"through rawWrite/w must NOT double-count)", len(got), got, len(want))
	}
}

// TestScan_RejectsUnresolvedOPrefixedFlag is the HIGH-2 regression.
// openFileVerdict used to pass ANY identifier starting with "O_" as a safe
// non-creating flag — so a look-alike name that is not actually one of the
// os package's own flag constants slipped through as "safe" purely because
// of its spelling. This plants exactly that: a local constant that shares
// the O_ prefix but names nothing the os package defines.
func TestScan_RejectsUnresolvedOPrefixedFlag(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/probe\n\ngo 1.21\n")
	write(t, dir, "lookalike.go", `package probe

import "os"

const O_LOOKALIKE = 0

func viaLookalike() error {
	f, err := os.OpenFile("/tmp/x", os.O_RDWR|O_LOOKALIKE, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
`)
	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || got[0].Site() != "viaLookalike:os.OpenFile" {
		t.Fatalf("Scan found %v; want exactly one finding at viaLookalike:os.OpenFile — "+
			"O_LOOKALIKE is not a real os package flag constant, and the prefix-only check "+
			"used to let it (and anything else spelled \"O_*\") pass as safe", got)
	}
}

// TestScan_AcceptsKnownSafeOpenFileFlags is the control for the fix above: a
// combination of GENUINE os package non-creating flags must still pass, or
// the tightened check would make the guard cry wolf on every read-write,
// non-creating os.OpenFile call in the tree (acquireRepoLock's O_RDWR among
// them, once combined with something other than O_CREATE).
func TestScan_AcceptsKnownSafeOpenFileFlags(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/probe\n\ngo 1.21\n")
	write(t, dir, "safe.go", `package probe

import "os"

func viaSafeFlags() error {
	f, err := os.OpenFile("/tmp/x", os.O_RDWR|os.O_APPEND|os.O_SYNC, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
`)
	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Scan found %v; want no findings — O_RDWR|O_APPEND|O_SYNC neither creates nor "+
			"truncates and every name in it is a genuine os package flag constant", got)
	}
}

func scanOne(t *testing.T, src string) Finding {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/probe\n\ngo 1.21\n")
	write(t, dir, "x.go", src)
	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Scan found %d findings %v; want exactly 1", len(got), got)
	}
	return got[0]
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

func sites(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Site())
	}
	sort.Strings(out)
	return out
}

// diffSites compares the allowlisted identities with the observed ones as
// MULTISETS: two permitted calls in the same function are two entries, and
// adding a third is caught.
func diffSites(want, got []string) (missing, extra []string) {
	counts := map[string]int{}
	for _, w := range want {
		counts[w]++
	}
	for _, g := range got {
		counts[g]--
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for i := 0; i < counts[k]; i++ {
			missing = append(missing, k)
		}
		for i := 0; i < -counts[k]; i++ {
			extra = append(extra, k)
		}
	}
	return missing, extra
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
