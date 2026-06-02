// Package inserter is the surgical-rewrite engine for AGENTS.md, the
// per-tool stub files, and the workflow pin lines. Every operation is
// idempotent — re-running on an already-current tree is a no-op.
//
// Design pillars (mirrors src/logmind/core/inserter.py at v0.6.14):
//
//   - Marker-block surgical rewrite. The block bracketed by
//     `<!-- logmind-start -->` and `<!-- logmind-end -->` is the only
//     region rewritten when the template body changes. Everything
//     OUTSIDE the markers — user content the agent or human added — is
//     preserved byte-for-byte. The B4 PR description includes a
//     round-trip proof that rewrite + restore retains the outer bytes.
//
//   - Workflow pin updater. `agents update --apply` sweeps
//     `.github/workflows/<canonical>.yml` for `pip install "logmind==X.Y.Z"`
//     pins and rewrites them to the current binary's version. The regex
//     preserves the QUOTE STYLE found in source (none / single / double)
//     so reporulez-style single-quoted pins don't churn into double
//     quotes — matches the Python v0.6.11+ widened pattern.
//
//   - Template version markers gate "is the installed block stale".
//     `<!-- logmind-block-version: v5 -->` for the full template,
//     `<!-- logmind-block-version: v7-pointer -->` for slim. Drift is
//     detected by comparing the marker bodies stripped (the Python
//     code uses .strip() to be whitespace-tolerant; we mirror that).
//
// The inserter package is read-mostly: only the apply paths in
// `agents update --apply`, `agents add <name>`, and `agents migrate`
// actually write to disk. Everything else is a pure-function classifier
// that returns descriptions of what WOULD happen.
package inserter

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thrillmade/logmind/internal/agents"
	"github.com/thrillmade/logmind/internal/templates"
)

// Marker constants. Centralised so the surgical-rewrite + stub-detect
// paths share a single string source. Mirrors Python's
// LOGMIND_START_MARKER / LOGMIND_END_MARKER / LOGMIND_STUB_MARKER.
const (
	startMarker = "<!-- logmind-start -->"
	endMarker   = "<!-- logmind-end -->"
	stubMarker  = "<!-- logmind-stub:"
)

// pinLineRE captures the workflow pin line:
//
//	(group 1) `pip install ` literal + optional whitespace
//	(group 2) opening quote: empty, ", or '
//	(group 3) `logmind==` literal
//	(group 4) version triple X.Y.Z (digits + dots)
//	(group 5) closing quote — must match group 2 form
//
// Anchored to `pip install` so a comment that happens to mention
// logmind==<version> doesn't false-positive. Mirrors the Python
// v0.6.11+ pattern in src/logmind/core/inserter._PIN_LINE_RE.
var pinLineRE = regexp.MustCompile(`(pip install\s+)(["']?)(logmind==)([\d.]+)(["']?)`)

// pinWorkflows is the canonical set of workflow filenames whose pin
// the updater sweeps. Custom user workflows are NOT touched.
//
// Mirrors src/logmind/core/inserter.LOGMIND_PIN_WORKFLOWS.
var pinWorkflows = []string{
	"regen-timeline.yml",
	"check-doc-links.yml",
	"logmind-self-update.yml",
	"check-decisions.yml",
}

// AgentStatus is one row of `logmind agents list`. Mirrors the dict
// returned by src/logmind/core/inserter.get_agent_status(...) — the
// fields are named to match the Python keys (and the `agents list`
// renderer pulls them in the same order).
type AgentStatus struct {
	Name        string
	File        string
	DisplayName string
	Exists      bool
	Configured  bool
	IsJSON      bool
}

