// search_test.go — exercises `logmind search` against tmpdir fixtures.
//
// Coverage:
//   - substring match returns a hit with context lines + >>> highlight <<<
//   - LITERAL (not regex) semantics: "cost($)" finds the literal "cost($)";
//     "v1.0" does NOT match "v1x0"; "a|b" matches only the literal "a|b"
//   - highlighting works for a query containing regex-special characters
//   - scope: a term living ONLY in docs/decisions.md IS found while on an
//     unrelated feature branch; a branch-file-only term is also found there
//   - --case-sensitive / --no-archive flag matrix (table-driven)
//   - empty query → error
//   - no matches → friendly message, exit 0
//   - --quiet collapses stdout to exactly one `ok k=v` line
//   - docs/ missing → friendly error + ErrSilent
package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// runSearchCmd runs `logmind search <query> [extraArgs...]` and returns
// combined output.
func runSearchCmd(t *testing.T, query string, extraArgs ...string) string {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(append([]string{"search", query}, extraArgs...))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("search %q %v: %v\n%s", query, extraArgs, err, out.String())
	}
	return out.String()
}

// TestSearch_SubstringMatch_ContextAndHighlight: a basic hit prints the
// found-count header, the file/decision-title/line-number header, context
// lines, and >>> highlight <<< markers around the match.
func TestSearch_SubstringMatch_ContextAndHighlight(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Use PostgreSQL\n"+
				"Body line 1\n"+
				"This mentions PostgreSQL again\n"+
				"Body line 3\n"+
				"Body line 4\n")

		body := runSearchCmd(t, "PostgreSQL")
		mustContain(t, body, "Found 2 matches for: PostgreSQL")
		mustContain(t, body, "docs/decisions.md - 2026-06-01 10:00 - Use PostgreSQL")
		mustContain(t, body, ">>> PostgreSQL <<<")
		mustContain(t, body, `ok search: 2 matches for "PostgreSQL"`)
	})
}

// TestSearch_LiteralSemantics: the query is matched as a LITERAL substring,
// never compiled as a regex. Each sub-case pins one way a regex engine would
// give the wrong answer.
func TestSearch_LiteralSemantics(t *testing.T) {
	cases := []struct {
		name    string
		line    string // the sole body line in the fixture
		query   string
		wantHit bool
	}{
		// A valid regex that as a regex would match ZERO of a plain-text line:
		// `cost($)` compiles fine (an empty group anchored at end) but a regex
		// search of the literal text "cost($)" yields nothing. Literal search
		// must FIND it.
		{name: "regex-special query finds literal", line: "estimated cost($) is high", query: "cost($)", wantHit: true},
		// `.` is the regex any-char wildcard: "v1.0" as a regex matches "v1x0".
		// As a literal it must NOT.
		{name: "dot is literal not wildcard", line: "shipped v1x0 today", query: "v1.0", wantHit: false},
		// Same query DOES match the literal "v1.0".
		{name: "dot matches literal dot", line: "shipped v1.0 today", query: "v1.0", wantHit: true},
		// `|` is regex alternation: "a|b" as a regex matches a line with just
		// "a". As a literal it must match ONLY the literal "a|b".
		{name: "pipe is literal not alternation", line: "just an a here", query: "a|b", wantHit: false},
		{name: "pipe matches literal pipe", line: "the a|b toggle", query: "a|b", wantHit: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTempCwd(t, func(d string) {
				mustMkdir(t, filepath.Join(d, "docs"))
				mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
					"## 2026-06-01 10:00 - Decision\n"+tc.line+"\n")

				body := runSearchCmd(t, tc.query)
				gotHit := strings.Contains(body, "Found ")
				if gotHit != tc.wantHit {
					t.Errorf("query %q over line %q: hit=%t, want %t; body:\n%s", tc.query, tc.line, gotHit, tc.wantHit, body)
				}
				// When we expect a hit, the regex-special query must also
				// highlight correctly (no regex compiled anywhere).
				if tc.wantHit {
					mustContain(t, body, ">>> "+tc.query+" <<<")
				}
			})
		})
	}
}

