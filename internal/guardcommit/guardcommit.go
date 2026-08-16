// Package guardcommit is the shared decision engine behind
// `logmind guard-commit` (internal/cli/guard_commit.go). It answers one
// question — "should this git commit be allowed to proceed without going
// through `logmind log`?" — and is deliberately kept free of cobra, stdin/
// stdout, and process-exit concerns so it is fully unit-testable and can be
// called from two different call sites with two different diff semantics:
//
//   - The git commit-msg hook (PR2, not built here) calls Evaluate with
//     DiffMode = StagedOnly, because by the time a commit-msg hook runs the
//     index is final — nothing more will be staged before the commit lands.
//   - The Claude Code harness PreToolUse hook (also PR2) calls Evaluate with
//     DiffMode = WorkingTreeUnion, because it fires BEFORE a Bash tool call
//     runs — including a compound `git add -A && git commit` shape where,
//     at evaluation time, the change under review is still sitting entirely
//     unstaged. StagedOnly would see an empty index and silently allow a
//     commit that is about to bypass enforcement; WorkingTreeUnion closes
//     that gap by unioning staged + unstaged-tracked + untracked changes.
//
// This package is pure logic: no install wiring, no hook registration, and
// no cobra command lives here. See internal/cli/guard_commit.go for the
// (Hidden, manually-invokable-only) CLI surface this feeds; the git
// commit-msg hook (internal/hooks) and the Claude Code PreToolUse hook
// (internal/claudehook) both shell out to `logmind guard-commit` and call
// it automatically on every commit.
package guardcommit

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thrillmade/logmind/internal/agents"
	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/gitcli"
)

// CarveOut names the specific reason Evaluate allowed a commit through
// without requiring a decision log. The empty string is used both for "no
// specific carve-out applies" (e.g. outside a git repo entirely — there's
// nothing to enforce) and as the Decision.CarveOut zero value on a Block.
type CarveOut string

const (
	// CarveOutNone means "allowed, but not via one of the named carve-outs
	// below" (currently: repoRoot isn't a git repo at all) — or, on a
	// Block decision, simply "no carve-out applied."
	CarveOutNone CarveOut = ""
	// CarveOutEnvAllow: LOGMIND_ALLOW_GIT_COMMIT is set truthy.
	CarveOutEnvAllow CarveOut = "env:LOGMIND_ALLOW_GIT_COMMIT"
	// CarveOutSkipLogmind: the commit subject contains the literal marker
	// "[skip-logmind]".
	CarveOutSkipLogmind CarveOut = "skip-logmind"
	// CarveOutRebaseInProgress: .git/rebase-apply or .git/rebase-merge
	// exists — git itself is mid-rebase and generates commits internally
	// (e.g. `rebase --continue`) that were never meant to go through
	// `logmind log`.
	CarveOutRebaseInProgress CarveOut = "rebase-in-progress"
	// CarveOutMergeInProgress: .git/MERGE_HEAD exists (mid `git merge`).
	CarveOutMergeInProgress CarveOut = "merge-in-progress"
	// CarveOutCherryPickOrRevert: .git/CHERRY_PICK_HEAD or
	// .git/REVERT_HEAD exists.
	CarveOutCherryPickOrRevert CarveOut = "cherry-pick-or-revert-in-progress"
	// CarveOutDecisionRecorded: the staged change WROTE a well-formed
	// §3.1 entry into a decision-log file — the commit IS the
	// documentation. Staging the file is not enough; see DecisionRecorded.
	//
	// The token reads `decision-recorded`, not the `decision-file-staged`
	// it was through v2.0. It is printed to a human ("✓ guard-commit:
	// allowed (decision-recorded)") and the old spelling names the
	// question this gate STOPPED asking — the one a content-free file
	// passed. Nothing parses it: the commit-msg hook branches on exit
	// status alone, and the line is suppressed under --quiet.
	CarveOutDecisionRecorded CarveOut = "decision-recorded"
	// CarveOutUnderThreshold: the computed substantive-line count is below
	// the configured threshold — too small to be worth a decision log.
	CarveOutUnderThreshold CarveOut = "under-threshold"
)

// DiffMode selects which git state Evaluate inspects to compute the
// substantive-line count. See the package doc for why the two hook layers
// need different modes.
type DiffMode int

