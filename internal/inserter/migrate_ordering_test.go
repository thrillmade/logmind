// migrate_ordering_test.go — logmind#350. `agents migrate` is the one
// command that MOVES the user's own words instead of adding logmind's, and
// it used to do the destructive half first: stub each source inside the
// loop, write the collected content into AGENTS.md afterwards, with the
// collection living only in a slice the error return threw away. A symlinked
// AGENTS.md (the guard fires there, correctly, at the append write) left two
// per-tool files holding the stub and the user's paragraphs in NEITHER place.
//
// Every test here asserts the ordering invariant at the level the user feels
// it: after a failure, is the sentence I wrote still on disk somewhere. Each
// one carries its own CONTROL — the same repo migrated without the injected
// failure — because "the source is byte-identical" is also what a migration
// that silently stopped doing anything would report.
package inserter

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// errInjected stands in for the failures no pre-check can predict: a full
// disk, a read-only mount, a mode change between the check and the write.
// The symlink case CAN be pre-checked and is covered separately — it cannot
// stand in for this one, because a fix that only hoisted the symlink guard
// would pass there and still lose content here.
var errInjected = errors.New("simulated: no space left on device")

// failMigrateWriteOn swaps in a writer that fails for exactly one path and
// delegates every other path to the real primitive, so the run reaches the
// injected failure through ordinary behaviour rather than being short-
// circuited wholesale. Restored on cleanup; these tests must not run in
// parallel with each other, and none of them calls t.Parallel.
func failMigrateWriteOn(t *testing.T, target string) {
	t.Helper()
	previous := migrateWrite
	migrateWrite = func(path string, data []byte, perm os.FileMode) error {
		if path == target {
			return errInjected
		}
		return previous(path, data, perm)
	}
	t.Cleanup(func() { migrateWrite = previous })
}

// migrateSources is the repro from #350, widened: sentinels the assertions
// can count, spread across five registry rows so "the third of five failed"
// has a middle to fail in. Loop order is agents.All()'s: claude, cursor,
// copilot, windsurf, aider.
var migrateSources = []struct{ rel, body string }{
	{"CLAUDE.md", "# mine\n\nMY_CLAUDE_CONVENTIONS\n"},
	{".cursorrules", "# mine\n\nMY_CURSOR_CONVENTIONS\n"},
	{".github/copilot-instructions.md", "MY_COPILOT_NOTES\n"},
	{".windsurfrules", "MY_WINDSURF_NOTES\n"},
	{"CONVENTIONS.md", "MY_AIDER_NOTES\n"},
}

// seedMigrateRepo writes a current AGENTS.md (so the EnsureAgentsMD call at
// the top of MigrateToAgentsMD no-ops rather than erroring for an unrelated
// reason) plus every source, and returns the exact bytes of each path so a
// later comparison is against what was really on disk.
func seedMigrateRepo(t *testing.T) (dir string, before map[string]string) {
	t.Helper()
	dir = t.TempDir()
	writeAgentFile(t, dir, "AGENTS.md", agentsMDTemplate())
	before = map[string]string{"AGENTS.md": agentsMDTemplate()}
	for _, s := range migrateSources {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, s.rel)), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", s.rel, err)
		}
		writeAgentFile(t, dir, s.rel, s.body)
		before[s.rel] = s.body
	}
	return dir, before
}

// assertSourcesUnchanged is the assertion the issue is about: not "the file
// still exists", not "it still mentions the sentinel" — byte-identical to
// what was there before the command ran.
func assertSourcesUnchanged(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	for _, s := range migrateSources {
		if got := readAgentFile(t, dir, s.rel); got != before[s.rel] {
			t.Errorf("%s was modified by a failed migration:\n got: %q\nwant: %q",
				s.rel, got, before[s.rel])
		}
	}
}

// assertMigrated is the CONTROL every failure assertion above is measured
// against: on a clean run each sentinel moves — once — into AGENTS.md and
// leaves its source a stub. Without this, "stop migrating entirely" would
// satisfy every other test in this file.
func assertMigrated(t *testing.T, dir string) {
	t.Helper()
	agentsBody := readAgentFile(t, dir, "AGENTS.md")
	for _, s := range migrateSources {
		sentinel := strings.TrimSpace(strings.TrimPrefix(s.body, "# mine\n\n"))
		if n := strings.Count(agentsBody, sentinel); n != 1 {
			t.Errorf("AGENTS.md contains %s %d times; want exactly 1", sentinel, n)
		}
		got := readAgentFile(t, dir, s.rel)
		if strings.Contains(got, sentinel) {
			t.Errorf("%s still holds %s after a successful migration: %q", s.rel, sentinel, got)
		}
		if !IsStub(got) {
			t.Errorf("%s is not a stub after a successful migration: %q", s.rel, got)
		}
	}
}

