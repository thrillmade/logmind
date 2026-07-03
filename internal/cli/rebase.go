package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/gitcli"
)

// newRebaseCmd wires `logmind rebase [--base BR] [--no-push] [--no-fetch]`.
//
// Mirror of Python cli.rebase (cli.py:1029-1156). Three-step wrapper:
//
//  1. git fetch origin <base>          (unless --no-fetch)
//  2. git rebase origin/<base>
//  3. git push --force-with-lease      (unless --no-push)
//
// Refuses to run when:
//   - not in a git repo                        → "Error: not in a git repository."
//   - HEAD is detached                         → "Error: detached HEAD — ..."
//   - current branch == base branch            → "Error: refusing to rebase ..."
//
// On step failure the wrapper exits 1 with a descriptive message that
// includes git's stderr — matches Python's `e.stderr` interpolation.
//
// Step output (`→ git fetch origin ...`, `✓ Rebased ...`, `ok ...`) is
// byte-identical to Python v0.6.14.
func newRebaseCmd() *cobra.Command {
	var base string
	var noPush bool
	var noFetch bool
	cmd := &cobra.Command{
		Use:   "rebase",
		Short: "Fetch origin, rebase the current branch, force-with-lease push",
		Long: `Fetch origin, rebase the current branch onto origin/<base>, and force-with-lease push.

Convenience wrapper for the recurring three-step pattern hit when a PR
goes DIRTY after another PR's derived-doc regen lands on main:

    git fetch origin
    git rebase origin/<default-branch>
    git push --force-with-lease

Exits non-zero on any step failure with a clear message about which
step failed and what to do next. Refuses to run on a detached HEAD
or on the default branch itself (rebasing main onto main is nonsense).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runRebase(cwd, base, noPush, noFetch, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&base, "base", "",
		"Base branch to rebase onto. Defaults to the repo's default branch.")
	cmd.Flags().BoolVar(&noPush, "no-push", false,
		"Skip the push step. Just fetch + rebase.")
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false,
		"Skip the fetch step. Rebase against whatever origin/<base> already points at.")
	return cmd
}

func runRebase(cwd, base string, noPush, noFetch bool, stdout io.Writer) error {
	if !gitcli.IsRepo(cwd) {
		fmt.Fprintln(stdout, "Error: not in a git repository.")
		return ErrSilent
	}
	branch := gitcli.CurrentBranch(cwd)
	if branch == "" {
		fmt.Fprintln(stdout, "Error: detached HEAD — `logmind rebase` needs a named branch.")
		return ErrSilent
	}
	baseBranch := base
	if baseBranch == "" {
		baseBranch = gitcli.DefaultBranch(cwd)
	}
	if branch == baseBranch {
		fmt.Fprintf(stdout, "Error: refusing to rebase '%s' onto itself. Check out a feature branch first.\n", baseBranch)
		return ErrSilent
	}

	// Step 1: fetch.
	if !noFetch {
		fmt.Fprintf(stdout, "→ git fetch origin %s\n", baseBranch)
		if _, stderr, err := gitcli.RunCaptured(cwd, "fetch", "origin", baseBranch); err != nil {
			// Python: "Error: git fetch failed.\n{e.stderr}" via click.secho.
			fmt.Fprintf(stdout, "Error: git fetch failed.\n%s\n", stderr)
			return ErrSilent
		}
	}

	// Step 2: rebase.
	fmt.Fprintf(stdout, "→ git rebase origin/%s\n", baseBranch)
	if _, stderr, err := gitcli.RunCaptured(cwd, "rebase", "origin/"+baseBranch); err != nil {
		fmt.Fprintf(stdout, "Error: git rebase failed.\n%s\nResolve conflicts manually, then run `git rebase --continue` (or `git rebase --abort` to bail).\n", stderr)
		return ErrSilent
	}

	// Step 3: push (unless --no-push).
	if noPush {
		fmt.Fprintf(stdout, "✓ Rebased '%s' onto origin/%s (push skipped).\n", branch, baseBranch)
		fmt.Fprintf(stdout, "ok rebased: %s onto origin/%s (no push)\n", branch, baseBranch)
		return nil
	}

	fmt.Fprintf(stdout, "→ git push --force-with-lease origin %s\n", branch)
	if _, stderr, err := gitcli.RunCaptured(cwd, "push", "--force-with-lease", "origin", branch); err != nil {
		fmt.Fprintf(stdout, "Error: git push --force-with-lease failed.\n%s\nRebase succeeded locally; you can retry push manually.\n", stderr)
		return ErrSilent
	}

	fmt.Fprintf(stdout, "✓ Rebased '%s' onto origin/%s and pushed.\n", branch, baseBranch)
	fmt.Fprintf(stdout, "ok rebased: %s onto origin/%s (pushed)\n", branch, baseBranch)
	return nil
}
