// decisions_rotate_test.go — SPEC §1.3.2 capacity rotation. Covers:
//
//   - rotateDecisions at the boundary: N-1 existing entries + 1 new = at
//     cap (no rotation), N existing + 1 new = one-over-cap (exactly the
//     oldest entry migrates, byte-exact), and a multi-overflow case where
//     more than one entry must migrate in a single call.
//   - appendToArchive: scaffolds the archive from the template on first
//     overflow, then pure-appends (never re-sorts) on subsequent overflows.
//   - an end-to-end `logmind log` run proving decisions.max_recent is
//     actually honored, the archive receives the byte-exact oldest entry,
//     and the SPEC §3.1 three-line stdout contract is unchanged by
//     rotation.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/decisions"
)

// buildDecisionsMD assembles a synthetic docs/decisions.md-shaped byte
// string: the standard scaffolded header followed by n distinct entries
// (oldest-first, matching decisions.md's own append-only convention), each
// shaped exactly like buildDecisionEntry's output.
func buildDecisionsMD(n int) []byte {
	var b strings.Builder
	b.WriteString("# Decision Log\n\nThis file contains the 20 most recent decisions. Older decisions are archived in [decisions-archive.md](decisions-archive.md).\n\n---\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "## 2026-01-%02d 10:00 - Entry %d\n\n**Reasoning:** seq %d\n\n---\n\n", i, i, i)
	}
	return []byte(b.String())
}

// TestRotateDecisions_AtCap_NoRotation: existing holds cap-1 entries.
// `logmind log` is about to append ONE more, landing exactly AT the cap —
// no rotation should occur and the bytes must pass through untouched.
func TestRotateDecisions_AtCap_NoRotation(t *testing.T) {
	const maxRecent = 3
	existing := buildDecisionsMD(maxRecent - 1)
	kept, migrated := rotateDecisions(existing, maxRecent)
	if migrated != "" {
		t.Fatalf("migrated = %q; want \"\" (no rotation at the cap boundary)", migrated)
	}
	if !bytes.Equal(kept, existing) {
		t.Fatalf("kept content changed when no rotation should occur:\ngot:  %q\nwant: %q", kept, existing)
	}
}

// TestRotateDecisions_OneOverCap_MigratesOldestVerbatim: existing already
// holds exactly maxRecent entries; the new entry `logmind log` is about to
// append pushes the count to maxRecent+1 — exactly the OLDEST entry must
// migrate, byte-identical to what was on disk (SPEC §1.3.2: "Tools MUST
// preserve byte-exact entry content during overflow").
func TestRotateDecisions_OneOverCap_MigratesOldestVerbatim(t *testing.T) {
	const maxRecent = 3
	existing := buildDecisionsMD(maxRecent)
	_, entries := decisions.SplitRawBytes(string(existing))
	if len(entries) != maxRecent {
		t.Fatalf("test setup: got %d entries; want %d", len(entries), maxRecent)
	}
	wantMigrated := entries[0].Raw // the oldest entry, byte-exact

	kept, migrated := rotateDecisions(existing, maxRecent)
	if migrated != wantMigrated {
		t.Fatalf("migrated = %q; want byte-exact oldest entry %q", migrated, wantMigrated)
	}
	_, keptEntries := decisions.SplitRawBytes(string(kept))
	if len(keptEntries) != maxRecent-1 {
		t.Fatalf("kept has %d entries; want %d (exactly one rotated out)", len(keptEntries), maxRecent-1)
	}
	if keptEntries[0].Title != "Entry 2" {
		t.Fatalf("kept's new oldest = %q; want %q", keptEntries[0].Title, "Entry 2")
	}
	// Every surviving entry's bytes are untouched — no re-rendering.
	for i, e := range keptEntries {
		if e.Raw != entries[i+1].Raw {
			t.Fatalf("kept entry %d bytes changed:\ngot:  %q\nwant: %q", i, e.Raw, entries[i+1].Raw)
		}
	}
}

