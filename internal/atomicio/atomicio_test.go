package atomicio

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestWriteFile_CreatesNewFile covers the "file doesn't exist yet" path:
// content and mode land correctly, and no temp file is left behind.
//
// The expected mode is MEASURED against os.WriteFile rather than asserted as
// a literal, because rule 1 is "reproduce os.WriteFile" and os.WriteFile
// hands perm to open(2), where the umask takes bits off it. A hardcoded
// 0o644 here would pass on a 022 machine and fail on a 077 one — and would
// have hidden the very bug this test now pins.
func TestWriteFile_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.json")

	if err := WriteFile(path, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	assertContent(t, path, `{"a":1}`)
	assertMode(t, path, referenceCreateMode(t, 0o644))
	assertNoTmpResidue(t, dir)
}

// TestWriteFile_OverwritesExistingFile is the core guard for the MAJOR
// fix: overwriting an EXISTING file must land the new content in full,
// and leave no temp residue behind.
func TestWriteFile_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("old content"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := WriteFile(path, []byte("new content, longer than old"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	assertContent(t, path, "new content, longer than old")
	assertMode(t, path, 0o644)
	assertNoTmpResidue(t, dir)
}

// TestWriteFile_PreservesExistingModeAndIgnoresPerm is rule 1, and the
// regression for what two review panels found: converting an existing-file
// write from os.WriteFile to atomicio.WriteFile silently RE-PERMISSIONED the
// user's file, because the old implementation chmod'd the temp to perm
// unconditionally. os.WriteFile hands perm to open(2), which ignores it when
// the file already exists — so a 0600 AGENTS.md stayed 0600 under os.WriteFile
// and became world-readable 0644 the moment the call was "made safer".
//
// Mutation note: this fails on a one-character change (dropping the Lstat
// branch in write()), which is the mutation that reintroduces the bug.
func TestWriteFile_PreservesExistingModeAndIgnoresPerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't map cleanly on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil { // defeat the seeding umask
		t.Fatalf("chmod seed: %v", err)
	}

	if err := WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	assertContent(t, path, "new")
	assertMode(t, path, 0o600)

	// And the reference implementation agrees — this is not our own opinion
	// about what "preserve" means.
	ref := filepath.Join(dir, "reference.md")
	if err := os.WriteFile(ref, []byte("old"), 0o600); err != nil {
		t.Fatalf("seed reference: %v", err)
	}
	if err := os.Chmod(ref, 0o600); err != nil {
		t.Fatalf("chmod reference: %v", err)
	}
	if err := os.WriteFile(ref, []byte("new"), 0o644); err != nil {
		t.Fatalf("rewrite reference: %v", err)
	}
	assertMode(t, ref, 0o600)
}

// TestWriteFileMode_AssertsModeOnExistingFile is the other half of rule 1:
// a call site for which the mode IS the point says so, and gets it whether
// the destination existed or not. This is what installHook (executable bit)
// and copyTree (source bits) call.
func TestWriteFileMode_AssertsModeOnExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't map cleanly on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "pre-commit")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho old\n"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod seed: %v", err)
	}

	if err := WriteFileMode(path, []byte("#!/bin/sh\necho new\n"), 0o755); err != nil {
		t.Fatalf("WriteFileMode: %v", err)
	}
	assertContent(t, path, "#!/bin/sh\necho new\n")
	assertMode(t, path, 0o755)
	assertNoTmpResidue(t, dir)
}

// TestWriteFileMode_AssertsModeIgnoringUmaskOnCreate pins that the assert
// path is a chmod, not an open(2) mode — the umask must not take the
// executable bit back off a hook we are creating.
func TestWriteFileMode_AssertsModeIgnoringUmaskOnCreate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't map cleanly on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh-hook")

	if err := WriteFileMode(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFileMode: %v", err)
	}
	assertMode(t, path, 0o755)
}

