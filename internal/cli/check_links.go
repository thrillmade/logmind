package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/linkcheck"
)

// newCheckLinksCmd wires `logmind check-links [--json]`.
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
// `--json` (v1.2.0+): instead of the human-readable report, emit the
// CheckReport struct as JSON. Used by the v5 check-doc-links workflow
// template's mode-B self-heal job (the deterministic PR-comment path
// that runs when no ANTHROPIC_API_KEY is configured). The JSON shape
// is intentionally stable so workflow-side consumers don't break on
// every logmind release.
//
// Exit codes match Python: 0 on a clean run, 1 if any broken links
// or orphans were found. The report is byte-identical via
// linkcheck.FormatReport.
func newCheckLinksCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "check-links",
		Short: "Verify all relative markdown links resolve and no docs/*.md is orphaned",
		Long: `Verify all relative markdown links resolve and no docs/*.md is orphaned.

Walks README.md, AGENTS.md, CLAUDE.md, and the entire docs/ tree by
default. Configure roots and orphan allowlist via .logmind/config.yml:

    linkcheck:
      roots: [README.md, docs]
      allow_orphans: [docs/legacy.md]

Exits 0 on a clean run, 1 if any broken or orphan links are found.

With --json, emits the agent-readable CheckReport (broken/orphans
with SuggestedFix strings) used by the v5 check-doc-links workflow's
mode-B self-heal job.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runCheckLinks(cwd, asJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false,
		"Emit the CheckReport as JSON instead of the human-readable text. "+
			"Used by the v5 check-doc-links workflow's mode-B self-heal job.")
	return cmd
}

func runCheckLinks(cwd string, asJSON bool, stdout io.Writer) error {
	if asJSON {
		report, err := linkcheck.CheckWithReport(cwd, nil, nil)
		if err != nil {
			return err
		}
		// Marshal with stable key ordering. The workflow template's
		// Python parser reads `broken` + `orphans` + `path` + `reason`
		// + `suggestedFix` — pin those exact names via JSON tags on
		// the Finding/CheckReport structs.
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(data))
		if report.HasIssues() {
			return ErrSilent
		}
		return nil
	}

	broken, orphans, err := linkcheck.Check(cwd, nil, nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, linkcheck.FormatReport(broken, orphans))
	if len(broken) > 0 || len(orphans) > 0 {
		return ErrSilent
	}
	return nil
}
