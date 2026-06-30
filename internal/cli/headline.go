// headline.go — `logmind headline <summary>` subcommand.
//
// Sets the branch's one-sentence "headline" summary: the plain-English line
// the main-canonical timeline shows for this branch (the §1.6.3 marker at the
// top of docs/decisions-branches/<branch>.md). The timeline copies it
// verbatim; the next agent reads it first. The <date>-<slug> KEY stays stable
// — only the visible sentence is refined (logmind makes no LLM call; the agent
// authors the sentence). See also `logmind log --headline` (the bundled form)
// and the per-log nudge in log.go.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/timeline"
)

func newHeadlineCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "headline <summary>",
		Short: "Set the one-sentence branch summary shown at the top of the timeline",
		Long: `Set the branch's one-sentence summary — the "headline" the main-canonical
timeline shows for this branch, and the first thing the next agent reads.

Write a plain-English sentence describing what the WHOLE branch did (not just
one decision). It's stored in the §1.6.3 marker at the top of the branch's
decision file and copied verbatim into docs/timeline.md. The <date>-<slug>
key stays stable — only the sentence changes, so you can refine it as the
branch grows.

Requires timeline.canonical: main-canonical (a no-op otherwise, and on the
default branch). Set LOGMIND_PR to append a (#NN) suffix to the visible line.

Example:
    logmind headline "Added JWT session auth with refresh-token rotation"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runHeadline(cwd, args[0], file, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&file, "file", "",
		"Target a specific branch decision file (e.g. docs/decisions-branches/feat__x.md) instead of the current branch — for summarizing old/merged branches.")
	return cmd
}

func runHeadline(cwd, summary, fileOverride string, stdout, stderr io.Writer) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		fmt.Fprintln(stdout, "Error: headline summary is empty.")
		return ErrSilent
	}
	cfg, _ := config.Load(cwd)
	if !cfg.Timeline.IsMainCanonical() {
		fmt.Fprintln(stdout, "Branch summaries are a main-canonical-timeline feature.")
		fmt.Fprintln(stdout, "Enable it with `timeline.canonical: main-canonical` in .logmind/config.yml.")
		return nil
	}

	var target string
	if fileOverride != "" {
		// Explicit file (e.g. an old/merged branch) — skip git-branch resolution.
		target = fileOverride
		if !filepath.IsAbs(target) {
			target = filepath.Join(cwd, target)
		}
		target = filepath.Clean(target)
		// Only branch detail files carry timeline markers — refuse to splice a
		// marker into decisions.md/archive or anything outside the branches dir.
		if filepath.Dir(target) != filepath.Join(cwd, "docs", "decisions-branches") {
			fmt.Fprintf(stdout, "--file must target a file under docs/decisions-branches/ (got %s).\n", fileOverride)
			return ErrSilent
		}
		if !pathExists(target) {
			fmt.Fprintf(stdout, "No such branch file: %s\n", fileOverride)
			return ErrSilent
		}
	} else {
		docsPath := filepath.Join(cwd, "docs")
		t, isBranchFile := resolveDecisionsPath(cwd, docsPath, cfg)
		if !isBranchFile {
			fmt.Fprintln(stdout, "Branch summaries apply on a feature branch; the default branch logs to docs/decisions.md directly.")
			return nil
		}
		if !pathExists(t) {
			fmt.Fprintln(stdout, "No decisions logged on this branch yet — run `logmind log` first, then set the summary.")
			return nil
		}
		target = t
	}
	return setHeadlineInFile(cwd, target, summary, stdout)
}

// setHeadlineInFile sets/refreshes the §1.6.3 marker headline in the given
// branch file: it rewrites the visible line of an existing marker (keeping the
// stable <date>-<slug> key), or inserts a marker if the file has none. Shared
// by `logmind headline` (current branch + --file). main-canonical gating is
// the caller's responsibility.
func setHeadlineInFile(cwd, target, summary string, stdout io.Writer) error {
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read %s: %w", target, err)
	}
	content := string(data)
	prSuffix := prSuffixFromEnv()

	updated, replaced := timeline.ReplaceFirstHeadline(content, summary, prSuffix)
	if !replaced {
		if timeline.HasEntryBlocks(content) {
			// A marker exists but couldn't be rewritten (unclosed/malformed) —
			// don't stack a second one. logmind never writes such a marker, so
			// this only fires on a hand-corrupted file.
			fmt.Fprintln(stdout, "Branch file has a malformed timeline marker — fix it manually, then retry.")
			return ErrSilent
		}
		// Markerless (pre-opt-in) branch file — insert a marker now.
		updated = string(insertMarkerAfterHeader([]byte(content), buildTimelineMarker(time.Now(), summary, prSuffix)))
	}
	rel := relForOk(cwd, target)
	if updated == content {
		fmt.Fprintln(stdout, "  branch summary already up to date")
		fmt.Fprintf(stdout, "ok headline: %s (unchanged)\n", rel)
		return nil
	}
	if err := writeAtomic(target, updated); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	fmt.Fprintf(stdout, "✓ Branch summary set: %s\n", summary)
	fmt.Fprintf(stdout, "ok headline: %s\n", rel)
	return nil
}

// relForOk renders target relative to cwd in posix form for the `ok` trailer,
// falling back to the absolute path if the relativise fails.
func relForOk(cwd, target string) string {
	rel, err := filepath.Rel(cwd, target)
	if err != nil {
		return target
	}
	return filepath.ToSlash(rel)
}
