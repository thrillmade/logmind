package cli

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// update lets `go test ./... -update` rewrite golden files in place. Used
// by `make snapshot` during development; never set in CI. Subsequent
// waves should reuse this exact mechanism so a single -update flag
// refreshes every snapshot in the repo.
var update = flag.Bool("update", false, "regenerate testdata/*.golden files from current Go output")

// TestVersionLine_InProcess locks the `--version` output format from the
// versionLine() function alone (no subprocess). Fast, runs without a
// pre-built binary, and serves as the canonical assertion that the
// protocol-contract format hasn't drifted.
//
// Format: `logmind <version> (spec <spec-version>)`
//
// This is the BYTE-IDENTICAL parity contract. Subsequent waves' golden
// files will follow the same testdata/<command>.golden pattern.
func TestVersionLine_InProcess(t *testing.T) {
	golden := goldenPath(t, "version.golden")
	got := versionLine() + "\n"

	if *update {
		writeGolden(t, golden, got)
		return
	}

	want := readGolden(t, golden)
	if got != want {
		t.Fatalf("versionLine() drifted from %s\n--- want ---\n%q\n--- got ---\n%q",
			golden, want, got)
	}
}

// TestVersionSubcommand_InProcess drives the cobra tree directly and
// asserts `logmind version` emits the golden line. Exercises the
// subcommand wiring (Args validation, output writer plumbing) without
// requiring a built binary.
func TestVersionSubcommand_InProcess(t *testing.T) {
	golden := goldenPath(t, "version.golden")
	want := readGolden(t, golden)

	root := NewRootCmd()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logmind version failed: %v\nstdout:\n%s", err, stdout.String())
	}
	if got := stdout.String(); got != want {
		t.Fatalf("`logmind version` drifted from %s\n--- want ---\n%q\n--- got ---\n%q",
			golden, want, got)
	}
}

// TestVersionFlag_InProcess covers `logmind --version`. Cobra renders
// the Version field through SetVersionTemplate; we pin that path too so
// both invocations stay equivalent.
func TestVersionFlag_InProcess(t *testing.T) {
	golden := goldenPath(t, "version.golden")
	want := readGolden(t, golden)

	root := NewRootCmd()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logmind --version failed: %v\nstdout:\n%s", err, stdout.String())
	}
	if got := stdout.String(); got != want {
		t.Fatalf("`logmind --version` drifted from %s\n--- want ---\n%q\n--- got ---\n%q",
			golden, want, got)
	}
}

// TestVersionBinary_Subprocess is the FOUNDATION snapshot test: it
// builds the real binary, runs it with `--version`, and compares the
// captured stdout to testdata/version.golden byte-for-byte.
//
// Subsequent waves should follow this exact pattern for `init`, `log`,
// `show`, etc.: build once, exec with args, diff against golden. The
// parity gate against Python v0.6.14 (wave verify-parity, later) will
// reuse the same golden files.
//
// Skipped automatically if `go` is not on PATH (e.g., minimal CI
// runners that strip the SDK after test compilation) — the in-process
// tests above still cover the format contract.
func TestVersionBinary_Subprocess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess snapshot test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; skipping subprocess snapshot")
	}

	repoRoot := repoRootFromCaller(t)
	binDir := t.TempDir()
	binName := "logmind"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(binDir, binName)

	build := exec.Command(goBin, "build", "-o", binPath, "./cmd/logmind")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	cmd := exec.Command(binPath, "--version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("logmind --version failed: %v\nstderr:\n%s", err, stderr.String())
	}

	golden := goldenPath(t, "version.golden")
	want := readGolden(t, golden)
	if got := stdout.String(); got != want {
		t.Fatalf("binary `logmind --version` drifted from %s\n--- want ---\n%q\n--- got ---\n%q",
			golden, want, got)
	}
}

// --- helpers -------------------------------------------------------------

// goldenPath resolves to internal/cli/testdata/<name>. Each package keeps
// its own testdata/ so future packages (e.g., internal/log) can add
// goldens without colliding.
func goldenPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func readGolden(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `make snapshot` to create it)", path, err)
	}
	return string(data)
}

func writeGolden(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write golden %s: %v", path, err)
	}
	t.Logf("updated %s", path)
}

// repoRootFromCaller walks up from this test file's location to the
// directory holding go.mod. Lets the subprocess test invoke `go build`
// from the repo root regardless of how `go test` was launched (CI, IDE,
// `cd internal/cli && go test`).
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
	return "" // unreachable
}

// Sanity-checks the format substring so a regression that changes the
// shape (e.g., drops the `(spec ...)` segment) trips this assertion
// immediately, even if someone forgot to regenerate the golden file.
func TestVersionLine_HasSpecSegment(t *testing.T) {
	got := versionLine()
	if !strings.Contains(got, " (spec ") {
		t.Fatalf("versionLine() missing protocol-spec segment: %q", got)
	}
	if !strings.HasPrefix(got, "logmind ") {
		t.Fatalf("versionLine() missing `logmind ` prefix: %q", got)
	}
}
