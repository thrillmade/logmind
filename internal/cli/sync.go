package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/skill"
)

// newSyncCmd wires `logmind sync [--dry-run] [--since <duration>]
// [--update-provenance] [--write-drafts]` per SPEC §3.9.
//
// The Python v0.6.x line never shipped a `sync` command — the loop was
// closed by `clud-bug` itself writing the `.clud-bug.json` usage
// counters. The Go port (B5b / G4.a) routes the same signal through the
// more durable `docs/reviews/PR-*.md` files for its default (no-flag)
// behaviour, which stays exactly as it always has: fold review
// citations into each cited skill's (pre-existing, non-SPEC)
// PROVENANCE.md skeleton. That default is left untouched by this
// change — see runSync below.
//
// The three flags §3.9 lists beyond --dry-run are additive, opt-in code
// paths (internal/skill/sync.go's ParseSinceDuration / UpdateProvenance
// / WriteSkillDrafts):
//
//   - --since restricts the review-file scan (and, when combined with
//     the two flags below, the decision + usage-counter scans) to
//     entries newer than the given duration.
//   - --update-provenance refreshes each installed skill's
//     PROVENANCE.md into the SPEC §1.11.1 NORMATIVE format, sourced
//     from `.claude/skills/.clud-bug.json` usage counters and
//     docs/decisions*.md — genuinely new behaviour, not a rename of
//     the legacy path (see the doc comment on skill.UpdateProvenance
//     for why the two provenance surfaces stay separate).
//   - --write-drafts writes skill-candidate drafts to
//     docs/skills-derived/ per SPEC §1.9, reusing the same
//     decision-pattern heuristic `logmind skill suggest` uses.
func newSyncCmd() *cobra.Command {
	var dryRun bool
	var since string
	var updateProvenance bool
	var writeDrafts bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Reconcile clud-bug usage, decisions, and reviews into PROVENANCE.md / skill drafts",
		Long: `Walk docs/reviews/PR-*.md and update each cited skill's PROVENANCE.md.

For every PR review file (NORMATIVE format defined in SPEC §1.8.1), sync
parses the **Skills cited:** block and increments cited-by-clud-bug for
each skill listed, sets last-refined to today's date, and records the
review SHA in applied-review-shas so subsequent runs are idempotent.
This default behaviour never changes based on the flags below — it is
identical whether or not --since / --update-provenance / --write-drafts
are passed.

--since <duration> (e.g. "7d", "30d") limits the scan(s) to entries
newer than the given window.

--update-provenance additionally refreshes each installed skill's
PROVENANCE.md into the SPEC §1.11.1 format, reconciling
.claude/skills/.clud-bug.json usage counters with docs/decisions*.md.

--write-drafts additionally writes skill-candidate drafts under
docs/skills-derived/ (SPEC §1.9) from recent decision-log patterns.

Re-running with nothing new to fold in is a no-op — sync is safe to wire
into post-merge hooks or scheduled jobs.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			opts := SyncCLIOptions{
				DryRun:           dryRun,
				Since:            since,
				UpdateProvenance: updateProvenance,
				WriteDrafts:      writeDrafts,
			}
			return runSync(cwd, opts, time.Time{}, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Compute what sync would do but skip every write (composes with the other flags).")
	cmd.Flags().StringVar(&since, "since", "",
		`Limit the scan to entries newer than this duration (e.g. "7d", "30d"). Unset means unrestricted.`)
	cmd.Flags().BoolVar(&updateProvenance, "update-provenance", false,
		"Refresh each installed skill's PROVENANCE.md (SPEC §1.11.1) from clud-bug usage + docs/decisions*.md.")
	cmd.Flags().BoolVar(&writeDrafts, "write-drafts", false,
		"Write skill-candidate drafts to docs/skills-derived/ (SPEC §1.9) from recent decision patterns.")
	return cmd
}

// SyncCLIOptions bundles the flag values `logmind sync` was invoked
// with. Exists (rather than four positional bools/strings) so runSync's
// signature stays stable as flags are added.
type SyncCLIOptions struct {
	DryRun           bool
	Since            string
	UpdateProvenance bool
	WriteDrafts      bool
}

// runSync is the testable core of `logmind sync`. The `now` arg lets
// unit tests pin timestamps to a fixed instant; production callers pass
// time.Time{} so the skill package defaults to time.Now().
func runSync(cwd string, opts SyncCLIOptions, now time.Time, stdout, stderr io.Writer) error {
	warn := func(msg string) {
		fmt.Fprintln(stderr, "warn:", msg)
	}

	var since time.Duration
	if opts.Since != "" {
		d, err := skill.ParseSinceDuration(opts.Since)
		if err != nil {
			// Malformed --since is a hard error, not a silent
			// fall-through to "scan everything" — see
			// skill.ParseSinceDuration's doc comment.
			fmt.Fprintf(stdout, "Error: %v\n", err)
			return ErrSilent
		}
		since = d
	}

	summary, err := skill.Sync(cwd, skill.SyncOptions{
		DryRun: opts.DryRun,
		Now:    now,
		Warn:   warn,
		Since:  since,
	})
	if err != nil {
		// Filesystem-level failures (e.g., docs/reviews/ readable but
		// world-permission scramble) are real errors — surface them to
		// the user verbatim. The per-file/per-skill issues that route
		// through `warn` don't trip this path.
		fmt.Fprintf(stdout, "Error: %v\n", err)
		return ErrSilent
	}

	skill.FormatSummary(stdout, summary, opts.DryRun)
	// Prefix the machine-parseable `ok sync:` line with `(dry-run) ` so
	// consumers (e.g., post-merge hooks scraping stdout, CI summaries)
	// can distinguish a real run's counters from a dry-run preview's.
	// FormatSummary already carries the marker; this line gates it on
	// the same dryRun flag so the two surfaces agree. (Review #135 / Bug 2.)
	prefix := ""
	if opts.DryRun {
		prefix = "(dry-run) "
	}
	fmt.Fprintf(stdout, "ok sync: %s%d skill(s) updated · %d citation(s) added · %d/%d review(s) applied\n",
		prefix, summary.SkillsUpdated, summary.CitationsAdded,
		summary.ReviewsApplied, summary.ReviewsScanned)

	if opts.UpdateProvenance {
		provSummary, err := skill.UpdateProvenance(cwd, skill.ProvenanceOptions{
			DryRun: opts.DryRun,
			Now:    now,
			Since:  since,
			Warn:   warn,
		})
		if err != nil {
			fmt.Fprintf(stdout, "Error: %v\n", err)
			return ErrSilent
		}
		skill.FormatProvenanceSummary(stdout, provSummary, opts.DryRun)
	}

	if opts.WriteDrafts {
		draftSummary, err := skill.WriteSkillDrafts(cwd, skill.DraftOptions{
			DryRun: opts.DryRun,
			Now:    now,
			Since:  since,
			Warn:   warn,
		})
		if err != nil {
			fmt.Fprintf(stdout, "Error: %v\n", err)
			return ErrSilent
		}
		skill.FormatDraftSummary(stdout, draftSummary, opts.DryRun)
	}

	return nil
}
