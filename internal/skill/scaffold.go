// Package skill implements the core logic for `logmind skill new/test/
// bench/audit/suggest`. Mirrors src/logmind/core/skill_cli.py at
// v0.6.16 — every public function here has a Python counterpart that
// the byte-identical-parity snapshot tests pin.
//
// Why a dedicated package vs inlining into internal/cli/: keeps the
// cobra wiring layer thin (one file: internal/cli/skill.go) and lets
// future tooling (e.g., `logmind sync` in B5b) import these helpers
// without dragging the whole CLI tree.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillsDir returns the canonical location for SKILL.md files in a repo.
//
// Defaults to `.claude/skills/` — the Claude Code convention that
// clud-bug and the agent-skills catalog use. Other harnesses (Cursor,
// Codex) follow the same per-skill subdirectory layout, so this works
// for any consumer. Mirrors Python's default_skills_dir().
func SkillsDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".claude", "skills")
}

// SkillDir returns the canonical directory for a skill of the given name.
func SkillDir(repoRoot, name string) string {
	return filepath.Join(SkillsDir(repoRoot), name)
}

// MDPath returns the SKILL.md path for the given skill name under
// repoRoot. Mirrors Python's skill_md_path().
func MDPath(repoRoot, name string) string {
	return filepath.Join(SkillDir(repoRoot, name), "SKILL.md")
}

// basicTemplate is the byte-identical template body Python emits when
// skdd is not on PATH. Each {NAME} / {DESC} / {TITLE} is replaced;
// everything else is copied verbatim. Maintaining the literal here is
// the load-bearing parity contract — drift here breaks the snapshot
// tests.
//
// Why a literal vs a Go template: text/template would force escaping
// the literal `{`/`}` characters used elsewhere in SKILL.md bodies.
// Direct string-replace stays simple and matches Python's str.format
// semantics exactly.
const basicTemplate = `---
name: {NAME}
description: {DESC}
metadata:
  logmind-managed: true
  spec: agentskills.io
---

# {TITLE}

<!-- One short paragraph describing what this skill does + when an agent
should load it. The description field above is the discovery surface;
this body is what the agent reads once they decide to load it. -->

## When to use

- Trigger condition 1
- Trigger condition 2

## Steps

1. Step one
2. Step two

## Examples

<!-- Concrete examples make skills easier to apply correctly. -->
`

// ScaffoldBasic creates a minimal agentskills.io/v1-compliant SKILL.md
// under .claude/skills/<name>/SKILL.md. Mirrors Python's
// scaffold_basic_skill — same template body, same TODO fallback for an
// empty description, same FileExistsError-style refusal to clobber.
//
// Returns the absolute path of the SKILL.md it created.
//
// Refuses to clobber an existing SKILL.md (returns an error wrapping
// os.ErrExist) — the caller (cli.skill) translates that into the same
// red-error + exit-1 shape Python emits.
func ScaffoldBasic(repoRoot, name, description string) (string, error) {
	target := MDPath(repoRoot, name)
	if _, err := os.Stat(target); err == nil {
		return target, fmt.Errorf("skill %q already exists at %s: %w",
			name, target, os.ErrExist)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return target, err
	}

	if description == "" {
		// Python's auto-TODO description — keep byte-identical so the
		// generated SKILL.md matches.
		description = fmt.Sprintf(
			"TODO: one-sentence trigger description for '%s'. "+
				"Be specific — this field is the discovery surface.",
			name,
		)
	}

	body := basicTemplate
	body = strings.ReplaceAll(body, "{NAME}", name)
	body = strings.ReplaceAll(body, "{DESC}", description)
	body = strings.ReplaceAll(body, "{TITLE}", titleCase(name))

	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		return target, err
	}
	return target, nil
}

// titleCase mirrors Python's `name.replace("-", " ").replace("_", " ").title()`
// for skill-name titles. Python's str.title() uppercases any letter
// that follows a non-letter and lowercases letters following letters,
// preserving non-letter characters and multi-space runs verbatim.
//
// Implementing this directly (vs strings.Fields + Join) is load-bearing
// for parity: `"foo--bar".title()` returns `"Foo  Bar"` with the double
// space intact, and `"_hidden".title()` returns `" Hidden"` with the
// leading space preserved. The snapshot tests in scaffold_test.go pin
// this edge-case behaviour against the Python reference.
//
// Examples:
//
//	"my-test-skill" → "My Test Skill"   (after dash-replace)
//	"foo--bar"      → "Foo  Bar"        (after dash-replace; preserves double space)
//	"abc123-def"    → "Abc123 Def"      (digits don't reset the casing)
//	"foo3bar"       → "Foo3Bar"         (Python re-caps after digit)
func titleCase(name string) string {
	spaced := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
	if spaced == "" {
		return ""
	}
	out := make([]rune, 0, len(spaced))
	prevIsLetter := false
	for _, r := range spaced {
		isLetter := isASCIILetter(r)
		switch {
		case isLetter && !prevIsLetter:
			out = append(out, toASCIIUpper(r))
		case isLetter && prevIsLetter:
			out = append(out, toASCIILower(r))
		default:
			out = append(out, r)
		}
		prevIsLetter = isLetter
	}
	return string(out)
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func toASCIIUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - ('a' - 'A')
	}
	return r
}

func toASCIILower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}
