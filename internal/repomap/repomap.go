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
// binary. TypeScript/JavaScript uses a zero-dep regex+brace scanner (see
// extract_tsjs.go). A new language is a new entry in the `extractors` registry.
//
// Known limitations (all intentional for this slice):
//   - Languages: Go + TS/JS. Other extensions are skipped (a registry add away).
//   - Only funcs, methods, and types. Exported const/var (including sentinel
//     errors) are omitted; a later slice may add ranked, budget-bounded ones.
//   - Composite type bodies collapse to the bare keyword (`type T struct`), so a
//     constraint interface loses its type set (`interface { ~int | ~string }` →
//     `interface`). Method/field detail is a ranking-driven, budgeted follow-up.
//   - Build-constrained (`//go:build ignore`) and generated files still
//     contribute symbols — they parse fine even when no build includes them.
package repomap

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	// Imports is the file's imported package paths (unquoted). Used by Rank to
	// compute intra-repo fan-in centrality; ignored by the default Render.
	Imports []string
}

// langExtractor is one language's signature extractor. extract takes a file's
// source and returns its top-level signatures (in source order) plus imported
// paths (used for Go fan-in; empty for languages without import-graph support
// yet). isTest reports whether a basename is that language's test file (skipped
// like Go's `_test.go`).
type langExtractor struct {
	extract func(src string) ([]Symbol, []string)
	isTest  func(base string) bool
}

// extractors maps a lowercased file extension to its extractor. Go uses the
// standard-library parser (exact); other languages use zero-dep regex
// extractors (a skeleton — less precise but deterministic). Additive by
// construction: a new language is a new entry, no other code changes.
var extractors = map[string]langExtractor{
	".go":  {extract: extractGoSource, isTest: func(b string) bool { return strings.HasSuffix(b, "_test.go") }},
	".ts":  {extract: extractTSJS, isTest: isJSTestFile},
	".tsx": {extract: extractTSJS, isTest: isJSTestFile},
	".js":  {extract: extractTSJS, isTest: isJSTestFile},
	".jsx": {extract: extractTSJS, isTest: isJSTestFile},
}

// Extract walks repoRoot for tracked, non-test source files whose extension has
// a registered extractor, honoring the same ignore rules as
// docs/file-structure.md, and extracts each file's top-level signatures. Files
// are returned sorted by path; symbols keep source order — both deterministic,
// the property caching depends on. A file that fails to extract is skipped, not
// fatal: the repomap is a convenience, never a gate.
func Extract(repoRoot string, rules tree.IgnoreRules) ([]FileSymbols, error) {
	var srcFiles []string
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A permission-denied or vanished path is a recoverable I/O
			// condition, not a reason to abort the whole map — the repomap is a
			// convenience, never a gate (mirrors internal/tree's own guard).
			// Skip it; propagate only genuinely unexpected walk errors.
			if os.IsPermission(err) || os.IsNotExist(err) {
				return nil
			}
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
			// Skip ignored dirs AND testdata/ — Go tooling ignores testdata,
			// and its fixtures are not part of the repo's API surface.
			if base == "testdata" || rules.Matches(rel, base) {
				return filepath.SkipDir
			}
			return nil
		}
		// Regular files only: a symlinked source file would otherwise be counted
		// twice (under both its own path and the target's); other irregular
		// entries aren't real source.
		if !d.Type().IsRegular() {
			return nil
		}
		if rules.Matches(rel, base) {
			return nil
		}
		// Dispatch by extension: include the file only when a language extractor
		// is registered for it and the file is not that language's test file.
		if ex, ok := extractors[strings.ToLower(filepath.Ext(base))]; !ok || ex.isTest(base) {
			return nil
		}
		srcFiles = append(srcFiles, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(srcFiles)

	out := make([]FileSymbols, 0, len(srcFiles))
	for _, rel := range srcFiles {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			continue // unreadable file — skip, never fatal
		}
		syms, imports := extractors[strings.ToLower(filepath.Ext(rel))].extract(string(data))
		if len(syms) == 0 {
			continue
		}
		out = append(out, FileSymbols{Path: rel, Symbols: syms, Imports: imports})
	}
	return out, nil
}

// extractGoSource parses Go source and returns its top-level signatures in
// source order plus its imported package paths (unquoted). Returns nil, nil on
// a parse error (skip, don't fail the whole map). Standard-library parser —
// exact, zero external dependency, no CGo.
func extractGoSource(src string) ([]Symbol, []string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil
	}
	var syms []Symbol
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			syms = append(syms, funcSignature(d))
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						syms = append(syms, typeSignature(ts))
					}
				}
			}
		}
	}
	// file.Imports is populated by the full parse — the file's imported package
	// paths, used by Rank for intra-repo fan-in.
	var imports []string
	for _, imp := range file.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err == nil {
			imports = append(imports, p)
		}
	}
	return syms, imports
}

