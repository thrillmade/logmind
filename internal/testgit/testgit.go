// Package testgit is the single place every test in this module creates a
// real git repository. Route every `git init` / `git clone` a test needs
// through InitRepo / CloneRepo (or, when a repo is built some other way,
// call DisableMaintenance directly) so the repo can never end up missing
// the two config keys below.
//
// # Why this exists
//
// `git commit` (and merge/rebase/fetch/receive-pack) unconditionally calls
// run_auto_maintenance(), gated by `maintenance.auto` (default true) — a
// SEPARATE key from `gc.auto`. The spawned `git maintenance` process can
// daemonize (`maintenance.autoDetach`, default true) into a grandchild
// outside git's process tree, and is still writing into `.git/objects`
// when a test returns. That makes `t.TempDir()`'s RemoveAll fail with
// "directory not empty" — a CLEANUP error, not an assertion failure,
// landing on whichever test happens to lose the race. Because the spawned
// process finishes in single-digit milliseconds when the machine isn't
// loaded, this never reproduces locally; it only shows up as a flake on a
// busy CI runner.
//
// Measured with GIT_TRACE2_EVENT on git 2.39.5, same repository, 5 commits:
//
//	gc.auto=0 alone                    -> 5 of 5 commits still spawned `git maintenance`
//	gc.auto=0 + maintenance.auto=false -> 0 of 5
//	fix removed again                  -> 5 of 5
//
// See logmind issue #271 for the incident history: this reddened four pull
// requests across ubuntu and macOS while passing locally every time, and
// took three attempts to land because the first two fixed only the
// helpers already found rather than enumerating every repo-creating test.
// A `git clone` does NOT inherit these keys from its source (gc.auto and
// maintenance.auto are local, per-repo config that `git clone` does not
// copy), so a clone destination needs the same treatment even when the
// repo it was cloned from already called InitRepo.
package testgit

import (
	"os"
	"os/exec"
	"testing"
)

// InitRepo runs `git init <args...>` in dir and immediately disables
// background maintenance in the new repo. Works for bare repos too:
// `InitRepo(t, dir, "--bare", "-q")`.
//
// dir is created first if it doesn't already exist — callers commonly
// name a not-yet-created path (e.g. filepath.Join(t.TempDir(),
// "bare.git")) and expect `git init` to create it, same as plain
// `git init <dir>` would.
//
// Pass dir == "" to init in the test's current working directory (e.g.
// inside withTempCwd) rather than a directory named by path.
func InitRepo(t testing.TB, dir string, args ...string) {
	t.Helper()
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	run(t, dir, append([]string{"init"}, args...)...)
	DisableMaintenance(t, dir)
}

// CloneRepo runs `git clone <args...> <dst>` and immediately disables
// background maintenance in the new clone at dst. args is everything
// that goes between `clone` and the destination — flags plus the source,
// e.g. CloneRepo(t, dst, "--bare", "-q", src).
func CloneRepo(t testing.TB, dst string, args ...string) {
	t.Helper()
	full := append([]string{"clone"}, args...)
	full = append(full, dst)
	run(t, "", full...)
	DisableMaintenance(t, dst)
}

// DisableMaintenance sets gc.auto=0 and maintenance.auto=false on the
// repo at dir. Call this directly — instead of InitRepo/CloneRepo — when
// a test builds its repo some other way this package doesn't wrap: a
// `git init --bare` remote that later receives pushes, a clone made by
// hand, or any repo-creating command this package has no wrapper for
// yet. BOTH keys are required; gc.auto alone does not suppress the spawn
// (see package doc).
func DisableMaintenance(t testing.TB, dir string) {
	t.Helper()
	run(t, dir, "config", "gc.auto", "0")
	run(t, dir, "config", "maintenance.auto", "false")
}

func run(t testing.TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v (dir=%q): %v\n%s", args, dir, err, out)
	}
}
