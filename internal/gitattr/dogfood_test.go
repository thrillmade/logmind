// dogfood_test.go — two gates on the merge-driver contract that no unit test
// over a t.TempDir() can see, because both are about artifacts that live
// outside the test: THIS repo's committed `.gitattributes`, and the command
// strings already written into other people's `.git/config`.
//
// The merge-driver coverage that runs the drivers lives in
// internal/cli/timeline_archive_driver_test.go. What is here is the pair of
// facts that have a second copy somewhere logmind does not control.
package gitattr

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from the test's working directory to the module root (the
// directory holding go.mod) so the assertions below read logmind's OWN files
// rather than a fixture that could drift from them.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s — cannot locate the repo root", dir)
		}
		dir = parent
	}
}

// TestRepoGitattributes_RegistersEveryDefaultLine is the dogfooding gate:
// logmind's own `.gitattributes` must carry every registration logmind
// installs in a consumer repo.
//
// It caught the shipping state of #301: DefaultLines gained
// `docs/timeline-archive.md merge=logmind-timeline-archive`, the golden gained
// it, and this repo's committed `.gitattributes` did not — so logmind shipped
// a merge driver for a file it did not itself route through that driver, and
// two PRs touching docs/timeline-archive.md here would have collided in the
// tool's own tree. `logmind doctor` could not have flagged it either: its
// probe answers "current" on the block's PRESENCE, not its contents (#318).
//
// Pinned on the file's bytes rather than on a git shell-out, so it fails the
// same way in a checkout, a worktree, and a tarball.
func TestRepoGitattributes_RegistersEveryDefaultLine(t *testing.T) {
	path := filepath.Join(repoRoot(t), ".gitattributes")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read logmind's own %s: %v", path, err)
	}
	block, ok := managedBlock(string(body))
	if !ok {
		t.Fatalf("%s has no logmind-managed block (%s … %s):\n%s", path, BlockStart, BlockEnd, body)
	}
	for _, line := range DefaultLines {
		if !strings.Contains(block, line+"\n") {
			t.Errorf("logmind's own %s does not register %q.\n"+
				"DefaultLines installs it in every consumer repo; a driver this repo "+
				"ships but does not use is untested where it matters.\n"+
				"managed block is:\n%s", path, line, block)
		}
	}
}

// TestRepoGitattributes_ResolvesEachDerivedDocToItsOwnDriver closes the gap
// the test above cannot see. That one asks "is every DefaultLines entry
// PRESENT", which is a superset check — and superset is the right shape,
// because a user's own lines inside the block must survive. But `.gitattributes`
// is LAST-MATCH-WINS: appending
//
//	docs/timeline-archive.md  merge=logmind-timeline
//
// leaves every DefaultLines line present and still sends the archive through
// the RECENT half's driver, which writes docs/timeline.md's content into
// docs/timeline-archive.md on every merge. Presence is not resolution.
//
// So this asks git itself, via `git check-attr` — the same resolution git
// performs when it picks a driver during a merge. The expectation is DERIVED
// from DefaultLines rather than restated, so the two cannot drift.
//
// Skipped rather than failed where git is unavailable or this tree is not a
// repository (a tarball, a vendored copy): the byte-level test above is the
// one that must hold everywhere, and this is the "what does git actually do
// with those bytes" complement.
func TestRepoGitattributes_ResolvesEachDerivedDocToItsOwnDriver(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := repoRoot(t)
	if err := exec.Command("git", "-C", root, "rev-parse", "--git-dir").Run(); err != nil {
		t.Skip("not a git repository — nothing for check-attr to resolve")
	}

	want := map[string]string{}
	var paths []string
	for _, line := range DefaultLines {
		f := strings.Fields(line)
		if len(f) != 2 || !strings.HasPrefix(f[1], "merge=") {
			t.Fatalf("DefaultLines entry %q is not `<path> merge=<driver>` — this test's parse is "+
				"broken, not the tree", line)
		}
		want[f[0]] = strings.TrimPrefix(f[1], "merge=")
		paths = append(paths, f[0])
	}
	if len(paths) == 0 {
		t.Fatal("DefaultLines is empty — every assertion below would be vacuous")
	}

	args := append([]string{"-C", root, "check-attr", "merge", "--"}, paths...)
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		t.Fatalf("git check-attr: %v", err)
	}

	// `git check-attr merge -- <path>` prints `<path>: merge: <value>`.
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, ": merge: ", 2)
		if len(parts) != 2 {
			continue
		}
		got[parts[0]] = parts[1]
	}
	// Control: the parse must actually have found something, or every
	// comparison below passes by matching nothing against nothing.
	if len(got) != len(paths) {
		t.Fatalf("parsed %d of %d check-attr rows — this test's parse is broken, not the tree.\n"+
			"raw output:\n%s", len(got), len(paths), out)
	}

	for path, driver := range want {
		if got[path] != driver {
			t.Errorf("git resolves the merge driver for %s to %q; DefaultLines registers %q.\n"+
				"`.gitattributes` is last-match-wins, so a later line — one this repo added, or one a "+
				"future DefaultLines entry appends — silently retargets the file without removing "+
				"anything. The wrong driver here renders the WRONG HALF of the §3.3 split into the "+
				"file on every merge.\nfull check-attr output:\n%s", path, got[path], driver, out)
		}
	}
}

