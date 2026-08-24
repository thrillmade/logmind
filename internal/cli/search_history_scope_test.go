// search_history_scope_test.go — regressions for the two ways `logmind
// search` stopped finding decisions that are still on disk.
//
// Both are pinned on the RENDERED OUTPUT of the real `logmind search` command
// run inside a real git repo from a real non-default branch — the exact shape
// of the user's symptom ("search finds nothing"). A test on searchSources'
// return value would pass its own mutation and still go green when the bug
// ships again, because the harm is in what the command PRINTS.
//
// Covered:
//   - SPEC §3.2 made main a branch, moving its decisions into
//     docs/decisions-branches/main.md. Scanning only the current branch's file
//     made every decision ever logged on the default branch invisible from a
//     feature branch, which is where agents work.
//   - The default branch is RESOLVED (gitcli.DefaultBranch), never hardcoded
//     to "main" — a repo whose default is `trunk` must find its own history.
//   - Being ON the default branch must not double-count it.
//   - docs/decisions-archive.md, the retired rotation overflow, stays
//     readable: "a decision written is a decision kept" (SPEC §3.2).
package cli

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mustGitIn runs one git command in dir and fails the test on error. Local to
// this file: the shared repo-creation helpers are owned elsewhere, and these
// tests only need to nudge an already-created repo.
func mustGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// mustNotContain is the refutation half of mustContain.
func mustNotContain(t *testing.T, body, needle string) {
	t.Helper()
	if strings.Contains(body, needle) {
		t.Errorf("output unexpectedly contains %q:\n%s", needle, body)
	}
}

// TestSearch_FindsDefaultBranchHistoryFromAFeatureBranch is the regression for
// the layout collapse: a decision logged on the default branch MUST still be
// found by `logmind search` after checking out a feature branch.
//
// The CONTROL in the same repo and the same invocation is the feature
// branch's own decision — it proves the search machinery works here, so a
// zero for the main-log term is a scope bug and not a broken fixture.
func TestSearch_FindsDefaultBranchHistoryFromAFeatureBranch(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)

		// A decision logged on the default branch, through the real command,
		// so it lands wherever `logmind log` actually puts it today.
		withFakeTTY(t, false, func() { logOnce(t, "Adopt dogfood-workflows for CI") })
		mainFile := filepath.Join(d, "docs", "decisions-branches", "main.md")
		if !pathExists(mainFile) {
			t.Fatalf("fixture precondition: %s was not written by logmind log", mainFile)
		}

		checkoutBranch(t, d, "feat/unrelated")
		withFakeTTY(t, false, func() { logOnce(t, "Add rate limiting to the API") })

		// THE REGRESSION: main's history from a feature branch.
		body := runSearchCmd(t, "dogfood-workflows")
		mustContain(t, body, "docs/decisions-branches/main.md")
		mustNotContain(t, body, "No matches found")

		// CONTROL: the branch's own decision, same repo, same branch, same
		// command. If this ever goes to zero the fixture is broken, not the
		// scope.
		control := runSearchCmd(t, "rate limiting")
		mustContain(t, control, "docs/decisions-branches/feat__unrelated.md")
		mustNotContain(t, control, "No matches found")
	})
}

// TestSearch_ResolvesDefaultBranch_NotHardcodedMain: the default branch is
// whatever the repo says it is. This repo's default is `trunk` and it has NO
// main.md at all, so a fix that hardcoded "main" finds nothing here while the
// resolved one finds the decision.
func TestSearch_ResolvesDefaultBranch_NotHardcodedMain(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		// Point the repo's default at `trunk` the way a clone does, via
		// origin/HEAD — gitcli.DefaultBranch's first resolution step. Done
		// BEFORE scaffolding so `logmind init` seeds trunk.md, never main.md.
		mustGitIn(t, d, "branch", "-m", "trunk")
		mustGitIn(t, d, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
		scaffoldDocs(t)

		withFakeTTY(t, false, func() { logOnce(t, "Pick trunk-based development") })
		trunkFile := filepath.Join(d, "docs", "decisions-branches", "trunk.md")
		if !pathExists(trunkFile) {
			t.Fatalf("fixture precondition: %s was not written by logmind log", trunkFile)
		}
		if pathExists(filepath.Join(d, "docs", "decisions-branches", "main.md")) {
			t.Fatal("fixture precondition: this repo must have no main.md, so a hardcoded \"main\" cannot pass")
		}

		checkoutBranch(t, d, "feat/unrelated")

		body := runSearchCmd(t, "trunk-based")
		mustContain(t, body, "docs/decisions-branches/trunk.md")
		mustNotContain(t, body, "No matches found")
	})
}

// TestSearch_OnDefaultBranch_DoesNotDoubleCount: the default-branch source and
// the current-branch source resolve to the SAME path on the default branch.
// Path dedup must collapse them, or every hit on main would be reported twice.
func TestSearch_OnDefaultBranch_DoesNotDoubleCount(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		withFakeTTY(t, false, func() { logOnce(t, "Adopt dogfood-workflows for CI") })

		// The term appears exactly ONCE on disk: `logmind log` writes the
		// §1.6.3 branch-summary marker only on a non-default branch, so on
		// main there is just the "## " header. The default-branch source and
		// the current-branch source both resolve to main.md here, so without
		// path dedup this reports 2.
		body := runSearchCmd(t, "dogfood-workflows")
		mustContain(t, body, "Found 1 match for: dogfood-workflows")
		mustNotContain(t, body, "Found 2 matches")
	})
}

// TestSearch_FindsLegacyArchive: docs/decisions-archive.md holds real
// decisions in any repo that rotated under the retired `max_recent: 20`
// default. Nothing writes it any more, but every read path still reads it —
// dropping it would silently lose history on upgrade.
//
// Run from a feature branch, because that is where the loss was observable.
func TestSearch_FindsLegacyArchive(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
			"# Decision Archive\n\n## 2025-01-01 09:00 - Rotate logs with logrotate\n\n**Reasoning:** archived-rationale\n\n---\n")

		checkoutBranch(t, d, "feat/unrelated")
		withFakeTTY(t, false, func() { logOnce(t, "Add rate limiting to the API") })

		body := runSearchCmd(t, "archived-rationale")
		mustContain(t, body, "docs/decisions-archive.md")
		mustNotContain(t, body, "No matches found")

		// CONTROL: the branch's own decision is found by the same command, so
		// a zero above would be scope and not a broken fixture.
		control := runSearchCmd(t, "rate limiting")
		mustContain(t, control, "docs/decisions-branches/feat__unrelated.md")
	})
}
