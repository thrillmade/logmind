// Package version holds the build-time version constants for logmind.
//
// v1.0.0 shipped 2026-06-03 via the v1-go-rewrite → main cutover (#132).
// Local `go build` and `make build` produce a binary reporting the
// "1.0.0-dev" default; tagged release builds running through
// `.goreleaser.yaml` override via `-ldflags
// "-X 'github.com/thrillmade/logmind/internal/version.Version=v1.2.3'"`
// to inject the actual tag.
//
// SpecVersion tracks the thrillmade/protocol SPEC.md document — bumped
// to "0.8.0" 2026-07-03 (protocol #26: the Slice-2 main-canonical
// timeline pass — §1.6.4 union assembly, the agent-authored headline
// convention, the headline/doctor CLI, and the §5.1.2 advisory-first
// regen-timeline template).
package version

// Version is the logmind binary's semantic version. Bumped at release.
// Overridable via -ldflags at build time; see package docstring.
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
var Version = "1.2.0-dev"

// SpecVersion is the version of the thrillmade/protocol logmind contract
// this binary implements. Reported via `logmind --version` so downstream
// tools can detect protocol skew without parsing the binary version.
// Overridable via -ldflags at build time; see package docstring.
var SpecVersion = "0.8.0"
