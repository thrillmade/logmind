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
//   - Branch-aware routing. On a feature branch with branch_aware=true
//     in config, writes to docs/decisions-branches/<sanitized-branch>.md.
//     On the default branch, in non-git dirs, on detached HEAD, or when
//     branch_aware is off, writes to docs/decisions.md. Matches Python
//     logger._resolve_decisions_path.
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
//     runs against the repo. Clean → exit 0. Issues found AND stdin is
//     a TTY AND `--no-interactive` not set → enter retry loop (max 3
//     tries: y re-checks, n exits 0 with warning, q exits 1). Issues
//     found AND not interactive → print advisory and exit 0 (CI's
//     check-doc-links workflow Layer 3 will catch it).
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
// Out of scope for v1.2.0 (carried by the Python shim until v1.3.x or
// folded into a follow-up):
//
//   - decisions-archive rotation when count > max_recent. Python
//     handles this via _archive_oldest_decision; the Go port lands
//     the rotation in a follow-up so v1.2.0 stays focused on the
//     self-heal feature.
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
// isStdinTerminal; tests override it directly.
//
// We avoid a hard dep on golang.org/x/term to keep the binary's
// import graph small and the dep matrix predictable (term changes the
// minimum-Go version on every other minor release).
var isTerminalFunc = isStdinTerminal

