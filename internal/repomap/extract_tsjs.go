package repomap

// TypeScript / JavaScript signature extraction. Where the Go path uses the
// standard-library parser (exact), TS/JS has no stdlib parser we can lean on
// without a heavy external dependency (and logmind ships as a single static,
// CGo-free binary). So this is a deterministic, zero-dependency regex + brace
// scanner: less precise than a real parser, but byte-stable and panic-free —
// the properties caching and "never a gate" depend on.
//
// It extracts TOP-LEVEL (brace-depth 0) declarations only, in source order,
// each reduced to a dense one-line header with the body dropped — the direct
// analog of the Go extractor collapsing a composite type to its bare keyword:
//
//   - function / async function / generator function* -> "func"
//   - class (incl. abstract / extends / implements)   -> "class"  (header only)
//   - interface                                        -> "interface" (header only)
//   - type Name = <RHS>                                -> "type"   (whole RHS, one line)
//   - enum / const enum                                -> "enum"   (header only)
//   - const name = (params) => / = function(params)    -> "const"  (function-valued only)
//
// A leading `export` (part of the public signature) is preserved; the body,
// opening brace, trailing `=> {…}` and statement `;` are dropped. Multi-line
// parameter lists and RHS expressions collapse to a single line.
//
// Known limitations (deferred, all intentional for this slice):
//   - No descent into class/interface bodies — members are not emitted (the
//     depth-0-only rule, matching Go's collapsed composites).
//   - Anonymous `export default function()` / `export default class {}` are
//     skipped (no name to key the Symbol on).
//   - Best-effort comment/string stripping tracks strings but not regex
//     literals, so a top-level regex with unbalanced braces (`/{/`) could skew
//     brace depth. Rare in practice; not worth a full lexer here.
//   - Only declarations at the start of a logical line (after optional
//     `export`/modifiers) are matched — not a second statement after `;` on
//     the same line.

import (
	"regexp"
	"strings"
)

// isJSTestFile reports whether base is a JS/TS test/spec file, so it is skipped
// like Go's `_test.go`. Matches the ubiquitous `*.test.*` / `*.spec.*`
// conventions (jest, vitest, mocha, jasmine, ...).
func isJSTestFile(base string) bool {
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
}

// Declaration matchers, anchored at a (whitespace-trimmed) logical line start.
// Each keeps a single capture group for the declared name. `\b` after
// `function` lets both `function*` and `function foo` match while rejecting an
// identifier like `functionish`. The other keywords require trailing `\s+`,
// which already rejects `typeof` / `constant` / `interfaces` etc.
var (
	reFunc  = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:async\s+)?function\b\s*\*?\s*([A-Za-z_$][\w$]*)`)
	reClass = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)`)
	reIface = regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?interface\s+([A-Za-z_$][\w$]*)`)
	reType  = regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?type\s+([A-Za-z_$][\w$]*)`)
	reEnum  = regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?(?:const\s+)?enum\s+([A-Za-z_$][\w$]*)`)
	reConst = regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?const\s+([A-Za-z_$][\w$]*)`)
)

