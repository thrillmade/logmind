// symlink_write_test.go — the arbitrary-write half of the marker-block-
// overwrite class this branch is named for. A panel on #300 found that its
// symlink audit covered os.Create and os.OpenFile but never os.WriteFile —
// which FOLLOWS a symlink at the destination. os.ReadFile does too, so a
// DANGLING symlink at a path this package treats as "the user's instruction
// file" makes the missing-file check (errors.Is(err, fs.ErrNotExist)) true,
// the caller concludes the file is simply absent, and the WriteFile that
// follows creates the write body wherever the link points — possibly
// outside the repository entirely.
//
// Every write site in this file now goes through atomicio.WriteFile, which
// (as of #300) refuses outright when the destination already exists as a
// symlink, dangling or not (atomicio.RefuseSymlink). These tests plant that
// exact symlink and run the REAL exported entry points — EnsureAgentsMD,
// CreateAgentFile, MigrateToAgentsMD — asserting on what a user would
// actually observe: the file outside the repo was never created, the link
// itself is untouched, and the error surfaced is legible (not a generic I/O
// failure).
package inserter

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// skipOnWindows mirrors atomicio's own test skip: unprivileged symlink
// creation is unreliable on Windows CI runners.
func skipSymlinkTestsOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
}

// assertNotCreated fails the test if path exists at all (regular file,
// directory, or otherwise) — the escape-hatch write must never land.
func assertNotCreated(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		t.Errorf("write escaped the repo: %s was created", path)
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected error statting %s: %v", path, err)
	}
}

// assertStillDanglingSymlink fails unless path is still exactly the
// symlink it started as — RefuseSymlink's contract is to leave BOTH the
// link and whatever it points to untouched, not to clean up the link on
// its way out.
func assertStillDanglingSymlink(t *testing.T, path, wantTarget string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("symlink at %s is gone: %v", path, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s was replaced with a non-symlink (mode %v)", path, fi.Mode())
	}
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", path, err)
	}
	if got != wantTarget {
		t.Errorf("symlink target changed: got %q, want %q", got, wantTarget)
	}
}

// TestEnsureAgentsMD_RefusesDanglingAGENTSMDSymlink is the panel's exact
// reproduction: AGENTS.md is a symlink to a path that does not exist.
// os.ReadFile(agentsPath) returns fs.ErrNotExist — indistinguishable, to a
// caller that only checks errors.Is(err, fs.ErrNotExist), from AGENTS.md
// simply never having been created. The pre-#306-fix code took that branch
// straight into a bare os.WriteFile(agentsPath, ...), which opens(2) THROUGH
// the symlink and creates the target file wherever it points — outside
// repoRoot, in this test.
func TestEnsureAgentsMD_RefusesDanglingAGENTSMDSymlink(t *testing.T) {
	skipSymlinkTestsOnWindows(t)

	repoRoot := t.TempDir()
	outside := t.TempDir()
	escapeTarget := filepath.Join(outside, "escaped-agents.md")
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")

	if err := os.Symlink(escapeTarget, agentsPath); err != nil {
		t.Fatalf("plant dangling symlink: %v", err)
	}

	_, _, err := EnsureAgentsMD(repoRoot)

	if err == nil {
		t.Fatal("EnsureAgentsMD did not error on a dangling AGENTS.md symlink; " +
			"the write either silently followed the link or silently no-opped")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error does not name the actual cause (symlink): %v", err)
	}

	// The harm: nothing gets created outside the repo the link pointed at.
	assertNotCreated(t, escapeTarget)
	// The refusal: the link itself is left exactly as planted, not replaced
	// with a regular file — a caller re-running after removing the link
	// still sees the same dangling link, not a half-fixed state.
	assertStillDanglingSymlink(t, agentsPath, escapeTarget)
}

// TestCreateAgentFile_RefusesDanglingSymlink covers the per-agent path
// (CLAUDE.md, .cursorrules, ...): CreateAgentFile has no ReadFile/ErrNotExist
// check at all — it went straight to os.WriteFile pre-fix, so a dangling
// symlink here was never even caught by the "is it missing" branch other
// sites had; the vulnerability was the same, just with one less step.
func TestCreateAgentFile_RefusesDanglingSymlink(t *testing.T) {
	skipSymlinkTestsOnWindows(t)

	repoRoot := t.TempDir()
	outside := t.TempDir()
	escapeTarget := filepath.Join(outside, "escaped-claude.md")
	claudePath := filepath.Join(repoRoot, "CLAUDE.md")

	if err := os.Symlink(escapeTarget, claudePath); err != nil {
		t.Fatalf("plant dangling symlink: %v", err)
	}

	_, err := CreateAgentFile("claude", repoRoot)

	if err == nil {
		t.Fatal("CreateAgentFile did not error on a dangling CLAUDE.md symlink")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error does not name the actual cause (symlink): %v", err)
	}
	assertNotCreated(t, escapeTarget)
	assertStillDanglingSymlink(t, claudePath, escapeTarget)
}

