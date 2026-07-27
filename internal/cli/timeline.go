package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/atomicio"
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
    logmind timeline                              # to stdout
    logmind timeline --write docs/timeline.md     # on disk
    logmind timeline --write docs/timeline.md --check  # CI gate`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runTimeline(cwd, writePath, check, quietEnabled(cmd), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&writePath, "write", "",
		"Write the rendered timeline to PATH (typically docs/timeline.md). "+
			"Without this flag, prints to stdout.")
	cmd.Flags().BoolVar(&check, "check", false,
		"Exit nonzero if the file at --write is stale (would differ from a "+
			"fresh render), without writing it.")
	// --full is accepted but ignored as of v2.0.0: the timeline is now a
	// single format (the main-canonical entry-block union), so there is no
	// brief/full distinction to select. Kept registered so existing scripts,
	// hooks, and merge-driver invocations that still pass `--full` don't error.
	cmd.Flags().BoolVar(&full, "full", false,
		"No-op (kept for backward compatibility). The timeline is single-format as of v2.0.0.")
	return cmd
}

func runTimeline(cwd, writePath string, check, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		// Python: click.secho fg=red + sys.exit(1). secho defaults to
		// stdout — we match (quiet routes it to stderr). Then ErrSilent
		// triggers exit 1.
		q.fail("Error: docs/ directory not found. Run 'logmind init' first.\n")
		return ErrSilent
	}
	// v2.0.0: main-canonical (§1.6.4) is the sole, unconditional model. The
	// timeline is single-format (entry-block union) — `mode` stays a stable
	// receipt token so the `ok timeline …` trailer keeps its shape.
	const mode = "canonical"

	if check {
		if writePath == "" {
			// Python sys.exit(2); we exit 1 via ErrSilent. Stdout
			// message is byte-identical so consumers see the same
			// diagnostic (quiet routes it to stderr).
			q.fail("Error: --check requires --write PATH to compare against.\n")
			return ErrSilent
		}
		rendered, err := timeline.Generate(docsPath, stderr)
		if err != nil {
			return err
		}
		existing, _ := os.ReadFile(writePath)
		if string(existing) != rendered {
			q.fail("✗ %s is stale — re-run `logmind timeline --write %s` and commit.\n", writePath, writePath)
			return ErrSilent
		}
		q.chat("✓ %s is up to date\n", writePath)
		if quiet {
			q.ok("timeline path=%s mode=%s state=up-to-date", writePath, mode)
		} else {
			fmt.Fprintf(stdout, "ok timeline: %s up to date\n", writePath)
		}
		return nil
	}

	if writePath == "" {
		rendered, err := timeline.Generate(docsPath, stderr)
		if err != nil {
			return err
		}
		// Default mode prints the rendered timeline itself (the payload the
		// agent asked for) then the trailer. Quiet mode suppresses the body
		// — an agent that opted into quiet wants only the receipt.
		if !quiet {
			// Python: _orig_click_echo(rendered, nl=False) — no trailing
			// newline added beyond what render produces. Render already
			// ends with `\n`, so the next-line ok still starts on its own
			// line.
			fmt.Fprint(stdout, rendered)
			// utf-8 byte count. Python uses len(rendered.encode('utf-8'))
			// for the same reason — em-dashes/non-ASCII would otherwise
			// undercount. Go strings ARE UTF-8 bytes, so len() works.
			fmt.Fprintf(stdout, "ok timeline: %d bytes (stdout, %s)\n", len(rendered), mode)
		} else {
			q.ok("timeline bytes=%d mode=%s sink=stdout", len(rendered), mode)
		}
		return nil
	}

	rendered, err := timeline.Generate(docsPath, stderr)
	if err != nil {
		return err
	}
	existing, _ := os.ReadFile(writePath)
	changed := string(existing) != rendered
	if changed {
		if err := writeAtomic(writePath, rendered); err != nil {
			return err
		}
		q.chat("✓ Regenerated %s\n", writePath)
	} else {
		q.chat("  %s already up to date\n", writePath)
	}
	st, err := os.Stat(writePath)
	if err != nil {
		return err
	}
	if quiet {
		q.ok("timeline path=%s bytes=%d mode=%s changed=%t", writePath, st.Size(), mode, changed)
	} else {
		fmt.Fprintf(stdout, "ok timeline: %s (%d bytes, %s)\n", writePath, st.Size(), mode)
	}
	return nil
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// writeAtomic writes data to path via a temp file + rename so a
// crashed process can't leave a half-written file at the target.
// Mirror of Python's atomic_io.atomic_write_text. Thin string-typed
// wrapper around the shared internal/atomicio.WriteFile primitive,
// which every caller here routes through.
//
// atomicio.WriteFile gets a unique, random-suffixed temp name
// (os.CreateTemp) rather than a fixed `path+".tmp"` — a fixed name
// was a race all its own: two concurrent writeAtomic calls targeting
// the SAME path (e.g. two `logmind log` processes racing on
// docs/decisions.md) both wrote to the identical tmp file, so one
// process's os.Rename could fire on bytes the other process had
// already written, or on a tmp file the other process had already
// renamed away — producing crashes like `rename ...tmp ...: no such
// file or directory` and/or a renamed file that silently belongs to
// the wrong writer. A unique temp name per call removes that
// collision for EVERY caller of writeAtomic (log.go, headline.go,
// doctor.go, and this file's own --write path), not just the caller
// that happened to trigger it first.
//
// This alone does not make concurrent writers to the same target
// file safe from lost updates (last-rename-wins can still silently
// drop one writer's read-modify-write) — see log.go's
// acquireRepoLock for the cross-process lock that closes that half
// of the bug.
func writeAtomic(path, data string) error {
	// 0644 matches the historical permission of every file this
	// replaces (decisions.md, docs/timeline.md, etc.) — unchanged
	// from before the atomicio consolidation.
	return atomicio.WriteFile(path, []byte(data), 0o644)
}