// TestMigrateToAgentsMD_FinalWriteFails_SourcesByteIdentical is the mutation
// target: the preserving write fails for a reason nothing could have checked
// for, and every source must still hold its own bytes. Moving the stub write
// back ahead of the AGENTS.md write turns this red on all five files.
func TestMigrateToAgentsMD_FinalWriteFails_SourcesByteIdentical(t *testing.T) {
	dir, before := seedMigrateRepo(t)
	failMigrateWriteOn(t, filepath.Join(dir, agentsMDName))

	_, _, _, err := MigrateToAgentsMD(dir)

	if !errors.Is(err, errInjected) {
		t.Fatalf("MigrateToAgentsMD err = %v; want the injected write failure", err)
	}
	assertSourcesUnchanged(t, dir, before)
	if got := readAgentFile(t, dir, "AGENTS.md"); got != before["AGENTS.md"] {
		t.Errorf("AGENTS.md changed despite the write failing:\n got: %q\nwant: %q",
			got, before["AGENTS.md"])
	}
}

// TestMigrateToAgentsMD_HappyPathStillMigrates is the control for the test
// above, run against the same seed with nothing injected. It is what makes
// "byte-identical" evidence of an ordering fix rather than of a migration
// that stopped working.
func TestMigrateToAgentsMD_HappyPathStillMigrates(t *testing.T) {
	dir, _ := seedMigrateRepo(t)

	msgs, _, refused, err := MigrateToAgentsMD(dir)

	if err != nil {
		t.Fatalf("MigrateToAgentsMD: %v", err)
	}
	if len(refused) != 0 {
		t.Errorf("refused = %+v; want none on the happy path", refused)
	}
	assertMigrated(t, dir)
	// Both messages per source, and the stub line still follows its own
	// migrated line — the phases moved, the reported order did not.
	want := []string{
		"✓ Migrated Claude Code (CLAUDE.md) content into AGENTS.md",
		"✓ CLAUDE.md replaced with stub",
		"✓ Migrated Cursor (.cursorrules) content into AGENTS.md",
		"✓ .cursorrules replaced with stub",
	}
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, strings.Join(want, "\n")) {
		t.Errorf("message order changed:\ngot:\n%s\nwant it to contain:\n%s",
			joined, strings.Join(want, "\n"))
	}
}

