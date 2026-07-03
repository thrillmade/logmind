package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/tree"
)

// newFileStructureCmd wires `logmind file-structure [--write PATH] [--check] [--max-depth N]`.
//
// Behaviour mirror of Python cli.file_structure_cmd (cli.py:2693-2792):
//
//	--max-depth N (positive)   → truncate at depth N (root is depth 0)
//	--max-depth 0              → unbounded (full tree)
//	--max-depth omitted        → default 2 (DEFAULT_FILE_STRUCTURE_DEPTH)
//
// Output is byte-identical to Python v0.6.14 (stdout messages, sizes,
// labels). Like timeline, the --check-without-write divergence vs
// Python (exit 1 here, exit 2 in Python) is a known difference.
func newFileStructureCmd() *cobra.Command {
	var writePath string
	var check bool
	// maxDepth uses -1 as "flag not supplied" sentinel so we can
	// distinguish --max-depth 0 (unbounded) from the omitted case
	// (default 2). cobra's Flag().Changed() also exposes this; we use
	// the sentinel for consistency with newTreeCmd which needs both.
	var maxDepth int
	cmd := &cobra.Command{
		Use:   "file-structure",
		Short: "Print or regenerate docs/file-structure.md",
		Long: `Print or regenerate the derived docs/file-structure.md tree snapshot.

Mirror of ` + "`logmind timeline`" + ` for the file-structure derived doc.
The v0.3.0 git merge driver invokes this as
` + "`logmind file-structure --write %A`" + ` to resolve conflicts on
parallel-PR rebases without falling through to textual three-way merge.

Examples:
    logmind file-structure                                       # depth 2, stdout
    logmind file-structure --max-depth 0                         # full tree
    logmind file-structure --write docs/file-structure.md        # regenerate file at depth 2
    logmind file-structure --write docs/file-structure.md --check  # CI gate`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			// Translate CLI sentinel → internal depth:
			//   -1 (flag absent)  → DEFAULT_FILE_STRUCTURE_DEPTH (2)
			//    0 (--max-depth 0) → -1 internally (unbounded)
			//    >0                → as-is
			effective := tree.DefaultFileStructureDepth
			if cmd.Flag("max-depth").Changed {
				if maxDepth == 0 {
					effective = -1
				} else {
					effective = maxDepth
				}
			}
			return runFileStructure(cwd, writePath, check, effective, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&writePath, "write", "",
		"Write the rendered tree to PATH (typically docs/file-structure.md). "+
			"Without this flag, prints to stdout.")
	cmd.Flags().BoolVar(&check, "check", false,
		"Exit nonzero if writing would change the file. Used in CI to fail "+
			"the build before regen so the auto-commit step runs and updates the PR. "+
			"Mirrors `logmind timeline --check`.")
	cmd.Flags().IntVar(&maxDepth, "max-depth", -1,
		"Cap the tree at depth N (root is depth 0). Default: 2 (token-frugal). "+
			"Pass 0 for unbounded (full tree); pass a positive integer to truncate.")
	return cmd
}

func runFileStructure(cwd, writePath string, check bool, effective int, stdout, stderr io.Writer) error {
	// File-structure doesn't insist on docs/ existing for stdout mode.
	// Python's file-structure has no docs/-existence check either.
	depthLabel := fmt.Sprintf("depth=%d", effective)
	if effective < 0 {
		depthLabel = "unbounded"
	}

	if check {
		if writePath == "" {
			fmt.Fprintln(stdout, "Error: --check requires --write PATH to compare against.")
			return ErrSilent
		}
		rendered, err := tree.GenerateFileStructure(cwd, effective)
		if err != nil {
			return err
		}
		existing, _ := os.ReadFile(writePath)
		if string(existing) != rendered {
			fmt.Fprintf(stdout, "✗ %s is stale — re-run `logmind file-structure --write %s` and commit.\n", writePath, writePath)
			return ErrSilent
		}
		fmt.Fprintf(stdout, "✓ %s is up to date\n", writePath)
		fmt.Fprintf(stdout, "ok file-structure: %s up to date\n", writePath)
		return nil
	}

	if writePath == "" {
		rendered, err := tree.GenerateFileStructure(cwd, effective)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, rendered)
		fmt.Fprintf(stdout, "ok file-structure: %d bytes, %s (stdout)\n", len(rendered), depthLabel)
		return nil
	}

	changed, err := tree.WriteFileStructure(writePath, cwd, effective)
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(stdout, "✓ Regenerated %s\n", writePath)
	} else {
		fmt.Fprintf(stdout, "  %s already up to date\n", writePath)
	}
	st, err := os.Stat(writePath)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ok %s (%d bytes, %s)\n", writePath, st.Size(), depthLabel)
	return nil
}

// newTreeCmd wires `logmind tree [--max-depth N]`.
//
// Always writes to docs/file-structure.md (matches Python tree_cmd which
// calls update_file_structure(docs_path) by default). --max-depth follows
// the same CLI sentinel convention as `file-structure`: omitted = "default"
// (Python's update_file_structure default = depth 2), 0 = unbounded.
//
// Python prints `effective` as "default" when --max-depth is omitted; we
// match that to preserve byte-identical output even though internally we
// translate to depth 2.
func newTreeCmd() *cobra.Command {
	var maxDepth int
	cmd := &cobra.Command{
		Use:   "tree",
		Short: "Regenerate docs/file-structure.md with the current project tree",
		Long: `Regenerate docs/file-structure.md with the current project tree.

Equivalent to the side-effect that runs after every ` + "`logmind log`" + ` when
` + "`file_structure.auto_update: true`" + ` is set in ` + "`.logmind/config.yml`" + `.
Useful as a pre-commit hook step or when an agent has just written
several files and wants the docs/ snapshot to reflect them immediately.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			maxDepthFlagPresent := cmd.Flag("max-depth").Changed
			return runTree(cwd, maxDepth, maxDepthFlagPresent, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().IntVar(&maxDepth, "max-depth", -1,
		"Cap the tree at depth N (root is depth 0). Default: 2 (token-frugal — "+
			"matches `logmind file-structure`). Pass 0 for unbounded (full tree); "+
			"pass a positive integer to truncate.")
	return cmd
}

func runTree(cwd string, maxDepth int, maxDepthFlagPresent bool, stdout, stderr io.Writer) error {
	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		fmt.Fprintln(stdout, "Error: docs/ directory not found. Run 'logmind init' first.")
		return ErrSilent
	}
	// Mirror Python tree_cmd:
	//   --max-depth omitted: effective = "default" label, depth = 2 (tree_gen's default)
	//   --max-depth 0:       effective = "unbounded" label, depth = -1 internally
	//   --max-depth N>0:     effective = "depth=N" label, depth = N
	target := filepath.Join(docsPath, "file-structure.md")
	var effective int
	var effectiveLabel string
	if !maxDepthFlagPresent {
		effective = tree.DefaultFileStructureDepth
		effectiveLabel = "default"
	} else if maxDepth == 0 {
		effective = -1
		effectiveLabel = "unbounded"
	} else {
		effective = maxDepth
		effectiveLabel = fmt.Sprintf("depth=%d", maxDepth)
	}
	if _, err := tree.WriteFileStructure(target, cwd, effective); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "✓ Updated docs/file-structure.md")
	st, err := os.Stat(target)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ok docs/file-structure.md (%d bytes, %s)\n", st.Size(), effectiveLabel)
	return nil
}