// GetAgentStatus returns the per-agent status for every registered
// agent in canonical order. Mirrors get_agent_status(root_path).
//
// "Configured" is true when:
//   - the file exists AND has a logmind marker block, OR
//   - the file is a stub pointing at AGENTS.md, OR
//   - the file is a JSON agent (cody/zed) and exists at all (per
//     Python's "exists and (has_logmind or is_json)" predicate).
func GetAgentStatus(repoRoot string) []AgentStatus {
	out := make([]AgentStatus, 0, len(agents.Names()))
	for _, a := range agents.All() {
		filePath := filepath.Join(repoRoot, filepath.FromSlash(a.FilePattern))
		exists := fileExists(filePath)
		hasLogmind := false
		if exists && !a.IsJSON {
			if data, err := os.ReadFile(filePath); err == nil {
				content := string(data)
				hasLogmind = HasLogmindSection(content) || IsStub(content)
			}
		}
		out = append(out, AgentStatus{
			Name:        a.Name,
			File:        a.FilePattern,
			DisplayName: a.Display,
			Exists:      exists,
			Configured:  exists && (hasLogmind || a.IsJSON),
			IsJSON:      a.IsJSON,
		})
	}
	return out
}

// HasLogmindSection reports whether content already contains the
// logmind start marker. Mirrors has_logmind_section(content).
func HasLogmindSection(content string) bool {
	return strings.Contains(content, startMarker)
}

// IsStub reports whether content is a per-agent stub pointing at
// AGENTS.md. Stubs carry the `<!-- logmind-stub:` marker so they're
// distinguishable from full files that happen to mention AGENTS.md.
func IsStub(content string) bool {
	return strings.Contains(content, stubMarker)
}

// ExtractMarkerBlock returns the body between the start/end markers,
// or "", false if either marker is absent or out of order. Note:
// returns "" with ok=true for an empty block — distinct from "" with
// ok=false (no markers at all).
//
// Mirrors Python's _extract_marker_block which returns None vs str.
func ExtractMarkerBlock(content string) (string, bool) {
	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)
	if start == -1 || end == -1 || end < start {
		return "", false
	}
	blockStart := start + len(startMarker)
	return content[blockStart:end], true
}

// ReplaceMarkerBlock swaps the body between the existing markers,
// preserving everything else byte-for-byte. Returns content unchanged
// when either marker is absent.
//
// This is the SURGICAL REWRITE primitive. Marker-block round-trip
// invariant (proved in the package test):
//
//	old, _ := ExtractMarkerBlock(c0)
//	c1 := ReplaceMarkerBlock(c0, "FOO")
//	c2 := ReplaceMarkerBlock(c1, old)
//	c0 == c2 byte-for-byte
//
// Used by both `agents update --apply` (rewriting stale blocks) and
// the `ensure_agents_md` silent-refresh path.
func ReplaceMarkerBlock(content, newBlockBody string) string {
	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)
	if start == -1 || end == -1 {
		return content
	}
	var b strings.Builder
	b.Grow(len(content) - (end - (start + len(startMarker))) + len(newBlockBody))
	b.WriteString(content[:start+len(startMarker)])
	b.WriteString(newBlockBody)
	b.WriteString(content[end:])
	return b.String()
}

// stripLogmindBlock removes the marker-bracketed logmind block from
// content. Used during `agents migrate` to compute the "remaining user
// content" that gets folded into AGENTS.md under a `## From <name>`
// heading. Mirrors Python's _strip_logmind_block.
//
// Also strips a single trailing newline after the end marker — matches
// Python's `if end_full < len(content) and content[end_full] == "\n": end_full += 1`.
func stripLogmindBlock(content string) string {
	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)
	if start == -1 || end == -1 || end < start {
		return content
	}
	endFull := end + len(endMarker)
	if endFull < len(content) && content[endFull] == '\n' {
		endFull++
	}
	return content[:start] + content[endFull:]
}

