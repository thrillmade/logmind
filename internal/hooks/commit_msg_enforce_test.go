// commit_msg_enforce_test.go — a REAL `git commit` integration test for
// the v2.0.0 commit-msg hook body. BuildCommitMsgBody/InstallCommitMsg in
// hooks.go only produce and install a shell script; the actual
// allow/block DECISION lives in internal/guardcommit (exhaustively unit
// tested there and in internal/cli/guard_commit_test.go). This file's
// narrower job: prove the shell script this package emits actually wires
// a genuine `git commit` invocation through to that decision engine and
// back into git's exit-code protocol — i.e. that the wiring, not just the
// logic, works end to end.
package hooks

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/testgit"
)

func TestCommitMsgHook_RealGitCommit_EnforcesAndCarveOuts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; skipping subprocess test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping subprocess test")
	}

	binPath := buildLogmindBinaryForHookTest(t, goBin)
	binDir := filepath.Dir(binPath)

	t.Run("blocks a substantive commit with no decision log", func(t *testing.T) {
		dir := newEnforcingRepo(t, binDir)
		writeBigFile(t, dir, "app.go")
		runGitIn(t, dir, binDir, nil, "add", "app.go")

		out, err := attemptGitCommit(t, dir, binDir, nil, "substantive change, no decision log")
		if err == nil {
			t.Fatalf("expected the commit to be BLOCKED; it succeeded:\n%s", out)
		}
		// Stale-binary hardening: the hook now maps guard-commit's
		// distinctive exit 65 (EX_DATAERR) to its own `exit 1` — git
		// relays that as the commit's own exit code. Pin the concrete
		// exit code so a regression back to a blind `exit $?` relay (or
		// to some other code) is caught here, not just "err != nil".
		if exitErr, ok := err.(*exec.ExitError); ok {
			if got := exitErr.ExitCode(); got != 1 {
				t.Errorf("git commit exit code = %d; want 1 (the hook's block exit, driven by guard-commit's 65)", got)
			}
		} else {
			t.Errorf("expected a *exec.ExitError from the blocked commit; got %T (%v)", err, err)
		}
		if !strings.Contains(out, "logmind log") {
			t.Errorf("blocked-commit output should mention `logmind log`; got:\n%s", out)
		}
		assertCommitCount(t, dir, binDir, 1) // only newEnforcingRepo's initial commit
	})

	t.Run("allows with [skip-logmind] in the subject", func(t *testing.T) {
		dir := newEnforcingRepo(t, binDir)
		writeBigFile(t, dir, "app.go")
		runGitIn(t, dir, binDir, nil, "add", "app.go")

		out, err := attemptGitCommit(t, dir, binDir, nil, "substantive change [skip-logmind]")
		if err != nil {
			t.Fatalf("expected the commit to be ALLOWED via [skip-logmind]; got error: %v\n%s", err, out)
		}
		assertCommitCount(t, dir, binDir, 2)
	})

	t.Run("allows with LOGMIND_ALLOW_GIT_COMMIT=1", func(t *testing.T) {
		dir := newEnforcingRepo(t, binDir)
		writeBigFile(t, dir, "app.go")
		runGitIn(t, dir, binDir, nil, "add", "app.go")

		out, err := attemptGitCommit(t, dir, binDir, []string{"LOGMIND_ALLOW_GIT_COMMIT=1"}, "substantive change, explicit override")
		if err != nil {
			t.Fatalf("expected the commit to be ALLOWED via LOGMIND_ALLOW_GIT_COMMIT=1; got error: %v\n%s", err, out)
		}
		assertCommitCount(t, dir, binDir, 2)
	})

	// Carve-out 5, at the only surface that proves the WIRING: a real
	// `git commit` under the really-installed hook, against the really-
	// built binary.
	//
	// This subtest used to stage "# Decisions\n\n## entry\n" — a
	// decision-shaped PATH carrying no entry — and assert it was ALLOWED.
	// That was the sentinel hole in miniature: the gate answered on the
	// filename, so `git add docs/decisions.md` cleared it for any amount
	// of code. Round 14 closed the hole and left this assertion behind,
	// which is why the branch shipped a red suite. Every row now stages
	// CONTENT, and the rows are each other's mutation test: flip the
	// predicate in either direction and at least one fails.
	for _, tc := range []struct {
		name, body  string
		wantAllowed bool
	}{
		{
			name: "an entry with reasoning on the marker line",
			body: "# Decisions\n\n## 2026-08-07 14:30 - Inline reasoning\n\n" +
				"**Reasoning:** the shape logmind log writes.\n\n---\n",
			wantAllowed: true,
		},
		{
			// THE FIELD BUG. An ordinary hand-written entry whose body
			// sits below a blank line. Measured before the fix: exit 65,
			// commit refused, three sentences of reasoning right there in
			// the index.
			name: "an entry whose reasoning body sits below a blank line",
			body: "# Decisions\n\n## 2026-08-07 14:30 - Blank-separated body\n\n" +
				"**Reasoning:**\n\nThe author pressed return before writing the reason.\n" +
				"That is a paragraph break, not an empty section.\n\n---\n",
			wantAllowed: true,
		},
		{
			// A decision-shaped file with no entry in it. §3.4: a decision
			// "clears the gate by being written, not by existing."
			name:        "a decision file carrying no entry at all",
			body:        "# Decisions\n\n## entry\n",
			wantAllowed: false,
		},
		{
			// §3.1 makes omitting an empty section's header a MUST, so a
			// header with nothing under it is malformed rather than sparse.
			name: "an entry whose reasoning header has nothing under it",
			body: "# Decisions\n\n## 2026-08-07 14:30 - Empty section\n\n" +
				"**Reasoning:**\n\n**Alternatives considered:** none\n\n---\n",
			wantAllowed: false,
		},
	} {
		t.Run("staged decision file: "+tc.name, func(t *testing.T) {
			dir := newEnforcingRepo(t, binDir)
			writeBigFile(t, dir, "app.go")
			if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "docs", "decisions.md"), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			runGitIn(t, dir, binDir, nil, "add", "app.go", "docs/decisions.md")

			out, err := attemptGitCommit(t, dir, binDir, nil, "substantive change with a decision log")
			switch {
			case tc.wantAllowed && err != nil:
				t.Fatalf("expected the commit to be ALLOWED — the staged diff adds a §3.1 entry; got error: %v\n%s", err, out)
			case !tc.wantAllowed && err == nil:
				t.Fatalf("expected the commit to be BLOCKED — the staged diff records no decision; it succeeded:\n%s", out)
			}
			want := 1 // only newEnforcingRepo's initial commit
			if tc.wantAllowed {
				want = 2
			}
			assertCommitCount(t, dir, binDir, want)
		})
	}
}

