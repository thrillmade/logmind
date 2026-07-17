// pulse.go — the v2.0.0 "pulse" feature: `logmind log` surfaces repo-health
// advisories on STDERR after everything else has printed.
//
// Two independent, best-effort advisories, printed in this order (drift
// before spec) when applicable:
//
//   - Drift pulse: any doctor probe classified STALE (the drift class that
//     flips doctor's Overall — stale hooks / stale AGENTS.md block / stale
//     Claude PreToolUse guard; NOT missing, NOT markerless — see
//     doctor.StaleCount):
//     `logmind: <n> component(s) stale — run 'logmind doctor --fix'`
//
//   - Spec pulse: context.spec_file is configured, resolves
//     (config.ResolveSpecFile), the resolved file is git-TRACKED, and at
//     least specPulseThreshold decision entries postdate the spec's last
//     git commit:
//     `logmind: <spec-path> unchanged for <n> decisions — still accurate?`
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
package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/doctor"
	"github.com/thrillmade/logmind/internal/gitcli"
)

// specPulseThreshold is the number of decision entries that must postdate
// context.spec_file's last git commit before the spec pulse fires.
// Implementation-defined (no SPEC-mandated value, hence no config key): 20
// mirrors decisions.max_recent's default archive-rotation size — roughly
// "an archive-rotation's worth of decisions have landed since anyone
// touched the spec" is a reasonable staleness bar for a hand-authored,
// forward-looking doc that (unlike timeline.md / file-structure.md) is
// never auto-regenerated.
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
}

// driftPulseLine reports the doctor-drift advisory. Reuses
// doctor.StaleCount, which runs the identical probe set
// collectLogmindStatus uses internally, so the count here is EXACTLY the
// number of components that would flip a `logmind doctor` run on this repo
// to DRIFT — no re-derivation of doctor's classification rules.
func driftPulseLine(cwd string) (string, bool) {
	n := doctor.StaleCount(cwd)
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
//   - the resolved file is git-TRACKED. An untracked or uncommitted spec
//     file has no deterministic last-touched date across clones (mtime
//     isn't preserved by git, isn't preserved across clones/CI checkouts),
//     so git history is the only source of truth here and an untracked
//     file silently skips the pulse rather than guessing from mtime.
//   - at least specPulseThreshold decision entries — collected from
//     docs/decisions.md, docs/decisions-archive.md, and
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
	specTime, ok := gitcli.LastCommitTime(cwd, relSpec)
	if !ok {
		return "", false
	}

	entries, err := decisions.Collect(filepath.Join(cwd, "docs"), io.Discard)
	if err != nil {
		return "", false
	}
	count := 0
	for _, e := range entries {
		if e.Date.After(specTime) {
			count++
		}
	}
	if count < specPulseThreshold {
		return "", false
	}
	return fmt.Sprintf("logmind: %s unchanged for %d decisions — still accurate?", relSpec, count), true
}
