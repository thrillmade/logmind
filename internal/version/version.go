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
var Version = "1.0.0-dev"

// SpecVersion is the version of the thrillmade/protocol logmind contract
// this binary implements. Reported via `logmind --version` so downstream
// tools can detect protocol skew without parsing the binary version.
// Overridable via -ldflags at build time; see package docstring.
var SpecVersion = "0.1.1"