// TestCommitMsgHook_StaleBinaryOnPath_FailsOpen is the load-bearing
// regression test for the stale-binary hardening (CTO design amendment):
// a STALE-but-present `logmind` shadowing the real binary earlier on
// PATH — simulating an old 1.x Cobra build that doesn't know
// `guard-commit` (exit 1 on an unrecognized subcommand) or the frozen
// Python v0.6.16 CLI (argparse's exit 2 on the same) — must NOT block a
// substantive, no-decision-log commit. Before this hardening the hook did
// a blind `exit $?` relay of whatever `logmind` printed, so EITHER stale
// exit code would abort the commit exactly like a genuine block —
// bricking every commit on that machine, including `logmind log`'s own
// internal commit. After the hardening, only exit 65 (guard-commit's own
// distinctive EX_DATAERR signal) aborts; anything else falls open.
func TestCommitMsgHook_StaleBinaryOnPath_FailsOpen(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; skipping subprocess test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping subprocess test")
	}

	binPath := buildLogmindBinaryForHookTest(t, goBin)
	binDir := filepath.Dir(binPath)

	for _, staleExitCode := range []int{1, 2} {
		t.Run(fmt.Sprintf("stale logmind exiting %d fails open", staleExitCode), func(t *testing.T) {
			// The real (current) binary installs the hook and handles the
			// fixture's initial commit, same as every other subtest.
			dir := newEnforcingRepo(t, binDir)

			// A fake `logmind` that mimics a stale binary's unknown-command
			// error, placed on a PATH entry BEFORE binDir, so `command -v
			// logmind` resolves to it — not the real, current binary.
			staleDir := writeFakeStaleLogmind(t, staleExitCode)

			writeBigFile(t, dir, "app.go")
			runGitInPath(t, dir, []string{staleDir, binDir}, nil, "add", "app.go")

			out, err := attemptGitCommitPath(t, dir, []string{staleDir, binDir}, nil,
				"substantive change, stale logmind shadowing the real binary")
			if err != nil {
				t.Fatalf("expected the commit to SUCCEED (fail OPEN on a stale logmind exiting %d); got error: %v\n%s",
					staleExitCode, err, out)
			}
			assertCommitCount(t, dir, binDir, 2)

			// Issue #270 / SPEC §3.4: failing open must not be SILENT. The
			// hook has to name what it looked for, what it found (the
			// resolved engine path and its exit status), and which logmind
			// installed it — otherwise a repository whose gate has been off
			// for weeks looks exactly like one where every commit complied.
			// The stale binary's own "unknown command" line does not count:
			// it says nothing about the gate not having run.
			for _, must := range []string{
				"commit gate NOT RUN",
				filepath.Join(staleDir, "logmind"),
				fmt.Sprintf("exit %d", staleExitCode),
				"installed by logmind " + hookVersion(),
			} {
				if !strings.Contains(out, must) {
					t.Errorf("fail-open notice missing %q; git output was:\n%s", must, out)
				}
			}
		})
	}
}

