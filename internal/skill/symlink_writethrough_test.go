package skill

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Symlink write-through regressions for the two internal/skill call sites
// whose real command path cannot be driven from a unit test without an
// external dependency:
//
//   - copyTree is reached by `logmind skill push`, which first clones a
//     user-supplied --catalog-target over the network;
//   - WriteDrafts is reached by `logmind skill suggest --write-drafts`,
//     which is gated behind an LLM round-trip.
//
// Both are asserted here at the package boundary the command calls, on the
// same observable-damage invariant the internal/cli tests use: the file
// outside the repository is never created and never modified. The other
// three internal/skill sites (ScaffoldBasic, WriteProvenanceSkeleton, and
// the push provenance write) ARE covered end-to-end from internal/cli.

func skipNoSymlink(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
}

func assertUntouched(t *testing.T, loot string, err error) {
	t.Helper()
	if body, rerr := os.ReadFile(loot); rerr == nil {
		t.Fatalf("WROTE THROUGH THE SYMLINK: %s was created outside the destination tree, %d bytes:\n%s",
			loot, len(body), body)
	} else if !os.IsNotExist(rerr) {
		t.Fatalf("unexpected error reading %s: %v", loot, rerr)
	}
	if err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "symlink") &&
		!strings.Contains(strings.ToLower(err.Error()), "symbolic link") {
		t.Errorf("refused the write but the message does not name the problem: %v", err)
	}
}

// TestCopyTree_SymlinkInDestination is the catalog-side attack. `logmind
// skill push` git-clones a user-named catalog repo and copies the local
// skill into it — and git checks out symlinks verbatim. A catalog carrying
// skills/<name>/notes.md as a symlink pointed at, say, ~/.ssh/authorized_keys
// would have copyTree's os.WriteFile write the copied body straight through
// it. The destination tree here stands in for the fresh clone.
func TestCopyTree_SymlinkInDestination(t *testing.T) {
	skipNoSymlink(t)
	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "clone", "skills", "sample")
	loot := filepath.Join(base, "outside", "loot-notes.md")
	for _, d := range []string{src, dst, filepath.Join(base, "outside")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "notes.md"), []byte("payload from the local skill\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	// The hostile catalog's checked-out symlink.
	if err := os.Symlink(loot, filepath.Join(dst, "notes.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err := copyTree(src, dst, []string{"notes.md"})
	assertUntouched(t, loot, err)

	if err == nil {
		fi, lerr := os.Lstat(filepath.Join(dst, "notes.md"))
		if lerr != nil {
			t.Fatalf("copy succeeded but dest missing: %v", lerr)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			t.Errorf("dest is still a symlink after copyTree")
		}
		// The mode contract copyTree exists to keep (review #136 / bug 3)
		// must survive the switch to the atomic writer.
		if got := fi.Mode().Perm(); got != 0o644 {
			t.Errorf("dest perm = %04o; want 0644 (source mode must round-trip)", got)
		}
	}
}

// TestCopyTree_PreservesExecutableBitOverPreStagedDest pins the half of the
// old call site that was NOT a security fix: os.WriteFile only honours its
// perm argument on create, so a pre-existing destination kept its old mode
// and the site needed a follow-up os.Chmod. atomicio.WriteFile chmods the
// temp file before the rename, which subsumes that — this test fails if the
// subsumption is wrong and the executable bit silently stops copying.
func TestCopyTree_PreservesExecutableBitOverPreStagedDest(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src", "scripts")
	dst := filepath.Join(base, "dst", "scripts")
	for _, d := range []string{src, dst} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "run.sh"), []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	// Pre-staged destination at a DIFFERENT mode — the case a bare
	// os.WriteFile got wrong.
	if err := os.WriteFile(filepath.Join(dst, "run.sh"), []byte("stale\n"), 0o600); err != nil {
		t.Fatalf("pre-stage dst: %v", err)
	}

	if err := copyTree(filepath.Join(base, "src"), filepath.Join(base, "dst"), []string{"scripts/run.sh"}); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	fi, err := os.Stat(filepath.Join(dst, "run.sh"))
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o755 {
		t.Errorf("dest perm = %04o; want 0755 — the executable bit did not survive the copy "+
			"onto a pre-staged file", got)
	}
}

// TestWriteDrafts_DanglingSymlinkAtDraft covers `logmind skill suggest
// --write-drafts <dir>`. Draft filenames are fully derived from the
// suggestion slug (skill-proposal-<slug>.md), so they are predictable and a
// link can be pre-planted at one. "Overwrites existing files" was always
// meant as overwrite the FILE, not follow whatever sits at that name.
func TestWriteDrafts_DanglingSymlinkAtDraft(t *testing.T) {
	skipNoSymlink(t)
	base := t.TempDir()
	outDir := filepath.Join(base, "drafts")
	loot := filepath.Join(base, "outside", "loot-draft.md")
	for _, d := range []string{outDir, filepath.Join(base, "outside")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	draft := filepath.Join(outDir, "skill-proposal-sample-slug.md")
	if err := os.Symlink(loot, draft); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := os.Stat(draft); !os.IsNotExist(err) {
		t.Fatalf("planted link does not read as absent (Stat err = %v); test would be vacuous", err)
	}

	err := WriteDrafts(outDir, []Suggestion{{Slug: "sample-slug", Phrase: "sample phrase"}})
	assertUntouched(t, loot, err)

	if err == nil {
		fi, lerr := os.Lstat(draft)
		if lerr != nil {
			t.Fatalf("succeeded but draft missing: %v", lerr)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s is still a symlink after WriteDrafts", draft)
		}
	}
}
