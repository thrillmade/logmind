package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

// provenanceTemplate is the empty PROVENANCE.md skeleton emitted
// alongside every newly-created SKILL.md. Per the v0.6.x plan:
//
//   - derived-from-decisions list starts empty (filled in by future
//     `logmind sync` runs once decision IDs are stable)
//   - cited-by-clud-bug counter starts at 0 (incremented by the
//     clud-bug usage emitter)
//   - last-refined date starts blank (set by `logmind skill log` or
//     `logmind sync` whenever the skill body is touched in earnest)
//
// Keeping the keys named exactly like the plan section "Skill workflow
// loop" so future tooling can grep them without a translation layer.
const provenanceTemplate = `<!-- logmind:provenance v1 -->
# Provenance for skill: %s

This file is maintained by ` + "`logmind`" + ` and ` + "`clud-bug`" + ` to track:

- which logmind decisions justified the skill (` + "`derived-from-decisions`" + `)
- how often clud-bug-review cited the skill in PR reviews (` + "`cited-by-clud-bug`" + `)
- when the skill body was last refined (` + "`last-refined`" + `)

` + "```yaml" + `
derived-from-decisions: []
cited-by-clud-bug: 0
last-refined: ""
refinement-history: []
` + "```" + `

_Tooling rewrites the YAML block above. Free-form notes below the
block are preserved across rewrites — add author commentary, design
sketches, or links to PRs here._

---
`

// WriteProvenanceSkeleton writes a PROVENANCE.md skeleton next to the
// supplied SKILL.md path. The skill name is interpolated into the
// heading so the file is human-readable at a glance.
//
// Returns an error wrapping os.ErrExist when PROVENANCE.md already
// exists — callers shouldn't be surprised when a re-run preserves a
// previously edited provenance file.
func WriteProvenanceSkeleton(skillMDPath, name string) error {
	dir := filepath.Dir(skillMDPath)
	target := filepath.Join(dir, "PROVENANCE.md")
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("PROVENANCE.md already exists at %s: %w", target, os.ErrExist)
	}
	body := fmt.Sprintf(provenanceTemplate, name)
	return os.WriteFile(target, []byte(body), 0o644)
}
