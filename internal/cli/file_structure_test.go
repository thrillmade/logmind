package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/tree"
)

// makeRepoForTree lays out a deterministic subdir of t.TempDir() so
// the rendered root name is stable across runs (otherwise the
// t.TempDir suffix counter would leak into the golden).
func makeRepoForTree(t *testing.T, layout map[string]string) string {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "myrepo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range layout {
		full := filepath.Join(root, rel)
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFileStructureStdoutDefault(t *testing.T) {
	cwd := makeRepoForTree(t, map[string]string{
		"a/b.txt":   "b",
		"c.txt":     "c",
		"sub/d.txt": "d",
	})
	var stdout, stderr bytes.Buffer
	if err := runFileStructure(cwd, "", false, tree.DefaultFileStructureDepth, false, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	out := stdout.String()
	// Must contain the file-structure template head + tail + ok line.
	if !strings.HasPrefix(out, "# File Structure\n") {
		t.Errorf("output missing template head: %q", out[:60])
	}
	if !strings.Contains(out, "ok file-structure: ") {
		t.Errorf("output missing ok line: %q", out)
	}
	if !strings.Contains(out, "depth=2 (stdout)") {
		t.Errorf("ok line wrong depth label: %q", out)
	}
}

func TestFileStructureStdoutUnbounded(t *testing.T) {
	cwd := makeRepoForTree(t, map[string]string{
		"a/b/c/d.txt": "d",
	})
	var stdout, stderr bytes.Buffer
	// effective = -1 (unbounded)
	if err := runFileStructure(cwd, "", false, -1, false, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout.String(), "unbounded (stdout)") {
		t.Errorf("ok line missing 'unbounded': %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "d.txt") {
		t.Errorf("unbounded tree dropped deep file: %q", stdout.String())
	}
}

func TestFileStructureWriteIdempotent(t *testing.T) {
	cwd := makeRepoForTree(t, map[string]string{
		"a/b.txt": "b",
	})
	// Write target OUTSIDE the rendered tree so the second render
	// stays content-equal to the first.
	target := filepath.Join(t.TempDir(), "out.md")
	var stdout, stderr bytes.Buffer
	if err := runFileStructure(cwd, target, false, -1, false, &stdout, &stderr); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if !strings.Contains(stdout.String(), "✓ Regenerated") {
		t.Errorf("first run missing ✓ Regenerated: %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := runFileStructure(cwd, target, false, -1, false, &stdout, &stderr); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !strings.Contains(stdout.String(), "already up to date") {
		t.Errorf("second run missing 'already up to date': %q", stdout.String())
	}
}

func TestFileStructureCheckStale(t *testing.T) {
	cwd := makeRepoForTree(t, map[string]string{
		"a/b.txt": "b",
	})
	target := filepath.Join(t.TempDir(), "out.md")
	if err := os.WriteFile(target, []byte("# stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := runFileStructure(cwd, target, true, -1, false, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Errorf("stale check err = %v; want ErrSilent", err)
	}
	if !strings.Contains(stdout.String(), "is stale") {
		t.Errorf("stale check stdout = %q", stdout.String())
	}
}

func TestFileStructureCheckRequiresWrite(t *testing.T) {
	cwd := makeRepoForTree(t, nil)
	var stdout, stderr bytes.Buffer
	err := runFileStructure(cwd, "", true, -1, false, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Errorf("err = %v; want ErrSilent", err)
	}
	if stdout.String() != "Error: --check requires --write PATH to compare against.\n" {
		t.Errorf("stdout = %q", stdout.String())
	}
}

// TestTreeDocsMissing: tree subcommand requires docs/ to exist.
func TestTreeDocsMissing(t *testing.T) {
	cwd := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runTree(cwd, -1, false, &stdout, &stderr)
	if !errors.Is(err, ErrSilent) {
		t.Errorf("err = %v; want ErrSilent", err)
	}
	if !strings.Contains(stdout.String(), "docs/ directory not found") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

// TestTreeDefaultLabel: --max-depth omitted prints "default" label.
func TestTreeDefaultLabel(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "myrepo")
	if err := os.MkdirAll(filepath.Join(cwd, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := runTree(cwd, -1, false, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout.String(), "ok docs/file-structure.md (") {
		t.Errorf("ok line missing: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "default)") {
		t.Errorf("default label missing: %q", stdout.String())
	}
}

// TestTreeUnboundedLabel: --max-depth 0 prints "unbounded".
func TestTreeUnboundedLabel(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "myrepo")
	if err := os.MkdirAll(filepath.Join(cwd, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := runTree(cwd, 0, true, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout.String(), "unbounded)") {
		t.Errorf("unbounded label missing: %q", stdout.String())
	}
}

// TestTreeExplicitDepthLabel: --max-depth 5 prints "depth=5".
func TestTreeExplicitDepthLabel(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "myrepo")
	if err := os.MkdirAll(filepath.Join(cwd, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := runTree(cwd, 5, true, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout.String(), "depth=5)") {
		t.Errorf("depth=5 label missing: %q", stdout.String())
	}
}
