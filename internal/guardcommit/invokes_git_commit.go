package guardcommit

import "strings"

// wrapperCommands are process-wrapping binaries that invoke another
// command as their final argument. A Bash tool call that runs
// `timeout 30 git commit -m x` is invoking `git commit` just as much as
// a bare `git commit -m x` — InvokesGitCommit sees through these.
var wrapperCommands = map[string]bool{
	"timeout": true,
	"time":    true,
	"nice":    true,
	"nohup":   true,
	"stdbuf":  true,
}

// InvokesGitCommit reports whether bashCommand would, if run by a shell,
// execute `git commit` (exactly — not `git commit-tree`, not any other
// git subcommand) as one of its top-level statements or inside a command
// substitution.
//
// This is intentionally a hand-rolled tokenizer/splitter rather than a
// regexp, matching this project's minimal-dependency posture (see
// internal/gitcli's doc comment) and because shell quoting/substitution
// isn't expressible as a single regex without producing false positives
// on cases like `git commit -m "has && inside quotes"`.
//
// Algorithm:
//
//  1. Split bashCommand on shell statement separators (&&, ||, |&, |, &,
//     ;, newline) while OUTSIDE single/double quotes, producing a list of
//     top-level statements.
//  2. Separately, extract the contents of every $(...) and `...` command
//     substitution anywhere in the string (recursively, so a substitution
//     nested inside another substitution is also found), and run the same
//     split against each one. This is what makes `echo $(git commit -m x)`
//     match even though the top-level statement starts with "echo".
//  3. For each statement (from either source), tokenize it into
//     whitespace-separated, quote-aware words; optionally strip a leading
//     process-wrapper command (timeout/time/nice/nohup/stdbuf) and its own
//     arguments; if the first remaining word isn't literally "git", the
//     statement doesn't match.
//  4. Walk git's global flags (-C x, -c x, --git-dir[=x], --work-tree[=x],
//     any other -*) consuming their values as needed; the first non-flag
//     word is git's subcommand. Match only if it is EXACTLY "commit" (a
//     prefix match would wrongly catch `git commit-tree`).
func InvokesGitCommit(bashCommand string) bool {
	if statementsInvokeCommit(bashCommand) {
		return true
	}
	for _, sub := range extractSubstitutions(bashCommand) {
		if statementsInvokeCommit(sub) {
			return true
		}
	}
	return false
}

// statementsInvokeCommit splits s on top-level shell separators and
// checks each resulting statement independently.
func statementsInvokeCommit(s string) bool {
	for _, statement := range splitStatements(s) {
		if statementInvokesCommit(statement) {
			return true
		}
	}
	return false
}

// statementInvokesCommit checks a single already-split statement (no
// top-level && / ; / | etc. left in it — those were already split away;
// any such tokens remaining are inside quotes and are just data).
func statementInvokesCommit(statement string) bool {
	words := tokenizeWords(statement)
	words = stripWrapperPrefix(words)
	if len(words) == 0 || words[0] != "git" {
		return false
	}
	subcommand, ok := firstGitSubcommand(words[1:])
	return ok && subcommand == "commit"
}

// stripWrapperPrefix removes a leading chain of process-wrapper commands
// (and their own flags/positional args) so the wrapped command underneath
// can be inspected. Repeats so `nohup timeout 30 git commit` (a wrapper
// wrapping a wrapper) is also handled.
//
// Only `timeout` has a REQUIRED bare positional (its duration) ahead of
// the wrapped command; the others (`time`, `nice`, `nohup`, `stdbuf`) take
// only flag-style options (if any) before the wrapped command begins, so
// we special-case the single-positional skip to `timeout` alone.
func stripWrapperPrefix(words []string) []string {
	for len(words) > 0 && wrapperCommands[words[0]] {
		isTimeout := words[0] == "timeout"
		words = words[1:]
		for len(words) > 0 && strings.HasPrefix(words[0], "-") {
			words = words[1:]
		}
		if isTimeout && len(words) > 0 {
			words = words[1:] // the duration positional
		}
	}
	return words
}

// firstGitSubcommand walks git's global options (which appear BEFORE the
// subcommand) and returns the first non-flag word — git's subcommand.
// words is everything after the literal "git" token.
func firstGitSubcommand(words []string) (string, bool) {
	i := 0
	for i < len(words) {
		w := words[i]
		switch {
		case w == "-C" || w == "-c":
			// Both take a mandatory, space-separated value.
			i += 2
		case w == "--git-dir" || w == "--work-tree":
			i += 2
		case strings.HasPrefix(w, "--git-dir=") || strings.HasPrefix(w, "--work-tree="):
			i++
		case strings.HasPrefix(w, "-"):
			// Any other global flag (--no-pager, --bare, -p, ...):
			// treated as a bare flag with no consumed value.
			i++
		default:
			return w, true
		}
	}
	return "", false
}