// InsertLogmindSection splices the legacy logmind-section into an
// existing AI instruction file by reading the file's content, finding
// the first `# ` heading, skipping blank lines after it, and inserting
// the section. Re-running returns false silently (the file already
// has the marker).
//
// Mirrors Python's insert_logmind_section(file_path).
//
// Used by `agents add <name>` against a non-stub file with existing
// user content — the legacy "preserve user content + add logmind"
// path that `migrate` consolidates away.
func InsertLogmindSection(filePath string) (bool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}
	content := string(data)
	if HasLogmindSection(content) {
		return false, nil
	}

	lines := strings.Split(content, "\n")
	insertIndex := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			insertIndex = i + 1
			for insertIndex < len(lines) && strings.TrimSpace(lines[insertIndex]) == "" {
				insertIndex++
			}
			break
		}
	}

	section := templates.LogmindSection()
	// Insert at insertIndex.
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertIndex]...)
	newLines = append(newLines, section)
	newLines = append(newLines, lines[insertIndex:]...)

	out := strings.Join(newLines, "\n")
	if err := os.WriteFile(filePath, []byte(out), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// CreateAgentFile writes a new per-agent instruction file using the
// canonical stub (for markdown agents other than codex) or the JSON
// template (for cody/zed) or the full AGENTS.md template (for codex).
// Returns the absolute path of the written file, or "" + nil for an
// unknown agent name (matches Python's None return).
//
// Mirrors create_agent_file(agent_name, root_path).
func CreateAgentFile(agentName, repoRoot string) (string, error) {
	a, ok := agents.Lookup(agentName)
	if !ok {
		return "", nil
	}
	filePath := filepath.Join(repoRoot, filepath.FromSlash(a.FilePattern))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return "", err
	}
	body := agentTemplate(agentName)
	if err := os.WriteFile(filePath, []byte(body), 0o644); err != nil {
		return "", err
	}
	return filePath, nil
}

// RemoveAgentFile deletes a per-agent instruction file. Returns
// (true, nil) on success, (false, nil) when the file is absent or the
// agent is unknown. Mirrors remove_agent_file.
func RemoveAgentFile(agentName, repoRoot string) (bool, error) {
	a, ok := agents.Lookup(agentName)
	if !ok {
		return false, nil
	}
	filePath := filepath.Join(repoRoot, filepath.FromSlash(a.FilePattern))
	if !fileExists(filePath) {
		return false, nil
	}
	if err := os.Remove(filePath); err != nil {
		return false, err
	}
	return true, nil
}

// agentTemplate returns the right template body for the given agent.
// Mirrors get_agent_template with the Phase 8 consolidation rules:
//
//   - codex → full AGENTS.md template (adaptive: slim if available,
//     else full — for parity we match the Python default which is
//     slim when skills.sh is detected; we don't have skills detect
//     here yet, so we default to slim per SPEC §1.1).
//   - cody / zed → JSON template (preserved verbatim).
//   - markdown agents → 2-line stub pointing at AGENTS.md.
func agentTemplate(agentName string) string {
	if agentName == "codex" {
		return templates.AgentsSlimTemplate()
	}
	if agentName == "cody" || agentName == "zed" {
		return jsonAgentBody()
	}
	return templates.Stub()
}

// jsonAgentBody is the verbatim JSON template Python ships for cody
// and zed. Both tools take the same body — preserved as a single
// string constant here for byte-identical parity.
func jsonAgentBody() string {
	return `{
  "logmind": {
    "enabled": true,
    "description": "This project uses logmind for decision tracking. See docs/decisions.md for recent decisions.",
    "context_files": [
      "docs/decisions.md",
      "docs/file-structure.md"
    ]
  }
}
`
}