// TestCommitMsgHook_NoEngineOnPath_FailsOpenLoudly is the other half of the
// issue #270 regression: not a WRONG engine but NO engine. §3.4 requires the
// commit to be allowed (exit 0) and requires the hook to say so, naming what
// it looked for and what it found.
//
// Runs the installed hook script directly under an empty PATH rather than
// through `git commit`. That is the hermetic way to express "nothing on PATH
// answers to logmind" — filtering the host's real PATH would depend on where
// this machine happens to keep git and logmind (often the same directory).
// git contributes nothing to this path anyway: it runs the hook file and
// takes its exit code, which is exactly what this asserts.
func TestCommitMsgHook_NoEngineOnPath_FailsOpenLoudly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX sh hook body; not applicable on Windows")
	}
	dir := initRealGitRepo(t)
	if _, err := InstallCommitMsg(dir); err != nil {
		t.Fatalf("InstallCommitMsg: %v", err)
	}
	msgFile := filepath.Join(dir, "COMMIT_EDITMSG")
	if err := os.WriteFile(msgFile, []byte("substantive change, no decision log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("/bin/sh", filepath.Join(dir, ".git", "hooks", "commit-msg"), msgFile)
	cmd.Dir = dir
	cmd.Env = []string{"PATH="} // nothing on PATH answers to `logmind`
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	// Fail OPEN — a missing engine must never block a commit.
	if err != nil {
		t.Fatalf("hook exited non-zero with no engine on PATH: %v\nstderr: %s", err, stderr.String())
	}
	// ...but loudly, and on stderr: stdout belongs to whatever is capturing
	// git's output, and §3.4 keeps a gate's report about itself off it.
	for _, must := range []string{
		"commit gate NOT RUN",
		"found nothing",
		"installed by logmind " + hookVersion(),
	} {
		if !strings.Contains(stderr.String(), must) {
			t.Errorf("stderr missing %q; got: %q", must, stderr.String())
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("hook wrote to stdout: %q; the notice belongs on stderr", stdout.String())
	}
}

// writeFakeStaleLogmind creates a directory containing a `logmind` shell
// script that unconditionally prints an "unknown command" style message
// and exits with staleExitCode — standing in for a stale binary (an old
// Cobra build that predates `guard-commit`, or the frozen Python v0.6.16
// argparse CLI) that happens to still be resolvable on PATH ahead of the
// current one. Returns the directory to prepend to PATH.
func writeFakeStaleLogmind(t *testing.T, staleExitCode int) string {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\necho \"logmind: unknown command 'guard-commit'\" >&2\nexit %d\n", staleExitCode)
	path := filepath.Join(dir, "logmind")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake stale logmind: %v", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod fake stale logmind: %v", err)
	}
	return dir
}

