package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Symlink write-through regressions.
//
// THE BUG: os.WriteFile follows symlinks. Every call site that asked "does
// this file exist?" and read fs.ErrNotExist as "absent, safe to create" was
// exploitable, because os.Stat and os.ReadFile follow a symlink too — a
// DANGLING link reports ErrNotExist for a path that is very much occupied.
// The write that followed then resolved the same link and landed the payload
// wherever it pointed. Outside the repository. logmind runs inside repos its
// user did not write, so "wherever it pointed" is attacker-chosen.
//
// WHY THESE TESTS ARE SHAPED LIKE THIS: each one plants a link and then runs
// the REAL command entry point — runSkillNew, runInstallHook,
// runFileStructure, runAgentsUpdate — and asserts on the filesystem outside
// the repo. A test that asserted "atomicio.WriteFile was called" would pass
// its own mutation and still go green the day someone adds one more raw
// os.WriteFile next to the converted one. These fail on the observable
// damage, so they cover the call site no matter how it is written.
//
// STABLE ACROSS PR #300: that PR adds an explicit symlink refusal inside
// atomicio.WriteFile, turning these from "silently safe" into "refused with
// an error". The invariant asserted here — the outside target is never
// created and never modified — holds in both worlds, so these tests do not
// have to be rewritten when it lands. Where an error IS returned, it must
// name the problem; assertRefusalMentionsSymlink checks that much without
// requiring one yet.

func skipIfNoSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
}

// plantDanglingSymlink puts a symlink at linkPath pointing at a path that
// does not exist, and returns that (absent) target. This is the exact
// primitive the attack needs: os.Stat(linkPath) and os.ReadFile(linkPath)
// both report fs.ErrNotExist, so "absent" logic proceeds.
func plantDanglingSymlink(t *testing.T, linkPath, target string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("mkdir for link %s: %v", linkPath, err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("symlink %s -> %s: %v", linkPath, target, err)
	}
	// Control: prove the trap is armed. If os.Stat does NOT report
	// ErrNotExist here, the vulnerable branch is never entered and the whole
	// test is vacuous — it would pass against the unfixed code too.
	if _, err := os.Stat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("planted link %s does not read as absent (Stat err = %v); "+
			"the test would not exercise the create-because-absent branch", linkPath, err)
	}
	return target
}

// assertNotWrittenThrough is the security invariant.
func assertNotWrittenThrough(t *testing.T, outsideTarget string) {
	t.Helper()
	if body, err := os.ReadFile(outsideTarget); err == nil {
		t.Fatalf("WROTE THROUGH THE SYMLINK: %s was created outside the repository, %d bytes:\n%s",
			outsideTarget, len(body), truncate(string(body)))
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error stat-ing %s: %v", outsideTarget, err)
	}
}

// assertRefusalMentionsSymlink lets these tests survive PR #300 landing a
// hard refusal inside atomicio.WriteFile. Today the write is made safe by
// rename semantics and returns nil; once #300 lands it returns an error, and
// that error has to actually tell the user what happened.
func assertRefusalMentionsSymlink(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if !strings.Contains(strings.ToLower(err.Error()), "symlink") &&
		!strings.Contains(strings.ToLower(err.Error()), "symbolic link") {
		t.Errorf("command refused the write but the message does not name the problem: %v", err)
	}
}

// assertRealFile checks the tool-managed path ended up a regular file (the
// link was replaced, not followed) when the command reported success.
func assertRealFile(t *testing.T, path string, cmdErr error) {
	t.Helper()
	if cmdErr != nil {
		return // refused outright — nothing should have been written
	}
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("command succeeded but %s does not exist: %v", path, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("%s is still a symlink after a successful write; "+
			"the next write will follow it off-tree", path)
	}
}

func truncate(s string) string {
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}

