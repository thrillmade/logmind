package tree

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite *.golden files from current output")

// makeRepo lays out a small fixture tree under t.TempDir().
func makeRepo(t *testing.T, layout map[string]string) string {
	t.Helper()
	root := t.TempDir()
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

func TestRenderFlat(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"a.txt": "a",
		"b.txt": "b",
		"c.txt": "c",
	})
	got, err := Render(root, IgnoreRules{}, -1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "├── a.txt\n├── b.txt\n└── c.txt") {
		t.Errorf("Render output unexpected:\n%s", got)
	}
}

func TestRenderDirsFirst(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"zfile.txt": "z",
		"adir/x":    "x",
		"bdir/y":    "y",
	})
	got, err := Render(root, IgnoreRules{}, -1)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[1], "adir") {
		t.Errorf("expected adir at line 1, got line: %q\nfull:\n%s", lines[1], got)
	}
	zIdx := -1
	for i, ln := range lines {
		if strings.Contains(ln, "zfile.txt") {
			zIdx = i
			break
		}
	}
	bIdx := -1
	for i, ln := range lines {
		if strings.Contains(ln, "bdir") {
			bIdx = i
			break
		}
	}
	if zIdx <= bIdx {
		t.Errorf("expected zfile.txt after bdir, but got z=%d b=%d:\n%s", zIdx, bIdx, got)
	}
}

func TestRenderIgnoreDefault(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"keep.txt":           "k",
		"__pycache__/foo.py": "p",
		"node_modules/lib":   "n",
	})
	rules := IgnoreRules{{Pattern: "__pycache__"}, {Pattern: "node_modules"}}
	got, err := Render(root, rules, -1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "__pycache__") || strings.Contains(got, "node_modules") {
		t.Errorf("Render included ignored dirs:\n%s", got)
	}
	if !strings.Contains(got, "keep.txt") {
		t.Errorf("Render dropped kept file:\n%s", got)
	}
}

func TestRenderIgnoreGlob(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"keep.txt":  "k",
		"cache.pyc": "p",
		"sub/x.pyc": "p",
		"sub/x.py":  "p",
	})
	rules := IgnoreRules{{Pattern: "*.pyc"}}
	got, err := Render(root, rules, -1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, ".pyc") {
		t.Errorf("Render included *.pyc file:\n%s", got)
	}
	if !strings.Contains(got, "x.py\n") {
		t.Errorf("Render dropped .py file (only .pyc should be ignored):\n%s", got)
	}
}

func TestRenderIgnorePathPattern(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"site/.next/cache/x": "x",
		"site/.next/build/y": "y",
		"site/app/page.js":   "p",
		"other/.next/keep":   "k",
	})
	// Path-shaped ignore: site/.next should match only under site/.
	rules := IgnoreRules{{Pattern: "site/.next"}}
	got, err := Render(root, rules, -1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "cache") || strings.Contains(got, "build") {
		t.Errorf("Render leaked site/.next contents:\n%s", got)
	}
	// other/.next should still appear because site/.next pattern is path-anchored.
	if !strings.Contains(got, "other") {
		t.Errorf("Render dropped other/.next branch:\n%s", got)
	}
}

func TestRenderNegate(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"foo.log":  "f",
		"keep.log": "k",
	})
	rules := IgnoreRules{
		{Pattern: "*.log"},
		{Pattern: "keep.log", Negate: true},
	}
	got, err := Render(root, rules, -1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "foo.log") {
		t.Errorf("Render included foo.log:\n%s", got)
	}
	if !strings.Contains(got, "keep.log") {
		t.Errorf("Render dropped keep.log (negation should re-include):\n%s", got)
	}
}

func TestRenderMaxDepth(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"a/b/c/d.txt": "d",
		"a/b/e.txt":   "e",
		"a/f.txt":     "f",
		"g.txt":       "g",
	})
	// max_depth=1 means only the root's direct children appear.
	got, err := Render(root, IgnoreRules{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "a") || !strings.Contains(got, "g.txt") {
		t.Errorf("Render(maxDepth=1) missing direct children:\n%s", got)
	}
	if strings.Contains(got, "f.txt") || strings.Contains(got, "e.txt") {
		t.Errorf("Render(maxDepth=1) leaked depth-2 entries:\n%s", got)
	}
}