// newEnforcingRepo creates a fresh git repo with this package's
// commit-msg hook installed and one initial commit — so commit COUNTING
// (not merely "does HEAD exist"), is what distinguishes blocked vs.
// allowed across subtests below. The initial commit ALSO runs through
// the freshly-installed hook (with binDir on PATH, same as every other
// git invocation in this file) — it's tiny (one line), so it's expected
// to sail through the under-threshold carve-out; if that regresses, this
// helper fails loudly instead of masking it as "the fixture doesn't hit
// the hook."
func newEnforcingRepo(t *testing.T, binDir string) string {
	t.Helper()
	dir := t.TempDir()
	runGitIn(t, dir, binDir, nil, "init", "-q")
	// Disable background maintenance — see testgit's package doc (issue
	// #271). `git config` needs no special PATH/env, so this can go
	// straight through testgit rather than this file's binDir-aware
	// runGitIn wrapper.
	testgit.DisableMaintenance(t, dir)
	runGitIn(t, dir, binDir, nil, "config", "user.email", "t@t.com")
	runGitIn(t, dir, binDir, nil, "config", "user.name", "t")
	if _, err := InstallCommitMsg(dir); err != nil {
		t.Fatalf("InstallCommitMsg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, binDir, nil, "add", "README.md")
	out, err := attemptGitCommit(t, dir, binDir, nil, "init")
	if err != nil {
		t.Fatalf("fixture's initial commit unexpectedly blocked: %v\n%s", err, out)
	}
	return dir
}

// isolatedGitEnv builds the environment for every git invocation in this
// file: PATH extended with binDir (so the installed hook's
// `command -v logmind` finds the binary this test built rather than
// whatever `logmind` may already be on the host's real PATH — e.g. an
// older release without the `guard-commit` command, which is exactly the
// footgun this exists to avoid), plus isolation from the host's/CI
// runner's global or system git config (a stray global commit.gpgsign or
// core.hooksPath would make this flaky, or hang on a signing prompt).
func isolatedGitEnv(t *testing.T, binDir string) []string {
	t.Helper()
	return pathEnv(t, binDir)
}

// pathEnv is isolatedGitEnv's generalisation: it accepts an ORDERED list
// of directories to prepend to PATH (ahead of the host's real PATH) rather
// than a single binDir. Used by the stale-binary simulation to make a fake
// `logmind` resolve via `command -v logmind` BEFORE the real one — the
// same shape a stray pyenv shim or an old release earlier in a real
// developer's PATH would produce.
func pathEnv(t *testing.T, pathDirs ...string) []string {
	t.Helper()
	fakeHome := t.TempDir()
	parts := append(append([]string{}, pathDirs...), os.Getenv("PATH"))
	path := strings.Join(parts, string(os.PathListSeparator))
	return []string{
		"HOME=" + fakeHome,
		"USERPROFILE=" + fakeHome, // Windows' HOME equivalent
		"GIT_CONFIG_NOSYSTEM=1",
		"XDG_CONFIG_HOME=" + fakeHome,
		"PATH=" + path,
	}
}

// writeBigFile writes 30 substantive lines — comfortably over the
// default commit_line_threshold of 20 regardless of diff mode. Mirrors
// internal/cli/guard_commit_test.go's bigLines helper (duplicated here;
// different package, same small-helper-duplication convention this
// codebase already uses — see buildLogmindBinaryForHookTest below).
func writeBigFile(t *testing.T, dir, name string) {
	t.Helper()
	var b bytes.Buffer
	for i := 0; i < 30; i++ {
		b.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func runGitIn(t *testing.T, dir, binDir string, extraEnv []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	env := append(os.Environ(), isolatedGitEnv(t, binDir)...)
	cmd.Env = append(env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// attemptGitCommit runs `git commit -m message` — same isolated,
// binDir-extended PATH as runGitIn, plus any extraEnv (e.g.
// LOGMIND_ALLOW_GIT_COMMIT=1). Returns combined output and the *exec.Cmd
// error — non-nil on a non-zero exit, i.e. the hook blocked the commit.
func attemptGitCommit(t *testing.T, dir, binDir string, extraEnv []string, message string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "-m", message)
	cmd.Dir = dir
	env := append(os.Environ(), isolatedGitEnv(t, binDir)...)
	cmd.Env = append(env, extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runGitInPath and attemptGitCommitPath are runGitIn/attemptGitCommit's
// generalisation to an ORDERED list of PATH directories (via pathEnv)
// instead of a single binDir — used by the stale-binary simulation to put
// a fake `logmind` ahead of the real one on PATH.
func runGitInPath(t *testing.T, dir string, pathDirs []string, extraEnv []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	env := append(os.Environ(), pathEnv(t, pathDirs...)...)
	cmd.Env = append(env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func attemptGitCommitPath(t *testing.T, dir string, pathDirs []string, extraEnv []string, message string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "-m", message)
	cmd.Dir = dir
	env := append(os.Environ(), pathEnv(t, pathDirs...)...)
	cmd.Env = append(env, extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func assertCommitCount(t *testing.T, dir, binDir string, want int) {
	t.Helper()
	got := runGitIn(t, dir, binDir, nil, "rev-list", "--count", "HEAD")
	got = strings.TrimSpace(got)
	if got != fmt.Sprintf("%d", want) {
		t.Fatalf("commit count = %s; want %d", got, want)
	}
}

// buildLogmindBinaryForHookTest compiles ./cmd/logmind into a temp dir
// and returns the binary's path. Mirrors
// internal/cli/guard_commit_test.go's buildGuardCommitBinary — kept as a
// separate copy (rather than shared) per this codebase's existing
// small-test-helper-duplication convention (see e.g.
// internal/cli/install_hook_test.go's initRepo doc comment).
func buildLogmindBinaryForHookTest(t *testing.T, goBin string) string {
	t.Helper()
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
	return binPath
}
