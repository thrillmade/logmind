package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/atomicio"
	"github.com/thrillmade/logmind/internal/clierr"
	"github.com/thrillmade/logmind/internal/gitcli"
	"github.com/thrillmade/logmind/internal/hooks"
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

	// The directory git READS hooks from, never a `.git/hooks` join —
	// `core.hooksPath` moves it, and a hook written where git does not look
	// is installed as far as this command is concerned and dead as far as
	// git is concerned. hooks.Path is the one owner of that resolution;
	// `logmind init` and `doctor --fix` write through the same answer.
	hookPath, ok := hooks.Path(top, "pre-commit")
	if !ok {
		fmt.Fprintln(stdout, "Error: git could not resolve this repository's hooks directory.")
		return ErrSilent
	}

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
		// DELIBERATELY os.WriteFile, not atomicio. Justified by atomicio's
		// one rule (see internal/atomicio's package doc), not by an
		// exception to it: an atomic replace swaps the NAME, so it refuses
		// a symlink on the destination (rule 2) and severs hardlinks
		// (rule 3). Both are exactly what must not happen here.
		//
		//   - Write-through IS the intent. Pointing .git/hooks/pre-commit
		//     at a shared/tracked script — husky, chezmoi, a dotfile-managed
		//     hooks dir, or a hardlink into one — is a common, deliberate
		//     setup, and appending logmind's line to that shared script is
		//     what --force was asked to do. atomicio would refuse the write
		//     outright, or silently hand the user a detached private copy
		//     that stops tracking the shared file.
		//   - The dangling-symlink attack cannot reach this branch. We are
		//     on the ReadFile-SUCCEEDED path, so the name resolved to a real
		//     file; the exploit needs ErrNotExist (see the fresh-install
		//     branch below, which IS routed through atomicio).
		//   - Nor can a hostile repo plant the link: git never checks
		//     anything out into .git/, so .git/hooks/pre-commit is not
		//     attacker-supplied content in the threat model this sweep is
		//     about — unlike AGENTS.md, .github/workflows/*, or .claude/.
		//
		// (This keep used to argue a third point — that atomicio
		// "unconditionally chmods to the perm argument". That was true of
		// the old implementation and is no longer: rule 1 preserves an
		// existing file's mode. Deleted rather than restated, because a keep
		// that outlives its reason is how an exception becomes permanent.)
		if err := os.WriteFile(hookPath, []byte(newContent), 0o755); err != nil {
			return err
		}
		// Python doesn't re-chmod on the append path (the file
		// already had its exec bit when read). Mirror that — don't
		// chmod here either; if the user had a non-exec custom
		// hook, we preserve their mode (best-effort, matches Python).
		// os.WriteFile's perm argument above is inert for the same
		// reason: it only applies on create.
		fmt.Fprintln(stdout, "✓ Added logmind check-decisions to existing pre-commit hook.")
		return nil

	case errors.Is(readErr, os.ErrNotExist):
		// Fresh install. Python writes parent dir + "#!/bin/sh\n" + hook_line + chmod 0o755.
		//
		// atomicio.WriteFile (which does the MkdirAll itself), not
		// os.WriteFile: ErrNotExist here does NOT mean "nothing is at this
		// path". A dangling symlink at .git/hooks/pre-commit lands us on
		// exactly this branch, and a bare os.WriteFile would follow it and
		// drop an executable 0o755 shell script wherever it points. The
		// rename replaces the link itself.
		//
		// WriteFileMode, not WriteFile: 0o755 is the point, not a default.
		// Rule 1 makes WriteFile's perm a CREATE mode that the umask filters
		// (a user with `umask 077` would get 0o700, and one who somehow had a
		// non-exec hook would keep it), so the mode-asserting variant states
		// the contract at the call site and replaces the follow-up os.Chmod
		// this branch used to need. Python chmods 0o755 unconditionally too.
		body := "#!/bin/sh\n" + preCommitGuardedCall
		if err := atomicio.WriteFileMode(hookPath, []byte(body), 0o755); err != nil {
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
//     `command not found` non-zero that would wrongly block the commit. As
//     of issue #270 it is not a SILENT no-op: SPEC §3.4 requires a gate that
//     fails open to say so on stderr, "naming what it looked for and what it
//     found". This hook is opt-in — the user asked for it by running
//     `logmind install-hook` — so it not running is exactly what they need
//     told. Exit status is unchanged (still 0).
//   - Stale binary: unlike the commit-msg hook, no skew handshake is needed
//     here. This block PRESERVES check-decisions' exit code, so an engine
//     that doesn't know the subcommand exits nonzero and BLOCKS — noisy and
//     visible, never a silent allow. Missing is the only way this gate can
//     disappear quietly, and that is the branch made loud below.
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
	"# on the normal path. A missing binary is a clean no-op — but not a\n" +
	"# silent one (issue #270): a gate that cannot report its own absence\n" +
	"# gets trusted long after it stopped working.\n" +
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
	"else\n" +
	"    printf 'logmind: check-decisions NOT RUN — looked for `logmind` on PATH, found nothing. Commit allowed.\\n' >&2\n" +
	"fi\n"

// ErrSilent is the cli-layer alias of clierr.ErrSilent. Backward-compat
// shim so existing cli/* references (cobra hooks, tests) keep working
// against `cli.ErrSilent` without each grabbing the clierr import. The
// underlying variable is shared — `errors.Is(err, cli.ErrSilent)` and
// `errors.Is(err, clierr.ErrSilent)` resolve to the same sentinel, so
// cross-package wraps from `internal/skill/` continue to trigger the
// same silent-exit path through cmd/logmind/main.go.
var ErrSilent = clierr.ErrSilent
