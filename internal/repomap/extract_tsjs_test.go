package repomap

import (
	"reflect"
	"strings"
	"testing"
)

// tsSample exercises every declaration form the TS/JS extractor handles, plus
// the tricky bits: multi-line params, a multi-line union type, comments that
// must NOT match, class members that must NOT leak, and plain data consts that
// must be skipped.
const tsSample = `import { z } from 'zod';

// a line comment mentioning function fake() and class Ghost {} to ignore
/* block comment
   interface AlsoGhost { x: number }
   type Phantom = string; */

export function greet(name: string): string {
  return ` + "`hi ${name}`" + `;
}

async function loadData(url: string): Promise<Data> {
  return fetch(url);
}

export function* counter(): Generator<number> {
  yield 1;
}

export class Animal extends Base implements Runnable {
  legs = 4;
  move(): void {}
  private speak() { return "noise"; }
}

export interface Point {
  x: number;
  y: number;
}

export type ID = string | number;

export type Handler =
  | { kind: 'a'; run(): void }
  | { kind: 'b'; note: string };

export enum Color { Red, Green, Blue }

export const add = (a: number, b: number): number => a + b;

const makeThing = async (opts: Options) => {
  return opts;
};

export const legacy = function (x: number) {
  return x;
};

const PLAIN = { a: 1, b: 2 };
export const NUM = 42;

export function multi(
  a: number,
  b: string,
  c: boolean,
): Result {
  return null;
}

export function withObjParam(args: {
  first: string;
  second: number;
}): void {}
`

func tsSymbols(t *testing.T) []Symbol {
	t.Helper()
	syms, imports := extractTSJS(tsSample)
	if imports != nil {
		t.Errorf("imports = %v; want nil", imports)
	}
	return syms
}

func tsSigOf(syms []Symbol, name string) (Symbol, bool) {
	for _, s := range syms {
		if s.Name == name {
			return s, true
		}
	}
	return Symbol{}, false
}

