package testgit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitRepo_SetsBothMaintenanceKeys pins the exact config InitRepo
// writes, read back from the repo it built (not asserted against
// InitRepo's own source). A regression that drops one of the two keys
// from DisableMaintenance still compiles and still "creates a repo" —
// this is the check that would actually notice.
func TestInitRepo_SetsBothMaintenanceKeys(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	InitRepo(t, dir, "-q")

	for _, tc := range []struct{ key, want string }{
		{"gc.auto", "0"},
		{"maintenance.auto", "false"},
	} {
		cmd := exec.Command("git", "config", "--get", tc.key)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git config --get %s: %v", tc.key, err)
		}
		if got := strings.TrimSpace(string(out)); got != tc.want {
			t.Errorf("%s = %q; want %q", tc.key, got, tc.want)
		}
	}
}

// TestInitRepo_SuppressesMaintenanceSpawn pins the fix at the level the
// bug actually appears: not "the config keys are set" but "the spawn
// that races t.TempDir() cleanup does not happen". It runs 5 real
// commits in a repo InitRepo built, each under its own
// GIT_TRACE2_EVENT log, and asserts `git maintenance` is never among the
// child processes git spawns.
//
// This is the exact methodology and repo state issue #271 measured
// with on git 2.39.5 (same result reproduced here as a control before
// this test was written: gc.auto=0 alone left `git maintenance`
// spawning on 5 of 5 commits; gc.auto=0 + maintenance.auto=false took
// it to 0 of 5). A helper that regresses to gc.auto-only would make
// THIS test fail while TestInitRepo_SetsBothMaintenanceKeys above would
// need to check the right key to catch the same regression — this one
// catches it regardless of which key is missing, because it looks at
// git's actual behavior instead of at config state.
func TestInitRepo_SuppressesMaintenanceSpawn(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	InitRepo(t, dir, "-q", "--initial-branch=main")
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "Test")
	run(t, dir, "config", "commit.gpgsign", "false")

	traceDir := t.TempDir()
	const commits = 5
	spawns := 0
	for i := 0; i < commits; i++ {
		tracePath := filepath.Join(traceDir, fmt.Sprintf("trace-%d.jsonl", i))
		cmd := exec.Command("git", "commit", "--allow-empty", "-q", "-m", fmt.Sprintf("c%d", i))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TRACE2_EVENT="+tracePath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit #%d: %v\n%s", i, err, out)
		}
		data, err := os.ReadFile(tracePath)
		if err != nil {
			t.Fatalf("read trace2 event log for commit #%d: %v", i, err)
		}
		spawns += countMaintenanceSpawns(t, data)
	}

	if spawns != 0 {
		t.Fatalf("git maintenance spawned %d time(s) across %d commits in a repo built by InitRepo; want 0 (gc.auto=0 + maintenance.auto=false should suppress every spawn — see package doc)", spawns, commits)
	}
}

// countMaintenanceSpawns scans a GIT_TRACE2_EVENT JSONL log for
// child_start events whose argv names `maintenance` as a git subcommand
// (covers both the `git maintenance ...` and the resolved
// `.../git-core/git maintenance ...` argv shapes git's trace2 emits).
func countMaintenanceSpawns(t testing.TB, trace []byte) int {
	t.Helper()
	n := 0
	for _, line := range strings.Split(string(trace), "\n") {
		if line == "" {
			continue
		}
		var ev struct {
			Event string   `json:"event"`
			Argv  []string `json:"argv"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // trace2 emits event shapes this struct doesn't model; skip them
		}
		if ev.Event != "child_start" {
			continue
		}
		for _, a := range ev.Argv {
			if a == "maintenance" {
				n++
				break
			}
		}
	}
	return n
}
