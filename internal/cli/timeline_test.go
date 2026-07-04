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
func makeDocs(t *testing.T, decisionsBody, archiveBody string, branchFiles map[string]string) string {
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
	if archiveBody != "" {
		if err := os.WriteFile(filepath.Join(docs, "decisions-archive.md"), []byte(archiveBody), 0o644); err != nil {
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
	err := runTimeline(cwd, "", false, false, false, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("err = %v; want ErrSilent", err)
	}
	want := "Error: docs/ directory not found. Run 'logmind init' first.\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q; want %q", stdout.String(), want)
	}
}

// TestTimelineStdoutBrief renders to stdout in brief mode.
func TestTimelineStdoutBrief(t *testing.T) {
	cwd := makeDocs(t,
		"## 2026-06-04 14:00 - Newest\n"+
			"## 2026-06-03 10:00 - Mid 1\n"+
			"## 2026-06-02 09:00 - Mid 2\n"+
			"## 2026-06-01 08:00 - Oldest\n",
		"", nil,
	)
	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, "", false, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	compareCLI(t, "timeline_stdout_brief.golden", stdout.String())
}

// TestTimelineStdoutFull renders all entries.
func TestTimelineStdoutFull(t *testing.T) {
	cwd := makeDocs(t,
		"## 2026-06-04 14:00 - Newest\n"+
			"## 2026-06-03 10:00 - Mid 1\n"+
			"## 2026-06-02 09:00 - Mid 2\n"+
			"## 2026-06-01 08:00 - Oldest\n",
		"", nil,
	)
	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, "", false, true, false, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	compareCLI(t, "timeline_stdout_full.golden", stdout.String())
}

// TestTimelineWriteFresh: writes the file and reports "Regenerated".
func TestTimelineWriteFresh(t *testing.T) {
	cwd := makeDocs(t,
		"## 2026-06-04 14:00 - Newest\n"+
			"## 2026-06-01 08:00 - Oldest\n",
		"", nil,
	)
	target := filepath.Join(cwd, "docs", "timeline.md")
	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, target, false, false, false, &stdout, &stderr); err != nil {
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
	cwd := makeDocs(t, "## 2026-06-01 10:00 - One\n", "", nil)
	target := filepath.Join(cwd, "docs", "timeline.md")
	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, target, false, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("first run: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := runTimeline(cwd, target, false, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("already up to date")) {
		t.Errorf("idempotent run missing 'already up to date': %q", stdout.String())
	}
}

// TestTimelineCheckClean: docs/timeline.md is in sync → exit 0.
func TestTimelineCheckClean(t *testing.T) {
	cwd := makeDocs(t, "## 2026-06-01 10:00 - One\n", "", nil)
	target := filepath.Join(cwd, "docs", "timeline.md")
	// Seed the file by writing it first.
	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, target, false, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := runTimeline(cwd, target, true, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("check err = %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("is up to date")) {
		t.Errorf("check stdout = %q", stdout.String())
	}
}

// TestTimelineCheckStale: docs/timeline.md is out of date → exit 1.
func TestTimelineCheckStale(t *testing.T) {
	cwd := makeDocs(t, "## 2026-06-01 10:00 - One\n", "", nil)
	target := filepath.Join(cwd, "docs", "timeline.md")
	// Write a placeholder that doesn't match the rendered output.
	if err := os.WriteFile(target, []byte("# stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := runTimeline(cwd, target, true, false, false, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Errorf("stale check err = %v; want ErrSilent", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("is stale")) {
		t.Errorf("stale check stdout = %q", stdout.String())
	}
}

// TestTimelineCheckRequiresWrite: --check without --write errors.
func TestTimelineCheckRequiresWrite(t *testing.T) {
	cwd := makeDocs(t, "## 2026-06-01 10:00 - One\n", "", nil)
	var stdout, stderr bytes.Buffer
	err := runTimeline(cwd, "", true, false, false, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Errorf("err = %v; want ErrSilent", err)
	}
	want := "Error: --check requires --write PATH to compare against.\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q; want %q", stdout.String(), want)
	}
}

// TestTimelineMainCanonicalDispatch proves the config gate flips the emitted
// format AND that --write/--check use the SAME generator (no false-stale
// wedge). The existing golden tests above are the default-mode byte-parity
// guard; this is the opt-in path.
func TestTimelineMainCanonicalDispatch(t *testing.T) {
	cwd := makeDocs(t, "", "", map[string]string{
		"feat__x.md": "<!-- logmind-entry-start: 2026-06-29-x -->\n- row\n<!-- logmind-entry-end -->\n",
	})
	if err := os.MkdirAll(filepath.Join(cwd, ".logmind"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".logmind", "config.yml"),
		[]byte("timeline:\n  canonical: main-canonical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cwd, "docs", "timeline.md")

	var stdout, stderr bytes.Buffer
	if err := runTimeline(cwd, target, false, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("write: %v (%s)", err, stderr.String())
	}
	got, _ := os.ReadFile(target)
	if !timeline.HasEntryBlocks(string(got)) {
		t.Errorf("main-canonical mode did not emit entry-block format:\n%s", got)
	}

	stdout.Reset()
	stderr.Reset()
	if err := runTimeline(cwd, target, true, false, false, &stdout, &stderr); err != nil {
		t.Errorf("--check after a main-canonical write reported stale: %v\n%s", err, stdout.String())
	}
}

// compareCLI is the shared cli-package snapshot helper. testdata
// directory is internal/cli/testdata, shared with the existing
// install_hook + check_decisions goldens. Reuses version_test.go's
// `update` flag so a single `make snapshot` regenerates everything.
func compareCLI(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if got != string(want) {
		t.Errorf("output mismatch for %s:\n=== got ===\n%s\n=== want ===\n%s", name, got, string(want))
	}
}
