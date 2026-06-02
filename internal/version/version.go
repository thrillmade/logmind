// Package version holds the build-time version constants for logmind.
//
// During the v1.0 Go rewrite the version string is "1.0.0-dev"; it will
// be cut to "1.0.0" at the v1-go-rewrite → main cutover PR. The spec
// version tracks the thrillmade/protocol SPEC.md document and is
// currently "0.1.0-draft" — it will be locked once SPEC.md ships.
package version

// Version is the logmind binary's semantic version. Bumped at release.
const Version = "1.0.0-dev"

// SpecVersion is the version of the thrillmade/protocol logmind contract
// this binary implements. Reported via `logmind --version` so downstream
// tools can detect protocol skew without parsing the binary version.
const SpecVersion = "0.1.0-draft"
