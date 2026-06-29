package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/timeline"
)

// newTimelineCmd wires `logmind timeline [--write PATH] [--check] [--full]`.
//
// Output is byte-identical to Python v0.6.14 stdout messages. The
// `ok` trailer line is preserved (Python emits it via `_ok()`) because
// agents chain on its `ok <key-value>` shape.
//
// Known divergence vs Python: --check without --write returns exit 1
// in Go (Python uses exit 2). The stdout error message is byte-
// identical so consumers see the same diagnostic; only the exit code
// differs. We don't carry an exit-2 sentinel because the existing main.go
// shape only supports exit 0 / exit 1. Surfacing exit 2 would require
// a coordinated change to cmd/logmind/main.go that exceeds B3's scope.
func newTimelineCmd() *cobra.Command {
	var writePath string
	var check bool
	var full bool
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Print or regenerate the high-level decision timeline",
		Long: `Print or regenerate the high-level decision timeline.

Reads docs/decisions.md, docs/decisions-archive.md, and every
docs/decisions-branches/*.md as sources; renders a chronological
timeline grouped by year-month. Sources are never modified.

Examples:
    logmind timeline                              # brief, to stdout
    logmind timeline --full                       # full per-decision
    logmind timeline --write docs/timeline.md     # brief, on disk
    logmind timeline --write docs/timeline.md --check  # CI gate`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runTimeline(cwd, writePath, check, full, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&writePath, "write", "",
		"Write the rendered timeline to PATH (typically docs/timeline.md). "+
			"Without this flag, prints to stdout.")
	cmd.Flags().BoolVar(&check, "check", false,
		"Exit nonzero if writing would change the file. Used in CI to fail "+
			"the build before regen so the auto-commit step runs and updates the PR.")
	cmd.Flags().BoolVar(&full, "full", false,
		"Render the legacy per-decision format (one bullet per entry). "+
			"Default is brief (v0.5.4+): first + last entry per month with a "+
			"`... N more decisions ...` elision line — token-frugal on disk.")
	return cmd
}

func runTimeline(cwd, writePath string, check, full bool, stdout, stderr io.Writer) error {
	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		// Python: click.secho fg=red + sys.exit(1). secho defaults to
		// stdout — we match. Then ErrSilent triggers exit 1.
		fmt.Fprintln(stdout, "Error: docs/ directory not found. Run 'logmind init' first.")
		return ErrSilent
	}
	brief := !full
	// Model dispatch: main-canonical (§1.6.4) when opted in via config, else
	// the byte-stable default.
	canonical := canonicalEnabled(cwd)
	mode := "brief"
	if full {
		mode = "full"
	}
	if canonical {
		// Main-canonical is single-format (entry-block); --full is inert.
		mode = "canonical"
	}

	if check {
		if writePath == "" {
			// Python sys.exit(2); we exit 1 via ErrSilent. Stdout
			// message is byte-identical so consumers see the same
			// diagnostic.
			fmt.Fprintln(stdout, "Error: --check requires --write PATH to compare against.")
			return ErrSilent
		}
		rendered, err := timeline.GenerateFor(docsPath, brief, canonical, stderr)
		if err != nil {
			return err
		}
		existing, _ := os.ReadFile(writePath)
		if string(existing) != rendered {
			fmt.Fprintf(stdout, "✗ %s is stale — re-run `logmind timeline --write %s` and commit.\n", writePath, writePath)
			return ErrSilent
		}
		fmt.Fprintf(stdout, "✓ %s is up to date\n", writePath)
		fmt.Fprintf(stdout, "ok timeline: %s up to date\n", writePath)
		return nil
	}

	if writePath == "" {
		rendered, err := timeline.GenerateFor(docsPath, brief, canonical, stderr)
		if err != nil {
			return err
		}
		// Python: _orig_click_echo(rendered, nl=False) — no trailing
		// newline added beyond what render produces. Render already
		// ends with `\n`, so the next-line ok still starts on its own
		// line.
		fmt.Fprint(stdout, rendered)
		// utf-8 byte count. Python uses len(rendered.encode('utf-8'))
		// for the same reason — em-dashes/non-ASCII would otherwise
		// undercount. Go strings ARE UTF-8 bytes, so len() works.
		fmt.Fprintf(stdout, "ok timeline: %d bytes (stdout, %s)\n", len(rendered), mode)
		return nil
	}

	rendered, err := timeline.GenerateFor(docsPath, brief, canonical, stderr)
	if err != nil {
		return err
	}
	existing, _ := os.ReadFile(writePath)
	changed := string(existing) != rendered
	if changed {
		if err := writeAtomic(writePath, rendered); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ Regenerated %s\n", writePath)
	} else {
		fmt.Fprintf(stdout, "  %s already up to date\n", writePath)
	}
	st, err := os.Stat(writePath)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ok timeline: %s (%d bytes, %s)\n", writePath, st.Size(), mode)
	return nil
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// writeAtomic writes data to path via a .tmp + rename so a crashed
// process can't leave a half-written file at the target. Mirror of
// Python's atomic_io.atomic_write_text.
func writeAtomic(path, data string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// canonicalEnabled reports whether the repo at cwd has opted into the
// main-canonical timeline (`timeline.canonical: main-canonical`). Fail-safe:
// a missing or broken config degrades to false (the byte-stable default), so
// no regen path can fail or silently flip on a config error. Shared by
// runTimeline and the in-process init re-render calls.
func canonicalEnabled(cwd string) bool {
	cfg, err := config.Load(cwd)
	return err == nil && cfg.Timeline.IsMainCanonical()
}