func TestExtractTSJS(t *testing.T) {
	syms := tsSymbols(t)

	cases := []struct {
		name     string
		wantKind string
		wantSig  string
	}{
		{"greet", "func", "export function greet(name: string): string"},
		{"loadData", "func", "async function loadData(url: string): Promise<Data>"},
		{"counter", "func", "export function* counter(): Generator<number>"},
		{"Animal", "class", "export class Animal extends Base implements Runnable"},
		{"Point", "interface", "export interface Point"},
		{"ID", "type", "export type ID = string | number"},
		{"Handler", "type", "export type Handler = | { kind: 'a'; run(): void } | { kind: 'b'; note: string }"},
		{"Color", "enum", "export enum Color"},
		{"add", "const", "export const add = (a: number, b: number): number =>"},
		{"makeThing", "const", "const makeThing = async (opts: Options) =>"},
		{"legacy", "const", "export const legacy = function (x: number)"},
		// Multi-line parameter list collapses to a single line.
		{"multi", "func", "export function multi(a: number, b: string, c: boolean): Result"},
		// An object-type parameter is kept (its brace is not the body brace).
		{"withObjParam", "func", "export function withObjParam(args: { first: string; second: number; }): void"},
	}
	for _, c := range cases {
		s, ok := tsSigOf(syms, c.name)
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

func TestExtractTSJS_ExportPreserved(t *testing.T) {
	s, ok := tsSigOf(tsSymbols(t), "greet")
	if !ok || !strings.HasPrefix(s.Signature, "export ") {
		t.Errorf("greet sig should keep leading export: %q", s.Signature)
	}
}

func TestExtractTSJS_SourceOrderPreserved(t *testing.T) {
	var got []string
	for _, s := range tsSymbols(t) {
		got = append(got, s.Name)
	}
	want := []string{
		"greet", "loadData", "counter", "Animal", "Point", "ID", "Handler",
		"Color", "add", "makeThing", "legacy", "multi", "withObjParam",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v; want %v", got, want)
	}
}

func TestExtractTSJS_Deterministic(t *testing.T) {
	a, _ := extractTSJS(tsSample)
	b, _ := extractTSJS(tsSample)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("two runs differ:\n a = %v\n b = %v", a, b)
	}
}

func TestExtractTSJS_ClassMembersNotEmitted(t *testing.T) {
	syms := tsSymbols(t)
	for _, member := range []string{"move", "speak", "legs"} {
		if _, ok := tsSigOf(syms, member); ok {
			t.Errorf("class member %q was emitted as a top-level symbol", member)
		}
	}
	// And no member/body text should leak into any signature.
	for _, s := range syms {
		for _, leak := range []string{"return", "yield", "noise", "fetch", "legs = 4"} {
			if strings.Contains(s.Signature, leak) {
				t.Errorf("body token %q leaked into signature: %q", leak, s.Signature)
			}
		}
	}
}

func TestExtractTSJS_DataConstsSkipped(t *testing.T) {
	syms := tsSymbols(t)
	for _, name := range []string{"PLAIN", "NUM"} {
		if _, ok := tsSigOf(syms, name); ok {
			t.Errorf("plain data const %q should be skipped, not emitted", name)
		}
	}
}

func TestExtractTSJS_CommentsNotMatched(t *testing.T) {
	syms := tsSymbols(t)
	// Declarations that live only inside comments must never be extracted.
	for _, ghost := range []string{"fake", "Ghost", "AlsoGhost", "Phantom"} {
		if _, ok := tsSigOf(syms, ghost); ok {
			t.Errorf("declaration %q inside a comment was matched", ghost)
		}
	}
}

func TestExtractTSJS_GarbageNoPanic(t *testing.T) {
	inputs := []string{
		"",
		"   \n\t\n",
		"!@#$%^&*()_+{}[]<>?",
		"function", // keyword with nothing after it
		"const x =",
		"export type T =",               // unterminated type
		"class {",                       // anonymous, no name, unterminated
		"const f = (a, b",               // unterminated arrow params
		strings.Repeat("{", 5000),       // deeply unbalanced open braces
		strings.Repeat("}", 5000),       // deeply unbalanced close braces
		"const s = '\\'; class Fake {}", // tricky escaped quote in a string
	}
	for _, in := range inputs {
		syms, imports := extractTSJS(in) // must not panic
		if imports != nil {
			t.Errorf("imports = %v; want nil for %q", imports, in)
		}
		_ = syms
	}
}

func TestExtractTSJS_JSFlavors(t *testing.T) {
	// Plain JS (no type annotations) still extracts functions and classes.
	src := `export default function main() {}
function helper(a, b) { return a + b; }
export class Widget {}
export const boot = () => start();
`
	syms, _ := extractTSJS(src)
	want := map[string]string{"main": "func", "helper": "func", "Widget": "class", "boot": "const"}
	if len(syms) != len(want) {
		t.Fatalf("got %d symbols, want %d: %v", len(syms), len(want), syms)
	}
	for _, s := range syms {
		if k, ok := want[s.Name]; !ok || k != s.Kind {
			t.Errorf("unexpected symbol %q kind %q", s.Name, s.Kind)
		}
	}
}

func TestExtractTSJS_ObjectReturnType(t *testing.T) {
	// A function whose RETURN type is an object literal (possibly in a union,
	// or reached through a multi-line generic param) must keep the whole
	// return type — the first top-level `{` is the type, not the body.
	src := `export function make(
  a: number,
): { verdict: string; sig: number } | null {
  return null;
}
function tup(): Array<{ x: number }> { return []; }
`
	syms, _ := extractTSJS(src)
	want := map[string]string{
		"make": "export function make(a: number): { verdict: string; sig: number } | null",
		"tup":  "function tup(): Array<{ x: number }>",
	}
	if len(syms) != 2 {
		t.Fatalf("got %d symbols, want 2: %v", len(syms), syms)
	}
	for _, s := range syms {
		if w, ok := want[s.Name]; !ok || s.Signature != w {
			t.Errorf("%s: sig = %q; want %q", s.Name, s.Signature, w)
		}
	}
}

// A template literal whose `${…}` interpolation contains a nested string with a
// literal backtick (or a nested template literal, or a brace hidden in a nested
// string) must not terminate the scan early — every declaration after the
// literal must still be extracted (issue #219). The old flat-delimiter scanner
// closed at the first inner backtick and masked all subsequent real source.
func TestExtractTSJS_TemplateLiteralInterpolation(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string // top-level symbol names, in source order
	}{
		// Nested string containing a literal backtick inside `${…}` — the
		// canonical #219 repro. `afterNested` must survive.
		{
			"nested-string-with-backtick",
			"const t = `a ${f(\"`\")} b`;\nexport function afterNested(): void {}\n",
			[]string{"afterNested"},
		},
		// A nested template literal inside the interpolation must recurse, not
		// close the outer literal at its opening/closing backtick.
		{
			"nested-template-literal",
			"const t = `outer ${`inner ${x}`} end`;\nexport function afterTmpl(): void {}\n",
			[]string{"afterTmpl"},
		},
		// Several interpolations in one literal all scan correctly.
		{
			"multiple-interpolations",
			"const t = `${a} plus ${b} plus ${c}`;\nexport function afterMulti(): void {}\n",
			[]string{"afterMulti"},
		},
		// A plain literal with no interpolation still scans (and the data const
		// is skipped, not emitted).
		{
			"plain-no-interpolation",
			"const t = `just a plain literal`;\nexport function afterPlain(): void {}\n",
			[]string{"afterPlain"},
		},
		// A `}` hidden inside a nested string in the interpolation must not close
		// the interpolation early; brace depth stays balanced so the enclosing
		// function body closes correctly and the next decl is reached.
		{
			"brace-in-nested-interp-string",
			"export function wrap(): void {\n  const s = `x ${obj[\"}\"]} y`;\n}\nexport function afterBody(): void {}\n",
			[]string{"wrap", "afterBody"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			syms, _ := extractTSJS(c.src)
			var got []string
			for _, s := range syms {
				got = append(got, s.Name)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("symbols = %v; want %v", got, c.want)
			}
		})
	}
}

