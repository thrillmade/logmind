// gitattr_symlink_test.go — package-level regression for the three
// os.WriteFile call sites this package used to have (ensureBlockWithLines,
// addMissingLines, RemoveBlock), all of which now route through
// atomicio.WriteFile so a symlinked `.gitattributes` — dangling or not — is
// refused instead of followed.
//
// gitattr_symlink_test.go in internal/cli covers the fresh-file call site
// (ensureBlockWithLines' first branch) end-to-end through the real `logmind
// init` / `logmind doctor --fix` commands, because that is the ONE call
// site both reach and where the reported symptom (a false "✓ Added ..." /
// "gitattributes=written" success) is observable. addMissingLines and
// RemoveBlock share the identical write shape but aren't independently
// reachable through a command today (RemoveBlock has no caller at all yet —
// see its doc comment), so they're pinned here directly against the public
// API instead.
package gitattr

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// existingBlockMissingOneLine is a logmind block already on disk that an
// older binary wrote — has "docs/timeline.md" and
// "docs/timeline-archive.md" registered but is missing
// "docs/file-structure.md", the shape that drives addMissingLines.
const existingBlockMissingOneLine = "# >>> logmind >>>\n" +
	"docs/timeline.md          merge=logmind-timeline\n" +
	"docs/timeline-archive.md  merge=logmind-timeline-archive\n" +
	"# <<< logmind <<<\n"

// TestEnsureBlock_SymlinkedDestination_AddMissingLines_Refused pins the
// addMissingLines call site: a `.gitattributes` that already has the block
// but is missing a newer registration, reached through a symlink whose
// target is a real (non-dangling) file elsewhere. RefuseSymlink refuses ANY
// symlink at the final path component, not just a dangling one, so this
// must be refused exactly like the fresh-file case.
func TestEnsureBlock_SymlinkedDestination_AddMissingLines_Refused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
	dir := t.TempDir()
	outside := filepath.Join(dir, "..", "escaped-gitattributes-addmissing")
	if err := os.WriteFile(outside, []byte(existingBlockMissingOneLine), 0o644); err != nil {
		t.Fatalf("seed outside: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	link := filepath.Join(dir, ".gitattributes")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := EnsureBlock(link)
	if err == nil {
		t.Fatal("EnsureBlock returned no error for a symlinked destination whose block is missing a line; want a refusal")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %q; want it to name the symlink", err)
	}

	got, readErr := os.ReadFile(outside)
	if readErr != nil {
		t.Fatalf("read outside: %v", readErr)
	}
	if string(got) != existingBlockMissingOneLine {
		t.Errorf("outside target was modified despite the refusal:\n got: %q\nwant: %q", got, existingBlockMissingOneLine)
	}
	fi, lerr := os.Lstat(link)
	if lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".gitattributes symlink was replaced; want it left exactly as found")
	}
}

// TestRemoveBlock_SymlinkedDestination_Refused pins RemoveBlock's write —
// no production caller reaches it today (see its doc comment: uninstall
// paths and tests), but it shares the same os.WriteFile-turned-
// atomicio.WriteFile shape and the same class of bug would apply the moment
// an uninstall command calls it.
func TestRemoveBlock_SymlinkedDestination_Refused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
	dir := t.TempDir()
	fullBlock := "# >>> logmind >>>\n" +
		"docs/timeline.md          merge=logmind-timeline\n" +
		"docs/timeline-archive.md  merge=logmind-timeline-archive\n" +
		"docs/file-structure.md    merge=logmind-file-structure\n" +
		"# <<< logmind <<<\n"
	outside := filepath.Join(dir, "..", "escaped-gitattributes-remove")
	if err := os.WriteFile(outside, []byte(fullBlock), 0o644); err != nil {
		t.Fatalf("seed outside: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	link := filepath.Join(dir, ".gitattributes")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := RemoveBlock(link)
	if err == nil {
		t.Fatal("RemoveBlock returned no error for a symlinked destination; want a refusal")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %q; want it to name the symlink", err)
	}

	got, readErr := os.ReadFile(outside)
	if readErr != nil {
		t.Fatalf("read outside: %v", readErr)
	}
	if string(got) != fullBlock {
		t.Errorf("outside target was modified despite the refusal:\n got: %q\nwant: %q", got, fullBlock)
	}
	fi, lerr := os.Lstat(link)
	if lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".gitattributes symlink was replaced; want it left exactly as found")
	}
}
