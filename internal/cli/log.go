// log.go — `logmind log` subcommand. Go port of src/logmind/cli.py's
// log command (Python `@main.command()` at cli.py:673-822) folded
// together with src/logmind/core/logger.py's log() entry point.
//
// Deferred from Phase B3 — the Python shim has carried this surface
// through v1.0 and v1.1. v1.2.0 lands the native Go port AND, per
// plan §8.7, builds Layer 1 of 3-layer markdown self-healing on top:
// after writing + committing the decision, the binary runs
// `linkcheck.Check()` and, when the gate finds issues, enters an
// interactive retry loop (TTY-gated) that gives the user up to three
// attempts to fix and re-check.
//
// Behavior overview:
//
//   - Branch routing (SPEC §3.2). A decision goes in a file named for the
//     branch it was made on — docs/decisions-branches/<sanitized-branch>.md
//     — and the default branch is not an exception to that rule. Only a
//     directory resolveDecisionsPath cannot resolve a branch name to route
//     by (non-git — including an unreachable git binary — or detached HEAD)
//     or an explicit branch_aware:false falls back to docs/decisions.md. An
//     unborn repo is NOT one of them: see resolveDecisionsPath.
//
//   - First-write backlink header. When creating a branch decision file
//     for the first time, prepends `← back to [docs/timeline.md]` so
//     timeline ↔ branch decision linking is bidirectional. Subsequent
//     appends preserve the header verbatim and only add the new entry.
//
//   - Auto-commit. Unless `--no-commit`, runs `git add -A` (or
//     `git add <paths>` when --stage scoped) + `git commit`. The
//     commit message follows config.git.commit_message_template,
//     defaulting to `logmind: <decision>`. Per the user-memory note
//     `feedback_logmind_stage_scoped.md`, the default is `--stage all`
//     to avoid silently dropping unstaged work.
//
//   - Self-heal (Layer 1). After commit, `linkcheck.CheckWithReport()`
//     runs against the repo. Clean → exit 0. Issues found AND stderr is
//     a TTY (SPEC §3.1.1, issue #220) AND `--no-interactive` not set →
//     enter retry loop (max 3 tries: y re-checks, n exits 0 with warning,
//     q exits 1). Issues found AND not interactive → print advisory to
//     stderr and exit 0 (CI's check-doc-links workflow Layer 3 catches it).
//
//   - Pulse (v2.0.0). After everything above, `emitPulse` (pulse.go)
//     prints ZERO, ONE, or TWO repo-health advisory lines to STDERR: a
//     doctor-drift pulse and/or a spec-staleness pulse. Unconditional
//     across TTY / non-TTY / --quiet — stderr sits outside both the §3.1
//     stdout contract and the --quiet single-`ok`-line contract. See
//     pulse.go for the exact line formats and failure-safety wrapper.
//
// SPEC §3.1 stdout contract (reconciled, salvaged from retired #154):
//
// On success stdout's first three lines are, byte-exact:
//
//	ℹ Staging all changes (use --stage scoped to limit)
//	✓ Logged decision: "<summary>"
//	✓ Committed and pushed changes
//
// (line 3 reads "✓ Committed changes" when push was suppressed via
// --no-push / git.auto_push: false, or when the push itself failed —
// e.g. no upstream configured.) TTY-interactive invocations MAY print
// additional advisory lines AFTER these three (SPEC §3.1.1); non-TTY
// and --no-interactive invocations emit exactly the three lines. See
// runSelfHealLayer1 and nudgeBranchSummary for the TTY-gated extras.
//
// --no-push (this port, closing the gap the file docstring used to
// carry as "out of scope"): suppresses the auto-push step. gitcli.Push
// wraps the underlying `git push`.
//
// No rotation, no archive (SPEC §3.2). Every decision file is append-only
// and uncapped: "a decision written is a decision kept." What is bounded is
// the VIEW — docs/timeline.md renders the 50 most recent entries and
// docs/timeline-archive.md the remainder (§3.3, internal/timeline) — and
// that split is a rendering, so nothing here moves, peels or overflows.
//
// Out of scope for v1.2.0 (carried until v1.3.x or folded into a follow-up):
//
//   - `--template` flag for pre-filled reasoning/alternatives/impl
//     (low-traffic; users craft their own entries with -r/-a/-i).
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/gitcli"
	"github.com/thrillmade/logmind/internal/linkcheck"
	"github.com/thrillmade/logmind/internal/templates"
	"github.com/thrillmade/logmind/internal/timeline"
)

// isTerminalFunc is the indirection seam used by tests to fake TTY
// detection. Production points it at the os.Stat-based heuristic in
// isStderrTerminal; tests override it directly.
//
// We avoid a hard dep on golang.org/x/term to keep the binary's
// import graph small and the dep matrix predictable (term changes the
// minimum-Go version on every other minor release).
var isTerminalFunc = isStderrTerminal

// isStderrTerminal reports whether STDERR (fd 2) is connected to a
// terminal. SPEC §3.1.1 gates the interactive extras (the branch-summary
// nudge, the Layer-1 self-heal retry loop) on isatty(STDERR), NOT stdin
// (issue #220): those extras are written for a human WATCHING the terminal,
// and stderr is the stream that stays a TTY even when stdout is captured
// (`logmind log ... > out.txt`) or stdin is redirected (`... < answers`).
// Keying the gate off stdin mis-classified `logmind log > file` (stdin
// still a TTY) as interactive and, worse, let a redirected-stdin-but-TTY
// session miss the extras it should show.
//
// Pure stdlib implementation — checks the device mode of fd 2. The two
// cases that matter:
//
//   - Interactive shell: stderr is /dev/ttyN (Unix) or CONOUT$ (Win) →
//     ModeCharDevice set → returns true.
//   - Piped / redirected / captured: stderr is a pipe or file →
//     ModeCharDevice unset → returns false. Examples: `logmind log ...
//     2>err.txt`, CI runners that capture stderr.
//
// Falsing out on stat errors is the conservative default — better to
// behave as non-interactive (skip prompts, exit 0) than to block a
// scripted invocation waiting on stdin that will never deliver.
func isStderrTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	mode := fi.Mode()
	if (mode & os.ModeCharDevice) != 0 {
		return true
	}
	return false
}

// logFlags carries the parsed flags for `logmind log`. Mirrored on
// the Python click signature but skips fields not yet ported (see
// file-level docstring "Out of scope" list).
type logFlags struct {
	reasoning     string
	alternatives  []string
	implications  []string
	noCommit      bool
	noPush        bool
	noInteractive bool
	stage         string
	// headline, when set, becomes the branch's one-sentence timeline summary
	// (the §1.6.3 marker headline) — the bundled form of `logmind headline`.
	// Empty leaves the marker on its deterministic default / prior value.
	headline string
}

