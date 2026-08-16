// pulse.go — the v2.0.0 "pulse" feature: `logmind log` surfaces repo-health
// advisories on STDERR after everything else has printed.
//
// Three independent, best-effort advisories, printed in this order (drift,
// then spec, then main-decisions) when applicable:
//
//   - Drift pulse: any FILE-READ-ONLY doctor probe classified STALE (the
//     drift class that flips doctor's Overall — stale hooks / stale
//     AGENTS.md block / stale Claude PreToolUse guard; NOT missing, NOT
//     markerless — see doctor.StaleCountFast):
//     `logmind: <n> component(s) stale — run 'logmind doctor --fix'`
//
//   - Spec pulse: context.spec_file is configured, resolves
//     (config.ResolveSpecFile), the resolved file is git-TRACKED and has no
//     uncommitted modifications, and at least specPulseThreshold decision
//     entries postdate the spec's last git commit:
//     `logmind: <spec-path> unchanged for <n> decisions — still accurate?`
//
//   - Main-decisions pulse (v2.0.0 derived-docs-on-main freshness layer): on a
//     NON-default branch, the last-fetched origin/<default> remote-tracking
//     ref carries one or more decision-touching commits (docs/decisions.md,
//     docs/decisions-branches/) the branch does
//     not yet have — see mainDecisionsPulseLine:
//     `logmind: main has <n> new decision commit(s) — run 'logmind warp' to catch up`
//
// STDERR ONLY, unconditionally. This is deliberate and load-bearing:
//
//   - The §3.1 stdout contract (see log.go) is byte-exact for non-TTY
//     invocations — three lines, no more, no less. Pulse output must never
//     touch stdout or that contract breaks.
//   - Under --quiet / LOGMIND_QUIET, stdout carries exactly one `ok ...`
//     receipt line. Pulse output must never touch stdout there either —
//     but stderr is explicitly OUTSIDE both contracts, and an agent running
//     in quiet mode is the primary audience for a low-noise health signal,
//     so the pulse still fires under --quiet (just on stderr, same as
//     always).
//
// Emitted in every mode — TTY, non-TTY, --quiet — with no gating on any of
// them: only the STDOUT destination is contract-sensitive, and this package
// never writes there.
//
// HOT-PATH BUDGET: emitPulse runs on EVERY `logmind log`, so every probe it
// calls transitively must stay fast and NETWORK-FREE. The drift and spec
// pulses are subprocess-free (file reads / stats only). The drift pulse uses
// doctor.StaleCountFast specifically because doctor's full probe set
// (doctor.StaleCount, used by `logmind doctor` itself) includes a PATH
// lookup + a live `<binary> --version` subprocess — a hung or daemonizing
// PATH binary can stall or hang that call outright. See
// doctor.StaleCountFast's doc comment for the exact excluded-probe list. The
// main-decisions pulse is the one exception that shells out (`git
// rev-list`), but ONLY against the local, already-fetched origin/<default>
// ref — no `git fetch`, no `gh`, no HTTP anywhere in this file. Network
// access is reserved for the explicit `logmind warp` command (warp.go).
package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/doctor"
	"github.com/thrillmade/logmind/internal/gitcli"
)

// specPulseThreshold is the number of decision entries that must postdate
// context.spec_file's last git commit before the spec pulse fires.
// Implementation-defined (no SPEC-mandated value, hence no config key): 20 is
// roughly "a couple of weeks of decisions have landed since anyone touched the
// spec", a reasonable staleness bar for a hand-authored, forward-looking doc
// that (unlike the derived docs) is never auto-regenerated. It is deliberately
// unrelated to §3.3's 50-entry timeline bound: that one governs how much
// history a reader is shown, this one how long a spec may sit unexamined.
const specPulseThreshold = 20

