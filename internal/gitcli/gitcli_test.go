package gitcli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepo creates a fresh git repo at t.TempDir(), commits an initial
// README so HEAD is born, and returns the path. Skips the test if `git`
// is not on PATH (CI runners that strip it after compilation, locked-
// down sandboxes, etc.).
func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestIsRepo_TrueInsideRepo(t *testing.T) {
	if !IsRepo(initRepo(t)) {
		t.Fatalf("IsRepo() = false inside fresh repo")
	}
}

func TestIsRepo_FalseOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	if IsRepo(dir) {
		t.Fatalf("IsRepo(%q) = true; want false (no .git/)", dir)
	}
}

func TestRevParseTopLevel_ReturnsRepoRoot(t *testing.T) {
	dir := initRepo(t)
	top, err := RevParseTopLevel(dir)
	if err != nil {
		t.Fatalf("RevParseTopLevel: %v", err)
	}
	// macOS prepends /private to TempDir paths once resolved; compare
	// via filepath.EvalSymlinks so the test is portable.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("evalsymlinks(%q): %v", dir, err)
	}
	got, err := filepath.EvalSymlinks(top)
	if err != nil {
		t.Fatalf("evalsymlinks(%q): %v", top, err)
	}
	if got != want {
		t.Fatalf("RevParseTopLevel = %q; want %q", got, want)
	}
}

func TestDiffCachedNames_ReturnsStagedPaths(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}
	if err := AddPaths(dir, "new.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	names := DiffCachedNames(dir)
	if len(names) != 1 || names[0] != "new.txt" {
		t.Fatalf("DiffCachedNames = %v; want [new.txt]", names)
	}
}

func TestDiffCachedNumstat_ParsesRows(t *testing.T) {
	dir := initRepo(t)
	var body bytes.Buffer
	for i := 0; i < 10; i++ {
		body.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "code.go"), body.Bytes(), 0o644); err != nil {
		t.Fatalf("write code.go: %v", err)
	}
	if err := AddPaths(dir, "code.go"); err != nil {
		t.Fatalf("add: %v", err)
	}
	rows := DiffCachedNumstat(dir)
	if len(rows) != 1 {
		t.Fatalf("DiffCachedNumstat: got %d rows; want 1 (%+v)", len(rows), rows)
	}
	if rows[0].Path != "code.go" {
		t.Fatalf("row.Path = %q; want code.go", rows[0].Path)
	}
	if rows[0].Added != "10" || rows[0].Removed != "0" {
		t.Fatalf("row counts = (%q, %q); want (10, 0)", rows[0].Added, rows[0].Removed)
	}
}

func TestConfigGet_Set_Roundtrip(t *testing.T) {
	dir := initRepo(t)
	if err := ConfigSet(dir, "merge.logmind-test.driver", "echo test"); err != nil {
		t.Fatalf("ConfigSet: %v", err)
	}
	val, ok := ConfigGet(dir, "merge.logmind-test.driver")
	if !ok || val != "echo test" {
		t.Fatalf("ConfigGet = (%q, %v); want (echo test, true)", val, ok)
	}
}

func TestConfigGet_MissingKey(t *testing.T) {
	dir := initRepo(t)
	val, ok := ConfigGet(dir, "merge.does-not-exist.driver")
	if ok || val != "" {
		t.Fatalf("ConfigGet(missing) = (%q, %v); want ('', false)", val, ok)
	}
}

func TestCurrentBranch_ReturnsBranchName(t *testing.T) {
	dir := initRepo(t)
	branch := CurrentBranch(dir)
	// `git init` default branch is "main" or "master" depending on git
	// version + init.defaultBranch config; both are valid answers.
	if branch != "main" && branch != "master" {
		t.Fatalf("CurrentBranch = %q; want main or master", branch)
	}
}
