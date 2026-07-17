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
func TestWriteFile_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.json")

	if err := WriteFile(path, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	assertContent(t, path, `{"a":1}`)
	assertMode(t, path, 0o644)
	assertNoTmpResidue(t, dir)
}

// TestWriteFile_OverwritesExistingFile is the core guard for the MAJOR
// fix: overwriting an EXISTING file must land the new content in full,
// with the right mode, and leave no temp residue behind.
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

// TestWriteFile_RenameSemantics_DoesNotFollowSymlink proves WriteFile
// actually takes the temp-file-plus-rename path rather than an in-place
// truncate+write in disguise: os.Rename replaces the destination NAME
// itself (swapping a symlink for a regular file) instead of following the
// symlink and writing through to its target — which is exactly what a
// bare os.WriteFile(path, ...) on a symlinked path WOULD do. This is the
// same rename-is-atomic property that makes the technique crash-safe: the
// destination path only ever changes via one atomic filesystem op.
func TestWriteFile_RenameSemantics_DoesNotFollowSymlink(t *testing.T) {
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

	if err := WriteFile(link, []byte("new content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat %s: %v", link, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("%s is still a symlink after WriteFile; want it replaced by a regular file (rename semantics)", link)
	}
	assertContent(t, link, "new content")

	targetContent, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(targetContent) != "original target content" {
		t.Errorf("target content = %q; want unchanged (a naive truncate+write would have followed the symlink and clobbered it)", targetContent)
	}
}

// TestWriteFile_PreservesExecutableMode covers the git-hook call site
// (0o755) — the mode argument must round-trip exactly, matching the
// executable-bit contract installHook relies on.
func TestWriteFile_PreservesExecutableMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post-merge")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho old\n"), 0o755); err != nil {
		t.Fatalf("seed file: %v", err)
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