// TestMigrateToAgentsMD_RefusesSymlinkedPerAgentFile covers the stub-replace
// write inside `agents migrate`: the per-agent file (.cursorrules here) is
// NOT dangling — it resolves to a real file elsewhere with real user
// content, which os.ReadFile follows and reads successfully before the
// write. A bare os.WriteFile at that point writes the stub THROUGH the
// link, silently replacing content in a file this tool never created and
// does not own; atomicio.WriteFile refuses instead.
func TestMigrateToAgentsMD_RefusesSymlinkedPerAgentFile(t *testing.T) {
	skipSymlinkTestsOnWindows(t)

	repoRoot := t.TempDir()
	// AGENTS.md needs to be a plain, up-to-date file so the leading
	// EnsureAgentsMD(repoRoot) call inside MigrateToAgentsMD no-ops instead
	// of erroring for an unrelated reason.
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(agentsMDTemplate()), 0o644); err != nil {
		t.Fatalf("seed AGENTS.md: %v", err)
	}

	outside := t.TempDir()
	realTarget := filepath.Join(outside, "real-cursorrules")
	const userContent = "# my own cursor rules\n\nUSER_SENTINEL_UNTOUCHED\n"
	if err := os.WriteFile(realTarget, []byte(userContent), 0o644); err != nil {
		t.Fatalf("seed real target: %v", err)
	}
	cursorPath := filepath.Join(repoRoot, ".cursorrules")
	if err := os.Symlink(realTarget, cursorPath); err != nil {
		t.Fatalf("plant symlink: %v", err)
	}

	_, _, err := MigrateToAgentsMD(repoRoot)

	if err == nil {
		t.Fatal("MigrateToAgentsMD did not error on a symlinked .cursorrules")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error does not name the actual cause (symlink): %v", err)
	}
	// The harm: the real file the link points at keeps the user's content —
	// it must NOT have been silently replaced with the stub.
	got, readErr := os.ReadFile(realTarget)
	if readErr != nil {
		t.Fatalf("real target vanished: %v", readErr)
	}
	if string(got) != userContent {
		t.Errorf("migrate wrote through the symlink and clobbered user content:\n got: %q\nwant: %q",
			string(got), userContent)
	}
}

// TestRefreshMarkerBlockFile_RefusesSymlinkedTarget is ruling 3 of #306: THE
// write primitive for the marker block routes through atomicio.WriteFile
// rather than a bare os.WriteFile. The target here is a non-dangling
// symlink to a real file that already carries a well-formed marker block —
// ReadFile + ExtractMarkerBlock both follow the link and succeed, so this
// reaches the write call, which must refuse rather than silently rewrite
// the block through the link.
func TestRefreshMarkerBlockFile_RefusesSymlinkedTarget(t *testing.T) {
	skipSymlinkTestsOnWindows(t)

	outside := t.TempDir()
	realTarget := filepath.Join(outside, "real-agents.md")
	const original = "before\n<!-- logmind-start -->\nOLD BODY\n<!-- logmind-end -->\nafter\n"
	if err := os.WriteFile(realTarget, []byte(original), 0o644); err != nil {
		t.Fatalf("seed real target: %v", err)
	}

	repoRoot := t.TempDir()
	linkedPath := filepath.Join(repoRoot, "AGENTS.md")
	if err := os.Symlink(realTarget, linkedPath); err != nil {
		t.Fatalf("plant symlink: %v", err)
	}

	err := RefreshMarkerBlockFile(linkedPath, "NEW BODY")

	if err == nil {
		t.Fatal("RefreshMarkerBlockFile did not error on a symlinked target")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error does not name the actual cause (symlink): %v", err)
	}
	got, readErr := os.ReadFile(realTarget)
	if readErr != nil {
		t.Fatalf("real target vanished: %v", readErr)
	}
	if string(got) != original {
		t.Errorf("RefreshMarkerBlockFile wrote through the symlink:\n got: %q\nwant: %q",
			string(got), original)
	}
}

// TestMigrateToAgentsMD_RefusesSymlinkedAGENTSMD covers the SECOND write
// site inside MigrateToAgentsMD (the append of migrated per-agent content
// into AGENTS.md) as distinct from the stub-replace site covered above.
// AGENTS.md is a symlink to a real, already-current file, so the leading
// EnsureAgentsMD(repoRoot) call no-ops (no diff to refresh) rather than
// erroring for an unrelated reason — the append write further down is what
// this test exercises.
func TestMigrateToAgentsMD_RefusesSymlinkedAGENTSMD(t *testing.T) {
	skipSymlinkTestsOnWindows(t)

	repoRoot := t.TempDir()
	outside := t.TempDir()
	realAgents := filepath.Join(outside, "real-AGENTS.md")
	current := agentsMDTemplate()
	if err := os.WriteFile(realAgents, []byte(current), 0o644); err != nil {
		t.Fatalf("seed real AGENTS.md: %v", err)
	}
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	if err := os.Symlink(realAgents, agentsPath); err != nil {
		t.Fatalf("plant AGENTS.md symlink: %v", err)
	}

	// A plain (non-symlinked) per-agent file with real content, so its own
	// stub-replace succeeds and populates appendedBlocks — otherwise
	// MigrateToAgentsMD never reaches the append write at all.
	cursorPath := filepath.Join(repoRoot, ".cursorrules")
	if err := os.WriteFile(cursorPath, []byte("# my cursor rules\n\nSOME_CONTENT\n"), 0o644); err != nil {
		t.Fatalf("seed .cursorrules: %v", err)
	}

	_, _, err := MigrateToAgentsMD(repoRoot)

	if err == nil {
		t.Fatal("MigrateToAgentsMD did not error on a symlinked AGENTS.md")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error does not name the actual cause (symlink): %v", err)
	}
	got, readErr := os.ReadFile(realAgents)
	if readErr != nil {
		t.Fatalf("real AGENTS.md vanished: %v", readErr)
	}
	if string(got) != current {
		t.Errorf("migrate wrote through the AGENTS.md symlink:\n got: %q\nwant: %q", string(got), current)
	}
}
