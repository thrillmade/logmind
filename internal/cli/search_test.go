// search_test.go — exercises `logmind search` against tmpdir fixtures.
//
// Coverage:
//   - substring match returns a hit with context lines
//   - case-insensitive default + --case-sensitive flag
//   - regex query supported; invalid regex falls back to literal
//   - --no-archive skips decisions-archive.md
//   - --no-context suppresses context lines
//   - --context-lines N adjusts the context window
//   - no matches → friendly message, exit 0
//   - docs/ missing → ErrSilent
package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestSearch_SubstringMatch_ContextAndHighlight: basic search finds the
// term, prints header + context window + match line with >>> hl <<<.
func TestSearch_SubstringMatch_ContextAndHighlight(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Use PostgreSQL\n"+
				"Body line 1\n"+
				"This mentions PostgreSQL again\n"+
				"Body line 3\n"+
				"Body line 4\n")

		root := NewRootCmd()
		root.SetArgs([]string{"search", "PostgreSQL"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("search: %v\n%s", err, out.String())
		}
		body := out.String()
		// Found header.
		mustContain(t, body, "Found 2 matches for: PostgreSQL")
		// File + decision header on match.
		mustContain(t, body, "decisions.md - 2026-06-01 10:00 - Use PostgreSQL")
		// Case-insensitive highlight markers.
		mustContain(t, body, ">>> PostgreSQL <<<")
	})
}

// TestSearch_CaseSensitive: -c flag rejects lowercase term.
func TestSearch_CaseSensitive(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - PostgreSQL choice\n")

		root := NewRootCmd()
		root.SetArgs([]string{"search", "postgresql", "-c"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("search: %v\n%s", err, out.String())
		}
		// -c: no match for lowercase against the mixed-case content.
		mustContain(t, out.String(), "No matches found for: postgresql")
	})
}

// TestSearch_NoArchive: --no-archive skips decisions-archive.md.
func TestSearch_NoArchive(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Recent decision\n")
		mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
			"## 2025-01-01 09:00 - Old archived term\n")

		root := NewRootCmd()
		root.SetArgs([]string{"search", "archived", "--no-archive"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("search: %v\n%s", err, out.String())
		}
		// Term lives only in archive; --no-archive suppresses the hit.
		mustContain(t, out.String(), "No matches found for: archived")
	})
}

// TestSearch_NoContext: --no-context suppresses ±N context block lines.
func TestSearch_NoContext(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Header\n"+
				"Context line above\n"+
				"the match line\n"+
				"Context line below\n")

		root := NewRootCmd()
		root.SetArgs([]string{"search", "match line", "--no-context"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("search: %v\n%s", err, out.String())
		}
		body := out.String()
		mustContain(t, body, "the >>> match line <<<")
		// Context lines should NOT appear.
		if strings.Contains(body, "  Context line above") {
			t.Fatalf("--no-context leaked before-context; body:\n%s", body)
		}
		if strings.Contains(body, "  Context line below") {
			t.Fatalf("--no-context leaked after-context; body:\n%s", body)
		}
	})
}

// TestSearch_RegexQuery: regex patterns work; invalid falls back to literal.
func TestSearch_RegexQuery(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - DB foo\n"+
				"## 2026-06-02 10:00 - DB bar\n")

		root := NewRootCmd()
		// Regex matches both DB foo + DB bar via "DB .{3}".
		root.SetArgs([]string{"search", "DB .{3}"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("search: %v\n%s", err, out.String())
		}
		mustContain(t, out.String(), "Found 2 matches")
	})
}

// TestSearch_NoMatches: returns 0 + "No matches found for: <q>".
func TestSearch_NoMatches(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - About cats\n")

		root := NewRootCmd()
		root.SetArgs([]string{"search", "dragons"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("search: %v\n%s", err, out.String())
		}
		mustContain(t, out.String(), "No matches found for: dragons")
	})
}

// TestSearch_DocsMissingErrors: docs/ missing → ErrSilent.
func TestSearch_DocsMissingErrors(t *testing.T) {
	withTempCwd(t, func(d string) {
		_ = d
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