func TestRenderMaxDepthZeroRootOnly(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"a.txt": "a",
		"b/c":   "c",
	})
	got, err := Render(root, IgnoreRules{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "a.txt") || strings.Contains(got, "b") {
		t.Errorf("Render(maxDepth=0) should be root-only:\n%s", got)
	}
}

// TestReadGitignoreRules exercises the gitignore parser's trim logic, and
// that it hands rules back in FILE order.
//
// File order is not bookkeeping — under last-match-wins a negation's
// position relative to its neighbours IS the answer, so the second half of
// this test resolves the parsed rules both ways round and asserts the two
// verdicts differ. Assert the ordering in prose only and the claim survives
// any resolver, including one that ignores order entirely (which is what
// shipped before #303).
func TestReadGitignoreRules(t *testing.T) {
	dir := t.TempDir()
	body := "# comment\n" +
		"\n" +
		"__pycache__/\n" +
		"/foo\n" +
		"*.log\n" +
		"!keep.log\n" +
		"bare\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readGitignoreRules(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []Rule{
		{Pattern: "__pycache__"},
		{Pattern: "foo"},
		{Pattern: "*.log"},
		{Pattern: "keep.log", Negate: true},
		{Pattern: "bare"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rules = %v; want %v", got, want)
	}

	// As parsed: `*.log` then `!keep.log`. The negation is LAST, so
	// keep.log is re-included.
	if IgnoreRules(got).Matches("keep.log", "keep.log") {
		t.Errorf("keep.log is ignored with the negation last; want re-included")
	}
	// Swap those two neighbours and nothing else. Now `*.log` is last, so
	// the same file is ignored. Equal verdicts here would mean position
	// carries no meaning.
	swapped := append([]Rule(nil), got...)
	swapped[2], swapped[3] = swapped[3], swapped[2]
	if !IgnoreRules(swapped).Matches("keep.log", "keep.log") {
		t.Errorf("keep.log is re-included with the negation moved EARLIER than *.log; want ignored — resolution is not positional")
	}
}

// TestFileStructureTemplate pins the template head + tail bytes so
// any drift between Python and Go shows up immediately.
//
// Builds the fixture in a deterministic subdir of t.TempDir() so the
// rendered root line is stable across runs.
func TestFileStructureTemplate(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "myrepo")
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := GenerateFileStructure(root, -1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "# File Structure\n\nThis file is auto-generated") {
		t.Errorf("template head wrong: %q", got[:80])
	}
	if !strings.HasSuffix(got, "for the full tree._\n") {
		t.Errorf("template tail wrong: %q", got[len(got)-80:])
	}
	if !strings.Contains(got, "```\n") {
		t.Errorf("missing fenced code block")
	}
	compareGolden(t, "generate-file-structure.golden", got)
}

// TestRoundTripWrite covers WriteFileStructure's no-op detection.
// The target lives OUTSIDE the rendered tree so the second render
// stays content-equal to the first (rendering the tree after writing
// the target inside would otherwise pick up the new file and change
// the rendered output).
func TestRoundTripWrite(t *testing.T) {
	root := makeRepo(t, map[string]string{
		"a/b.txt": "b",
	})
	target := filepath.Join(t.TempDir(), "out.md")
	changed1, err := WriteFileStructure(target, root, -1)
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	if !changed1 {
		t.Errorf("first write should report changed=true")
	}
	changed2, err := WriteFileStructure(target, root, -1)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if changed2 {
		t.Errorf("second write should report changed=false (idempotent)")
	}
}

func compareGolden(t *testing.T, name, got string) {
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
		t.Fatalf("read golden %s: %v (run `make snapshot` to regenerate)", path, err)
	}
	if got != string(want) {
		t.Errorf("output mismatch for %s:\n=== got ===\n%s\n=== want ===\n%s", name, got, string(want))
	}
}