// TestSkillNew_DanglingSymlinkAtSkillMD covers internal/skill.ScaffoldBasic
// through `logmind skill new`. runSkillNew's own os.Stat guard AND
// ScaffoldBasic's both report "absent" for the dangling link, so the
// scaffold write is reached with the link on the destination.
func TestSkillNew_DanglingSymlinkAtSkillMD(t *testing.T) {
	skipIfNoSymlinks(t)
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	loot := filepath.Join(base, "outside", "loot-skill.md")
	if err := os.MkdirAll(filepath.Join(base, "outside"), 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	skillMD := filepath.Join(repo, ".claude", "skills", "sample", "SKILL.md")
	plantDanglingSymlink(t, skillMD, loot)

	var stdout bytes.Buffer
	err := runSkillNew(repo, "sample", "A test skill", true, true, &stdout)

	assertNotWrittenThrough(t, loot)
	assertRefusalMentionsSymlink(t, err)
	assertRealFile(t, skillMD, err)
}

// TestSkillNew_DanglingSymlinkAtProvenance covers
// internal/skill.WriteProvenanceSkeleton on the same command. Separate test
// because it is a separate call site behind its own os.Stat guard — routing
// only SKILL.md would leave this one writing off-tree.
func TestSkillNew_DanglingSymlinkAtProvenance(t *testing.T) {
	skipIfNoSymlinks(t)
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	loot := filepath.Join(base, "outside", "loot-provenance.md")
	if err := os.MkdirAll(filepath.Join(base, "outside"), 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	prov := filepath.Join(repo, ".claude", "skills", "sample", "PROVENANCE.md")
	plantDanglingSymlink(t, prov, loot)

	var stdout bytes.Buffer
	// noProvenance=false: the PROVENANCE.md write is part of this run.
	err := runSkillNew(repo, "sample", "A test skill", true, false, &stdout)

	assertNotWrittenThrough(t, loot)
	assertRefusalMentionsSymlink(t, err)
	assertRealFile(t, prov, err)
}

// TestInstallHook_DanglingSymlinkAtPreCommit covers the fresh-install branch
// of `logmind install-hook`. The nastiest of the set: the payload is a 0o755
// shell script, so writing through the link drops an EXECUTABLE at an
// attacker-chosen path.
//
// Note the deliberate asymmetry this pins: the append branch of the same
// function still uses os.WriteFile on purpose (a user may legitimately
// symlink .git/hooks/pre-commit at a shared script), and it is unreachable
// by this attack because it requires os.ReadFile to have SUCCEEDED.
func TestInstallHook_DanglingSymlinkAtPreCommit(t *testing.T) {
	skipIfNoSymlinks(t)
	repo := initRepo(t)
	base := t.TempDir()
	loot := filepath.Join(base, "loot-hook.sh")

	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	_ = os.Remove(hookPath)
	plantDanglingSymlink(t, hookPath, loot)

	var stdout bytes.Buffer
	err := runInstallHook(repo, false, &stdout)

	assertNotWrittenThrough(t, loot)
	assertRefusalMentionsSymlink(t, err)
	assertRealFile(t, hookPath, err)
}

// TestFileStructure_DanglingSymlinkAtPredictableTmp covers
// internal/tree.WriteFileStructure through `logmind file-structure --write`.
//
// This one was a second, sharper bug on top of the class: the old code
// hand-rolled temp+rename but wrote to the PREDICTABLE name
// targetPath+".tmp" with a bare os.WriteFile. So the destination did not
// even have to be attackable — planting the link at the temp name was
// enough, and the following os.Rename then moved the LINK into place,
// leaving docs/file-structure.md itself pointing off-tree for every
// subsequent write. atomicio uses os.CreateTemp, whose name cannot be
// guessed.
func TestFileStructure_DanglingSymlinkAtPredictableTmp(t *testing.T) {
	skipIfNoSymlinks(t)
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	loot := filepath.Join(base, "outside", "loot-tree.md")
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(base, "outside"), 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	target := filepath.Join(repo, "docs", "file-structure.md")
	plantDanglingSymlink(t, target+".tmp", loot)

	var stdout, stderr bytes.Buffer
	err := runFileStructure(repo, target, false, 2, true, &stdout, &stderr)

	assertNotWrittenThrough(t, loot)
	assertRefusalMentionsSymlink(t, err)

	// And the destination must be a real file holding the rendered tree —
	// not a symlink inherited from the renamed temp path.
	if err == nil {
		assertRealFile(t, target, err)
		body, rerr := os.ReadFile(target)
		if rerr != nil {
			t.Fatalf("read %s: %v", target, rerr)
		}
		if len(body) == 0 {
			t.Errorf("%s is empty; the render did not land", target)
		}
	}
}

// TestAgentsUpdate_SymlinkedWorkflowFile covers internal/cli/agents.go's
// apply path. This is the non-dangling half of the same class: the link
// RESOLVES, so the scan reads real (attacker-supplied) content off-tree and
// the rewrite goes straight back through the link. No ErrNotExist needed.
func TestAgentsUpdate_SymlinkedWorkflowFile(t *testing.T) {
	skipIfNoSymlinks(t)
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(filepath.Join(repo, ".github", "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	// A real file outside the repo, carrying a stale pin so the updater
	// decides it needs rewriting.
	victim := filepath.Join(outside, "victim.yml")
	const victimBody = "jobs:\n  x:\n    steps:\n      - run: pip install \"logmind==0.0.1\"\n"
	if err := os.WriteFile(victim, []byte(victimBody), 0o644); err != nil {
		t.Fatalf("write victim: %v", err)
	}

	link := filepath.Join(repo, ".github", "workflows", "regen-timeline.yml")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := runAgentsUpdate(repo, "9.9.9", true, &stdout, &stderr)
	assertRefusalMentionsSymlink(t, err)

	got, rerr := os.ReadFile(victim)
	if rerr != nil {
		t.Fatalf("read victim: %v", rerr)
	}
	if string(got) != victimBody {
		t.Fatalf("WROTE THROUGH THE SYMLINK: %s (outside the repository) was rewritten.\n got: %q\nwant: %q",
			victim, string(got), victimBody)
	}
	assertRealFile(t, link, err)
}

// TestAgentsUpdate_PreservesModeAndSeversHardlinks measures, through the REAL
// command, the two user-visible consequences of atomicio's one rule — the
// exact pair a review panel measured when it blocked this PR:
//
//	[before] mode=-rw------- links=2  twin updated   (os.WriteFile)
//	[broken] mode=-rw-r--r-- links=1  twin stale     (atomicio, chmod'ing)
//	[now   ] mode=-rw------- links=1  twin stale     (atomicio, rule 1)
//
// MODE is a regression: converting a write to atomicio must not re-permission
// a file the user owns. A 0600 workflow file stays 0600.
//
// LINKS is a CONTRACT, asserted here so the next person meets it in a test
// rather than in a bug report: an atomic replace swaps the NAME, so a
// hardlink twin keeps the old inode and the old content. That is inherent to
// atomic replace and is documented on atomicio.WriteFile. If a call site ever
// needs the inode preserved, it must not use that primitive.
func TestAgentsUpdate_PreservesModeAndSeversHardlinks(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	wf := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(wf, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}

	target := filepath.Join(wf, "regen-timeline.yml")
	const body = "jobs:\n  x:\n    steps:\n      - run: pip install \"logmind==0.0.1\"\n"
	if err := os.WriteFile(target, []byte(body), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	if err := os.Chmod(target, 0o600); err != nil { // defeat the ambient umask
		t.Fatalf("chmod workflow: %v", err)
	}
	twin := filepath.Join(wf, "regen-timeline.hardlink.yml")
	hardlinked := os.Link(target, twin) == nil

	var stdout, stderr bytes.Buffer
	if err := runAgentsUpdate(repo, "9.9.9", true, &stdout, &stderr); err != nil {
		t.Fatalf("runAgentsUpdate: %v", err)
	}

	fi, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat %s: %v", target, err)
	}
	// Control: the rewrite must actually have happened, or "mode unchanged"
	// is unchanged-because-untouched and proves nothing.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	if string(got) == body {
		t.Fatalf("the workflow pin was not rewritten; this test would pass vacuously")
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode after `agents update --apply` = %04o; want 0600. Converting a write to "+
			"atomicio must not re-permission the user's file — os.WriteFile only honours perm "+
			"on create, and atomicio.WriteFile reproduces that", perm)
	}

	if hardlinked {
		twinBody, rerr := os.ReadFile(twin)
		if rerr != nil {
			t.Fatalf("read hardlink twin: %v", rerr)
		}
		if string(twinBody) != body {
			t.Errorf("hardlink twin = %q; want the OLD content. The atomic rename is documented "+
				"to sever hardlinks; if that stopped being true, atomicio's WriteFile doc "+
				"comment (rule 3) is now wrong and callers were told the wrong thing", twinBody)
		}
	}
}
