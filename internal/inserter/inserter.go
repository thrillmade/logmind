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
//     `<!-- logmind-block-version: v8 -->` for the full template,
//     `<!-- logmind-block-version: v9-pointer -->` for slim (the
//     stale-binary-hardening / enforcement wave bumped full v7→v8 and
//     slim v8-pointer→v9-pointer; the Slice-2 branch-summary wave bumped
//     the full template v6→v7; v0.6.16 bumped it v5→v6 and the slim
//     variant v7-pointer→v8-pointer).
//     Drift is
//     detected by comparing the marker bodies stripped (the Python
//     code uses .strip() to be whitespace-tolerant; we mirror that).
//     The planBlockRefresh helper reads the flavour and the generation
//     OFF the installed marker and compares the generation against the
//     bundled one, so an older marker refreshes forward while a newer
//     one — written by a binary ahead of this one — is refused rather
//     than overwritten (SPEC §1.1, issue #267).
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
	"strconv"
	"strings"

	"github.com/thrillmade/logmind/internal/agents"
	"github.com/thrillmade/logmind/internal/atomicio"
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

// ErrNoMarkerBlock is returned by RefreshMarkerBlockFile when the file on
// disk carries no well-formed logmind marker block. It is a REFUSAL, not a
// failure to write: a file with no block is not logmind's to rewrite (SPEC
// §1.1 — "An artifact carrying no marker at all belongs to the user and MUST
// NOT be overwritten"), so the bytes are left exactly as found.
var ErrNoMarkerBlock = errors.New("no logmind marker block")

// RefreshMarkerBlockFile is THE write primitive for the marker block: the
// only exported way to move an installed block forward. It reads the file,
// refuses unless the file itself carries a well-formed block, and writes back
// the surgically-rewritten WHOLE file.
//
// It exists because the two-string signature it replaces could not be called
// wrong in a way the compiler or the runtime would notice. `ReplaceMarkerBlock(
// content, newBody)` took a whole file and a fragment as the same type, and
// returned its first argument unchanged when the markers were missing — so
// handing it a block body where a file belonged produced a fragment, silently,
// which `self-update` then wrote over the user's entire AGENTS.md (#297).
// Owning the read and the write here means a caller never holds a string that
// it could pass to the wrong parameter: there is no parameter to get wrong.
//
// The refusal is the second half of the same guarantee. `replaceMarkerBlock`
// returns its input when the markers are absent, which is the correct pure
// behaviour and a catastrophic write behaviour — "unchanged" is only safe if
// nobody writes it. This function turns that case into ErrNoMarkerBlock and
// writes nothing at all.
//
// Uses os.WriteFile (not atomicio) deliberately: it replaces an os.WriteFile
// call on the same path, and os.WriteFile FOLLOWS a symlink where a
// write-rename would replace it with a regular file — SPEC §1.1 requires a
// refresh to preserve the link. Hardening the durability of this write is a
// separate change from fixing what it writes.
func RefreshMarkerBlockFile(path, newBlockBody string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if _, ok := ExtractMarkerBlock(content); !ok {
		return fmt.Errorf("%s: %w", path, ErrNoMarkerBlock)
	}
	return os.WriteFile(path, []byte(replaceMarkerBlock(content, newBlockBody)), 0o644)
}

