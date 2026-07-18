// derived_test.go — exercises the shared derived-doc constants and the
// non-default-branch predicate that the L1 log guard, warp, the pulse
// probe, and context all key off of.
package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOnNonDefaultBranch: false on the default branch (main), true once a
// feature branch is checked out.
func TestOnNonDefaultBranch(t *testing.T) {
	dir := t.TempDir()
	initLogTestGitRepo(t, dir) // git init --initial-branch=main + config
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	commitAll(t, dir, "init")

	if onNonDefaultBranch(dir) {
		t.Fatal("default branch should be false")
	}
	runGitIn(t, dir, "checkout", "-b", "feat/x")
	if !onNonDefaultBranch(dir) {
		t.Fatal("feature branch should be true")
	}
}