const (
	// StagedOnly inspects only `git diff --cached` — what's in the index
	// right now. Correct for the git commit-msg hook layer.
	StagedOnly DiffMode = iota
	// WorkingTreeUnion inspects the union of staged, unstaged-tracked, and
	// untracked changes. Correct for the harness PreToolUse layer, which
	// runs before a compound `git add -A && git commit` has staged
	// anything.
	WorkingTreeUnion
)

// Decision is Evaluate's verdict.
type Decision struct {
	// Allow is true when the commit may proceed without a decision log.
	Allow bool
	// CarveOut names WHY (see the CarveOut constants). Empty on a Block,
	// or on the "not a git repo" Allow path.
	CarveOut CarveOut
	// Lines is the substantive-line count SubstantiveLines computed. Only
	// meaningful when the algorithm actually got as far as computing it
	// (the CarveOutUnderThreshold Allow case and every Block); earlier
	// carve-outs return before the diff is even read, so Lines is 0 there
	// — that's "not computed," not "zero lines changed."
	Lines int
	// Reason is a human-readable explanation, populated only on Block.
	Reason string
}

// allowedBy builds an Allow decision for the named carve-out. lines is 0
// for every carve-out except CarveOutUnderThreshold, where the caller
// passes the count it just computed.
func allowedBy(carveOut CarveOut, lines int) Decision {
	return Decision{Allow: true, CarveOut: carveOut, Lines: lines}
}

// Evaluate is the guard-commit decision engine. repoRoot is the working
// directory to evaluate from (the git repo, or a subdirectory of one).
// subject is the commit's (proposed or actual) subject line, used only to
// look for the "[skip-logmind]" marker. threshold is the substantive-line
// count at or above which a commit without a staged decision log is
// blocked. mode selects the diff semantics (see DiffMode).
//
// The checks run in order and the FIRST MATCH WINS:
//
//  1. Not a git repo at all               → Allow (nothing to enforce)
//  2. LOGMIND_ALLOW_GIT_COMMIT is truthy  → Allow (env carve-out)
//  3. subject contains "[skip-logmind]"   → Allow (skip-logmind carve-out)
//  4. git is mid-rebase/merge/cherry-pick/revert → Allow (the matching
//     in-progress carve-out) — these are git-internal commits, not
//     developer-authored ones.
//  5. the staged change RECORDED a decision → Allow (decision-recorded)
//  6. substantive lines < threshold       → Allow (under-threshold)
//  7. otherwise                           → Block, with a Reason
func Evaluate(repoRoot, subject string, threshold int, mode DiffMode) Decision {
	// 1. Outside a git repo there is nothing to enforce — never block a
	// `git commit` (or a command that merely mentions one) run somewhere
	// that isn't even under version control.
	if !gitcli.IsRepo(repoRoot) {
		return allowedBy(CarveOutNone, 0)
	}

	// 2. Explicit environment escape hatch. Checked before subject-parsing
	// so a caller who can't control the commit message (e.g. an
	// interactive `git commit` that opens $EDITOR) still has a way out.
	if envTruthy("LOGMIND_ALLOW_GIT_COMMIT") {
		return allowedBy(CarveOutEnvAllow, 0)
	}

	// 3. Explicit per-commit escape hatch via the subject line itself.
	if strings.Contains(subject, "[skip-logmind]") {
		return allowedBy(CarveOutSkipLogmind, 0)
	}

	// 4. git-internal operations in progress. These produce commits
	// (rebase replays, merge commits, cherry-pick/revert commits) that
	// are not "a developer made a substantive change out of nowhere" —
	// the ORIGINAL commit being replayed/merged/picked already went
	// through whatever enforcement applied when it was first authored.
	gitDir, err := gitcli.GitDir(repoRoot)
	if err != nil || gitDir == "" {
		// Best-effort fallback matching install_hook.go's pattern: if
		// git itself can't answer (shouldn't happen given IsRepo just
		// succeeded, but stay defensive), fall back to the naive join.
		// This only matters for the in-progress-state checks below —
		// wrong in a worktree, but "wrong" here means "might fail to
		// recognize an in-progress rebase," which degrades to falling
		// through to the normal threshold check, not an unsafe allow.
		gitDir = filepath.Join(repoRoot, ".git")
	}
	if exists(filepath.Join(gitDir, "rebase-apply")) || exists(filepath.Join(gitDir, "rebase-merge")) {
		return allowedBy(CarveOutRebaseInProgress, 0)
	}
	if exists(filepath.Join(gitDir, "MERGE_HEAD")) {
		return allowedBy(CarveOutMergeInProgress, 0)
	}
	if exists(filepath.Join(gitDir, "CHERRY_PICK_HEAD")) || exists(filepath.Join(gitDir, "REVERT_HEAD")) {
		return allowedBy(CarveOutCherryPickOrRevert, 0)
	}

	// 5. The commit itself documents the decision — RECORDED, not merely
	// staged. The index is the right scope in both modes: this asks what
	// the commit about to be made will carry, and a decision file sitting
	// unstaged carries nothing into it.
	evidence, _ := DecisionRecorded(gitcli.DiffCachedNames(repoRoot), func(path string) ([]string, error) {
		// DiffCachedAddedLines is best-effort by contract (nil on any git
		// failure), so there is no error to propagate here — a git that
		// cannot answer yields no added lines, which fails CLOSED into the
		// line count below rather than into an allow.
		return gitcli.DiffCachedAddedLines(repoRoot, path), nil
	})
	if evidence.Recorded {
		return allowedBy(CarveOutDecisionRecorded, 0)
	}

	// 6/7. Compute the substantive-line count per the requested diff mode
	// and compare against threshold.
	lines := SubstantiveLines(collectRows(repoRoot, mode))
	if lines < threshold {
		return allowedBy(CarveOutUnderThreshold, lines)
	}
	return Decision{
		Allow:  false,
		Lines:  lines,
		Reason: blockReason(lines, evidence.Touched),
	}
}

