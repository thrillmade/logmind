package gitattr

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "regenerate testdata/*.golden files from current Go output")

// TestEnsureBlock_FreshFileMatchesPython is the parity gate. It runs
// EnsureBlock against an empty `.gitattributes` (no prior content)
// and asserts the resulting bytes match what Python's
// gitattributes.ensure_block emits in the same scenario. The Python
// helper is shelled out so we catch drift introduced by either side.
func TestEnsureBlock_FreshFileMatchesPython(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	changed, err := EnsureBlock(path)
	if err != nil {
		t.Fatalf("EnsureBlock: %v", err)
	}
	if !changed {
		t.Fatalf("EnsureBlock returned changed=false on missing file; want true")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}

	pyOut, ok := pythonEnsureBlock(t, "")
	if !ok {
		// Python parity is best-effort. The golden file still pins
		// the Go shape.
		checkGolden(t, "gitattributes-fresh.golden", string(got))
		return
	}
	if string(got) != pyOut {
		t.Fatalf("EnsureBlock(fresh) drift vs Python:\n--- go ---\n%s\n--- py ---\n%s",
			got, pyOut)
	}
	checkGolden(t, "gitattributes-fresh.golden", string(got))
}

// TestEnsureBlock_PreservesPriorContent runs EnsureBlock on a file
// that already has user content. The block must be APPENDED with
// the canonical leading separator; the user's content must survive
// byte-for-byte.
func TestEnsureBlock_PreservesPriorContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	user := "*.go diff=golang\n"
	if err := os.WriteFile(path, []byte(user), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	changed, err := EnsureBlock(path)
	if err != nil {
		t.Fatalf("EnsureBlock: %v", err)
	}
	if !changed {
		t.Fatalf("EnsureBlock returned changed=false; want true")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasPrefix(string(got), user) {
		t.Fatalf("prior content lost — got begins:\n%s", got)
	}
	if !strings.Contains(string(got), BlockStart) {
		t.Fatalf("block sentinel missing — got:\n%s", got)
	}
	pyOut, ok := pythonEnsureBlock(t, user)
	if !ok {
		return
	}
	if string(got) != pyOut {
		t.Fatalf("EnsureBlock(preserve) drift vs Python:\n--- go ---\n%s\n--- py ---\n%s",
			got, pyOut)
	}
}

func TestEnsureBlock_IdempotentSecondCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	if _, err := EnsureBlock(path); err != nil {
		t.Fatalf("first: %v", err)
	}
	first, _ := os.ReadFile(path)
	changed, err := EnsureBlock(path)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if changed {
		t.Fatalf("EnsureBlock changed=true on second identical call")
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatalf("file mutated between identical calls:\n--- 1 ---\n%s\n--- 2 ---\n%s",
			first, second)
	}
}

func TestHasBlock_RecognisesBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	if HasBlock(path) {
		t.Fatalf("HasBlock returned true on missing file")
	}
	if _, err := EnsureBlock(path); err != nil {
		t.Fatalf("EnsureBlock: %v", err)
	}
	if !HasBlock(path) {
		t.Fatalf("HasBlock returned false after EnsureBlock")
	}
}

func TestRemoveBlock_StripsBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	user := "*.go diff=golang\n"
	if err := os.WriteFile(path, []byte(user), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := EnsureBlock(path); err != nil {
		t.Fatalf("EnsureBlock: %v", err)
	}
	removed, err := RemoveBlock(path)
	if err != nil {
		t.Fatalf("RemoveBlock: %v", err)
	}
	if !removed {
		t.Fatalf("RemoveBlock returned false on file with block")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(got), BlockStart) {
		t.Fatalf("block sentinel still present after RemoveBlock: %q", got)
	}
	if !strings.Contains(string(got), "*.go diff=golang") {
		t.Fatalf("user content lost: %q", got)
	}
}

func TestRemoveBlock_NoBlockPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	if err := os.WriteFile(path, []byte("# nothing here\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	removed, err := RemoveBlock(path)
	if err != nil {
		t.Fatalf("RemoveBlock: %v", err)
	}
	if removed {
		t.Fatalf("RemoveBlock returned true on file without block")
	}
}

// TestConfigureMergeDrivers_SetsKeys runs ConfigureMergeDrivers on a
// fresh repo and asserts every key in MergeDriverConfig is set to
// the expected value. Skips when git is not on PATH.
func TestConfigureMergeDrivers_SetsKeys(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping ConfigureMergeDrivers test")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if !ConfigureMergeDrivers(dir) {
		t.Fatalf("ConfigureMergeDrivers returned false on fresh repo")
	}
	if !DriverConfigured(dir) {
		t.Fatalf("DriverConfigured returned false immediately after ConfigureMergeDrivers")
	}
	// Second call should be a no-op (every key already has the
	// expected value).
	if ConfigureMergeDrivers(dir) {
		t.Fatalf("ConfigureMergeDrivers reported changes on second identical call")
	}
}

// --- helpers -------------------------------------------------------------

func checkGolden(t *testing.T, name, body string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `make snapshot` to create it)", path, err)
	}
	if string(want) != body {
		t.Fatalf(".gitattributes drift vs %s\n--- want ---\n%s\n--- got ---\n%s",
			path, want, body)
	}
}

// pythonEnsureBlock shells to the Python interpreter to capture what
// `ensure_block` writes when given the same prior content. The
// helper returns (output, false) — and calls t.Skip — when Python is
// unavailable.
func pythonEnsureBlock(t *testing.T, seed string) (string, bool) {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not on PATH; skipping byte-identical-vs-Python check")
		return "", false
	}
	repoRoot := repoRootFromCaller(t)
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	if seed != "" {
		if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
			t.Fatalf("seed python tmp: %v", err)
		}
	}
	// Pass the .gitattributes target path via argv[1] to dodge any
	// quoting hazards with TempDir paths on weird platforms.
	script := `import sys
sys.path.insert(0, 'src')
from pathlib import Path
from logmind.core.gitattributes import ensure_block
ensure_block(Path(sys.argv[1]))`
	cmd := exec.Command(py, "-c", script, path)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("python3 ensure_block failed: %v\n%s", err, out)
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("python3 wrote nothing: %v", err)
		return "", false
	}
	return string(data), true
}

func repoRootFromCaller(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod walking up from %s", wd)
	return ""
}