// emitPulse prints ZERO, ONE, or TWO advisory lines to stderr — drift
// pulse first, then spec pulse — and is the LAST thing `logmind log`
// writes to stderr. Called unconditionally at the very end of runLog,
// after the §3.1 stdout contract lines, the branch-summary nudge, the
// commit/push lines, and the --quiet `ok` receipt.
//
// Failure-safety is the entire point of this function's shape: pulse
// computation must NEVER fail or slow down `logmind log`. Every probe
// underneath (git subprocess calls, file reads, decision-file parsing) is
// already best-effort and returns a zero value / false on any error rather
// than an error type — there is deliberately nothing here to check or
// propagate. The recover() is a defensive backstop for the unexpected
// case (a nil-map access, a slice-index bug introduced later), not an
// expected path: even a genuine bug in pulse computation must degrade to
// "no pulse this run," never to a failed or slowed `logmind log`. No
// probe here makes a network call — every read is local (git log/ls-files
// against the local repo, local file reads, local config/decisions
// parsing).
func emitPulse(cwd string, stderr io.Writer) {
	defer func() {
		_ = recover()
	}()
	if line, ok := driftPulseLine(cwd); ok {
		fmt.Fprintln(stderr, line)
	}
	if line, ok := specPulseLine(cwd); ok {
		fmt.Fprintln(stderr, line)
	}
	if line, ok := mainDecisionsPulseLine(cwd); ok {
		fmt.Fprintln(stderr, line)
	}
}

// driftPulseLine reports the doctor-drift advisory. Reuses
// doctor.StaleCountFast — the subprocess-free subset of doctor's probes —
// NOT doctor.StaleCount. `logmind log` runs on every commit; doctor's full
// probe set includes a PATH lookup + a live `<binary> --version`
// subprocess (probePathResolution) and a `git config` shell-out
// (probeMergeDriverConfig), either of which can add real latency or, in
// the pathological case of a hung/daemonizing PATH binary, hang the log
// outright. StaleCountFast excludes exactly those two probes; everything
// else is a plain file read, so the count here can differ from what
// `logmind doctor` itself reports (PATH / merge-driver-config drift is
// invisible to the pulse) — that gap is intentional, see StaleCountFast's
// doc comment. Run `logmind doctor` for the complete picture.
func driftPulseLine(cwd string) (string, bool) {
	n := doctor.StaleCountFast(cwd)
	if n <= 0 {
		return "", false
	}
	plural := "s"
	if n == 1 {
		plural = ""
	}
	return fmt.Sprintf("logmind: %d component%s stale — run 'logmind doctor --fix'", n, plural), true
}