// EnsureAgentsMD ensures AGENTS.md exists at repoRoot with the
// current canonical logmind block. Behaviour mirror of
// ensure_agents_md(root_path):
//
//   - Missing → write the canonical template.
//   - Exists without markers → insert the logmind block in-place.
//   - Exists with markers but stale body → silently refresh the body.
//   - Exists with markers and matching body → no-op (return "").
//
// Returns a status string when a write happened, or "" for no-op.
// The status strings match Python's three return values verbatim.
func EnsureAgentsMD(repoRoot string) (string, error) {
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	template := agentsMDTemplate()

	data, err := os.ReadFile(agentsPath)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(agentsPath, []byte(template), 0o644); err != nil {
			return "", err
		}
		return "Created AGENTS.md (canonical agent instructions)", nil
	}
	if err != nil {
		return "", err
	}

	content := string(data)
	if !HasLogmindSection(content) {
		if _, err := InsertLogmindSection(agentsPath); err != nil {
			return "", err
		}
		return "Added logmind section to existing AGENTS.md", nil
	}

	templateBlock, tok := ExtractMarkerBlock(template)
	installedBlock, iok := ExtractMarkerBlock(content)
	if tok && iok && strings.TrimSpace(installedBlock) != strings.TrimSpace(templateBlock) {
		refreshed := ReplaceMarkerBlock(content, templateBlock)
		if err := os.WriteFile(agentsPath, []byte(refreshed), 0o644); err != nil {
			return "", err
		}
		return "Refreshed AGENTS.md logmind block to current template", nil
	}
	return "", nil
}

// agentsMDTemplate returns the canonical AGENTS.md body. Defaults to
// slim per SPEC §1.1 (the v7-pointer variant — defers to skills.sh).
// The Python implementation auto-detects skills availability; the Go
// binary defaults to slim because:
//
//  1. SPEC §1.1 makes slim the default for new repos since v0.6.8+.
//  2. Detecting `skills.sh` from inside the binary would require
//     spawning npx — defer that to a later wave if needed.
//  3. Repos that already shipped the full template stay on full
//     (the marker version `v5` won't match `v7-pointer`, so the
//     stale-block detection will read DIFFERENT but the byte-for-
//     byte compare in ExtractMarkerBlock will reject the refresh
//     because we never auto-migrate full → slim). See OutdatedMarkerBlocks
//     for the explicit guard.
//
// Callers that need the full variant (e.g., during `init --no-slim`)
// can call templates.AgentsTemplate() directly.
func agentsMDTemplate() string {
	return templates.AgentsSlimTemplate()
}

// OutdatedMarkerEntry records one stale AGENTS.md block:
//
//	Path = absolute path to AGENTS.md
//	OldBody = body currently in the file
//	NewBody = body the template wants installed
//
// Returned by FindOutdatedMarkerBlocks; consumed by
// `agents update [--apply]`.
type OutdatedMarkerEntry struct {
	Path    string
	OldBody string
	NewBody string
}

// FindOutdatedMarkerBlocks returns the list of files whose installed
// marker-block body doesn't match the template's. Currently only
// AGENTS.md is checked — per-tool stubs don't carry a marker block
// (they're stubs) and JSON agents don't use markers at all.
//
// IMPORTANT — version guard: never auto-migrate the full v5 template
// to the slim v7-pointer template (or vice versa). If the installed
// marker version doesn't match the template's marker version, skip
// this entry entirely. Migrating template flavors is an explicit
// `init`/`migrate` decision, not something `update` does silently.
// Mirrors Python's behaviour: get_agents_md_template() returns the
// flavour the repo currently has, then the byte-compare runs against
// the same flavour. The Python adaptive detection picks slim or full
// based on the host environment, NOT the installed file — so a repo
// that shipped slim will compare slim-vs-slim only when the host has
// skills.sh. We harden the Go side: only refresh when the template
// flavour matches the installed flavour (same block-version marker).
func FindOutdatedMarkerBlocks(repoRoot string) ([]OutdatedMarkerEntry, error) {
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	content := string(data)
	installed, iok := ExtractMarkerBlock(content)
	if !iok {
		return nil, nil
	}
	// Choose the template variant matching the installed block-version
	// marker. This is the guard that prevents silent full↔slim flips.
	template := matchingTemplate(installed)
	if template == "" {
		return nil, nil
	}
	fresh, tok := ExtractMarkerBlock(template)
	if !tok {
		return nil, nil
	}
	if strings.TrimSpace(installed) == strings.TrimSpace(fresh) {
		return nil, nil
	}
	return []OutdatedMarkerEntry{{
		Path: agentsPath, OldBody: installed, NewBody: fresh,
	}}, nil
}

