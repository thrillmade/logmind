// timeline_split_test.go — the SPEC §3.3 split as a RENDERING, and the rule
// that keeps it from writing files nobody asked for: one `--write`, one file.
//
// These drive the real command and assert on the files it leaves on disk, not
// on internal/timeline's return values: a test at that level would pass its own
// mutation and still go green if the writer started accumulating into the
// archive, or wrote a second file beside the one it was given.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/timeline"
)

// seedTimelineSources writes n entry-block markers into one branch file under
// <dir>/docs, each on its own date so the union order is total.
func seedTimelineSources(t *testing.T, dir string, n int) {
	t.Helper()
	var b strings.Builder
	b.WriteString("← back to [docs/timeline.md](../timeline.md)\n\n")
	for i := 0; i < n; i++ {
		day := i + 1
		date := fmt.Sprintf("2026-%02d-%02d", 1+day/28, 1+day%28)
		fmt.Fprintf(&b, "<!-- logmind-entry-start: %s-entry-%03d -->\n- **%s** — entry %03d\n<!-- logmind-entry-end -->\n\n",
			date, i, date, i)
	}
	mustMkdir(t, filepath.Join(dir, "docs", "decisions-branches"))
	mustWrite(t, filepath.Join(dir, "docs", "decisions-branches", "feat__many.md"), b.String())
}

// runTimelineArgs drives the real cobra command and fails the test on error.
func runTimelineArgs(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetArgs(append([]string{"timeline"}, args...))
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("timeline %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

// runTimelineWrite regenerates BOTH halves the way every real caller does it —
// the hooks, the regen-timeline workflow and `logmind init` all name each file
// they want written, because `--write` writes that file and no other.
func runTimelineWrite(t *testing.T, dir string) {
	t.Helper()
	runTimelineArgs(t, "--write", filepath.Join(dir, "docs", "timeline.md"))
	runTimelineArgs(t, "--write", filepath.Join(dir, "docs", "timeline-archive.md"), "--half", "archive")
}

func countEntries(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Count(string(data), "<!-- logmind-entry-start: ")
}

// TestTimelineWrite_HalvesAreCutAtRecentLimit: naming both files renders the
// §3.3 split, cut at timeline.RecentLimit, from one set of sources.
func TestTimelineWrite_HalvesAreCutAtRecentLimit(t *testing.T) {
	withTempCwd(t, func(d string) {
		total := timeline.RecentLimit + 7
		seedTimelineSources(t, d, total)
		runTimelineWrite(t, d)

		recentPath := filepath.Join(d, "docs", "timeline.md")
		archivePath := filepath.Join(d, "docs", "timeline-archive.md")

		if got := countEntries(t, recentPath); got != timeline.RecentLimit {
			t.Errorf("docs/timeline.md holds %d entries; want %d", got, timeline.RecentLimit)
		}
		if got, want := countEntries(t, archivePath), total-timeline.RecentLimit; got != want {
			t.Errorf("docs/timeline-archive.md holds %d entries; want %d", got, want)
		}
	})
}

// TestTimelineWrite_WritesOnlyTheFileItIsGiven is the rule BLOCK-1 turns on,
// pinned at the level a human uses the command: `--write docs/timeline.md`
// leaves docs/timeline-archive.md exactly as it found it — absent here, so its
// mere existence is the failure.
//
// The old behaviour inferred the archive as a sibling of whatever path
// `--write` named. That is the same code path git's merge driver runs with
// `%A` (a scratch file at the WORKTREE ROOT), so every timeline merge dropped
// a stray `timeline-archive.md` there — and where such a file was tracked, it
// was silently replaced. Both symptoms are one bug: a write to a path no
// caller named.
func TestTimelineWrite_WritesOnlyTheFileItIsGiven(t *testing.T) {
	withTempCwd(t, func(d string) {
		seedTimelineSources(t, d, timeline.RecentLimit+7)

		out := runTimelineArgs(t, "--write", filepath.Join(d, "docs", "timeline.md"))

		sibling := filepath.Join(d, "docs", "timeline-archive.md")
		if pathExists(sibling) {
			t.Errorf("--write docs/timeline.md also wrote %s, a file it was never given.\noutput:\n%s", sibling, out)
		}
		if strings.Contains(out, "timeline-archive.md") {
			t.Errorf("--write docs/timeline.md reported touching the archive:\n%s", out)
		}
	})
}

// TestTimelineWrite_ArchiveIsRenderedNotAccumulated is the ruling that §3.3
// turns on: "Nothing is transferred between files, nothing is consumed."
//
// It corrupts docs/timeline-archive.md with content that is not in the
// sources, regenerates, and requires the file to come back byte-identical to
// the render before the corruption. An implementation that ever READ the
// archive to decide what belongs in it — appending what fell off the end of
// the last timeline, say — would carry the corruption forward, and the
// entry counts would drift on every regeneration.
func TestTimelineWrite_ArchiveIsRenderedNotAccumulated(t *testing.T) {
	withTempCwd(t, func(d string) {
		seedTimelineSources(t, d, timeline.RecentLimit+5)
		runTimelineWrite(t, d)

		archivePath := filepath.Join(d, "docs", "timeline-archive.md")
		first, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}

		mustWrite(t, archivePath,
			"# vandalised\n\n<!-- logmind-entry-start: 1999-01-01-not-in-any-source -->\n"+
				"- **1999-01-01** — not in any source\n<!-- logmind-entry-end -->\n")

		runTimelineWrite(t, d)

		second, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("re-read archive: %v", err)
		}
		if !bytes.Equal(first, second) {
			t.Errorf("the archive did not come back to its rendered content — it is being accumulated, not rendered\n--- want ---\n%s\n--- got ---\n%s", first, second)
		}
		if strings.Contains(string(second), "not-in-any-source") {
			t.Errorf("the archive kept content that is in no source file — it was read as an input")
		}
	})
}