// newLogCmd wires the `logmind log <summary>` subcommand.
//
// Help text mirrors Python's click docstring + flag descriptions so
// users moving between the Python shim and the Go binary see the same
// guidance via `logmind log --help`.
func newLogCmd() *cobra.Command {
	f := &logFlags{stage: "all"}
	cmd := &cobra.Command{
		Use:   "log <summary>",
		Short: "Log a decision to the decision log + self-heal markdown links",
		Long: `Log a decision to the decision log.

Writes a dated entry (summary + reasoning + alternatives + implications) to
docs/decisions-branches/<sanitized-branch>.md — the file named for the branch
you are on, with the default branch treated like any other. When creating a
branch decision file for the first time, prepends a backlink header pointing
at docs/timeline.md so the two files cross-link bidirectionally.

After committing (unless --no-commit), pushes (unless --no-push or
git.auto_push: false in .logmind/config.yml), then runs linkcheck.Check()
against the repo. If issues are found, prints an advisory + (when stderr is
a TTY) enters an interactive retry loop giving you up to 3 attempts to fix
and re-check. Non-interactive contexts (CI, captured stderr, --no-interactive)
print the advisory and exit 0 — the check-doc-links workflow's Layer 3
self-heal will catch any leftover issues at PR time.

Example:
    logmind log "Use PostgreSQL for database" \
        -r "Need ACID compliance" \
        -a "MongoDB" -a "SQLite" \
        -i "Need connection pooling"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runLog(cwd, args[0], f, quietEnabled(cmd), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVarP(&f.reasoning, "reasoning", "r", "",
		"Why this decision was made.")
	cmd.Flags().StringArrayVarP(&f.alternatives, "alternative", "a", nil,
		"Alternative option considered (repeatable).")
	cmd.Flags().StringArrayVarP(&f.implications, "implication", "i", nil,
		"Implication of this decision (repeatable).")
	cmd.Flags().BoolVar(&f.noCommit, "no-commit", false,
		"Don't auto-commit the decision (write the file only).")
	cmd.Flags().BoolVar(&f.noPush, "no-push", false,
		"Don't auto-push after committing. Honored alongside .logmind/config.yml's git.auto_push (either suppresses the push).")
	cmd.Flags().BoolVar(&f.noInteractive, "no-interactive", false,
		"Skip the Layer 1 interactive retry prompt; print advisory + exit 0 when linkcheck has issues.")
	cmd.Flags().StringVar(&f.stage, "stage", "all",
		"What to stage in the decision commit: 'all' (default) sweeps the working tree, 'scoped' stages only the decision file(s).")
	cmd.Flags().StringVarP(&f.headline, "headline", "H", "",
		"Set/refresh the branch's one-sentence timeline summary (main-canonical only). Bundled form of `logmind headline`.")
	return cmd
}

// runLog is the testable core. Takes explicit cwd + IO writers so
// in-process tests can drive the command without touching the global
// os.Stdin / os.Stdout.
//
// Returns nil on success, ErrSilent for user-facing errors already
// printed to stdout, or a regular error for plumbing failures.
func runLog(cwd, summary string, f *logFlags, quiet bool, stdin io.Reader, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		q.fail("Error: docs/ directory not found. Run 'logmind init' first.\n")
		return ErrSilent
	}

	if strings.TrimSpace(summary) == "" {
		q.fail("Error: decision summary is empty.\n")
		return ErrSilent
	}

	// Stage validation — only accept the two documented values. Mirror
	// Python click.Choice behavior: print the actual user input + the
	// allowed set so a typo is easy to fix.
	if f.stage != "all" && f.stage != "scoped" {
		q.fail("Error: invalid --stage %q (allowed: all, scoped).\n", f.stage)
		return ErrSilent
	}

	// Reasoning is required by the AGENTS.md contract — silent decisions
	// defeat the point. Allow empty for parity with Python's optional
	// flag, but warn so agents get the nudge. We don't error out:
	// log_first_decision passes reasoning, and follow-up logs often
	// only carry a summary in scripted use.
	//
	// The warning names the GATE, not just the lost value, because the
	// consequence is now concrete: SPEC §3.4 requires `check-decisions`
	// to reject an entry whose reasoning section is empty ("A decision
	// clears the gate by being written, not by existing"). So a
	// reasoning-less entry does not merely lose value — it fails to
	// clear the merge gate for any change over the threshold, and the
	// author finds out in CI rather than here.
	//
	// Deliberately still a warning and not an error. SPEC §3.1 says an
	// entry "MUST carry a title and a timestamp, and SHOULD carry the
	// reasoning", and states outright that "an entry of title plus `---`
	// is well-formed" — so refusing to write one would over-implement
	// the record format. §3.1 and §3.4 disagree about this; raised as
	// thrillmade/protocol#93. Until that resolves, logmind writes what
	// §3.1 permits and warns about what §3.4 will do to it.
	if strings.TrimSpace(f.reasoning) == "" {
		fmt.Fprintln(stderr, "Warning: -r/--reasoning is empty. Decision logs without reasoning lose most of their value,")
		fmt.Fprintln(stderr, "         and the check-decisions gate rejects a reasoning-less entry (SPEC §3.4) — so this")
		fmt.Fprintln(stderr, "         entry will NOT clear CI for a change above the line threshold.")
	}

	cfg, _ := config.Load(cwd)

	// Commit/push gating. Commit keeps its existing --no-commit-only gate
	// (cfg.Git.AutoCommit is a pre-existing, still-unwired config knob —
	// out of scope here). Push is the new surface: CLI flag OR config can
	// suppress it, matching git.auto_push's documented semantics.
	shouldCommit := !f.noCommit
	shouldPush := shouldCommit && cfg.Git.AutoPush && !f.noPush

	// SPEC §3.1 line 1 of 3 — stage notice, the first line of the
	// required stdout contract. The --stage all wording is the SPEC's
	// own §3.1 example, verbatim; --stage scoped has no SPEC example so
	// this is this port's extrapolation (flagged in the PR description).
	// Gated on BOTH a pending commit AND actually being in a git repo:
	// --no-commit stages nothing, and outside a git repo the commit is
	// skipped too (see the AddAll block below), so in neither case does
	// any staging occur — the notice would be a lie.
	if shouldCommit && gitcli.IsRepo(cwd) {
		if f.stage == "all" {
			q.chat("ℹ Staging all changes (use --stage scoped to limit)\n")
		} else {
			q.chat("ℹ Staging decision file only (--stage scoped)\n")
		}
	}

	// Resolve target file based on git state + config.branch_aware.
	target, isBranchFile := resolveDecisionsPath(cwd, docsPath, cfg)

	// Serialize concurrent `logmind log` invocations against this repo.
	// Without this, two concurrent logs both read the pre-write content
	// of `target`, both append their own entry in memory, and the last
	// writeAtomic call wins — silently dropping every other concurrent
	// decision (and, if a commit follows, committing a diff that
	// doesn't match its own message). Acquired before the read below
	// and held through the write + the git add/commit further down so
	// the eventual commit reflects exactly what was written; released
	// right after that commit/push block (see the matching Unlock
	// call) so self-heal + the pulse advisories below don't pay for
	// holding it. See filelock.go for why this is a repo-scoped lock
	// rather than a per-target-file one.
	lock, err := acquireRepoLock(cwd)
	if err != nil {
		return fmt.Errorf("logmind log: %w", err)
	}
	locked := true
	defer func() {
		if locked {
			_ = lock.Unlock()
		}
	}()

	// Detect first-creation: a branch file that doesn't exist yet gets
	// the backlink header. Default-branch decisions.md is created by
	// `logmind init` so this path doesn't trigger for it.
	firstCreation := isBranchFile && !pathExists(target)

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create decisions dir: %w", err)
	}

	// Build the entry. Format is byte-identical to Python's
	// logger._format_decision so old + new tools render the same shape.
	entry := buildDecisionEntry(summary, f.reasoning, f.alternatives, f.implications)

	// Read existing content (empty for first creation).
	var existing []byte
	if !firstCreation && pathExists(target) {
		data, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read %s: %w", target, err)
		}
		existing = data
	}

	// Compose: header (if first-creation branch file) + the §1.6.3 timeline
	// marker (unconditional since v2.0) + existing + entry. Header is the
	// templates.DecisionsBranchHeader() POSIX-terminated single line +
	// trailing blank line.
	var body strings.Builder
	if firstCreation {
		body.WriteString(templates.DecisionsBranchHeader())
	}
	// Open the branch detail page with its timeline marker (the PR headline
	// the main-canonical union consumes). First creation emits it right after
	// the header; a later append inserts one only if the file has none yet
	// (e.g. a legacy file predating markers). Routed through
	// branchSummaryApplies — the ONE owner of "does the summary surface apply
	// here" — so this write, `logmind headline`, and the nudge below cannot
	// drift apart. They had: this site gated on isBranchFile alone while the
	// other two also required a non-default branch, so on main `logmind log
	// -H "x"` set the headline and `logmind headline "x"` refused to.
	if branchSummaryApplies(isBranchFile) {
		now := time.Now()
		prSuffix := prSuffixFromEnv()
		// The headline is the branch SUMMARY when --headline is given, else the
		// decision summary (the deterministic default until the agent refines
		// it via the nudge or `logmind headline`).
		headlineText := summary
		if f.headline != "" {
			headlineText = f.headline
		}
		switch {
		case firstCreation:
			body.WriteString(buildTimelineMarker(now, headlineText, prSuffix))
		case !timeline.HasEntryBlocks(string(existing)):
			existing = insertMarkerAfterHeader(existing, buildTimelineMarker(now, headlineText, prSuffix))
		case f.headline != "":
			// Marker already present + an explicit --headline → refresh the
			// visible line, keeping the stable <date>-<slug> key.
			if replaced, ok := timeline.ReplaceFirstHeadline(string(existing), f.headline, prSuffix); ok {
				existing = []byte(replaced)
			}
		}
	}
	body.Write(existing)
	body.WriteString(entry)

	if err := writeAtomic(target, body.String()); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}

	relTarget, err := filepath.Rel(cwd, target)
	if err != nil {
		relTarget = target
	}
	relTarget = filepath.ToSlash(relTarget)

	// SPEC §3.1 line 2 of 3 — byte-exact `✓ Logged decision: "<summary>"`.
	// %q quotes + escapes the summary the same way Go would for any
	// embedded control characters; a plain-text summary renders as a
	// simple double-quoted string, matching the SPEC example exactly.
	//
	// The Go-quoted rendering is a DELIBERATE, unambiguous escape choice:
	// SPEC §3.1 shows plain surrounding quotes, but a summary that itself
	// contains a `"` or `\` would make the naive `"%s"` form ambiguous
	// (unparseable back to the original). %q is the safe superset —
	// identical to the plain form for ordinary summaries — and no §6.6
	// fixture pins the naive spelling, so this stays conformant.
	q.chat("✓ Logged decision: %q\n", summary)

	// Branch-summary nudge — steer the author toward a clean one-sentence
	// summary of the WHOLE branch (the timeline headline the next agent reads).
	// Skipped when --headline was just set (already current). Runs BEFORE the
	// commit so a TTY edit lands in the SAME commit. Gated to a branch file.
	// Best-effort: never fails the log. Suppressed under QUIET — an agent that
	// opted into terse output doesn't want the multi-line nudge.
	//
	// SPEC §3.1.1 ordering (issue #206, "reorder the code"): the interactive
	// prompt I/O all happens on STDERR here (a human at a TTY still sees it;
	// stderr conventionally carries prompts so stdout stays a clean contract
	// stream), which keeps stdout byte-exact through required line 2. The edit
	// is applied to disk here so it's captured by the commit below — but the
	// human-facing STDOUT confirmation is deferred until AFTER required line 3
	// (see summaryEdited below the commit block), so any nudge stdout appears
	// strictly after the three §3.1 lines rather than between lines 2 and 3.
	// The non-TTY / --no-interactive path is unchanged: it emits the advisory
	// to stderr only and returns false, so its stdout stays exactly the three
	// lines (SPEC §3.1.1's carve-out for the fixture-relevant case).
	// One shared stdin line-reader for BOTH the interactive nudge (below) and
	// the self-heal loop (further down): only one goroutine ever drains stdin,
	// so a timed-out nudge read can't steal the human's first self-heal answer
	// (#233). stdinOK gates whether we may BLOCK on stdin at all (finding #3:
	// stderr a TTY but stdin a never-delivering pipe). Both are cheap to compute
	// and harmless on the non-interactive paths (the reader's goroutine starts
	// lazily on the first prompt read, which those paths never reach).
	lines := newStdinLines(stdin)
	stdinOK := stdinReadable(stdin)

	// The unprompted ask, which is narrower than the capability: see
	// branchSummaryNudgeApplies for why a prompt stays off the default branch
	// while `logmind headline` and -H do not. Skipped when --headline already
	// supplied the sentence, and under --quiet, where there is nobody to ask.
	summaryEdited := false
	if branchSummaryNudgeApplies(cwd, isBranchFile) && f.headline == "" && !quiet {
		summaryEdited = nudgeBranchSummary(target, f.noInteractive, stdinOK, lines, stderr)
	}

	// Commit + push (unless --no-commit OR not in a git repo). SPEC §3.1
	// line 3 of 3 — byte-exact "✓ Committed and pushed changes" when the
	// push actually lands, else byte-exact "✓ Committed changes" (push
	// suppressed via --no-push / git.auto_push: false, OR the push
	// itself failed — e.g. no upstream on a fresh local repo).
	committed := false
	pushed := false
	if shouldCommit && gitcli.IsRepo(cwd) {
		if err := commitDecision(cwd, target, relTarget, f.stage, summary, cfg); err != nil {
			// Commit failure isn't fatal to the decision-logging
			// surface: the file is on disk; the user can commit
			// manually. Surface the error to stderr so the user knows
			// to follow up.
			fmt.Fprintf(stderr, "Warning: auto-commit failed: %v\n", err)
		} else {
			committed = true
			if shouldPush {
				if err := gitcli.Push(cwd); err != nil {
					// Push failure is non-fatal — the commit already
					// landed locally. Surface to stderr so the user
					// knows to retry the push manually; stdout still
					// reports the (accurate) "Committed changes" line.
					fmt.Fprintf(stderr, "Warning: auto-push failed: %v\n", err)
				} else {
					pushed = true
				}
			}
			if pushed {
				q.chat("✓ Committed and pushed changes\n")
			} else {
				q.chat("✓ Committed changes\n")
			}
		}
	} else if shouldCommit {
		fmt.Fprintln(stderr, "Warning: not inside a git repo; skipping auto-commit.")
	}

	// SPEC §3.1.1 extra, AFTER required line 3 (issue #206): the branch-summary
	// confirmation for an interactive TTY edit. The edit itself was applied to
	// disk BEFORE the commit above, so it's already captured by that commit —
	// this line is only the human-facing receipt, deferred to here so nothing
	// from the nudge lands on stdout between required lines 2 and 3. Only the
	// interactive path (isTerminalFunc + a non-empty reply) sets summaryEdited,
	// which is why this never touches the byte-exact non-TTY stdout contract.
	if summaryEdited {
		q.chat("✓ Branch summary updated.\n")
	}

	// Release the repo lock now — the write + commit/push sequence it
	// was guarding is done. Everything below (self-heal, the pulse
	// advisories) only reads/analyzes the repo; it doesn't need to
	// hold up other concurrent `logmind log` invocations.
	_ = lock.Unlock()
	locked = false

	// Layer 1 self-heal — runs whether we committed or not. The file is
	// on disk either way, so a stale link introduced by this decision
	// should surface immediately. Under QUIET the advisory is routed to
	// stderr (never onto the single-ok-line stdout) and the prompt is
	// skipped.
	if err := runSelfHealLayer1(cwd, f.noInteractive, quiet, stdinOK, lines, stdout, stderr); err != nil {
		// runSelfHealLayer1 returns ErrSilent on user abort (`q` reply)
		// or three failed retries. Propagate so the CLI exits non-zero
		// and the agent sees the abort signal.
		return err
	}

	// QUIET receipt — the single chainable summary line. Default mode keeps
	// its historical multi-line ✓ output (no `ok` trailer) for byte parity;
	// this line is the quiet MODE's sole stdout output.
	if quiet {
		q.ok("logged path=%s committed=%t pushed=%t", relTarget, committed, pushed)
	}

	// v2.0.0 pulse — repo-health advisories, STDERR ONLY, always last on
	// stderr. See pulse.go for the two probes (doctor drift + spec
	// staleness) and the failure-safety wrapper. Runs unconditionally in
	// every mode (TTY, non-TTY, --quiet): stderr sits outside both the
	// §3.1 stdout contract above and the --quiet single-`ok`-line contract,
	// so emitting here never touches either.
	emitPulse(cwd, stderr)

	return nil
}

