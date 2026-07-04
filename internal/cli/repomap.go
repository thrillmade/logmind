// repomap.go — `logmind repomap` (experimental; token-killer Phase 2).
//
// Prints a deterministic SIGNATURE SKELETON of the repo: every top-level Go
// func / method / type as its signature with the body dropped. It is the
// structural companion to `logmind file-structure` — the tree tells an agent
// WHERE code lives; the repomap tells it WHAT the API surface is, at a tiny,
// cache-stable fraction of the source's token cost.
//
// Experimental + additive: this is a NEW, standalone command. It touches no
// golden-locked surface (file-structure.md / timeline.md are unchanged) and
// changes no config default. A later slice folds the skeleton into the
// `logmind context` payload behind a config key that flips on at v1.0.
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/repomap"
)

func newRepomapCmd() *cobra.Command {
	var mapTokens int
	cmd := &cobra.Command{
		Use:   "repomap",
		Short: "Print a deterministic Go signature skeleton of the repo (experimental; token-killer Phase 2)",
		Long: `Print a signature skeleton of the repository — every top-level Go function,
method, and type rendered as its signature with the BODY dropped.

Where ` + "`logmind file-structure`" + ` gives an agent the name-tree (the WHERE),
the repomap gives the API surface it actually reasons over (the WHAT) — at a
tiny, cache-stable fraction of the source's token cost. Read it to orient
before opening any file.

Deterministic (files sorted, stdlib pretty-printing, no timestamps / absolute
paths / filesystem order) so it caches as a stable prefix, exactly like
` + "`logmind context`" + `.

--map-tokens N ranks files by importance (files the team logged decisions
about, then intra-repo import fan-in, then path) and keeps as many whole files
as fit an estimated N-token budget, marking the rest omitted. Without it, every
file is emitted in path order (unchanged, byte-stable).

Experimental: currently Go-only (uses the go/parser standard library — zero
external dependency). Other languages land in a later slice.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runRepomap(cwd, mapTokens, quietEnabled(cmd), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().IntVar(&mapTokens, "map-tokens", 0,
		"Rank by importance and pack the skeleton to an estimated N-token budget (est. ~4 chars/token), omitting the least-important files. 0 = no budget (all files, path order).")
	return cmd
}

func runRepomap(cwd string, mapTokens int, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	cfg, _ := config.Load(cwd)
	text, kept, omitted, err := repomap.GenerateBudget(cwd, cfg.FileStructure.IgnorePatterns, mapTokens)
	if err != nil {
		return err
	}
	nSyms := repomap.CountSymbols(kept)
	if quiet {
		q.ok("repomap files=%d symbols=%d omitted=%d bytes=%d sink=stdout", len(kept), nSyms, omitted, len(text))
		return nil
	}
	fmt.Fprint(stdout, text)
	fmt.Fprintf(stdout, "ok repomap: %d files, %d symbols, %d omitted, %d bytes (stdout)\n", len(kept), nSyms, omitted, len(text))
	return nil
}
