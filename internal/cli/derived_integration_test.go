// derived_integration_test.go — the headline zero-conflict integration proof
// for the derived-docs-on-main feature (plan Task 9). Two concurrent
// branches each log a decision — after first DIRTYING docs/timeline.md with
// branch-specific content, simulating a stray post-merge/merge-driver regen
// — and both merge into main with zero conflict on the two derived docs
// (docs/timeline.md, docs/file-structure.md).
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
		// Commit the scaffold (decisions.md, timeline.md, file-structure.md)
		// on main so both branches share the same merge-base copy of the
		// derived docs.
		commitAll(t, d, "initial scaffold")

		timelinePath := filepath.Join(d, "docs", "timeline.md")
		mainTimeline := readFileStr(t, timelinePath)

		branches := []string{"feat/a", "feat/b"}
		for _, br := range branches {
			runGitIn(t, d, "checkout", "main")
			runGitIn(t, d, "checkout", "-b", br)

			// Dirty docs/timeline.md with branch-specific content BEFORE
			// logging — simulates a stray post-merge/merge-driver regen
			// landing branch-local content in the derived doc. Confirm the
			// dirty content actually differs from main's so the later
			// byte-identical assertion is meaningful (not vacuously true).
			dirty := "DIRTY ON " + br + " — simulated stray regen\n"
			if dirty == mainTimeline {
				t.Fatalf("dirty content for %s accidentally matches main's timeline.md; assertion would be meaningless", br)
			}
			if err := os.WriteFile(timelinePath, []byte(dirty), 0o644); err != nil {
				t.Fatalf("dirty docs/timeline.md on %s: %v", br, err)
			}

			withFakeTTY(t, false, func() {
				f := &logFlags{stage: "all"}
				if err := runLog(d, "decision on "+br, f, true, strings.NewReader(""), io.Discard, io.Discard); err != nil {
					t.Fatalf("runLog on %s: %v", br, err)
				}
			})

			// L1 proof: the resulting HEAD commit's docs/timeline.md must be
			// byte-identical to main's — NOT the dirtied branch content —
			// proving commitDecision's onNonDefaultBranch restore ran before
			// staging. Without L1 this would still read "DIRTY ON <br>...".
			headTimeline, ok := gitcli.ShowFile(d, "HEAD", "docs/timeline.md")
			if !ok {
				t.Fatalf("git show HEAD:docs/timeline.md failed on %s", br)
			}
			if headTimeline != mainTimeline {
				t.Fatalf("branch %s: HEAD docs/timeline.md diverged from main (L1 did not restore it)\nwant:\n%s\ngot:\n%s", br, mainTimeline, headTimeline)
			}
		}

		// Merge both branches into main. Zero-conflict invariant: neither
		// merge should touch docs/timeline.md or docs/file-structure.md —
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