// blockEscapeHatches names SPEC §3.4's two per-commit escapes. One owner
// for the sentence: a block that names the remedy and forgets the escapes
// (or names them in one branch below and not the other) is a gate the
// author cannot get past without guessing.
const blockEscapeHatches = "record it with `logmind log` " +
	"(or add [skip-logmind] to the subject / set LOGMIND_ALLOW_GIT_COMMIT=1 to bypass)"

// blockReason renders Evaluate's Block explanation. `touched` names the
// decision files the change wrote to WITHOUT recording anything a §3.1
// reader can find.
//
// That second shape needs its own sentence. Reporting the bare "N lines
// changed without a decision log" over a diff that visibly stages
// docs/decisions.md reads as a bug in the gate — the author looks at the
// index, sees the decision file, and concludes the block is spurious. It
// is not: the file is there and the entry is not, and only the message can
// say which.
func blockReason(lines int, touched []string) string {
	if len(touched) == 0 {
		return fmt.Sprintf("%d lines changed without a decision log — %s", lines, blockEscapeHatches)
	}
	return fmt.Sprintf(
		"%d lines changed without a decision log — %s is staged but adds no entry a §3.1 reader "+
			"can find (a title, a timestamp, and non-empty reasoning), so it documents nothing; %s",
		lines, strings.Join(touched, ", "), blockEscapeHatches,
	)
}

// collectRows gathers the gitcli.NumstatLine rows relevant to mode.
//
//   - StagedOnly: just the index (`git diff --cached --numstat`) — correct
//     for the git commit-msg hook, where the index is already final.
//   - WorkingTreeUnion: the union of staged + unstaged-tracked +
//     untracked. This is the critical correctness point for the harness
//     layer: PreToolUse fires BEFORE a compound `git add -A && git commit`
//     stages anything, so `--cached` alone would undercount (and silently
//     bypass enforcement) whenever the change under review hasn't been
//     staged yet.
func collectRows(repoRoot string, mode DiffMode) []gitcli.NumstatLine {
	switch mode {
	case WorkingTreeUnion:
		var rows []gitcli.NumstatLine
		rows = append(rows, gitcli.DiffCachedNumstat(repoRoot)...)
		rows = append(rows, gitcli.DiffNumstat(repoRoot)...)
		rows = append(rows, gitcli.UntrackedNumstat(repoRoot)...)
		// Note: a file with BOTH staged and unstaged hunks appears in
		// both DiffCachedNumstat and DiffNumstat, so its changed lines are
		// counted twice here. That's intentional and left as-is — the
		// double-count only ever OVER-counts, biasing toward Block, which
		// is the safe direction for an enforcement gate (it can't cause a
		// silent bypass).
		return rows
	default: // StagedOnly
		return gitcli.DiffCachedNumstat(repoRoot)
	}
}

