package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/clierr"
	"github.com/thrillmade/logmind/internal/gitcli"
)

// newInstallHookCmd wires the `logmind install-hook` subcommand.
//
// Behaviour mirror of src/logmind/cli.install_hook (cli.py:2814-2862):
//
//   - Not a git repo → "Error: not a git repository." to STDERR, exit 1.
//   - No prior pre-commit hook → write "#!/bin/sh\n" + preCommitGuardedCall,
//     chmod 0755, "✓ Installed logmind pre-commit hook."
//   - Existing hook already contains "logmind check-decisions" → no-op,
//     "✓ logmind hook already installed."
//   - Existing hook is foreign + --force not set → "A pre-commit hook
//     already exists. Use --force to append logmind to it.", exit 1.
//   - Existing hook is foreign + --force set → append preCommitGuardedCall
//     after stripping trailing newlines from the original, "✓ Added logmind
//     check-decisions to existing pre-commit hook."
//
// #213 divergence from Python parity: the emitted `logmind check-decisions`
// invocation is now wrapped in a POSIX-portable hang-guard (see
// preCommitGuardedCall) so a wedged binary can't stall `git commit`; it also
// gains a `command -v logmind` guard the bare Python line lacked.
//
// The "✓ " prefix is preserved verbatim — agents that grep for it
// (and downstream tooling that diffs Python and Go output) need the
// byte-identical string. STDERR vs STDOUT routing matches Python's
// click.echo / click.secho defaults: success messages go to stdout,
// errors go to stderr (click.secho fg=red implies stderr unless the
// command overrides — install_hook uses click.secho with no err=
// override, so it lands on stdout. We match that.).
//
// Exit code conventions are enforced by the caller (cmd/logmind/main.go
// returns 1 on cobra error); we return a sentinel error to trigger it.
func newInstallHookCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "install-hook",
		Short: "Install logmind check-decisions as a git pre-commit hook",
		Long: `Install logmind check-decisions as a git pre-commit hook.

Creates or appends to .git/hooks/pre-commit so that every commit
is checked for undocumented decisions.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runInstallHook(cwd, force, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Add logmind to an existing pre-commit hook without prompting")
	return cmd
}

// runInstallHook is the testable core. Splitting it out lets unit
// tests drive the routine against a temp repo without spawning a
// cobra root command.
func runInstallHook(cwd string, force bool, stdout io.Writer) error {
	if !gitcli.IsRepo(cwd) {
		// The Python branch emits via click.secho(... fg="red") then
		// sys.exit(1). click.secho defaults to writing to stdout
		// (click.echo's target) unless err=True. We match Python
		// here — the error string lands on stdout, NOT stderr.
		// Downstream pipelines that pipe `logmind install-hook | foo`
		// expect this layout.
		fmt.Fprintln(stdout, "Error: not a git repository.")
		return ErrSilent
	}

	top, err := gitcli.RevParseTopLevel(cwd)
	if err != nil {
		// Best-effort fallback: if rev-parse fails we still attempt
		// to install relative to cwd. Python does the same — the
		// Path() conversion at line 2839 would yield "" + raise on
		// any subsequent .git/ lookup; here we degrade gracefully.
		top = cwd
	}
	if top == "" {
		top = cwd
	}

	hookPath := filepath.Join(top, ".git", "hooks", "pre-commit")

	data, readErr := os.ReadFile(hookPath)
	switch {
	case readErr == nil:
		// Hook file exists. Decide: already-ours / no-force-conflict /
		// force-append.
		content := string(data)
		if strings.Contains(content, preCommitMarker) {
			fmt.Fprintln(stdout, "✓ logmind hook already installed.")
			return nil
		}
		if !force {
			fmt.Fprintln(stdout, "A pre-commit hook already exists. Use --force to append logmind to it.")
			return ErrSilent
		}
		// Append the hang-guarded block after the existing content
		// (Python appended a bare `logmind check-decisions` line here; #213
		// upgrades that to the deadline-wrapped invocation).
		newContent := strings.TrimRight(content, "\n") + "\n" + preCommitGuardedCall
		if err := os.WriteFile(hookPath, []byte(newContent), 0o755); err != nil {
			return err
		}
		// Python doesn't re-chmod on the append path (the file
		// already had its exec bit when read). Mirror that — don't
		// chmod here either; if the user had a non-exec custom
		// hook, we preserve their mode (best-effort, matches Python).
		fmt.Fprintln(stdout, "✓ Added logmind check-decisions to existing pre-commit hook.")
		return nil

	case errors.Is(readErr, os.ErrNotExist):
		// Fresh install. Python writes parent dir + "#!/bin/sh\n" + hook_line + chmod 0o755.
		if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
			return err
		}
		body := "#!/bin/sh\n" + preCommitGuardedCall
		if err := os.WriteFile(hookPath, []byte(body), 0o755); err != nil {
			return err
		}
		// WriteFile only honours perm bits on CREATE; chmod
		// explicitly to make sure the exec bit survives umask
		// stripping (Python does chmod(0o755) unconditionally too).
		if err := os.Chmod(hookPath, 0o755); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "✓ Installed logmind pre-commit hook.")
		return nil

	default:
		// Permission error or other read failure — propagate; cobra
		// will print it on stderr.
		return readErr
	}
}

// preCommitMarker is the substring that identifies a logmind-owned
// pre-commit hook. Kept literal (not the whole guarded body) so an existing
// install written by ANY logmind version — the pre-#213 bare
// `logmind check-decisions` line or the current hang-guarded block — is
// recognized as ours and reported "already installed".
const preCommitMarker = "logmind check-decisions"

// preCommitGuardedCall is the pre-commit hook's `logmind check-decisions`
// invocation wrapped in a POSIX-portable deadline (issue #213). A hung or
// wedged logmind binary on PATH must never stall `git commit`; timeout(1)
// isn't on macOS by default, so we background the call, background a
// `sleep N; kill` watchdog, `wait` the main pid for its real exit code,
// then reap the watchdog.
//
//   - Missing binary: the `command -v logmind` guard makes it a clean no-op
//     (fall through to the end of the script → exit 0) instead of a
//     `command not found` non-zero that would wrongly block the commit.
//   - Normal completion: check-decisions' own exit code is PRESERVED —
//     `exit 0` when clean/under-threshold, `exit 1` when it blocks an
//     undocumented over-threshold change (its designed pre-commit behavior).
//   - Timeout / crash: the watchdog kill (or a crash) yields a signal exit
//     >128; we FAIL OPEN (exit 0) — matching the enforcement's "all else
//     fails open" principle. The goal is to never HANG, not to block.
//
// The watchdog subshell's fds are redirected to /dev/null so its (possibly
// orphaned) `sleep` child can't hold this hook's stdout/stderr open — else a
// caller that CAPTURES git's output via a pipe (Claude Code's Bash tool, CI)
// would block reading until the sleep expired, stalling even a fast commit.
const preCommitGuardedCall = "# logmind check-decisions — hang-guarded (issue #213): run under a\n" +
	"# deadline so a wedged logmind binary can never stall `git commit`.\n" +
	"# Fail OPEN (exit 0) on timeout/crash; preserve a real block exit code\n" +
	"# on the normal path. A missing binary is a clean no-op.\n" +
	"if command -v logmind >/dev/null 2>&1; then\n" +
	"    logmind check-decisions &\n" +
	"    __lm_pid=$!\n" +
	"    ( sleep 10; kill \"$__lm_pid\" 2>/dev/null ) >/dev/null 2>&1 &\n" +
	"    __lm_watcher=$!\n" +
	"    wait \"$__lm_pid\" 2>/dev/null\n" +
	"    __lm_rc=$?\n" +
	"    kill \"$__lm_watcher\" 2>/dev/null\n" +
	"    wait \"$__lm_watcher\" 2>/dev/null\n" +
	"    if [ \"$__lm_rc\" -gt 128 ]; then\n" +
	"        exit 0\n" +
	"    fi\n" +
	"    exit \"$__lm_rc\"\n" +
	"fi\n"

// ErrSilent is the cli-layer alias of clierr.ErrSilent. Backward-compat
// shim so existing cli/* references (cobra hooks, tests) keep working
// against `cli.ErrSilent` without each grabbing the clierr import. The
// underlying variable is shared — `errors.Is(err, cli.ErrSilent)` and
// `errors.Is(err, clierr.ErrSilent)` resolve to the same sentinel, so
// cross-package wraps from `internal/skill/` continue to trigger the
// same silent-exit path through cmd/logmind/main.go.
var ErrSilent = clierr.ErrSilent