func TestIsJSTestFile(t *testing.T) {
	cases := map[string]bool{
		"foo.test.ts":        true,
		"foo.spec.tsx":       true,
		"a.b.test.js":        true,
		"component.spec.jsx": true,
		"foo.ts":             false,
		"testing.ts":         false, // "test" without dots is not a test file
		"spec.ts":            false,
	}
	for base, want := range cases {
		if got := isJSTestFile(base); got != want {
			t.Errorf("isJSTestFile(%q) = %v; want %v", base, got, want)
		}
	}
}

// --- Regression tests for the R4 dual-review findings (const arrow detection
// and the `extends` word-boundary). ---

// A semicolon-less data const must NOT be emitted or latch onto the next
// declaration's arrow (clud-bug MEDIUM).
func TestExtractTSJS_SemicolonlessDataConst(t *testing.T) {
	syms, _ := extractTSJS("const VERSION = '1.0.0'\nconst parse = (x) => x\n")
	if _, ok := tsSigOf(syms, "VERSION"); ok {
		t.Error("semicolon-less data const VERSION must be skipped, not emitted")
	}
	if s, ok := tsSigOf(syms, "parse"); !ok || s.Signature != "const parse = (x) =>" {
		t.Errorf("parse arrow wrong: %q ok=%v", s.Signature, ok)
	}
}

// An arrow with an object-literal RETURN TYPE must be kept (adversarial #1).
func TestExtractTSJS_ArrowObjectReturnType(t *testing.T) {
	syms, _ := extractTSJS("export const build = (): { ok: boolean } => ({ ok: true })\n")
	if s, ok := tsSigOf(syms, "build"); !ok || s.Signature != "export const build = (): { ok: boolean } =>" {
		t.Errorf("arrow with object return type wrong: %q ok=%v", s.Signature, ok)
	}
}

// A const bound to an expression that merely CONTAINS an arrow (a ternary) is
// not a direct arrow function → skipped, no misleading signature (adversarial #2).
func TestExtractTSJS_ConstTernaryNotArrow(t *testing.T) {
	syms, _ := extractTSJS("export const pick = cond ? (a) => a : fallback;\n")
	if _, ok := tsSigOf(syms, "pick"); ok {
		t.Error("const bound to a ternary must be skipped (not a direct arrow)")
	}
}

// A generic arrow still extracts (guard against the rewrite regressing it).
func TestExtractTSJS_GenericArrow(t *testing.T) {
	syms, _ := extractTSJS("export const identity = <T>(x: T): T => x;\n")
	if s, ok := tsSigOf(syms, "identity"); !ok || s.Signature != "export const identity = <T>(x: T): T =>" {
		t.Errorf("generic arrow wrong: %q ok=%v", s.Signature, ok)
	}
}

// A type/base name ending in "extends" must not trigger the `extends`
// type-position heuristic and swallow the body + the next decl (adversarial #3).
func TestExtractTSJS_ExtendsWordBoundary(t *testing.T) {
	src := "class Foo extends myextends { hidden(): void {} }\nexport function afterFoo(): void {}\n"
	syms, _ := extractTSJS(src)
	if s, ok := tsSigOf(syms, "Foo"); !ok || s.Signature != "class Foo extends myextends" {
		t.Errorf("class header should stop at the body brace: %q ok=%v", s.Signature, ok)
	}
	if _, ok := tsSigOf(syms, "afterFoo"); !ok {
		t.Error("afterFoo must be extracted separately, not swallowed")
	}
}
