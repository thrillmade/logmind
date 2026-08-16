// self_update.go — `logmind self-update` subcommand.
//
// Replaces the auto-refresh that used to happen inside `logmind log`.
// `log` now only WARNS about drift; refresh is a separate user-invoked
// step so it never piggy-backs onto a decision-logging branch and races
// with concurrent self-update PRs.
//
// Refreshes:
//   - AGENTS.md logmind block (in-place body replacement; preserves
//     content above + below the markers)
//   - Per-agent stub files (CLAUDE.md, .cursorrules, etc.) when their
//     enabled-agent config requires
//   - Hooks (post-merge + post-rewrite + commit-msg) — re-installed
//     from the current binary's body (closes the v0.6.10 drift loop)
//   - The Claude Code PreToolUse guard in .claude/settings.json (Layer 1
//     of v2.0.0 commit enforcement; internal/claudehook), gated on the
//     same agents.claude config check doctor --fix uses (default true).
//     Without this, a repo whose only refresh path is self-update would
//     get its commit-msg hook auto-upgraded to enforcing (Layer 2) while
//     Layer 1 stays missing forever — and doctor would never nudge,
//     because "missing" is benign.
//
// pinVersion (SPEC §1.2.1 / §3.7): when `.logmind/config.yml` sets a
// non-empty top-level `pinVersion`, self-update no-ops entirely — none of
// the refreshes above run. This is checked FIRST, before any refresh
// work, so a pinned repo never partially refreshes.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/claudehook"
	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/inserter"
)

func newSelfUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "self-update",
		Short: "Refresh AGENTS.md block, per-agent stubs, and git hooks",
		Long: "Refresh AGENTS.md block, per-agent stubs, and git hooks to\n" +
			"match the currently-running logmind binary. Idempotent: a\n" +
			"no-op when everything is already current.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSelfUpdate(cmd)
		},
	}
}

func runSelfUpdate(cmd *cobra.Command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	// pinVersion floor (SPEC §1.2.1 / §3.7): a repo that pins below the
	// running binary's version wants NO refresh at all — checked before any
	// file is touched, so a pin is a true, complete no-op.
	if cfg, _ := config.Load(cwd); cfg.PinVersion != "" {
		fmt.Fprintf(out, "logmind self-update: pinned to %s, skipping (unset pinVersion in .logmind/config.yml to resume updates)\n", cfg.PinVersion)
		fmt.Fprintln(out, "ok self-update pinned")
		return nil
	}

	updated := false
	blockRefused := false
	blockFailed := false

	// THE ERROR IS NOT OPTIONAL (#306). This used to read
	// `if …, err := inserter.EnsureAgentsMD(cwd); err == nil {` with no else,
	// which was survivable only while the error was unreachable. Teaching the
	// AGENTS.md write to refuse a symlink at its destination made it reachable,
	// and the discarded branch then printed "✓ logmind templates are up to
	// date." over a block that was still stale, exited 0, and never showed the
	// refusal at all — the worst of the three outcomes, because it is the one
	// that stops the user from looking.
	msg, declined, err := inserter.EnsureAgentsMD(cwd)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: self-update: AGENTS.md logmind block was NOT refreshed —", err)
		blockFailed = true
	} else {
		if msg != "" {
			fmt.Fprintln(out, msg)
			updated = true
		}
		// self-update is the surface a stale binary is most likely to be
		// run from, so it is the most likely to meet a block a newer one
		// wrote. Say so rather than reporting "up to date" (#267).
		reportAgentsBlockRefusal(cmd.ErrOrStderr(), declined)
		blockRefused = declined != nil
	}

	// NOTE — there is deliberately no second AGENTS.md refresher here (#297).
	//
	// A FindOutdatedMarkerBlocks loop used to sit at this point, writing
	// AGENTS.md a second time in the same command. FindOutdatedMarkerBlocks
	// only ever reports AGENTS.md, and EnsureAgentsMD above has already
	// refreshed it against the same classifier (planBlockRefresh) — so the
	// loop was pure duplication, which SPEC §5.2 forbids outright ("Exactly
	// one automation owns any generated or copied path. Two refreshers MUST
	// NOT write the same path"). Being unreachable is also why nothing caught
	// that it passed the BLOCK BODY where the WHOLE FILE belonged and wrote
	// the resulting fragment over the user's entire AGENTS.md.
	//
	// Deleting the duplicate is the fix, not repairing its arguments: the
	// second writer had no work of its own to do, and a dead write path is
	// exactly where an untested defect survives.
	//
	// Precisely what is single here, since "EnsureAgentsMD is the single owner
	// of this path" overstated it: AGENTS.md's bytes have exactly one WRITE
	// PRIMITIVE, inserter.RefreshMarkerBlockFile, which owns the read, requires
	// the markers to be present, and writes the whole file back. Two commands
	// route through it — this one via inserter.EnsureAgentsMD, and `logmind
	// agents update --apply` via runAgentsUpdate — which is one owner reached
	// from two surfaces, not two refreshers. What SPEC §5.2 forbids is the
	// second REFRESHER, and within this command the call above is the only one.
	// EnsureAgentsMD reports through `msg`.

	// Refresh local hooks to match the running binary's body.
	if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
		if changed, _ := hooks.InstallPostMerge(cwd); changed {
			fmt.Fprintln(out, "✓ Refreshed .git/hooks/post-merge")
			updated = true
		}
		if changed, _ := hooks.InstallPostRewrite(cwd); changed {
			fmt.Fprintln(out, "✓ Refreshed .git/hooks/post-rewrite")
			updated = true
		}
		if changed, _ := hooks.InstallCommitMsg(cwd); changed {
			fmt.Fprintln(out, "✓ Refreshed .git/hooks/commit-msg")
			updated = true
		}
	}

	// Layer 1 of commit enforcement (Claude Code PreToolUse guard) —
	// same agents.claude config gate as doctor --fix (default true), and
	// error-swallowed like the hook refreshes above (a malformed
	// settings.json degrades to residual state, never a failed
	// self-update). Not gated on .git presence: .claude/settings.json is
	// repo content, not git-clone state.
	if claudeAgentEnabledFromConfig(cwd) {
		if changed, _ := claudehook.EnsurePreToolUseGuard(cwd); changed {
			fmt.Fprintln(out, "✓ Refreshed .claude/settings.json (Claude Code guard-commit hook)")
			updated = true
		}
	}

	if !updated {
		switch {
		case blockFailed:
			// Neither "up to date" nor "left unchanged": the refresh was
			// attempted and FAILED, and the block on disk is still whatever it
			// was — stale, most likely, since that is why the write was tried.
			fmt.Fprintln(out, "! AGENTS.md logmind block could NOT be refreshed — see the error on stderr.")
		case blockRefused:
			// "up to date" would contradict the note on stderr — this repo's
			// block is one this binary can't move forward, not one it just
			// verified (#267).
			fmt.Fprintln(out, "! AGENTS.md logmind block left unchanged — see the note on stderr.")
		default:
			fmt.Fprintln(out, "✓ logmind templates are up to date.")
		}
	}
	if blockFailed {
		// `ok self-update applied` is a receipt for work that happened. Exit
		// non-zero as well: the self-update workflow runs this unattended, and
		// a zero exit is the difference between a repo whose stale block gets
		// noticed and one where it never does.
		fmt.Fprintln(out, "ok self-update incomplete")
		return ErrSilent
	}
	fmt.Fprintln(out, "ok self-update applied")
	return nil
}
