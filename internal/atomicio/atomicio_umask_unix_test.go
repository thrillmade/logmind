//go:build !windows

package atomicio

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestWriteFile_CreateRespectsUmask is the second half of the mode bug two
// panels found. The old implementation created the temp at 0600 and then
// chmod'd it to perm — and chmod(2) is not filtered by the umask, so
// `logmind init` under `umask 077` produced world-readable 0644 files where
// the os.WriteFile it replaced produced 0600.
//
// The fix hands perm to open(2) on the create path, exactly like
// os.WriteFile, so the kernel applies the umask. Asserted against
// os.WriteFile under the same umask rather than against a literal, so the
// test states the contract ("indistinguishable from os.WriteFile") rather
// than a number somebody has to keep in sync.
//
// syscall.Umask is process-global. Tests in this package do not call
// t.Parallel(), so the mask is restored before any other test runs.
func TestWriteFile_CreateRespectsUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)

	dir := t.TempDir()

	reference := filepath.Join(dir, "reference.md")
	if err := os.WriteFile(reference, []byte("x"), 0o644); err != nil {
		t.Fatalf("reference os.WriteFile: %v", err)
	}
	refFi, err := os.Stat(reference)
	if err != nil {
		t.Fatalf("stat reference: %v", err)
	}
	// Control: prove the umask is actually in force, or the comparison
	// below is vacuous and would pass with the bug present.
	if refFi.Mode().Perm() != 0o600 {
		t.Fatalf("os.WriteFile(0644) under umask 077 produced %04o, want 0600 — "+
			"the umask is not in effect and this test proves nothing", refFi.Mode().Perm())
	}

	subject := filepath.Join(dir, "subject.md")
	if err := WriteFile(subject, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fi, err := os.Stat(subject)
	if err != nil {
		t.Fatalf("stat subject: %v", err)
	}
	if fi.Mode().Perm() != refFi.Mode().Perm() {
		t.Errorf("atomicio.WriteFile created %04o under umask 077; os.WriteFile created %04o. "+
			"Rule 1 says perm is the CREATE mode and the umask applies to it — a chmod to perm "+
			"after the fact ignores the umask and widens the file",
			fi.Mode().Perm(), refFi.Mode().Perm())
	}
}

// TestWriteFile_SeveredHardlinkDropsLinkCount is the syscall-level half of
// TestWriteFile_RenameSeversHardlinks: st_nlink goes 2 -> 1, which is the
// number the panel measured on the real command.
func TestWriteFile_SeveredHardlinkDropsLinkCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	twin := filepath.Join(dir, "AGENTS-twin.md")

	if err := os.WriteFile(path, []byte("shared\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.Link(path, twin); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	if got := nlink(t, path); got != 2 {
		t.Fatalf("link count before = %d, want 2; the test would be vacuous", got)
	}

	if err := WriteFile(path, []byte("rewritten\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := nlink(t, path); got != 1 {
		t.Errorf("link count after = %d, want 1 — the rename is documented to sever hardlinks", got)
	}
}

func nlink(t *testing.T, path string) uint64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skipf("no syscall.Stat_t on %T", fi.Sys())
	}
	return uint64(st.Nlink)
}