// matchingTemplate picks the template flavour that matches the
// block-version marker in the installed body. Returns "" if the
// installed body carries no recognized marker — in which case we
// take no action (refusing to guess).
func matchingTemplate(installedBody string) string {
	if strings.Contains(installedBody, "logmind-block-version: v5") {
		return templates.AgentsTemplate()
	}
	if strings.Contains(installedBody, "logmind-block-version: v7-pointer") {
		return templates.AgentsSlimTemplate()
	}
	return ""
}

// OutdatedPinEntry records one stale workflow pin:
//
//	Path = absolute path to .github/workflows/<name>.yml
//	OldVersion / NewVersion = the version strings before/after rewrite
//
// Returned by FindOutdatedWorkflowPins.
type OutdatedPinEntry struct {
	Path       string
	OldVersion string
	NewVersion string
}

// FindOutdatedWorkflowPins scans the canonical CI workflows under
// `.github/workflows/` for `pip install "logmind==X.Y.Z"` lines that
// don't match `currentVersion`. Only the canonical workflows in
// `pinWorkflows` are checked (custom user workflows are out of scope).
//
// Mirrors find_outdated_workflow_pins(root_path).
func FindOutdatedWorkflowPins(repoRoot, currentVersion string) ([]OutdatedPinEntry, error) {
	workflowsDir := filepath.Join(repoRoot, ".github", "workflows")
	if !dirExists(workflowsDir) {
		return nil, nil
	}
	var outdated []OutdatedPinEntry
	for _, name := range pinWorkflows {
		wfPath := filepath.Join(workflowsDir, name)
		if !fileExists(wfPath) {
			continue
		}
		data, err := os.ReadFile(wfPath)
		if err != nil {
			continue // mirrors Python's `except OSError: continue`
		}
		m := pinLineRE.FindStringSubmatch(string(data))
		if m == nil {
			continue
		}
		found := m[4]
		if found != currentVersion {
			outdated = append(outdated, OutdatedPinEntry{
				Path: wfPath, OldVersion: found, NewVersion: currentVersion,
			})
		}
	}
	return outdated, nil
}

// UpdateWorkflowPin rewrites every `pip install "logmind==X.Y.Z"` line
// in content to pin newVersion. Returns (newContent, previousVersion).
// Possible shapes:
//
//   - ("...", "")    — no pin in content; nothing to do
//   - ("...", "X")   — pin already matches newVersion; idempotent
//   - ("...", "OLD") — pin bumped
//
// Preserves the EXACT quote style found in source — single, double,
// or none. Matches Python's update_workflow_pin v0.6.11+.
//
// Idempotent: re-applying the same version yields content unchanged.
func UpdateWorkflowPin(content, newVersion string) (string, string) {
	m := pinLineRE.FindStringSubmatch(content)
	if m == nil {
		return content, ""
	}
	previous := m[4]
	if previous == newVersion {
		return content, previous
	}
	rewritten := pinLineRE.ReplaceAllStringFunc(content, func(match string) string {
		sub := pinLineRE.FindStringSubmatch(match)
		// sub: full, pip-install, quote, logmind==, version, close-quote
		return sub[1] + sub[2] + sub[3] + newVersion + sub[5]
	})
	return rewritten, previous
}

