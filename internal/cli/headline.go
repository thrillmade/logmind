// headline.go — `logmind headline <summary>` subcommand.
//
// Sets the branch's one-sentence "headline" summary: the plain-English line
// the main-canonical timeline shows for this branch (the §1.6.3 marker at the
// top of docs/decisions-branches/<branch>.md). The timeline copies it
// verbatim; the next agent reads it first. The <date>-<slug> KEY stays stable
// — only the visible sentence is refined (logmind makes no LLM call; the agent
// authors the sentence). See also `logmind log --headline` (the bundled form)
// and the per-log nudge in log.go.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/timeline"
)

func newHeadlineCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "headline <summary>",
		Short: "Set the one-sentence branch summary shown at the top of the timeline",
		Long: `Set the branch's one-sentence summary — the "headline" the main-canonical
timeline shows for this branch, and the first thing the next agent reads.

Write a plain-English sentence describing what the WHOLE branch did (not just
one decision). It's stored in the §1.6.3 marker at the top of the branch's
decision file and copied verbatim into docs/timeline.md. The <date>-<slug>
key stays stable — only the sentence changes, so you can refine it as the
branch grows.

Applies on every branch, the default branch included: main's decisions live in
its own branch file like every other branch's (SPEC §3.2), and that file
carries a marker and a timeline row like every other branch's too. Identical
to the -H/--headline flag on 'logmind log'.
Set LOGMIND_PR to append a (#NN) suffix to the visible line.

Example:
    logmind headline "Added JWT session auth with refresh-token rotation"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runHeadline(cwd, args[0], file, quietEnabled(cmd), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&file, "file", "",
		"Target a specific branch decision file (e.g. docs/decisions-branches/feat__x.md) instead of the current branch — for summarizing old/merged branches.")
	return cmd
}

func runHeadline(cwd, summary, fileOverride string, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		q.fail("Error: headline summary is empty.\n")
		return ErrSilent
	}
	cfg, _ := config.Load(cwd)
	layout := decisions.ResolveLayout(cwd)

	var target string
	if fileOverride != "" {
		// Explicit file (e.g. an old/merged branch) — skip git-branch resolution.
		target = fileOverride
		if !filepath.IsAbs(target) {
			target = filepath.Join(cwd, target)
		}
		target = filepath.Clean(target)
		// Only branch detail files carry timeline markers — refuse to splice a
		// marker into decisions.md/archive or anything outside the branches dir.
		if filepath.Dir(target) != filepath.Join(layout.Dir(), decisions.BranchDirName) {
			q.fail("--file must target a file under docs/decisions-branches/ (got %s).\n", fileOverride)
			return ErrSilent
		}
		if !pathExists(target) {
			q.fail("No such branch file: %s\n", fileOverride)
			return ErrSilent
		}
	} else {
		t, isBranchFile := resolveDecisionsPath(cwd, layout, cfg)
		if !branchSummaryApplies(isBranchFile) {
			// Not a default-branch refusal — the default branch gets a summary
			// like every other branch. This is the case where `logmind log`
			// itself would not write a branch file, so there is no §1.6.3
			// marker to set a headline in: branch_aware off, non-git, or a
			// detached HEAD. Use --file to target a branch file directly.
			q.chat("Branch summaries live in a branch decision file, and `logmind log` would not write one here (detached HEAD, not a git repo, or decisions.branch_aware is off). Use --file docs/decisions-branches/<branch>.md to target one directly.\n")
			if quiet {
				q.ok("headline state=skipped reason=no-branch-file")
			}
			return nil
		}
		if !pathExists(t) {
			q.chat("No decisions logged on this branch yet — run `logmind log` first, then set the summary.\n")
			if quiet {
				q.ok("headline state=skipped reason=no-decisions")
			}
			return nil
		}
		target = t
	}
	return setHeadlineInFile(cwd, target, summary, q)
}

// setHeadlineInFile sets/refreshes the §1.6.3 marker headline in the given
// branch file: it rewrites the visible line of an existing marker (keeping the
// stable <date>-<slug> key), or inserts a marker if the file has none. Shared
// by `logmind headline` (current branch + --file). The caller ensures the
// target is a branch detail file (markers belong only there).
func setHeadlineInFile(cwd, target, summary string, q qout) error {
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read %s: %w", target, err)
	}
	content := string(data)
	prSuffix := prSuffixFromEnv()

	updated, replaced := timeline.ReplaceFirstHeadline(content, summary, prSuffix)
	if !replaced {
		if timeline.HasEntryBlocks(content) {
			// A marker exists but couldn't be rewritten (unclosed/malformed) —
			// don't stack a second one. logmind never writes such a marker, so
			// this only fires on a hand-corrupted file.
			q.fail("Branch file has a malformed timeline marker — fix it manually, then retry.\n")
			return ErrSilent
		}
		// Markerless (pre-opt-in) branch file — insert a marker now.
		updated = string(insertMarkerAfterHeader([]byte(content), buildTimelineMarker(time.Now(), summary, prSuffix)))
	}
	rel := relForOk(cwd, target)
	if updated == content {
		q.chat("  branch summary already up to date\n")
		if q.quiet {
			q.ok("headline path=%s state=unchanged", rel)
		} else {
			fmt.Fprintf(q.stdout, "ok headline: %s (unchanged)\n", rel)
		}
		return nil
	}
	if err := writeAtomic(target, updated); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	q.chat("✓ Branch summary set: %s\n", summary)
	if q.quiet {
		q.ok("headline path=%s state=set", rel)
	} else {
		fmt.Fprintf(q.stdout, "ok headline: %s\n", rel)
	}
	return nil
}

// relForOk renders target relative to cwd in posix form for the `ok` trailer,
// falling back to the absolute path if the relativise fails.
func relForOk(cwd, target string) string {
	rel, err := filepath.Rel(cwd, target)
	if err != nil {
		return target
	}
	return filepath.ToSlash(rel)
}

// branchSummaryApplies reports whether a one-sentence branch summary CAN be
// set where `logmind log` would write right now. The two ways to set one share
// this single answer so they cannot disagree: `logmind headline <summary>` and
// `logmind log --headline`'s marker write.
//
// Distinct from branchSummaryNudgeApplies, which asks whether logmind should
// PROMPT for one unasked. "Can the user set this" and "should we interrupt to
// ask for it" are different questions and collapsing them is what produced the
// bug below — the prompt's reasonable reticence about main leaked onto the
// command whose entire job is to set the thing.
//
// The answer is "wherever a branch file is written", with no default-branch
// exception, because the §1.6.3 marker the summary lives in has none. As of
// v2.0.0 main-canonical is the sole timeline model: log.go writes a marker
// into EVERY branch file it creates, main.md included, and timeline.Generate
// renders a row from each. So main.md already carries a headline — seeded
// from the first decision's summary — and it is already rendered into
// docs/timeline.md.
//
// This function used to AND in onNonDefaultBranch, on the argument that main
// has no in-flight unit of work for one sentence to describe. The argument
// does not survive what the code does: the sentence is being written on main
// either way, `logmind log -H` overwrites it happily, and doctor's
// placeholder-summary report names main.md and tells the reader to enrich it.
// The gate only ever stopped the ONE command named after the job from doing
// it — `logmind headline "x"` refused on main and left the seeded placeholder
// standing, while `logmind log … -H "x"` two lines later replaced it. The
// shipped AGENTS.md block presents them as interchangeable forms of one
// operation ("It applies on every branch — the default branch is a branch
// like any other"), which is now true of both.
//
// It remains NOT a decision-routing rule. SPEC §3.2 has exactly one of those
// and this is not it (resolveDecisionsPath). The parameter is the routing
// decision's OUTPUT: a summary belongs where a branch file does, so the two
// answers move together by construction.
func branchSummaryApplies(isBranchFile bool) bool {
	return isBranchFile
}

// branchSummaryNudgeApplies reports whether `logmind log` should PROMPT for a
// branch summary it was not given — the unasked-for interruption, not the
// capability.
//
// Everything branchSummaryApplies requires, plus a non-default branch. The
// summary captions a bounded unit of work: the branch this PR is about, from
// its first decision to its merge. The default branch has no such unit — main
// is permanent and its file is never "the branch this PR is about" — so
// prompting "summarise the whole branch" after every direct-to-main log asks
// for a sentence nobody can write, on a cadence nobody asked for.
//
// That reticence is right for a PROMPT and wrong for a COMMAND, which is the
// whole distinction this pair of functions exists to hold. main.md carries a
// §1.6.3 marker like every other branch file (log.go writes one on creation)
// and timeline.Generate renders a row from it, so the sentence exists on main
// whether or not anyone is asked for it — seeded from the first decision's
// summary. `logmind headline` and `logmind log -H` must therefore be able to
// refine it there; only the unprompted nudge stays quiet.
func branchSummaryNudgeApplies(cwd string, isBranchFile bool) bool {
	return branchSummaryApplies(isBranchFile) && onNonDefaultBranch(cwd)
}
