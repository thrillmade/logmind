package timeline

import (
	"bytes"
	"os"
	"path/filepath"
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
	out, err := GenerateMainCanonical(docs, &stderr)
	if err != nil {
		t.Fatalf("GenerateMainCanonical: %v", err)
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
	// The markered branch body is copied VERBATIM.
	if blocks[0].Body != "- **2026-06-29** — Add A" {
		t.Errorf("block[0].Body = %q; want the verbatim marker body", blocks[0].Body)
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
		out, err := GenerateMainCanonical(docs, &sb)
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

func TestGenerateMainCanonical_LegacyMarkerlessFallback(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	// A markerless (pre-Slice-2) branch file: only decision headers. It must
	// still contribute exactly one synthesized row from the FIRST header.
	writeDoc(t, docs, "decisions-branches/feat__legacy.md",
		"# legacy\n\n## 2026-06-10 12:00 - Legacy work\nbody\n## 2026-06-11 13:00 - More work\nbody\n")
	var stderr bytes.Buffer
	out, err := GenerateMainCanonical(docs, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	blocks := extractEntryBlocks(out, &stderr)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks; want 1 synthesized row\n%s", len(blocks), out)
	}
	if blocks[0].Key != "2026-06-10-legacy-work" {
		t.Errorf("key = %q; want 2026-06-10-legacy-work (slug of the first header)", blocks[0].Key)
	}
}

func TestGenerateMainCanonical_CollisionGetsStableSuffix(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	// Two genuinely-different entries colliding on the same <date>-<slug>.
	writeDoc(t, docs, "decisions-branches/feat__a.md", block("2026-06-29-dup", "- body from A"))
	writeDoc(t, docs, "decisions-branches/feat__b.md", block("2026-06-29-dup", "- body from B"))
	var stderr bytes.Buffer
	out, _ := GenerateMainCanonical(docs, &stderr)
	blocks := extractEntryBlocks(out, &stderr)
	if len(blocks) != 2 {
		t.Fatalf("got %d; want 2 (collision → suffix)\n%s", len(blocks), out)
	}
	got := map[string]bool{blocks[0].Key: true, blocks[1].Key: true}
	if !got["2026-06-29-dup"] || !got["2026-06-29-dup-2"] {
		t.Errorf("keys = %v; want {2026-06-29-dup, 2026-06-29-dup-2}", got)
	}
	// The bare slug goes to the lexicographically-smallest source path
	// (feat__a.md < feat__b.md), so it carries body-from-A.
	for _, b := range blocks {
		if b.Key == "2026-06-29-dup" && b.Body != "- body from A" {
			t.Errorf("bare-slug body = %q; want body-from-A (smallest source path)", b.Body)
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
	out, _ := GenerateMainCanonical(docs, &stderr)
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
	out, _ := GenerateMainCanonical(docs, &stderr)
	blocks := extractEntryBlocks(out, &stderr)
	if len(blocks) != 1 {
		t.Errorf("got %d; want 1 (identical body collapses to one)\n%s", len(blocks), out)
	}
}

func TestGenerateMainCanonical_EmptyDocs(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	out, err := GenerateMainCanonical(docs, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if out != header+emptyBody {
		t.Errorf("empty docs = %q; want header+emptyBody", out)
	}
}

func TestGenerateFor_DefaultIsByteIdenticalToGenerate(t *testing.T) {
	// The byte-parity guard for the dispatch: with canonical=false, GenerateFor
	// MUST produce exactly what Generate produces (the v0.6.14 path).
	docs := filepath.Join(t.TempDir(), "docs")
	writeDoc(t, docs, "decisions.md", "# D\n\n## 2026-06-20 09:00 - X\nbody\n")
	var s1, s2 bytes.Buffer
	viaFor, err := GenerateFor(docs, true, false, &s1)
	if err != nil {
		t.Fatal(err)
	}
	viaGenerate, err := Generate(docs, true, &s2)
	if err != nil {
		t.Fatal(err)
	}
	if viaFor != viaGenerate {
		t.Errorf("GenerateFor(canonical=false) diverged from Generate:\n--- For ---\n%s\n--- Generate ---\n%s", viaFor, viaGenerate)
	}
}