// resolveDecisionsPath implements SPEC §3.2's ONE path rule: a decision goes
// in a file named for the branch it was made on, and the default branch is
// not an exception to it. Returns the target file path AND a bool indicating
// whether it's a branch-specific file (used to decide whether to write the
// backlink header + timeline marker on first creation).
//
//	on any branch (main included), branch_aware on
//	  → docs/decisions-branches/<sanitized>.md, isBranchFile=true
//	where the router cannot resolve a branch name to route by — non-git
//	(including an unreachable git binary), or detached HEAD — or
//	branch_aware is explicitly off
//	  → docs/decisions.md, isBranchFile=false
//
// Exactly three code paths below reach docs/decisions.md, and that is the
// whole list: branch_aware off, non-git, detached HEAD. They are not a
// default-branch special case; they are the states where the router cannot
// resolve a branch name to name a file after — not always because no name
// exists. IsRepo's exit code conflates "not a repository" with "the `git`
// binary itself is unreachable", so the non-git case can fire inside a real
// repo on a real branch; branch_aware:false is a policy opt-out, not an
// absence either — a name may well exist, it is simply not consulted. Only
// detached HEAD is genuinely nameless. `main` used to be routed here too,
// which is what made the main log look like a second kind of decision file
// with its own conventions; that case is gone, and main's decisions live in
// main's own branch file.
//
// An UNBORN repo is NOT one of the three, and the intuition that it is has
// been written into this comment before. `git symbolic-ref --short HEAD`
// resolves HEAD's ref without dereferencing it to a commit, so it SUCCEEDS
// before the first commit — a fresh `git init` answers with the branch HEAD
// already points at (e.g. `main`, exit 0) even though `git rev-parse
// --verify HEAD` fails. CurrentBranch is therefore non-empty and the first
// decision in an empty repo lands in docs/decisions-branches/main.md, not
// docs/decisions.md. Detached HEAD is the case that yields "" — symbolic-ref
// exits non-zero there because HEAD holds a raw SHA, not a ref.
// Both halves are pinned by TestResolveDecisionsPathUnbornVsDetached.
func resolveDecisionsPath(cwd, docsPath string, cfg config.Config) (target string, isBranchFile bool) {
	branchlessPath := filepath.Join(docsPath, "decisions.md")
	if !cfg.Decisions.BranchAware {
		return branchlessPath, false
	}
	if !gitcli.IsRepo(cwd) {
		return branchlessPath, false
	}
	branch := gitcli.CurrentBranch(cwd)
	if branch == "" {
		// Detached HEAD. NOT an unborn repo — symbolic-ref answers `main`
		// there; see this function's doc comment.
		return branchlessPath, false
	}
	branchFile := filepath.Join(docsPath, "decisions-branches",
		sanitizeBranchName(branch)+".md")
	return branchFile, true
}