// splitStatements splits s on shell statement separators — && || |& | & ;
// and newline — while outside single/double quotes. Longer separators
// (&&, ||, |&) are matched before their single-character prefixes (&, |)
// so e.g. "&&" isn't mistaken for two consecutive "&" separators.
func splitStatements(s string) []string {
	var statements []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	n := len(s)
	flush := func() {
		statements = append(statements, cur.String())
		cur.Reset()
	}
	for i := 0; i < n; {
		c := s[i]
		switch {
		case inSingle:
			cur.WriteByte(c)
			if c == '\'' {
				inSingle = false
			}
			i++
		case inDouble:
			cur.WriteByte(c)
			if c == '"' {
				inDouble = false
			}
			i++
		case c == '\'':
			inSingle = true
			cur.WriteByte(c)
			i++
		case c == '"':
			inDouble = true
			cur.WriteByte(c)
			i++
		case c == '\\' && i+1 < n:
			// Bare-context escape: copy both bytes literally so the
			// escaped char (e.g. "\;") doesn't get treated as a
			// separator.
			cur.WriteByte(c)
			cur.WriteByte(s[i+1])
			i += 2
		case c == '&' && i+1 < n && s[i+1] == '&':
			flush()
			i += 2
		case c == '|' && i+1 < n && (s[i+1] == '|' || s[i+1] == '&'):
			flush()
			i += 2
		case c == ';' || c == '|' || c == '&' || c == '\n':
			flush()
			i++
		default:
			cur.WriteByte(c)
			i++
		}
	}
	flush()
	return statements
}

// tokenizeWords splits a single statement into whitespace-separated
// words, stripping (but not otherwise interpreting) single/double quote
// delimiters so a quoted phrase becomes one word. Good enough for our
// purposes — we only ever inspect the first few words (wrapper name, git,
// flags, subcommand); a quoted commit message's exact contents never
// matter to this function.
func tokenizeWords(s string) []string {
	var words []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	hasContent := false
	n := len(s)
	flush := func() {
		if hasContent {
			words = append(words, cur.String())
			cur.Reset()
			hasContent = false
		}
	}
	for i := 0; i < n; {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(c)
			}
			i++
		case inDouble:
			if c == '"' {
				inDouble = false
			} else {
				cur.WriteByte(c)
			}
			i++
		case c == '\'':
			inSingle = true
			hasContent = true
			i++
		case c == '"':
			inDouble = true
			hasContent = true
			i++
		case c == ' ' || c == '\t':
			flush()
			i++
		default:
			cur.WriteByte(c)
			hasContent = true
			i++
		}
	}
	flush()
	return words
}

// extractSubstitutions walks s and returns the inner contents of every
// $(...) and `...` (backtick) command substitution, recursing into nested
// substitutions so they're returned too (flattened, not nested). Content
// inside single quotes is skipped (POSIX: single quotes suppress ALL
// expansion, including command substitution); content inside double
// quotes IS scanned, since $() and backticks still expand there.
func extractSubstitutions(s string) []string {
	var subs []string
	inSingle := false
	n := len(s)
	for i := 0; i < n; {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
			i++
		case c == '\'':
			inSingle = true
			i++
		case c == '$' && i+1 < n && s[i+1] == '(':
			inner, end := scanParenSubstitution(s, i+2)
			subs = append(subs, inner)
			subs = append(subs, extractSubstitutions(inner)...)
			i = end
		case c == '`':
			end := strings.IndexByte(s[i+1:], '`')
			if end < 0 {
				// Unterminated backtick span — nothing more to parse.
				i = n
				continue
			}
			inner := s[i+1 : i+1+end]
			subs = append(subs, inner)
			subs = append(subs, extractSubstitutions(inner)...)
			i = i + 1 + end + 1
		default:
			i++
		}
	}
	return subs
}

// scanParenSubstitution scans a $(...)  body starting at start (the byte
// right after "$("), tracking nested parens and quotes so an inner ")"
// belonging to a nested substitution or a quoted string doesn't
// prematurely close the outer one. Returns the inner text (excluding the
// closing paren) and the index right after the closing paren (or len(s)
// if unterminated).
func scanParenSubstitution(s string, start int) (inner string, end int) {
	depth := 1
	inSingle, inDouble := false, false
	n := len(s)
	j := start
	for j < n && depth > 0 {
		c := s[j]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			if c == '"' {
				inDouble = false
			}
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == '(':
			depth++
		case c == ')':
			depth--
		}
		j++
	}
	closeIdx := j - 1 // index of the ')' that closed depth to 0, or len(s) if unterminated
	if depth > 0 {
		return s[start:n], n
	}
	return s[start:closeIdx], j
}