// TestRotateDecisions_MultiOverflow_MigratesFIFO: existing already holds
// MORE than one entry beyond capacity — e.g. decisions.max_recent was
// lowered after entries had already accumulated. A single `logmind log`
// call must migrate every overflowing entry at once, oldest-first (FIFO),
// not just the single oldest one.
func TestRotateDecisions_MultiOverflow_MigratesFIFO(t *testing.T) {
	const maxRecent = 3
	existing := buildDecisionsMD(maxRecent + 4) // 7 entries already on disk
	_, entries := decisions.SplitRawBytes(string(existing))

	kept, migrated := rotateDecisions(existing, maxRecent)

	// 7 existing + 1 new - 3 cap = 5 entries must migrate, oldest-first.
	const wantOverflow = 5
	var wantMigrated strings.Builder
	for i := 0; i < wantOverflow; i++ {
		wantMigrated.WriteString(entries[i].Raw)
	}
	if migrated != wantMigrated.String() {
		t.Fatalf("migrated bytes mismatch:\ngot:  %q\nwant: %q", migrated, wantMigrated.String())
	}

	_, keptEntries := decisions.SplitRawBytes(string(kept))
	if len(keptEntries) != maxRecent-1 {
		t.Fatalf("kept has %d entries; want %d", len(keptEntries), maxRecent-1)
	}
	if keptEntries[0].Title != "Entry 6" || keptEntries[1].Title != "Entry 7" {
		t.Fatalf("kept entries = %q, %q; want Entry 6, Entry 7", keptEntries[0].Title, keptEntries[1].Title)
	}
}

// TestRotateDecisions_MaxRecentZero_DisablesRotation: a non-positive
// max_recent is treated as "rotation off", not "archive everything" — a
// destructive surprise no config author is likely to intend.
func TestRotateDecisions_MaxRecentZero_DisablesRotation(t *testing.T) {
	existing := buildDecisionsMD(10)
	kept, migrated := rotateDecisions(existing, 0)
	if migrated != "" {
		t.Fatalf("migrated = %q; want \"\" (max_recent<=0 disables rotation)", migrated)
	}
	if !bytes.Equal(kept, existing) {
		t.Fatalf("kept content changed with max_recent<=0")
	}
}