// shippedPathDrivers freezes WHICH DRIVER each derived doc is routed through,
// which the command-string freeze below does not cover.
//
// The two are separate facts and only one of them was pinned. Retargeting
//
//	docs/timeline.md  merge=logmind-timeline
//
// to a NEW driver name, and defining that name as `logmind timeline --write %A
// --half recent`, leaves shippedDriverCommands untouched and every assertion
// green — while doing exactly what the freeze exists to prevent. The new name
// is written into `.git/config` by a CURRENT binary and executed by whatever
// `logmind` is on PATH at merge time; on v1.2.0 that is `exit 1, unknown flag`,
// which git reports as an ordinary `CONFLICT (content)` on a file whose own
// header says never to edit it by hand.
//
// A driver's identity is therefore the PAIR — the path it is registered for
// and the command it runs — and both halves are frozen for anything already
// shipped. `docs/timeline-archive.md` is deliberately absent for the same
// reason it is absent from shippedDriverCommands: it is new, so nothing in
// the wild is holding it to anything yet.
var shippedPathDrivers = map[string]string{
	"docs/timeline.md":       "logmind-timeline",
	"docs/file-structure.md": "logmind-file-structure",
}

func TestDefaultLines_ShippedPathToDriverMappingIsFrozen(t *testing.T) {
	got := map[string]string{}
	for _, line := range DefaultLines {
		if f := strings.Fields(line); len(f) == 2 && strings.HasPrefix(f[1], "merge=") {
			got[f[0]] = strings.TrimPrefix(f[1], "merge=")
		}
	}
	// Control: the parse must see the lines, or the loop below compares
	// nothing and reports success.
	if len(got) != len(DefaultLines) {
		t.Fatalf("parsed %d of %d DefaultLines entries — this test's parse is broken, not the tree",
			len(got), len(DefaultLines))
	}
	for path, driver := range shippedPathDrivers {
		if got[path] != driver {
			t.Errorf("DefaultLines routes %s through %q, want %q.\n"+
				"Moving a SHIPPED path to a different driver name is the same cross-version break "+
				"as editing its command string: the new name's definition is written by the binary "+
				"that runs `init`/`doctor --fix` and executed by whatever `logmind` is on PATH at "+
				"merge time. Ship new behaviour by leaving this pair alone.", path, got[path], driver)
		}
	}
}

// managedBlock returns the text strictly between the managed sentinels.
func managedBlock(body string) (string, bool) {
	start := strings.Index(body, BlockStart)
	if start < 0 {
		return "", false
	}
	rest := body[start+len(BlockStart):]
	end := strings.Index(rest, BlockEnd)
	if end < 0 {
		return "", false
	}
	return strings.TrimPrefix(rest[:end], "\n"), true
}

// shippedDriverCommands are the merge-driver command strings that are ALREADY
// in the wild, written into `.git/config` by a released binary.
//
// A driver's command string is a cross-version contract: one binary writes it,
// and a different, often older, binary on PATH executes it at merge time. Git
// reads a driver's nonzero exit as "could not resolve" and reports an ordinary
// content conflict — `CONFLICT (content)` and a `UU` entry on a derived doc
// whose own header says never to edit it by hand — with nothing naming the
// driver as the cause. Measured against the release the fleet is on:
//
//	v1.2.0, `logmind timeline --write %A`                → exit 0
//	v1.2.0, `logmind timeline --write %A --half recent`  → exit 1, unknown flag
//
// So these strings are frozen. New behaviour goes on a NEW driver name — which
// is exactly what `logmind-timeline-archive` is, and why it is not listed here.
// Changing one of these deliberately means shipping it under a new name, or
// waiting for the fleet; it does not mean editing this map.
var shippedDriverCommands = map[string]string{
	"merge.logmind-timeline.driver":       "logmind timeline --write %A",
	"merge.logmind-file-structure.driver": "logmind file-structure --write %A",
}

func TestMergeDriverConfig_ShippedCommandStringsAreFrozen(t *testing.T) {
	got := map[string]string{}
	for _, entry := range MergeDriverConfig {
		got[entry.Key] = entry.Value
	}
	for key, want := range shippedDriverCommands {
		if got[key] != want {
			t.Errorf("%s = %q, want %q.\n"+
				"This string is executed by whatever `logmind` is on PATH at merge time, "+
				"which may predate the binary that wrote it. A driver that exits nonzero "+
				"leaves the derived doc UU and reports only `CONFLICT (content)`. "+
				"Ship the new behaviour under a new driver name.", key, got[key], want)
		}
	}
}
