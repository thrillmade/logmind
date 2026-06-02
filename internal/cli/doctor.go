// doctor.go — `logmind doctor [--json] [--offline] [--exit-zero]`
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
//   - All other paths return nil (exit 0).
//
// Output:
//   - Default: human-readable table to stdout.
//   - --json: report.ToJSON() (2-space indent) to stdout.
//
// Network:
//   - Default: best-effort PyPI probe with 2-second timeout. Failures
//     degrade to "?" in the latest column.
//   - --offline: skip the probe entirely; latest column reads
//     "(offline)".
package cli

import (
	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/doctor"
)

func newDoctorCmd() *cobra.Command {
	var asJSON, offline, exitZero bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report installed versions and workflow drift",
		Long: "Report installed versions and workflow drift for logmind +\n" +
			"clud-bug across .github/workflows/, AGENTS.md, git hooks, and\n" +
			"merge-driver configuration. Read-only; exits non-zero on drift\n" +
			"so it's CI-pluggable.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
	return cmd
}
