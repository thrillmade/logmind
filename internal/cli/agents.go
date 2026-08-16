package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/agents"
	"github.com/thrillmade/logmind/internal/atomicio"
	"github.com/thrillmade/logmind/internal/inserter"
	"github.com/thrillmade/logmind/internal/version"
)

// newAgentsCmd wires the `logmind agents` subcommand group: list,
// add, remove, update, migrate.
//
// Behaviour mirror of src/logmind/cli.agents (cli.py:1562-1900). The
// Go binary produces byte-identical stdout to Python v0.6.14 for each
// child command — the snapshot tests in testdata/agents_*.golden pin
// the exact output.
//
// Why a single newAgentsCmd factory that builds the whole tree: cobra
// parent commands aren't usable without children. Bundling the group
// here keeps the wiring + flag binding in one file so future readers
// don't have to chase cross-file references for the five subcommands.
func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Manage AI agent configuration files",
		Long: `Manage AI agent configuration files.

Sub-commands:
  list      List all supported agents and their status
  add       Add (or insert logmind into) an agent file
  remove    Remove an agent file
  update    Refresh stale logmind blocks + CI workflow pins
  migrate   Consolidate per-agent files into AGENTS.md (stubs)`,
		// Parent commands without their own action MUST surface help
		// when run bare so the user sees the child list.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAgentsListCmd())
	cmd.AddCommand(newAgentsAddCmd())
	cmd.AddCommand(newAgentsRemoveCmd())
	cmd.AddCommand(newAgentsUpdateCmd())
	cmd.AddCommand(newAgentsMigrateCmd())
	return cmd
}

// newAgentsListCmd wires `logmind agents list`.
//
// Behaviour mirror of src/logmind/cli.agents_list (cli.py:1569-1598).
// Output is BYTE-IDENTICAL to Python's CliRunner-captured output
// (non-TTY mode — ANSI codes stripped). The column widths come from
// the Python f-string: `agent_name:12 file:40`.
//
// Sync-on-list (the Python `sync_messages = sync_agent_files_from_config`
// call) is DEFERRED to B6 when the config loader lands. Without it,
// `agents list` just renders status. The Python sync only fires when
// `.logmind/config.yml` exists, so the absence of B6 means the Go
// binary's output diverges from Python only on configured repos —
// fresh repos produce identical bytes.
func newAgentsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all supported agents and their status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runAgentsList(cwd, cmd.OutOrStdout())
		},
	}
}

