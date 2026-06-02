package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/gitcli"
)

// newCheckDecisionsCmd wires `logmind check-decisions [--threshold N] [--no-fail]`.
//
// Behaviour mirror of src/logmind/cli.check_decisions (cli.py:2425-2519).
// Output strings are byte-identical to Python v0.6.14:
//
//   not-a-git-repo:      "Not a git repository, skipping check."  (exit 0)
//   decision file staged: "✓ A decision log file is staged — changes are documented."
//   under threshold:     "✓ N lines changed (below T-line threshold)."  (exit 0)
//   over threshold:      multi-line warning, exit 1 unless --no-fail
//
// "Decision file" matches the Python predicate:
//   - exact path "docs/decisions.md"
//   - suffix "/decisions.md" (covers nested decisions.md)
//   - prefix "docs/decisions-branches/" (per-branch decision files)
//
// The numstat-based LOC count REPLICATES the Python skip rules:
//   - filepath beginning with "docs/" is skipped (not code, lives in docs)
//   - "added == '-'" rows are binary files, skipped
//   - rows that fail integer-parse are silently swallowed
func newCheckDecisionsCmd() *cobra.Command {
	var threshold int
	var noFail bool
	cmd := &cobra.Command{
		Use:   "check-decisions",
		Short: "Check that significant code changes have corresponding decision logs",
		Long: `Check that significant code changes have corresponding decision logs.

Designed for use as a git pre-commit hook. Exits with code 1 if staged
changes exceed the line threshold without an update to docs/decisions.md.

To install as a pre-commit hook, run: logmind install-hook

Examples:
    logmind check-decisions
    logmind check-decisions --threshold 50
    logmind check-decisions --no-fail`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runCheckDecisions(cwd, threshold, noFail, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVarP(&threshold, "threshold", "t", 20,
		"Minimum lines changed to require a decision log entry (default: 20)")
	cmd.Flags().BoolVar(&noFail, "no-fail", false,
		"Warn but exit with code 0 (don't block the commit)")
	return cmd
}

// runCheckDecisions is the testable core. Returns errSilentExit1 to
// trigger exit 1 without cobra re-printing the message — mirrors
// the Python sys.exit(1) shape.
func runCheckDecisions(cwd string, threshold int, noFail bool, stdout io.Writer) error {
	if !gitcli.IsRepo(cwd) {
		// Python: click.echo(...) which goes to stdout, no exit.
		fmt.Fprintln(stdout, "Not a git repository, skipping check.")
		return nil
	}

	staged := gitcli.DiffCachedNames(cwd)
	for _, f := range staged {
		if isDecisionFile(f) {
			fmt.Fprintln(stdout, "✓ A decision log file is staged — changes are documented.")
			return nil
		}
	}

	total := 0
	for _, row := range gitcli.DiffCachedNumstat(cwd) {
		// Mirror the Python skip rules verbatim (cli.py:2498-2504):
		//   * filepath.startswith("docs/")  → skipped
		//   * added == "-"                  → binary, skipped
		//   * any int parse failure         → swallowed (pass)
		if strings.HasPrefix(row.Path, "docs/") {
			continue
		}
		if row.Added == "-" {
			continue
		}
		added, errA := strconv.Atoi(row.Added)
		removed, errR := strconv.Atoi(row.Removed)
		if errA != nil || errR != nil {
			continue
		}
		total += added + removed
	}

	if total >= threshold {
		// Multi-line warning identical to Python click.secho block.
		// Python uses a Unicode "⚠" then two spaces — preserved.
		// The final "git commit --no-verify" hint is part of the same
		// stdout block; we replicate the linebreaks exactly.
		fmt.Fprintf(stdout, "⚠  %d lines changed without updating docs/decisions.md.\n", total)
		fmt.Fprintln(stdout, "   Log this decision: logmind log \"Your decision here\"")
		fmt.Fprintln(stdout, "   To skip this check: git commit --no-verify")
		if !noFail {
			return errSilentExit1
		}
		return nil
	}

	fmt.Fprintf(stdout, "✓ %d lines changed (below %d-line threshold).\n", total, threshold)
	return nil
}

// isDecisionFile mirrors the inline _is_decision_file predicate from
// cli.py:2469-2474. Branch-aware mode routes feature-branch logs
// to docs/decisions-branches/<branch>.md; both layouts must satisfy
// "documented".
func isDecisionFile(path string) bool {
	if path == "docs/decisions.md" {
		return true
	}
	if strings.HasSuffix(path, "/decisions.md") {
		return true
	}
	if strings.HasPrefix(path, "docs/decisions-branches/") {
		return true
	}
	return false
}