// specPulseLine reports the spec-staleness advisory. Fires only when ALL
// of the following hold:
//
//   - context.spec_file is configured AND resolves per config.ResolveSpecFile
//     (unset, absolute, or path-escaping values are already treated as
//     UNSET there — same rule the real `logmind context` fold-in uses, so
//     this advisory and that behavior can never disagree).
//   - the resolved file is git-TRACKED. An untracked spec file has no
//     deterministic last-touched date across clones (mtime isn't preserved
//     by git, isn't preserved across clones/CI checkouts), so git history
//     is the only source of truth here and an untracked file silently
//     skips the pulse rather than guessing from mtime.
//   - the tracked file has NO uncommitted modifications (working-tree or
//     staged) right now. This log call may be the one editing the spec —
//     `--stage scoped` or `--no-commit` can leave that edit uncommitted at
//     the point emitPulse runs — and the very commit that updates the spec
//     shouldn't turn around and ask "is this still accurate?" about the
//     file it's mid-edit on. See gitcli.StatusPorcelain.
//   - at least specPulseThreshold decision entries — collected from
//     docs/decisions.md and
//     docs/decisions-branches/*.md via decisions.Collect, the same
//     aggregator `logmind timeline` uses — carry a `## YYYY-MM-DD HH:MM`
//     header timestamp strictly AFTER the spec file's last commit
//     (committer date, git log -1 --format=%cI).
func specPulseLine(cwd string) (string, bool) {
	cfg, err := config.Load(cwd)
	if err != nil {
		return "", false
	}
	if _, ok := config.ResolveSpecFile(cwd, cfg); !ok {
		return "", false
	}
	relSpec := cfg.Context.SpecFile

	if !gitcli.IsTrackedFile(cwd, relSpec) {
		return "", false
	}
	if gitcli.StatusPorcelain(cwd, relSpec) != "" {
		// Uncommitted changes (staged or working-tree) — the spec is
		// mid-edit right now; skip rather than nag about the file this
		// very log might be updating.
		return "", false
	}
	specTime, ok := gitcli.LastCommitTime(cwd, relSpec)
	if !ok {
		return "", false
	}

	entries, err := decisions.Collect(filepath.Join(cwd, "docs"), io.Discard)
	if err != nil {
		return "", false
	}
	// Count decisions logged strictly AFTER the spec's last commit, compared
	// at CALENDAR-DAY granularity (issue #222, residual of #211). Why day
	// granularity and not the instant:
	//
	//   - A decision header `## YYYY-MM-DD HH:MM` is a ZONELESS local wall
	//     clock: decisions.Iter labels it UTC, but buildDecisionEntry (log.go)
	//     wrote it from time.Now().Format(...) on whatever machine logged it.
	//     There is NO recoverable instant for a header written on a different
	//     machine in a different zone.
	//   - specTime IS a true instant (gitcli.LastCommitTime parses git's %cI,
	//     with an explicit offset).
	//
	// An instant-vs-instant compare therefore flips by the running machine's
	// UTC offset — the exact bug #211's localize attempt could not fix for the
	// cross-machine case (TZ=Etc/GMT-12 vs TZ=UTC gave opposite verdicts on
	// identical repo state). Reducing both sides to their Y/M/D at UTC midnight
	// and requiring the decision's DAY to be strictly after the spec commit's
	// DAY bounds cross-machine skew to ≤1 day instead of a full offset, and a
	// same-day tie counts as NOT-after (no false "still accurate?" nudge). This
	// stays local to the pulse: decisions.Iter keeps its UTC label, so the
	// timeline dedup/sort path (internal/timeline/canonical.go) is untouched.
	specDay := dateOnlyUTC(specTime)
	count := 0
	for _, e := range entries {
		if dateOnlyUTC(e.Date).After(specDay) {
			count++
		}
	}
	if count < specPulseThreshold {
		return "", false
	}
	return fmt.Sprintf("logmind: %s unchanged for %d decisions — still accurate?", relSpec, count), true
}

// mainDecisionsPulseLine reports the "main has advanced" freshness advisory:
// on a NON-default branch, when the last-fetched origin/<default> carries
// decision-touching commits the branch does not, nudge a catch-up. NETWORK-FREE:
// reads the existing remote-tracking ref (no fetch), so the count is as of the
// last warp/fetch. Best-effort: ("", false) on any error or missing origin ref —
// this runs on every `logmind log`, so it must never fail or slow the hot path.
func mainDecisionsPulseLine(cwd string) (string, bool) {
	if !onNonDefaultBranch(cwd) {
		return "", false
	}
	def := gitcli.DefaultBranch(cwd)
	if def == "" {
		return "", false
	}
	out, _, err := gitcli.RunCaptured(cwd, "rev-list", "--count", "HEAD..origin/"+def, "--",
		"docs/decisions.md", "docs/decisions-branches")
	if err != nil {
		return "", false
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || n <= 0 {
		return "", false
	}
	plural := "s"
	if n == 1 {
		plural = ""
	}
	return fmt.Sprintf("logmind: main has %d new decision commit%s — run 'logmind warp' to catch up", n, plural), true
}

// dateOnlyUTC reduces t to its calendar day (year/month/day) anchored at UTC
// midnight, discarding the time-of-day and zone. specPulseLine uses it to
// compare a zoneless decision-header wall clock against the spec's last-commit
// instant at day granularity, which makes the staleness verdict independent of
// the running machine's timezone and bounds cross-machine skew to ≤1 day
// (issue #222). Note t.Year()/Month()/Day() read the components in t's OWN
// location, so each side contributes the calendar day its author actually saw.
func dateOnlyUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
