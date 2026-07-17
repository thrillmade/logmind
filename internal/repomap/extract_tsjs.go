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
	if strings.HasSuffix(s, "=>") || endsWithWord(s, "extends") {
		return true
	}
	switch s[len(s)-1] {
	case ':', '|', '&', ',', '<':
		return true
	}
	return false
}

// endsWithWord reports whether s ends with the whole word w (w preceded by a
// non-identifier byte or start-of-string) — so a type named `myextends` does
// NOT count as ending in the `extends` keyword.
func endsWithWord(s, w string) bool {
	if !strings.HasSuffix(s, w) {
		return false
	}
	i := len(s) - len(w)
	return i == 0 || !isIdentByte(s[i-1])
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

	// Arrow function. The RHS must be a DIRECT arrow — a parenthesized (or
	// generic-then-parenthesized) parameter list, or a single bare-identifier
	// param, reaching `=>` past an optional return type. This rejects a plain
	// data const (string/number/object/call) and a const bound to an expression
	// that merely CONTAINS an arrow (e.g. a ternary), and never crosses a
	// statement boundary — so a semicolon-less data const cannot latch onto the
	// next declaration's arrow.
	k := j
	if mask[k] == '<' { // generic arrow: <T>(params) =>
		if k = matchAngle(mask, k); k < 0 {
			return Symbol{}, 0, false
		}
		k = skipSpace(mask, k)
	}
	if k < n && mask[k] == '(' {
		closeParen := matchParen(mask, k)
		if closeParen < 0 {
			return Symbol{}, 0, false
		}
		if arrow := findArrow(display, mask, closeParen+1); arrow >= 0 {
			return Symbol{Kind: "const", Name: name, Signature: tidyParams(collapse(display[off : arrow+2]))}, arrow + 1, true
		}
		return Symbol{}, 0, false
	}
	// Single unparenthesized param: `x => …`.
	if k < n && isIdentByte(mask[k]) {
		e := k
		for e < n && isIdentByte(mask[e]) {
			e++
		}
		if s := skipSpace(mask, e); s+1 < n && mask[s] == '=' && mask[s+1] == '>' {
			return Symbol{Kind: "const", Name: name, Signature: tidyParams(collapse(display[off : s+2]))}, s + 1, true
		}
	}
	return Symbol{}, 0, false
}

// matchAngle returns the index just past the `>` closing the `<` at open,
// tracking nesting, or -1 for a malformed list. Best-effort (a type-parameter
// list has no comparison operators to confuse the count).
func matchAngle(mask string, open int) int {
	depth := 0
	for i := open; i < len(mask); i++ {
		switch mask[i] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				return i + 1
			}
		case ';', '{', '(', ')':
			return -1
		}
	}
	return -1
}

// findArrow scans from off (just past an arrow's `)` parameter list) for the
// `=>` that makes it an arrow function, allowing an optional return-type
// annotation that may itself contain object-type braces (`(): { ok } =>`).
// Returns the index of the `=` in `=>`, or -1 at a statement boundary (`;`, a
// top-level newline, or a `{` that is a body rather than an object type).
func findArrow(display, mask string, off int) int {
	paren, brack, brace := 0, 0, 0
	var b strings.Builder
	for i := off; i < len(mask); i++ {
		c := mask[i]
		if paren == 0 && brack == 0 && brace == 0 {
			if c == '=' && i+1 < len(mask) && mask[i+1] == '>' {
				return i
			}
			if c == ';' || c == '\n' {
				return -1
			}
			if c == '{' && !inTypePosition(b.String()) {
				return -1
			}
		}
		trackDepth(c, &paren, &brack, &brace)
		b.WriteByte(display[i])
	}
	return -1
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
	if s == "" || strings.HasSuffix(s, "=>") || endsWithWord(s, "extends") {
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
	if strings.HasPrefix(mask[i:], "=>") || hasWord(mask, i, "extends") {
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
			i = maskLineComment(src, i)
		case c == '/' && i+1 < n && src[i+1] == '*':
			i = maskBlockComment(src, i, &d, &m)
		case c == '\'' || c == '"':
			i = maskFlatString(src, i, &d, &m)
		case c == '`':
			i = maskTemplate(src, i, &d, &m)
		default:
			d.WriteByte(c)
			m.WriteByte(c)
			i++
		}
	}
	return d.String(), m.String()
}

// maskLineComment drops a `//` comment from src[i] to end of line, writing
// nothing to either builder — the terminating '\n' is copied by the caller's
// next iteration. Returns the index at the newline (or EOF).
func maskLineComment(src string, i int) int {
	n := len(src)
	for i < n && src[i] != '\n' {
		i++
	}
	return i
}

