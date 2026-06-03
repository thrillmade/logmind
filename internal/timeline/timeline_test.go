package timeline

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thrillmade/logmind/internal/decisions"
)

// -update regenerates the .golden files from current Go output. Use
// `make snapshot` from the repo root to drive it.
var update = flag.Bool("update", false, "rewrite *.golden files from current output")

func TestRenderEmpty(t *testing.T) {
	got := Render(nil, true)
	want := header + emptyBody
	if got != want {
		t.Errorf("Render(empty) =\n%q\nwant\n%q", got, want)
	}
}

// mkEntry is shorthand for a fixture entry. Title is intentionally
// short — golden fixtures stay readable.
func mkEntry(date string, title, srcPath, srcLabel string) decisions.Entry {
	t, _ := time.Parse("2006-01-02 15:04", date)
	return decisions.Entry{
		Date:        t,
		Title:       title,
		SourcePath:  srcPath,
		SourceLabel: srcLabel,
	}
}

func TestRenderBriefMixedMonths(t *testing.T) {
	// Mixed-fixture: 2026-06 has 4 entries (brief elides), 2026-05 has 2
	// (brief renders both), 2026-04 has 1 (brief renders it). Tests the
	// month-count-suffix gate (>=3) and the singular-noun gate (== 1).
	entries := []decisions.Entry{
		// 2026-06 — 4 entries, newest first
		mkEntry("2026-06-04 14:00", "Newest June entry", "decisions.md", "main"),
		mkEntry("2026-06-03 10:00", "Second June entry", "decisions-branches/feat__a.md", "feat/a"),
		mkEntry("2026-06-02 09:00", "Third June entry", "decisions.md", "main"),
		mkEntry("2026-06-01 08:00", "Oldest June entry", "decisions-archive.md", "archive"),
		// 2026-05 — 2 entries (no elision)
		mkEntry("2026-05-31 22:00", "May newer", "decisions.md", "main"),
		mkEntry("2026-05-01 09:00", "May older", "decisions-branches/feat__b.md", "feat/b"),
		// 2026-04 — 1 entry
		mkEntry("2026-04-15 12:00", "Lone April entry", "decisions.md", "main"),
	}
	got := Render(entries, true)
	compareGolden(t, "brief-mixed.golden", got)
}

func TestRenderBriefSingleElision(t *testing.T) {
	// Exactly 3 entries → elided=1 → singular noun "decision".
	entries := []decisions.Entry{
		mkEntry("2026-06-03 14:00", "Newest", "decisions.md", "main"),
		mkEntry("2026-06-02 10:00", "Middle", "decisions.md", "main"),
		mkEntry("2026-06-01 09:00", "Oldest", "decisions.md", "main"),
	}
	got := Render(entries, true)
	compareGolden(t, "brief-singular-elision.golden", got)
}

func TestRenderFullMode(t *testing.T) {
	// Full mode: no count suffix on month header, all entries rendered.
	entries := []decisions.Entry{
		mkEntry("2026-06-03 14:00", "Newest", "decisions.md", "main"),
		mkEntry("2026-06-02 10:00", "Middle", "decisions.md", "main"),
		mkEntry("2026-06-01 09:00", "Oldest", "decisions.md", "main"),
		mkEntry("2026-05-31 22:00", "May entry", "decisions.md", "main"),
	}
	got := Render(entries, false)
	compareGolden(t, "full-mixed.golden", got)
}

func TestRenderTwoElidedMonths(t *testing.T) {
	// Two months each with > 3 entries to verify inter-month spacing.
	entries := []decisions.Entry{
		mkEntry("2026-06-04 14:00", "Jun newest", "decisions.md", "main"),
		mkEntry("2026-06-03 10:00", "Jun mid1", "decisions.md", "main"),
		mkEntry("2026-06-02 09:00", "Jun mid2", "decisions.md", "main"),
		mkEntry("2026-06-01 08:00", "Jun oldest", "decisions.md", "main"),
		mkEntry("2026-05-31 22:00", "May newest", "decisions.md", "main"),
		mkEntry("2026-05-20 18:00", "May mid", "decisions.md", "main"),
		mkEntry("2026-05-01 09:00", "May oldest", "decisions.md", "main"),
	}
	got := Render(entries, true)
	compareGolden(t, "brief-two-elided-months.golden", got)
}

// TestRenderEndsWithSingleNewline pins a property the merge driver
// depends on — the byte just before EOF MUST be a single \n. Catching
// regressions here means the timeline diff never gains a trailing-
// whitespace edit on a fresh regen.
func TestRenderEndsWithSingleNewline(t *testing.T) {
	entries := []decisions.Entry{
		mkEntry("2026-06-01 10:00", "Single entry", "decisions.md", "main"),
	}
	got := Render(entries, true)
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("Render output should end with \\n; got tail = %q", got[len(got)-10:])
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("Render output ended with double-newline; got tail = %q", got[len(got)-10:])
	}
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `make snapshot` to regenerate)", path, err)
	}
	if got != string(want) {
		t.Errorf("output mismatch for %s:\n=== got ===\n%s\n=== want ===\n%s", name, got, string(want))
	}
}
