// derived_repair_test.go — the v2.0.0 4b-ter reversal: commitDecision's L1
// restore (log.go) must target HEAD, NOT the merge-base with the default
// branch, on the commit path. The short-lived 4b-bis "repair-path fix"
// pointed this restore at gitcli.DefaultBranchMergeBase so an already-
// diverged branch could self-heal on its next `logmind log` — but that
// target depends on refs/remotes/origin/<default> being CURRENT, and
// `logmind log` is deliberately network-free (no implicit `git fetch`
// anywhere on its path). On a clone that hasn't fetched recently, the
// "merge-base" 4b-bis computed was stale, so the restore could silently
// commit an OLDER snapshot than the branch's TRUE merge-base — actively
// writing WRONG bytes, and typically FAILING the very CI gate the restore
// exists to satisfy. Before 4b-bis, a wrong restore (to HEAD, on an
// undiverged branch) was a no-op; 4b-bis made it strictly worse.
//
// The correction: L1 goes back to HEAD (see TestLog_DoesNotRepairDivergedBranch
// below), and the merge-base repair capability moves to `logmind warp` (see
// internal/cli/warp_test.go's TestWarp_RepairsAlreadyDivergedBranch) — the one
// commit-adjacent surface that fetches origin FIRST, so it has a trustworthy
// ref to compute a merge-base against.
package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/gitcli"
)

// TestLog_DoesNotRepairDivergedBranch_RestoresToHead is the successor to the
// removed TestLog_RepairsDivergedBranchDerivedDocs_MergeBaseNotHead — it pins
// the OPPOSITE outcome, deliberately. Reproduces the same setup (a branch
// whose HEAD already carries a diverged copy of docs/timeline.md, simulating
// an old binary's local regen or a hand edit landed before any guard
// existed), the user follows the CI gate's own repair advice (`git checkout
// main -- docs/timeline.md`), then runs `logmind log`. Post-4b-ter, L1
// restores to HEAD — UNDOING that manual repair, re-affirming the stale
// content — because L1's job is narrower than "repair": it only has to keep
// an ALREADY-clean branch clean (stop a stray dirty copy from riding into
// THIS commit), a guarantee it can make entirely from local, already-known
// state. Repairing a branch that has ALREADY diverged is `logmind warp`'s
// job now (see TestWarp_RepairsAlreadyDivergedBranch in warp_test.go) — the
// one surface with a just-fetched origin/<default> in hand.
func TestLog_DoesNotRepairDivergedBranch_RestoresToHead(t *testing.T) {
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

		// Sanity: HEAD really does carry the stale content, and it differs
		// from main's — otherwise this test would prove nothing.
		headBefore, ok := gitcli.ShowFile(d, "HEAD", "docs/timeline.md")
		if !ok {
			t.Fatalf("ShowFile(HEAD, docs/timeline.md) failed")
		}
		if headBefore != stale {
			t.Fatalf("test setup: HEAD content = %q; want the staged stale content %q", headBefore, stale)
		}

		// The manual repair attempt: follow the CI gate's own advice, using
		// the local `main` branch as the no-origin-remote stand-in for
		// `origin/main`.
		runGitIn(t, d, "checkout", "main", "--", "docs/timeline.md", "docs/file-structure.md")
		if got := readFileStr(t, timelinePath); got != mainContent {
			t.Fatalf("test setup: working tree after the manual repair = %q; want %q", got, mainContent)
		}

		// Now `logmind log` — post-4b-ter this UNDOES the manual repair,
		// restoring docs/timeline.md back to HEAD's stale content, because
		// L1 no longer consults the merge-base.
		withFakeTTY(t, false, func() {
			f := &logFlags{stage: "all"}
			if err := runLog(d, "attempted repair of the diverged derived docs", f, true, strings.NewReader(""), io.Discard, io.Discard); err != nil {
				t.Fatalf("runLog: %v", err)
			}
		})

		gotHeadContent, ok := gitcli.ShowFile(d, "HEAD", "docs/timeline.md")
		if !ok {
			t.Fatalf("ShowFile(HEAD, docs/timeline.md) failed after logmind log")
		}
		if gotHeadContent != stale {
			t.Fatalf("logmind log did not restore to HEAD as expected: committed docs/timeline.md = %q; want the pre-existing stale content %q (L1 must re-affirm, not repair) — main's content was %v",
				gotHeadContent, stale, gotHeadContent == mainContent)
		}
		if got := readFileStr(t, timelinePath); got != stale {
			t.Fatalf("working-tree docs/timeline.md after logmind log = %q; want the re-affirmed stale content %q", got, stale)
		}
	})
}

