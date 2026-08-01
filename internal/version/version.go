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
// then advanced to "1.5.0" ahead of the v2.0.0 tag (for what were THEN
// numbered §15 decision-logging enforcement, §16 the canonical spec-file
// contract, and §3.1.1 the pulse — those three numbers are from the
// archived predecessor document and address nothing in the live SPEC;
// they are recorded here as history, not as citations to follow), then
// to the current "2.0.0" for logmind#264: the
// document was rewritten and renumbered as SPEC-2, restarting its own
// version at 2.0.0 (Status: Draft) — the old "1.5.0" no longer
// corresponds to anything in the live section numbering. See SpecVersion's
// own doc comment below for the full evidence trail.
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
//
// Bumped to "2.0.0" for logmind#264 (SPEC §7.3 — "What a tool declares").
// Fetched from thrillmade/protocol SPEC.md@main: the document header reads
// `<!-- spec-version: 2.0.0 -->` / `**Version:** 2.0.0` / `**Status:**
// Draft`. §7.2 — "`Status: Draft` names the version being drafted, not the
// last one released, and requires no tag" — so the absence of a
// `spec-v2.0.0` tag (`gh api repos/thrillmade/protocol/tags` tops out at
// `spec-v0.6.3`, then jumps straight to `spec-pre-rewrite-2026-07-31`: the
// 0.x/1.x line was retired and renumbered as this SPEC-2 document starting
// at 2.0.0) does not block declaring it. §7.4 confirms tools are meant to
// declare support for a major version BEFORE it is cut final — "A major
// version MUST NOT be declared final until every first-party tool has
// released a version implementing it" — so declaring 2.0.0 now, while
// Draft, is the expected order of operations, not a jump of the gun. The
// prior "1.5.0" was measured against the archived predecessor document's
// numbering (thrillmade/protocol:docs/SPEC-0.7.2-archive.md — it lives in
// the protocol repository, NOT this one) and does not correspond to
// anything in the live SPEC-2 text.
var SpecVersion = "2.0.0"

// Areas is the comma-and-space-joined SPEC §7.3 area declaration this
// binary claims, in the vocabulary's fixed order (orient, work, record,
// review, propagate, gates, versioning — only the ones actually
// implemented are listed). Printed as the second line of `--version`
// output: `areas: <Areas>`.
//
// Coarse by design (§7.3: "Claiming an area is deliberately coarse... A
// tool claims an area when it implements any part of it"), and each word
// below is backed by shipped code, not aspiration (§0.4: a tool "does not
// claim the ones it does not [implement]"):
//
//   - orient  — §1: the AGENTS.md tool-owned block (internal/templates),
//     `.logmind/config.yml` (internal/config, §1.6), and `logmind context`
//     assembling the cold-start payload verbatim to §1.5's envelope order
//     (internal/cli/context.go).
//   - work    — §2.1/§2.2: skill-file authoring, frontmatter validation
//     and `kind` routing (internal/skill/{scaffold,validate}.go); §2.7/2.8:
//     the LOGMIND_QUIET / `ok <k=v>` discipline (internal/cli/quiet.go)
//     and the `ceil(bytes/4)` token estimate plus "(N omitted)" truncation
//     markers used throughout internal/tree, internal/repomap,
//     internal/cli/context.go and internal/cli/file_structure.go.
//   - record  — §3 in full: `logmind log` writes
//     `docs/decisions-branches/<branch>.md` entries in the §3.1 format,
//     branch-sanitizes per §3.2, and regenerates the derived
//     `docs/timeline.md` / `docs/file-structure.md` per §3.3
//     (internal/decisions, internal/inserter, internal/timeline,
//     internal/repomap).
//   - propagate — §5.2's upward "nomination" path specifically (a pull
//     request against the catalog, opened by the repository that homes
//     the item, refusing to target a catalog the repo has not named):
//     `logmind skill push` (internal/skill/push.go,
//     internal/cli/skill_push.go). Not the harness's downward
//     distribution/seeding/skills-lock machinery of §5.1/§5.2 — that is
//     skdd's job, not logmind's, so only the part actually shipped is
//     claimed.
//   - gates   — §6.2's canonical checks: `check-decisions`,
//     `check-derived-docs` and `check-links` are all installed and
//     produced by logmind's own templates and CLI verbs
//     (internal/cli/check_decisions.go, internal/cli/check_links.go,
//     internal/timeline/canonical.go, internal/templates/github/*.yml.template).
//
// Deliberately NOT claimed:
//
//   - review     — clud-bug's job (§0.1); logmind never examines a change
//     or emits a finding. `logmind sync` only consumes clud-bug's already-
//     written review output to update local skill PROVENANCE.md — reading
//     a review's output is not performing one.
//   - versioning — §7 governs the SPEC document's own version-agreement
//     and tag rules, checked by the protocol repository's own tooling;
//     declaring this binary's own version (this file) is a §7.3
//     obligation every conformant tool has, not an implementation of §7's
//     rules for others.
var Areas = "orient, work, record, propagate, gates"

// SatisfiesMin reports whether v is at least min, using a simple
// major.minor.patch integer compare — NOT full semver precedence. Originally
// added for v2.0.0 B6's derived-docs adoption gate version floor
// (`derived_docs.min_binary`, compared against the running Version by
// `logmind doctor`); that gate — and its only caller — was removed when the
// zero-conflict invariant became unconditional. Kept as a general-purpose
// version-floor helper for any future advisory-only version check.
//
// Deliberately no semver dependency (per the B6 build ruling: "do NOT add a
// dependency") — go.mod carries none today, and the compare this needs is
// small enough not to justify one.
//
// Both v and min tolerate a leading "v" and a trailing prerelease/build
// suffix (anything from the first '-' or '+' onward) — the suffix is
// stripped BEFORE comparing, a deliberate departure from strict semver
// precedence (where "2.0.0-dev" < "2.0.0"): this repo's own dogfood binary
// reports "2.0.0-dev" (see Version above), and it must satisfy its own core
// version as a floor rather than perpetually warning about itself. A caller
// that needs strict prerelease ordering isn't served by this helper — there
// is no such caller today.
//
// A version string that isn't parseable as three dot-separated integers
// (missing a component, non-numeric, empty) makes SatisfiesMin return true
// for BOTH v and min — fail open. A floor check is meant to be an
// advisory-only nudge, never a gate; reporting a false warning off a
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