// extractTSJS returns a TS/JS file's top-level signatures in source order. The
// second return (imports) is always nil — the TS import graph is not modeled
// for ranking in this slice. Garbage or empty input yields nil, never a panic.
func extractTSJS(src string) ([]Symbol, []string) {
	// Normalize CRLF so line handling and brace counting see a single '\n'.
	src = strings.ReplaceAll(src, "\r\n", "\n")
	display, mask := maskTSJS(src)
	if len(mask) == 0 {
		return nil, nil
	}

	// Byte offset of each line's first character. display and mask are the same
	// length and index-aligned, so a mask offset indexes display identically.
	lineStart := []int{0}
	for i := 0; i < len(mask); i++ {
		if mask[i] == '\n' {
			lineStart = append(lineStart, i+1)
		}
	}
	numLines := len(lineStart)
	lineEnd := func(li int) int {
		if li+1 < numLines {
			return lineStart[li+1] - 1 // drop the trailing '\n'
		}
		return len(mask)
	}
	lineOf := func(off int) int {
		lo, hi := 0, numLines-1
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if lineStart[mid] <= off {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		return lo
	}

	var syms []Symbol
	depth := 0 // running brace depth; declarations are matched only at depth 0
	li := 0
	for li < numLines {
		if depth == 0 {
			s, e := lineStart[li], lineEnd(li)
			lineMask := mask[s:e]
			trimmed := strings.TrimLeft(lineMask, " \t")
			off := s + (len(lineMask) - len(trimmed))
			if sym, endOff, kind, ok := matchDecl(display, mask, trimmed, off); ok {
				syms = append(syms, sym)
				// A `type` alias is brace-balanced and its RHS can span many
				// depth-0 lines (union members etc.); skip past it so those
				// lines are not re-scanned for declarations. Every other kind
				// keeps only a header — let the normal walk descend so the body
				// brace pushes depth and its members are skipped by depth.
				if kind == "type" {
					if el := lineOf(endOff); el > li {
						li = el + 1
						continue
					}
				}
			}
		}
		depth += braceDelta(mask[lineStart[li]:lineEnd(li)])
		if depth < 0 {
			depth = 0
		}
		li++
	}
	return syms, nil
}

// matchDecl tries each declaration form against a trimmed line start. On a hit
// it collects the dense one-line signature (from the flat display text) and
// returns the Symbol, the offset where the signature ended, its kind, and true.
func matchDecl(display, mask, trimmed string, off int) (Symbol, int, string, bool) {
	if m := reFunc.FindStringSubmatch(trimmed); m != nil {
		raw, end := scanToBody(display, mask, off)
		return Symbol{Kind: "func", Name: m[1], Signature: tidyParams(collapse(raw))}, end, "func", true
	}
	if m := reClass.FindStringSubmatch(trimmed); m != nil {
		raw, end := scanToBody(display, mask, off)
		return Symbol{Kind: "class", Name: m[1], Signature: collapse(raw)}, end, "class", true
	}
	if m := reIface.FindStringSubmatch(trimmed); m != nil {
		raw, end := scanToBody(display, mask, off)
		return Symbol{Kind: "interface", Name: m[1], Signature: collapse(raw)}, end, "interface", true
	}
	if m := reType.FindStringSubmatch(trimmed); m != nil {
		raw, end := scanType(display, mask, off)
		return Symbol{Kind: "type", Name: m[1], Signature: collapse(raw)}, end, "type", true
	}
	// enum before const: `const enum` must not be swallowed by the const form.
	if m := reEnum.FindStringSubmatch(trimmed); m != nil {
		raw, end := scanToBody(display, mask, off)
		return Symbol{Kind: "enum", Name: m[1], Signature: collapse(raw)}, end, "enum", true
	}
	if m := reConst.FindStringSubmatch(trimmed); m != nil {
		if sym, end, ok := scanConst(display, mask, off, m[1]); ok {
			return sym, end, "const", true
		}
	}
	return Symbol{}, 0, "", false
}

// collapse renders raw signature text as one line with single-space runs.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// tidyParams removes the cosmetic artifacts that a trailing-comma, multi-line
// parameter list leaves after collapse: the space just inside the parens and a
// dangling comma before the closer — so `foo( a, b, ): T` reads `foo(a, b): T`,
// matching the density of the Go path's printer output.
func tidyParams(s string) string {
	s = strings.ReplaceAll(s, "( ", "(")
	s = strings.ReplaceAll(s, " )", ")")
	s = strings.ReplaceAll(s, ",)", ")")
	return s
}

// braceDelta is a line's net `{` minus `}` count (on masked text, so braces
// inside strings/comments don't count).
func braceDelta(s string) int {
	d := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			d++
		case '}':
			d--
		}
	}
	return d
}

// scanToBody collects a header (class/interface/enum/function) from off up to
// the body-opening `{` or a terminating `;` at top level. `()`/`[]`/`{}` nesting
// is tracked so an object-type parameter `(args: { ... })` does not end the scan
// early — only a brace at all-zero depth is the body. Returns the collected
// display text and the stop offset.
func scanToBody(display, mask string, off int) (string, int) {
	paren, brack, brace := 0, 0, 0
	var b strings.Builder
	for i := off; i < len(mask); i++ {
		c := mask[i]
		if paren == 0 && brack == 0 && brace == 0 {
			if c == ';' {
				return b.String(), i
			}
			// A top-level `{` is the body — UNLESS the preceding token puts us
			// in type position (`: { ... }` return type, a `<{...}>` type arg,
			// a union/intersection member), in which case it opens an object
			// TYPE, not the body: consume it and keep looking for the real body.
			if c == '{' && !inTypePosition(b.String()) {
				return b.String(), i
			}
		}
		trackDepth(c, &paren, &brack, &brace)
		b.WriteByte(display[i])
	}
	return b.String(), len(mask)
}

// inTypePosition reports whether collected header text ends on a token after
// which a `{` would open an object TYPE rather than a body: the return-type
// colon, a union `|` / intersection `&`, a `<`/`,` inside a type-argument list,
// or an arrow / `extends` clause.
func inTypePosition(s string) bool {
	s = strings.TrimRight(s, " \t\n")
	if s == "" {
		return false
	}
	if strings.HasSuffix(s, "=>") || strings.HasSuffix(s, "extends") {
		return true
	}
	switch s[len(s)-1] {
	case ':', '|', '&', ',', '<':
		return true
	}
	return false
}

