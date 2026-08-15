// derived_integration_test.go — the headline zero-conflict integration proof
// for the derived-docs-on-main feature (plan Task 9). Two concurrent
// branches each log a decision — after first DIRTYING every derived doc with
// branch-specific content, simulating a stray post-merge/merge-driver regen
// — and both merge into main with zero conflict on any of them.
//
// It dirties and asserts over derivedDocPaths itself rather than naming
// files, so a doc added to that list (docs/timeline-archive.md, the older
// half of the §3.3 rendering split) is covered here the moment it is added,
// instead of quietly sitting outside the only end-to-end proof L1 has.
//
// This is deliberately NOT a naive "log then merge" test: `logmind log`
// alone never rewrites docs/timeline.md, so a naive version (dirty nothing,
// just log + merge) would pass even without L1. By dirtying the file first
// and asserting the resulting HEAD commit's copy is byte-identical to
// main's, L1 (commitDecision's onNonDefaultBranch restore, log.go:886-888)
// is load-bearing for this test: remove the restore and the per-branch
// assertion below fails immediately. It also makes the final merge
// non-trivial — without L1, feat/a and feat/b would each carry their own
// divergent docs/timeline.md content, and merging feat/b into a main
// already fast-forwarded/merged to feat/a's tip would be a genuine
// three-way conflict (both sides differ from the merge-base AND from each
// other), not just a one-side-changed auto-merge.
//
// The zero-conflict invariant this test proves is UNCONDITIONAL (the
// v2.0.0 B6 `derived_docs.mode` per-repo adoption gate is gone — see
// internal/cli/derived.go) — no config declaration is needed for this
// scenario to hold.
package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/gitcli"
)

func TestConcurrentBranches_MergeWithoutDerivedDocConflict(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		// Commit the scaffold (every derived doc, plus main's own decision
		// file) so both branches share the same merge-base copy of them.
		commitAll(t, d, "initial scaffold")

		mainCopies := map[string]string{}
		for _, rel := range derivedDocPaths {
			mainCopies[rel] = readFileStr(t, filepath.Join(d, rel))
		}

		branches := []string{"feat/a", "feat/b"}
		for _, br := range branches {
			runGitIn(t, d, "checkout", "main")
			runGitIn(t, d, "checkout", "-b", br)

			// Dirty EVERY derived doc with branch-specific content BEFORE
			// logging — simulates a stray post-merge/merge-driver regen
			// landing branch-local content in them. Confirm the dirty content
			// actually differs from main's so the later byte-identical
			// assertion is meaningful (not vacuously true).
			for _, rel := range derivedDocPaths {
				dirty := "DIRTY ON " + br + " in " + rel + " — simulated stray regen\n"
				if dirty == mainCopies[rel] {
					t.Fatalf("dirty content for %s accidentally matches main's %s; assertion would be meaningless", br, rel)
				}
				if err := os.WriteFile(filepath.Join(d, rel), []byte(dirty), 0o644); err != nil {
					t.Fatalf("dirty %s on %s: %v", rel, br, err)
				}
			}

			withFakeTTY(t, false, func() {
				f := &logFlags{stage: "all"}
				if err := runLog(d, "decision on "+br, f, true, strings.NewReader(""), io.Discard, io.Discard); err != nil {
					t.Fatalf("runLog on %s: %v", br, err)
				}
			})

			// L1 proof: the resulting HEAD commit's copy of EVERY derived doc
			// must be byte-identical to main's — NOT the dirtied branch
			// content — proving commitDecision's onNonDefaultBranch restore
			// ran over all of them before staging. Without L1 each would still
			// read "DIRTY ON <br>...".
			for _, rel := range derivedDocPaths {
				headCopy, ok := gitcli.ShowFile(d, "HEAD", rel)
				if !ok {
					t.Fatalf("git show HEAD:%s failed on %s", rel, br)
				}
				if headCopy != mainCopies[rel] {
					t.Fatalf("branch %s: HEAD %s diverged from main (L1 did not restore it)\nwant:\n%s\ngot:\n%s", br, rel, mainCopies[rel], headCopy)
				}
			}
		}

		// Merge both branches into main. Zero-conflict invariant: neither
		// merge should touch any derived doc —
		// check the merge's exit code, the absence of "CONFLICT" in git's
		// own output, AND the absence of literal conflict markers in both
		// files on disk afterward.
		runGitIn(t, d, "checkout", "main")
		for _, br := range branches {
			out, _, err := gitcli.RunCaptured(d, "merge", "--no-edit", br)
			if err != nil {
				t.Fatalf("merge %s failed: %v\n%s", br, err, out)
			}
			if strings.Contains(out, "CONFLICT") {
				t.Fatalf("merge %s reported a conflict:\n%s", br, out)
			}
			for _, p := range derivedDocPaths {
				content := readFileStr(t, filepath.Join(d, p))
				if strings.Contains(content, "<<<<<<<") {
					t.Fatalf("merge %s left conflict markers in %s:\n%s", br, p, content)
				}
			}
		}
	})
}

// TestBranch_CannotCommitAChangeToTheTimelineArchive names
// docs/timeline-archive.md literally, on purpose.
//
// TestConcurrentBranches_MergeWithoutDerivedDocConflict above iterates
// derivedDocPaths, which makes it grow with that list — but also shrink with
// it: deleting a path from the list deletes it from that test's coverage in
// the same edit, and the suite stays green while a derived file quietly
// becomes editable on a branch. This one fails in exactly that case.
//
// docs/timeline-archive.md is the older half of the SPEC §3.3 rendering
// split, and §3.3 governs it identically: "A non-default branch MUST NOT
// modify any derived file — the history, its archive, or the map."
func TestBranch_CannotCommitAChangeToTheTimelineArchive(t *testing.T) {
	const rel = "docs/timeline-archive.md"
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		commitAll(t, d, "initial scaffold")

		mainCopy := readFileStr(t, filepath.Join(d, rel))
		if strings.TrimSpace(mainCopy) == "" {
			t.Fatalf("%s is empty on main; the byte-identical assertion below would be meaningless", rel)
		}

		runGitIn(t, d, "checkout", "-b", "feat/archive-edit")

		dirty := "DIRTY — a branch-local edit to the timeline archive\n"
		if dirty == mainCopy {
			t.Fatalf("dirty content accidentally matches main's %s", rel)
		}
		if err := os.WriteFile(filepath.Join(d, rel), []byte(dirty), 0o644); err != nil {
			t.Fatalf("dirty %s: %v", rel, err)
		}

		withFakeTTY(t, false, func() {
			f := &logFlags{stage: "all"}
			if err := runLog(d, "a decision on the branch", f, true, strings.NewReader(""), io.Discard, io.Discard); err != nil {
				t.Fatalf("runLog: %v", err)
			}
		})

		committed, ok := gitcli.ShowFile(d, "HEAD", rel)
		if !ok {
			t.Fatalf("git show HEAD:%s failed", rel)
		}
		if committed != mainCopy {
			t.Fatalf("the branch committed a change to %s — a non-default branch must not modify the history's archive (§3.3)\nwant:\n%s\ngot:\n%s",
				rel, mainCopy, committed)
		}
	})
}