// replaceMarkerBlock swaps the body between the existing markers,
// preserving everything else byte-for-byte. Returns content unchanged
// when either marker is absent OR when the markers are out of order
// (end appears before start — malformed input, never a legitimate state).
// Mirrors ExtractMarkerBlock's `end < start` guard so the two primitives
// share the same well-formedness contract.
//
// UNEXPORTED on purpose (#297). "Returns its input unchanged" is a safe
// contract for a pure function and a data-loss contract for anything that
// writes the result, so the function that can produce a fragment is not
// reachable from outside this package; RefreshMarkerBlockFile is, and it
// checks first. Callers outside the package have no way to express the
// mistake because they have no way to name this function.
//
// This is the SURGICAL REWRITE primitive. Marker-block round-trip
// invariant (proved in the package test):
//
//	old, _ := ExtractMarkerBlock(c0)
//	c1 := replaceMarkerBlock(c0, "FOO")
//	c2 := replaceMarkerBlock(c1, old)
//	c0 == c2 byte-for-byte
func replaceMarkerBlock(content, newBlockBody string) string {
	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)
	if start == -1 || end == -1 || end < start {
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
	// filePath is an EXISTING file (read successfully above) — write
	// atomically so a crash mid-write can't leave the user's AGENTS.md/etc.
	// truncated or partial.
	if err := atomicio.WriteFile(filePath, []byte(out), 0o644); err != nil {
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
//   - Missing → write the canonical (slim) template.
//   - Exists without markers → insert the logmind block in-place.
//   - Exists with markers but stale body → refresh the body IN PLACE,
//     preserving the installed flavour (a repo that shipped the full
//     block refreshes into the current full body, a slim repo into the
//     current slim body — never a silent full↔slim flip).
//   - Exists with markers and matching body → no-op (return "").
//   - Exists with a version id this binary can't move forward (newer
//     than the one it ships, or unreadable) → REFUSED: the file is left
//     byte-for-byte alone and the refusal is returned for the caller to
//     report (#267).
//
// Returns a status string when a write happened, or "" for no-op, plus a
// non-nil *AgentsBlockRefusal when the block was deliberately left alone.
// The status strings match Python's three return values verbatim.
func EnsureAgentsMD(repoRoot string) (string, *AgentsBlockRefusal, error) {
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")

	data, err := os.ReadFile(agentsPath)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(agentsPath, []byte(agentsMDTemplate()), 0o644); err != nil {
			return "", nil, err
		}
		return "Created AGENTS.md (canonical agent instructions)", nil, nil
	}
	if err != nil {
		return "", nil, err
	}

	content := string(data)
	if !HasLogmindSection(content) {
		if _, err := InsertLogmindSection(agentsPath); err != nil {
			return "", nil, err
		}
		return "Added logmind section to existing AGENTS.md", nil, nil
	}

	installedBlock, iok := ExtractMarkerBlock(content)
	if !iok {
		return "", nil, nil
	}
	// Refresh AGAINST THE INSTALLED BLOCK'S OWN ID — flavour and
	// generation both — never against this binary's default. planBlockRefresh
	// is the single classifier FindOutdatedMarkerBlocks also uses, so the two
	// cannot drift apart on what an unreadable or newer id means; they used
	// to, and EnsureAgentsMD's half of that disagreement silently downgraded
	// the repo's block (#267).
	plan := planBlockRefresh(installedBlock)
	if plan.Template == "" {
		plan.Refusal.Path = agentsPath
		return "", plan.Refusal, nil
	}
	templateBlock, tok := ExtractMarkerBlock(plan.Template)
	if tok && strings.TrimSpace(installedBlock) != strings.TrimSpace(templateBlock) {
		// Routed through the one write primitive rather than doing its own
		// read/replace/write — EnsureAgentsMD is the SINGLE refresher of this
		// path (SPEC §1.1: "Exactly one automation owns any generated or
		// copied path. Two refreshers MUST NOT write the same path"), and it
		// gets the markers-required refusal for free.
		if err := RefreshMarkerBlockFile(agentsPath, templateBlock); err != nil {
			return "", nil, err
		}
		return "Refreshed AGENTS.md logmind block to current template", nil, nil
	}
	return "", nil, nil
}

// agentsMDTemplate returns the canonical AGENTS.md body. Defaults to
// slim per SPEC §1.1 (the v9-pointer variant — defers to skills.sh;
// the stale-binary-hardening / enforcement wave bumped this from
// v8-pointer, which itself bumped it from v7-pointer).
// The Python implementation auto-detects skills availability; the Go
// binary defaults to slim because:
//
//  1. SPEC §1.1 makes slim the default for new repos since v0.6.8+.
//  2. Detecting `skills.sh` from inside the binary would require
//     spawning npx — defer that to a later wave if needed.
//  3. Repos that already shipped the full template stay on full: the
//     refresh paths (EnsureAgentsMD and FindOutdatedMarkerBlocks) both
//     select the template flavour matching the installed block-version
//     marker via planBlockRefresh, so an older full block refreshes into
//     the current full body rather than silently flipping to slim.
//     See planBlockRefresh for the flavour + ordering guards.
//
// Callers that need the full variant (e.g., during `init --no-slim`)
// can call templates.AgentsTemplate() directly.
func agentsMDTemplate() string {
	return templates.AgentsSlimTemplate()
}

// OutdatedMarkerEntry records one stale AGENTS.md block:
//
//	Path = absolute path to AGENTS.md
//	NewBody = body the template wants installed
//
// Returned by FindOutdatedMarkerBlocks; consumed by
// `agents update [--apply]`, which feeds Path+NewBody straight into
// RefreshMarkerBlockFile.
//
// There is deliberately NO OldBody field (#297). It carried the block body
// currently on disk, had no consumer but one — `self-update` passed it as the
// WHOLE-FILE argument of ReplaceMarkerBlock and wrote the fragment that came
// back over the user's entire AGENTS.md — and its presence is what made that
// call look plausible: a struct that hands you a path and a body next to each
// other invites writing the body to the path. Detection needs only "which file
// is stale, and what should be in it"; anything that needs the current bytes
// reads the file.
type OutdatedMarkerEntry struct {
	Path    string
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
//
// Returns the refusal alongside the entries when the installed id is one
// this binary can't move forward — skipping was always the right ACTION
// here, but doing it silently let `agents update` report a block that is
// ahead of the binary as "current" (#267).
func FindOutdatedMarkerBlocks(repoRoot string) ([]OutdatedMarkerEntry, *AgentsBlockRefusal, error) {
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	content := string(data)
	installed, iok := ExtractMarkerBlock(content)
	if !iok {
		return nil, nil, nil
	}
	// Choose the template variant matching the installed block-version
	// marker. This is the guard that prevents silent full↔slim flips, and
	// (since #267) silent downgrades of a newer block.
	plan := planBlockRefresh(installed)
	if plan.Template == "" {
		plan.Refusal.Path = agentsPath
		return nil, plan.Refusal, nil
	}
	fresh, tok := ExtractMarkerBlock(plan.Template)
	if !tok {
		return nil, nil, nil
	}
	if strings.TrimSpace(installed) == strings.TrimSpace(fresh) {
		return nil, nil, nil
	}
	return []OutdatedMarkerEntry{{
		Path: agentsPath, NewBody: fresh,
	}}, nil, nil
}

// blockVersionRE captures the version token out of an AGENTS.md block
// marker: `<!-- logmind-block-version: v9-pointer -->` → "v9-pointer".
// Mirrors internal/doctor's logmindBlockVersionRe — doctor PROBES the
// marker, inserter WRITES against it, so the pattern lives once per side
// of that boundary.
var blockVersionRE = regexp.MustCompile(`<!--\s*logmind-block-version:\s*(\S+)\s*-->`)

// pointerSuffix is the flavour tag that distinguishes the slim
// ("pointer") block from the full one. SPEC §1.1 forbids a silent
// full↔slim flip, so this suffix — not a hardcoded list of generations —
// is what decides which template body a block may refresh into.
const pointerSuffix = "-pointer"

// AgentsBlockRefusal records a REFUSED AGENTS.md marker-block refresh
// (#267): the installed block carries a version id this binary cannot
// move forward, so the file was left byte-for-byte alone.
//
// Returned by EnsureAgentsMD / FindOutdatedMarkerBlocks / MigrateToAgentsMD
// and reported by every caller — SPEC §3.4's rule for the analogous
// fail-open case ("Failing open MUST NOT be silent... MUST say so on
// stderr, naming what it looked for and what it found") applies here too:
// a repo whose block this binary declines to touch has to be told.
type AgentsBlockRefusal struct {
	Path      string // absolute path to AGENTS.md
	Installed string // version token found in the block, e.g. "v10-pointer"; "" when the block carries no marker
	Bundled   string // what this binary ships: the same-flavour marker, or both markers when the flavour is unreadable
	Ahead     bool   // Installed parsed and orders AFTER Bundled — a downgrade, as opposed to an unreadable id
}

// blockPlan is the classification of one installed marker-block body:
// the template flavour it may be refreshed into, or the refusal that says
// why it may not be touched. Exactly one of the two fields is set.
type blockPlan struct {
	Template string              // flavour-matched template body; "" when the block must be left alone
	Refusal  *AgentsBlockRefusal // non-nil iff Template == ""; Path is filled in by the caller that read the file
}

// planBlockRefresh decides what may happen to an installed AGENTS.md
// marker block. Both refresh paths (EnsureAgentsMD and
// FindOutdatedMarkerBlocks) route through it, so they cannot disagree
// about what an unreadable id means — the disagreement between them WAS
// issue #267.
//
// Two guards, both load-bearing:
//
//  1. FLAVOUR (SPEC §1.1). The `-pointer` suffix separates the slim block
//     from the full one; a repo that shipped full refreshes into the
//     current full body, a slim repo into the current slim body, never a
//     silent flip. The suffix is READ OFF the installed marker rather
//     than enumerated, so a generation this binary has never heard of
//     still classifies into the right flavour instead of falling into a
//     default.
//
//  2. ORDER (SPEC §1.1: "a tool MUST NOT replace an installed block with
//     an older id"). The generation compares NUMERICALLY against the
//     marker this binary ships for that flavour, and a block that orders
//     AFTER it is refused. This used to be membership in a hardcoded
//     {v5,v6,v7,v8,v7-pointer,v8-pointer,v9-pointer} set, which made
//     every FUTURE generation "unrecognised" — and EnsureAgentsMD read
//     unrecognised as "install the slim default", silently downgrading
//     any repo a newer binary had moved forward (#267). Extending that
//     set to v10, v11, ... would re-break at the next bump; ordering
//     against the bundled marker cannot go stale, because the next
//     generation is newer by arithmetic rather than by enumeration.
//     Mixed binary versions is not an edge case during a fleet migration
//     (#257) — it is the definition of one.
//
// An UNREADABLE id — absent, not `v<digits>`, or carrying a flavour
// suffix this binary doesn't know — is refused as well: there is no
// flavour to preserve and no generation to compare, and guessing "it's
// probably slim" is precisely what #267 was.
func planBlockRefresh(installedBody string) blockPlan {
	found := ""
	if m := blockVersionRE.FindStringSubmatch(installedBody); len(m) >= 2 {
		found = m[1]
	}
	gen, suffix, ok := parseBlockMarker(found)
	var template string
	if ok {
		switch suffix {
		case "":
			template = templates.AgentsTemplate()
		case pointerSuffix:
			template = templates.AgentsSlimTemplate()
		}
	}
	if template == "" {
		return blockPlan{Refusal: &AgentsBlockRefusal{
			Installed: found,
			Bundled:   bundledBlockMarkerList(),
		}}
	}
	bundled := bundledBlockMarker(template)
	bundledGen, bok := ParseMarkerGeneration(bundled)
	if !bok {
		// The template WE ship carries no readable marker — a build-time
		// defect, not a repo state. Refuse rather than compare against
		// nothing; the alternative is downgrading on a typo in our own body.
		return blockPlan{Refusal: &AgentsBlockRefusal{
			Installed: found,
			Bundled:   bundledBlockMarkerList(),
		}}
	}
	if gen > bundledGen {
		return blockPlan{Refusal: &AgentsBlockRefusal{
			Installed: found,
			Bundled:   bundled,
			Ahead:     true,
		}}
	}
	return blockPlan{Template: template}
}

// bundledBlockMarker returns the version token carried by one of OUR
// template bodies — the "what this binary knows" side of every compare.
// Read from the body rather than restated as a constant, so a template
// bump moves the guard with it.
func bundledBlockMarker(templateBody string) string {
	if m := blockVersionRE.FindStringSubmatch(templateBody); len(m) >= 2 {
		return m[1]
	}
	return ""
}

// bundledBlockMarkerList names both bundled markers for the refusal
// message when the installed id is unreadable and there's no single
// flavour to compare against.
func bundledBlockMarkerList() string {
	return bundledBlockMarker(templates.AgentsTemplate()) + " (full) or " +
		bundledBlockMarker(templates.AgentsSlimTemplate()) + " (slim)"
}

// ParseMarkerGeneration extracts the ORDERING key from a logmind version
// marker token: "v11" → 11, "v9-pointer" → 9. Only the numeric generation
// orders — a string compare gets this exactly backwards ("v11" < "v4"
// lexically), which is the trap #286 fell into for workflow templates and
// #267 sidestepped entirely by not comparing at all.
//
// Shared with internal/cli's workflow-template guard so the two artifacts
// order by the same rule; the block path additionally needs the flavour
// suffix, which parseBlockMarker returns.
//
// Returns ok=false for anything that isn't a leading "v" followed by at
// least one digit.
func ParseMarkerGeneration(marker string) (int, bool) {
	n, _, ok := parseBlockMarker(marker)
	return n, ok
}

// parseBlockMarker splits a marker token into its generation and its
// flavour suffix: "v9-pointer" → (9, "-pointer", true), "v8" → (8, "",
// true), "vNOPE" → (0, "", false).
func parseBlockMarker(marker string) (int, string, bool) {
	s := strings.TrimSpace(marker)
	if !strings.HasPrefix(s, "v") {
		return 0, "", false
	}
	s = strings.TrimPrefix(s, "v")
	suffix := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		suffix, s = s[i:], s[:i]
	}
	if s == "" {
		return 0, "", false
	}
	// Reject anything Atoi would tolerate but a marker never contains
	// (a sign, whitespace) — only bare digits order.
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, "", false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, "", false
	}
	return n, suffix, true
}