// TestSearch_Scope_SpansMainAndBranch: search must span docs/decisions.md
// (main's decisions) AND the current branch file even when checked out on an
// unrelated feature branch — a term living ONLY in decisions.md is found, and
// a term living ONLY in the branch file is found.
func TestSearch_Scope_SpansMainAndBranch(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		// A decision on main → docs/decisions.md.
		withFakeTTY(t, false, func() { logOnce(t, "Use PostgreSQL for storage") })

		// Move to an unrelated feature branch and log there.
		checkoutBranch(t, d, "feat/unrelated")
		withFakeTTY(t, false, func() { logOnce(t, "Add rate limiting") })

		// Main's decision IS found from the feature branch.
		body := runSearchCmd(t, "PostgreSQL")
		mustContain(t, body, "Found 1 match for: PostgreSQL")
		mustContain(t, body, "docs/decisions.md")

		// The branch's own decision is ALSO found. It appears twice on disk —
		// once in the §1.6.3 branch-summary marker line, once in the "## ..."
		// header — so 2 matches.
		body = runSearchCmd(t, "rate limiting")
		mustContain(t, body, "Found 2 matches for: rate limiting")
		mustContain(t, body, "docs/decisions-branches/feat__unrelated.md")
	})
}

// TestSearch_FlagMatrix: --case-sensitive and --no-archive, table-driven
// across the combinations that matter.
func TestSearch_FlagMatrix(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		extraArgs []string
		wantHit   bool
	}{
		{name: "case-insensitive default matches mixed case", query: "postgresql", wantHit: true},
		{name: "case-sensitive rejects mismatched case", query: "postgresql", extraArgs: []string{"--case-sensitive"}, wantHit: false},
		{name: "case-sensitive accepts exact case", query: "PostgreSQL", extraArgs: []string{"--case-sensitive"}, wantHit: true},
		{name: "archive included by default", query: "archived-term", wantHit: true},
		{name: "no-archive excludes the archive", query: "archived-term", extraArgs: []string{"--no-archive"}, wantHit: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTempCwd(t, func(d string) {
				mustMkdir(t, filepath.Join(d, "docs"))
				mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
					"## 2026-06-01 10:00 - PostgreSQL choice\n")
				mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
					"## 2025-01-01 09:00 - Old decision mentioning archived-term\n")

				body := runSearchCmd(t, tc.query, tc.extraArgs...)
				gotHit := strings.Contains(body, "Found ")
				if gotHit != tc.wantHit {
					t.Errorf("query %q args %v: hit=%t, want %t; body:\n%s", tc.query, tc.extraArgs, gotHit, tc.wantHit, body)
				}
			})
		})
	}
}

// TestSearch_CaseInsensitiveHighlightPreservesOriginalCasing: a lowercase
// query highlights the mixed-case text that actually matched, not the query.
func TestSearch_CaseInsensitiveHighlightPreservesOriginalCasing(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Use PostgreSQL here\n")

		body := runSearchCmd(t, "postgresql")
		mustContain(t, body, ">>> PostgreSQL <<<")
	})
}

// TestSearch_EmptyQueryErrors: an empty query must error, not match every line.
func TestSearch_EmptyQueryErrors(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Something\n")

		root := NewRootCmd()
		root.SetArgs([]string{"search", ""})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err == nil {
			t.Fatalf("expected an error for an empty query; output:\n%s", out.String())
		}
		mustContain(t, out.String(), "empty search query")
	})
}

// TestSearch_NoMatches: returns 0 + "No matches found for: <q>", exit 0.
func TestSearch_NoMatches(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - About cats\n")

		body := runSearchCmd(t, "dragons")
		mustContain(t, body, "No matches found for: dragons")
		mustContain(t, body, `ok search: 0 matches for "dragons"`)
	})
}

// TestSearch_Quiet_EmitsOneOkLine: --quiet collapses stdout to exactly one
// `ok k=v` line — no result bodies, no "Found N matches" chatter.
func TestSearch_Quiet_EmitsOneOkLine(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Use PostgreSQL\n")

		body := runSearchCmd(t, "PostgreSQL", "--quiet")
		lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
		if len(lines) != 1 {
			t.Fatalf("quiet search: want exactly 1 line, got %d:\n%s", len(lines), body)
		}
		if !strings.HasPrefix(lines[0], "ok search ") {
			t.Errorf("quiet search line = %q; want prefix %q", lines[0], "ok search ")
		}
		mustContain(t, body, "matches=1")
	})
}

// TestSearch_DocsMissingErrors: no docs/ → ErrSilent.
func TestSearch_DocsMissingErrors(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"search", "term"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		err := root.Execute()
		if err == nil {
			t.Fatalf("expected ErrSilent when docs/ missing")
		}
		mustContain(t, out.String(), "docs/ directory not found")
	})
}
