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

SPEC §3.3 splits that rendering in two: the 50 most recent entries
(docs/timeline.md) and everything older (docs/timeline-archive.md). Neither
file is ever read to decide what the other holds — the split is a cut in one
rendering, not a move, so no entry is ever transferred or consumed.

--write writes ONE file: the one PATH names, and nothing else. --half picks
which half of the split goes into it ("recent" is the default). Regenerating
both means naming both, which is what the hooks, the CI workflow, and the two
merge drivers all do — a command that also wrote a file the caller never named
would drop a stray beside git's merge scratch file, or silently overwrite a
tracked file of that name.

Examples:
    logmind timeline                                          # to stdout
    logmind timeline --write docs/timeline.md                 # recent half
    logmind timeline --write docs/timeline-archive.md --half archive
    logmind timeline --write docs/timeline.md --check         # CI gate
    logmind timeline --write %A --half archive                # merge driver`,
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
		"Write the rendered timeline to PATH (typically docs/timeline.md) — that "+
			"file and no other. Without this flag, prints to stdout.")
	cmd.Flags().StringVar(&half, "half", "",
		"Which half of the SPEC §3.3 split to render: \"recent\" (the 50 most "+
			"recent entries) or \"archive\" (everything older). Default: recent.")
	cmd.Flags().BoolVar(&check, "check", false,
		"Exit nonzero if the file at --write is stale (would differ from a fresh "+
			"render of the selected half), without writing it.")
	// --full is accepted but ignored as of v2.0.0: the timeline is now a
	// single format (the main-canonical entry-block union), so there is no
	// brief/full distinction to select. Kept registered so existing scripts,
	// hooks, and merge-driver invocations that still pass `--full` don't error.
	cmd.Flags().BoolVar(&full, "full", false,
		"No-op (kept for backward compatibility). The timeline is single-format as of v2.0.0.")
	return cmd
}

// Timeline halves — the two renderings of the SPEC §3.3 split. `--half`
// selects WHICH ONE goes into the file `--write` names; it never selects how
// MANY files are written, because that answer is always one.
//
// halfDefault (the flag omitted) is the recent half, so `--write PATH` alone
// means what it has meant since v1.x: render the timeline, put it at PATH.
const (
	halfDefault = ""
	halfRecent  = "recent"
	halfArchive = "archive"
)

// ONE --write, ONE file. There is no derived sibling, and adding one back is
// the bug this constant block sits above as a warning against.
//
// The command previously wrote a second file next to --write's target, and
// both ways of choosing that second path were wrong in their own direction:
//
//   - PINNED to <cwd>/docs: `logmind timeline --write /somewhere/else.md`
//     rendered the file it was asked for AND silently rewrote the repo's
//     tracked docs/timeline-archive.md on the way past. It fired on every
//     merge-driver run — the driver reported "✓ Regenerated
//     .../docs/timeline-archive.md" while merging a temp file.
//   - FOLLOWING --write: git hands a merge driver a scratch file at the
//     WORKTREE ROOT (%A is `.merge_file_XXXXXX`), so every timeline merge
//     dropped an untracked `timeline-archive.md` there. Untracked litter is
//     not where it ends: `logmind log` defaults to --stage all, so the very
//     next decision commits the stray and propagates it to every clone — and
//     in a repo that legitimately tracks a root-level timeline-archive.md,
//     the merge REPLACES the user's content and prints "✓ Regenerated
//     timeline-archive.md" as if that were a service.
//
// Both hazards are the same hazard: a write to a path no caller named. So the
// rule is the caller names every file that gets written. Regenerating the pair
// is two invocations (see internal/hooks and the regen-timeline workflow), and
// the `merge.logmind-timeline` command string — frozen, because an older
// binary on PATH executes it — needs no change to be safe under it, because
// `logmind timeline --write %A` now means exactly what it meant in v1.2.0.

func runTimeline(cwd, writePath, half string, check, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	switch half {
	case halfDefault, halfRecent, halfArchive:
	default:
		q.fail("Error: --half must be %q or %q (omit it for %q).\n", halfRecent, halfArchive, halfRecent)
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

	if check {
		if writePath == "" {
			// Python sys.exit(2); we exit 1 via ErrSilent. Stdout
			// message is byte-identical so consumers see the same
			// diagnostic (quiet routes it to stderr).
			q.fail("Error: --check requires --write PATH to compare against.\n")
			return ErrSilent
		}
		body, err := renderHalf(docsPath, half, stderr)
		if err != nil {
			return err
		}
		existing, _ := os.ReadFile(writePath)
		if string(existing) != body {
			// The advice has to be the command that FIXES this file, which
			// for the archive means carrying --half through. Advice that
			// names the wrong half is worse than none: run as printed, it
			// writes the recent timeline into the archive's path.
			q.fail("✗ %s is stale — re-run `logmind timeline --write %s%s` and commit.\n", writePath, writePath, halfFlagSuffix(half))
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
		// stdout mirrors the file the selected half would have been written
		// to — docs/timeline.md by default, so an agent that runs the command
		// bare sees exactly what it would have read.
		rendered, err := renderHalf(docsPath, half, stderr)
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

	body, err := renderHalf(docsPath, half, stderr)
	if err != nil {
		return err
	}
	// The selected half is written unconditionally, including when its body is
	// empty: docs/timeline.md links to the archive, so the archive file has to
	// exist for that link to resolve (and for the restore paths and the
	// check-derived-docs gate to have something to pin).
	existing, _ := os.ReadFile(writePath)
	changed := string(existing) != body
	if changed {
		if err := writeAtomic(writePath, body); err != nil {
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

// renderHalf generates BOTH renderings from one read of the sources and
// returns the one `half` selects. Both are always rendered because neither
// file is ever read to decide what the other holds (§3.3) — the cut is made in
// memory, from the sources, every time. Only one of them is ever written, by
// the single caller-named path.
func renderHalf(docsPath, half string, stderr io.Writer) (string, error) {
	recent, archive, err := timeline.Generate(docsPath, stderr)
	if err != nil {
		return "", err
	}
	if half == halfArchive {
		return archive, nil
	}
	return recent, nil
}

// halfFlagSuffix renders the `--half` the caller passed back into a command
// line, for remediation advice that reproduces THIS invocation. Empty when the
// flag was omitted, so the default advice stays the v1.x string agents already
// recognise.
func halfFlagSuffix(half string) string {
	if half == halfDefault {
		return ""
	}
	return " --half " + half
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