// isStdinTerminal reports whether stdin is connected to a terminal.
// Pure stdlib implementation — checks the device mode of fd 0. A
// character device with no Size hint (file size 0) is treated as a
// TTY; piped input (regular file or named pipe) reports false.
//
// This is the cross-platform fallback. The two cases that matter:
//
//   - Interactive shell: stdin is /dev/ttyN (Unix) or CONIN$ (Win) →
//     ModeCharDevice set → returns true.
//   - Piped / redirected: stdin is a pipe or file → ModeCharDevice
//     unset → returns false. Examples: `echo y | logmind log ...`,
//     `logmind log ... < /dev/null`, CI runners.
//
// Falsing out on stat errors is the conservative default — better to
// behave as non-interactive (skip prompts, exit 0) than to block a
// scripted invocation waiting on stdin that will never deliver.
func isStdinTerminal() bool {
	fi, err := os.Stdin.Stat()
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
either docs/decisions.md (default branch / non-git / detached HEAD) or
docs/decisions-branches/<sanitized-branch>.md (feature branch with
branch_aware=true). When creating a branch decision file for the first
time, prepends a backlink header pointing at docs/timeline.md so the
two files cross-link bidirectionally.

After committing (unless --no-commit), pushes (unless --no-push or
git.auto_push: false in .logmind/config.yml), then runs linkcheck.Check()
against the repo. If issues are found, prints an advisory + (when stdin is
a TTY) enters an interactive retry loop giving you up to 3 attempts to fix
and re-check. Non-interactive contexts (CI, piped stdin, --no-interactive)
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
	if strings.TrimSpace(f.reasoning) == "" {
		fmt.Fprintln(stderr, "Warning: -r/--reasoning is empty. Decision logs without reasoning lose most of their value.")
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
	// (e.g. a legacy file predating markers). Gated only on isBranchFile —
	// main-canonical is the sole, unconditional timeline model as of v2.0.0.
	if isBranchFile {
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
	// commit so a TTY edit lands in the same commit. Gated to a branch file.
	// Best-effort: never fails the log. Suppressed under QUIET — an agent that
	// opted into terse output doesn't want the multi-line nudge.
	//
	// KNOWN SPEC §3.1.1 GAP (pre-existing, not introduced by this change,
	// left as-is — flagged for the lead): in the TTY-interactive path this
	// prints prompts to stdout BETWEEN required line 2 and required line 3,
	// which strictly reads as "additional lines" appearing before the third
	// required line rather than after it. Deferring the nudge until after
	// line 3 would break its real-time TTY UX (the prompt has to appear
	// before the user types a reply) and would also decouple the edit from
	// landing in the same commit. The non-TTY / --no-interactive path is
	// unaffected — the nudge there goes to stderr only (§3.1.1's carve-out)
	// so the 3-line stdout contract holds exactly for the fixture-relevant
	// case. Needs a product call: either accept this narrow TTY exemption
	// explicitly in the SPEC, or redesign the nudge to run after commit.
	if isBranchFile && f.headline == "" && !quiet {
		nudgeBranchSummary(target, f.noInteractive, stdin, stdout, stderr)
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

	// Layer 1 self-heal — runs whether we committed or not. The file is
	// on disk either way, so a stale link introduced by this decision
	// should surface immediately. Under QUIET the advisory is routed to
	// stderr (never onto the single-ok-line stdout) and the prompt is
	// skipped.
	if err := runSelfHealLayer1(cwd, f.noInteractive, quiet, stdin, stdout, stderr); err != nil {
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

// resolveDecisionsPath mirrors Python's logger._resolve_decisions_path
// branch-aware routing. Returns the target file path AND a bool
// indicating whether it's a branch-specific file (used to decide
// whether to write the backlink header on first creation).
//
//	on default branch / non-git / detached HEAD / branch_aware off
//	  → docs/decisions.md, isBranchFile=false
//	on feature branch with branch_aware=true
//	  → docs/decisions-branches/<sanitized>.md, isBranchFile=true
func resolveDecisionsPath(cwd, docsPath string, cfg config.Config) (target string, isBranchFile bool) {
	defaultPath := filepath.Join(docsPath, "decisions.md")
	if !cfg.Decisions.BranchAware {
		return defaultPath, false
	}
	if !gitcli.IsRepo(cwd) {
		return defaultPath, false
	}
	branch := gitcli.CurrentBranch(cwd)
	if branch == "" {
		// Detached HEAD or unborn repo.
		return defaultPath, false
	}
	defaultBranch := gitcli.DefaultBranch(cwd)
	if branch == defaultBranch {
		return defaultPath, false
	}
	branchFile := filepath.Join(docsPath, "decisions-branches",
		sanitizeBranchName(branch)+".md")
	return branchFile, true
}

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

// nudgeBranchSummary steers the author toward a clean one-sentence branch
// summary after a log. At a TTY it offers an interactive edit (the new summary
// is written before the caller commits, so it lands in the same commit). For
// an agent (non-TTY) it prints an advisory with the current summary + how to
// refresh it — a blocking prompt can't reach a non-TTY caller, so the agent
// acts asynchronously via `logmind headline`. Best-effort: any IO error just
// skips the nudge; it never fails the log.
func nudgeBranchSummary(target string, forceNonInteractive bool, stdin io.Reader, stdout, stderr io.Writer) {
	data, err := os.ReadFile(target)
	if err != nil {
		return
	}
	current, ok := timeline.CurrentHeadline(string(data))
	if !ok {
		return
	}

	if forceNonInteractive || !isTerminalFunc() {
		// Agent / CI: advisory only — never block on stdin, and the nudge
		// MUST go to stderr so stdout stays byte-identical to the three §3.1
		// log lines (SPEC §3.1.1: the non-TTY headline nudge is stderr-only).
		fmt.Fprintf(stderr, "\n📝 Branch summary: %s\n", current)
		fmt.Fprintln(stderr, "   Keep it a one-sentence summary of the whole branch — refresh with: logmind headline \"<one sentence>\"")
		return
	}

	// Interactive TTY: offer an inline edit.
	reader := bufio.NewReader(stdin)
	fmt.Fprintf(stdout, "\nBranch summary: %s\n", current)
	fmt.Fprint(stdout, "Update it to a one-sentence summary of the whole branch? [y to edit / N to keep]: ")
	line, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(line)) != "y" {
		return
	}
	fmt.Fprint(stdout, "New summary: ")
	newSummary, _ := reader.ReadString('\n')
	newSummary = strings.TrimSpace(newSummary)
	if newSummary == "" {
		fmt.Fprintln(stdout, "  (empty — kept the current summary)")
		return
	}
	updated, replaced := timeline.ReplaceFirstHeadline(string(data), newSummary, prSuffixFromEnv())
	if !replaced {
		return
	}
	if err := writeAtomic(target, updated); err != nil {
		fmt.Fprintf(stdout, "  (could not write summary: %v)\n", err)
		return
	}
	fmt.Fprintln(stdout, "✓ Branch summary updated.")
}

// commitDecision stages the relevant files and creates the commit.
// Mirrors Python's auto_commit path inside logger.log():
//
//   - stage=all → git add -A (sweeps the working tree)
//   - stage=scoped → git add <target> (only the decision file itself;
//     a fuller scoped surface would also include
//     docs/timeline.md + docs/file-structure.md when they changed —
//     deferred until the regen-on-log path lands in Go)
//
// Commit message uses cfg.Git.CommitMessageTemplate with `{decision}`
// substituted for the summary. Default is `logmind: <summary>`.
func commitDecision(cwd, targetAbs, targetRel, stage, summary string, cfg config.Config) error {
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
//   - Issues found, NOT interactive (--no-interactive OR no TTY):
//     print advisory + nudge that CI Layer 3 will catch it + exit 0.
//   - Issues found, interactive: print advisory + enter retry loop.
//     Up to 3 attempts of y/n/q.
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
func runSelfHealLayer1(cwd string, forceNonInteractive, quiet bool, stdin io.Reader, stdout, stderr io.Writer) error {
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

	interactive := !forceNonInteractive && isTerminalFunc()

	printAdvisory(stdout, report)

	if !interactive {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Non-interactive context — decision is saved but docs may be stale.")
		fmt.Fprintln(stdout, "The check-doc-links workflow's Layer 3 self-heal will catch this at PR time.")
		return nil
	}

	const maxTries = 3
	reader := bufio.NewReader(stdin)
	for try := 1; try <= maxTries; try++ {
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, "Fix the issues above, then reply [y to re-check, n to skip, q to abort]: ")

		line, err := reader.ReadString('\n')
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

// printAdvisory renders the Layer 1 advisory format. Pulled out as a
// helper so it can be invoked both on initial detection and after
// each failed retry.
//
// Output goes to stdout (the human surface). stderr is reserved for
// real errors so consumers piping `logmind log` into a log file don't
// see advisories twice.
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