// TestWriteFile_RenameSeversHardlinks pins rule 3 as a CONTRACT rather than
// leaving it as a surprise. An atomic replace swaps the name, so the
// destination gets a new inode: any hardlink twin keeps pointing at the old
// inode with the OLD content, and os.SameFile stops agreeing.
//
// This is exactly what the panel measured on `logmind agents update --apply`
// (before: links=2, twin updated; after: links=1, twin stale). It is not a
// bug to fix — it is inherent to atomic replace — so it is written down and
// pinned here, and call sites that need the inode preserved are told not to
// use this package.
func TestWriteFile_RenameSeversHardlinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	twin := filepath.Join(dir, "AGENTS-hardlink.md")

	if err := os.WriteFile(path, []byte("shared original\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Link(path, twin); err != nil {
		t.Skipf("hardlinks unavailable on this filesystem: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}
	// Control: prove the twin really is the same inode, or the assertion
	// below proves nothing.
	twinBefore, err := os.Stat(twin)
	if err != nil {
		t.Fatalf("stat twin before: %v", err)
	}
	if !os.SameFile(before, twinBefore) {
		t.Fatalf("hardlink twin is not the same inode; test would be vacuous")
	}

	if err := WriteFile(path, []byte("rewritten\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if os.SameFile(before, after) {
		t.Errorf("%s kept its inode across the write; the package doc promises a rename "+
			"(new inode) and callers are told to expect severed hardlinks — one of the two is now wrong", path)
	}
	twinBody, err := os.ReadFile(twin)
	if err != nil {
		t.Fatalf("read twin: %v", err)
	}
	if string(twinBody) != "shared original\n" {
		t.Errorf("hardlink twin = %q; want the OLD content — rename severs the link, "+
			"and the doc comment on WriteFile says so", twinBody)
	}
}

// TestWriteFile_RefusesSymlinkDestination_ExistingTargetUntouched covers
// the non-dangling case: the destination is a symlink to a real file
// elsewhere. WriteFile must refuse rather than silently deciding between
// the two available (and both wrong) behaviours — following the link and
// clobbering whatever it points to, or swapping the link out for a
// regular file via rename. Neither the link nor its target may change.
func TestWriteFile_RefusesSymlinkDestination_ExistingTargetUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte("original target content"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err := WriteFile(link, []byte("new content"), 0o644)
	if err == nil {
		t.Fatal("WriteFile returned no error for a symlinked destination; want a refusal")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %q; want it to name the symlink so the caller knows what to remove", err)
	}
	// The refusal has to be actionable, not just loud: it names the target
	// so a caller with a DELIBERATE link knows which path to write instead.
	if !strings.Contains(err.Error(), target) {
		t.Errorf("error = %q; want it to name the link target %s so the caller knows how to proceed", err, target)
	}
	if !strings.Contains(err.Error(), "remove the link") {
		t.Errorf("error = %q; want it to say what to do about the link, not only that it exists", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat %s: %v", link, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s was replaced by a regular file; WriteFile must leave a refused symlink exactly as found", link)
	}

	targetContent, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(targetContent) != "original target content" {
		t.Errorf("target content = %q; want unchanged — WriteFile must not follow the link and clobber it", targetContent)
	}
}

// TestWriteFile_RefusesDanglingSymlinkDestination is the exact shape of
// the reported escape: a symlink at the managed path whose target does
// not exist yet. A bare os.WriteFile follows it via open(2)'s O_CREATE
// and lands the body at the target — an arbitrary-write primitive out of
// anything that can plant a symlink in a repo a caller did not write.
// WriteFile must refuse and create nothing at the target.
func TestWriteFile_RefusesDanglingSymlinkDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
	dir := t.TempDir()
	outside := filepath.Join(dir, "..", "escaped.json")
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err := WriteFile(link, []byte("attacker-controlled body"), 0o644)
	if err == nil {
		t.Fatal("WriteFile returned no error for a dangling symlinked destination; want a refusal")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %q; want it to name the symlink so the caller knows what to remove", err)
	}
	if _, statErr := os.Lstat(outside); statErr == nil {
		t.Errorf("WriteFile created %s by following the dangling symlink", outside)
	}
}

// TestRefuseSymlink_DoesNotSeeThroughAnAncestorDirectory pins the STATED
// LIMIT of rule 2 so nobody mistakes it for a containment boundary.
// RefuseSymlink lstats the final component only. When an ancestor directory
// is a symlink — `ln -s /elsewhere repo/.claude`, then `logmind skill new
// sample` — the write resolves through it and lands outside the repo, and
// this package returns nil.
//
// This test asserts the CURRENT behaviour on purpose. It exists so the limit
// is discovered by reading a test rather than by a report: refusing every
// symlinked ancestor would break ordinary setups (a repo under a symlinked
// ~/code; /tmp -> /private/tmp on macOS), and a real fix is a
// resolve-beneath-the-repo-root check at the layer that knows where the repo
// root is, not here. If somebody later adds that check and this test goes
// red, that is the good outcome — delete it and say so.
func TestRefuseSymlink_DoesNotSeeThroughAnAncestorDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{repo, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".claude")); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}

	dest := filepath.Join(repo, ".claude", "skills", "sample", "SKILL.md")
	if err := RefuseSymlink(dest); err != nil {
		t.Fatalf("RefuseSymlink returned %v; the documented behaviour is nil for a symlinked ANCESTOR", err)
	}
	if err := WriteFile(dest, []byte("body"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	landed := filepath.Join(outside, "skills", "sample", "SKILL.md")
	if _, err := os.Lstat(landed); err != nil {
		t.Fatalf("expected the write to land at %s (the documented limit); Lstat: %v", landed, err)
	}
}

// TestWriteFile_PreservesExecutableMode covers the git-hook call site: a
// pre-existing 0o755 hook stays 0o755 across an atomic rewrite. Under rule 1
// this now passes by PRESERVATION rather than by an unconditional chmod —
// which is why installHook, whose whole job is to guarantee the executable
// bit, calls WriteFileMode instead of relying on this.
func TestWriteFile_PreservesExecutableMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post-merge")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho old\n"), 0o755); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod seed: %v", err)
	}

	if err := WriteFile(path, []byte("#!/bin/sh\necho new\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	assertContent(t, path, "#!/bin/sh\necho new\n")
	assertMode(t, path, 0o755)
	assertNoTmpResidue(t, dir)
}

// TestWriteFile_CreatesMissingParentDir mirrors writeFreshSettings'
// .claude/ directory creation — WriteFile must create the parent dir like
// os.MkdirAll + os.WriteFile did before.
func TestWriteFile_CreatesMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "file.md")

	if err := WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	assertContent(t, path, "hello")
}

