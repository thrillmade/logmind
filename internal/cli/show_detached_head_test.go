// show_detached_head_test.go — `logmind show` must reach the repository's
// decisions when HEAD is detached.
//
// This is not a corner: `actions/checkout` checks out a detached HEAD on
// `pull_request` events, so it is the state of EVERY CI run. resolveDecisionsPath
// routes a detached HEAD to docs/decisions.md — correct for `log`, which needs
// a file to write when no branch names one — and `show` follows it to the same
// base. Since §3.2 that file holds no decisions, so bare `show` answered "No
// decisions logged yet on this branch." and `--json` answered
// `{"decisions": null}` with the entire record sitting in
// docs/decisions-branches/, one flag away.
//
// The `--all` band is a NARROWING of the enumeration every other read path
// (Collect, timeline, search) does unconditionally — it means "branches other
// than the one I am on". Detached, there is no branch for it to be other
// than, so the narrowing has nothing to express and the branch files are
// simply the decisions.
package cli

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// detachHead commits whatever is in the worktree and detaches HEAD onto that
// commit — the shape `actions/checkout` leaves behind on a pull_request run.
func detachHead(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-m", "fixture [skip-logmind]"},
		{"checkout", "--detach"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Fatal("fixture precondition: HEAD is still a symbolic ref, so this is not a detached checkout")
	}
}

// TestShow_DetachedHead_ReachesTheDecisions pins all three output modes on the
// state CI is always in.
func TestShow_DetachedHead_ReachesTheDecisions(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		mustWrite(t, filepath.Join(d, "docs", "decisions-branches", "main.md"),
			"# main\n\n## 2026-05-05 09:00 - Chose Postgres over MySQL\n\n**Reasoning:** detached-head-rationale\n\n---\n")
		detachHead(t, d)

		// Raw stream.
		body := runShowCmd(t)
		mustContain(t, body, "Chose Postgres over MySQL")
		mustContain(t, body, "detached-head-rationale")
		mustNotContain(t, body, "No decisions logged yet on this branch.")

		// --brief.
		brief := runShowCmd(t, "--brief")
		mustContain(t, brief, "Chose Postgres over MySQL")
		mustContain(t, brief, "ok show: 1 decision(s)")

		// --json: the machine surface, and the one that answered null.
		var doc struct {
			Decisions []struct {
				Title  string `json:"title"`
				Source string `json:"source"`
			} `json:"decisions"`
		}
		raw := runShowCmd(t, "--json")
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatalf("show --json emitted invalid JSON: %v\n%s", err, raw)
		}
		if len(doc.Decisions) != 1 {
			t.Fatalf("show --json on a detached HEAD returned %d decisions, want 1\n%s", len(doc.Decisions), raw)
		}
		if doc.Decisions[0].Source != "branch:main" {
			t.Errorf("source = %q, want %q", doc.Decisions[0].Source, "branch:main")
		}
	})
}

// TestShow_OnABranch_StillHidesOtherBranches: the fix must not turn bare
// `show` into `show --all` everywhere. On a real branch the narrowing is
// meaningful and stays — otherwise the flag would have stopped meaning
// anything, and this test is what separates "the narrowing does not apply
// here" from "the narrowing was removed".
func TestShow_OnABranch_StillHidesOtherBranches(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		mustWrite(t, filepath.Join(d, "docs", "decisions-branches", "main.md"),
			"# main\n\n## 2026-05-05 09:00 - A main decision\n\n**Reasoning:** on-main\n\n---\n")
		mustWrite(t, filepath.Join(d, "docs", "decisions-branches", "feat__other.md"),
			"# other\n\n## 2026-05-06 09:00 - Another branch decision\n\n**Reasoning:** elsewhere\n\n---\n")

		bare := runShowCmd(t)
		mustContain(t, bare, "A main decision")
		mustNotContain(t, bare, "Another branch decision")

		// CONTROL: --all does reach it, so the absence above is the narrowing
		// working rather than the file being unreadable.
		mustContain(t, runShowCmd(t, "--all"), "Another branch decision")
	})
}

// TestShow_SkipsAnEntrylessLegacyFile: `logmind init` now scaffolds
// docs/decisions.md as a pointer at the real layout (it exists so a logmind
// older than v2.0 recognises the repo — see init_legacy_sentinel_test.go), and
// the always-on legacy band would otherwise open every freshly-initialised
// repo's `show` with an empty "LEGACY MAIN LOG" section. That band exists to
// surface decisions living in no branch file; a file holding none has nothing
// to surface.
func TestShow_SkipsAnEntrylessLegacyFile(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		mustWrite(t, filepath.Join(d, "docs", "decisions-branches", "main.md"),
			"# main\n\n## 2026-05-05 09:00 - A main decision\n\n**Reasoning:** on-main\n\n---\n")

		if !pathExists(filepath.Join(d, "docs", "decisions.md")) {
			t.Fatal("fixture precondition: init must have scaffolded the pointer file")
		}
		body := runShowCmd(t)
		mustNotContain(t, body, "LEGACY MAIN LOG")
		mustNotContain(t, body, "legacy file(s)")

		// CONTROL: give the same file a real entry and the band must engage —
		// otherwise this test would pass with the legacy band deleted outright.
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"# Decisions\n\n## 2019-05-05 11:11 - A pre-3.2 decision\n\n**Reasoning:** history\n\n---\n")
		withEntries := runShowCmd(t)
		mustContain(t, withEntries, "LEGACY MAIN LOG")
		mustContain(t, withEntries, "A pre-3.2 decision")
	})
}

// TestShow_DetachedHead_DoesNotStreamThePointerBody: the base on a detached
// HEAD IS docs/decisions.md, so without suppressing an entry-less non-branch
// base every CI `show` would open with the pointer's explanation of v1.2.0's
// install sentinel before reaching a decision.
func TestShow_DetachedHead_DoesNotStreamThePointerBody(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		mustWrite(t, filepath.Join(d, "docs", "decisions-branches", "main.md"),
			"# main\n\n## 2026-05-05 09:00 - A main decision\n\n**Reasoning:** on-main\n\n---\n")
		detachHead(t, d)

		body := runShowCmd(t)
		mustNotContain(t, body, "already initialised")
		mustContain(t, body, "A main decision")
		// The first thing printed is the one-line note, not a file body.
		if first, _, _ := strings.Cut(body, "\n"); !strings.HasPrefix(first, "No decisions in the file") {
			t.Errorf("first line = %q, want the short base-is-empty note", first)
		}
	})
}
