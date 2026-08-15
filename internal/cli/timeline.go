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
	var half string
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Print or regenerate the high-level decision timeline",
		Long: `Print or regenerate the high-level decision timeline.

Reads docs/decisions.md, docs/decisions-archive.md (where they exist), and
every docs/decisions-branches/*.md as sources; renders a chronological
timeline grouped by year-month. Sources are never modified.

--write renders BOTH halves of the SPEC §3.3 split from one read of those
sources: the 50 most recent entries to the given PATH, and everything older to
timeline-archive.md NEXT TO IT. Neither file is ever read to decide what the
other holds, so the two are a split in a rendering, not a move — and the pair
always lands together, in one directory, the one you named.

--half writes exactly ONE of them to PATH and touches nothing else. That is
what the merge drivers use, where PATH is git's scratch file for a single
conflicted path and a sibling write would leave a stray behind.

Examples:
    logmind timeline                              # to stdout
    logmind timeline --write docs/timeline.md     # on disk (+ the archive)
    logmind timeline --write docs/timeline.md --check  # CI gate
    logmind timeline --write %A --half archive    # merge driver, archive only`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runTimeline(cwd, writePath, half, check, quietEnabled(cmd), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&writePath, "write", "",
		"Write the rendered timeline to PATH (typically docs/timeline.md), and "+
			"the entries older than the 50 most recent to timeline-archive.md in "+
			"the same directory. Without this flag, prints to stdout.")
	cmd.Flags().StringVar(&half, "half", "",
		"Restrict --write/--check to ONE half of the SPEC §3.3 split: "+
			"\"recent\" (the file at PATH) or \"archive\" (everything older). "+
			"Default: both, as a pair.")
	cmd.Flags().BoolVar(&check, "check", false,
		"Exit nonzero if the file at --write or its sibling timeline-archive.md "+
			"is stale (would differ from a fresh render), without writing either.")
	// --full is accepted but ignored as of v2.0.0: the timeline is now a
	// single format (the main-canonical entry-block union), so there is no
	// brief/full distinction to select. Kept registered so existing scripts,
	// hooks, and merge-driver invocations that still pass `--full` don't error.
	cmd.Flags().BoolVar(&full, "full", false,
		"No-op (kept for backward compatibility). The timeline is single-format as of v2.0.0.")
	return cmd
}

// Timeline halves — the two renderings of the SPEC §3.3 split. `--half`
// selects one; the empty value means both, as a pair, which is what every
// human and CI caller wants.
const (
	halfBoth    = ""
	halfRecent  = "recent"
	halfArchive = "archive"
)

// archiveSiblingPath is where the older half of the §3.3 split goes when
// --write names the recent half: timeline-archive.md in the SAME directory.
//
// It follows --write rather than being pinned to <cwd>/docs, and that is the
// whole point. Pinned, `logmind timeline --write /somewhere/else.md` rendered
// the file it was asked for AND silently rewrote the repo's tracked
// docs/timeline-archive.md on the way past — a side-channel write no caller
// asked for and none could see coming. It fired on every merge-driver run,
// which invokes `--write <git scratch file>`: the driver reported
// "✓ Regenerated .../docs/timeline-archive.md" while merging a temp file.
// A command writes where it is told, and its derived sibling lands next to
// what it wrote.
func archiveSiblingPath(writePath string) string {
	return filepath.Join(filepath.Dir(writePath), "timeline-archive.md")
}

func runTimeline(cwd, writePath, half string, check, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	switch half {
	case halfBoth, halfRecent, halfArchive:
	default:
		q.fail("Error: --half must be %q or %q (omit it for both).\n", halfRecent, halfArchive)
		return ErrSilent
	}
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

	// Where each half of the §3.3 split lands. Without --half, --write
	// produces the PAIR — the recent half at the path the caller named, the
	// archive half beside it — and there is no way to ask for one without the
	// other, so neither can go stale while the other is refreshed. "Both
	// regenerate together" is structural, not a convention every call site has
	// to remember. --half is the one deliberate exception: it collapses the
	// pair onto the single file --write named and writes no sibling at all,
	// which is what a merge driver needs when PATH is git's scratch file for
	// one conflicted path. An empty target means "this half is not this
	// invocation's business".
	recentTarget, archiveTarget := writePath, archiveSiblingPath(writePath)
	switch half {
	case halfRecent:
		archiveTarget = ""
	case halfArchive:
		recentTarget, archiveTarget = "", writePath
	}

	if check {
		if writePath == "" {
			// Python sys.exit(2); we exit 1 via ErrSilent. Stdout
			// message is byte-identical so consumers see the same
			// diagnostic (quiet routes it to stderr).
			q.fail("Error: --check requires --write PATH to compare against.\n")
			return ErrSilent
		}
		rendered, renderedArchive, err := timeline.Generate(docsPath, stderr)
		if err != nil {
			return err
		}
		if recentTarget != "" {
			existing, _ := os.ReadFile(recentTarget)
			if string(existing) != rendered {
				q.fail("✗ %s is stale — re-run `logmind timeline --write %s` and commit.\n", recentTarget, writePath)
				return ErrSilent
			}
		}
		if archiveTarget != "" {
			existingArchive, _ := os.ReadFile(archiveTarget)
			if string(existingArchive) != renderedArchive {
				q.fail("✗ %s is stale — re-run `logmind timeline --write %s` and commit.\n", archiveTarget, writePath)
				return ErrSilent
			}
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
		// stdout mirrors docs/timeline.md — the bounded view, so an agent
		// that runs the command sees exactly what it would have read.
		rendered, _, err := timeline.Generate(docsPath, stderr)
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

	rendered, renderedArchive, err := timeline.Generate(docsPath, stderr)
	if err != nil {
		return err
	}
	changed := false
	// Each half is written unconditionally where it is in play, including when
	// the archive body is empty: the recent half links to it, so the file has
	// to exist for that link to resolve (and for the restore paths and the
	// check-derived-docs gate to have something to pin).
	writeHalf := func(target, body string) error {
		if target == "" {
			return nil
		}
		existing, _ := os.ReadFile(target)
		if string(existing) == body {
			q.chat("  %s already up to date\n", target)
			return nil
		}
		if err := writeAtomic(target, body); err != nil {
			return err
		}
		changed = true
		q.chat("✓ Regenerated %s\n", target)
		return nil
	}
	if err := writeHalf(recentTarget, rendered); err != nil {
		return err
	}
	if err := writeHalf(archiveTarget, renderedArchive); err != nil {
		return err
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
