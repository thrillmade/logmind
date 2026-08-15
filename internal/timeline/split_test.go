// split_test.go — SPEC §3.3's bounded history.
//
// "The history is bounded; the record behind it is not. docs/timeline.md
// carries the 50 most recent entries. Everything older renders to
// docs/timeline-archive.md in the same format under the same rules […] This is
// a split in a rendering, not a move. Nothing is transferred between files,
// nothing is consumed, and no entry changes hands […] Changing where the split
// falls is a regeneration, not a migration."
//
// The three properties that sentence asserts, each pinned here on the rendered
// bytes rather than on the slice arithmetic that produces them:
//
//   - the cut lands exactly at the limit (49 / 50 / 51 entries);
//   - a regeneration is idempotent — the same sources give the same bytes;
//   - moving the limit loses nothing, because it is a pure re-render of one
//     union rather than a transfer between two files.
package timeline

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeNBranchEntries seeds docs/decisions-branches/ with n entry-block
// markers, each on its own distinct date so the union's order is total and
// the split point is unambiguous. Dates ascend with i, so entry n-1 is the
// newest and sorts first.
func writeNBranchEntries(t *testing.T, docs string, n int) {
	t.Helper()
	var b strings.Builder
	b.WriteString("← back\n\n")
	for i := 0; i < n; i++ {
		// 2026-01-01 + i days, kept inside one month range by construction.
		day := i + 1
		date := fmt.Sprintf("2026-%02d-%02d", 1+day/28, 1+day%28)
		b.WriteString(block(fmt.Sprintf("%s-entry-%03d", date, i),
			fmt.Sprintf("- **%s** — entry %03d", date, i)))
		b.WriteString("\n")
	}
	writeDoc(t, docs, "decisions-branches/feat__many.md", b.String())
}

// entryKeys returns the <date>-<slug> keys of every entry-block in a rendered
// timeline, in document order.
func entryKeys(t *testing.T, rendered string) []string {
	t.Helper()
	var sink bytes.Buffer
	blocks := extractEntryBlocks(rendered, &sink)
	keys := make([]string, 0, len(blocks))
	for _, b := range blocks {
		keys = append(keys, b.Key)
	}
	return keys
}

// TestGenerate_SplitBoundary walks the cut across the limit itself: one under,
// exactly on it, and one over. Under and on, the archive holds nothing; one
// over, it holds exactly the single oldest entry and the recent half is
// unchanged from the on-the-limit case.
func TestGenerate_SplitBoundary(t *testing.T) {
	const limit = 10
	for _, n := range []int{limit - 1, limit, limit + 1} {
		t.Run(fmt.Sprintf("%d_entries", n), func(t *testing.T) {
			docs := filepath.Join(t.TempDir(), "docs")
			writeNBranchEntries(t, docs, n)

			var stderr bytes.Buffer
			recent, archive, err := generateAt(docs, limit, &stderr)
			if err != nil {
				t.Fatalf("generateAt: %v", err)
			}
			recentKeys := entryKeys(t, recent)
			archiveKeys := entryKeys(t, archive)

			wantRecent := n
			if wantRecent > limit {
				wantRecent = limit
			}
			if len(recentKeys) != wantRecent {
				t.Errorf("recent holds %d entries; want %d", len(recentKeys), wantRecent)
			}
			if want := n - wantRecent; len(archiveKeys) != want {
				t.Errorf("archive holds %d entries; want %d", len(archiveKeys), want)
			}

			// Nothing is lost and nothing is duplicated: the two renderings
			// partition the union exactly.
			seen := map[string]bool{}
			for _, k := range append(append([]string{}, recentKeys...), archiveKeys...) {
				if seen[k] {
					t.Errorf("key %q appears in both halves — the split must partition, not copy", k)
				}
				seen[k] = true
			}
			if len(seen) != n {
				t.Errorf("the two halves together hold %d distinct entries; want %d", len(seen), n)
			}

			// The cut falls between the limit-th newest and the next one:
			// the archive's first entry is older than the recent's last.
			if len(archiveKeys) > 0 {
				if oldestRecent, newestArchive := recentKeys[len(recentKeys)-1], archiveKeys[0]; oldestRecent <= newestArchive {
					t.Errorf("cut is misplaced: recent ends at %q but archive begins at %q (archive must be strictly older)",
						oldestRecent, newestArchive)
				}
			}
		})
	}
}

// TestGenerate_ArchiveExistsEvenWhenEmpty: the recent half links to the
// archive, so the archive is always a real rendering — header and all — never
// an omitted file. A repo whose whole history fits in docs/timeline.md still
// gets one.
func TestGenerate_ArchiveExistsEvenWhenEmpty(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	writeNBranchEntries(t, docs, 3)

	var stderr bytes.Buffer
	recent, archive, err := generateAt(docs, 10, &stderr)
	if err != nil {
		t.Fatalf("generateAt: %v", err)
	}
	if !strings.Contains(recent, "timeline-archive.md") {
		t.Errorf("the recent half must point a reader at the continuation; got:\n%s", recent)
	}
	if !strings.HasPrefix(archive, archiveHeader) {
		t.Errorf("the archive must carry its own header even with no rows; got:\n%s", archive)
	}
	if !strings.Contains(archive, "timeline.md") {
		t.Errorf("the archive must point back at the recent half; got:\n%s", archive)
	}
}

