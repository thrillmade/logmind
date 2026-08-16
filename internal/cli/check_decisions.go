package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/gitcli"
	"github.com/thrillmade/logmind/internal/guardcommit"
)

// newCheckDecisionsCmd wires
// `logmind check-decisions [--threshold N] [--no-fail] [--base R --head R]`.
//
// This is SPEC §3.4's THIRD interception point — the `check-decisions`
// gate of §6.2, "the one that actually blocks a merge." It judges the
// same rule as the two local points (the commit-msg hook and the harness
// hook, both served by `logmind guard-commit`) and shares their
// evaluation: guardcommit.IsExcludedPath / SubstantiveLines decide what
// counts, here as there. §3.4: "Both interception points and the gate
// MUST use this one list."
//
// What the gate does NOT share is the local allowances. §3.4: every
// allowance "that depends on local process state — the environment
// variable, a git operation in progress, running outside a repository, a
// marker in a commit subject — is invisible to it and MUST NOT be
// honoured there. Exactly two things clear the gate: the change carries a
// decision, or it falls under the threshold." So there is deliberately no
// [skip-logmind] arm and no LOGMIND_ALLOW_GIT_COMMIT arm below.
//
// Output strings on the staged path are byte-identical to Python
// v0.6.14 (cli.py:2425-2519):
//
//	not-a-git-repo:      "Not a git repository, skipping check."  (exit 0)
//	decision written:    "✓ A decision log file is staged — changes are documented."
//	under threshold:     "✓ N lines changed (below T-line threshold)."  (exit 0)
//	over threshold:      multi-line warning, exit 1 unless --no-fail
func newCheckDecisionsCmd() *cobra.Command {
	var opts checkDecisionsOpts
	cmd := &cobra.Command{
		Use:   "check-decisions",
		Short: "Check that significant code changes have corresponding decision logs",
		Long: `Check that significant code changes have corresponding decision logs.

Evaluates the staged index by default, which is what a git pre-commit
hook wants. Pass --base and --head together to evaluate a commit range
instead — that is the CI shape, judging a pull request's diff against
its base ref.

Exits with code 1 when the change exceeds the line threshold without
adding a well-formed decision entry. The threshold comes from
git.commit_line_threshold in .logmind/config.yml (default 20);
--threshold overrides it.

To install as a pre-commit hook, run: logmind install-hook

Examples:
    logmind check-decisions
    logmind check-decisions --threshold 50
    logmind check-decisions --no-fail
    logmind check-decisions --base origin/main --head HEAD`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			opts.cwd = cwd
			opts.thresholdExplicit = cmd.Flags().Changed("threshold")
			return runCheckDecisions(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVarP(&opts.threshold, "threshold", "t", 20,
		"Minimum lines changed to require a decision log entry (default: git.commit_line_threshold, or 20)")
	cmd.Flags().BoolVar(&opts.noFail, "no-fail", false,
		"Warn but exit with code 0 (don't block the commit)")
	cmd.Flags().StringVar(&opts.base, "base", "",
		"Base ref of the range to evaluate (requires --head; default: evaluate the staged index)")
	cmd.Flags().StringVar(&opts.head, "head", "",
		"Head ref of the range to evaluate (requires --base)")
	return cmd
}

// checkDecisionsOpts is runCheckDecisions' input. A struct rather than a
// parameter list because base/head/threshold-explicitness pushed the
// positional form past the point of being readable at a call site.
type checkDecisionsOpts struct {
	// cwd is the directory to evaluate from — resolved to the repo
	// toplevel once inside runCheckDecisions and used for BOTH config and
	// every git command, per SPEC §3.4's "an engine MUST resolve the
	// repository root once and use that one answer."
	cwd string
	// threshold is the --threshold flag's value; thresholdExplicit says
	// whether the user actually passed it. Unset means config decides.
	threshold         int
	thresholdExplicit bool
	// noFail downgrades a failing check to a warning + exit 0.
	noFail bool
	// base and head select range mode. Both or neither.
	base, head string
}

// runCheckDecisions is the testable core. Returns ErrSilent to
// trigger exit 1 without cobra re-printing the message — mirrors
// the Python sys.exit(1) shape.
func runCheckDecisions(opts checkDecisionsOpts, stdout io.Writer) error {
	rangeMode := opts.base != "" || opts.head != ""
	if rangeMode && (opts.base == "" || opts.head == "") {
		return fmt.Errorf("--base and --head must be given together")
	}

	// §3.4 again: exactly two things clear the gate. --no-fail is a third,
	// and although it is set by whoever wrote the workflow rather than by
	// the pull request's author, a repository that adds it has a gate that
	// reports and never blocks. Refuse the combination outright so the
	// escape cannot be added quietly to a workflow file.
	if rangeMode && opts.noFail {
		return fmt.Errorf("--no-fail cannot be combined with --base/--head: SPEC §3.4 allows exactly two things to clear the gate — the change carries a decision, or it falls under the threshold")
	}

	// --threshold is the fourth, by exactly the same argument, and it is
	// worse than --no-fail because it does not look like an escape:
	// `--threshold 999999` reads as configuration. §3.4 pins the gate's
	// threshold to `git.commit_line_threshold` and to nothing else, so in
	// range mode the flag has no legitimate use — the repository already
	// has a way to say what its threshold is, read from the base ref
	// precisely so the change under judgement cannot forge it.
	//
	// Refused rather than ignored: silently discarding a flag someone
	// passed deliberately is its own way of lying about what ran.
	if rangeMode && opts.thresholdExplicit {
		return fmt.Errorf("--threshold cannot be combined with --base/--head: SPEC §3.4 pins the gate's threshold to git.commit_line_threshold; set it in the repository's .logmind/config.yml instead")
	}

	// Running outside a repository is one of §3.4's SIX local allowances,
	// and like the other five it MUST NOT reach the gate: "every allowance
	// above that depends on local process state — the environment
	// variable, a git operation in progress, running outside a repository,
	// a marker in a commit subject — is invisible to it and MUST NOT be
	// honoured there. Exactly two things clear the gate."
	//
	// In range mode this is the gate, so a missing repository is a hard
	// error rather than a pass. Otherwise a workflow that resolves its
	// working directory wrongly — `working-directory:` pointing off the
	// checkout, or `actions/checkout` with a `path:` — turns every PR
	// green regardless of size, and reports success while doing it.
	if !gitcli.IsRepo(opts.cwd) {
		if rangeMode {
			return fmt.Errorf("not a git repository: %s (the gate cannot evaluate a range outside a repository; check the job's working directory)", opts.cwd)
		}
		// Local path only. Python: click.echo(...) to stdout, no exit.
		fmt.Fprintln(stdout, "Not a git repository, skipping check.")
		return nil
	}

	// One repository root for the config read AND every git command
	// below. Resolving them separately is SPEC §3.4's named silent
	// bypass: "configuration from where the process started, diffs from
	// where the command ran — silently waves through every commit made
	// from a subdirectory."
	repoRoot := opts.cwd
	if toplevel, ok := gitcli.TopLevel(opts.cwd); ok {
		repoRoot = toplevel
	}
	cfg, _ := config.Load(repoRoot)
	threshold := resolveThreshold(cfg, opts.threshold, opts.thresholdExplicit)

	names, rows, err := collectCheckDiff(repoRoot, opts)
	if err != nil {
		return err
	}

	// SPEC §3.4: "A decision clears the gate by being written, not by
	// existing. ... MUST NOT be satisfied by the decision file merely
	// appearing in the diff." So we read what the change ADDED to each
	// decision file and require a §3.1-shaped entry in it — a title, a
	// timestamp, and non-empty reasoning. A touched-but-empty decision
	// file falls through to the line count below, exactly as if it had
	// not been touched.
	for _, f := range names {
		if !guardcommit.IsDecisionFile(f) {
			continue
		}
		added, err := addedLines(repoRoot, f, opts)
		if err != nil {
			return err
		}
		if guardcommit.WellFormedDecisionAdded(added) {
			fmt.Fprintln(stdout, "✓ A decision log file is staged — changes are documented.")
			return nil
		}
	}

	// The exclusion table and the summing live in
	// guardcommit.SubstantiveLines — shared with `logmind guard-commit`
	// so the gate and the two local interception points can never drift
	// on what counts as "substantive" (SPEC §3.4).
	total := guardcommit.SubstantiveLines(rows)

	if total >= threshold {
		// Multi-line warning identical to Python click.secho block.
		// Python uses a Unicode "⚠" then two spaces — preserved.
		// The final "git commit --no-verify" hint is part of the same
		// stdout block; we replicate the linebreaks exactly.
		// Name the file the branch-aware write path actually produces, not
		// the legacy one. guardcommit.IsDecisionFile — the gate this message
		// explains — accepts docs/decisions-branches/*.md, and telling the
		// user to update docs/decisions.md sent them at a file that would not
		// have cleared the check. The CI-shape twin
		// (check-decisions.yml.template) already says it this way.
		fmt.Fprintf(stdout, "⚠  %d lines changed without updating docs/decisions-branches/<branch>.md.\n", total)
		fmt.Fprintln(stdout, "   Log this decision: logmind log \"Your decision here\"")
		fmt.Fprintln(stdout, "   To skip this check: git commit --no-verify")
		if !opts.noFail {
			return ErrSilent
		}
		return nil
	}

	fmt.Fprintf(stdout, "✓ %d lines changed (below %d-line threshold).\n", total, threshold)
	return nil
}

// collectCheckDiff reads the changed paths and their numstat rows for
// whichever scope opts selects: the staged index (the pre-commit hook
// shape, and the default) or a base...head range (the CI shape).
//
// Range mode propagates git's error instead of degrading to an empty
// diff. An unresolvable ref and an empty range produce the same zero
// rows, and only one of them should let a pull request through.
func collectCheckDiff(repoRoot string, opts checkDecisionsOpts) ([]string, []gitcli.NumstatLine, error) {
	if opts.base == "" {
		return gitcli.DiffCachedNames(repoRoot), gitcli.DiffCachedNumstat(repoRoot), nil
	}
	names, err := gitcli.DiffRangeNames(repoRoot, opts.base, opts.head)
	if err != nil {
		return nil, nil, err
	}
	rows, err := gitcli.DiffRangeNumstat(repoRoot, opts.base, opts.head)
	if err != nil {
		return nil, nil, err
	}
	return names, rows, nil
}

// addedLines returns the lines opts' scope ADDED to path, for the
// well-formedness check above.
func addedLines(repoRoot, path string, opts checkDecisionsOpts) ([]string, error) {
	if opts.base == "" {
		return gitcli.DiffCachedAddedLines(repoRoot, path), nil
	}
	return gitcli.DiffRangeAddedLines(repoRoot, opts.base, opts.head, path)
}