// funcSignature renders a FuncDecl with its body (and doc) dropped — the exact
// stdlib-printed `func Name(params) results`, or `func (recv) Name(...)` for a
// method. Printing the mutated node is how we get a byte-accurate signature
// without hand-assembling parameter text.
func funcSignature(fn *ast.FuncDecl) Symbol {
	shallow := *fn
	shallow.Body = nil
	shallow.Doc = nil
	kind := "func"
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		kind = "method"
	}
	return Symbol{Kind: kind, Name: fn.Name.Name, Signature: printNode(&shallow)}
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
func typeSignature(ts *ast.TypeSpec) Symbol {
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
	sig := "type " + printNode(&spec)
	sig = strings.TrimSuffix(sig, " { }")
	sig = strings.TrimSuffix(sig, " {}")
	return Symbol{Kind: "type", Name: ts.Name.Name, Signature: sig}
}

// printNode pretty-prints an AST node to a single canonical line.
//
// It prints with a FRESH (empty) FileSet, not the one the node was parsed from:
// with no source positions to honor, the printer falls back to its own
// canonical layout, so `func F(\n a,\n b,\n)` and `func F(a, b)` render
// IDENTICALLY (source-layout independence — the caching invariant) and without
// the stray ` , )` artifacts a source-faithful print would leave. The only
// newlines the fresh printer emits are between the fields of an inline
// struct/interface; flattenOneLine turns those into Go's `;` separator so the
// one-line result stays valid Go.
func printNode(node ast.Node) string {
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces, Tabwidth: 4}
	if err := cfg.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return ""
	}
	return flattenOneLine(buf.String())
}

// flattenOneLine collapses a fresh-FileSet printer rendering to a single line.
// Newlines survive only between inline struct/interface members, where Go's
// automatic-semicolon rule requires a `;` when they share a line — but never
// adjacent to a brace. Intra-line alignment padding collapses to single spaces.
func flattenOneLine(s string) string {
	var lines []string
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.Join(strings.Fields(ln), " "); ln != "" {
			lines = append(lines, ln)
		}
	}
	var b strings.Builder
	for i, ln := range lines {
		if i > 0 {
			switch {
			case strings.HasPrefix(ln, "}"), strings.HasSuffix(lines[i-1], "{"):
				b.WriteByte(' ') // no semicolon adjacent to a brace
			default:
				b.WriteString("; ")
			}
		}
		b.WriteString(ln)
	}
	return b.String()
}

const repomapHeader = "# Repomap\n\nSignature skeleton (bodies dropped) — the repo's API surface for agents.\n"

// fileBlock renders one file's portion of the skeleton: a leading blank line,
// the path, then each signature indented two spaces. Shared by Render and the
// budget renderer so their output stays byte-identical, and used by Pack to
// cost each file.
func fileBlock(f FileSymbols) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(f.Path)
	b.WriteString("\n")
	for _, s := range f.Symbols {
		b.WriteString("  ")
		b.WriteString(s.Signature)
		b.WriteString("\n")
	}
	return b.String()
}

// Render assembles the deterministic skeleton text: each file's path as a
// header, its signatures indented beneath. Byte-stable by construction.
func Render(files []FileSymbols) string {
	if len(files) == 0 {
		return "# Repomap\n\nNo symbols found.\n"
	}
	var b strings.Builder
	b.WriteString(repomapHeader)
	for _, f := range files {
		b.WriteString(fileBlock(f))
	}
	return b.String()
}

// RenderWithOmitted renders kept files plus, when omitted > 0, a canonical
// truncation marker (§14.4) naming how many files were dropped to fit the
// budget. omitted == 0 renders identically to Render(kept).
func RenderWithOmitted(kept []FileSymbols, omitted int) string {
	if omitted <= 0 {
		return Render(kept)
	}
	var b strings.Builder
	b.WriteString(repomapHeader)
	for _, f := range kept {
		b.WriteString(fileBlock(f))
	}
	fmt.Fprintf(&b, "\n... (%d files omitted to fit the token budget)\n", omitted)
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
	files, err := Extract(repoRoot, rules)
	if err != nil {
		return "", nil, err
	}
	return Render(files), files, nil
}

// GenerateBudget is Generate + importance ranking + token-budget packing: it
// ranks files (see Rank) and keeps as many whole files as fit maxTokens,
// appending a truncation marker for the rest. maxTokens <= 0 behaves exactly
// like Generate — no ranking, no budget, the byte-stable path-sorted default.
// Returns the rendered text, the KEPT files, and the omitted count.
func GenerateBudget(repoRoot string, defaults []string, maxTokens int) (text string, kept []FileSymbols, omitted int, err error) {
	rules, err := tree.ResolveRules(repoRoot, defaults)
	if err != nil {
		return "", nil, 0, fmt.Errorf("resolve ignore rules: %w", err)
	}
	files, err := Extract(repoRoot, rules)
	if err != nil {
		return "", nil, 0, err
	}
	if maxTokens <= 0 {
		return Render(files), files, 0, nil
	}
	full := Render(files)
	ranked := Rank(repoRoot, files)
	kept, omitted = Pack(ranked, maxTokens)
	packed := RenderWithOmitted(kept, omitted)
	// Never-worse (§14.5): the truncation marker's fixed overhead can exceed a
	// few small omitted blocks, making the packed render LARGER than the full
	// one — worse AND lossy. When budgeting fails to actually shrink the output,
	// passthrough the full map; a budget must never make the output bigger.
	if len(packed) >= len(full) {
		return full, files, 0, nil
	}
	return packed, kept, omitted, nil
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
