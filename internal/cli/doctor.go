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
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/doctor"
	"github.com/thrillmade/logmind/internal/timeline"
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
			"one idempotent pass, and backfills a §1.6.3 timeline marker into any\n" +
			"markerless branch detail file. --fix never rewrites decision text,\n" +
			"foreign (hand-written) hooks, or your PATH.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			quiet := quietEnabled(cmd)
			if fix {
				// --fix is already quiet-shaped: it emits exactly one
				// `ok doctor-fix …` line to stdout and routes residual notes to
				// stderr, so it satisfies the QUIET discipline in both modes.
				return runDoctorFix(cmd, offline, asJSON)
			}
			report := doctor.CollectStatus("", offline)
			switch {
			case asJSON:
				// --json is an explicit format request that wins over quiet's
				// terse line — an agent that asked for the full JSON report
				// gets it.
				out, err := report.ToJSON()
				if err != nil {
					return err
				}
				cmd.Println(out)
			case quiet:
				// Suppress the human table; emit the single chainable receipt.
				fmt.Fprintf(cmd.OutOrStdout(), "ok doctor overall=%s drift=%d\n",
					report.Overall, driftCount(report))
			default:
				cmd.Println(doctor.RenderStatus(report))
			}
			if report.Overall == "DRIFT" && !exitZero {
				return ErrSilent
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the report as JSON.")
	cmd.Flags().BoolVar(&offline, "offline", false, "No-op: doctor makes no network calls (kept for backward compatibility).")
	cmd.Flags().BoolVar(&exitZero, "exit-zero", false, "Always exit 0, even on drift (for informational CI runs).")
	cmd.Flags().BoolVar(&fix, "fix", false, "Re-install drifted workflows, AGENTS.md block, .gitattributes, merge-driver config, and git hooks (idempotent); and (main-canonical) backfill markers into markerless branch files. Never rewrites decision text, foreign hooks, or PATH.")
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
	res, refreshErr := applyRefresh(cwd, refreshOpts{
		githubActions:      true,
		git:                true,
		claudeAgentEnabled: claudeAgentEnabledFromConfig(cwd),
	})
	if refreshErr != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: doctor --fix:", refreshErr)
		return ErrSilent
	}

	// Backfill the §1.6.3 timeline marker into any markerless branch detail
	// file (main-canonical only) — the deterministic structural half of the
	// branch-summary migration. The rich one-sentence summary stays the
	// agent's job (doctor's advisory lists the placeholders to enrich).
	summariesBackfilled := backfillBranchSummaries(cwd, cmd.ErrOrStderr())

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

	cmd.Println(formatDoctorFixOK(res, residual, summariesBackfilled))
	for _, name := range residual {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"note: %q still drifted — not auto-fixable by `doctor --fix` "+
				"(PATH/version, or a hand-written hook). See `logmind doctor`.\n", name)
	}
	return nil
}

// driftCount counts probe rows that are "stale" — the drift signal that flips
// Overall to DRIFT (see doctor.CollectStatus). "missing" (benign in a fresh
// repo) and "markerless" (a main-canonical migration advisory, not a hard
// drift) are excluded so the QUIET `ok doctor …` receipt stays consistent
// with `overall` (e.g. overall=OK never pairs with drift>0).
func driftCount(r doctor.StatusReport) int {
	n := 0
	for _, t := range r.Tools {
		for _, wf := range t.Workflows {
			if wf.Drift == "stale" {
				n++
			}
		}
	}
	return n
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

// claudeAgentEnabledFromConfig resolves whether Layer 1 of commit
// enforcement (the Claude Code harness's PreToolUse guard; see
// internal/claudehook) should be installed/refreshed for this repo.
// Mirrors agents.DefaultEnabled's "claude" default (enabled) — a repo
// with no `agents.claude` key at all, or an unparseable config, gets the
// same default-true behavior config.LoadAsMap already applies. Only an
// EXPLICIT `agents.claude: false` opts a repo out.
func claudeAgentEnabledFromConfig(cwd string) bool {
	m, err := config.LoadAsMap(cwd)
	if err != nil {
		return true
	}
	v, ok := config.GetPath(m, "agents.claude")
	if !ok {
		return true
	}
	b, ok := v.(bool)
	if !ok {
		return true
	}
	return b
}

// formatDoctorFixOK renders the single quiet `ok` summary line.
func formatDoctorFixOK(res refreshResult, residual []string, summariesBackfilled int) string {
	state := func(changed bool, changedWord string) string {
		if changed {
			return changedWord
		}
		return "current"
	}
	return fmt.Sprintf(
		"ok doctor-fix workflows=%d agents-md=%s gitattributes=%s merge-driver=%s hooks=%d claude-hook=%s summaries-backfilled=%d residual=%d",
		len(res.WorkflowsCreated)+len(res.WorkflowsRefreshed),
		state(res.AgentsMDMsg != "", "changed"),
		state(res.GitattrChanged, "written"),
		state(res.MergeDriverSet, "set"),
		len(res.HooksRefreshed),
		state(res.ClaudeHookChanged, "changed"),
		summariesBackfilled,
		len(residual),
	)
}

// backfillBranchSummaries inserts the deterministic §1.6.3 marker (first-
// decision-title headline) into every markerless branch detail file — the
// structural half of the branch-summary migration. Runs unconditionally
// (main-canonical is the sole timeline as of v2.0.0); returns the count
// fixed. Best-effort: a per-file read/write error skips that file. Reuses
// the cli-local marker helpers (buildTimelineMarker / insertMarkerAfterHeader),
// so no export is needed.
func backfillBranchSummaries(cwd string, stderr io.Writer) int {
	files, err := decisions.ListBranchFiles(filepath.Join(cwd, "docs", "decisions-branches"))
	if err != nil {
		return 0
	}
	n := 0
	for _, bf := range files {
		data, err := os.ReadFile(bf)
		if err != nil {
			continue
		}
		content := string(data)
		if timeline.HasEntryBlocks(content) {
			continue // already has a marker
		}
		entries, _ := decisions.Iter(bf, io.Discard)
		if len(entries) == 0 {
			continue // empty file — nothing to summarize
		}
		marker := buildTimelineMarker(entries[0].Date, entries[0].Title, prSuffixFromEnv())
		updated := string(insertMarkerAfterHeader([]byte(content), marker))
		if err := writeAtomic(bf, updated); err != nil {
			// Best-effort: surface the failure so a partial --fix is not
			// silently indistinguishable from a clean no-op.
			fmt.Fprintf(stderr, "note: could not backfill %s: %v\n", bf, err)
			continue
		}
		n++
	}
	return n
}