// templateMarkerRE matches the `# logmind-template-version: vN` line an
// installed workflow template carries. Anchored at `^` and applied to LINE 1
// ONLY — see ExtractTemplateMarker for why that anchor is the ownership test
// rather than an implementation detail.
var templateMarkerRE = regexp.MustCompile(`^# logmind-template-version:\s*(\S+)`)

// MarkerOwnership answers the only question a write path may ask about an
// installed artifact: is this file logmind's to overwrite?
type MarkerOwnership int

const (
	// MarkerOwned — the logmind marker is on line 1. logmind installed this
	// file and MAY refresh it (subject to the ordering guard, #286).
	MarkerOwned MarkerOwnership = iota
	// MarkerAbsent — no logmind marker anywhere. SPEC §1.1: "An artifact
	// carrying no marker at all belongs to the user and MUST NOT be
	// overwritten."
	MarkerAbsent
	// MarkerDisplaced — a marker exists, but not on line 1. Neither clearly
	// ours nor clearly the user's, so it is refused and reported rather than
	// guessed at — the same "unknown means refuse, never guess" rule #267 and
	// #286 landed on.
	MarkerDisplaced
)

// TemplateMarker is the result of reading an installed artifact's
// template-version marker.
type TemplateMarker struct {
	Ownership MarkerOwnership
	Version   string // the marker token, e.g. "v5"; "" when MarkerAbsent
	Line      int    // 1-based line the marker was found on; 0 when MarkerAbsent
}

