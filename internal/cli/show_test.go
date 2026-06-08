// show_test.go — exercises `logmind show` against tmpdir fixtures.
//
// Coverage:
//   - default verbatim view streams decisions.md and prints ok-line
//   - --all appends decisions-archive.md under ARCHIVED DECISIONS banner
//   - --brief renders one-line-per-entry
//   - --limit caps to N most-recent
//   - --json emits stable JSON shape and routes ok-line to stderr
//   - empty (no decisions.md) → friendly message + 0-count ok-line
//   - docs/ missing → friendly error + ErrSilent
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShow_DefaultVerbatimStreamsDecisions: no flags → stream
// decisions.md + ok-trailer.
func TestShow_DefaultVerbatimStreamsDecisions(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"# Decisions\n\n## 2026-06-01 10:00 - First decision\nBody.\n")

		root := NewRootCmd()
		root.SetArgs([]string{"show"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("show: %v\n%s", err, out.String())
		}
		body := out.String()
		mustContain(t, body, "## 2026-06-01 10:00 - First decision")
		mustContain(t, body, "ok show: docs/decisions.md")
		mustContain(t, body, "bytes")
	})
}

// TestShow_AllAppendsArchive: --all → decisions.md + ARCHIVED banner +
// decisions-archive.md.
func TestShow_AllAppendsArchive(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Main\n")
		mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
			"## 2025-01-01 09:00 - Archived\n")

		root := NewRootCmd()
		root.SetArgs([]string{"show", "--all"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("show --all: %v\n%s", err, out.String())
		}
		body := out.String()
		mustContain(t, body, "## 2026-06-01 10:00 - Main")
		mustContain(t, body, "ARCHIVED DECISIONS")
		mustContain(t, body, "## 2025-01-01 09:00 - Archived")
		mustContain(t, body, "+ archive")
	})
}

// TestShow_BriefRendersOneLinePerEntry: --brief → "<date> — <title> [main]".
func TestShow_BriefRendersOneLinePerEntry(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Alpha\nbody\n## 2026-06-02 11:00 - Beta\n")

		root := NewRootCmd()
		root.SetArgs([]string{"show", "--brief"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("show --brief: %v\n%s", err, out.String())
		}
		body := out.String()
		mustContain(t, body, "2026-06-02 11:00 — Beta [main]")
		mustContain(t, body, "2026-06-01 10:00 — Alpha [main]")
		// Verbatim body should NOT appear in brief mode.
		if strings.Contains(body, "\nbody\n") {
			t.Fatalf("brief mode leaked verbatim body:\n%s", body)
		}
	})
}

// TestShow_LimitCapsResults: --limit N keeps newest N entries.
func TestShow_LimitCapsResults(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - A\n## 2026-06-02 10:00 - B\n## 2026-06-03 10:00 - C\n")

		root := NewRootCmd()
		root.SetArgs([]string{"show", "--limit", "2"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("show --limit: %v\n%s", err, out.String())
		}
		body := out.String()
		// Newest two are C + B.
		mustContain(t, body, "2026-06-03 10:00 — C [main]")
		mustContain(t, body, "2026-06-02 10:00 — B [main]")
		if strings.Contains(body, "— A [main]") {
			t.Fatalf("--limit 2 included entry A; body:\n%s", body)
		}
	})
}

// TestShow_JSONEmitsStableShape: --json → JSON array on stdout + ok
// line on stderr.
func TestShow_JSONEmitsStableShape(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Alpha\n## 2026-06-02 11:00 - Beta\n")

		root := NewRootCmd()
		root.SetArgs([]string{"show", "--json"})
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		if err := root.Execute(); err != nil {
			t.Fatalf("show --json: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
		}
		// Parse stdout as JSON — must round-trip cleanly.
		var entries []map[string]string
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &entries); err != nil {
			t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries; got %d: %v", len(entries), entries)
		}
		// Newest-first.
		if entries[0]["title"] != "Beta" {
			t.Fatalf("entry[0].title = %q; want Beta", entries[0]["title"])
		}
		// ok-line on stderr (JSON mode), not stdout.
		if !strings.Contains(stderr.String(), "ok show: 2 decisions (json)") {
			t.Fatalf("stderr missing ok-line; stderr:\n%s", stderr.String())
		}
		if strings.Contains(stdout.String(), "ok show:") {
			t.Fatalf("ok-line should not be on stdout in JSON mode; stdout:\n%s", stdout.String())
		}
	})
}

// TestShow_NoDecisionsYet: decisions.md missing → "No decisions logged yet."
func TestShow_NoDecisionsYet(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		root := NewRootCmd()
		root.SetArgs([]string{"show"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("show on empty: %v\n%s", err, out.String())
		}
		mustContain(t, out.String(), "No decisions logged yet.")
		mustContain(t, out.String(), "ok show: 0 decisions")
	})
}

// TestShow_DocsMissingErrors: no docs/ → ErrSilent.
func TestShow_DocsMissingErrors(t *testing.T) {
	withTempCwd(t, func(d string) {
		_ = d
		root := NewRootCmd()
		root.SetArgs([]string{"show"})
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

// --- small helpers --------------------------------------------------------

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