func runAgentsList(cwd string, stdout io.Writer) error {
	statuses := inserter.GetAgentStatus(cwd)
	fmt.Fprintln(stdout, "AI Agent Status:")
	fmt.Fprintln(stdout)
	for _, s := range statuses {
		icon, label := statusGlyph(s)
		// Python: f"  {icon} {agent_name:12} {info['file']:40} ({status_text})"
		// Python's f"{x:12}" left-pads to 12 chars; same for 40. The
		// 12-char pad is wider than every registered agent name; the
		// 40-char pad is wider than every registered file pattern
		// EXCEPT ".github/copilot-instructions.md" which is 31 chars
		// — well within 40. So padding never gets truncated.
		fmt.Fprintf(stdout, "  %s %-12s %-40s (%s)\n", icon, s.Name, s.File, label)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Supported agents: %s\n", strings.Join(agents.Names(), ", "))
	return nil
}

// statusGlyph returns the per-row icon + label per the Python
// branches: configured → "✓ / configured", exists (no logmind) →
// "~ / exists (no logmind)", missing → "✗ / not configured".
func statusGlyph(s inserter.AgentStatus) (icon, label string) {
	switch {
	case s.Configured:
		return "✓", "configured"
	case s.Exists:
		return "~", "exists (no logmind)"
	default:
		return "✗", "not configured"
	}
}

// newAgentsAddCmd wires `logmind agents add <name> [--no-commit]`.
//
// Behaviour mirror of src/logmind/cli.agents_add (cli.py:1601-1666):
//
//   - Unknown agent → "Error: Unknown agent '<name>'. Valid agents: ..." + exit 1
//   - File exists + JSON → "✓ <name>.json already exists (JSON format)"
//   - File exists + already has logmind section → "✓ <file> already has logmind instructions"
//   - File exists + needs insertion → InsertLogmindSection + "✓ Added logmind instructions to <file>"
//   - File doesn't exist → CreateAgentFile + "✓ Created <file> with logmind instructions"
//
// The `--no-commit` flag suppresses the git commit-and-push step. The
// commit-and-push wiring is DEFERRED to B6 (depends on the
// git_handler port). For now Go's add command writes the file (the
// load-bearing part) and prints a deferred-commit notice when the
// user didn't pass --no-commit, telling the user to commit manually.
// Without this notice, agents that auto-commit-on-add would silently
// stop committing during the Python→Go transition.
func newAgentsAddCmd() *cobra.Command {
	var noCommit bool
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add (or insert logmind into) an agent file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runAgentsAdd(cwd, args[0], noCommit, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&noCommit, "no-commit", false, "Don't commit the new file")
	return cmd
}

func runAgentsAdd(cwd, agentName string, noCommit bool, stdout io.Writer) error {
	a, ok := agents.Lookup(agentName)
	if !ok {
		fmt.Fprintf(stdout, "Error: Unknown agent '%s'. Valid agents: %s\n",
			agentName, strings.Join(agents.Names(), ", "))
		return ErrSilent
	}
	filePath := filepath.Join(cwd, filepath.FromSlash(a.FilePattern))

	if fileExists(filePath) {
		if a.IsJSON {
			fmt.Fprintf(stdout, "✓ %s already exists (JSON format)\n", filepath.Base(filePath))
			return nil
		}
		inserted, err := inserter.InsertLogmindSection(filePath)
		if err != nil {
			return err
		}
		if inserted {
			fmt.Fprintf(stdout, "✓ Added logmind instructions to %s\n", filepath.Base(filePath))
			noteDeferredCommit(noCommit, stdout)
			return nil
		}
		fmt.Fprintf(stdout, "✓ %s already has logmind instructions\n", filepath.Base(filePath))
		return nil
	}

	created, err := inserter.CreateAgentFile(agentName, cwd)
	if err != nil {
		return err
	}
	if created == "" {
		fmt.Fprintf(stdout, "Error: Failed to create file for agent '%s'\n", agentName)
		return ErrSilent
	}
	fmt.Fprintf(stdout, "✓ Created %s with logmind instructions\n", filepath.Base(created))
	noteDeferredCommit(noCommit, stdout)
	return nil
}

// noteDeferredCommit prints a one-line notice when the user didn't
// pass --no-commit but the Go binary can't yet auto-commit (B6
// dependency). Skipped when --no-commit is set so the user doesn't
// see a notice for behaviour they already opted out of.
//
// Once B6 lands the git_handler port, this helper becomes a wrapper
// around commit_and_push; the message goes away.
func noteDeferredCommit(noCommit bool, stdout io.Writer) {
	if noCommit {
		return
	}
	// Auto-commit-on-write is DEFERRED to wave B6 (depends on the
	// git_handler port not yet landed in v1-go-rewrite). For parity
	// behaviour we surface a single-line notice so callers used to
	// the Python auto-commit ergonomics know the difference.
	fmt.Fprintln(stdout, "(commit deferred — run `git add` + `git commit` manually until logmind v1.0 ships)")
}

// newAgentsRemoveCmd wires `logmind agents remove <name> [--force] [--no-commit]`.
//
// Behaviour mirror of src/logmind/cli.agents_remove (cli.py:1669-1726).
// Unknown agent → exit 1. Missing file → yellow "not configured"
// message + return (no error). Otherwise prompt unless --force, then
// remove and print "✓ Removed <file>".
//
// The interactive confirm uses bufio.Scanner on stdin — matches
// Python's click.confirm default behaviour (y/N).
func newAgentsRemoveCmd() *cobra.Command {
	var noCommit, force bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an agent file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runAgentsRemove(cwd, args[0], force, noCommit, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&noCommit, "no-commit", false, "Don't commit the removal")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Remove without confirmation")
	return cmd
}

func runAgentsRemove(cwd, agentName string, force, noCommit bool, stdin io.Reader, stdout io.Writer) error {
	a, ok := agents.Lookup(agentName)
	if !ok {
		fmt.Fprintf(stdout, "Error: Unknown agent '%s'. Valid agents: %s\n",
			agentName, strings.Join(agents.Names(), ", "))
		return ErrSilent
	}
	filePath := filepath.Join(cwd, filepath.FromSlash(a.FilePattern))
	if !fileExists(filePath) {
		fmt.Fprintf(stdout, "Agent '%s' is not configured (file does not exist)\n", agentName)
		return nil
	}
	if !force {
		fmt.Fprintf(stdout, "Remove %s? [y/N]: ", filepath.Base(filePath))
		if !confirmYes(stdin) {
			fmt.Fprintln(stdout, "Cancelled.")
			return nil
		}
	}
	removed, err := inserter.RemoveAgentFile(agentName, cwd)
	if err != nil {
		return err
	}
	if !removed {
		fmt.Fprintf(stdout, "Error: Failed to remove %s\n", filepath.Base(filePath))
		return ErrSilent
	}
	fmt.Fprintf(stdout, "✓ Removed %s\n", filepath.Base(filePath))
	noteDeferredCommit(noCommit, stdout)
	return nil
}

// confirmYes reads one line from stdin and returns true on
// "y"/"yes" (case-insensitive). Empty input → default "no" to match
// click.confirm's default=False.
//
// Doesn't depend on terminal mode — works with piped input from
// test harnesses.
func confirmYes(stdin io.Reader) bool {
	if stdin == nil {
		return false
	}
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}

// newAgentsUpdateCmd wires `logmind agents update [--apply]`.
//
// Behaviour mirror of src/logmind/cli.agents_update (cli.py:1729-1856).
// Dry-run (default) reports which files have stale logmind blocks and
// which CI workflows have stale pins. `--apply` rewrites them in
// place.
//
// The version source for the workflow pin sweep is the binary's
// `version.Version` variable — matches Python's `__version__` import.
// (B7 distribution wave converted Version from const → var so
// GoReleaser can inject the tag value via ldflags at release-build time.)
func newAgentsUpdateCmd() *cobra.Command {
	var doApply bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Refresh stale logmind blocks + CI workflow pins",
		Long: `Refresh outdated logmind marker blocks in AGENTS.md and CI workflow pins.

Dry-run (default): reports which files would be updated.
--apply: rewrites the block body in place, preserving content above + below the markers.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runAgentsUpdate(cwd, version.Version, doApply, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&doApply, "apply", false, "Rewrite stale marker blocks in place. Default is dry-run.")
	return cmd
}

func runAgentsUpdate(cwd, currentVersion string, doApply bool, stdout, stderr io.Writer) error {
	outdated, declined, err := inserter.FindOutdatedMarkerBlocks(cwd)
	if err != nil {
		return err
	}
	reportAgentsBlockRefusal(stderr, declined)
	stalePins, err := inserter.FindOutdatedWorkflowPins(cwd, currentVersion)
	if err != nil {
		return err
	}

	if len(outdated) == 0 && len(stalePins) == 0 {
		// Split the no-action message per cli.py:1764-1793. AGENTS.md
		// might be absent, present-without-block, or present-and-current
		// — each case gets its own user-facing message.
		agentsPath := filepath.Join(cwd, "AGENTS.md")
		if !fileExists(agentsPath) {
			fmt.Fprintln(stdout, "✓ No AGENTS.md in this repo — nothing to update. Run `logmind init` to install one.")
			return nil
		}
		data, err := os.ReadFile(agentsPath)
		if err != nil {
			return err
		}
		if _, ok := inserter.ExtractMarkerBlock(string(data)); !ok {
			fmt.Fprintln(stdout, "✓ AGENTS.md exists but has no logmind marker block. Run `logmind init` to install one (will preserve existing content above + below the markers).")
			return nil
		}
		if declined != nil {
			// A block this binary can't read or can't move forward is NOT
			// "current" — claiming it is was the quiet half of #267.
			fmt.Fprintln(stdout, "! AGENTS.md logmind block left unchanged — see the note on stderr.")
			return nil
		}
		fmt.Fprintln(stdout, "✓ AGENTS.md logmind block is current (no update needed).")
		return nil
	}

	if len(outdated) > 0 {
		if !doApply {
			fmt.Fprintf(stdout, "Would update %d file(s) with stale logmind block(s):\n", len(outdated))
		} else {
			fmt.Fprintf(stdout, "Found %d file(s) with stale logmind block(s):\n", len(outdated))
		}
		for _, e := range outdated {
			rel, _ := filepath.Rel(cwd, e.Path)
			fmt.Fprintf(stdout, "  - %s\n", filepath.ToSlash(rel))
		}
	}

	if len(stalePins) > 0 {
		if !doApply {
			fmt.Fprintf(stdout, "Would update %d CI workflow pin(s):\n", len(stalePins))
		} else {
			fmt.Fprintf(stdout, "Found %d CI workflow pin(s) to bump:\n", len(stalePins))
		}
		for _, p := range stalePins {
			rel, _ := filepath.Rel(cwd, p.Path)
			fmt.Fprintf(stdout, "  - %s (logmind==%s → logmind==%s)\n",
				filepath.ToSlash(rel), p.OldVersion, p.NewVersion)
		}
	}

	if !doApply {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Dry-run. Re-run with --apply to refresh.")
		return nil
	}

	// Apply path: rewrite each file in place through the one write
	// primitive, inserter.RefreshMarkerBlockFile. It OWNS THE READ (#297):
	// the two-string form it replaces took a whole file and a block body as
	// the same type and returned its first argument unchanged when the
	// markers were absent, so passing the wrong one produced a fragment
	// silently and wrote it over the user's whole file. Here there is no
	// whole-file parameter to get wrong, and a markerless file is refused
	// (inserter.ErrNoMarkerBlock) rather than rewritten.
	//
	// It writes through atomicio.WriteFile, not os.WriteFile, and that is
	// load-bearing here for #313's reason: these paths come from a scan of
	// a repository logmind did not necessarily write, and os.WriteFile
	// follows symlinks. A symlinked AGENTS.md / workflow file would have
	// its logmind block rewritten through the link, outside the repo.
	// The rename lands on the NAME instead. (Also makes the rewrite
	// crash-safe — the user's AGENTS.md is never a truncated stub.)
	//
	// UNDISCLOSED UNTIL NOW, NOW DISCLOSED: atomicio's rule 3 (see its
	// package doc) means the rename gives the destination a NEW inode. A
	// HARDLINKED AGENTS.md — a twin path pointing at the same inode, set up
	// by hand or by a dotfile manager — is silently detached: the twin keeps
	// the OLD content, this command still exits 0, and nothing here warns
	// that the two files just diverged. install_hook.go's force-append
	// branch faces the identical fact and chooses the opposite behaviour
	// (raw os.WriteFile, deliberately, to write THROUGH the link) — but that
	// is not an inconsistency to resolve by matching it: a shared git hook
	// via a hardlinked/symlinked .git/hooks/pre-commit is a common,
	// documented setup (husky, chezmoi, dotfile managers), where write-
	// through is the intent, while a hardlinked AGENTS.md twin is not a
	// setup this command supports or has ever advertised — there is no
	// call-site reasoning that write-through is wanted here, only the
	// absence of a check. Detecting it (an Lstat + link-count check before
	// every rewrite in this loop) is a real fix that belongs to whoever
	// next touches this path; until then, the accepted behaviour is
	// atomicio's documented one: a hardlinked destination is detached
	// silently, same as any other atomicio.WriteFile call site that hasn't
	// opted out for a stated reason.
	for _, e := range outdated {
		if err := inserter.RefreshMarkerBlockFile(e.Path, e.NewBody); err != nil {
			return err
		}
		rel, _ := filepath.Rel(cwd, e.Path)
		fmt.Fprintf(stdout, "✓ Refreshed %s\n", filepath.ToSlash(rel))
	}
	for _, p := range stalePins {
		data, err := os.ReadFile(p.Path)
		if err != nil {
			return err
		}
		rewritten, _ := inserter.UpdateWorkflowPin(string(data), p.NewVersion)
		if err := atomicio.WriteFile(p.Path, []byte(rewritten), 0o644); err != nil {
			return err
		}
		rel, _ := filepath.Rel(cwd, p.Path)
		fmt.Fprintf(stdout, "✓ Bumped logmind pin in %s\n", filepath.ToSlash(rel))
	}

	return nil
}

// newAgentsMigrateCmd wires `logmind agents migrate [--no-commit]`.
//
// Behaviour mirror of src/logmind/cli.agents_migrate (cli.py:1859-1900).
// Consolidates per-agent instruction files into AGENTS.md (replacing
// each with a 2-line stub). Idempotent — re-running on an
// already-stubbed tree prints "No agent files to migrate".
func newAgentsMigrateCmd() *cobra.Command {
	var noCommit bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Consolidate per-agent files into AGENTS.md",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runAgentsMigrate(cwd, noCommit, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&noCommit, "no-commit", false, "Don't commit the migration changes")
	return cmd
}

func runAgentsMigrate(cwd string, noCommit bool, stdout, stderr io.Writer) error {
	messages, declined, err := inserter.MigrateToAgentsMD(cwd)
	if err != nil {
		return err
	}
	// Reported before the messages: migrate consolidates the per-agent
	// files either way, but AGENTS.md's own block was left alone (#267).
	reportAgentsBlockRefusal(stderr, declined)
	if len(messages) == 0 {
		fmt.Fprintln(stdout, "No agent files to migrate (already consolidated).")
		return nil
	}
	for _, m := range messages {
		fmt.Fprintln(stdout, m)
	}
	noteDeferredCommit(noCommit, stdout)
	return nil
}

// fileExists is a small local helper. B3 (sibling wave) ships an
// identical function in timeline.go; one of the two will be deleted
// during the trivial rebase that merges B3 and B4. Kept here so this
// branch builds standalone without depending on B3's package landing
// first.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
