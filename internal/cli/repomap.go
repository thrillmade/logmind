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

Experimental: currently Go-only (uses the go/parser standard library — zero
external dependency). Other languages, importance ranking, and a token budget
land in later slices; the default derived docs are unchanged.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runRepomap(cwd, quietEnabled(cmd), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	return cmd
}

func runRepomap(cwd string, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	cfg, _ := config.Load(cwd)
	text, files, err := repomap.Generate(cwd, cfg.FileStructure.IgnorePatterns)
	if err != nil {
		return err
	}
	nSyms := repomap.CountSymbols(files)
	if quiet {
		q.ok("repomap files=%d symbols=%d bytes=%d sink=stdout", len(files), nSyms, len(text))
		return nil
	}
	fmt.Fprint(stdout, text)
	fmt.Fprintf(stdout, "ok repomap: %d files, %d symbols, %d bytes (stdout)\n", len(files), nSyms, len(text))
	return nil
}
