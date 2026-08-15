package gitattr

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/testgit"
)

var update = flag.Bool("update", false, "regenerate testdata/*.golden files from current Go output")

// TestEnsureBlock_FreshFileMatchesGolden pins the EXACT bytes EnsureBlock
// writes to a fresh/empty `.gitattributes` — the first-run path every new repo
// hits on `logmind init`.
//
// This replaces the retired Python-parity test that used to be the only caller
// of checkGolden. Dropping that test outright would have left the fresh-file
// format pinned by nothing at all (the other EnsureBlock tests only exercise
// the file-already-has-content and second-call-is-a-no-op paths). The Python
// half is gone — the frozen Python is history, not a contract, and its source
// no longer exists in this repo — but the golden itself still earns its keep:
// the emitted block is a wire format that `logmind doctor`, the merge-driver
// registration, and every consumer repo's `.gitattributes` depend on, so a
// silent reflow of it must trip CI loudly.
func TestEnsureBlock_FreshFileMatchesGolden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	if _, err := EnsureBlock(path); err != nil {
		t.Fatalf("EnsureBlock on a fresh file: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
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
	testgit.InitRepo(t, dir, "-q")
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

// TestAddMissingLines_DoesNotReinstateDeliberatelyRemovedLine pins
// logmind#301 round 5 LOW: addMissingLines used to re-add ANY DefaultLines
// pattern absent from the block, with no way to tell "this repo predates
// the pattern" apart from "the user deleted it on purpose". Reinstating a
// line someone removed on purpose is the same class of bug as overwriting
// a user-owned artifact.
//
// Sequence: seed a block that predates docs/timeline-archive.md (simulating
// an old repo), run the CURRENT EnsureBlock once so it's offered and added
// (an upgrade must still land it), delete it by hand (the user's deliberate
// removal), then run EnsureBlock again — it must NOT come back.
func TestAddMissingLines_DoesNotReinstateDeliberatelyRemovedLine(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping addMissingLines offered-tracking test")
	}
	dir := t.TempDir()
	testgit.InitRepo(t, dir, "-q")
	path := filepath.Join(dir, ".gitattributes")

	oldLines := []string{
		"docs/timeline.md          merge=logmind-timeline",
		"docs/file-structure.md    merge=logmind-file-structure",
	}
	if _, err := ensureBlockWithLines(path, oldLines); err != nil {
		t.Fatalf("seed pre-archive block: %v", err)
	}

	// Upgrade: the CURRENT DefaultLines (adds timeline-archive.md) must
	// still land on a repo that has never seen that pattern.
	changed, err := EnsureBlock(path)
	if err != nil {
		t.Fatalf("EnsureBlock (upgrade): %v", err)
	}
	if !changed {
		t.Fatalf("EnsureBlock did not add the new timeline-archive.md registration on upgrade")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after upgrade: %v", err)
	}
	if !strings.Contains(string(body), "docs/timeline-archive.md") {
		t.Fatalf("timeline-archive.md missing after upgrade:\n%s", body)
	}

	// The user deliberately deletes the line logmind just added.
	const archiveLine = "docs/timeline-archive.md  merge=logmind-timeline-archive\n"
	if !strings.Contains(string(body), archiveLine) {
		t.Fatalf("archive line not in the exact expected form; got:\n%s", body)
	}
	withoutArchive := strings.Replace(string(body), archiveLine, "", 1)
	if err := os.WriteFile(path, []byte(withoutArchive), 0o644); err != nil {
		t.Fatalf("simulate user deletion: %v", err)
	}

	// A later run (another `init`, or `refresh`) must NOT bring it back.
	changed, err = EnsureBlock(path)
	if err != nil {
		t.Fatalf("EnsureBlock (after deletion): %v", err)
	}
	if changed {
		t.Fatalf("EnsureBlock reported changed=true reinstating a deliberately deleted line")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second EnsureBlock: %v", err)
	}
	if strings.Contains(string(after), "docs/timeline-archive.md") {
		t.Fatalf("timeline-archive.md was reinstated after the user deleted it:\n%s", after)
	}
}

// TestAddMissingLines_FailedWriteDoesNotRecordAsOffered pins the HIGH from
// logmind#301 round 6: a write that never happened must not be recorded as
// "offered", or addMissingLines treats the missing pattern as a line the
// user deliberately removed and skips it forever afterwards — even once
// whatever blocked the write is gone.
//
// Two runs against a block that's missing docs/timeline-archive.md:
//
//  1. `.gitattributes` is a symlink, so atomicio.WriteFile refuses the
//     write. EnsureBlock must return an error and the pattern must NOT be
//     recorded as offered.
//  2. The SAME path, now a regular file with the identical missing-line
//     block, must still pick up docs/timeline-archive.md — proving run 1's
//     failure didn't poison the record.
//
// Before this fix, run 1's `defer recordOfferedPatterns(...)` fired
// unconditionally on the error return, so run 2 also reported
// changed=false and never wrote the line — exactly the panel's
// reproduction (docs/timeline-archive.md merge driver never registered
// again).
func TestAddMissingLines_FailedWriteDoesNotRecordAsOffered(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping addMissingLines offered-tracking test")
	}
	dir := t.TempDir()
	testgit.InitRepo(t, dir, "-q")
	path := filepath.Join(dir, ".gitattributes")

	oldLines := []string{
		"docs/timeline.md          merge=logmind-timeline",
		"docs/file-structure.md    merge=logmind-file-structure",
	}
	if _, err := ensureBlockWithLines(path, oldLines); err != nil {
		t.Fatalf("seed pre-archive block: %v", err)
	}
	seeded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seeded block: %v", err)
	}

	// Run 1: retarget .gitattributes to a symlink pointing at a copy of the
	// identical seeded block, so addMissingLines computes the same
	// "missing docs/timeline-archive.md" set but the write is refused.
	outside := filepath.Join(dir, "..", "escaped-gitattributes-offered-tracking")
	if err := os.WriteFile(outside, seeded, 0o644); err != nil {
		t.Fatalf("seed outside: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove seeded .gitattributes: %v", err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	changed, err := EnsureBlock(path)
	if err == nil {
		t.Fatalf("run 1 (symlinked): want a symlink refusal error, got changed=%v err=nil", changed)
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("run 1 error = %q; want it to name the symlink", err)
	}
	if changed {
		t.Fatalf("run 1 (symlinked): want changed=false on a refused write, got true")
	}
	t.Logf("run 1 (symlinked): changed=%v err=%v", changed, err)

	// Run 2: same path, now a regular file with the identical missing-line
	// block. Must still register docs/timeline-archive.md.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove symlink before run 2: %v", err)
	}
	if err := os.WriteFile(path, seeded, 0o644); err != nil {
		t.Fatalf("restore regular file before run 2: %v", err)
	}

	changed, err = EnsureBlock(path)
	t.Logf("run 2 (regular file): changed=%v err=%v", changed, err)
	if err != nil {
		t.Fatalf("run 2 (regular file): unexpected error: %v", err)
	}
	if !changed {
		t.Fatalf("BUG: run 1's refused write permanently suppressed docs/timeline-archive.md — run 2 reported changed=false (\"nothing to do\")")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after run 2: %v", err)
	}
	if !strings.Contains(string(after), "docs/timeline-archive.md") {
		t.Fatalf("BUG: docs/timeline-archive.md merge driver never registered again:\n%s", after)
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
