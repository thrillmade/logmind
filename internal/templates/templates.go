// Package templates embeds the canonical AGENTS.md, per-agent stub, and
// per-tool legacy templates so the Go binary doesn't need to ship them as
// loose files on disk. The embedded bytes originated as BYTE-IDENTICAL
// copies of src/logmind/templates/* at v0.6.14 — see the verification
// step in the B4 PR description. (src/logmind was deleted in 5979eef;
// the byte-parity check against it is gone with it — see
// templates_test.go's marker/shape tests for the tree's current
// self-checks.)
//
// Why embed.FS vs string constants:
//
//   - Python loads templates via `(Path(__file__).parent.parent /
//     "templates" / name).read_text(encoding="utf-8")`. Mapping that to
//     a single `embed.FS` lets the Go binary use the SAME file names
//     (AGENTS.md.template, AGENTS.md.slim.template, agent-stub.md,
//     logmind-section.md) so future template additions cost nothing —
//     drop the file into internal/templates/ and re-run `go build`.
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
	"sort"
	"strings"
)

//go:embed AGENTS.md.template AGENTS.md.slim.template agent-stub.md logmind-section.md
//go:embed config.yml.template file-structure.md.template
//go:embed decisions-branch-header.md.template
//go:embed dependabot.yml.template
//go:embed spec.md.template
//go:embed github/*.yml.template
//go:embed auto/*.yml.template
var embedFS embed.FS

// AgentsTemplate returns the full v8 AGENTS.md template (the inline
// procedure variant) — used when the host doesn't have skills.sh
// available, or when the caller explicitly requests the full body.
//
// The block between `<!-- logmind-start -->` and `<!-- logmind-end -->`
// carries the version marker `<!-- logmind-block-version: v8 -->`. The
// inserter package uses that marker to decide whether an installed block
// is stale.
//
// The stale-binary-hardening / enforcement wave bumped v7→v8: the
// DO-NOT-git-commit blockquote now reflects that the commit-msg hook and
// the Claude Code PreToolUse hook installed by `logmind init` / `logmind
// doctor --fix` BLOCK a substantive commit lacking a decision (not just
// warn), and documents the carve-outs (`[skip-logmind]`,
// `LOGMIND_ALLOW_GIT_COMMIT=1`, `git.enforce_commits: false`). The
// Slice-2 branch-summary wave bumped v6→v7: added the branch-summary
// (headline) convention to the inline procedure. v0.6.16 bumped v5→v6:
// heading reframed as "REQUIRED for substantive commits", added an
// explicit DO-NOT-git-commit blockquote that pairs with the commit-msg
// hook installed by `logmind init`.
func AgentsTemplate() string {
	return readEmbed("AGENTS.md.template")
}

// AgentsSlimTemplate returns the slim v9-pointer AGENTS.md template
// (defaults to slim for new repos since logmind v0.6.8+). Body marker
// is `<!-- logmind-block-version: v9-pointer -->`.
//
// Slim defers the WHAT/WHEN/HOW procedure to the `logmind` skill on
// skills.sh — short body, less to maintain, less for the agent to wade
// through if the skill is already installed.
//
// The stale-binary-hardening / enforcement wave bumped v8-pointer→v9-pointer:
// same enforcement-prose update as the full template's v7→v8 bump (BLOCKS,
// not warns; documents the `[skip-logmind]` / `LOGMIND_ALLOW_GIT_COMMIT=1` /
// `git.enforce_commits: false` carve-outs), condensed to match the slim
// flavour's tone/length. v0.6.16 bumped v7-pointer→v8-pointer: heading now
// "REQUIRED for substantive commits", added an explicit DO-NOT-git-commit
// blockquote pairing with the commit-msg hook installed by `logmind init`.
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

// SpecTemplate returns the bundled docs/spec.md seed — the skeleton for the
// project's canonical, forward-looking spec (see `logmind init --spec`).
// Unlike every other file this package seeds, the spec is never
// regenerated: `logmind init --spec` writes this template ONLY when
// docs/spec.md is absent, then the file is edited by hand via normal PRs.
func SpecTemplate() string {
	return readEmbed("spec.md.template")
}

// Workflow returns the embedded body of a single .github/workflows/<name>.yml
// template by its bare filename (e.g. "regen-timeline.yml.template"). The
// returned string is the raw template body with __LOGMIND_VERSION__
// placeholders still in place — callers must render via RenderWorkflow.
func Workflow(name string) string {
	return readEmbed("github/" + name)
}

// AutoDirective returns the embedded body of one `logmind auto <profile>`
// standing-directive template by its bare filename (e.g.
// "unattended.yml.template"). The body still carries the
// __LOGMIND_CHECKPOINT__ placeholder — callers render it (see
// internal/auto.Render), exactly as Workflow bodies carry
// __LOGMIND_VERSION__.
//
// Each body's FIRST line is `# logmind-auto-version: vN`, the SPEC §5.2
// ownership marker. Bump N whenever the directive's content changes —
// `logmind doctor` compares the installed marker against this one and
// nudges when a repo's directive predates the current policy. The bodies
// restate two skills (session-heartbeat, unattended-operation); a change
// on either side is what the bump exists to surface.
func AutoDirective(name string) string {
	return readEmbed("auto/" + name)
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
