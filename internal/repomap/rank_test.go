package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/tree"
)

// setupRankRepo builds a tiny module: `hub` is imported by `a` and `b`
// (fan-in 2); `lonely` has no fan-in but is named in a decision doc.
func setupRankRepo(t *testing.T) (dir string, files []FileSymbols) {
	t.Helper()
	dir = t.TempDir()
	write := func(rel, src string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/m\n\ngo 1.22\n")
	write("hub/hub.go", "package hub\nfunc H() {}\n")
	write("a/a.go", "package a\nimport \"example.com/m/hub\"\nfunc A() { hub.H() }\n")
	write("b/b.go", "package b\nimport \"example.com/m/hub\"\nfunc B() { hub.H() }\n")
	write("lonely/lonely.go", "package lonely\nfunc L() {}\n")
	write("docs/decisions.md", "## X\n\nWe refactored lonely/lonely.go for good reasons.\n")

	rules, err := tree.ResolveRules(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	files, err = ExtractGo(dir, rules)
	if err != nil {
		t.Fatal(err)
	}
	return dir, files
}

func paths(fs []FileSymbols) string {
	var p []string
	for _, f := range fs {
		p = append(p, f.Path)
	}
	return strings.Join(p, ",")
}

func TestRank_DecisionLinkedThenFanInThenPath(t *testing.T) {
	dir, files := setupRankRepo(t)
	got := paths(Rank(dir, files))
	// lonely: decision-linked (beats fan-in). hub: fan-in 2. a,b: fan-in 0 → path order.
	want := "lonely/lonely.go,hub/hub.go,a/a.go,b/b.go"
	if got != want {
		t.Errorf("rank order = %q; want %q", got, want)
	}
	if paths(Rank(dir, files)) != got {
		t.Error("Rank not deterministic across runs")
	}
	// Rank must not mutate the input order (ExtractGo returns path-sorted).
	if paths(files) != "a/a.go,b/b.go,hub/hub.go,lonely/lonely.go" {
		t.Errorf("Rank mutated the input slice: %q", paths(files))
	}
}

func TestImportFanIn_Counts(t *testing.T) {
	dir, files := setupRankRepo(t)
	fi := importFanIn(dir, files)
	if fi["hub/hub.go"] != 2 {
		t.Errorf("hub fan-in = %d; want 2", fi["hub/hub.go"])
	}
	if fi["a/a.go"] != 0 || fi["lonely/lonely.go"] != 0 {
		t.Errorf("leaf files should have fan-in 0: %+v", fi)
	}
}

func TestImportFanIn_NoModuleIsZero(t *testing.T) {
	dir := t.TempDir()
	// no go.mod
	files := []FileSymbols{{Path: "a.go", Imports: []string{"fmt"}}}
	if fi := importFanIn(dir, files); fi["a.go"] != 0 {
		t.Errorf("no go.mod should yield zero fan-in, got %+v", fi)
	}
}

func TestPack_NoBudgetKeepsAll(t *testing.T) {
	_, files := setupRankRepo(t)
	kept, omitted := Pack(files, 0)
	if len(kept) != len(files) || omitted != 0 {
		t.Fatalf("no-budget should keep all: kept=%d omitted=%d (of %d)", len(kept), omitted, len(files))
	}
}

func TestPack_HugeBudgetKeepsAll(t *testing.T) {
	dir, files := setupRankRepo(t)
	kept, omitted := Pack(Rank(dir, files), 1_000_000)
	if len(kept) != len(files) || omitted != 0 {
		t.Fatalf("huge budget should keep all: kept=%d omitted=%d", len(kept), omitted)
	}
}

func TestPack_TrimsAndAccountsExactly(t *testing.T) {
	dir, files := setupRankRepo(t)
	ranked := Rank(dir, files)
	kept, omitted := Pack(ranked, 40) // tiny — nothing fits past the header
	if len(kept)+omitted != len(ranked) {
		t.Fatalf("kept(%d)+omitted(%d) != total(%d)", len(kept), omitted, len(ranked))
	}
	if omitted == 0 {
		t.Fatal("tiny budget should omit at least one file")
	}
	// never-worse (§14.5): the packed render is never larger than the full render.
	if packed, full := RenderWithOmitted(kept, omitted), Render(files); len(packed) > len(full) {
		t.Errorf("packed (%d B) larger than full (%d B) — never-worse violated", len(packed), len(full))
	}
}

func TestGenerateBudget_ZeroEqualsGenerate(t *testing.T) {
	dir, _ := setupRankRepo(t)
	a, _, err := Generate(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _, err := GenerateBudget(dir, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("GenerateBudget(…, 0) must byte-equal Generate:\n--a--\n%s\n--b--\n%s", a, b)
	}
}

func idxOf(fs []FileSymbols, p string) int {
	for i, f := range fs {
		if f.Path == p {
			return i
		}
	}
	return -1
}

// TestGenerateBudget_NeverWorseThanFull: across budgets that would omit only a
// few small files (where the marker overhead exceeds the dropped blocks),
// GenerateBudget must never emit more than the full render — it passes through
// the full map instead (§14.5). Regression for the never-worse BLOCKER.
func TestGenerateBudget_NeverWorseThanFull(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "go.mod", "module x\n")
	writeGo(t, dir, "a.go", "package p\nfunc A() {}\n")
	writeGo(t, dir, "b.go", "package p\nfunc B() {}\n")
	full, _, _, err := GenerateBudget(dir, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, budget := range []int{1, 20, 40, 48, 55, 80} {
		text, _, omitted, err := GenerateBudget(dir, nil, budget)
		if err != nil {
			t.Fatal(err)
		}
		if len(text) > len(full) {
			t.Errorf("budget %d: packed %dB > full %dB — never-worse violated:\n%s", budget, len(text), len(full), text)
		}
		// At full size, passthrough must have kicked in (omitted 0, == full).
		if len(text) == len(full) && (omitted != 0 || text != full) {
			t.Errorf("budget %d: full-size output but omitted=%d / not the full map", budget, omitted)
		}
	}
}

// TestRank_DecisionLinkSubstringNotMatched: a short path must not be falsely
// decision-linked because it is a substring of an unrelated word in the docs.
func TestRank_DecisionLinkSubstringNotMatched(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "go.mod", "module example.com/m\n")
	writeGo(t, dir, "a.go", "package p\nfunc A() {}\n") // fan-in 0, NOT referenced
	writeGo(t, dir, "hub/hub.go", "package hub\nfunc H() {}\n")
	writeGo(t, dir, "u/u.go", "package u\nimport \"example.com/m/hub\"\nfunc U() { hub.H() }\n")
	// The doc mentions only "data.go" — which CONTAINS "a.go" as a substring.
	writeGo(t, dir, "docs/decisions.md", "## X\n\nWe touched data.go for reasons.\n")

	rules, _ := tree.ResolveRules(dir, nil)
	files, err := ExtractGo(dir, rules)
	if err != nil {
		t.Fatal(err)
	}
	ranked := Rank(dir, files)
	// hub (fan-in 1) must outrank a.go — i.e. a.go was NOT falsely boosted.
	if idxOf(ranked, "hub/hub.go") > idxOf(ranked, "a.go") {
		t.Errorf("a.go falsely decision-linked via `data.go` substring; order: %s", paths(ranked))
	}
	// Sanity: a legitimately-named path IS matched.
	if !mentionsPath("see internal/x/a.go here", "internal/x/a.go") {
		t.Error("a real whole-path mention should match")
	}
}

func TestModuleImportPath_TrailingComment(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "go.mod", "module example.com/m // vanity import path\n\ngo 1.22\n")
	if got := moduleImportPath(dir); got != "example.com/m" {
		t.Errorf("module path = %q; want example.com/m (trailing comment must be stripped)", got)
	}
}

func TestGenerateBudget_MarkerAndDeterminism(t *testing.T) {
	dir, _ := setupRankRepo(t)
	text, kept, omitted, err := GenerateBudget(dir, nil, 60)
	if err != nil {
		t.Fatal(err)
	}
	if omitted > 0 && !strings.Contains(text, "omitted to fit the token budget") {
		t.Errorf("omitted=%d but no truncation marker:\n%s", omitted, text)
	}
	if len(kept)+omitted != 4 {
		t.Errorf("kept+omitted = %d; want 4", len(kept)+omitted)
	}
	b, _, _, _ := GenerateBudget(dir, nil, 60)
	if text != b {
		t.Error("GenerateBudget not byte-deterministic")
	}
}