// TestWriteFile_TempNameIsUnguessable keeps the property the whole
// temp+rename design leans on after os.CreateTemp was replaced with an
// explicit O_EXCL loop: two writes in the same directory must not reuse a
// name an attacker could pre-plant.
func TestWriteFile_TempNameIsUnguessable(t *testing.T) {
	dir := t.TempDir()
	seen := map[string]bool{}
	for i := 0; i < 32; i++ {
		f, name, err := createTemp(dir, "probe", 0o600)
		if err != nil {
			t.Fatalf("createTemp: %v", err)
		}
		_ = f.Close()
		base := filepath.Base(name)
		if seen[base] {
			t.Fatalf("createTemp reused the name %s; the temp path must not be predictable", base)
		}
		seen[base] = true
		if !strings.HasPrefix(base, "probe.tmp-") || len(base) <= len("probe.tmp-")+8 {
			t.Fatalf("temp name %q has no substantial random suffix", base)
		}
	}
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("content = %q; want %q", got, want)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return // POSIX permission bits don't map cleanly on Windows.
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if fi.Mode().Perm() != want {
		t.Errorf("mode = %o; want %o", fi.Mode().Perm(), want)
	}
}

// referenceCreateMode measures what os.WriteFile — the behaviour rule 1
// promises to reproduce — actually produces for perm on a fresh file under
// this process's umask, instead of assuming a umask.
func referenceCreateMode(t *testing.T, perm os.FileMode) os.FileMode {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "umask-probe")
	if err := os.WriteFile(probe, nil, perm); err != nil {
		t.Fatalf("umask probe: %v", err)
	}
	fi, err := os.Stat(probe)
	if err != nil {
		t.Fatalf("stat umask probe: %v", err)
	}
	return fi.Mode().Perm()
}

// assertNoTmpResidue confirms WriteFile cleaned up: no "<base>.tmp-*"
// sibling is left in dir once the rename lands.
func assertNoTmpResidue(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