// scanType collects a `type Name = RHS` alias from off to its statement end:
// the `;` at top level, or — lacking one (ASI) — a line boundary where nesting
// is balanced and neither side continues the expression. Newlines become
// spaces. `<>` is deliberately NOT tracked (a function type's `=>` would corrupt
// angle depth); brace/paren/bracket nesting is enough to find the real `;`.
func scanType(display, mask string, off int) (string, int) {
	paren, brack, brace := 0, 0, 0
	var b strings.Builder
	n := len(mask)
	for i := off; i < n; i++ {
		c := mask[i]
		if c == ';' && paren == 0 && brack == 0 && brace == 0 {
			return b.String(), i
		}
		if c == '\n' {
			if paren == 0 && brack == 0 && brace == 0 &&
				!endsWithTypeCont(b.String()) && !nextStartsCont(mask, i+1) {
				return b.String(), i
			}
			b.WriteByte(' ')
			continue
		}
		trackDepth(c, &paren, &brack, &brace)
		b.WriteByte(display[i])
	}
	return b.String(), n
}

// scanConst handles a `const name = …` whose RHS is a function value (arrow or
// function expression); it returns ok=false for a plain data const (object,
// array, literal, call), which the caller then skips. The signature keeps up to
// the arrow `=>` (arrows) or the parameter list's closing `)` (function
// expressions) — the body is dropped.
func scanConst(display, mask string, off int, name string) (Symbol, int, bool) {
	n := len(mask)
	// Find the assignment `=` at top level (skipping `==`, `=>`, `<=`, compound
	// assigns, and any `=` nested in `()`/`[]`/`{}` or an arrow in the LHS type).
	paren, brack, brace := 0, 0, 0
	eq := -1
	for i := off; i < n; i++ {
		c := mask[i]
		if paren == 0 && brack == 0 && brace == 0 {
			if c == ';' {
				return Symbol{}, 0, false // `const x: T;` — no initializer
			}
			if c == '=' && isAssignEq(mask, i) {
				eq = i
				break
			}
		}
		trackDepth(c, &paren, &brack, &brace)
	}
	if eq == -1 {
		return Symbol{}, 0, false
	}

	j := skipSpace(mask, eq+1)
	if hasWord(mask, j, "async") {
		j = skipSpace(mask, j+len("async"))
	}
	if j >= n {
		return Symbol{}, 0, false
	}

	// Function expression: `= function (params)` — keep through the params.
	if hasWord(mask, j, "function") {
		k := j + len("function")
		for k < n && mask[k] != '(' {
			if mask[k] == '{' || mask[k] == ';' {
				return Symbol{}, 0, false
			}
			k++
		}
		if k >= n {
			return Symbol{}, 0, false
		}
		if end := matchParen(mask, k); end >= 0 {
			return Symbol{Kind: "const", Name: name, Signature: tidyParams(collapse(display[off : end+1]))}, end, true
		}
		return Symbol{}, 0, false
	}

	// Arrow: the first `=>` at top level before any body `{` or terminator `;`.
	p, br, bc := 0, 0, 0
	for i := j; i < n; i++ {
		c := mask[i]
		if p == 0 && br == 0 && bc == 0 {
			if c == '=' && i+1 < n && mask[i+1] == '>' {
				return Symbol{Kind: "const", Name: name, Signature: tidyParams(collapse(display[off : i+2]))}, i + 1, true
			}
			if c == '{' || c == ';' {
				return Symbol{}, 0, false // body/terminator before any arrow
			}
		}
		trackDepth(c, &p, &br, &bc)
	}
	return Symbol{}, 0, false
}

// trackDepth updates paren/bracket/brace counters for one masked char. Closers
// never go negative (defensive against unbalanced/garbage input).
func trackDepth(c byte, paren, brack, brace *int) {
	switch c {
	case '(':
		*paren++
	case ')':
		if *paren > 0 {
			*paren--
		}
	case '[':
		*brack++
	case ']':
		if *brack > 0 {
			*brack--
		}
	case '{':
		*brace++
	case '}':
		if *brace > 0 {
			*brace--
		}
	}
}

// isAssignEq reports whether the `=` at i is a plain assignment (not part of
// `==`, `=>`, `<=`, `>=`, `!=`, `+=`, or another compound operator).
func isAssignEq(mask string, i int) bool {
	if i+1 < len(mask) && (mask[i+1] == '=' || mask[i+1] == '>') {
		return false
	}
	if i > 0 && strings.IndexByte("=!<>+-*/%&|^~?", mask[i-1]) >= 0 {
		return false
	}
	return true
}