// TestTimelineWrite_IsIdempotentOnDisk: a second regeneration over unchanged
// sources leaves both files byte-identical. §3.3 requires regeneration to do
// nothing when the bytes already match — "without that, its own push starts
// the next run and the loop never stops."
func TestTimelineWrite_IsIdempotentOnDisk(t *testing.T) {
	withTempCwd(t, func(d string) {
		seedTimelineSources(t, d, timeline.RecentLimit+3)
		runTimelineWrite(t, d)

		paths := []string{
			filepath.Join(d, "docs", "timeline.md"),
			filepath.Join(d, "docs", "timeline-archive.md"),
		}
		before := map[string][]byte{}
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read %s: %v", p, err)
			}
			before[p] = data
		}

		runTimelineWrite(t, d)

		for _, p := range paths {
			after, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("re-read %s: %v", p, err)
			}
			if !bytes.Equal(before[p], after) {
				t.Errorf("%s changed on a no-op regeneration", p)
			}
		}
	})
}

// TestTimelineCheck_CatchesAStaleArchive: the archive has its own gate, run
// the same way it is written — `--write <the archive> --half archive
// --check`. A CI recipe that only checked docs/timeline.md would wave a
// drifted archive through, so the archive's own invocation has to be able to
// see it.
func TestTimelineCheck_CatchesAStaleArchive(t *testing.T) {
	withTempCwd(t, func(d string) {
		seedTimelineSources(t, d, timeline.RecentLimit+4)
		runTimelineWrite(t, d)

		archivePath := filepath.Join(d, "docs", "timeline-archive.md")
		// Only the archive drifts; docs/timeline.md stays current.
		mustWrite(t, archivePath, "# stale\n")

		var out bytes.Buffer
		root := NewRootCmd()
		root.SetArgs([]string{"timeline", "--write", archivePath, "--half", "archive", "--check"})
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err == nil {
			t.Fatalf("--check passed with a stale docs/timeline-archive.md:\n%s", out.String())
		}
		if !strings.Contains(out.String(), "timeline-archive.md is stale") {
			t.Errorf("--check did not name the stale file; got:\n%s", out.String())
		}
		// The remediation has to be the command that FIXES this file. Advice
		// that drops `--half archive` writes the RECENT timeline into the
		// archive's path when run as printed.
		if !strings.Contains(out.String(), "--half archive") {
			t.Errorf("the stale-archive advice omits --half archive, so running it as printed writes the wrong half:\n%s", out.String())
		}
	})
}

// TestTimelineCheck_JudgesOnlyTheHalfItWasGiven is the symmetric half of the
// rule: `--check` compares the file `--write` named against the rendering
// `--half` selected, and nothing else. A check that also judged a sibling
// would fail a repo for a file the caller never asked it to govern.
func TestTimelineCheck_JudgesOnlyTheHalfItWasGiven(t *testing.T) {
	withTempCwd(t, func(d string) {
		seedTimelineSources(t, d, timeline.RecentLimit+4)
		runTimelineWrite(t, d)

		// The archive drifts. docs/timeline.md's own gate must still pass:
		// it is current, and the archive is not its business.
		mustWrite(t, filepath.Join(d, "docs", "timeline-archive.md"), "# stale\n")

		var out bytes.Buffer
		root := NewRootCmd()
		root.SetArgs([]string{"timeline", "--write", filepath.Join(d, "docs", "timeline.md"), "--check"})
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("--check on a current docs/timeline.md failed over a sibling it does not govern: %v\n%s", err, out.String())
		}
	})
}