// TestGenerate_IsIdempotent: two regenerations from the same sources yield the
// same bytes in BOTH halves. §3.3 requires the history to be a pure function
// of its sources; without this, a regeneration would push on every run and the
// regen-on-main loop would never settle.
func TestGenerate_IsIdempotent(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	writeNBranchEntries(t, docs, 60)

	var stderr bytes.Buffer
	recent1, archive1, err := Generate(docs, &stderr)
	if err != nil {
		t.Fatalf("Generate (first): %v", err)
	}
	recent2, archive2, err := Generate(docs, &stderr)
	if err != nil {
		t.Fatalf("Generate (second): %v", err)
	}
	if recent1 != recent2 {
		t.Errorf("docs/timeline.md is not byte-stable across regenerations")
	}
	if archive1 != archive2 {
		t.Errorf("docs/timeline-archive.md is not byte-stable across regenerations")
	}
}

// TestGenerate_MovingTheLimitIsAPureRegeneration is the load-bearing property
// of §3.3: "Changing where the split falls is a regeneration, not a
// migration." Rendered at three different limits from ONE unchanged set of
// branch files, the union of the two halves is identical every time — same
// entries, same order, nothing consumed by the previous rendering.
//
// It also pins the mechanism that makes that true. The renderings are
// produced from the sources alone: if either output file were ever an input —
// if the archive were built by moving what fell off the end of the last
// timeline — then rendering at a smaller limit and then a larger one could not
// give the entries back, and this test would fail on the third pass.
func TestGenerate_MovingTheLimitIsAPureRegeneration(t *testing.T) {
	const total = 40
	docs := filepath.Join(t.TempDir(), "docs")
	writeNBranchEntries(t, docs, total)

	var stderr bytes.Buffer
	var reference []string
	// Deliberately not monotonic: shrink the window hard, then widen it past
	// where it started. A "move" implementation survives the shrink and fails
	// the re-widening.
	for _, limit := range []int{30, 5, 35, 40, 1} {
		recent, archive, err := generateAt(docs, limit, &stderr)
		if err != nil {
			t.Fatalf("generateAt(%d): %v", limit, err)
		}
		union := append(entryKeys(t, recent), entryKeys(t, archive)...)
		if len(union) != total {
			t.Fatalf("limit %d: the two halves hold %d entries; want %d — the record must not shrink with the view",
				limit, len(union), total)
		}
		if reference == nil {
			reference = union
			continue
		}
		if strings.Join(union, ",") != strings.Join(reference, ",") {
			t.Fatalf("limit %d: the union changed when only the split point moved\n got: %v\nwant: %v",
				limit, union, reference)
		}
	}

	// And the sources are genuinely untouched by rendering: re-render at the
	// production limit and confirm the branch file still holds every entry.
	body := readDoc(t, docs, "decisions-branches/feat__many.md")
	if n := strings.Count(body, entryStartPrefix); n != total {
		t.Errorf("the branch file holds %d entry markers after %d regenerations; want %d — rendering must never consume a source",
			n, 5, total)
	}
}

// readDoc reads <docs>/<rel>.
func readDoc(t *testing.T, docs, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(docs, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// TestCollectMarked_MarkerlessFileDatesByNewestEntry pins the rule that a
// markerless file's single synthesized row takes its NEWEST entry, not its
// first.
//
// Every branch file except one eventually closes: the branch merges and the
// file stops changing, so first and last are days apart inside one window of
// work and the choice is immaterial. `main.md` never closes — it keeps
// collecting decisions made directly on the default branch. Dating from the
// first entry froze main's row at its oldest decision while the file grew
// underneath it, and the row sank past §3.3's 50-entry cut into the archive
// and stayed there, making anything logged on main invisible in the recent
// view.
//
// Deliberately ONE rule for every file rather than a default-branch special
// case: it only *matters* for a file that stays open, which is the honest
// reason to prefer it over branching on the branch name.
func TestCollectMarked_MarkerlessFileDatesByNewestEntry(t *testing.T) {
	docs := t.TempDir()
	branches := filepath.Join(docs, "decisions-branches")
	if err := os.MkdirAll(branches, 0o755); err != nil {
		t.Fatal(err)
	}
	// A markerless file (no entry-block markers) spanning two months.
	body := "# Decisions — main\n\n" +
		"## 2026-05-15 10:00 - Oldest decision\n\n**Reasoning:** first\n\n---\n\n" +
		"## 2026-06-20 10:00 - Middle decision\n\n**Reasoning:** second\n\n---\n\n" +
		"## 2026-07-16 10:00 - Newest decision\n\n**Reasoning:** third\n\n---\n"
	if err := os.WriteFile(filepath.Join(branches, "main.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := collectMarked(docs, io.Discard)
	if err != nil {
		t.Fatalf("collectMarked: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("a markerless file must synthesize exactly one row, got %d", len(items))
	}
	got := items[0].date.Format("2006-01-02")
	if got != "2026-07-16" {
		t.Errorf("row dated %s; want 2026-07-16 (the NEWEST entry) — dating by the "+
			"oldest freezes an open file's row while it keeps growing", got)
	}
	// The rendered body must agree with the date, or the row reads as the
	// newest date attached to the oldest decision's title.
	if !strings.Contains(items[0].body, "Newest decision") {
		t.Errorf("row body does not carry the newest entry's title; got %q", items[0].body)
	}
	if strings.Contains(items[0].body, "Oldest decision") {
		t.Errorf("row body still carries the oldest entry's title; got %q", items[0].body)
	}
}
