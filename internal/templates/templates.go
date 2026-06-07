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

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed AGENTS.md.template AGENTS.md.slim.template agent-stub.md logmind-section.md CLAUDE.md.template
//go:embed config.yml.template decisions.md.template decisions-archive.md.template file-structure.md.template
//go:embed decisions-branch-header.md.template
//go:embed dependabot.yml.template
//go:embed github/*.yml.template
var embedFS embed.FS

// AgentsTemplate returns the full v6 AGENTS.md template (the inline
// procedure variant) — used when the host doesn't have skills.sh
// available, or when the caller explicitly requests the full body.
//
// The block between `<!-- logmind-start -->` and `<!-- logmind-end -->`
// carries the version marker `<!-- logmind-block-version: v6 -->`. The
// inserter package uses that marker to decide whether an installed block
// is stale.
//
// v0.6.16 bumped v5→v6: heading reframed as "REQUIRED for substantive
// commits", added an explicit DO-NOT-git-commit blockquote that pairs
// with the commit-msg hook installed by `logmind init`.
func AgentsTemplate() string {
	return readEmbed("AGENTS.md.template")
}

// AgentsSlimTemplate returns the slim v8-pointer AGENTS.md template
// (defaults to slim for new repos since logmind v0.6.8+). Body marker
// is `<!-- logmind-block-version: v8-pointer -->`.
//
// Slim defers the WHAT/WHEN/HOW procedure to the `logmind` skill on
// skills.sh — short body, less to maintain, less for the agent to wade
// through if the skill is already installed.
//
// v0.6.16 bumped v7-pointer→v8-pointer: heading now "REQUIRED for
// substantive commits", added an explicit DO-NOT-git-commit blockquote
// pairing with the commit-msg hook installed by `logmind init`.
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
func FS() embed.FS { return embedFS }

// readEmbed is a thin wrapper around embedFS.ReadFile that panics on a
// missing entry. Every name fed into it appears in the //go:embed
// directive above — a panic here means a developer dropped a name in
// the directive without adding the corresponding file, which we want
// to surface at first call, not at silent empty-string time.
func readEmbed(name string) string {
	data, err := embedFS.ReadFile(name)
	if err != nil {
		// embed.FS errors at runtime are programmer bugs (missing file
		// in directive). Panic so they're impossible to ignore.
		panic("templates: missing embedded template " + name + ": " + err.Error())
	}
	return string(data)
}

// ConfigTemplate returns the bundled .logmind/config.yml seed content
// emitted by `logmind init`. Byte-identical to
// src/logmind/templates/config.yml.template.
func ConfigTemplate() string {
	return readEmbed("config.yml.template")
}

// DecisionsTemplate returns the bundled docs/decisions.md seed.
func DecisionsTemplate() string {
	return readEmbed("decisions.md.template")
}

// DecisionsArchiveTemplate returns the bundled docs/decisions-archive.md seed.
func DecisionsArchiveTemplate() string {
	return readEmbed("decisions-archive.md.template")
}

// FileStructureTemplate returns the bundled docs/file-structure.md seed
// (used as a placeholder before the first real tree walk overwrites it).
func FileStructureTemplate() string {
	return readEmbed("file-structure.md.template")
}

// DecisionsBranchHeader returns the single-line backlink header
// prepended to a freshly-created `docs/decisions-branches/<branch>.md`
// file by `logmind log`. Format (POSIX line endings):
//
//	← back to [docs/timeline.md](../timeline.md)
//	<blank line>
//
// Ships in v1.2.0 alongside the Go port of `log` (plan §8.7 deliverable
// 3). Provides bidirectional linking by design: timeline.md already
// links INTO each branch decision file; the header completes the
// round-trip so an agent reading a branch file can navigate back to
// the canonical entry point without re-running `find`.
//
// The header is written ONLY on first creation of the branch decision
// file — subsequent `logmind log` invocations on the same branch
// append the new decision entry after existing content (header
// preserved verbatim).
//
// Not added to the default-branch `docs/decisions.md` (no `..` parent
// hop needed; the link target would be `timeline.md` not
// `../timeline.md`, and `decisions.md` is the original entry point
// rather than a derived per-branch file).
func DecisionsBranchHeader() string {
	return readEmbed("decisions-branch-header.md.template")
}

// DependabotTemplate returns the bundled .github/dependabot.yml seed
// shipped since logmind v1.1.0. Carries a single `github-actions`
// ecosystem entry with a `thrillmade` group that bundles
// `thrillmade/*` action bumps (notably `thrillmade/setup-logmind@vX.Y.Z`)
// into one PR per release. Used by the init/refresh path via
// inserter.EnsureDependabot — see that function for the merge
// semantics when the consumer repo already has a dependabot.yml.
func DependabotTemplate() string {
	return readEmbed("dependabot.yml.template")
}

// Workflow returns the embedded body of a single .github/workflows/<name>.yml
// template by its bare filename (e.g. "regen-timeline.yml.template"). The
// returned string is the raw template body with __LOGMIND_VERSION__
// placeholders still in place — callers must render via RenderWorkflow.
func Workflow(name string) string {
	return readEmbed("github/" + name)
}

// ListWorkflowTemplates returns the sorted list of bundled GitHub workflow
// template filenames (each ending in `.yml.template`). Used by the init
// path so adding a workflow template is purely additive — drop the file
// into internal/templates/github/ and re-run `go build`.
func ListWorkflowTemplates() []string {
	entries, err := embedFS.ReadDir("github")
	if err != nil {
		panic("templates: cannot list embedded github/: " + err.Error())
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml.template") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// WalkEmbedded returns an fs.FS view of the templates dir; tests use it
// to enumerate embedded files without re-listing them.
func WalkEmbedded() fs.FS { return embedFS }
