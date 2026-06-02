// Package agents is the registry of every AI agent tool logmind ships
// per-project instruction files for. The registry mirrors
// src/logmind/core/inserter.AGENT_REGISTRY at v0.6.14 in ITERATION
// ORDER and content so `logmind agents list`, `agents add`,
// `agents remove`, and `agents migrate` all render the same agents in
// the same order as the Python CLI.
//
// Iteration order matters because the `agents list` output is
// position-stable for downstream tooling — agents that grep the
// output for their tool's row need it to land at a deterministic line.
// The Python implementation uses a dict literal whose insertion order
// is preserved by CPython 3.7+; we replicate that with the order slice
// alongside the lookup map.
package agents

import "path/filepath"

// Agent describes a single registered AI tool: where its per-project
// instruction file lives (relative to repo root), how to display the
// tool's name to humans, and whether the file format is JSON (in which
// case the marker-block surgical rewriter doesn't apply).
type Agent struct {
	// Name is the canonical short name used as the CLI argument
	// (`logmind agents add <name>`). MUST match Python's registry key.
	Name string
	// FilePattern is the repo-root-relative path the file lives at.
	// Always forward-slash, even on Windows — this is a logical path
	// for display + lookup, converted to OS-native via filepath.Join
	// when used as an actual filesystem path.
	FilePattern string
	// Display is the human-readable tool name shown in error messages.
	Display string
	// IsJSON is true for tools whose config is a .json file (cody,
	// zed). The marker-block surgical rewriter and the stub model
	// don't apply to JSON formats — they get their JSON template
	// written as-is and never inserted into.
	IsJSON bool
}

// Names returns every registered agent name in the canonical
// insertion order. The slice is shared (not cloned) — callers MUST
// NOT mutate it.
//
// Used by the help text on `agents add` / `agents remove` to render
// the comma-joined list of valid names, and by `agents list` as the
// iteration order over registered agents.
func Names() []string { return registryOrder }

// All returns every registered agent in canonical order. Returned
// slice is freshly allocated so callers can sort / filter / mutate
// without affecting the registry.
func All() []Agent {
	out := make([]Agent, len(registryOrder))
	for i, name := range registryOrder {
		out[i] = registry[name]
	}
	return out
}

// Lookup returns the Agent for a registered name, plus an `ok` bool
// matching the Python "if name in AGENT_REGISTRY" check. Unknown
// names return the zero value + false; callers print the
// `Error: Unknown agent '<name>'.` message and exit 1.
func Lookup(name string) (Agent, bool) {
	a, ok := registry[name]
	return a, ok
}

// FilePath returns the OS-native absolute path of an agent's
// instruction file rooted at repoRoot. Returns "" + false for an
// unknown agent — callers should print the `Unknown agent` error.
//
// Uses filepath.Join so the OS path separator wins on Windows.
func FilePath(name, repoRoot string) (string, bool) {
	a, ok := registry[name]
	if !ok {
		return "", false
	}
	return filepath.Join(repoRoot, filepath.FromSlash(a.FilePattern)), true
}

// IsJSON returns true when an agent's file format is JSON. Mirrors
// src/logmind/core/inserter.is_agent_json. Returns false for unknown
// agents (defensive — matches Python's default-false behaviour).
func IsJSON(name string) bool {
	if a, ok := registry[name]; ok {
		return a.IsJSON
	}
	return false
}

// DefaultEnabled reports the default-enabled set per SPEC §1.2 — the
// agents whose files `logmind init` writes by default when there's
// no `.logmind/config.yml` override. Currently {claude, cursor},
// matching the Python DEFAULT_CONFIG["agents"] entries set to True.
//
// Order matches Python iteration: claude first, cursor second. The
// `agents` Python dict literal preserves that ordering across the
// default config and the registry, so we pin it here too.
func DefaultEnabled() []string {
	return []string{"claude", "cursor"}
}

// registryOrder is the canonical insertion order. MUST mirror the key
// order of src/logmind/core/inserter.AGENT_REGISTRY.
var registryOrder = []string{
	"claude",
	"cursor",
	"copilot",
	"windsurf",
	"aider",
	"continue",
	"cody",
	"zed",
	"amazonq",
	"cline",
	"codex",
}

// registry maps name → Agent. Lookups are O(1) so the `agents add`
// validation check runs in constant time. Keep in sync with
// registryOrder.
var registry = map[string]Agent{
	"claude":   {Name: "claude", FilePattern: "CLAUDE.md", Display: "Claude Code", IsJSON: false},
	"cursor":   {Name: "cursor", FilePattern: ".cursorrules", Display: "Cursor", IsJSON: false},
	"copilot":  {Name: "copilot", FilePattern: ".github/copilot-instructions.md", Display: "GitHub Copilot", IsJSON: false},
	"windsurf": {Name: "windsurf", FilePattern: ".windsurfrules", Display: "Windsurf", IsJSON: false},
	"aider":    {Name: "aider", FilePattern: "CONVENTIONS.md", Display: "Aider", IsJSON: false},
	"continue": {Name: "continue", FilePattern: ".continuerules", Display: "Continue", IsJSON: false},
	"cody":     {Name: "cody", FilePattern: ".sourcegraph/cody.json", Display: "Sourcegraph Cody", IsJSON: true},
	"zed":      {Name: "zed", FilePattern: ".zed/settings.json", Display: "Zed AI", IsJSON: true},
	"amazonq":  {Name: "amazonq", FilePattern: ".amazonq/rules.md", Display: "Amazon Q", IsJSON: false},
	"cline":    {Name: "cline", FilePattern: ".clinerules", Display: "Cline", IsJSON: false},
	"codex":    {Name: "codex", FilePattern: "AGENTS.md", Display: "OpenAI Codex", IsJSON: false},
}
