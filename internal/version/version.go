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
// to "0.1.1" 2026-06-03 (protocol PR #1 + tag v0.1.1: align AGENTS.md
// marker versions v5→v6 and v7-pointer→v8-pointer per §8.3).
package version

// Version is the logmind binary's semantic version. Bumped at release.
// Overridable via -ldflags at build time; see package docstring.
//
// v1.2.1 (2026-06-08) — patch release closing 5 SPEC + UX gaps the
// 2026-06-08 audit found:
//
//   - `logmind show` ported to Go (SPEC §A.3 REQUIRED). Restored from
//     Python v0.6.16 with byte-identical surface (decisions.md verbatim
//     default, --all/--brief/--limit/--json views).
//   - `logmind search` ported to Go (SPEC §A.3 REQUIRED). Restored from
//     Python v0.6.16: regex/literal search across decisions.md +
//     decisions-archive.md with ±N context lines.
//   - `logmind log` stdout now matches SPEC §3.1 byte-identical 3-line
//     contract on non-TTY / --no-interactive paths. Interactive paths
//     emit extended advisory + retry loop AFTER the 3 lines (SPEC
//     v0.2.0 §3.1.1 exemption, in flight in protocol#3).
//   - `logmind log` push behavior restored. Python pushed after commit
//     by default; Go silently dropped it. v1.2.1 reintroduces push via
//     gitcli.Push() honoring config.git.auto_push (default true) + new
//     --no-push CLI flag for opt-out parity with Python.
//   - P0 bot commit author email fix. regen-timeline.yml.template v5
//     bumped to use the canonical `github-actions[bot]` identity
//     (`41898282+github-actions[bot]@users.noreply.github.com`) instead
//     of the fake `logmind-auto-regen@users.noreply.github.com` that
//     no GitHub user owns. Vercel + many CI integrations REJECT
//     deployments authored by it (caught on clud-bug-app PR #22
//     preview deploy 2026-06-08).
//   - Template setup-logmind pins bumped v1.0.0 → v1.0.1.
//   - regen-timeline.yml.template v5 — single-job GITHUB_TOKEN
//     dual-mode (mirrors check-doc-links v5 fix). Unblocks Dependabot
//     PRs that can't access secret-scoped LOGMIND_AUTO_REGEN_PAT.
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
// templates: they use `uses: thrillmade/setup-logmind@v1.0.1` and let
// Dependabot bump the action pin. install.sh defaults to the latest
// release (no hardcoded sweeps) and detects GITHUB_ACTIONS=true to
// nudge CI users at setup-logmind.
var Version = "1.2.1"

// SpecVersion is the version of the thrillmade/protocol logmind contract
// this binary implements. Reported via `logmind --version` so downstream
// tools can detect protocol skew without parsing the binary version.
// Overridable via -ldflags at build time; see package docstring.
var SpecVersion = "0.1.1"
