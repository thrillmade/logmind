package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/gitcli"
)

func newWarpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "warp",
		Short: "Refresh docs/timeline.md + docs/file-structure.md from main (read-only catch-up + repair)",
		Long: `Fetch the latest default branch and refresh your working copy of the two
DERIVED docs (docs/timeline.md, docs/file-structure.md) so your context reflects
main's current decisions.

Never commits: warp does not create a commit or move HEAD. They are
regenerated on main only; a branch must keep them byte-identical to its
merge-base with main so that merges never conflict. Your branch's own
decisions live in docs/decisions-branches/<branch>.md (committed, per-branch,
conflict-free).

In a repo that has opted into ` + "`derived_docs: {mode: integration-point}`" + ` (see
'logmind doctor'), warp is also the repair surface for a branch that has
ALREADY diverged from that invariant (e.g. an old binary's local regen, or a
hand edit, landed in a past commit): after fetching, it restores both files
to their merge-base-with-default content in both the index and the working
tree. logmind log's own restore (and the pre-commit / harness guards) target
HEAD instead — cheap and offline, but unable to repair a divergence that
already happened — precisely because warp is the one surface that fetches
first, giving it a trustworthy, current origin/<default> to compute a
merge-base against.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runWarp(cwd, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func runWarp(cwd string, stdout, stderr io.Writer) error {
	if !gitcli.IsRepo(cwd) {
		fmt.Fprintln(stdout, "Not a git repo — nothing to warp.")
		return nil
	}
	def := gitcli.DefaultBranch(cwd)
	if def == "" {
		def = "main"
	}
	if err := gitcli.Fetch(cwd, "origin", def); err != nil {
		fmt.Fprintf(stderr, "warn: git fetch origin %s failed: %v (refreshing from last-known main)\n", def, err)
	}
	ref := "origin/" + def
	refreshed := 0
	for _, rel := range derivedDocPaths {
		content, ok := gitcli.ShowFile(cwd, ref, rel)
		if !ok {
			continue
		}
		if err := writeAtomic(filepath.Join(cwd, rel), content); err != nil {
			fmt.Fprintf(stderr, "warn: could not write %s: %v\n", rel, err)
			continue
		}
		refreshed++
	}

	// Repair step (v2.0.0 4b-ter): the merge-base restore that USED to live
	// on the commit-path surfaces (logmind log's L1, the pre-commit hook's
	// L2a, the harness's L2b) moved here — see those surfaces' doc comments
	// for why. warp is the ONE surface with a just-fetched origin/<default>
	// in hand (the Fetch call above), so it's the only place that can
	// compute a TRUSTWORTHY merge-base and self-heal a branch whose HEAD
	// already carries a diverged copy of these two files.
	//
	// Gated on integrationPointMode: driver-mode repos (the default) have no
	// merge-base invariant to repair, so this step is a pure no-op there —
	// warp keeps its original, unconditional read-refresh-from-origin's-tip
	// behavior for those repos, unchanged.
	//
	// Interaction with the read-refresh loop above: that loop just wrote
	// origin/<default>'s TIP content (freshest — good for a human/agent
	// reading current context) into the working tree. The merge-base can be
	// an ANCESTOR of that tip (whenever origin has advanced past this
	// branch's fork point), so the two can disagree. Where they do, this
	// step's write runs SECOND and wins: the repaired, invariant-correct
	// (merge-base) content is what's left on disk and staged, at the cost of
	// no longer showing the human the very latest main content for these two
	// files. That's a deliberate trade — a branch that can't merge cleanly
	// is worse than a local timeline copy that's one merge-base behind — see
	// warp's --help text and the CTO ruling this cites in the PR history.
	//
	// Restores BOTH the index and the working tree (gitcli.RestorePathsToRef
	// is `git checkout <ref> -- <path>`, the same primitive the commit-path
	// surfaces use) — narrower than a `git add -A` (it only ever touches
	// these two known, already-tracked paths) and never creates a commit, so
	// warp's "never commits" contract holds; see TestWarp_DoesNotCommit.
	repaired := false
	if onNonDefaultBranch(cwd) && integrationPointMode(cwd) {
		mergeBase := gitcli.DefaultBranchMergeBase(cwd)
		// RestorePathsToRef is per-path best-effort: it attempts EVERY path
		// in derivedDocPaths regardless of an earlier one erroring, and
		// returns only the FIRST error (if any) purely for logging. A path
		// untracked at mergeBase (e.g. a repo that added one derived doc
		// more recently than the other) is a normal, partial outcome, not a
		// reason to call the whole repair a failure — so `repaired` reports
		// "the repair ran", not "every path resolved cleanly", matching the
		// fully-silent, fully-best-effort stance every other restore call
		// site in this codebase (L1, L2a, L2b) already takes.
		if err := gitcli.RestorePathsToRef(cwd, mergeBase, derivedDocPaths...); err != nil {
			fmt.Fprintf(stderr, "warn: repair to merge-base %s had a partial error (expected if a path doesn't exist there yet): %v\n", mergeBase, err)
		}
		repaired = true
	}

	ahead := ""
	if out, _, err := gitcli.RunCaptured(cwd, "rev-list", "--count", "HEAD.."+ref, "--",
		"docs/decisions.md", "docs/decisions-branches", "docs/decisions-archive.md"); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(out)); e == nil && n > 0 {
			ahead = fmt.Sprintf(" · main is +%d decision commit(s) ahead", n)
		}
	}
	repairNote := ""
	if repaired {
		repairNote = " · repaired derived doc(s) to merge-base (zero-conflict invariant)"
	}
	fmt.Fprintf(stdout, "ok warp: refreshed %d derived doc(s) from %s (read-only — not committed)%s%s\n", refreshed, ref, ahead, repairNote)
	return nil
}