// matchParen returns the index of the `)` matching the `(` at open, or -1.
func matchParen(mask string, open int) int {
	depth := 0
	for i := open; i < len(mask); i++ {
		switch mask[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// skipSpace advances past spaces, tabs, and newlines.
func skipSpace(mask string, i int) int {
	for i < len(mask) && (mask[i] == ' ' || mask[i] == '\t' || mask[i] == '\n' || mask[i] == '\r') {
		i++
	}
	return i
}

// hasWord reports whether word sits at mask[i] on identifier boundaries.
func hasWord(mask string, i int, word string) bool {
	if i+len(word) > len(mask) || mask[i:i+len(word)] != word {
		return false
	}
	if i > 0 && isIdentByte(mask[i-1]) {
		return false
	}
	if a := i + len(word); a < len(mask) && isIdentByte(mask[a]) {
		return false
	}
	return true
}

func isIdentByte(b byte) bool {
	return b == '_' || b == '$' ||
		(b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// endsWithTypeCont reports whether accumulated type RHS text ends on an operator
// that must continue onto the next line (so an ASI line break is NOT the end).
func endsWithTypeCont(s string) bool {
	s = strings.TrimRight(s, " \t")
	if s == "" || strings.HasSuffix(s, "=>") || strings.HasSuffix(s, "extends") {
		return true
	}
	switch s[len(s)-1] {
	case '=', '|', '&', ',', '(', '<', '?', ':', '.':
		return true
	}
	return false
}

// nextStartsCont reports whether the next non-blank line begins with a token
// that continues a type expression (a union `|`, intersection `&`, `extends`,
// etc.), so an ASI line break is NOT the end.
func nextStartsCont(mask string, off int) bool {
	i := off
	for i < len(mask) && (mask[i] == ' ' || mask[i] == '\t') {
		i++
	}
	if i >= len(mask) || mask[i] == '\n' {
		return false // blank line or EOF
	}
	rest := mask[i:]
	if strings.HasPrefix(rest, "=>") || strings.HasPrefix(rest, "extends") {
		return true
	}
	switch mask[i] {
	case '|', '&', '?', ':', '.', ',':
		return true
	}
	return false
}

// maskTSJS returns two index-aligned, equal-length renderings of src:
//   - display: comments removed (so they never leak into a signature), string
//     and template literals kept intact.
//   - mask: display with string/template interiors replaced by a neutral 'x',
//     so structural scanning (brace depth, keyword matching, terminators) never
//     trips on a brace, quote, or `//` living inside a string.
//
// Every branch writes the same number of bytes to both, and newlines are
// mirrored, so a byte offset means the same position in each. This is a
// best-effort stripper (no regex-literal awareness), which the package doc notes.
func maskTSJS(src string) (display, mask string) {
	var d, m strings.Builder
	d.Grow(len(src))
	m.Grow(len(src))
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '/' && i+1 < n && src[i+1] == '/':
			for i < n && src[i] != '\n' {
				i++ // drop to end of line; the '\n' is copied next iteration
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			d.WriteByte(' ') // one space so tokens around the comment don't fuse
			m.WriteByte(' ')
			i += 2
			for i < n && !(src[i] == '*' && i+1 < n && src[i+1] == '/') {
				if src[i] == '\n' { // preserve line structure across both
					d.WriteByte('\n')
					m.WriteByte('\n')
				}
				i++
			}
			i += 2 // consume the closing */ (harmless if at EOF)
		case c == '\'' || c == '"' || c == '`':
			quote := c
			d.WriteByte(c)
			m.WriteByte(c)
			i++
			for i < n {
				ch := src[i]
				if ch == '\\' && i+1 < n {
					nx := src[i+1]
					d.WriteByte(ch)
					m.WriteByte(neutralOrNewline(ch))
					d.WriteByte(nx)
					m.WriteByte(neutralOrNewline(nx))
					i += 2
					continue
				}
				d.WriteByte(ch)
				m.WriteByte(neutralOrNewline(ch))
				i++
				if ch == quote {
					break
				}
			}
		default:
			d.WriteByte(c)
			m.WriteByte(c)
			i++
		}
	}
	return d.String(), m.String()
}

// neutralOrNewline maps a string-interior byte to a structurally neutral 'x',
// but keeps a newline so line offsets stay mirrored between display and mask.
func neutralOrNewline(b byte) byte {
	if b == '\n' {
		return '\n'
	}
	return 'x'
}
