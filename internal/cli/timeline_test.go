package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thrillmade/logmind/internal/timeline"
)

// makeDocs lays out a minimal docs/ tree with a few decision entries.
// Returns the cwd path the runTimeline subroutine should use.
func makeDocs(t *testing.T, decisionsBody string, branchFiles map[string]string) string {
	t.Helper()
	cwd := t.TempDir()
	docs := filepath.Join(cwd, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "decisions-branches"), 0o755); err != nil {
		t.Fatal(err)
	}
	if decisionsBody != "" {
		if err := os.WriteFile(filepath.Join(docs, "decisions.md"), []byte(decisionsBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range branchFiles {
		if err := os.WriteFile(filepath.Join(docs, "decisions-branches", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cwd
}

// TestTimelineNoDocs: docs/ missing → "Error: docs/..." + ErrSilent.
func TestTimelineNoDocs(t *testing.T) {
	cwd := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runTimeline(cwd, "", halfDefault, false, false, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("err = %v; want ErrSilent", err)
	}
	want := "Error: docs/ directory not found. Run 'logmind init' first.\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q; want %q", stdout.String(), want)
	}
}

// TestTimelineStdoutMainCanonical renders to stdout — the sole (main-canonical)
// format. Output is the §1.6.4 entry-block union, not the removed brief/full
// renderer.
func TestTimelineStdoutMainCanonical(t *testing.T) {
	cwd := makeDocs(t,
		"## 2026-06-04 14:00 - Newest\n"+
			"## 2026-06-03 10:00 - Mid 1\n"+
			"## 2026-06-02 09:00 - Mid 2\n"+
			"## 2026-06-01 08:00 - Oldest\n",
		nil,
	)
	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, "", halfDefault, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !timeline.HasEntryBlocks(stdout.String()) {
		t.Errorf("stdout is not entry-block (main-canonical) format:\n%s", stdout.String())
	}
	// All four decisions surface as rows (no brief-mode elision).
	for _, want := range []string{"Newest", "Mid 1", "Mid 2", "Oldest"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Errorf("stdout missing decision %q:\n%s", want, stdout.String())
		}
	}
}

// TestTimelineFullFlagIsNoop: `--full` is accepted but ignored as of v2.0.0
// (the timeline is single-format), so passing it produces identical output.
func TestTimelineFullFlagIsNoop(t *testing.T) {
	cwd := makeDocs(t, "## 2026-06-04 14:00 - Newest\n## 2026-06-01 08:00 - Oldest\n", nil)
	root := NewRootCmd()
	root.SetArgs([]string{"timeline", "--full"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	// Drive it from the real cobra tree so the --full flag is exercised. It
	// must parse without error and emit entry-block output.
	oldWd, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := root.Execute(); err != nil {
		t.Fatalf("timeline --full errored; the flag must be an accepted no-op: %v\n%s", err, out.String())
	}
	if !timeline.HasEntryBlocks(out.String()) {
		t.Errorf("--full changed the format; it must be inert:\n%s", out.String())
	}
}

// TestTimelineWriteFresh: writes the file and reports "Regenerated".
func TestTimelineWriteFresh(t *testing.T) {
	cwd := makeDocs(t,
		"## 2026-06-04 14:00 - Newest\n"+
			"## 2026-06-01 08:00 - Oldest\n",
		nil,
	)
	target := filepath.Join(cwd, "docs", "timeline.md")
	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, target, halfDefault, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("✓ Regenerated")) {
		t.Errorf("stdout missing ✓ Regenerated: %q", stdout.String())
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("target not created: %v", err)
	}
}

// TestTimelineWriteIdempotent: second invocation reports "already up to date".
func TestTimelineWriteIdempotent(t *testing.T) {
	cwd := makeDocs(t, "## 2026-06-01 10:00 - One\n", nil)
	target := filepath.Join(cwd, "docs", "timeline.md")
	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, target, halfDefault, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("first run: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := runTimeline(cwd, target, halfDefault, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("already up to date")) {
		t.Errorf("idempotent run missing 'already up to date': %q", stdout.String())
	}
}

// TestTimelineCheckClean: docs/timeline.md is in sync → exit 0.
func TestTimelineCheckClean(t *testing.T) {
	cwd := makeDocs(t, "## 2026-06-01 10:00 - One\n", nil)
	target := filepath.Join(cwd, "docs", "timeline.md")
	// Seed the file by writing it first.
	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, target, halfDefault, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := runTimeline(cwd, target, halfDefault, true, false, &stdout, &stderr); err != nil {
		t.Fatalf("check err = %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("is up to date")) {
		t.Errorf("check stdout = %q", stdout.String())
	}
}

// TestTimelineCheckStale: docs/timeline.md is out of date → exit 1.
func TestTimelineCheckStale(t *testing.T) {
	cwd := makeDocs(t, "## 2026-06-01 10:00 - One\n", nil)
	target := filepath.Join(cwd, "docs", "timeline.md")
	// Write a placeholder that doesn't match the rendered output.
	if err := os.WriteFile(target, []byte("# stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := runTimeline(cwd, target, halfDefault, true, false, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Errorf("stale check err = %v; want ErrSilent", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("is stale")) {
		t.Errorf("stale check stdout = %q", stdout.String())
	}
}

// TestTimelineCheckRequiresWrite: --check without --write errors.
func TestTimelineCheckRequiresWrite(t *testing.T) {
	cwd := makeDocs(t, "## 2026-06-01 10:00 - One\n", nil)
	var stdout, stderr bytes.Buffer
	err := runTimeline(cwd, "", halfDefault, true, false, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Errorf("err = %v; want ErrSilent", err)
	}
	want := "Error: --check requires --write PATH to compare against.\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q; want %q", stdout.String(), want)
	}
}

// TestTimelineLegacyConfigKeyIgnored proves a repo whose .logmind/config.yml
// still carries the REMOVED `timeline.canonical` key loads + regenerates
// cleanly (the now-unknown key is dropped, not errored), emits the sole
// main-canonical entry-block format, and that --write/--check use the SAME
// generator (no false-stale wedge).
func TestTimelineLegacyConfigKeyIgnored(t *testing.T) {
	cwd := makeDocs(t, "", map[string]string{
		"feat__x.md": "<!-- logmind-entry-start: 2026-06-29-x -->\n- row\n<!-- logmind-entry-end -->\n",
	})
	if err := os.MkdirAll(filepath.Join(cwd, ".logmind"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A stale config carrying the removed key — must be ignored, not fatal.
	if err := os.WriteFile(filepath.Join(cwd, ".logmind", "config.yml"),
		[]byte("timeline:\n  canonical: main-canonical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cwd, "docs", "timeline.md")

	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, target, halfDefault, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("write: %v (%s)", err, stderr.String())
	}
	got, _ := os.ReadFile(target)
	if !timeline.HasEntryBlocks(string(got)) {
		t.Errorf("timeline did not emit entry-block format:\n%s", got)
	}

	stdout.Reset()
	stderr.Reset()
	if err := runTimeline(cwd, target, halfDefault, true, false, &stdout, &stderr); err != nil {
		t.Errorf("--check after a write reported stale: %v\n%s", err, stdout.String())
	}
}