// Writable reports whether a refresh path may overwrite this artifact.
// Only MarkerOwned qualifies.
func (m TemplateMarker) Writable() bool { return m.Ownership == MarkerOwned }

// ExtractTemplateMarker is the SINGLE OWNER of "does this file carry a
// logmind template-version marker, and is it ours". Every reader and every
// writer routes through it.
//
// It replaces two extractors that disagreed (#299): doctor matched an
// anchored regex against LINE 1 ONLY, while init's extractTemplateVersion
// prefix-matched EVERY line. A file whose marker sat on line 2 was therefore
// "markerless" to the component that REPORTED and versioned-and-stale to the
// component that WROTE — so `doctor --fix` overwrote a file it had just told
// the user was theirs. Which extractor was right did not matter to that bug;
// that there were two of them did.
//
// FIRST LINE ONLY is the surviving semantics, and not merely because it is
// the stricter of the two:
//
//   - It matches what logmind WRITES. Every bundled template in
//     internal/templates/github carries the marker as its first byte, so
//     "marker on line 1" is exactly the set of files logmind produced. An
//     ownership test should recognise our own output, not a superset of it.
//   - Any-line makes ownership a substring search, and a substring search
//     over a user's file CLAIMS it. A workflow that quotes the marker in a
//     heredoc, a comment, or an echoed setup step would be adopted — and
//     then overwritten — because it mentioned us. Permissiveness on the read
//     side is not neutral when the write side is destructive.
//   - It leaves the user a deliberate way to take a file back: move the
//     marker, or drop it. Under any-line, disowning a file requires finding
//     and deleting every occurrence.
//
// The cost is that a marker displaced by a hand-added header stops being
// recognised. That cost is paid explicitly rather than silently: displacement
// is its own state (MarkerDisplaced), every write path refuses it, and the
// refusal is reported — never collapsed into "markerless" and acted on.
func ExtractTemplateMarker(text string) TemplateMarker {
	for i, line := range strings.Split(text, "\n") {
		m := templateMarkerRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if i == 0 {
			return TemplateMarker{Ownership: MarkerOwned, Version: m[1], Line: 1}
		}
		return TemplateMarker{Ownership: MarkerDisplaced, Version: m[1], Line: i + 1}
	}
	return TemplateMarker{Ownership: MarkerAbsent}
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
// the slice line-by-line. The second return is EnsureAgentsMD's refusal
// (#267): migrate still consolidates the per-agent files, but the caller
// must say that AGENTS.md's own block was left alone.
func MigrateToAgentsMD(repoRoot string) ([]string, *AgentsBlockRefusal, error) {
	var messages []string
	_, declined, err := EnsureAgentsMD(repoRoot)
	if err != nil {
		return nil, declined, err
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
			return messages, declined, err
		}
		messages = append(messages,
			fmt.Sprintf("✓ %s replaced with stub", filepath.Base(filePath)))
	}

	if len(appendedBlocks) > 0 {
		existing, err := os.ReadFile(agentsPath)
		if err != nil {
			return messages, declined, err
		}
		// Match Python: existing.rstrip() + "\n\n" + "\n".join(appended).
		body := strings.TrimRight(string(existing), " \t\n\r") +
			"\n\n" + strings.Join(appendedBlocks, "\n")
		if err := os.WriteFile(agentsPath, []byte(body), 0o644); err != nil {
			return messages, declined, err
		}
	}
	return messages, declined, nil
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