// DELIBERATELY ABSENT: a defaultBranchDecisionsPath helper.
//
// A read path that wants the repo's accumulated history — not just the
// working branch's — must ENUMERATE the decision files
// (decisions.ListSources), never build one out of a resolved branch name.
// `search` did the latter, and gitcli.DefaultBranch's fallback chain ends
// "single-branch repo → that branch IS the default → 'main'": wherever
// origin/HEAD is unset (a `git clone --single-branch`, an `actions/checkout`
// working copy, every locally-created repo), the name collapsed onto the
// current branch or onto a main.md that does not exist, and the default
// branch's file was dropped from the read while sitting on disk. Enumeration
// cannot miss a file that exists. See decisions.ListSources.
//
// sanitizeBranchName mirrors Python's logger._sanitize_branch:
// `/` → `__`, `\` → `__`, `:` → `_`. Everything else passes through
// (most VCS-legal branch names use those three plus dashes and dots,
// both of which are already filesystem-safe).
func sanitizeBranchName(name string) string {
	name = strings.ReplaceAll(name, "/", "__")
	name = strings.ReplaceAll(name, "\\", "__")
	name = strings.ReplaceAll(name, ":", "_")
	return name
}

// buildDecisionEntry renders the per-decision markdown block. Format
// byte-identical to Python's logger._format_decision so files written
// by the Go binary parse cleanly by old decisions.Iter + new
// timeline.Generate code paths.
//
// Layout:
//
//	## YYYY-MM-DD HH:MM - <summary>
//
//	**Reasoning:** <reasoning>
//
//	**Alternatives considered:** alt1, alt2, alt3
//
//	**Implications:**
//	- impl1
//	- impl2
//
//	---
//
// Sections without content are skipped (no empty `**Reasoning:** `
// line). Trailing `---` + newline terminator separates from the next
// entry.
func buildDecisionEntry(summary, reasoning string, alternatives, implications []string) string {
	now := time.Now().Format("2006-01-02 15:04")
	var b strings.Builder
	fmt.Fprintf(&b, "## %s - %s\n\n", now, summary)

	if strings.TrimSpace(reasoning) != "" {
		fmt.Fprintf(&b, "**Reasoning:** %s\n\n", reasoning)
	}

	if len(alternatives) > 0 {
		fmt.Fprintf(&b, "**Alternatives considered:** %s\n\n",
			strings.Join(alternatives, ", "))
	}

	if len(implications) > 0 {
		b.WriteString("**Implications:**\n")
		for _, impl := range implications {
			fmt.Fprintf(&b, "- %s\n", impl)
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n\n")
	return b.String()
}

// buildTimelineMarker renders the §1.6.3 entry-block headline that opens a
// branch detail page — the one per-PR row the main-canonical timeline unions.
// The body is timeline.HeadlineLine (link-free; the union adds the detail
// link from the source path). Per §1.6.3.1 the (#NN) PR suffix appears in the
// visible line ONLY, never in the <date>-<slug> key. A trailing blank line
// separates the marker from the decision entries below.
func buildTimelineMarker(date time.Time, headline, prSuffix string) string {
	key := date.Format("2006-01-02") + "-" + timeline.Slugify(headline)
	line := timeline.HeadlineLine(date, headline+prSuffix)
	return fmt.Sprintf("<!-- logmind-entry-start: %s -->\n%s\n<!-- logmind-entry-end -->\n\n", key, line)
}

// prSuffixFromEnv returns " (#NN)" when LOGMIND_PR is set (CI exports it),
// else "". Tolerates a leading "#". Kept env-only so `logmind log` stays
// fast + offline — no `gh` network call on the hot path.
func prSuffixFromEnv() string {
	v := strings.TrimPrefix(strings.TrimSpace(os.Getenv("LOGMIND_PR")), "#")
	if v == "" {
		return ""
	}
	return " (#" + v + ")"
}

// insertMarkerAfterHeader splices marker in right after the branch-file
// backlink header, preserving the rest of the file. If the header isn't found
// (a hand-edited file), the marker is prepended — never silently dropped.
func insertMarkerAfterHeader(existing []byte, marker string) []byte {
	header := templates.DecisionsBranchHeader()
	s := string(existing)
	if strings.HasPrefix(s, header) {
		return []byte(s[:len(header)] + marker + s[len(header):])
	}
	return append([]byte(marker), existing...)
}

// nudgeBudget is the TOTAL wall-clock the interactive branch-summary nudge may
// spend across BOTH of its stdin reads (the y/N prompt AND the "New summary:"
// prompt), sharing ONE deadline. runLog holds the repo lock across the whole
// nudge — the edit must land in the same commit — so an unbounded (or a
// per-read) wait on a human who steps away could keep the lock past
// lockAcquireTimeout (15s) and make a concurrent `logmind log` in the same cwd
// fail with the misleading "appears stuck" (#228, #233). Bounding each read
// SEPARATELY at nudgeBudget was the bug: two reads could hold the lock ~2×
// budget. One shared deadline (computed once, drawn down by both reads) caps
// the whole nudge — and thus the lock hold — at nudgeBudget. 12s leaves a
// comfortable margin under the 15s lock window even if a human pauses at each
// prompt; on timeout the nudge keeps the current headline and falls back to the
// stderr advisory. A package var so tests can shrink it.
var nudgeBudget = 12 * time.Second

// nudgeSummaryAdvisory prints the stderr-only branch-summary advisory: the
// current one-sentence summary plus how to refresh it asynchronously via
// `logmind headline`. Shown to non-interactive callers (agents / CI) and to an
// interactive caller whose prompt timed out (#228). In both cases stdout is
// left untouched, so the byte-exact §3.1 stdout contract holds.
func nudgeSummaryAdvisory(current string, stderr io.Writer) {
	fmt.Fprintf(stderr, "\n📝 Branch summary: %s\n", current)
	fmt.Fprintln(stderr, "   Keep it a one-sentence summary of the whole branch — refresh with: logmind headline \"<one sentence>\"")
}

// stdinLineResult carries one line (or the terminal read error) from the
// single stdin-draining goroutine to its consumers.
type stdinLineResult struct {
	line string
	err  error // io.EOF or a read error once the stream is exhausted; nil otherwise
}

// stdinLines is a single-goroutine line reader SHARED by the interactive
// branch-summary nudge and the Layer-1 self-heal retry loop. Exactly one
// goroutine ever touches stdin: it drains '\n'-terminated lines, in order, into
// a buffered channel. Two properties matter:
//
//   - No stolen answers (#233). Pre-fix, the nudge and the self-heal loop each
//     wrapped os.Stdin in their OWN bufio.Reader. When a nudge read timed out,
//     its goroutine stayed parked on ReadString; the next byte the human typed
//     was consumed by THAT parked reader (and discarded to a channel nobody
//     drained), so the self-heal loop's second reader blocked forever. With one
//     shared reader every line flows through one channel and is handed to
//     whoever reads NEXT — the self-heal loop receives the human's first
//     answer, never the abandoned nudge.
//
//   - Lazy start. The goroutine starts on the FIRST read, so non-interactive
//     invocations (which never prompt) never touch stdin and the byte-exact
//     non-TTY stdout contract is untouched.
//
// started/done/doneErr are only ever read/written by the single consumer
// goroutine (runLog runs the nudge, then the self-heal loop, sequentially), so
// they need no synchronization; the channel carries the only cross-goroutine
// handoff. A late line delivered after a timed-out read stays buffered in the
// channel and is picked up by the next reader rather than leaking on the send.
type stdinLines struct {
	r       *bufio.Reader
	ch      chan stdinLineResult
	started bool
	done    bool
	doneErr error
}

func newStdinLines(stdin io.Reader) *stdinLines {
	return &stdinLines{r: bufio.NewReader(stdin), ch: make(chan stdinLineResult, 1)}
}

func (s *stdinLines) start() {
	if s.started {
		return
	}
	s.started = true
	go func() {
		for {
			line, err := s.r.ReadString('\n')
			s.ch <- stdinLineResult{line: line, err: err}
			if err != nil {
				return
			}
		}
	}()
}

// next waits at most d for the next line. Returns (line, true) when a line (or
// the final EOF chunk) arrives in time, or ("", false) on timeout. A
// non-positive d is treated as an immediate timeout — this is what lets the
// nudge share ONE deadline across its two reads (#233): once the budget is
// spent the second read returns instantly instead of arming a fresh full
// timeout. The terminal read error (io.EOF / worse) is returned alongside so
// callers can distinguish a real reply from a closed stream.
func (s *stdinLines) next(d time.Duration) (line string, ok bool, err error) {
	s.start()
	if s.done {
		return "", true, s.doneErr
	}
	if d <= 0 {
		return "", false, nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case res := <-s.ch:
		if res.err != nil {
			s.done, s.doneErr = true, res.err
		}
		return res.line, true, res.err
	case <-timer.C:
		return "", false, nil
	}
}

// nextBlocking waits INDEFINITELY for the next line. The self-heal loop uses it
// because a human may take arbitrarily long to go fix docs before replying —
// bounding that read would regress the UX (#220-follow-up). The unbounded wait
// is safe only because runLog gates entry to the loop on stdinReadable (a
// never-delivering pipe never gets here); see runSelfHealLayer1.
func (s *stdinLines) nextBlocking() (line string, err error) {
	s.start()
	if s.done {
		return "", s.doneErr
	}
	res := <-s.ch
	if res.err != nil {
		s.done, s.doneErr = true, res.err
	}
	return res.line, res.err
}

// stdinReadable reports whether stdin can safely serve a block-and-wait
// interactive prompt. #220 moved the interactivity gate to isatty(STDERR) so
// that `logmind log > out.txt` (stdout captured) still shows a human the
// prompts — but the self-heal loop's reply read is UNBOUNDED, so with stderr a
// TTY and stdin an open-but-never-delivering pipe the read would hang where the
// old isatty(STDIN) gate exited 0. Output routing stays keyed on isatty(STDERR)
// per §3.1.1; this gate only decides whether we may BLOCK on stdin waiting for
// a reply.
//
// A terminal (char device) or a regular file always makes progress — a human
// replies or sends EOF, a file delivers bytes then EOF — so both are safe to
// block on. A pipe / socket / fifo can block forever with no data and no EOF,
// so we decline to wait on those and fall through to the non-interactive
// advisory instead. A non-*os.File reader (in-process tests, or an embedder
// that injected its own stream) is trusted — the caller wired it deliberately.
func stdinReadable(stdin io.Reader) bool {
	f, ok := stdin.(*os.File)
	if !ok {
		return true
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	mode := fi.Mode()
	return mode&os.ModeCharDevice != 0 || mode.IsRegular()
}

// nudgeBranchSummary steers the author toward a clean one-sentence branch
// summary after a log. At a TTY (and with a readable stdin) it offers an
// interactive edit: the prompt I/O goes to STDERR (SPEC §3.1.1 / issue #206 —
// stdout stays the clean contract stream) and the edit is written before the
// caller commits, so it lands in the SAME commit; it returns true so the caller
// prints the stdout confirmation AFTER required line 3. The y/N prompt and the
// "New summary:" prompt SHARE a single nudgeBudget deadline (#228, #233): two
// per-read timeouts could hold the repo lock ~2× the budget and blow past
// lockAcquireTimeout (15s), reintroducing the "appears stuck" failure a
// concurrent `logmind log` sees — one shared deadline caps the whole nudge (and
// thus the lock) under budget. On timeout it keeps the current headline and
// falls back to the advisory. For an agent (non-TTY), a --no-interactive
// caller, or a stdin we can't safely block on (finding #3), it prints the
// advisory and returns false — a blocking prompt can't reach those callers, so
// they act asynchronously via `logmind headline`. The stdin reads share
// runLog's single stdinLines reader so a timed-out read here can't steal the
// self-heal loop's first answer (#233). Best-effort: any IO error just skips
// the nudge; it never fails the log.
func nudgeBranchSummary(target string, forceNonInteractive, stdinOK bool, lines *stdinLines, stderr io.Writer) bool {
	data, err := os.ReadFile(target)
	if err != nil {
		return false
	}
	current, ok := timeline.CurrentHeadline(string(data))
	if !ok {
		return false
	}

	if forceNonInteractive || !isTerminalFunc() || !stdinOK {
		// Agent / CI / non-readable stdin: advisory only — never block on
		// stdin, and the nudge MUST go to stderr so stdout stays byte-identical
		// to the three §3.1 log lines (SPEC §3.1.1: the non-TTY headline nudge
		// is stderr-only).
		nudgeSummaryAdvisory(current, stderr)
		return false
	}

	// Interactive TTY: offer an inline edit. Prompts go to stderr (§3.1.1 /
	// #206). BOTH reads draw down ONE shared deadline (#233) so the repo lock
	// runLog holds across this nudge can never be held ≥ lockAcquireTimeout.
	deadline := time.Now().Add(nudgeBudget)
	fmt.Fprintf(stderr, "\nBranch summary: %s\n", current)
	fmt.Fprint(stderr, "Update it to a one-sentence summary of the whole branch? [y to edit / N to keep]: ")
	line, ok, _ := lines.next(time.Until(deadline))
	if !ok {
		// Human stepped away — keep the current headline + fall back to the
		// advisory so the caller releases the lock and commits promptly (#228).
		nudgeSummaryAdvisory(current, stderr)
		return false
	}
	if strings.ToLower(strings.TrimSpace(line)) != "y" {
		return false
	}
	fmt.Fprint(stderr, "New summary: ")
	// Second read draws down the SAME deadline (remaining budget), not a fresh
	// one — if the y/N reply consumed most of the budget this returns promptly.
	newSummary, ok, _ := lines.next(time.Until(deadline))
	if !ok {
		nudgeSummaryAdvisory(current, stderr)
		return false
	}
	newSummary = strings.TrimSpace(newSummary)
	if newSummary == "" {
		fmt.Fprintln(stderr, "  (empty — kept the current summary)")
		return false
	}
	updated, replaced := timeline.ReplaceFirstHeadline(string(data), newSummary, prSuffixFromEnv())
	if !replaced {
		return false
	}
	if err := writeAtomic(target, updated); err != nil {
		fmt.Fprintf(stderr, "  (could not write summary: %v)\n", err)
		return false
	}
	// Edit applied on disk (captured by the pending commit). The caller prints
	// the stdout "✓ Branch summary updated." receipt AFTER required line 3.
	return true
}

// commitDecision stages the relevant files and creates the commit.
// Mirrors Python's auto_commit path inside logger.log():
//
//   - stage=all → git add -A (sweeps the working tree)
//   - stage=scoped → git add <target> (only the decision file itself.
//     docs/timeline.md + docs/file-structure.md are deliberately NEVER
//     added here, scoped or not: under the zero-conflict invariant
//     (see the restore call below) they are purely-derived, main-only
//     artifacts, so a branch commit must never carry a local edit to
//     either one. This is permanent, not a gap to close later.)
//
// Commit message uses cfg.Git.CommitMessageTemplate with `{decision}`
// substituted for the summary. Default is `logmind: <summary>`.
func commitDecision(cwd, targetAbs, targetRel, stage, summary string, cfg config.Config) error {
	// Zero-conflict invariant (v2.0.0): docs/timeline.md +
	// docs/file-structure.md are purely-derived, main-only artifacts (see
	// internal/cli/derived.go). On a non-default branch, restore them to
	// their committed (HEAD) content BEFORE staging so neither a stray hook
	// regen nor `git add -A` can sweep a branch-local edit into this commit
	// — which would diverge the branch from main and cause a future merge
	// conflict. Lossless: they regenerate deterministically from the
	// decision files (which ARE committed, per-branch, and never conflict).
	// On the default branch this is intentionally skipped — main is where
	// the derived docs SHOULD be current. Unconditional as of the removal
	// of the v2.0.0 B6 `derived_docs.mode` adoption gate — every repo gets
	// this restore, not just ones that opted in.
	//
	// Restore target is HEAD, NOT the merge-base with the default branch
	// (v2.0.0 4b-ter reversal of the short-lived 4b-bis "repair-path fix"):
	// 4b-bis pointed this restore at gitcli.DefaultBranchMergeBase to
	// self-heal an already-diverged branch, but that target depends on
	// refs/remotes/origin/<default> being CURRENT — and nothing on this
	// path ever refreshes it. `logmind log` is deliberately network-free
	// (no implicit `git fetch`), so on a clone that hasn't fetched recently,
	// the "merge-base" computed here is stale, and this restore would
	// silently commit an OLDER snapshot than the branch's true merge-base —
	// actively writing WRONG bytes, and typically causing the very CI gate
	// this restore exists to satisfy to FAIL. Restoring to HEAD has no such
	// dependency: it only ever needs to know what's already committed
	// locally, so it is correct offline and always. The tradeoff, accepted
	// deliberately: this restore can no longer repair a branch whose HEAD
	// already carries a diverged copy (an old binary's local regen, or a
	// hand edit) — it re-affirms whatever's there instead. That's fine: L1's
	// job is narrower than "repair" — it only has to keep an ALREADY-clean
	// branch clean, i.e. stop divergence from ARRIVING via a stray hook
	// regen or `git add -A` sweep. Repairing a branch that has ALREADY
	// diverged needs a trustworthy, freshly-fetched `origin/<default>` to
	// compute a correct merge-base against — `logmind warp` is the one
	// surface that fetches first, so that's where the repair now lives (see
	// runWarp in warp.go). See TestLog_CommitPathDoesNotDependOnOriginRef
	// (derived_repair_test.go) for the regression pin: deform
	// refs/remotes/origin/<default> and confirm a clean branch's commit is
	// unaffected.
	//
	// v2.0.0 4b-quater — the L1-vs-`warp` seam, and its fix: moving the
	// repair to `logmind warp` (above) reintroduced the exact bug that move
	// was meant to avoid, one level up. `warp`'s repair DELIBERATELY STAGES
	// the derived docs (`git checkout <merge-base> -- <path>`, which
	// writes the index too — see runWarp) so the fix rides into the
	// caller's NEXT commit. But this restore ran unconditionally, so the
	// remediation sequence the CI gate's own failure message tells a user
	// to run — `logmind warp` then `logmind log` — silently no-op'd: warp
	// repairs + stages, then THIS restore checked the path back out to
	// HEAD's still-divergent content, undoing the repair before the `git
	// add` below could even run, and the resulting commit captured the
	// divergence AGAIN. Same error, same bytes, no indication the "fix"
	// did nothing. Root cause: this restore couldn't tell a deliberate
	// staged repair apart from an accidental unstaged dirty copy, so it
	// undid both.
	//
	// The fix: restore only the paths gitcli.IsPathStaged (via
	// unstagedDerivedDocPaths, derived.go) reports as NOT already staged.
	// Rule, in one line: unstaged means accidental → revert it; staged
	// means intentional → leave it alone — that's what staging means
	// everywhere else in git. This works here specifically because this
	// restore runs BEFORE commitDecision's OWN staging step (`git add -A` /
	// `git add <target>`, immediately below): the only way a derived doc
	// can already be staged at this point is a prior, SEPARATE action —
	// chiefly `logmind warp` — never something this function itself just
	// did. (guardCommitHarness's L2b restore in guard_commit.go shares this
	// property — it fires before the pending Bash command even runs — and
	// applies the identical filter. The pre-commit git hook, L2a, does NOT:
	// it fires from `git commit` itself, AFTER whatever staged the index
	// for THAT commit, so "already staged" there is the normal state of
	// anything about to be committed, not a reliable intent signal — see
	// BuildPreCommitBody's doc comment in internal/hooks/hooks.go for the
	// full reasoning.) IMPORTANT CAVEAT: L2a is a REAL `.git/hooks/pre-commit`
	// script, so it fires for EVERY `git commit` in this repo — including
	// the one THIS function's own gitcli.Commit call makes a few lines
	// below. A repo with the hook installed (unconditional, the moment
	// `logmind init`/`doctor --fix` runs with git enabled — see init.go)
	// therefore still runs L2a's unconditional restore-to-HEAD on this exact
	// commit, AFTER this function's own restore already (correctly) left a
	// warp-staged repair alone — undoing it right back. Closing THAT
	// coupling is unresolved, out-of-scope follow-up work (it needs a signal
	// from this call site to L2a, e.g. an environment variable set only
	// around this specific `git commit`, that a POSIX-sh hook could check
	// for and stand down on); see
	// BuildPreCommitBody's doc comment (internal/hooks/hooks.go) for the
	// full account. Everything this comment block otherwise describes is
	// still correct and tested (TestWarpThenLog_PreservesRepairAcrossCommit
	// below passes) precisely because that test's fixtures — like every
	// other fixture in this package — never install a REAL pre-commit hook;
	// see initClonePairScaffolded / scaffoldDocs, which run `logmind init
	// --no-git` and so skip hook installation entirely.
	//
	// THE TRADE-OFF, ACCEPTED DELIBERATELY: this relaxes L1. If a user
	// hand-stages a DIVERGENT derived doc — `git add docs/timeline.md`
	// after editing it directly, rather than trusting `logmind warp` to
	// produce a correct one — and then runs `logmind log`, L1 now skips it
	// and the divergence gets committed. There's no cheap way around this:
	// the only signal available on this offline hot path is "does the
	// index differ from HEAD", which cannot distinguish "staged because
	// warp repaired it correctly" from "staged because a human added a bad
	// copy" — and deliberately NOT closed with a merge-base comparison,
	// because the merge-base is unavailable here for the exact staleness
	// reason this restore targets HEAD instead of the merge-base in the
	// first place (see above): computing one needs a freshly-fetched
	// origin/<default>, which nothing on this network-free hot path ever
	// refreshes. L3 (the CI check-derived-docs gate) remains the backstop
	// for this residual gap, same as it always was for a genuinely
	// diverged branch.
	//
	// See TestWarpThenLog_PreservesRepairAcrossCommit (derived_repair_test.go)
	// for the regression pin: the full warp-then-log remediation sequence,
	// asserting the resulting commit carries the merge-base content warp
	// staged, not the divergent HEAD content this restore used to
	// re-affirm. See TestLog_DoesNotCommitDirtiedDerivedDocOnBranch for
	// proof L1's original job — reverting an UNSTAGED dirty derived doc —
	// still holds unchanged.
	if onNonDefaultBranch(cwd) {
		_ = gitcli.RestorePathsToHead(cwd, unstagedDerivedDocPaths(cwd, derivedDocPaths)...)
	}

	if stage == "all" {
		if err := gitcli.AddAll(cwd); err != nil {
			return err
		}
	} else {
		if err := gitcli.AddPaths(cwd, targetRel); err != nil {
			return err
		}
	}

	tmpl := cfg.Git.CommitMessageTemplate
	if tmpl == "" {
		tmpl = "logmind: {decision}"
	}
	message := strings.ReplaceAll(tmpl, "{decision}", summary)

	if err := gitcli.Commit(cwd, message); err != nil {
		return err
	}
	return nil
}

// runSelfHealLayer1 implements the Layer 1 advisory + interactive
// retry loop per plan §8.7. Returns ErrSilent on user abort (`q`
// reply) or 3 failed retries; nil otherwise.
//
// Flow:
//
//   - Run linkcheck.CheckWithReport(). Clean → exit 0 silently.
//   - Issues found, NOT interactive (--no-interactive OR no stderr TTY OR a
//     stdin we can't safely block on — see stdinReadable / finding #3):
//     print advisory + nudge that CI Layer 3 will catch it + exit 0.
//   - Issues found, interactive: print advisory + enter retry loop.
//     Up to 3 attempts of y/n/q.
//
// The retry loop reads replies from the shared stdinLines reader (the same one
// the branch-summary nudge used) so a timed-out nudge read can't steal the
// human's first answer here (#233); the read is unbounded because a human may
// take arbitrarily long to go fix docs, and stdinOK guarantees stdin can't hang
// us (finding #3).
//
// The advisory format is the one the plan locked in §8.7:
//
//	⚠ Standard markdown links need attention (3 items):
//	  - docs/timeline.md is stale (...)
//	    → fix: ...
//	  - docs/install.md is orphan (...)
//	    → fix: ...
//	Fix the issues above, then reply [y] to re-check, [n] to skip, [q] to abort:
//	>
func runSelfHealLayer1(cwd string, forceNonInteractive, quiet, stdinOK bool, lines *stdinLines, stdout, stderr io.Writer) error {
	report, err := linkcheck.CheckWithReport(cwd, nil, nil)
	if err != nil {
		// Linkcheck plumbing failure isn't a self-heal-able error;
		// warn but don't block the log success.
		fmt.Fprintf(stderr, "Warning: linkcheck failed: %v\n", err)
		return nil
	}
	if !report.HasIssues() {
		return nil
	}

	// QUIET: the link advisory is a recovery hint, not chatter — emit it to
	// stderr (never onto the single-ok-line stdout) and skip the prompt. The
	// decision is saved; CI's Layer 3 will catch any leftover issues.
	if quiet {
		printAdvisory(stderr, report)
		fmt.Fprintln(stderr, "Non-interactive/quiet context — decision is saved but docs may be stale.")
		return nil
	}

	// Output routing stays keyed on isatty(STDERR) (§3.1.1 / #220), but we only
	// ENTER the block-and-wait retry loop when stdin is one we can safely block
	// on: the reply read below is UNBOUNDED (a human may take arbitrarily long
	// to go fix docs), so a stderr-TTY invocation whose stdin is a
	// never-delivering pipe would hang here without the stdinOK gate (finding
	// #3). A non-readable stdin falls through to the non-interactive advisory,
	// exactly where the old isatty(STDIN) gate exited 0.
	interactive := !forceNonInteractive && isTerminalFunc() && stdinOK

	if !interactive {
		// §3.1.1: non-TTY / --no-interactive / non-readable-stdin stdout MUST
		// stay exactly the three §3.1 lines. The advisory is a recovery hint,
		// not part of the contract — route it to stderr, mirroring the quiet
		// path above and the headline nudge (PR #173). Fixes issue #206(a).
		printAdvisory(stderr, report)
		fmt.Fprintln(stderr, "Non-interactive context — decision is saved but docs may be stale.")
		fmt.Fprintln(stderr, "The check-doc-links workflow's Layer 3 self-heal will catch this at PR time.")
		return nil
	}

	printAdvisory(stdout, report)

	const maxTries = 3
	// Reads come from runLog's SHARED stdinLines reader (not a fresh bufio.Reader
	// over stdin) so the branch-summary nudge above can't have left a parked
	// reader that swallows the human's first answer here (#233).
	for try := 1; try <= maxTries; try++ {
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, "Fix the issues above, then reply [y to re-check, n to skip, q to abort]: ")

		line, err := lines.nextBlocking()
		if err != nil && err != io.EOF {
			// Plumbing error reading stdin — fail open, exit 0.
			fmt.Fprintln(stderr, "Warning: stdin read failed:", err)
			return nil
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "y", "yes":
			report, err = linkcheck.CheckWithReport(cwd, nil, nil)
			if err != nil {
				fmt.Fprintf(stderr, "Warning: linkcheck failed: %v\n", err)
				return nil
			}
			if !report.HasIssues() {
				fmt.Fprintln(stdout, "✓ all links healthy")
				return nil
			}
			fmt.Fprintln(stdout)
			fmt.Fprintf(stdout, "Still has issues (attempt %d of %d).\n", try, maxTries)
			printAdvisory(stdout, report)
		case "n", "no":
			fmt.Fprintln(stdout, "Skipped — decision is saved; CI will catch any leftover issues.")
			return nil
		case "q", "quit", "abort":
			fmt.Fprintln(stdout, "Aborted — exit 1 so the caller knows.")
			return ErrSilent
		case "":
			// Empty reply (just Enter). Re-prompt without consuming a try.
			try-- // don't decrement past 0
			if try < 0 {
				try = 0
			}
		default:
			fmt.Fprintf(stdout, "Unrecognised reply %q. Expected y / n / q.\n", answer)
			try--
			if try < 0 {
				try = 0
			}
		}
		if errors.Is(err, io.EOF) {
			// stdin closed mid-loop (e.g., test driver). Treat as skip.
			return nil
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "3 attempts exhausted; please address remaining issues before pushing.\n")
	return ErrSilent
}

// printAdvisory renders the Layer 1 advisory format to whichever writer the
// caller hands it. Pulled out as a helper so it can be invoked both on
// initial detection and after each failed retry.
//
// The destination is NOT fixed to stdout: the interactive-TTY retry loop
// (runSelfHealLayer1) writes it to stdout, since that path is a live
// back-and-forth with a human at the terminal. Every non-interactive case —
// --quiet, --no-interactive, or no TTY on stdin — writes it to stderr
// instead, so the §3.1 stdout contract (three fixed lines) and the --quiet
// single-`ok`-line contract both stay byte-exact; the advisory is a
// recovery hint on those paths, not part of either contract (see issue
// #206(a) / PR #207). Callers choose the writer; this function has no
// opinion of its own.
func printAdvisory(stdout io.Writer, report linkcheck.CheckReport) {
	count := len(report.Broken) + len(report.Orphans)
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "⚠ Standard markdown links need attention (%d items):\n", count)
	for _, f := range report.Broken {
		fmt.Fprintf(stdout, "  - %s: %s\n", f.Path, f.Reason)
		if f.SuggestedFix != "" {
			fmt.Fprintf(stdout, "    %s\n", f.SuggestedFix)
		}
	}
	for _, f := range report.Orphans {
		fmt.Fprintf(stdout, "  - %s is orphan (%s)\n", f.Path, f.Reason)
		if f.SuggestedFix != "" {
			fmt.Fprintf(stdout, "    %s\n", f.SuggestedFix)
		}
	}
}