// AddedLinesFunc reports the lines a change ADDED to one repo-relative
// path, with git's leading "+" already stripped:
// gitcli.DiffCachedAddedLines for an index, gitcli.DiffRangeAddedLines for
// a base...head range.
//
// Taking the reader as a parameter is what lets the two local
// interception points and the CI gate share DecisionRecorded's judgement
// while each keeps its own diff scope. The alternative — passing a
// repoRoot and a mode — would put every scope this rule is ever judged
// over inside this package, which is how the scopes drift.
type AddedLinesFunc func(path string) ([]string, error)

// DecisionEvidence is DecisionRecorded's answer.
type DecisionEvidence struct {
	// Recorded is the gate's actual question: the change ADDED a
	// §3.1-well-formed entry to a decision file.
	Recorded bool
	// Touched names the decision files the change wrote to without
	// recording anything well-formed. Empty whenever Recorded is true —
	// it exists only so a BLOCK can name the file that was staged in vain.
	Touched []string
}

// DecisionRecorded answers "did this change record a decision?" — the ONE
// question every enforcement surface asks, and the one place it is
// answered. `logmind guard-commit` (Evaluate's carve-out 5) and the
// `check-decisions` gate (internal/cli/check_decisions.go) both route
// through it.
//
// Two halves, and BOTH are required:
//
//   - the path is a decision file (isDecisionFile), and
//   - the lines the change ADDED to it carry an entry that is well-formed
//     under §3.1 (WellFormedDecisionAdded).
//
// The path half alone is not an answer, and shipping it as one was a live
// gate hole: since SPEC §3.2, docs/decisions.md is an install sentinel
// that "is not written to, and holds no decisions of its own", so
// `git add docs/decisions.md` staged a file logmind itself had written and
// cleared the commit gate for any amount of code. Measured on the PR head:
// 302 lines of new Go, sentinel staged, `guard-commit --layer git-hook`
// exit 0 "allowed (decision-file-staged)". check-decisions asked the
// second half and guard-commit did not, which is the whole defect — two
// callers of one path predicate, two different answers to the question the
// SPEC actually poses. There is now no exported way to ask the path half
// on its own.
//
// SPEC §3.4: "A decision clears the gate by being written, not by
// existing. ... MUST NOT be satisfied by the decision file merely
// appearing in the diff."
//
// The error is addedLines' own and is returned unwrapped, so the gate's
// loud-on-failure contract (an unresolvable ref must not read as an empty
// diff) survives the trip through here.
func DecisionRecorded(names []string, addedLines AddedLinesFunc) (DecisionEvidence, error) {
	var ev DecisionEvidence
	for _, path := range names {
		if !isDecisionFile(path) {
			continue
		}
		added, err := addedLines(path)
		if err != nil {
			return DecisionEvidence{}, err
		}
		if WellFormedDecisionAdded(added) {
			return DecisionEvidence{Recorded: true}, nil
		}
		ev.Touched = append(ev.Touched, path)
	}
	return ev, nil
}

// isDecisionFile reports whether path is a decision-log file:
//
//   - exact path "docs/decisions.md"
//   - suffix "/decisions.md" (covers nested decisions.md)
//   - prefix "docs/decisions-branches/" (per-branch decision files)
//
// UNEXPORTED on purpose. It answers "is this the kind of file a decision
// lives in", which is only ever half of "did this change record a
// decision" — and the half that a content-free file passes. It was
// exported once, one caller asked it alone, and that caller was the
// commit gate. DecisionRecorded is the exported question; this is an
// implementation detail of it.
//
// docs/decisions.md stays on the list even though nothing writes it since
// §3.2: a repository that predates the collapse carries a real decision
// log at that path, and a change that appends an entry there has recorded
// a decision by any reading of §3.4.
func isDecisionFile(path string) bool {
	if path == "docs/decisions.md" {
		return true
	}
	if strings.HasSuffix(path, "/decisions.md") {
		return true
	}
	if strings.HasPrefix(path, "docs/decisions-branches/") {
		return true
	}
	return false
}

// reasoningMarker opens SPEC §3.1's reasoning section. §3.1 lets a
// producer omit an empty section's header entirely, so a marker present
// with nothing under it is malformed, not merely sparse.
const reasoningMarker = "**Reasoning:**"

