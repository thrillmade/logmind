// show.go — `logmind show` subcommand.
//
// SKILL.md / AGENTS.md / internal/templates/logmind-section.md all document
// `logmind show` as "recent decisions on the current branch" with `--all` to
// include the archive, but the v2 Go binary never shipped it (a v1.0
// rewrite gap; a retired PR #154 built a version against the pre-branch-aware
// v1 shape). This is the v2 re-implementation: it reads the SAME file `logmind
// log` would write to right now — resolveDecisionsPath's branch-aware target,
// not a hardcoded docs/decisions.md — so `show` always reflects "this branch's
// recent decisions", matching the documented contract.
//
// Output shape:
//
//   - default: streams the resolved decision file verbatim.
//   - --all: appends docs/decisions-archive.md under an ARCHIVED DECISIONS
//     banner, when the archive exists.
//
// Quiet discipline (quiet.go): under --quiet/LOGMIND_QUIET the verbatim body
// is suppressed — the deliverable a human wants (the markdown) isn't
// "chatter", but an agent that opted into quiet mode wants a receipt, not the
// payload, matching `logmind repomap`'s stdout-sink precedent. The single `ok`
// line always carries the byte count so a script can tell empty from missing.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
)

// newShowCmd wires the `logmind show` subcommand.
func newShowCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show recent decisions on the current branch",
		Long: `Show recent decisions on the current branch.

Streams the decision file "logmind log" would write to right now:
docs/decisions.md on the default branch, or
docs/decisions-branches/<branch>.md on a feature branch (when
decisions.branch_aware is on and this branch has entries).

Pass --all to also print docs/decisions-archive.md, appended under an
ARCHIVED DECISIONS banner.

Examples:
    logmind show
    logmind show --all`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runShow(cwd, all, quietEnabled(cmd), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&all, "all", false,
		"Also print docs/decisions-archive.md, appended under an ARCHIVED DECISIONS banner.")
	return cmd
}

// runShow implements `logmind show`.
func runShow(cwd string, all, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		q.fail("Error: docs/ directory not found. Run 'logmind init' first.\n")
		return ErrSilent
	}

	cfg, _ := config.Load(cwd)
	target, _ := resolveDecisionsPath(cwd, docsPath, cfg)
	rel := relForOk(cwd, target)

	var body string
	if pathExists(target) {
		data, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read %s: %w", target, err)
		}
		body = string(data)
	}

	var archiveBody string
	archiveShown := false
	if all {
		archivePath := filepath.Join(docsPath, "decisions-archive.md")
		if pathExists(archivePath) {
			data, err := os.ReadFile(archivePath)
			if err != nil {
				return fmt.Errorf("read %s: %w", archivePath, err)
			}
			archiveBody = string(data)
			archiveShown = true
		}
	}

	if quiet {
		q.ok("show path=%s bytes=%d all=%t archive=%t", rel, len(body), all, archiveShown)
		return nil
	}

	if body == "" {
		fmt.Fprintln(stdout, "No decisions logged yet on this branch.")
	} else {
		fmt.Fprint(stdout, body)
	}

	if archiveShown {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, strings.Repeat("=", 80))
		fmt.Fprintln(stdout, "ARCHIVED DECISIONS")
		fmt.Fprintln(stdout, strings.Repeat("=", 80))
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, archiveBody)
	}

	suffix := ""
	if all {
		if archiveShown {
			suffix = " + archive"
		} else {
			suffix = " (no archive)"
		}
	}
	fmt.Fprintf(stdout, "ok show: %s (%d bytes%s)\n", rel, len(body), suffix)
	return nil
}