// TestLog_CommitPathDoesNotDependOnOriginRef pins the staleness reasoning
// that MOTIVATED the 4b-ter reversal: refs/remotes/origin/<default> is NEVER
// refreshed on `logmind log`'s network-free hot path, so a restore target
// computed from it can only be as fresh as the last `git fetch` happened to
// leave behind — which, this test proves, can be arbitrarily WRONG without
// affecting a clean branch's commit at all. It DEFORMS
// refs/remotes/origin/main to point at a fabricated commit carrying WRONG
// derived-doc content, then confirms `logmind log` on an otherwise-CLEAN
// branch (HEAD's derived-doc content already correct, no local divergence)
// neither reads nor is influenced by that ref: the committed content is
// HEAD's own, not the deformed ref's. If L1 ever again computes a restore
// target via gitcli.DefaultBranchMergeBase (or any other origin-ref-derived
// ref) instead of a bare HEAD, this test fails by picking up the deformed
// ref's wrong content.
func TestLog_CommitPathDoesNotDependOnOriginRef(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		writeDerivedDocsMode(t, d, "integration-point")
		commitAll(t, d, "initial scaffold")

		timelinePath := filepath.Join(d, "docs", "timeline.md")
		correctContent := readFileStr(t, timelinePath)
		headSha := strings.TrimSpace(runGitOut(t, d, "rev-parse", "HEAD"))

		// Fabricate a commit object carrying WRONG derived-doc content —
		// never reached by any real branch history, purely to deform
		// refs/remotes/origin/main into pointing somewhere misleading.
		wrongContent := "WRONG — must never be committed by a clean branch's logmind log\n"
		if err := os.WriteFile(timelinePath, []byte(wrongContent), 0o644); err != nil {
			t.Fatalf("write wrong content: %v", err)
		}
		runGitIn(t, d, "add", "docs/timeline.md")
		wrongTree := strings.TrimSpace(runGitOut(t, d, "write-tree"))
		wrongCommit := strings.TrimSpace(runGitOut(t, d, "commit-tree", wrongTree, "-p", headSha, "-m", "deform"))

		// The fabrication above touched the index/working tree — put both
		// back to the real, committed state before proceeding; only the
		// fabricated commit object (and the ref we point at it next) should
		// carry the wrong content.
		runGitIn(t, d, "reset", "--hard", "HEAD")

		// Deform refs/remotes/origin/main to point at the fabricated wrong
		// commit — simulating a stale (or simply incorrect) last-known
		// fetch. gitcli.DefaultBranchMergeBase's resolution order tries
		// exactly this ref first.
		runGitIn(t, d, "update-ref", "refs/remotes/origin/main", wrongCommit)

		runGitIn(t, d, "checkout", "-b", "feat/clean")

		withFakeTTY(t, false, func() {
			f := &logFlags{stage: "all"}
			if err := runLog(d, "a clean decision, unrelated to the derived docs", f, true, strings.NewReader(""), io.Discard, io.Discard); err != nil {
				t.Fatalf("runLog: %v", err)
			}
		})

		got, ok := gitcli.ShowFile(d, "HEAD", "docs/timeline.md")
		if !ok {
			t.Fatalf("ShowFile(HEAD, docs/timeline.md) failed")
		}
		if got != correctContent {
			t.Fatalf("logmind log wrote bytes influenced by the deformed origin ref: got %q; want the real HEAD content %q (the deformed ref carried %q)",
				got, correctContent, wrongContent)
		}
	})
}
