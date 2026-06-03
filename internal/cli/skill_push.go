package cli

import (
	"errors"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/skill"
)

// newSkillPushCmd wires `logmind skill push <name> [--catalog <owner/repo>]
// [--dry-run]`.
//
// Direction: LOCAL → CATALOG. Per plan §"Skill suggestion cycle §4" the
// consumer repo always owns the source-of-truth skill; this command
// promotes a polished local version to a shared catalog repo by opening
// a PR there. There is no inverse "pull from catalog" command, because
// that would invert the End State #5 promise.
//
// Default catalog target: thrillmade/agent-skills. Override per-repo
// via .logmind/config.yml `catalog_target` or per-invocation via
// --catalog.
//
// Auth: uses the user's `gh` CLI session. The catalog repo decides
// merge policy independently (clud-bug-review + maintainer approval);
// this command just opens the PR cleanly.
func newSkillPushCmd() *cobra.Command {
	var catalogFlag string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "push <name>",
		Short: "Publish a local SKILL.md to a catalog repo via PR (local → catalog)",
		Long: `Publish a local SKILL.md to a catalog repo by opening a pull request.

Skills are AUTHORED in the consumer repo first (` + "`" + `.claude/skills/<name>/SKILL.md` + "`" + `).
This command promotes a polished local skill to a shared catalog repo —
the catalog is downstream of every consumer; there's no inverse
"pull from catalog" command.

The default catalog target is ` + "`" + `thrillmade/agent-skills` + "`" + `; override
per-repo via ` + "`" + `.logmind/config.yml catalog_target` + "`" + ` or per-invocation
via --catalog.

Authentication: this command uses your local ` + "`" + `gh` + "`" + ` CLI session.
You must have ` + "`" + `gh auth login` + "`" + ` configured with write access to the
catalog repo (or be in an org where ` + "`" + `gh` + "`" + ` can open PRs on the
catalog).

Examples:
  logmind skill push critical-issues-only
  logmind skill push my-skill --catalog acme/private-skills
  logmind skill push my-skill --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSkillPush(cwd, args[0], catalogFlag, dryRun, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&catalogFlag, "catalog", "",
		"Target catalog repo (<owner>/<repo>); overrides .logmind/config.yml catalog_target.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Validate + report what would happen, but skip clone/push/PR creation.")
	return cmd
}

// runSkillPush is the testable core. Resolves the catalog target
// (flag > config > default), then delegates to skill.Push.
//
// CLI surface error convention: user-facing messages are written to
// stdout (matches Python CLI parity) and the RunE returns ErrSilent so
// cobra exits non-zero without re-printing the error. Unexpected
// errors (no cwd, IO failure) bubble up as real errors so main()'s
// stderr printer surfaces them.
func runSkillPush(cwd, name, catalogFlag string, dryRun bool, stdout io.Writer) error {
	target := resolveCatalogTarget(cwd, catalogFlag)

	_, err := skill.Push(skill.PushOptions{
		SkillName:      name,
		CatalogTarget:  target,
		DryRun:         dryRun,
		SourceRepoRoot: cwd,
		Stdout:         stdout,
	})
	if err != nil {
		// Translate the known-shape errors into ErrSilent so the user
		// sees the message we already printed (no double-print on
		// stderr). Anything else is a real failure.
		switch {
		case errors.Is(err, skill.ErrSkillNotFound),
			errors.Is(err, skill.ErrInvalidCatalogTarget),
			errors.Is(err, skill.ErrGhNotFound),
			errors.Is(err, skill.ErrGhNotAuthed):
			return ErrSilent
		}
		return err
	}
	return nil
}

// resolveCatalogTarget picks the catalog destination per:
//
//  1. --catalog CLI flag (if non-empty), wins over everything;
//  2. .logmind/config.yml `catalog_target` (if set), overrides default;
//  3. built-in default "thrillmade/agent-skills" (from config.DefaultConfig()).
//
// Tests construct the config file directly to exercise (2). The config
// loader silently degrades to defaults on parse error, so (3) is the
// no-config fallback path.
func resolveCatalogTarget(cwd, flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	cfg, _ := config.Load(cwd)
	if cfg.CatalogTarget != "" {
		return cfg.CatalogTarget
	}
	// Defensive: if for any reason the loader returned an empty
	// CatalogTarget (shouldn't happen since DefaultConfig sets it), fall
	// back to the same constant Push's docstring promises.
	return "thrillmade/agent-skills"
}
