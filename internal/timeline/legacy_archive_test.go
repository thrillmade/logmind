// legacy_archive_test.go — the timeline half of "a decision written is a
// decision kept" (SPEC §3.2).
//
// §3.2 stopped rotation, so nothing writes docs/decisions-archive.md any
// more. It did NOT make the decisions already sitting in one stop counting:
// the old `max_recent: 20` default means any long-lived repo that upgrades
// has a populated archive, and dropping it from the timeline would erase that
// history from the derived docs on the very first regeneration.
package timeline

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerate_RendersLegacyArchiveRows pins the RENDERED timeline, not the
// collector's return value: the archived decisions must appear as rows a
// reader can see, alongside the branch-file rows, with the CONTROL in the same
// fixture proving the generator is working.
func TestGenerate_RendersLegacyArchiveRows(t *testing.T) {
	docs := t.TempDir()
	if err := os.MkdirAll(filepath.Join(docs, "decisions-branches"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(docs, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// main is a branch like any other under §3.2 — the CONTROL.
	write("decisions-branches/main.md",
		"## 2026-03-01 10:00 - Adopt the collapsed layout\n\n**Reasoning:** why\n\n---\n")
	// The legacy rotation overflow — the REGRESSION.
	write("decisions-archive.md",
		"# Decision Archive\n\n"+
			"## 2025-01-01 09:00 - Rotate logs with logrotate\n\n**Reasoning:** why\n\n---\n"+
			"## 2025-02-02 09:00 - Cache sessions in Redis\n\n**Reasoning:** why\n\n---\n")
	// The pre-§3.2 main log, still read for the same reason.
	write("decisions.md",
		"## 2025-12-01 10:00 - Pick PostgreSQL for storage\n\n**Reasoning:** why\n\n---\n")

	var stderr bytes.Buffer
	recent, archive, err := Generate(docs, &stderr)
	if err != nil {
		t.Fatalf("Generate: %v\n%s", err, stderr.String())
	}
	rendered := recent + archive

	for _, want := range []string{
		"Rotate logs with logrotate", // archived
		"Cache sessions in Redis",    // archived
		"Pick PostgreSQL for storage",
		"Adopt the collapsed layout", // CONTROL: proves the generator ran
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("timeline is missing %q; rendered:\n%s", want, rendered)
		}
	}

	// CONTROL for the probe itself: a title that is in NO source must not
	// appear, or "Contains" would be proving nothing.
	if strings.Contains(rendered, "Never logged anywhere") {
		t.Error("the absence probe is broken: matched a title no source contains")
	}
}
