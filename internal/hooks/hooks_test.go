package hooks

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// update mirrors the snapshot pattern from internal/cli — `make
// snapshot` passes it to regenerate testdata/ from the current Go
// output. The hook bodies are byte-identical against Python v0.6.14
// modulo the version-marker line; the golden files capture the Go
// shape (with `1.0.0-dev`) and the parity test below regenerates
// the Python shape on the fly and normalises the marker line.
var update = flag.Bool("update", false, "regenerate testdata/*.golden files from current Go output")

// TestPostMergeBody_MatchesGolden pins the Go-rendered post-merge
// hook body to the canonical bytes captured in testdata. Any drift
// in the embedded shell script breaks this assertion — the goldens
// are CHECKED IN so a refactor that re-orders a comment or trims a
// trailing newline trips CI loudly.
func TestPostMergeBody_MatchesGolden(t *testing.T) {
	checkGolden(t, "post-merge.golden", BuildPostMergeBody())
}

// TestPostRewriteBody_MatchesGolden — same role for the post-rewrite
// hook.
func TestPostRewriteBody_MatchesGolden(t *testing.T) {
	checkGolden(t, "post-rewrite.golden", BuildPostRewriteBody())
}

// TestPostMergeBody_RollupInvariants pins the Slice 2 roll-up contract as
// INTENT (distinct from the byte-golden): the post-merge hook MUST regenerate
// the timeline + file-structure (so a main-canonical repo rebuilds its §1.6.4
// union on every local merge — the regen command dispatches on config) and
// MUST NOT push to a branch (no push-to-default → no GITHUB_TOKEN stranding /
// self-trigger loop). A golden regen that silently violated either still
// trips THIS test. (The "leave regens unstaged" v0.6.7 behavior is pinned by
// the golden + the in-body comment, which itself names `git add`, so we don't
// substring-match that here.)
func TestPostMergeBody_RollupInvariants(t *testing.T) {
	body := BuildPostMergeBody()
	for _, must := range []string{
		"logmind timeline --write docs/timeline.md",
		"logmind file-structure --write docs/file-structure.md",
	} {
		if !strings.Contains(body, must) {
			t.Errorf("post-merge body missing roll-up regen %q", must)
		}
	}
	// Match an actual push invocation, not a stray mention: a `git push` at a
	// shell-command position (line start, after indentation). The body has
	// none — the roll-up never pushes.
	if regexp.MustCompile(`(?m)^\s*git push`).MatchString(body) {
		t.Errorf("post-merge body issues `git push` — the roll-up must NEVER push to a branch")
	}
}

// TestPostMergeBody_ByteIdenticalToPython is THE parity contract for
// wave B2. It shells to the Python interpreter (skipping the test if
// Python or the src/logmind package isn't importable), captures
// _build_post_merge_hook_body() output verbatim, normalises the
// version-marker line so the Go (1.0.0-dev) and Python (0.6.14)
// markers are equivalent, and asserts byte-equality.
//
// Skip semantics: if `python3` isn't on PATH, or `import logmind`
// fails (e.g., in a CI container that strips the venv between
// stages), the test is SKIPPED, not failed. The golden-based test
// above still pins the Go shape; the parity test is the secondary
// check that catches drift introduced when the Python helper itself
// changes.
func TestPostMergeBody_ByteIdenticalToPython(t *testing.T) {
	pyBody, ok := pythonHookBody(t, "_build_post_merge_hook_body")
	if !ok {
		return // skipped
	}
	goBody := BuildPostMergeBody()
	assertParity(t, "post-merge", goBody, pyBody)
}

// TestPostRewriteBody_ByteIdenticalToPython — same parity contract
// for post-rewrite.
func TestPostRewriteBody_ByteIdenticalToPython(t *testing.T) {
	pyBody, ok := pythonHookBody(t, "_build_post_rewrite_hook_body")
	if !ok {
		return
	}
	goBody := BuildPostRewriteBody()
	assertParity(t, "post-rewrite", goBody, pyBody)
}

func TestInstallPostMerge_FreshInstall(t *testing.T) {
	repo := tempRepoWithHooks(t)
	changed, err := InstallPostMerge(repo)
	if err != nil {
		t.Fatalf("InstallPostMerge: %v", err)
	}
	if !changed {
		t.Fatalf("InstallPostMerge returned changed=false on a fresh repo; want true")
	}
	body, err := os.ReadFile(filepath.Join(repo, ".git", "hooks", "post-merge"))
	if err != nil {
		t.Fatalf("read post-merge: %v", err)
	}
	if string(body) != BuildPostMergeBody() {
		t.Fatalf("installed body drifts from BuildPostMergeBody()")
	}
}

func TestInstallPostMerge_Idempotent(t *testing.T) {
	repo := tempRepoWithHooks(t)
	if _, err := InstallPostMerge(repo); err != nil {
		t.Fatalf("first install: %v", err)
	}
	changed, err := InstallPostMerge(repo)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if changed {
		t.Fatalf("InstallPostMerge changed=true on identical second install; want false")
	}
}

func TestInstallPostMerge_LeavesForeignHook(t *testing.T) {
	repo := tempRepoWithHooks(t)
	hookPath := filepath.Join(repo, ".git", "hooks", "post-merge")
	custom := "#!/bin/sh\n# user's custom hook\necho hi\n"
	if err := os.WriteFile(hookPath, []byte(custom), 0o755); err != nil {
		t.Fatalf("seed custom hook: %v", err)
	}
	changed, err := InstallPostMerge(repo)
	if err != nil {
		t.Fatalf("InstallPostMerge: %v", err)
	}
	if changed {
		t.Fatalf("InstallPostMerge changed=true on foreign hook; want false (leave alone)")
	}
	got, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("re-read hook: %v", err)
	}
	if string(got) != custom {
		t.Fatalf("foreign hook was modified:\n%s", got)
	}
}

