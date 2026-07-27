// derived_repair_test.go — the v2.0.0 4b-bis repair-path fix: commitDecision's
// L1 restore (log.go) and guardCommitHarness's L2b restore (guard_commit.go)
// must target the merge-base with the default branch, NOT HEAD, on a branch
// that ALREADY diverged before this `logmind log`/commit runs. Restoring to
// HEAD in that case silently re-affirms the divergence — exactly backwards
// for the CI gate's own repair advice ("git checkout origin/main --
// docs/timeline.md docs/file-structure.md, then commit").
package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/gitcli"
)

// TestLog_RepairsDivergedBranchDerivedDocs_MergeBaseNotHead reproduces the
// bug hand-found on a real PR: a branch whose HEAD already carries a
// diverged copy of docs/timeline.md (simulating an old binary's local regen,
// or a hand edit, having been committed on the branch BEFORE this fix
// existed). The user follows the CI gate's advice — `git checkout main --
// docs/timeline.md` — then runs `logmind log`. The fix must survive: the
// resulting commit's docs/timeline.md must be main's content (the
// merge-base), not the stale diverged HEAD content a HEAD-targeted restore
// would silently re-affirm.
func TestLog_RepairsDivergedBranchDerivedDocs_MergeBaseNotHead(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		writeDerivedDocsMode(t, d, "integration-point")
		commitAll(t, d, "initial scaffold")

		timelinePath := filepath.Join(d, "docs", "timeline.md")
		mainContent := readFileStr(t, timelinePath)

		runGitIn(t, d, "checkout", "-b", "feat/diverged")

		// Simulate the pre-existing divergence: a commit ALREADY on this
		// branch's HEAD carries a bad copy of the derived doc — this is the
		// state the CI gate's blocking check would have caught on push.
		stale := "STALE DIVERGED CONTENT — pre-existing bad regen\n"
		if stale == mainContent {
			t.Fatalf("test setup: stale content accidentally matches main's — assertions would be meaningless")
		}
		if err := os.WriteFile(timelinePath, []byte(stale), 0o644); err != nil {
			t.Fatalf("write diverged timeline.md: %v", err)
		}
		commitAll(t, d, "bad regen (pre-existing divergence)")

		// Sanity: HEAD really does differ from the merge-base with main now
		// — otherwise this test would prove nothing.
		mergeBase := gitcli.DefaultBranchMergeBase(d)
		baseContent, ok := gitcli.ShowFile(d, mergeBase, "docs/timeline.md")
		if !ok {
			t.Fatalf("ShowFile(mergeBase, docs/timeline.md) failed")
		}
		if baseContent != mainContent {
			t.Fatalf("merge-base content = %q; want main's scaffolded content %q", baseContent, mainContent)
		}
		headContent, ok := gitcli.ShowFile(d, "HEAD", "docs/timeline.md")
		if !ok {
			t.Fatalf("ShowFile(HEAD, docs/timeline.md) failed")
		}
		if headContent != stale {
			t.Fatalf("test setup: HEAD content = %q; want the staged stale content %q", headContent, stale)
		}

		// The repair: follow the CI gate's own advice, using the local
		// `main` branch as the no-origin-remote stand-in for `origin/main`
		// (this is the local-dev repro; DefaultBranchMergeBase falls back to
		// the same local ref when there's no origin tracking branch).
		runGitIn(t, d, "checkout", "main", "--", "docs/timeline.md", "docs/file-structure.md")
		if got := readFileStr(t, timelinePath); got != mainContent {
			t.Fatalf("test setup: working tree after the manual repair = %q; want %q", got, mainContent)
		}

		// Now the fix, via `logmind log` — this is what must NOT undo the
		// repair.
		withFakeTTY(t, false, func() {
			f := &logFlags{stage: "all"}
			if err := runLog(d, "repair the diverged derived docs", f, true, strings.NewReader(""), io.Discard, io.Discard); err != nil {
				t.Fatalf("runLog: %v", err)
			}
		})

		gotHeadContent, ok := gitcli.ShowFile(d, "HEAD", "docs/timeline.md")
		if !ok {
			t.Fatalf("ShowFile(HEAD, docs/timeline.md) failed after logmind log")
		}
		if gotHeadContent != mainContent {
			t.Fatalf("logmind log undid the repair: committed docs/timeline.md = %q; want main's content %q (the merge-base) — got the stale content back = %v",
				gotHeadContent, mainContent, gotHeadContent == stale)
		}
		if got := readFileStr(t, timelinePath); got != mainContent {
			t.Fatalf("working-tree docs/timeline.md after logmind log = %q; want %q", got, mainContent)
		}
	})
}
