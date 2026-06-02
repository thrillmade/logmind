// Package version holds the build-time version constants for logmind.
//
// During the v1.0 Go rewrite the version string is "1.0.0-dev"; it will
// be cut to "1.0.0" at the v1-go-rewrite → main cutover PR. The spec
// version tracks the thrillmade/protocol SPEC.md document and is
// currently "0.1.0-draft" — it will be locked once SPEC.md ships.
//
// Wave B7 (distribution): both values are `var` (not `const`) so the
// GoReleaser build step can inject the released tag via `-ldflags
// "-X 'github.com/thrillmade/logmind/internal/version.Version=v1.2.3'"`.
// Local `go build` and `make build` still produce a binary that reports
// the "1.0.0-dev" default — the override only fires on tagged release
// builds running through `.goreleaser.yaml`.
package version

// Version is the logmind binary's semantic version. Bumped at release.
// Overridable via -ldflags at build time; see package docstring.
var Version = "1.0.0-dev"

// SpecVersion is the version of the thrillmade/protocol logmind contract
// this binary implements. Reported via `logmind --version` so downstream
// tools can detect protocol skew without parsing the binary version.
// Overridable via -ldflags at build time; see package docstring.
var SpecVersion = "0.1.0-draft"