func TestInstallPostMerge_MissingHooksDir(t *testing.T) {
	dir := t.TempDir()
	// No .git/hooks here — installer should return (false, nil), not
	// error. Matches the Python install_post_merge_hook line 229-230.
	changed, err := InstallPostMerge(dir)
	if err != nil {
		t.Fatalf("InstallPostMerge: %v", err)
	}
	if changed {
		t.Fatalf("changed=true with no .git/hooks/; want false")
	}
}

func TestExtractVersion_FromInstalledHook(t *testing.T) {
	repo := tempRepoWithHooks(t)
	if _, err := InstallPostMerge(repo); err != nil {
		t.Fatalf("install: %v", err)
	}
	v, ok := ExtractVersion(filepath.Join(repo, ".git", "hooks", "post-merge"))
	if !ok {
		t.Fatalf("ExtractVersion returned ok=false on hook we just installed")
	}
	if v == "" {
		t.Fatalf("ExtractVersion returned empty string")
	}
}

func TestExtractVersion_MissingFile(t *testing.T) {
	dir := t.TempDir()
	v, ok := ExtractVersion(filepath.Join(dir, "does-not-exist"))
	if ok || v != "" {
		t.Fatalf("ExtractVersion(missing) = (%q, %v); want ('', false)", v, ok)
	}
}

func TestExtractVersion_PreV0610Hook(t *testing.T) {
	// Pre-v0.6.10 hooks didn't embed the version marker — extractor
	// must return ok=false (NOT a default-empty-string). Mirrors
	// Python gitattributes.extract_hook_version line 273-279.
	dir := t.TempDir()
	old := "#!/bin/sh\n# logmind post-merge hook\necho old\n"
	path := filepath.Join(dir, "post-merge")
	if err := os.WriteFile(path, []byte(old), 0o755); err != nil {
		t.Fatalf("write old hook: %v", err)
	}
	v, ok := ExtractVersion(path)
	if ok || v != "" {
		t.Fatalf("ExtractVersion(pre-v0.6.10) = (%q, %v); want ('', false)", v, ok)
	}
}

// --- helpers -------------------------------------------------------------

// versionMarkerRE strips the version-marker line so the parity test
// can compare Go (1.0.0-dev) and Python (0.6.14) bodies. The line
// is a single comment; everything else MUST be byte-identical.
var versionMarkerRE = regexp.MustCompile(`(?m)^# logmind-hook-version: \S+\n`)

func assertParity(t *testing.T, name, goBody, pyBody string) {
	t.Helper()
	goNorm := versionMarkerRE.ReplaceAllString(goBody, "# logmind-hook-version: VERSION\n")
	pyNorm := versionMarkerRE.ReplaceAllString(pyBody, "# logmind-hook-version: VERSION\n")
	if goNorm != pyNorm {
		// Surface the first differing line for quick triage.
		goLines := strings.Split(goNorm, "\n")
		pyLines := strings.Split(pyNorm, "\n")
		max := len(goLines)
		if len(pyLines) > max {
			max = len(pyLines)
		}
		for i := 0; i < max; i++ {
			var gl, pl string
			if i < len(goLines) {
				gl = goLines[i]
			}
			if i < len(pyLines) {
				pl = pyLines[i]
			}
			if gl != pl {
				t.Fatalf("%s body drifts from Python at line %d:\n  go: %q\n  py: %q",
					name, i+1, gl, pl)
			}
		}
		t.Fatalf("%s body drift detected but no per-line diff (length mismatch?)", name)
	}
}

// pythonHookBody shells to the Python interpreter to capture the
// _build_post_merge_hook_body / _build_post_rewrite_hook_body output
// from the in-tree src/logmind package. Returns (body, false) — and
// calls t.Skip — when Python isn't available or the import fails.
//
// Skip path covers:
//   - python3 not on PATH (uncommon but happens on slim CI images)
//   - logmind package not importable from src/ (e.g., running the
//     Go test suite outside the repo, or after a partial checkout)
//
// We do NOT want a missing Python interpreter to fail the Go test
// suite — the golden-based test above keeps the contract honest for
// CI runs without Python.
func pythonHookBody(t *testing.T, fn string) (string, bool) {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not on PATH; skipping byte-identical-vs-Python check")
		return "", false
	}
	repoRoot := repoRootFromCaller(t)
	script := `import sys; sys.path.insert(0, 'src'); ` +
		`from logmind.core.gitattributes import ` + fn + `; ` +
		`print(` + fn + `(), end='')`
	cmd := exec.Command(py, "-c", script)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("python3 import-and-run failed (Python source unavailable?): %v\n%s", err, out)
		return "", false
	}
	return string(out), true
}

func tempRepoWithHooks(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir .git/hooks: %v", err)
	}
	return dir
}

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
		t.Fatalf("hook body drift vs %s\n--- want ---\n%s\n--- got ---\n%s",
			path, want, body)
	}
}

// repoRootFromCaller walks up from the test file's cwd to the
// directory holding go.mod. Lets us locate the Python source no
// matter where `go test` was launched from.
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