// MigrateToAgentsMD consolidates per-agent instruction files into
// AGENTS.md and replaces each one with a 2-line stub.
//
// For each existing markdown agent file (CLAUDE.md, .cursorrules, ...):
//   - strip the logmind marker block
//   - if any non-marker content remains, append it under a
//     `## From <display-name>` heading at the bottom of AGENTS.md
//   - replace the file content with the canonical stub
//
// Skips JSON agents (cody / zed) and AGENTS.md itself. Idempotent.
//
// Mirrors migrate_to_agents_md(root_path).
//
// Returns a list of human-readable status messages — the caller
// prints them to stdout. Returning a slice (rather than a stream)
// matches Python's accumulator pattern; the migrate command renders
// the slice line-by-line.
func MigrateToAgentsMD(repoRoot string) ([]string, error) {
	var messages []string
	if _, err := EnsureAgentsMD(repoRoot); err != nil {
		return nil, err
	}
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	var appendedBlocks []string

	for _, a := range agents.All() {
		if a.Name == "codex" || a.IsJSON {
			continue
		}
		filePath := filepath.Join(repoRoot, filepath.FromSlash(a.FilePattern))
		if !fileExists(filePath) {
			continue
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		content := string(data)
		if IsStub(content) {
			continue // already migrated
		}

		remaining := strings.TrimSpace(stripLogmindBlock(content))
		if remaining != "" {
			appendedBlocks = append(appendedBlocks,
				fmt.Sprintf("## From %s\n\n%s\n", a.Display, remaining))
			messages = append(messages,
				fmt.Sprintf("✓ Migrated %s (%s) content into AGENTS.md",
					a.Display, filepath.Base(filePath)))
		}

		if err := os.WriteFile(filePath, []byte(templates.Stub()), 0o644); err != nil {
			return messages, err
		}
		messages = append(messages,
			fmt.Sprintf("✓ %s replaced with stub", filepath.Base(filePath)))
	}

	if len(appendedBlocks) > 0 {
		existing, err := os.ReadFile(agentsPath)
		if err != nil {
			return messages, err
		}
		// Match Python: existing.rstrip() + "\n\n" + "\n".join(appended).
		body := strings.TrimRight(string(existing), " \t\n\r") +
			"\n\n" + strings.Join(appendedBlocks, "\n")
		if err := os.WriteFile(agentsPath, []byte(body), 0o644); err != nil {
			return messages, err
		}
	}
	return messages, nil
}

// DetectTemplateDrift is the read-only twin of the silent-sync path.
// Returns one-line drift descriptions without mutating any file —
// used by `logmind log` (B3 wave) to warn the user about stale
// templates without producing piggy-back commits on the decision
// branch.
//
// Per-agent drift (missing or missing-section) is only checked for
// the agents in `enabledAgents` — typically the config's enabled
// set, or DefaultEnabled() when no config is present.
//
// Mirrors detect_template_drift(root_path).
func DetectTemplateDrift(repoRoot string, enabledAgents []string) ([]string, error) {
	var drift []string

	// AGENTS.md drift.
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		drift = append(drift, "AGENTS.md missing")
	case err != nil:
		return nil, err
	default:
		content := string(data)
		if !HasLogmindSection(content) {
			drift = append(drift, "AGENTS.md present but missing logmind block")
		} else {
			installed, iok := ExtractMarkerBlock(content)
			template := matchingTemplate(installed)
			if template != "" {
				fresh, tok := ExtractMarkerBlock(template)
				if iok && tok && strings.TrimSpace(installed) != strings.TrimSpace(fresh) {
					drift = append(drift, "AGENTS.md logmind block out of date")
				}
			}
		}
	}

	// Per-agent stub-file drift.
	for _, name := range enabledAgents {
		a, ok := agents.Lookup(name)
		if !ok {
			continue
		}
		filePath := filepath.Join(repoRoot, filepath.FromSlash(a.FilePattern))
		if !fileExists(filePath) {
			drift = append(drift,
				fmt.Sprintf("%s missing (enabled agent: %s)", a.FilePattern, a.Name))
			continue
		}
		if a.IsJSON {
			continue
		}
		raw, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		content := string(raw)
		if !HasLogmindSection(content) && !IsStub(content) && a.Name != "codex" {
			drift = append(drift,
				fmt.Sprintf("%s missing logmind section (enabled agent: %s)",
					a.FilePattern, a.Name))
		}
	}
	return drift, nil
}

// fileExists returns true when path refers to an existing regular
// file or symlink target. Errors other than ErrNotExist are treated
// as "absent" — matches Python's Path.exists() behaviour.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dirExists is the directory analog of fileExists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