// TestMigrateToAgentsMD_UnreadableSourceAbortsWholeMigration is the
// all-or-nothing ruling, pinned. The THIRD of five sources cannot be read;
// the first two must not have been consolidated, because a tree where half
// the instructions moved and nothing says which half is worse to hand back
// than a refusal naming the file.
func TestMigrateToAgentsMD_UnreadableSourceAbortsWholeMigration(t *testing.T) {
	dir, before := seedMigrateRepo(t)
	third := filepath.Join(dir, filepath.FromSlash(migrateSources[2].rel))
	if err := os.Chmod(third, 0o000); err != nil {
		t.Fatalf("chmod third source: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(third, 0o644) })
	// CONTROL: root ignores the mode, and the test would then be asserting
	// on a successful migration. Prove the read really fails first.
	if _, err := os.ReadFile(third); err == nil {
		t.Skip("this user can read a 0000 file (root?); the read failure cannot be staged")
	}

	_, _, _, err := MigrateToAgentsMD(dir)

	if err == nil {
		t.Fatal("MigrateToAgentsMD succeeded with an unreadable source")
	}
	if !strings.Contains(err.Error(), "copilot-instructions.md") {
		t.Errorf("error does not name the file that stopped the migration: %v", err)
	}
	if err := os.Chmod(third, 0o644); err != nil {
		t.Fatalf("restore mode for comparison: %v", err)
	}
	assertSourcesUnchanged(t, dir, before)
	if got := readAgentFile(t, dir, "AGENTS.md"); got != before["AGENTS.md"] {
		t.Errorf("AGENTS.md was written by an aborted migration:\n got: %q\nwant: %q",
			got, before["AGENTS.md"])
	}
}

// TestMigrateToAgentsMD_SymlinkedAGENTSMD_LeavesSourcesIntact is the
// reproduction from the issue, asserted where the harm was: not on the
// symlink's target (symlink_write_test.go already covers that) but on the
// per-tool files, which the old ordering stubbed on the way to a refusal.
func TestMigrateToAgentsMD_SymlinkedAGENTSMD_LeavesSourcesIntact(t *testing.T) {
	skipSymlinkTestsOnWindows(t)

	dir, before := seedMigrateRepo(t)
	// Re-point AGENTS.md at a real, already-current file outside the repo,
	// so EnsureAgentsMD no-ops and the refusal comes from the append write.
	realAgents := filepath.Join(t.TempDir(), "agents-real.md")
	writeAgentFile(t, filepath.Dir(realAgents), filepath.Base(realAgents), agentsMDTemplate())
	if err := os.Remove(filepath.Join(dir, agentsMDName)); err != nil {
		t.Fatalf("remove seeded AGENTS.md: %v", err)
	}
	if err := os.Symlink(realAgents, filepath.Join(dir, agentsMDName)); err != nil {
		t.Fatalf("plant AGENTS.md symlink: %v", err)
	}

	_, _, _, err := MigrateToAgentsMD(dir)

	if err == nil {
		t.Fatal("MigrateToAgentsMD did not error on a symlinked AGENTS.md")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error does not name the actual cause (symlink): %v", err)
	}
	assertSourcesUnchanged(t, dir, before)
	if got, rerr := os.ReadFile(realAgents); rerr != nil || string(got) != agentsMDTemplate() {
		t.Errorf("the symlink's target changed: %q (err %v)", got, rerr)
	}
}

// TestMigrateToAgentsMD_SymlinkedSourceLeavesEarlierSourcesIntact covers the
// other late guard on this path. The refusal for a symlinked per-tool file
// used to live in its own stub write, halfway through the loop — so a link
// on the FOURTH file was found after the first three had been consolidated.
// Hoisting it into the plan phase is what makes the abort all-or-nothing.
func TestMigrateToAgentsMD_SymlinkedSourceLeavesEarlierSourcesIntact(t *testing.T) {
	skipSymlinkTestsOnWindows(t)

	dir, before := seedMigrateRepo(t)
	linked := filepath.Join(dir, filepath.FromSlash(migrateSources[3].rel))
	outside := filepath.Join(t.TempDir(), "real-windsurfrules")
	const outsideBody = "USER_SENTINEL_OUTSIDE_THE_REPO\n"
	writeAgentFile(t, filepath.Dir(outside), filepath.Base(outside), outsideBody)
	if err := os.Remove(linked); err != nil {
		t.Fatalf("remove seeded source: %v", err)
	}
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatalf("plant source symlink: %v", err)
	}
	before[migrateSources[3].rel] = outsideBody // read through the link

	_, _, _, err := MigrateToAgentsMD(dir)

	if err == nil {
		t.Fatal("MigrateToAgentsMD did not error on a symlinked per-tool file")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error does not name the actual cause (symlink): %v", err)
	}
	assertSourcesUnchanged(t, dir, before)
	if got := readAgentFile(t, dir, "AGENTS.md"); got != before["AGENTS.md"] {
		t.Errorf("AGENTS.md was written by an aborted migration:\n got: %q\nwant: %q",
			got, before["AGENTS.md"])
	}
}

// TestMigrateToAgentsMD_StubWriteFailsAfterPreserve_ContentSurvives states
// the remaining failure mode out loud rather than leaving it emergent. Once
// AGENTS.md holds the content, a source that will not take the stub leaves
// the user's words in TWO places. That is duplication, and it is the price
// of the ordering: the same failure under the old ordering left them in
// none.
func TestMigrateToAgentsMD_StubWriteFailsAfterPreserve_ContentSurvives(t *testing.T) {
	dir, before := seedMigrateRepo(t)
	stubborn := filepath.Join(dir, filepath.FromSlash(migrateSources[1].rel))
	failMigrateWriteOn(t, stubborn)

	_, _, _, err := MigrateToAgentsMD(dir)

	if !errors.Is(err, errInjected) {
		t.Fatalf("MigrateToAgentsMD err = %v; want the injected write failure", err)
	}
	agentsBody := readAgentFile(t, dir, "AGENTS.md")
	if !strings.Contains(agentsBody, "MY_CURSOR_CONVENTIONS") {
		t.Errorf("the preserving write did not run before the stub write; AGENTS.md: %q", agentsBody)
	}
	if got := readAgentFile(t, dir, migrateSources[1].rel); got != before[migrateSources[1].rel] {
		t.Errorf("the source that refused the stub was modified anyway:\n got: %q\nwant: %q",
			got, before[migrateSources[1].rel])
	}
}
