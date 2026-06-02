package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/linkcheck"
)

// newCheckLinksCmd wires `logmind check-links`.
//
// Behaviour mirror of src/logmind/cli.check_links (cli.py:2795-2811)
// which delegates entirely to logmind.actions.link_check.main(). The
// Go binary calls into internal/linkcheck.Check directly with the
// same defaults: DEFAULT_ROOTS for what to scan and
// DEFAULT_ALLOW_ORPHANS for what to exempt.
//
// Configuration override via .logmind/config.yml's `linkcheck.roots`
// and `linkcheck.allow_orphans` keys is DEFERRED to a later wave —
// the config loader port hasn't landed yet (it's part of B3's `log`
// and `init` work). Without config support, the Go binary uses the
// same defaults as the Python action when no config is present, so
// the parity surface is identical for the dominant case (most
// projects don't override).
//
// Exit codes match Python: 0 on a clean run, 1 if any broken links
// or orphans were found. The report is byte-identical via
// linkcheck.FormatReport.
func newCheckLinksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check-links",
		Short: "Verify all relative markdown links resolve and no docs/*.md is orphaned",
		Long: `Verify all relative markdown links resolve and no docs/*.md is orphaned.

Walks README.md, AGENTS.md, CLAUDE.md, and the entire docs/ tree by
default. Configure roots and orphan allowlist via .logmind/config.yml:

    linkcheck:
      roots: [README.md, docs]
      allow_orphans: [docs/legacy.md]

Exits 0 on a clean run, 1 if any broken or orphan links are found.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runCheckLinks(cwd, cmd.OutOrStdout())
		},
	}
}

func runCheckLinks(cwd string, stdout io.Writer) error {
	broken, orphans, err := linkcheck.Check(cwd, nil, nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, linkcheck.FormatReport(broken, orphans))
	if len(broken) > 0 || len(orphans) > 0 {
		return errSilentExit1
	}
	return nil
}
