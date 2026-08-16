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
