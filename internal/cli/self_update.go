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
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/claudehook"
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
	updated := false

	if msg, err := inserter.EnsureAgentsMD(cwd); err == nil && msg != "" {
		fmt.Fprintln(out, msg)
		updated = true
	}

	// Refresh per-agent stubs by detecting drift and rewriting affected
	// files. The inserter package's FindOutdatedMarkerBlocks +
	// MigrateToAgentsMD pipeline already covers this; we call them here
	// in best-effort mode.
	if entries, err := inserter.FindOutdatedMarkerBlocks(cwd); err == nil {
		for _, entry := range entries {
			refreshed := inserter.ReplaceMarkerBlock(entry.OldBody, entry.NewBody)
			if err := os.WriteFile(entry.Path, []byte(refreshed), 0o644); err == nil {
				if rel, err := filepath.Rel(cwd, entry.Path); err == nil {
					fmt.Fprintln(out, "✓ Refreshed marker block in", rel)
					updated = true
				}
			}
		}
	}

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
		fmt.Fprintln(out, "✓ logmind templates are up to date.")
	}
	fmt.Fprintln(out, "ok self-update applied")
	return nil
}