// WellFormedDecisionAdded reports whether the lines a diff ADDED to a
// decision file carry an entry well-formed enough to clear the
// `check-decisions` gate.
//
// SPEC §3.4: "A decision clears the gate by being written, not by
// existing. The gate MUST check that the added entry is well-formed under
// §3.1 — a title, a timestamp, and a reasoning section that is not empty
// — and MUST NOT be satisfied by the decision file merely appearing in
// the diff. A test that asks only whether the file was touched is passed
// by a single meaningless line."
//
// added is the diff's added lines with git's leading "+" already
// stripped, in file order. Title and timestamp come free from
// decisions.SplitRawBytes, which only opens an entry on a line matching
// `## YYYY-MM-DD HH:MM - <title>` whose date/time actually parses — the
// same boundary rule every other reader in this codebase uses. This
// function adds §3.4's one extra requirement on top: non-empty reasoning.
//
// Note this is shape, not quality — §3.4 is explicit that "a determined
// author can still write three plausible sentences that explain nothing,
// and no gate can catch that." What it removes is the version that costs
// nothing.
func WellFormedDecisionAdded(added []string) bool {
	_, entries := decisions.SplitRawBytes(strings.Join(added, "\n"))
	for _, e := range entries {
		if hasReasoning(e.Raw) {
			return true
		}
	}
	return false
}

// hasReasoning reports whether raw carries a reasoningMarker section with
// content under it. Content may sit on the marker's own line (what
// `logmind log` writes) or anywhere below it until the section ends. A
// marker with nothing at all under it is an empty section and does not
// count.
//
// A SECTION ENDS AT THE NEXT SECTION HEADER OR AT THE ENTRY TERMINATOR —
// NOT AT A BLANK LINE. §3.1's own template separates sections with a
// blank line, but a blank line inside a section is a paragraph break, and
// treating it as the end reads an ordinary hand-written entry
//
//	**Reasoning:**
//
//	Three sentences of prose that say exactly why.
//
//	---
//
// as an EMPTY reasoning section and blocks the commit. Measured across
// four identically-built repos before this was fixed: reasoning inline on
// the marker line → exit 0; wrapped onto the next line with no blank →
// exit 0; a blank line after the marker → exit 65; a bullet list after a
// blank → exit 65. The last two carry real reasoning. §3.1 also forbids
// the shape this was implicitly assuming: a consumer "MUST NOT require a
// section order beyond the title coming first and `---` terminating the
// entry", and "the body starts on the marker's line or the one after it"
// is exactly such a requirement.
//
// The next-section-header stop is what keeps the loosening honest. §3.1
// says a producer MUST omit an empty section's header, so
//
//	**Reasoning:**
//	**Alternatives considered:** none
//
// is an empty reasoning section, and without the header stop the
// following header's own text would be swallowed as this section's body
// — the "single meaningless line" §3.4 rejects. Blank-separated or not
// makes no difference to that, which is the point: the header ends the
// section either way.
//
// Scope note: raw is one entry, cut by decisions.SplitRawBytes at its own
// title and at the next one, so this scan cannot run into a neighbouring
// entry's prose.
func hasReasoning(raw string) bool {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), reasoningMarker) {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), reasoningMarker))
		for _, next := range lines[i+1:] {
			trimmed := strings.TrimSpace(next)
			if isSectionHeader(trimmed) || trimmed == entryTerminator {
				break
			}
			body += trimmed
		}
		if body != "" {
			return true
		}
	}
	return false
}

// entryTerminator is the `---` that ends an entry per SPEC §3.1 ("`---`
// terminating the entry"). Reasoning cannot run past it.
const entryTerminator = "---"

// isSectionHeader reports whether a trimmed line opens a new §3.1 section
// — a bolded label, e.g. `**Alternatives considered:**`. Deliberately
// shape-based rather than a fixed list of the known section names: §3.1
// says a consumer "MUST NOT require a section order", and a hand-written
// entry may carry a section this build has never heard of. Anything that
// opens a bold run and carries a colon terminates the previous section.
func isSectionHeader(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "**") {
		return false
	}
	rest := trimmed[2:]
	end := strings.Index(rest, "**")
	if end < 0 {
		return false
	}
	return strings.Contains(rest[:end], ":")
}

// docsPrefix and configPrefix are the two directory exclusions of SPEC
// §3.4's table: the decision record plus the documents derived from it,
// and the toolchain's own configuration.
const (
	docsPrefix   = "docs/"
	configPrefix = ".logmind/"
)

// instructionFile is SPEC §1.1's single instruction file, named in its
// own right by §3.4's table alongside "the per-tool files of §1.2". The
// §1.2 set is NOT listed here — it comes from agents.FilePatterns(), the
// registry that writes those files (see excludedFiles).
const instructionFile = "AGENTS.md"

