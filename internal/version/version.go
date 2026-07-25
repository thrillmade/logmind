// Package version holds the build-time version constants for logmind.
//
// v1.0.0 shipped 2026-06-03 via the v1-go-rewrite → main cutover (#132).
// Local `go build` and `make build` produce a binary reporting the
// "2.0.0-dev" default; tagged release builds running through
// `.goreleaser.yaml` override via `-ldflags
// "-X 'github.com/thrillmade/logmind/internal/version.Version=v1.2.3'"`
// to inject the actual tag.
//
// SpecVersion tracks the thrillmade/protocol SPEC.md document. It was
// bumped to "1.0.0" 2026-07-04 (the coordinated SPEC 1.0.0 that REMOVES
// the branch-divergent timeline model: §1.6.4 main-canonical union
// assembly is now the sole, unconditional timeline. See v2.0.0 below),
// then advanced to the current "1.5.0" ahead of the v2.0.0 tag (§15
// decision-logging enforcement, §16 the canonical spec-file contract,
// §3.1.1 the pulse — see docs/spec.md).
package version

import (
	"strconv"
	"strings"
)

// Version is the logmind binary's semantic version. Bumped at release.
// Overridable via -ldflags at build time; see package docstring.
//
// v2.0.0 (2026-07-04) — BREAKING: the branch-divergent timeline model
// (the Python-parity full-regen brief/full renderer) is removed
// entirely. main-canonical (§1.6.4 deterministic source-derived union)
// is now the SOLE, unconditional timeline assembly model. The
// `timeline.canonical` config key is gone: a repo whose config still
// carries it is unaffected (the now-unknown key is ignored). The
// agent-authored branch headline, the per-log timeline marker, and the
// `doctor --fix` marker backfill are always active — no opt-in. The
// `logmind timeline --full` flag is accepted but ignored (the timeline
// is single-format). SpecVersion advances to 1.0.0 to match.
//
// v1.2.0 (2026-06-07) — Go port of `logmind log` (Phase B3 catch-up)
// + 3-layer markdown self-healing per plan §8.7. The shim that
// previously lived in the Python v0.6.16 distribution is now native:
// the Go binary writes the decision file, commits it, and runs
// `linkcheck.Check()` as a self-heal gate. When the gate finds
// issues, an interactive retry loop (TTY-gated) gives the user up to
// three attempts to fix and re-check before the command exits. CI /
// scripted invocations bypass the prompt via `--no-interactive`
// or non-TTY auto-detection. The companion check-doc-links workflow
// template bumps to v5 with dual-mode self-heal — full Anthropic
// auto-fix when `ANTHROPIC_API_KEY` is set, a deterministic PR
// comment otherwise. linkcheck.CheckReport() surfaces fix-suggestion
// strings keyed to each finding so both interactive and CI surfaces
// emit actionable guidance instead of red-only diagnostics.
//
// v1.1.0 (2026-06-05) — install.sh fetch-latest mode + setup-logmind
// scaffold pattern. Per the 2026-06-05 distribution lock, consumer
// repos no longer ship `pip install logmind==X.Y.Z` in workflow
// templates: they use `uses: thrillmade/setup-logmind@v1.0.0` and let
// Dependabot bump the action pin. install.sh defaults to the latest
// release (no hardcoded sweeps) and detects GITHUB_ACTIONS=true to
// nudge CI users at setup-logmind.
var Version = "2.0.0-dev"

// SpecVersion is the version of the thrillmade/protocol logmind contract
// this binary implements. Reported via `logmind --version` so downstream
// tools can detect protocol skew without parsing the binary version.
// Overridable via -ldflags at build time; see package docstring.
var SpecVersion = "1.5.0"

// SatisfiesMin reports whether v is at least min, using a simple
// major.minor.patch integer compare — NOT full semver precedence. v2.0.0 B6
// (the derived-docs adoption gate's version floor, `derived_docs.min_binary`)
// is the only caller today: `logmind doctor` compares the running Version
// against a repo-declared floor and warns (never errors) when it's older.
//
// Deliberately no semver dependency (per the B6 build ruling: "do NOT add a
// dependency") — go.mod carries none today, and the compare this needs is
// small enough not to justify one.
//
// Both v and min tolerate a leading "v" and a trailing prerelease/build
// suffix (anything from the first '-' or '+' onward) — the suffix is
// stripped BEFORE comparing, a deliberate departure from strict semver
// precedence (where "2.0.0-dev" < "2.0.0"): this repo's own dogfood binary
// reports "2.0.0-dev" (see Version above), and it must satisfy its own
// repo's `min_binary: "2.0.0"` floor rather than perpetually warning about
// itself. A caller that needs strict prerelease ordering isn't served by
// this helper — there is no such caller today.
//
// A version string that isn't parseable as three dot-separated integers
// (missing a component, non-numeric, empty) makes SatisfiesMin return true
// for BOTH v and min — fail open. A floor check is an advisory-only nudge
// (see internal/doctor), never a gate; reporting a false warning off a
// version string this helper can't even parse would be worse than staying
// silent.
func SatisfiesMin(v, min string) bool {
	vp, ok := parseVersionCore(v)
	if !ok {
		return true
	}
	mp, ok := parseVersionCore(min)
	if !ok {
		return true
	}
	for i := 0; i < 3; i++ {
		if vp[i] != mp[i] {
			return vp[i] > mp[i]
		}
	}
	return true // equal
}

// parseVersionCore extracts the [major, minor, patch] integer triple from a
// version string, stripping a leading "v" and any trailing
// prerelease/build suffix (from the first '-' or '+' onward) first. Returns
// ok=false when the (suffix-stripped) remainder isn't exactly three
// dot-separated non-negative integers.
func parseVersionCore(s string) (core [3]int, ok bool) {
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return core, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return core, false
		}
		core[i] = n
	}
	return core, true
}
