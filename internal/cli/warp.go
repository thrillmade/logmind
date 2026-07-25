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
		Short: "Refresh docs/timeline.md + docs/file-structure.md from main (read-only catch-up)",
		Long: `Fetch the latest default branch and refresh your working copy of the two
DERIVED docs (docs/timeline.md, docs/file-structure.md) so your context reflects
main's current decisions.

Read-only: warp NEVER stages or commits these files. They are regenerated on
main only; a branch must keep them byte-identical to its merge-base with main so
that merges never conflict. Your branch's own decisions live in
docs/decisions-branches/<branch>.md (committed, per-branch, conflict-free).`,
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
	ahead := ""
	if out, _, err := gitcli.RunCaptured(cwd, "rev-list", "--count", "HEAD.."+ref, "--",
		"docs/decisions.md", "docs/decisions-branches", "docs/decisions-archive.md"); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(out)); e == nil && n > 0 {
			ahead = fmt.Sprintf(" · main is +%d decision commit(s) ahead", n)
		}
	}
	fmt.Fprintf(stdout, "ok warp: refreshed %d derived doc(s) from %s (read-only — not committed)%s\n", refreshed, ref, ahead)
	return nil
}
