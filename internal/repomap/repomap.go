// Package repomap builds a deterministic SIGNATURE SKELETON of a repository —
// the token-killer's Phase 2 structural win. Where docs/file-structure.md gives
// an agent the bare name-tree (the "where"), a repomap gives the API surface an
// agent actually reasons over (the "what"): every top-level function, method,
// and type rendered as its signature with the BODY DROPPED.
//
// The point is token economy. A signature skeleton is a tiny, cache-stable
// fraction of the source it summarizes, yet it carries the structure an agent
// would otherwise reconstruct by reading whole files. Like every logmind
// surface it is deterministic (sorted files, stdlib pretty-printing, no
// timestamps / absolute paths / filesystem order) so it caches as a stable
// prefix.
//
// Go extraction uses the standard library (go/parser + go/printer) — accurate,
// zero external dependency, and no CGo, so logmind stays a single static
// binary. Other languages get regex-based extraction in a later slice; this
// slice ships the Go path (logmind's own dogfood case is exact).
package repomap

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thrillmade/logmind/internal/tree"
)

// Symbol is one top-level declaration reduced to its signature (no body).
type Symbol struct {
	Kind      string // "func", "method", or "type"
	Name      string // declared name (methods: the method name)
	Signature string // the exact signature text, body dropped
}

// FileSymbols groups a file's extracted symbols under its repo-relative path.
type FileSymbols struct {
	Path    string // repo-relative, forward-slashed (stable across platforms)
	Symbols []Symbol
}

// ExtractGo walks repoRoot for tracked, non-test .go files (honoring the same
// ignore rules as docs/file-structure.md) and extracts each file's top-level
// func / method / type signatures. Files are returned sorted by path; symbols
// keep source order within a file — both deterministic, the property caching
// depends on. A file that fails to parse is skipped, not fatal: the repomap is
// a convenience, never a gate.
func ExtractGo(repoRoot string, rules tree.IgnoreRules) ([]FileSymbols, error) {
	var goFiles []string
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		base := d.Name()
		if d.IsDir() {
			if rules.Matches(rel, base) {
				return filepath.SkipDir
			}
			return nil
		}
		if rules.Matches(rel, base) {
			return nil
		}
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") {
			return nil
		}
		goFiles = append(goFiles, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(goFiles)

	out := make([]FileSymbols, 0, len(goFiles))
	for _, rel := range goFiles {
		syms := extractGoFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if len(syms) == 0 {
			continue
		}
		out = append(out, FileSymbols{Path: rel, Symbols: syms})
	}
	return out, nil
}

// extractGoFile parses a single Go file and returns its top-level signatures in
// source order. Returns nil on a parse error (skip, don't fail the whole map).
func extractGoFile(absPath string) []Symbol {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	var syms []Symbol
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			syms = append(syms, funcSignature(fset, d))
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						syms = append(syms, typeSignature(fset, ts))
					}
				}
			}
		}
	}
	return syms
}

// funcSignature renders a FuncDecl with its body (and doc) dropped — the exact
// stdlib-printed `func Name(params) results`, or `func (recv) Name(...)` for a
// method. Printing the mutated node is how we get a byte-accurate signature
// without hand-assembling parameter text.
func funcSignature(fset *token.FileSet, fn *ast.FuncDecl) Symbol {
	shallow := *fn
	shallow.Body = nil
	shallow.Doc = nil
	kind := "func"
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		kind = "method"
	}
	return Symbol{Kind: kind, Name: fn.Name.Name, Signature: printNode(fset, &shallow)}
}

// typeSignature renders a type as a COLLAPSED signature. Composite types keep
// their generic type parameters but drop their members — `type List[T any]
// struct{}` / `type Ifc interface{}` — so the skeleton stays dense without
// losing the generic surface. Aliases and named scalar/func/map/etc. types keep
// their full underlying (`type ID = string`, `type Count int`), where the
// underlying IS the signal.
//
// It prints the TypeSpec node itself (printer renders `Name[TypeParams] …`
// correctly, which hand-assembly from ts.Name would drop) with any struct/
// interface body emptied.
func typeSignature(fset *token.FileSet, ts *ast.TypeSpec) Symbol {
	spec := *ts
	spec.Doc = nil
	spec.Comment = nil
	// Empty the composite body AND zero the brace positions: preserving the
	// original Opening/Closing token positions makes the printer render
	// `struct { }` for a multi-line source but `struct{}` for a single-line
	// one — source-layout-dependent output that would break the caching
	// invariant. A zero-position empty FieldList always renders canonically.
	switch t := ts.Type.(type) {
	case *ast.StructType:
		empty := *t
		empty.Fields = &ast.FieldList{}
		spec.Type = &empty
	case *ast.InterfaceType:
		empty := *t
		empty.Methods = &ast.FieldList{}
		spec.Type = &empty
	}
	// The emptied composite prints as `... struct { }` / `... interface { }`.
	// Trim the empty braces to the dense keyword form (`type Rec struct`) — it
	// reads as "Rec is a struct" rather than falsely implying an EMPTY struct,
	// and type params survive ahead of the keyword. Non-composite types (no
	// trailing empty braces) are unaffected.
	sig := "type " + printNode(fset, &spec)
	sig = strings.TrimSuffix(sig, " { }")
	sig = strings.TrimSuffix(sig, " {}")
	return Symbol{Kind: "type", Name: ts.Name.Name, Signature: sig}
}

// printNode pretty-prints an AST node to a single logical line using the
// standard printer, collapsing any internal newlines a multi-line result type
// might introduce so every symbol stays one row.
func printNode(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces, Tabwidth: 4}
	if err := cfg.Fprint(&buf, fset, node); err != nil {
		return ""
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

// Render assembles the deterministic skeleton text: each file's path as a
// header, its signatures indented beneath. Byte-stable by construction.
func Render(files []FileSymbols) string {
	if len(files) == 0 {
		return "# Repomap\n\nNo Go symbols found.\n"
	}
	var b strings.Builder
	b.WriteString("# Repomap\n\n")
	b.WriteString("Signature skeleton (bodies dropped) — the repo's API surface for agents.\n")
	for _, f := range files {
		b.WriteString("\n")
		b.WriteString(f.Path)
		b.WriteString("\n")
		for _, s := range f.Symbols {
			b.WriteString("  ")
			b.WriteString(s.Signature)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// Generate is the one-call entry point: resolve ignore rules the same way
// file-structure does, extract Go signatures, and render the skeleton. Returns
// the rendered text plus the structured per-file symbols (so callers can report
// counts without re-parsing).
func Generate(repoRoot string, defaults []string) (string, []FileSymbols, error) {
	rules, err := tree.ResolveRules(repoRoot, defaults)
	if err != nil {
		return "", nil, fmt.Errorf("resolve ignore rules: %w", err)
	}
	files, err := ExtractGo(repoRoot, rules)
	if err != nil {
		return "", nil, err
	}
	return Render(files), files, nil
}

// CountSymbols totals the symbols across all files — the denominator for the
// `ok repomap` receipt.
func CountSymbols(files []FileSymbols) int {
	n := 0
	for _, f := range files {
		n += len(f.Symbols)
	}
	return n
}
