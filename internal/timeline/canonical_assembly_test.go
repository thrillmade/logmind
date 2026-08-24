package timeline

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDoc writes content under <docs>/<rel>, creating parent dirs.
func writeDoc(t *testing.T, docs, rel, content string) {
	t.Helper()
	p := filepath.Join(docs, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateMainCanonical_UnionNewestFirst(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	// Branch A — markered (+ a decision header, which must NOT become a
	// second row: the marker is the branch's one headline).
	writeDoc(t, docs, "decisions-branches/feat__a.md",
		"← back\n\n"+block("2026-06-29-add-a", "- **2026-06-29** — Add A")+
			"\n## 2026-06-29 10:00 - Add A\nbody\n")
	// Branch B — markered.
	writeDoc(t, docs, "decisions-branches/feat__b.md",
		block("2026-06-15-add-b", "- **2026-06-15** — Add B"))
	// decisions.md — direct-to-main (synthesized row).
	writeDoc(t, docs, "decisions.md", "# Decisions\n\n## 2026-06-20 09:00 - Hotfix C\nbody\n")

	var stderr bytes.Buffer
	out, _, err := Generate(docs, &stderr)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !HasEntryBlocks(out) {
		t.Fatalf("output is not entry-block format:\n%s", out)
	}
	blocks := extractEntryBlocks(out, &stderr)
	wantKeys := []string{"2026-06-29-add-a", "2026-06-20-hotfix-c", "2026-06-15-add-b"}
	if len(blocks) != len(wantKeys) {
		t.Fatalf("got %d blocks; want %d\n%s", len(blocks), len(wantKeys), out)
	}
	for i, w := range wantKeys {
		if blocks[i].Key != w {
			t.Errorf("block[%d].Key = %q; want %q (newest-first)", i, blocks[i].Key, w)
		}
	}
	// The markered headline + the detail link computed from the source path.
	wantBody := "- **2026-06-29** — Add A → [detail](decisions-branches/feat__a.md)"
	if blocks[0].Body != wantBody {
		t.Errorf("block[0].Body = %q; want %q", blocks[0].Body, wantBody)
	}
}

func TestGenerateMainCanonical_DeterministicAcrossCheckoutPaths(t *testing.T) {
	build := func(root string) string {
		docs := filepath.Join(root, "docs")
		// Create files in non-sorted order; the assembler must not depend on it.
		writeDoc(t, docs, "decisions-branches/feat__zeta.md", block("2026-06-01-zeta", "- z"))
		writeDoc(t, docs, "decisions-branches/feat__alpha.md", block("2026-06-29-alpha", "- a"))
		writeDoc(t, docs, "decisions.md", "# D\n\n## 2026-06-15 09:00 - Mid\nx\n")
		var sb bytes.Buffer
		out, _, err := Generate(docs, &sb)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	a := build(filepath.Join(t.TempDir(), "checkout-one"))
	b := build(filepath.Join(t.TempDir(), "a-totally-different-path"))
	if a != b {
		t.Errorf("non-deterministic across checkout paths:\n--- A ---\n%s\n--- B ---\n%s", a, b)
	}
}

// TestGenerateMainCanonical_BackfillIsTimelineNeutral pins the determinism the
// doctor --fix backfill depends on: a markerless branch's fallback row (sorted
// by its decision's HH:MM) and the SAME branch's backfilled date-only marker
// must render IDENTICALLY — otherwise backfilling silently reorders same-day
// rows. Guards the day-vs-timestamp sort fix (date-only synthesized rows).
func TestGenerateMainCanonical_BackfillIsTimelineNeutral(t *testing.T) {
	build := func(markered bool) string {
		docs := filepath.Join(t.TempDir(), "docs")
		// A direct-to-main decision at 09:00 the same day as the branch's
		// first decision at 15:00 — the case where time-order ≠ date-order.
		writeDoc(t, docs, "decisions.md", "# D\n\n## 2026-06-10 09:00 - Zebra direct to main\nbody\n")
		if markered {
			writeDoc(t, docs, "decisions-branches/feat__a.md",
				"← back\n\n"+block("2026-06-10-apple-branch-work", "- **2026-06-10** — Apple branch work")+
					"\n## 2026-06-10 15:00 - Apple branch work\nbody\n")
		} else {
			writeDoc(t, docs, "decisions-branches/feat__a.md",
				"← back\n\n## 2026-06-10 15:00 - Apple branch work\nbody\n")
		}
		var sb bytes.Buffer
		out, _, err := Generate(docs, &sb)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if fallback, backfilled := build(false), build(true); fallback != backfilled {
		t.Errorf("backfilling reordered/changed the timeline (determinism violated):\n--- fallback ---\n%s\n--- backfilled ---\n%s", fallback, backfilled)
	}
}

func TestGenerateMainCanonical_LegacyMarkerlessFallback(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	// A markerless (pre-Slice-2) branch file: only decision headers. It must
	// still contribute exactly ONE synthesized row, derived from the FIRST
	// header — SPEC line 646 makes that a MUST. Dating by the newest entry
	// is proposed at thrillmade/protocol#97; see collectMarked.
	writeDoc(t, docs, "decisions-branches/feat__legacy.md",
		"# legacy\n\n## 2026-06-10 12:00 - Legacy work\nbody\n## 2026-06-11 13:00 - More work\nbody\n")
	var stderr bytes.Buffer
	out, _, err := Generate(docs, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	blocks := extractEntryBlocks(out, &stderr)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks; want 1 synthesized row\n%s", len(blocks), out)
	}
	if blocks[0].Key != "2026-06-10-legacy-work" {
		t.Errorf("key = %q; want 2026-06-10-legacy-work (slug of the FIRST header, SPEC line 646)", blocks[0].Key)
	}
}

func TestGenerateMainCanonical_CollisionGetsStableSuffix(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	// Two genuinely-different entries colliding on the same <date>-<slug>.
	writeDoc(t, docs, "decisions-branches/feat__a.md", block("2026-06-29-dup", "- body from A"))
	writeDoc(t, docs, "decisions-branches/feat__b.md", block("2026-06-29-dup", "- body from B"))
	var stderr bytes.Buffer
	out, _, _ := Generate(docs, &stderr)
	blocks := extractEntryBlocks(out, &stderr)
	if len(blocks) != 2 {
		t.Fatalf("got %d; want 2 (collision → suffix)\n%s", len(blocks), out)
	}
	got := map[string]bool{blocks[0].Key: true, blocks[1].Key: true}
	if !got["2026-06-29-dup"] || !got["2026-06-29-dup-2"] {
		t.Errorf("keys = %v; want {2026-06-29-dup, 2026-06-29-dup-2}", got)
	}
	// The bare slug goes to the lexicographically-smallest source path
	// (feat__a.md < feat__b.md), so it carries body-from-A + its detail link.
	for _, b := range blocks {
		if b.Key == "2026-06-29-dup" {
			want := "- body from A → [detail](decisions-branches/feat__a.md)"
			if b.Body != want {
				t.Errorf("bare-slug body = %q; want %q (smallest source path)", b.Body, want)
			}
		}
	}
}

func TestGenerateMainCanonical_CrossGroupKeysStayUnique(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	// An organic 2-way collision on "dup" (different bodies) PLUS a separate
	// literal entry already keyed "dup-2". The collision's generated suffix
	// must NOT duplicate the literal key — §1.6.3.1 requires unique keys.
	writeDoc(t, docs, "decisions-branches/feat__a.md", block("2026-06-29-dup", "- body A"))
	writeDoc(t, docs, "decisions-branches/feat__b.md", block("2026-06-29-dup", "- body B"))
	writeDoc(t, docs, "decisions-branches/feat__c.md", block("2026-06-29-dup-2", "- literal dup-2"))
	var stderr bytes.Buffer
	out, _, _ := Generate(docs, &stderr)
	blocks := extractEntryBlocks(out, &stderr)
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks; want 3\n%s", len(blocks), out)
	}
	counts := map[string]int{}
	for _, b := range blocks {
		counts[b.Key]++
	}
	for k, n := range counts {
		if n != 1 {
			t.Errorf("key %q appears %dx; §1.6.3.1 requires unique keys\n%s", k, n, out)
		}
	}
}

func TestGenerateMainCanonical_SameEntryTwoSourcesCollapses(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	// Identical body in two sources = ONE entry (the same-entry carve-out).
	writeDoc(t, docs, "decisions-branches/feat__a.md", block("2026-06-29-dup", "- identical body"))
	writeDoc(t, docs, "decisions-branches/feat__b.md", block("2026-06-29-dup", "- identical body"))
	var stderr bytes.Buffer
	out, _, _ := Generate(docs, &stderr)
	blocks := extractEntryBlocks(out, &stderr)
	if len(blocks) != 1 {
		t.Errorf("got %d; want 1 (identical body collapses to one)\n%s", len(blocks), out)
	}
}

// TestGenerateMainCanonical_CRLFAndLFSameEntryCollapses guards the CRLF
// determinism fix end-to-end: a branch file checked out with CRLF line
// endings (core.autocrlf=true on Windows) and one checked out with plain LF,
// both carrying the SAME entry body, must still collapse to one row via the
// same-entry carve-out — a stray "\r" surviving into the body would make
// "body\r" != "body" and silently duplicate the entry.
func TestGenerateMainCanonical_CRLFAndLFSameEntryCollapses(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	crlfBlock := strings.ReplaceAll(block("2026-06-29-dup", "- identical body"), "\n", "\r\n")
	writeDoc(t, docs, "decisions-branches/feat__a.md", crlfBlock)
	writeDoc(t, docs, "decisions-branches/feat__b.md", block("2026-06-29-dup", "- identical body"))
	var stderr bytes.Buffer
	out, _, err := Generate(docs, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	blocks := extractEntryBlocks(out, &stderr)
	if len(blocks) != 1 {
		t.Errorf("got %d; want 1 (CRLF- and LF-sourced identical bodies must collapse)\n%s", len(blocks), out)
	}
	if strings.Contains(out, "\r") {
		t.Errorf("rendered timeline contains a stray \\r:\n%q", out)
	}
}

func TestGenerateMainCanonical_EmptyDocs(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	out, _, err := Generate(docs, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if out != header+emptyBody {
		t.Errorf("empty docs = %q; want header+emptyBody", out)
	}
}
