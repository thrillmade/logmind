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
// The correction: L1 goes back to HEAD (see
// TestLog_PreservesManuallyStagedRepairOfDivergedBranch below), and the
// merge-base repair capability moves to `logmind warp` (see
// internal/cli/warp_test.go's TestWarp_RepairsAlreadyDivergedBranch) — the one
// commit-adjacent surface that fetches origin FIRST, so it has a trustworthy
// ref to compute a merge-base against.
//
// v2.0.0 4b-quater (this file's newest addition,
// TestWarpThenLog_PreservesRepairAcrossCommit): moving the repair to
// `logmind warp` reintroduced the exact bug 4b-ter fixed, one layer up.
// warp's repair DELIBERATELY STAGES the two derived docs so the fix rides
// into the next commit — but L1 (and L2b) restored unconditionally, so the
// very next `logmind log` silently undid the repair and recommitted the
// divergence. The fix: L1/L2b now skip any derived-doc path that is
// ALREADY STAGED (gitcli.IsPathStaged / unstagedDerivedDocPaths,
// derived.go) — unstaged means accidental, staged means intentional. See
// commitDecision's doc comment (log.go) for the full account, including
// the accepted trade-off this relaxation makes.
package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/gitcli"
)

// TestLog_PreservesManuallyStagedRepairOfDivergedBranch is the successor to
// TestLog_DoesNotRepairDivergedBranch_RestoresToHead (which itself succeeded
// the removed TestLog_RepairsDivergedBranchDerivedDocs_MergeBaseNotHead) — it
// pins the OPPOSITE outcome yet again, deliberately, as part of the v2.0.0
// 4b-quater fix (see TestWarpThenLog_PreservesRepairAcrossCommit below for
// the headline version of this same fix via `logmind warp`).
//
// Reproduces the same setup as its predecessor: a branch whose HEAD already
// carries a diverged copy of docs/timeline.md (simulating an old binary's
// local regen or a hand edit landed before any guard existed), the user
// follows the CI gate's own repair advice (`git checkout main --
// docs/timeline.md`), then runs `logmind log`.
//
// Pre-4b-quater (what this test used to pin, under its old name): L1
// unconditionally restored to HEAD regardless of git state, UNDOING the
// manual repair and re-affirming the stale content. Post-4b-quater: L1
// skips any derived-doc path that is ALREADY STAGED relative to HEAD — and
// `git checkout <ref> -- <path>` (the manual repair command above, and the
// exact primitive `logmind warp`'s own repair uses) stages its result, not
// just the working tree. L1 now recognizes that staged state as deliberate
// and leaves it alone, so the manual repair SURVIVES the commit.
//
// This is THE HONEST TRADE made concrete on a path that never touches
// `logmind warp` at all: L1 cannot tell "staged because it's a correct,
// deliberate repair" apart from "staged because someone `git add`ed a bad
// copy" — it only ever sees "staged" — see commitDecision's doc comment
// (log.go) for the full write-up of the trade-off this represents. See
// TestLog_DoesNotCommitDirtiedDerivedDocOnBranch for proof L1's ORIGINAL
// job — reverting an UNSTAGED dirty derived doc — is untouched: that
// fixture dirties the working tree only, via os.WriteFile, and never
// stages anything.
func TestLog_PreservesManuallyStagedRepairOfDivergedBranch(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
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
		// `origin/main`. `git checkout <ref> -- <path>` STAGES the result —
		// the same index-writing mechanism `logmind warp`'s repair uses —
		// so this is a proxy for a warp-style repair without invoking warp.
		runGitIn(t, d, "checkout", "main", "--", "docs/timeline.md", "docs/file-structure.md")
		if got := readFileStr(t, timelinePath); got != mainContent {
			t.Fatalf("test setup: working tree after the manual repair = %q; want %q", got, mainContent)
		}
		if out := runGitOut(t, d, "diff", "--cached", "--name-only"); !strings.Contains(out, "docs/timeline.md") {
			t.Fatalf("test setup: manual repair must be staged; git diff --cached --name-only = %q", out)
		}

		// Now `logmind log` — post-4b-quater this PRESERVES the manual
		// repair: L1 sees docs/timeline.md already staged (index differs
		// from HEAD) and skips restoring it, treating the staged state as
		// deliberate rather than accidental.
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
		if gotHeadContent != mainContent {
			t.Fatalf("logmind log did not preserve the staged manual repair: committed docs/timeline.md = %q; want main's content %q (must NOT be the stale content %q L1 used to re-affirm pre-4b-quater)",
				gotHeadContent, mainContent, stale)
		}
		if got := readFileStr(t, timelinePath); got != mainContent {
			t.Fatalf("working-tree docs/timeline.md after logmind log = %q; want the preserved repair content %q", got, mainContent)
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

// TestWarpThenLog_PreservesRepairAcrossCommit is the HEADLINE regression pin
// for the v2.0.0 4b-quater seam: moving the merge-base repair capability to
// `logmind warp` (4b-ter) fixed the staleness bug, but reintroduced the
// ORIGINAL "remediation advice silently no-ops" bug one layer up, because
// warp's repair DELIBERATELY STAGES the two derived docs (see runWarp,
// warp.go) and L1 (commitDecision's restore, log.go) used to restore BOTH
// docs to HEAD unconditionally — undoing warp's staged repair before the
// commit could ever capture it.
//
// This test reproduces the FULL remediation sequence the CI gate's own
// failure message tells a user to run: a branch whose HEAD ALREADY carries
// a diverged docs/timeline.md (simulating a pre-existing bad regen, exactly
// like TestWarp_RepairsAlreadyDivergedBranch's setup), then `logmind warp`
// (fetch + repair to merge-base, staged), then `logmind log` (a completely
// unrelated decision). The resulting COMMIT must carry the merge-base
// content warp staged — NOT the branch's stale divergent HEAD content L1
// would have re-affirmed pre-fix. Before the 4b-quater fix in this PR, this
// test FAILS: L1 unconditionally restores docs/timeline.md to HEAD's stale
// content before staging, so the commit recaptures the divergence and the
// CI gate would fail again with the exact same error — the user's "fix"
// silently did nothing.
//
// NOT covered here: this fixture (via initClonePairScaffolded → scaffoldDocs
// → `logmind init --no-git`) never installs a REAL `.git/hooks/pre-commit`
// script, so it can't exercise L2a. A repo that DOES have the pre-commit
// hook installed (unconditional, the default the moment git is enabled)
// still routes `logmind log`'s own `git commit` through that real hook,
// which restores unconditionally and can undo this same repair on that
// path — see commitDecision's doc comment (log.go) and BuildPreCommitBody's
// (internal/hooks/hooks.go) for that separate, currently-unresolved
// coupling.
func TestWarpThenLog_PreservesRepairAcrossCommit(t *testing.T) {
	origin, repo := initClonePairScaffolded(t)

	// repo's scaffold commit (from initClonePairScaffolded) is the fork
	// point both origin's later advance and the feature branch below share.
	// docs/timeline.md itself is untouched after that commit, so its
	// content right now IS the true merge-base content computed below.
	forkContent := readFileStr(t, filepath.Join(repo, "docs", "timeline.md"))

	// origin's main advances independently of repo's local clone —
	// simulating other work landing on main after this repo forked. The
	// merge-base between this new origin tip and the feature branch below
	// is therefore the fork-point commit above, not this fresher one.
	commitOn(t, origin, "docs/timeline.md", "MAIN-ADVANCED-FRESH\n")

	runGitIn(t, repo, "checkout", "-b", "feat/warp-then-log")
	stale := "STALE — pre-existing diverged content\n"
	mustWriteUnder(t, repo, "docs/timeline.md", stale)
	commitAll(t, repo, "bad regen (pre-existing divergence, before any guard existed)")

	// Step 1 of the CI gate's remediation advice: `logmind warp` — fetches
	// origin, repairs both derived docs to the merge-base, and STAGES the
	// repair (see runWarp).
	if err := runWarp(repo, io.Discard, io.Discard); err != nil {
		t.Fatalf("runWarp: %v", err)
	}
	if got := readFileStr(t, filepath.Join(repo, "docs", "timeline.md")); got != forkContent {
		t.Fatalf("test setup: warp did not repair docs/timeline.md to the merge-base content; got %q want %q", got, forkContent)
	}
	if !isStaged(t, repo, "docs/timeline.md") {
		t.Fatalf("test setup: warp's repair must be staged")
	}

	// Step 2 of the remediation advice: an UNRELATED `logmind log`. THIS is
	// the seam: pre-fix, L1 unconditionally restores docs/timeline.md to
	// HEAD (the stale divergent content) before staging, undoing warp's
	// repair and recommitting the divergence. Post-fix, L1 skips a path
	// that is ALREADY STAGED (warp's deliberate repair) and restores only
	// unstaged paths.
	withFakeTTY(t, false, func() {
		f := &logFlags{stage: "all"}
		if err := runLog(repo, "an unrelated decision", f, true, strings.NewReader(""), io.Discard, io.Discard); err != nil {
			t.Fatalf("runLog: %v", err)
		}
	})

	got, ok := gitcli.ShowFile(repo, "HEAD", "docs/timeline.md")
	if !ok {
		t.Fatalf("ShowFile(HEAD, docs/timeline.md) failed")
	}
	if got != forkContent {
		t.Fatalf("logmind log undid warp's merge-base repair: committed docs/timeline.md = %q; want the merge-base content %q (must NOT be the stale divergent content %q re-affirmed from HEAD, which is what happened pre-4b-quater)",
			got, forkContent, stale)
	}
	if gotDisk := readFileStr(t, filepath.Join(repo, "docs", "timeline.md")); gotDisk != forkContent {
		t.Fatalf("working-tree docs/timeline.md after logmind log = %q; want the preserved repair content %q", gotDisk, forkContent)
	}
}
