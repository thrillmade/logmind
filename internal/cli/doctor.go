// doctor.go — `logmind doctor [--json] [--offline] [--exit-zero] [--fix]`
// subcommand.
//
// Thin shim over internal/doctor.CollectStatus + RenderStatus. The
// heavy lifting (PATH probe, workflow drift, hook marker scraping) is
// in the doctor package so other waves can reuse the same probes
// without depending on the cobra command surface.
//
// Exit semantics:
//   - overall == "DRIFT" + !--exit-zero → cobra returns ErrSilent so
//     main exits 1 without re-printing the rendered output.
//   - --fix applies remediation and exits 0 (residual PATH/version drift
//     and foreign hooks are warned, not failed); a hard write error during
//     --fix exits 1.
//   - All other paths return nil (exit 0).
//
// Output:
//   - Default: human-readable table to stdout.
//   - --json: report.ToJSON() (2-space indent) to stdout.
//   - --fix: a single quiet `ok doctor-fix …` line to stdout (or the
//     post-fix report JSON with --fix --json).
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/doctor"
)

func newDoctorCmd() *cobra.Command {
	var asJSON, offline, exitZero, fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report (and optionally --fix) installed versions and workflow drift",
		Long: "Report installed versions and workflow drift for logmind +\n" +
			"clud-bug across .github/workflows/, AGENTS.md, git hooks, and\n" +
			"merge-driver configuration. Read-only by default; exits non-zero\n" +
			"on drift so it's CI-pluggable.\n\n" +
			"With --fix, re-installs the drifted on-disk artifacts (workflows,\n" +
			"AGENTS.md block, .gitattributes, merge-driver config, git hooks) in\n" +
			"one idempotent pass. --fix never edits docs/ decision content,\n" +
			"foreign (hand-written) hooks, or your PATH.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if fix {
				return runDoctorFix(cmd, offline, asJSON)
			}
			report := doctor.CollectStatus("", offline)
			if asJSON {
				out, err := report.ToJSON()
				if err != nil {
					return err
				}
				cmd.Println(out)
			} else {
				cmd.Println(doctor.RenderStatus(report))
			}
			if report.Overall == "DRIFT" && !exitZero {
				return ErrSilent
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the report as JSON.")
	cmd.Flags().BoolVar(&offline, "offline", false, "Skip PyPI / npm probes; use only locally-readable signals.")
	cmd.Flags().BoolVar(&exitZero, "exit-zero", false, "Always exit 0, even on drift (for informational CI runs).")
	cmd.Flags().BoolVar(&fix, "fix", false, "Re-install drifted workflows, AGENTS.md block, .gitattributes, merge-driver config, and git hooks (idempotent). Never edits docs/ decisions, foreign hooks, or PATH.")
	return cmd
}

// runDoctorFix applies the idempotent remediation pass (applyRefresh) and
// reports what it changed. It exits 0 even when residual drift remains —
// PATH/version drift and foreign (markerless) hooks are surfaced as a
// warning, not a failure, because --fix is an action verb a human runs to
// clean a repo. A genuine write failure exits 1.
func runDoctorFix(cmd *cobra.Command, offline, asJSON bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	res, refreshErr := applyRefresh(cwd, refreshOpts{githubActions: true, git: true})
	if refreshErr != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: doctor --fix:", refreshErr)
		return ErrSilent
	}

	// Re-probe to compute the residual drift that --fix cannot address.
	after := doctor.CollectStatus(cwd, offline)
	residual := residualProbes(after)

	if asJSON {
		out, err := after.ToJSON()
		if err != nil {
			return err
		}
		cmd.Println(out)
		return nil
	}

	cmd.Println(formatDoctorFixOK(res, residual))
	for _, name := range residual {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"note: %q still drifted — not auto-fixable by `doctor --fix` "+
				"(PATH/version, or a hand-written hook). See `logmind doctor`.\n", name)
	}
	return nil
}

// residualProbes returns the names of probes still drifted after a fix pass:
// PATH/version drift and foreign (markerless) hooks, which --fix
// deliberately does not touch.
func residualProbes(r doctor.StatusReport) []string {
	var out []string
	for _, t := range r.Tools {
		for _, wf := range t.Workflows {
			if wf.Drift == "stale" || wf.Drift == "markerless" {
				out = append(out, wf.Name)
			}
		}
	}
	return out
}

// formatDoctorFixOK renders the single quiet `ok` summary line.
func formatDoctorFixOK(res refreshResult, residual []string) string {
	state := func(changed bool, changedWord string) string {
		if changed {
			return changedWord
		}
		return "current"
	}
	return fmt.Sprintf(
		"ok doctor-fix workflows=%d agents-md=%s gitattributes=%s merge-driver=%s hooks=%d residual=%d",
		len(res.WorkflowsCreated)+len(res.WorkflowsRefreshed),
		state(res.AgentsMDMsg != "", "changed"),
		state(res.GitattrChanged, "written"),
		state(res.MergeDriverSet, "set"),
		len(res.HooksRefreshed),
		len(residual),
	)
}
