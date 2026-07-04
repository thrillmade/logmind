package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/tree"
)

// writeGo writes a .go file under dir/rel, creating parents.
func writeGo(t *testing.T, dir, rel, src string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sampleGo = `package sample

// Doc comments are dropped from the skeleton.
func Exported(a int, b string) (int, error) { return a, nil }

func unexported() { println("body dropped") }

type Rec struct {
	Field int
	other string
}

func (r *Rec) Method() string { return r.other }
func (r Rec) ValueMethod(x int) bool { return x > 0 }

type Ifc interface {
	Do() error
}

type ID = string

type Count int

func MultiLine(
	a int,
	b int,
) (
	int,
	error,
) {
	return a + b, nil
}

type Stack[T any] struct {
	items []T
}

func Map[T, U any](s []T, f func(T) U) []U { return nil }
`

func extractOne(t *testing.T, src string) []Symbol {
	t.Helper()
	dir := t.TempDir()
	writeGo(t, dir, "sample.go", src)
	rules, err := tree.ResolveRules(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	files, err := ExtractGo(dir, rules)
	if err != nil {
		t.Fatalf("ExtractGo: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}
	return files[0].Symbols
}

func sigOf(syms []Symbol, name string) (Symbol, bool) {
	for _, s := range syms {
		if s.Name == name {
			return s, true
		}
	}
	return Symbol{}, false
}

func TestExtractGo_Signatures(t *testing.T) {
	syms := extractOne(t, sampleGo)

	cases := []struct {
		name     string
		wantKind string
		wantSig  string
	}{
		{"Exported", "func", "func Exported(a int, b string) (int, error)"},
		{"unexported", "func", "func unexported()"},
		{"Rec", "type", "type Rec struct"},
		{"Method", "method", "func (r *Rec) Method() string"},
		{"ValueMethod", "method", "func (r Rec) ValueMethod(x int) bool"},
		{"Ifc", "type", "type Ifc interface"},
		{"ID", "type", "type ID = string"},
		{"Count", "type", "type Count int"},
		// A multi-line signature is collapsed to a single row.
		{"MultiLine", "func", "func MultiLine( a int, b int, ) ( int, error, )"},
		// Generics: type params are preserved on both types and funcs.
		{"Stack", "type", "type Stack[T any] struct"},
		{"Map", "func", "func Map[T, U any](s []T, f func(T) U) []U"},
	}
	for _, c := range cases {
		s, ok := sigOf(syms, c.name)
		if !ok {
			t.Errorf("%s: not extracted", c.name)
			continue
		}
		if s.Kind != c.wantKind {
			t.Errorf("%s: kind = %q; want %q", c.name, s.Kind, c.wantKind)
		}
		if s.Signature != c.wantSig {
			t.Errorf("%s: sig = %q; want %q", c.name, s.Signature, c.wantSig)
		}
	}
}

func TestExtractGo_BodyDropped(t *testing.T) {
	syms := extractOne(t, sampleGo)
	// What must NOT leak is real body/member content — statements and
	// struct/interface fields (composites collapse to the bare keyword).
	leaks := []string{"body dropped", "println", "return", "Field", "other", "items"}
	for _, s := range syms {
		for _, bad := range leaks {
			if strings.Contains(s.Signature, bad) {
				t.Errorf("member/body token %q leaked into signature: %q", bad, s.Signature)
			}
		}
	}
}

func TestExtractGo_SourceOrderPreserved(t *testing.T) {
	syms := extractOne(t, sampleGo)
	var names []string
	for _, s := range syms {
		names = append(names, s.Name)
	}
	got := strings.Join(names, ",")
	want := "Exported,unexported,Rec,Method,ValueMethod,Ifc,ID,Count,MultiLine,Stack,Map"
	if got != want {
		t.Errorf("symbol order = %q; want %q", got, want)
	}
}

func TestExtractGo_ExcludesTestAndIgnored(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "keep.go", "package p\nfunc Keep() {}\n")
	writeGo(t, dir, "keep_test.go", "package p\nfunc TestSkip() {}\n")
	writeGo(t, dir, "vendored/dep.go", "package dep\nfunc Vendored() {}\n")

	rules, err := tree.ResolveRules(dir, []string{"vendored"})
	if err != nil {
		t.Fatal(err)
	}
	files, err := ExtractGo(dir, rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "keep.go" {
		t.Fatalf("want only keep.go, got %+v", files)
	}
	if _, ok := sigOf(files[0].Symbols, "TestSkip"); ok {
		t.Error("_test.go symbol leaked into the repomap")
	}
}

func TestExtractGo_FilesSortedDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "zeta.go", "package p\nfunc Z() {}\n")
	writeGo(t, dir, "alpha.go", "package p\nfunc A() {}\n")
	writeGo(t, dir, "sub/mid.go", "package s\nfunc M() {}\n")
	rules, _ := tree.ResolveRules(dir, nil)

	a, err := ExtractGo(dir, rules)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ExtractGo(dir, rules)
	var pathsA, pathsB []string
	for _, f := range a {
		pathsA = append(pathsA, f.Path)
	}
	for _, f := range b {
		pathsB = append(pathsB, f.Path)
	}
	if strings.Join(pathsA, ",") != "alpha.go,sub/mid.go,zeta.go" {
		t.Errorf("files not path-sorted: %v", pathsA)
	}
	if strings.Join(pathsA, ",") != strings.Join(pathsB, ",") {
		t.Error("ExtractGo not deterministic across runs")
	}
}

func TestExtractGo_UnparseableSkipped(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "good.go", "package p\nfunc Good() {}\n")
	writeGo(t, dir, "broken.go", "package p\nfunc Broken( {{{ not go\n")
	rules, _ := tree.ResolveRules(dir, nil)
	files, err := ExtractGo(dir, rules)
	if err != nil {
		t.Fatalf("a parse error must not be fatal: %v", err)
	}
	if len(files) != 1 || files[0].Path != "good.go" {
		t.Fatalf("want only good.go, got %+v", files)
	}
}

func TestRender_Deterministic(t *testing.T) {
	files := []FileSymbols{
		{Path: "a.go", Symbols: []Symbol{{Kind: "func", Name: "F", Signature: "func F()"}}},
	}
	if Render(files) != Render(files) {
		t.Error("Render not byte-deterministic")
	}
	out := Render(files)
	if !strings.Contains(out, "a.go\n  func F()\n") {
		t.Errorf("Render layout unexpected:\n%s", out)
	}
}

func TestRender_Empty(t *testing.T) {
	out := Render(nil)
	if !strings.Contains(out, "No Go symbols found") {
		t.Errorf("empty render should note no symbols: %q", out)
	}
}

func TestGenerate_And_CountSymbols(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", "package p\nfunc A() {}\ntype T struct{}\n")
	writeGo(t, dir, "b.go", "package p\nfunc B() {}\n")
	text, files, err := Generate(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := CountSymbols(files); got != 3 {
		t.Errorf("CountSymbols = %d; want 3", got)
	}
	if !strings.HasPrefix(text, "# Repomap\n") {
		t.Errorf("Generate text missing header: %q", text[:min(40, len(text))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
