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
		Short: "Refresh the derived docs from main (read-only catch-up + repair)",
		Long: `Fetch the latest default branch and refresh your working copy of the
DERIVED docs (docs/timeline.md, docs/timeline-archive.md,
docs/file-structure.md) so your context reflects main's current decisions.

Never commits: warp does not create a commit or move HEAD. They are
regenerated on main only; a branch must keep them byte-identical to its
merge-base with main so that merges never conflict. Your branch's own
decisions live in docs/decisions-branches/<branch>.md (committed, per-branch,
conflict-free).

warp is also the repair surface for a branch that has ALREADY diverged from
the zero-conflict invariant (e.g. an old binary's local regen, or a hand
edit, landed in a past commit): after fetching, it restores those files to
their merge-base-with-default content in BOTH the index and the working
tree — i.e. it deliberately STAGES the repair. That's not a side effect to
apologize for: staging is what lets the repair ride into your NEXT commit
instead of silently vanishing. 'logmind log' recognizes an already-staged
derived doc as YOUR deliberate fix and leaves it alone (rather than
reverting it back to the branch's own possibly-still-divergent HEAD
content) — but ONLY if this repo has no pre-commit hook installed, or the
installed one predates this fix: a repo with the hook installed (the
default the moment 'logmind init'/'doctor --fix' runs with git enabled;
see 'logmind doctor') still routes 'logmind log's own commit THROUGH that
hook, which — being a plain POSIX-sh script with no fetch and no reliable
way to tell "warp staged this on purpose" apart from "git add -a swept up
an accidental dirty copy" — restores to HEAD unconditionally and can still
undo the repair on that path. This coupling is a known, currently
UNRESOLVED gap (see BuildPreCommitBody's doc comment, internal/hooks/
hooks.go); a raw 'git commit' instead of 'logmind log' hits the exact same
hook and has the same exposure.

warp's output has TWO layers holding DIFFERENT content, and which one you
commit is up to you. The read-refresh leaves origin/<default>'s TIP content
in the working tree, unstaged; the repair leaves MERGE-BASE content in the
index. 'logmind log' and a plain 'git commit' both do the right thing — the
first reverts the unstaged copies to HEAD, the second takes only the index —
but 'git commit -a' (or 'git add -A') sweeps the refreshed tip copies in as
well, and on any branch that forked before the default branch's last regen
the tip is NOT the merge-base, so CI's check-derived-docs rejects the
result. Commit what warp STAGED, not what it refreshed.

logmind log's own restore (and the harness guard) target HEAD rather than
the merge-base on their OWN hot path — cheap and offline, but unable to
repair a divergence that already happened — precisely because warp is the
one surface that fetches first, giving it a trustworthy, current
origin/<default> to compute a merge-base against.`,
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
	// Conditional on ACTUAL DIVERGENCE, not on branch alone. The v2.0.0 B6
	// `derived_docs.mode` adoption gate is gone — correctly, the invariant
	// is unconditional now — but that gate's `if` was carrying a SECOND
	// condition that had nothing to do with adoption: whether this branch
	// has anything to repair. Deleting both at once made warp repair every
	// branch, which broke the read-refresh above (see below) and hollowed
	// out the §0.4.1.2 staging signal. divergedDerivedDocPaths (derived.go)
	// restores that condition; its doc comment carries the full rationale.
	//
	// Interaction with the read-refresh loop above — this is why the
	// condition matters, not just a nicety. That loop just wrote
	// origin/<default>'s TIP content (freshest — good for a human/agent
	// reading current context) into the working tree. The merge-base can be
	// an ANCESTOR of that tip (whenever origin has advanced past this
	// branch's fork point), so the two disagree routinely. An unconditional
	// repair runs SECOND and wins, discarding the refresh it just announced
	// — warp printed "refreshed N derived doc(s)" and then threw them away,
	// one statement later, in the same command. Repairing only the paths
	// that actually diverged means a healthy branch keeps the fresh copy
	// (the whole point of warp) and a broken one still gets fixed.
	//
	// Restores — and therefore DELIBERATELY STAGES — BOTH the index and the
	// working tree (gitcli.RestorePathsToRef is `git checkout <ref> --
	// <path>`, the same primitive the commit-path surfaces use). This is
	// intentional, not merely a side effect of the primitive chosen: staging
	// the repair is what lets it survive into the caller's next commit
	// instead of sitting in the working tree until something else (a fresh
	// `warp`, another read-refresh) overwrites it again. It is narrower than
	// a `git add -A` (it only ever touches these few known, already-tracked
	// paths) and never creates a commit, so warp's "never commits" contract
	// holds; see TestWarp_DoesNotCommit.
	//
	// v2.0.0 4b-quater: this deliberate staging is also the signal
	// commitDecision's L1 (log.go) and guardCommitHarness's L2b
	// (guard_commit.go) now key off of — gitcli.IsPathStaged /
	// unstagedDerivedDocPaths (derived.go) — to recognize this repair as
	// intentional and leave it alone, instead of reverting it back to
	// HEAD's still-divergent content on the very next `logmind log`. See
	// TestWarpThenLog_PreservesRepairAcrossCommit (derived_repair_test.go)
	// for the end-to-end proof. That recognition does NOT extend past L1/L2b:
	// the pre-commit git hook (L2a) can't safely make the same distinction
	// (see BuildPreCommitBody's doc comment, internal/hooks/hooks.go) and
	// restores unconditionally — and since it is a REAL git hook, it also
	// fires during `logmind log`'s OWN commit if installed (unconditional,
	// the default the moment git is enabled), not just on a raw `git
	// commit`. This repair can still be undone on that path; closing it is
	// unresolved follow-up work, not part of this fix.
	repaired := false
	if onNonDefaultBranch(cwd) {
		mergeBase := gitcli.DefaultBranchMergeBase(cwd)
		diverged := divergedDerivedDocPaths(cwd, mergeBase, derivedDocPaths)
		if len(diverged) > 0 {
			// RestorePathsToRef is per-path best-effort: it attempts EVERY
			// path it is handed regardless of an earlier one erroring, and
			// returns only the FIRST error (if any) purely for logging. A
			// path untracked at mergeBase (e.g. a repo that added one derived
			// doc more recently than the other) is a normal, partial outcome,
			// not a reason to call the whole repair a failure — so `repaired`
			// reports "the repair ran", not "every path resolved cleanly",
			// matching the fully-silent, fully-best-effort stance every other
			// restore call site in this codebase (L1, L2a, L2b) already takes.
			if err := gitcli.RestorePathsToRef(cwd, mergeBase, diverged...); err != nil {
				fmt.Fprintf(stderr, "warn: repair to merge-base %s had a partial error (expected if a path doesn't exist there yet): %v\n", mergeBase, err)
			}
			repaired = true
		}
	}

	ahead := ""
	if out, _, err := gitcli.RunCaptured(cwd, "rev-list", "--count", "HEAD.."+ref, "--",
		"docs/decisions.md", "docs/decisions-branches"); err == nil {
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
