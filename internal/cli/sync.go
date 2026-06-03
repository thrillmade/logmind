package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/skill"
)

// newSyncCmd wires `logmind sync [--dry-run]`. Per SPEC §3.9, sync is
// the loop-closer that turns clud-bug-review output (
// `docs/reviews/PR-<n>.md`) into incremental updates of each cited
// skill's `PROVENANCE.md`.
//
// The Python v0.6.x line never shipped a `sync` command — the loop
// was closed by `clud-bug` itself writing the `.clud-bug.json` usage
// counters. The Go port (B5b / G4.a) routes the same signal through
// the more durable `docs/reviews/PR-*.md` files so it survives
// `.clud-bug.json` schema migrations, works for the GitHub-App
// variant of clud-bug (no on-disk JSON edit), and stays grep-friendly
// for humans reading their own repo. See SPEC §6.5 — sync MUST read
// from local files; no GitHub API call.
//
// SPEC §3.9 also lists `--since`, `--update-provenance`, and
// `--write-drafts`. This Go port ships `--dry-run` first; the other
// three are tracked for later waves (see PR description). Provenance
// update is the default (and currently only) behaviour, so an
// `--update-provenance` flag here would be vacuous until the
// `--write-drafts` path lands.
func newSyncCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fold clud-bug review citations into each cited skill's PROVENANCE.md",
		Long: `Walk docs/reviews/PR-*.md and update each cited skill's PROVENANCE.md.

For every PR review file (NORMATIVE format defined in SPEC §1.8.1), sync
parses the **Skills cited:** block and increments cited-by-clud-bug for
each skill listed, sets last-refined to today's date, and records the
review SHA in applied-review-shas so subsequent runs are idempotent.

Re-running with no new review files is a no-op — sync is safe to wire
into post-merge hooks or scheduled jobs.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSync(cwd, dryRun, time.Time{}, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Parse + aggregate citations but skip the PROVENANCE.md writes.")
	return cmd
}

// runSync is the testable core of `logmind sync`. The `now` arg lets
// unit tests pin `last-refined` to a fixed instant; production callers
// pass time.Time{} so skill.Sync defaults to time.Now().
func runSync(cwd string, dryRun bool, now time.Time, stdout, stderr io.Writer) error {
	warn := func(msg string) {
		fmt.Fprintln(stderr, "warn:", msg)
	}
	summary, err := skill.Sync(cwd, skill.SyncOptions{
		DryRun: dryRun,
		Now:    now,
		Warn:   warn,
	})
	if err != nil {
		// Filesystem-level failures (e.g., docs/reviews/ readable but
		// world-permission scramble) are real errors — surface them to
		// the user verbatim. The per-file/per-skill issues that route
		// through `warn` don't trip this path.
		fmt.Fprintf(stdout, "Error: %v\n", err)
		return ErrSilent
	}

	skill.FormatSummary(stdout, summary, dryRun)
	// Prefix the machine-parseable `ok sync:` line with `(dry-run) ` so
	// consumers (e.g., post-merge hooks scraping stdout, CI summaries)
	// can distinguish a real run's counters from a dry-run preview's.
	// FormatSummary already carries the marker; this line gates it on
	// the same dryRun flag so the two surfaces agree. (Review #135 / Bug 2.)
	prefix := ""
	if dryRun {
		prefix = "(dry-run) "
	}
	fmt.Fprintf(stdout, "ok sync: %s%d skill(s) updated · %d citation(s) added · %d/%d review(s) applied\n",
		prefix, summary.SkillsUpdated, summary.CitationsAdded,
		summary.ReviewsApplied, summary.ReviewsScanned)
	return nil
}
