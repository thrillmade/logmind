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
// (Hidden, manually-invokable-only) CLI surface this feeds; the hooks that
// call it automatically are a follow-up PR.
package guardcommit

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	// CarveOutDecisionFileStaged: a decision-log file (docs/decisions.md,
	// docs/decisions-branches/*.md, or any path ending in "/decisions.md")
	// is already staged — the commit IS the documentation.
	CarveOutDecisionFileStaged CarveOut = "decision-file-staged"
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
//  5. a decision-log file is already staged → Allow (decision-file-staged)
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

	// 5. The commit itself documents the decision.
	for _, f := range gitcli.DiffCachedNames(repoRoot) {
		if IsDecisionFile(f) {
			return allowedBy(CarveOutDecisionFileStaged, 0)
		}
	}

	// 6/7. Compute the substantive-line count per the requested diff mode
	// and compare against threshold.
	lines := SubstantiveLines(collectRows(repoRoot, mode))
	if lines < threshold {
		return allowedBy(CarveOutUnderThreshold, lines)
	}
	return Decision{
		Allow: false,
		Lines: lines,
		Reason: fmt.Sprintf(
			"%d lines changed without a decision log — record it with `logmind log` "+
				"(or add [skip-logmind] to the subject / set LOGMIND_ALLOW_GIT_COMMIT=1 to bypass)",
			lines,
		),
	}
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

// IsDecisionFile reports whether path is a decision-log file. Moved
// verbatim from internal/cli/check_decisions.go's former isDecisionFile so
// both check-decisions and guard-commit share one predicate:
//
//   - exact path "docs/decisions.md"
//   - suffix "/decisions.md" (covers nested decisions.md)
//   - prefix "docs/decisions-branches/" (per-branch decision files)
func IsDecisionFile(path string) bool {
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

// SubstantiveLines sums added+removed lines across rows, applying the same
// skip rules check-decisions has always used (moved verbatim from
// internal/cli/check_decisions.go's runCheckDecisions loop):
//
//   - a row whose Path starts with "docs/" is skipped (documentation
//     changes aren't the kind of decision this gate is watching for)
//   - a row with Added == "-" is a binary file (git's numstat marker) and
//     is skipped
//   - a row that fails to parse as integers is silently swallowed
//
// Taking a flat []gitcli.NumstatLine (rather than a repoRoot + diff mode)
// is deliberate: it lets WorkingTreeUnion feed rows from three different
// git commands (staged, unstaged-tracked, untracked) through the exact
// same skip logic as StagedOnly's single source, with no risk of the two
// modes' counting rules drifting apart.
func SubstantiveLines(rows []gitcli.NumstatLine) int {
	total := 0
	for _, row := range rows {
		if strings.HasPrefix(row.Path, "docs/") {
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