// TestAppendToArchive_ScaffoldsFromTemplateOnFirstOverflow: the first
// overflow ever, against a repo with no docs/decisions-archive.md yet,
// creates the file from the standard template (mirroring `logmind init`)
// rather than a bare entry dump.
func TestAppendToArchive_ScaffoldsFromTemplateOnFirstOverflow(t *testing.T) {
	dir := t.TempDir()
	docsPath := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	migrated := "## 2026-01-01 10:00 - Entry 1\n\n**Reasoning:** seq 1\n\n---\n\n"
	if err := appendToArchive(docsPath, migrated); err != nil {
		t.Fatalf("appendToArchive: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(docsPath, "decisions-archive.md"))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if !strings.HasPrefix(string(got), "# Decision Archive") {
		t.Fatalf("archive missing the standard header:\n%s", got)
	}
	if !strings.HasSuffix(string(got), migrated) {
		t.Fatalf("archive missing the migrated entry verbatim:\n%s", got)
	}
}

// TestAppendToArchive_AppendsWithoutResorting: a second overflow appends
// AFTER the first, in call order — the archive is never re-sorted (SPEC
// §1.5: "Tools MUST NOT re-sort the archive on write").
func TestAppendToArchive_AppendsWithoutResorting(t *testing.T) {
	dir := t.TempDir()
	docsPath := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	first := "## 2026-01-01 10:00 - Entry 1\n\n---\n\n"
	second := "## 2026-01-02 10:00 - Entry 2\n\n---\n\n"
	if err := appendToArchive(docsPath, first); err != nil {
		t.Fatalf("appendToArchive first: %v", err)
	}
	if err := appendToArchive(docsPath, second); err != nil {
		t.Fatalf("appendToArchive second: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(docsPath, "decisions-archive.md"))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	firstIdx := strings.Index(string(got), first)
	secondIdx := strings.Index(string(got), second)
	if firstIdx == -1 || secondIdx == -1 || firstIdx > secondIdx {
		t.Fatalf("archive entries out of append order:\n%s", got)
	}
}

// TestLog_RotatesDecisionsOnOverflow_EndToEnd: end-to-end wiring through
// the real `logmind log` command —
//
//   - decisions.max_recent from .logmind/config.yml is actually honored
//     (it previously governed nothing).
//   - the archive receives the byte-exact oldest entry BEFORE the new
//     entry is appended to decisions.md.
//   - the SPEC §3.1 three-line stdout contract gains no line from
//     rotation.
func TestLog_RotatesDecisionsOnOverflow_EndToEnd(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		writeConfig(t, d, "decisions:\n  max_recent: 2\n")

		// Seed decisions.md with exactly 2 entries — AT the cap — so the
		// next `logmind log` call is the one that overflows.
		decisionsPath := filepath.Join(d, "docs", "decisions.md")
		existing, err := os.ReadFile(decisionsPath)
		if err != nil {
			t.Fatalf("read scaffolded decisions.md: %v", err)
		}
		oldestSeeded := "## 2026-01-01 09:00 - Oldest seeded decision\n\n**Reasoning:** first\n\n---\n\n"
		seed := string(existing) + oldestSeeded +
			"## 2026-01-02 09:00 - Second seeded decision\n\n**Reasoning:** second\n\n---\n\n"
		if err := os.WriteFile(decisionsPath, []byte(seed), 0o644); err != nil {
			t.Fatalf("seed decisions.md: %v", err)
		}
		commitAll(t, d, "initial")

		var out bytes.Buffer
		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "Third decision", "-r", "third", "--no-push", "--no-interactive"})
			root.SetOut(&out)
			root.SetErr(&out)
			if err := root.Execute(); err != nil {
				t.Fatalf("log: %v\n%s", err, out.String())
			}
		})

		// SPEC §3.1 three required lines, byte-exact, in order — rotation
		// must not add or remove a line.
		wantLines := []string{
			"ℹ Staging all changes (use --stage scoped to limit)",
			`✓ Logged decision: "Third decision"`,
			"✓ Committed changes",
		}
		gotLines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
		if len(gotLines) != len(wantLines) {
			t.Fatalf("stdout has %d lines; want %d (§3.1 contract):\n%s", len(gotLines), len(wantLines), out.String())
		}
		for i, want := range wantLines {
			if gotLines[i] != want {
				t.Fatalf("stdout line %d = %q; want %q", i, gotLines[i], want)
			}
		}

		archived, err := os.ReadFile(filepath.Join(d, "docs", "decisions-archive.md"))
		if err != nil {
			t.Fatalf("read decisions-archive.md: %v", err)
		}
		if !strings.HasSuffix(string(archived), oldestSeeded) {
			t.Fatalf("archive missing the byte-exact migrated entry;\ngot:\n%s", archived)
		}

		final, err := os.ReadFile(decisionsPath)
		if err != nil {
			t.Fatalf("read decisions.md: %v", err)
		}
		_, finalEntries := decisions.SplitRawBytes(string(final))
		if len(finalEntries) != 2 {
			t.Fatalf("decisions.md has %d entries; want 2 (the configured cap)", len(finalEntries))
		}
		if finalEntries[0].Title != "Second seeded decision" {
			t.Fatalf("decisions.md's oldest surviving entry = %q; want %q", finalEntries[0].Title, "Second seeded decision")
		}
		if finalEntries[1].Title != "Third decision" {
			t.Fatalf("decisions.md's newest entry = %q; want %q", finalEntries[1].Title, "Third decision")
		}
	})
}
