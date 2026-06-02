// Package templates embeds the canonical AGENTS.md, per-agent stub, and
// per-tool legacy templates so the Go binary doesn't need to ship them as
// loose files on disk. The embedded bytes are BYTE-IDENTICAL copies of
// src/logmind/templates/* at v0.6.14 — see the verification step in the
// B4 PR description and the package-level test that diffs the embed
// against the Python source tree at test time.
//
// Why embed.FS vs string constants:
//
//   - Python loads templates via `(Path(__file__).parent.parent /
//     "templates" / name).read_text(encoding="utf-8")`. Mapping that to
//     a single `embed.FS` lets the Go binary use the SAME file names
//     (AGENTS.md.template, AGENTS.md.slim.template, agent-stub.md,
//     logmind-section.md, CLAUDE.md.template) so future template
//     additions cost nothing — drop the file into internal/templates/
//     and re-run `go build`.
//
//   - String constants would force a name-mangling step (the Python
//     path "AGENTS.md.slim.template" can't be a Go identifier) and
//     drift would be invisible: a stale constant won't trip a build
//     error the way a missing embed file does.
//
// The templates ship under the same v0.6.14 byte image so this package
// is the SINGLE source of truth for "what does the current template
// look like" — the inserter package reads via AgentsTemplate() /
// AgentsSlimTemplate() / Stub() / LogmindSection() and never opens
// the filesystem directly.
package templates

import "embed"

//go:embed AGENTS.md.template AGENTS.md.slim.template agent-stub.md logmind-section.md CLAUDE.md.template
var fs embed.FS

// AgentsTemplate returns the full v5 AGENTS.md template (the inline
// procedure variant) — used when the host doesn't have skills.sh
// available, or when the caller explicitly requests the full body.
//
// The block between `<!-- logmind-start -->` and `<!-- logmind-end -->`
// carries the version marker `<!-- logmind-block-version: v5 -->`. The
// inserter package uses that marker to decide whether an installed block
// is stale.
func AgentsTemplate() string {
	return readEmbed("AGENTS.md.template")
}

// AgentsSlimTemplate returns the slim v7-pointer AGENTS.md template
// (defaults to slim for new repos since logmind v0.6.8+). Body marker
// is `<!-- logmind-block-version: v7-pointer -->`.
//
// Slim defers the WHAT/WHEN/HOW procedure to the `logmind` skill on
// skills.sh — short body, less to maintain, less for the agent to wade
// through if the skill is already installed.
func AgentsSlimTemplate() string {
	return readEmbed("AGENTS.md.slim.template")
}

// Stub returns the 2-line per-agent stub that replaces in-place agent
// instruction files when `logmind agents migrate` consolidates content
// into AGENTS.md. Carries the `<!-- logmind-stub: ... -->` marker so
// the inserter package can identify a stub without re-parsing the
// pointer phrase.
func Stub() string {
	return readEmbed("agent-stub.md")
}

// LogmindSection returns the legacy in-place insertion block — the
// section that gets spliced into an existing CLAUDE.md / .cursorrules /
// etc. when the user runs `logmind agents add <name>` against a file
// that already has user content (so the file isn't a stub yet).
//
// Still shipped for parity with Python's insert_logmind_section path;
// new repos default to AGENTS.md + stubs via migrate.
func LogmindSection() string {
	return readEmbed("logmind-section.md")
}

// FullClaudeTemplate returns the standalone CLAUDE.md template used by
// the legacy `create_claude_md` Python path. Preserved here for parity
// with src/logmind/core/inserter.get_full_claude_template().
func FullClaudeTemplate() string {
	return readEmbed("CLAUDE.md.template")
}

// FS exposes the underlying embed.FS so tests can iterate every shipped
// template name + byte content without each test re-hardcoding the
// filename list. NOT for production use — production code should
// always go through the named accessors above so a typo on a filename
// trips at compile time, not runtime.
func FS() embed.FS { return fs }

// readEmbed is a thin wrapper around fs.ReadFile that panics on a
// missing entry. Every name fed into it appears in the //go:embed
// directive above — a panic here means a developer dropped a name in
// the directive without adding the corresponding file, which we want
// to surface at first call, not at silent empty-string time.
func readEmbed(name string) string {
	data, err := fs.ReadFile(name)
	if err != nil {
		// embed.FS errors at runtime are programmer bugs (missing file
		// in directive). Panic so they're impossible to ignore.
		panic("templates: missing embedded template " + name + ": " + err.Error())
	}
	return string(data)
}
