package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// newContextCmd wires `logmind context`.
//
// Prints the agent cold-start payload — the two pre-baked derived docs
// (docs/timeline.md = the "why", docs/file-structure.md = the "what") — to
// stdout in one read. This is the single command an agent runs at the start
// of a task to orient, instead of burning tokens reconstructing context from
// `git log` / `ls -R` / `grep`. It is the practical expression of the
// logmind thesis: a repo self-describing on the edge of inference.
//
// Read-only. A missing derived doc is noted (with how to regenerate), never
// an error — `context` is a convenience, not a gate.
func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Print the agent cold-start context (timeline + file-structure) in one read",
		Long: `Print the agent cold-start context in one read.

Concatenates the two pre-baked derived docs an agent reads to orient:

    docs/timeline.md         the WHY — the decision timeline
    docs/file-structure.md   the WHAT — the repo tree

Run it at the start of a task instead of piecing context together from
git log / ls / grep. A missing derived doc is noted (with how to
regenerate it), not treated as an error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runContext(cwd, cmd.OutOrStdout())
		},
	}
	return cmd
}

// contextDoc is one section of the cold-start payload.
type contextDoc struct {
	rel   string // repo-relative path to the derived doc
	label string // section heading
	regen string // command to regenerate it when missing
}

func runContext(cwd string, stdout io.Writer) error {
	fmt.Fprint(stdout, "# logmind context\n\n"+
		"The pre-baked cold-start payload for this repo: the decision timeline\n"+
		"(the \"why\") and the file structure (the \"what\"). Read this to orient\n"+
		"instead of reconstructing context from git log / ls / grep.\n")

	docs := []contextDoc{
		{"docs/timeline.md", "Decision timeline — the why", "logmind timeline --write docs/timeline.md"},
		{"docs/file-structure.md", "File structure — the what", "logmind file-structure --write docs/file-structure.md"},
	}
	for _, d := range docs {
		fmt.Fprintf(stdout, "\n## %s (%s)\n\n", d.label, d.rel)
		data, err := os.ReadFile(filepath.Join(cwd, d.rel))
		if err != nil {
			fmt.Fprintf(stdout, "(%s not found — regenerate with: %s)\n", d.rel, d.regen)
			continue
		}
		stdout.Write(data)
		// Guarantee a trailing newline so the next section separates cleanly.
		if n := len(data); n == 0 || data[n-1] != '\n' {
			fmt.Fprintln(stdout)
		}
	}
	return nil
}
