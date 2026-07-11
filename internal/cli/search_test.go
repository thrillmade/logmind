// search_test.go — exercises `logmind search` against tmpdir fixtures.
//
// Coverage:
//   - substring match returns a hit with context lines + >>> highlight <<<
//   - "current branch" scoping: a feature branch's search does NOT see
//     docs/decisions.md content, only its own branch file + the archive
//   - --case-sensitive / --no-archive flag matrix (table-driven)
//   - regex query supported; an invalid regex falls back to a literal match
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

// TestSearch_CurrentBranchOnly: search is scoped to the SAME "recent" file
// `show` prints (resolveDecisionsPath's branch-aware target). A term that
// only lives in docs/decisions.md (the default branch's file) must NOT be
// found while on a feature branch that has its own, unrelated decisions
// file — matching the documented "current branch" contract.
func TestSearch_CurrentBranchOnly(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		withFakeTTY(t, false, func() { logOnce(t, "Use PostgreSQL for storage") })

		checkoutBranch(t, d, "feat/unrelated")
		withFakeTTY(t, false, func() { logOnce(t, "Add rate limiting") })

		body := runSearchCmd(t, "PostgreSQL")
		mustContain(t, body, "No matches found for: PostgreSQL")

		// Sanity: the branch's own decision IS found. It appears twice on
		// disk — once in the §1.6.3 branch-summary marker line `logmind log`
		// writes automatically, once in the "## ..." decision header itself.
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
				_ = d

				body := runSearchCmd(t, tc.query, tc.extraArgs...)
				gotHit := strings.Contains(body, "Found ")
				if gotHit != tc.wantHit {
					t.Errorf("query %q args %v: hit=%t, want %t; body:\n%s", tc.query, tc.extraArgs, gotHit, tc.wantHit, body)
				}
			})
		})
	}
}

// TestSearch_RegexQuery: regex patterns work; an invalid regex falls back
// to a literal substring match instead of erroring.
func TestSearch_RegexQuery(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - DB foo\n"+
				"## 2026-06-02 10:00 - DB bar\n")

		// Valid regex: matches both "DB foo" and "DB bar" headers.
		body := runSearchCmd(t, "DB .{3}")
		mustContain(t, body, "Found 2 matches")

		// Invalid regex (unbalanced group) falls back to a literal search
		// for the string "DB (" — no such literal text exists, so 0 hits,
		// and critically: no error/panic.
		body = runSearchCmd(t, "DB (")
		mustContain(t, body, "No matches found for: DB (")
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