// excludedFiles is the exact-match half of §3.4's exclusion table:
// AGENTS.md plus every per-tool file of §1.2, derived from the agents
// registry so the two can't disagree. Codex's registry entry is also
// "AGENTS.md" (§1.2 lists it as reading the instruction file directly),
// so the two sources overlap by one — harmless for a set.
var excludedFiles = func() map[string]bool {
	set := map[string]bool{instructionFile: true}
	for _, p := range agents.FilePatterns() {
		set[p] = true
	}
	return set
}()

// IsExcludedPath reports whether a repo-root-relative path is excluded
// from the substantive-line count by SPEC §3.4's table. Three of the four
// exclusions are path rules:
//
//   - everything under "docs/" — the decision record and the documents
//     derived from it
//   - AGENTS.md and the per-tool files of §1.2 — "a refresh rewrites
//     these, carrying no decision of its own"
//   - ".logmind/", the toolchain's own configuration — same reason
//
// The fourth ("any file the forge reports as binary") is not a path rule
// at all: it's git's numstat "-" marker, which SubstantiveLines skips
// separately.
//
// §3.4 says "four things are excluded from the count, and NOTHING ELSE
// IS." In particular markdown is NOT excluded merely for being markdown:
// "A skill file counts. So does an agent definition. ... Excluding
// markdown wholesale switches the rule off in the repositories where
// writing *is* the work." Do not add a "*.md" arm here.
func IsExcludedPath(path string) bool {
	// A path carrying git's rename rendering is never excluded.
	//
	// gitcli.numstatFlags passes --no-renames, so in practice git does not
	// hand us this shape at all. This is the second line of defence, and it
	// exists because the first one was removed once already: when the flag
	// went missing, `docs/notes.md => src/payload.go` prefix-matched
	// `docs/` and the gate counted 550 lines of new Go as zero.
	//
	// Deliberately a refusal rather than a parser. Git has at least two
	// renderings — `old => new` and the compact `{docs => src}/sub/file` —
	// so parsing owes both plus whatever git adds later, and a parser that
	// falls behind fails OPEN. Refusing to exclude fails CLOSED: the worst
	// case is counting a rename that a correct parser would have excluded,
	// which asks an author for a decision they did not owe. That is a far
	// cheaper error than waving through the change the gate exists to stop.
	if strings.Contains(path, " => ") {
		return false
	}
	if strings.HasPrefix(path, docsPrefix) || strings.HasPrefix(path, configPrefix) {
		return true
	}
	return excludedFiles[path]
}

// SubstantiveLines sums added+removed lines across rows, applying SPEC
// §3.4's exclusion table:
//
//   - a row whose Path is excluded by IsExcludedPath is skipped
//   - a row with Added == "-" is a binary file (git's numstat marker) and
//     is skipped — §3.4's fourth exclusion, "a line count means nothing
//     there"
//   - a row that fails to parse as integers is silently swallowed
//     (inherited from the Python original; not a SPEC rule)
//
// Taking a flat []gitcli.NumstatLine (rather than a repoRoot + diff mode)
// is deliberate: it lets WorkingTreeUnion feed rows from three different
// git commands (staged, unstaged-tracked, untracked) through the exact
// same skip logic as StagedOnly's single source, with no risk of the two
// modes' counting rules drifting apart. The `check-decisions` gate
// (internal/cli/check_decisions.go) feeds it a base...head range through
// the same function for the same reason — §3.4: "driven by one shared
// evaluation so they cannot disagree."
func SubstantiveLines(rows []gitcli.NumstatLine) int {
	total := 0
	for _, row := range rows {
		if IsExcludedPath(row.Path) {
			continue
		}
		if row.Added == "-" {
			continue
		}
		added, errA := strconv.Atoi(row.Added)
		removed, errR := strconv.Atoi(row.Removed)
		if errA != nil || errR != nil {
			continue
		}
		total += added + removed
	}
	return total
}

// envTruthy reports whether the named environment variable is set to a
// truthy value: "1", "true", or "yes" (case-insensitive, surrounding
// whitespace trimmed). Anything else — including unset — is falsey.
func envTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// exists reports whether path exists (file OR directory — rebase-apply
// and rebase-merge are directories; MERGE_HEAD/CHERRY_PICK_HEAD/
// REVERT_HEAD are files). Any stat error (including "not found") reports
// false — best-effort, matching the rest of this package's git wrappers.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