// maskBlockComment consumes a `/* … */` comment from src[i], emitting one space
// (so tokens around it don't fuse) and mirroring interior newlines so line
// offsets stay aligned. Returns the index just past the closing `*/`.
func maskBlockComment(src string, i int, d, m *strings.Builder) int {
	n := len(src)
	d.WriteByte(' ')
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
	return i
}

// maskFlatString consumes a single-quote or double-quote string from src[i]
// (its opening delimiter), keeping the display verbatim and neutralising the
// interior in the mask (the opening delimiter is preserved in the mask, every
// other byte including the closer becomes 'x'/newline). A backslash escapes the
// next byte so an escaped quote does not close the string. Returns the index
// just past the closing quote (or EOF for an unterminated string).
func maskFlatString(src string, i int, d, m *strings.Builder) int {
	n := len(src)
	quote := src[i]
	d.WriteByte(quote)
	m.WriteByte(quote)
	i++
	for i < n {
		ch := src[i]
		if ch == '\\' && i+1 < n {
			d.WriteByte(ch)
			m.WriteByte(neutralOrNewline(ch))
			d.WriteByte(src[i+1])
			m.WriteByte(neutralOrNewline(src[i+1]))
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
	return i
}

// maskTemplate consumes a template literal from src[i] (the opening backtick).
// A template literal is NOT a flat single-delimiter span: a `${…}` interpolation
// holds arbitrary code (nested strings, comments, and further template
// literals), and only a backtick reached in string-body mode — never one buried
// inside an interpolation — closes it. String-body bytes and interpolation code
// alike are neutralised in the mask (matching the flat-string treatment); the
// state machine exists solely to locate the correct closing backtick so real
// source after the literal is not swallowed (issue #219). Returns the index
// just past the closing backtick (or EOF for an unterminated literal).
func maskTemplate(src string, i int, d, m *strings.Builder) int {
	n := len(src)
	// Opening backtick: verbatim in display, preserved as delimiter in mask.
	d.WriteByte(src[i])
	m.WriteByte(src[i])
	i++
	for i < n {
		ch := src[i]
		switch {
		case ch == '\\' && i+1 < n:
			// Escape: neutralise this byte and the escaped one. This also
			// disarms `\${`, which is a literal `$` and opens no interpolation.
			d.WriteByte(ch)
			m.WriteByte(neutralOrNewline(ch))
			d.WriteByte(src[i+1])
			m.WriteByte(neutralOrNewline(src[i+1]))
			i += 2
		case ch == '`':
			// Closing delimiter — reached only in string-body mode.
			d.WriteByte(ch)
			m.WriteByte(neutralOrNewline(ch))
			return i + 1
		case ch == '$' && i+1 < n && src[i+1] == '{':
			// Enter interpolation: emit `${`, then scan code at brace depth 1.
			d.WriteByte(ch)
			m.WriteByte(neutralOrNewline(ch))
			d.WriteByte(src[i+1])
			m.WriteByte(neutralOrNewline(src[i+1]))
			i = maskInterp(src, i+2, d, m)
		default:
			d.WriteByte(ch)
			m.WriteByte(neutralOrNewline(ch))
			i++
		}
	}
	return i
}

// maskInterp consumes template-literal interpolation code starting just past
// `${` (brace depth 1) and returns the index just past the matching `}` (depth
// 0). Nested comments, strings, and template literals are consumed with the
// SAME masking helpers so a `}` or backtick living inside one never closes the
// interpolation or the enclosing literal; only a bare `{`/`}` moves the depth.
// All bytes are neutralised in the mask (structural braces are counted from the
// raw source, not the mask). Returns EOF for an unterminated interpolation.
func maskInterp(src string, i int, d, m *strings.Builder) int {
	n := len(src)
	depth := 1
	for i < n {
		ch := src[i]
		switch {
		case ch == '/' && i+1 < n && src[i+1] == '/':
			i = maskLineComment(src, i)
		case ch == '/' && i+1 < n && src[i+1] == '*':
			i = maskBlockComment(src, i, d, m)
		case ch == '\'' || ch == '"':
			i = maskFlatString(src, i, d, m)
		case ch == '`':
			i = maskTemplate(src, i, d, m) // nested literal: recurse, don't close
		case ch == '{':
			depth++
			d.WriteByte(ch)
			m.WriteByte(neutralOrNewline(ch))
			i++
		case ch == '}':
			depth--
			d.WriteByte(ch)
			m.WriteByte(neutralOrNewline(ch))
			i++
			if depth == 0 {
				return i
			}
		default:
			d.WriteByte(ch)
			m.WriteByte(neutralOrNewline(ch))
			i++
		}
	}
	return i
}

// neutralOrNewline maps a string-interior byte to a structurally neutral 'x',
// but keeps a newline so line offsets stay mirrored between display and mask.
func neutralOrNewline(b byte) byte {
	if b == '\n' {
		return '\n'
	}
	return 'x'
}
