// timeline_split_test.go — `logmind timeline --write` writes BOTH halves of
// the SPEC §3.3 split, and the older half is a rendering rather than a place
// entries are moved to.
//
// These drive the real command and assert on the two files it leaves on disk,
// not on internal/timeline's return values: a test at that level would pass
// its own mutation and still go green if the writer stopped writing the
// archive, or started accumulating into it.
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

func runTimelineWrite(t *testing.T, dir string) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetArgs([]string{"timeline", "--write", filepath.Join(dir, "docs", "timeline.md")})
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("timeline --write: %v\n%s", err, out.String())
	}
}

func countEntries(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Count(string(data), "<!-- logmind-entry-start: ")
}

// TestTimelineWrite_WritesBothHalves: one `--write` produces both files, cut
// at timeline.RecentLimit. There is no flag to ask for one without the other,
// which is what makes "both regenerate together" (§3.3) structural instead of
// something every call site has to remember.
func TestTimelineWrite_WritesBothHalves(t *testing.T) {
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

// TestTimelineWrite_IsIdempotentOnDisk: a second `--write` over unchanged
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

// TestTimelineCheck_CatchesAStaleArchive: --check judges BOTH halves. An
// archive that has drifted from what the sources render is stale, and a gate
// that only compares the recent half would wave it through.
func TestTimelineCheck_CatchesAStaleArchive(t *testing.T) {
	withTempCwd(t, func(d string) {
		seedTimelineSources(t, d, timeline.RecentLimit+4)
		runTimelineWrite(t, d)

		// Only the archive drifts; docs/timeline.md stays current.
		mustWrite(t, filepath.Join(d, "docs", "timeline-archive.md"), "# stale\n")

		var out bytes.Buffer
		root := NewRootCmd()
		root.SetArgs([]string{"timeline", "--write", filepath.Join(d, "docs", "timeline.md"), "--check"})
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err == nil {
			t.Fatalf("--check passed with a stale docs/timeline-archive.md:\n%s", out.String())
		}
		if !strings.Contains(out.String(), "timeline-archive.md is stale") {
			t.Errorf("--check did not name the stale file; got:\n%s", out.String())
		}
	})
}
