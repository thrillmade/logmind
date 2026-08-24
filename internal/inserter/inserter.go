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
//     `<!-- logmind-block-version: v9 -->` for the full template,
//     `<!-- logmind-block-version: v10-pointer -->` for slim (the §3.2
//     layout-collapse wave bumped full v8→v9 AND slim
//     v9-pointer→v10-pointer — both bodies dropped docs/decisions.md from
//     required reading, so both markers had to move; the
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
	"encoding/json"
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

// agentsMDName is the one path EnsureAgentsMD owns (SPEC §5.2: "Exactly one
// automation owns any generated or copied path"). It is also row 11 of SPEC
// §1.2's per-tool table — the codex entry — which is why the per-tool writer
// below has to recognise it by name rather than treating it as one more stub.
const agentsMDName = "AGENTS.md"

// logmindOwner is the component name logmind writes into its own markers.
// The redirect files of SPEC §1.2 are shared with other components (skdd
// today), so "whose marker is this" is a name compare, not a boolean.
const logmindOwner = "logmind"

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

// IsStub reports whether content MENTIONS logmind's stub marker at all.
// Stubs carry the `<!-- logmind-stub:` marker so they're distinguishable
// from full files that happen to mention AGENTS.md.
//
// This is NOT the ownership test, and the two must not be confused — that
// confusion is #299. IsStub answers "has this file been stubbed", for
// `agents list`'s status column and for `agents migrate`'s
// already-consolidated skip; ReadRedirectOwner answers "may logmind WRITE
// this file", and is line-1-only.
//
// The looser rule is safe HERE and only here, because both consumers fail
// safe when it over-matches: the status column says "configured" about a file
// nobody will write, and migrate SKIPS it. An over-inclusive answer on the
// write side is what destroys a file, which is why that side does not use
// this function.
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
// §5.2 — "An artifact carrying no marker at all belongs to the user and MUST
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
// Routed through atomicio.WriteFile rather than a bare os.WriteFile: this
// was the one write in the package that still used the truncate+write
// two-syscall form, on the file SPEC §1.1 names as the artifact a repo
// reads project instructions from. (An earlier version of this comment
// claimed SPEC §1.1 required preserving a symlink here; it does not — the
// symlink-preservation rule is §5.2's, scoped to catalog-subscribed items,
// not to AGENTS.md.) atomicio.WriteFile also refuses to write through a
// symlink at path (atomicio.RefuseSymlink, #300) — a bare os.WriteFile
// FOLLOWS a symlink, which for a dangling one at agentsPath means creating
// a file wherever it points, possibly outside the repo.
func RefreshMarkerBlockFile(path, newBlockBody string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if _, ok := ExtractMarkerBlock(content); !ok {
		return fmt.Errorf("%s: %w", path, ErrNoMarkerBlock)
	}
	return atomicio.WriteFile(path, []byte(replaceMarkerBlock(content, newBlockBody)), 0o644)
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

// RedirectWrite is what CreateAgentFile did, for a caller that has to print a
// receipt. Path is "" when nothing was written — which now includes the
// ordinary idempotent case, an entry already carrying the current body.
//
// Created distinguishes the two writes that are not the same event: a file
// this run brought into existence, and one that was already there and had its
// logmind entry refreshed in place. Reporting both as "✓ Created" is the
// cheapest kind of lie for a receipt to tell, and it is the line that made
// #336 invisible — the user saw "✓ Created CLAUDE.md" over the file the run
// had just destroyed, and had no reason to look.
type RedirectWrite struct {
	Path    string
	Created bool
}

// CreateAgentFile installs logmind's entry in a per-agent instruction file —
// the canonical stub for markdown agents, the JSON body for cody/zed, the
// AGENTS.md template for codex — MERGING it into whatever is already there.
//
// It is the single write primitive for SPEC §1.2's redirect files, and it owns
// the read as well as the write for the reason RefreshMarkerBlockFile does: a
// caller that cannot hold the bytes cannot decide about them, so there is no
// way to express "write it without checking". Pre-#336 this function took an
// agent name and wrote the bundled body over the path, unconditionally, and
// the whole of SPEC:1101 was unrepresentable in its signature — a repository's
// hand-written CLAUDE.md was destroyed by `logmind init` on exit 0, and so was
// the `@AGENTS.md` import line that is how Claude Code loads AGENTS.md at all.
//
// Returns (write, nil, nil) when bytes were written, ("", refusal, nil) when
// the file on disk is not logmind's — the caller MUST report the refusal — and
// the zero write for an unknown agent name (matching Python's None return), an
// AGENTS.md whose refresher is EnsureAgentsMD, or an entry already current.
//
// Mirrors create_agent_file(agent_name, root_path), plus the ownership rule.
func CreateAgentFile(agentName, repoRoot string) (RedirectWrite, *RedirectRefusal, error) {
	a, ok := agents.Lookup(agentName)
	if !ok {
		return RedirectWrite{}, nil, nil
	}
	filePath := filepath.Join(repoRoot, filepath.FromSlash(a.FilePattern))

	// Refuse a symlink BEFORE the read, not just before the write. The
	// ownership verdict below is made from the file's own bytes and
	// os.ReadFile resolves the final component, so a link pointing outside the
	// repository would have some other file's content answer "is CLAUDE.md
	// logmind's?" — and whatever the answer, it was answered about the wrong
	// file. Same ordering, for the same reason, as the workflow loop in
	// internal/cli/init.go.
	if err := atomicio.RefuseSymlink(filePath); err != nil {
		return RedirectWrite{}, nil, err
	}
	// os.ReadFile follows a symlink, so a DANGLING one at filePath reports
	// fs.ErrNotExist exactly as an absent file does; the RefuseSymlink above
	// has already declined it, and atomicio.WriteFile below closes the window
	// between the two syscalls.
	existing, err := os.ReadFile(filePath)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return RedirectWrite{}, nil, err
	}

	plan := planRedirectWrite(a, string(existing), exists)
	if plan.Refusal != nil {
		return RedirectWrite{}, plan.Refusal, nil
	}
	if plan.Body == "" {
		return RedirectWrite{}, nil, nil
	}
	// atomicio.WriteFile makes its own parent directory and refuses
	// (atomicio.RefuseSymlink, #300) rather than silently writing through a
	// symlink at filePath.
	if err := atomicio.WriteFile(filePath, []byte(plan.Body), 0o644); err != nil {
		return RedirectWrite{}, nil, err
	}
	return RedirectWrite{Path: filePath, Created: !exists}, nil, nil
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

// jsonAgentBody is the shared JSON body written to .sourcegraph/cody.json
// and .zed/settings.json (see internal/agents registry). Both tools take
// the same body.
//
// This is NOT the verbatim Python string any more, and deliberately so.
// Python's version predates SPEC §3.2 and pointed both tools at
// docs/decisions.md as "recent decisions" — the one file the branch-aware
// write path never writes. An agent that loaded this config read a legacy
// file that is empty in every repo initialised after §3.2, and never saw
// docs/decisions-branches/ or docs/timeline.md at all. Byte-parity with a
// pre-§3.2 Python release is not worth shipping two AI tools a stale map.
//
// Ordering mirrors the canonical reading order in skill/SKILL.md and
// AGENTS.md.template: timeline first (the source-derived union of every
// branch), then the per-branch detail, then the tree. docs/decisions.md
// stays LAST and is still listed — it is read-where-it-exists legacy, and a
// repo that predates §3.2 keeps its history findable.
//
// docs/decisions-branches/ is named as a directory: the branch file's name
// is not known when this config is written, and both tools accept a
// directory in this list. Entries that do not exist yet are inert in both.
func jsonAgentBody() string {
	return `{
  "logmind": {
    "enabled": true,
    "description": "This project uses logmind for decision tracking. Decisions live in docs/decisions-branches/<branch>.md — one file per branch, the default branch included (main.md). Start from docs/timeline.md, the source-derived union of every branch.",
    "context_files": [
      "docs/timeline.md",
      "docs/timeline-archive.md",
      "docs/decisions-branches/",
      "docs/file-structure.md",
      "docs/decisions.md"
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
	agentsPath := filepath.Join(repoRoot, agentsMDName)

	data, err := os.ReadFile(agentsPath)
	if errors.Is(err, fs.ErrNotExist) {
		// os.ReadFile follows a symlink, so a DANGLING symlink at agentsPath
		// (pointing at a target that doesn't exist) also returns
		// fs.ErrNotExist here — the file looks "absent" when it is really a
		// link somewhere outside the repo. A bare os.WriteFile would then
		// follow that same link and create the write target wherever it
		// points. atomicio.WriteFile refuses instead (atomicio.RefuseSymlink,
		// #300), the same as every other AGENTS.md write in this file.
		if err := atomicio.WriteFile(agentsPath, []byte(agentsMDTemplate()), 0o644); err != nil {
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
		// path (SPEC §5.2: "Exactly one automation owns any generated or
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
// slim per SPEC §1.1 (the v10-pointer variant — defers to skills.sh; the
// §3.2 layout-collapse wave bumped this from v9-pointer, which the
// stale-binary-hardening / enforcement wave bumped from v8-pointer, which
// itself bumped it from v7-pointer).
//
// This default is why the slim marker is the one that MATTERS: a body change
// shipped under an unchanged slim marker reaches every repo scaffolded
// without an explicit flavour request, and `logmind doctor` calls all of them
// current.
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
	agentsPath := filepath.Join(repoRoot, agentsMDName)
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
// marker: `<!-- logmind-block-version: v10-pointer -->` → "v10-pointer".
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
//     any repo a newer binary had moved forward (#267). That set is
//     already one bump out of date (slim ships v10-pointer now), which is
//     the point: extending it to v10, v11, ... would re-break at the next
//     bump; ordering
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
	// MarkerAbsent — no logmind marker anywhere. SPEC §5.2: "An artifact
	// carrying no marker at all belongs to the user and MUST NOT be
	// overwritten."
	MarkerAbsent
	// MarkerDisplaced — a marker exists, but not on line 1. Neither clearly
	// ours nor clearly the user's, so it is refused and reported rather than
	// guessed at — the same "unknown means refuse, never guess" rule #267 and
	// #286 landed on.
	MarkerDisplaced
	// MarkerForeign — the marker on line 1 names a DIFFERENT component (#336,
	// protocol#77: "present, carrying another component's marker → nobody").
	// Only the shared artifacts of SPEC §1.2 can be in this state; the
	// workflow templates carry no other component's marker, so
	// ExtractTemplateMarker never returns it.
	MarkerForeign
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

// redirectMarkerRE matches a component's OWNER MARKER at the start of a line
// in a per-tool redirect file, capturing the component that claimed it and the
// KIND of claim:
//
//	<!-- logmind-stub: ... -->         → "logmind", "stub"
//	<!-- skdd-stub: ... -->            → "skdd",    "stub"
//	<!-- clud-bug-start -->            → "clud-bug","start"
//	<!-- clud-bug-block-version: … --> → "clud-bug","block-version"
//
// The kind matters because only "stub" names a SPAN this package may rewrite.
// A `<!-- logmind-start -->` in a CLAUDE.md is logmind's too, but it is the
// legacy in-place section — a different artifact with a different writer — and
// splicing a stub over its opening line orphans its `-end` and drops what is
// between them.
//
// The kinds are enumerated rather than left open (`<!-- (\S+) -->`) so an
// ordinary HTML comment a person wrote is not mistaken for a component's
// claim. A marker shape this does not know falls through to MarkerAbsent,
// which refuses the write anyway — the rule degrades toward leaving the file
// alone, never toward claiming it.
var redirectMarkerRE = regexp.MustCompile(
	`^<!--\s*([A-Za-z0-9][A-Za-z0-9._-]*?)-(stub|start|end|block-version)\b`)

// markerKindStub is the only claim kind that names a rewritable span.
const markerKindStub = "stub"

// fenceRE matches a markdown code-fence delimiter. A marker INSIDE a fence is
// a quotation, not a claim: `docs/ai-agent-files.md` shows the stub verbatim,
// and a repository is entitled to do the same in its own CLAUDE.md without
// handing logmind a write span in the middle of its example.
var fenceRE = regexp.MustCompile("^\\s*(?:```|~~~)")

// RedirectOwner is the answer to the only question a write path may ask about
// an existing per-tool redirect file (SPEC §1.2's table): whose is it?
//
// Marker is what ownership was decided FROM, phrased for a human — the three
// file forms in that table prove ownership three different ways, and a refusal
// that cannot name what it looked for is not a report (SPEC §3.4).
type RedirectOwner struct {
	Ownership MarkerOwnership
	Owner     string // component named by the marker, e.g. "skdd"; "" when there is none
	Line      int    // 1-based line the marker sat on; 0 when there is none
	Marker    string // what logmind looks for here, e.g. "a `<!-- logmind-stub: -->` line"
}

// Writable reports whether logmind may write ITS OWN ENTRY in this redirect
// file. Only MarkerOwned qualifies — the SAME predicate, spelled the same way,
// that TemplateMarker.Writable gives the workflow path. protocol#77 and SPEC
// §5.2 are one rule over two artifacts, so they get one predicate: another
// component's marker and no marker at all are both "not ours", and a caller
// cannot accidentally handle one and forget the other.
//
// Writable is NOT permission to write the whole file. SPEC:1101's first
// sentence — "An installer MUST merge rather than replace: it writes only the
// entry it owns and leaves every other entry, including one the user wrote by
// hand, exactly as it found it" — binds whether or not logmind's marker is
// there. A file carrying logmind's marker is not thereby logmind's file:
// `protocol`'s CLAUDE.md carries four markers and no component owns it. What
// this predicate gates is a splice, planned by planRedirectWrite.
func (r RedirectOwner) Writable() bool { return r.Ownership == MarkerOwned }

// ReadRedirectOwner is the SINGLE OWNER of "whose is this per-tool redirect
// file", for every one of the eleven rows in SPEC §1.2's table.
//
// It exists because `logmind init` used to answer the question by not asking
// it: CreateAgentFile rendered the bundled body straight over whatever was
// there, so a hand-written CLAUDE.md was destroyed on exit 0 with nothing on
// stderr (#336). protocol#77's ruling is the contract now —
//
//	absent                     → whichever component is installing, with its own marker
//	another component's marker → nobody; leave it, and say so on stderr
//	no marker at all           → nobody; SPEC:1101, it belongs to the user
//
// — and this is where rows 2 and 3 are decided, once, so the two commands that
// write these files cannot answer differently.
//
// WHY NOT "MARKER ON LINE 1", the rule ExtractTemplateMarker uses for
// workflows. It was the first thing tried here and it is wrong for this
// artifact, measurably: the CLAUDE.md in `agent-skills`, `reporulez`,
// `clud-bug` and this repository all open with Claude Code's `@AGENTS.md`
// import directive, so logmind's marker sits on line 3 in four of the five
// repositories that carry one. Line-1-only reads every one of them as
// displaced and refuses forever. The workflow rule can be strict because a
// workflow is a whole file logmind renders; a redirect file is SHARED, and a
// component's entry does not get to be first just because it installed first.
//
// What replaces it is that a match no longer authorises a whole-file write. A
// marker anywhere identifies the SPAN logmind owns (see logmindEntrySpan), and
// everything outside that span is left exactly as found — so the read side can
// afford to be permissive because the write side stopped being destructive.
// Two narrower guards remain, both of which a naive substring search fails:
// the marker must START a line (a mention inside a sentence is prose), and it
// must not be inside a code fence (a mention inside ``` is a quotation).
//
// Three forms, three proofs of ownership, one verdict type:
//
//   - MARKDOWN STUB (nine rows) — the `<!-- logmind-stub: -->` line.
//   - JSON (cody, zed) — JSON has no comments, so the marker is a KEY; see
//     jsonRedirectOwner.
//   - AGENTS.md (codex) — the logmind marker block, which is what
//     EnsureAgentsMD reads and writes.
func ReadRedirectOwner(a agents.Agent, content string) RedirectOwner {
	if a.IsJSON {
		return jsonRedirectOwner(content)
	}
	if a.FilePattern == agentsMDName {
		return agentsMDRedirectOwner(content)
	}
	const proof = "a `<!-- logmind-stub: -->` line"
	var found *RedirectOwner
	forEachMarkerLine(strings.Split(content, "\n"), func(i int, owner, _ string) bool {
		if owner == logmindOwner {
			found = &RedirectOwner{Ownership: MarkerOwned, Owner: logmindOwner, Line: i + 1, Marker: proof}
			return false // logmind's own claim ends the search; a foreign one does not
		}
		if found == nil {
			found = &RedirectOwner{Ownership: MarkerForeign, Owner: owner, Line: i + 1, Marker: proof}
		}
		return true // keep looking: logmind's entry may sit below another component's
	})
	if found != nil {
		return *found
	}
	return RedirectOwner{Ownership: MarkerAbsent, Marker: proof}
}

// forEachMarkerLine calls fn with the 0-based index, component name and claim
// kind of every component marker that STARTS a line and is not inside a code
// fence, stopping when fn returns false. The fence state is tracked rather
// than the fences merely skipped, so an opening ``` protects everything up to
// its close.
func forEachMarkerLine(lines []string, fn func(i int, owner, kind string) bool) {
	inFence := false
	for i, line := range lines {
		if fenceRE.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := redirectMarkerRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if !fn(i, m[1], m[2]) {
			return
		}
	}
}

// logmindEntrySpan returns the half-open LINE range [start, end) that
// logmind's entry occupies, and whether there is one at all.
//
// THE SPAN IS THE WHOLE FIX. Everything outside it survives byte-for-byte —
// the `@AGENTS.md` import above it, another component's block below it, a
// paragraph the user wrote.
//
// The entry is the marker line plus the lines immediately under it, ending at
// the first BLANK line, the first line that opens another component's HTML
// comment, or end of file. That is markdown's own paragraph boundary, and it
// is exactly the shape logmind writes (a marker plus two body lines, then
// nothing or a blank). Measured against the five CLAUDE.md files in this org:
// it selects lines 1-3 of `protocol`'s (whose blank line 4 separates
// clud-bug's block) and lines 3-5 of `agent-skills`' (whose lines 1-2 are the
// import and its blank), and re-splicing the current stub into either
// reproduces the file byte-for-byte.
//
// The alternative — bracketing the entry with an end marker, the way
// AGENTS.md's block is — was rejected: every stub already installed in the
// fleet lacks one, so the boundary would still have to be inferred for exactly
// the files this bug is about, and protocol#77 settled these files as "one
// line, one owner, one marker". The cost of the paragraph rule is that prose
// glued to the marker with no blank line between is inside the entry and is
// replaced; that is stated here and pinned by a test, rather than discovered.
func logmindEntrySpan(lines []string) (int, int, bool) {
	start := -1
	forEachMarkerLine(lines, func(i int, owner, kind string) bool {
		// STUB ONLY. `<!-- logmind-start -->` is logmind's as well, but it
		// opens the legacy in-place section, and treating its first line as
		// the head of a stub span replaces it plus the line under it and
		// leaves the `-end` marker dangling over content that is now gone.
		if owner != logmindOwner || kind != markerKindStub {
			return true
		}
		start = i
		return false
	})
	if start < 0 {
		return 0, 0, false
	}
	for i := start + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "<!--") {
			return start, i, true
		}
	}
	return start, len(lines), true
}

// jsonRedirectOwner decides ownership for the two JSON rows of SPEC §1.2's
// table (.sourcegraph/cody.json, .zed/settings.json).
//
// JSON has no comment syntax, so these files cannot carry the marker the
// markdown rows do — logmind's entry is the top-level `"logmind"` KEY, which
// the body it already ships is built around (see jsonAgentBody). A file
// carrying that key is one logmind installed into and may refresh; a file
// without it is somebody's real configuration. That is not hypothetical for
// `.zed/settings.json`: it is where a Zed user's entire configuration lives,
// and init used to render the logmind object straight over it.
//
// Unparseable is MarkerAbsent, deliberately — and it is the common case for
// Zed, whose settings file is JSONC and routinely carries comments. A file
// logmind cannot read is a file logmind cannot claim.
func jsonRedirectOwner(content string) RedirectOwner {
	const proof = `a top-level "logmind" key`
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &top); err != nil {
		return RedirectOwner{Ownership: MarkerAbsent, Marker: proof}
	}
	if _, ok := top[logmindOwner]; ok {
		return RedirectOwner{Ownership: MarkerOwned, Owner: logmindOwner, Marker: proof}
	}
	return RedirectOwner{Ownership: MarkerAbsent, Marker: proof}
}

// agentsMDRedirectOwner decides ownership for row 11 — AGENTS.md itself, which
// the registry reaches under the name "codex".
//
// Its marker is the logmind BLOCK, the same one EnsureAgentsMD installs and
// refreshes. Reusing HasLogmindSection/IsStub here rather than looking for a
// stub marker is the point: AGENTS.md is a file whose own content the user
// writes AROUND the block, so the block being present is what makes the file
// logmind-installed.
func agentsMDRedirectOwner(content string) RedirectOwner {
	const proof = "a `<!-- logmind-start -->` block"
	if HasLogmindSection(content) || IsStub(content) {
		return RedirectOwner{Ownership: MarkerOwned, Owner: logmindOwner, Marker: proof}
	}
	return RedirectOwner{Ownership: MarkerAbsent, Marker: proof}
}

// RedirectRefusal records one per-tool redirect file (SPEC §1.2) a run left
// alone because the bytes on disk are not logmind's — protocol#77's rows 2
// and 3. It is a REFUSAL, not a failure: the run continues, exits 0, and the
// caller MUST report it (SPEC §3.4's rule for the analogous fail-open case —
// "Failing open MUST NOT be silent"). Silence is the whole defect in #336.
type RedirectRefusal struct {
	Path      string          // repo-root-relative, e.g. "CLAUDE.md"
	Agent     string          // registry name, e.g. "claude"
	Display   string          // human tool name, e.g. "Claude Code"
	Ownership MarkerOwnership // which refusing state this is
	Owner     string          // component that owns it, when one is named
	Line      int             // 1-based line its marker sat on
	Marker    string          // what logmind looked for and did not find
}

// redirectPlan is what a write path may do with one per-tool redirect file:
// the exact bytes to write, or the refusal that says why nothing may be. At
// most one field is set — Body "" with a nil Refusal is the third outcome,
// "it is ours and there is nothing to change".
//
// Shaped after blockPlan, and for the same reason: one classifier that every
// path routes through cannot disagree with itself about what a file's state
// means, and the disagreement between two of them WAS #267.
type redirectPlan struct {
	Body    string
	Refusal *RedirectRefusal
}

// planRedirectWrite decides what happens to one per-tool redirect file, from
// its agent registry entry and the bytes already on disk. `exists` is passed
// separately because "" is a legitimate content for a file that IS there, and
// an empty file is the user's, not an invitation.
//
// The five states of protocol#77's ruling as amended by SPEC:1101, in order:
//
//  1. ABSENT → write the bundled body, carrying logmind's own marker. Nothing
//     but logmind produces these files in a repository that has not installed
//     the harness, so this row is what keeps SPEC §0.1's independence clause
//     true.
//  2. LOGMIND'S ENTRY, ALONE → splice the current body over that span. Same
//     mechanism as row 3; there is simply nothing else in the file.
//  3. LOGMIND'S ENTRY, ALONGSIDE SOMEBODY ELSE'S → splice the same span. The
//     `@AGENTS.md` import above it and another component's block below it are
//     outside the span and are copied through untouched.
//  4. FOREIGN MARKER, NO LOGMIND ENTRY → refuse and report.
//  5. NO MARKER AT ALL → refuse and report (SPEC:1101).
func planRedirectWrite(a agents.Agent, existing string, exists bool) redirectPlan {
	body := agentTemplate(a.Name)
	if !exists {
		return redirectPlan{Body: body}
	}
	owner := ReadRedirectOwner(a, existing)
	if !owner.Writable() {
		return redirectPlan{Refusal: refusalFor(a, owner)}
	}
	if a.FilePattern == agentsMDName {
		// Ours, and NOT ours to rewrite HERE. SPEC §5.2: "Exactly one
		// automation owns any generated or copied path" — AGENTS.md's is
		// EnsureAgentsMD, which rewrites the marked block and preserves every
		// byte around it. Rendering the bundled template over the file from
		// here undid that surgical refresh in the same `init` run, taking the
		// repository's own prose with it.
		return redirectPlan{}
	}
	if a.IsJSON {
		return redirectPlan{Body: mergeJSONEntry(existing, body)}
	}
	return redirectPlan{Body: mergeStubEntry(existing, body)}
}

// mergeStubEntry splices `body` over the span logmind owns in `existing`,
// returning "" when that leaves the file unchanged (so an idempotent re-run
// writes nothing and claims nothing).
//
// Returns "" as well when the file carries a logmind marker with no entry span
// to replace — a CLAUDE.md holding the LEGACY in-place `<!-- logmind-start -->`
// section rather than a stub. That file is logmind's, but converting it is
// `agents migrate`'s job, which folds its content into AGENTS.md first;
// stamping a stub over it from here would drop everything the block contains.
func mergeStubEntry(existing, body string) string {
	lines := strings.Split(existing, "\n")
	start, end, ok := logmindEntrySpan(lines)
	if !ok {
		return ""
	}
	// TrimRight, not Split-and-drop-last: the body's own trailing newline is
	// represented by whatever followed the entry in the ORIGINAL file, which
	// lines[end:] already carries. Re-adding it here would insert a blank line
	// on every refresh.
	merged := make([]string, 0, len(lines))
	merged = append(merged, lines[:start]...)
	merged = append(merged, strings.Split(strings.TrimRight(body, "\n"), "\n")...)
	merged = append(merged, lines[end:]...)
	out := strings.Join(merged, "\n")
	if out == existing {
		return ""
	}
	return out
}

// mergeJSONEntry replaces the top-level "logmind" value in `existing` with the
// one from `body`, leaving every other top-level key as it was, and returns ""
// when nothing changed.
//
// The re-render is deliberately skipped when logmind's key is the only one:
// the bundled body is hand-formatted, and round-tripping it through
// encoding/json would sort its keys and rewrite the file on the first run in
// every repository that has one. When there ARE other keys, re-rendering is
// unavoidable — and it is still the right trade, because the alternative is
// dropping the user's Zed configuration.
func mergeJSONEntry(existing, body string) string {
	var top, fresh map[string]json.RawMessage
	if json.Unmarshal([]byte(existing), &top) != nil || json.Unmarshal([]byte(body), &fresh) != nil {
		return ""
	}
	if len(top) == 1 {
		if existing == body {
			return ""
		}
		return body
	}
	top[logmindOwner] = fresh[logmindOwner]
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return ""
	}
	rendered := string(out) + "\n"
	if rendered == existing {
		return ""
	}
	return rendered
}

// refusalFor packages a non-writable verdict for the caller to report. One
// constructor, so a refusal cannot be raised without the fields the reporter
// needs to describe it.
func refusalFor(a agents.Agent, owner RedirectOwner) *RedirectRefusal {
	return &RedirectRefusal{
		Path:      a.FilePattern,
		Agent:     a.Name,
		Display:   a.Display,
		Ownership: owner.Ownership,
		Owner:     owner.Owner,
		Line:      owner.Line,
		Marker:    owner.Marker,
	}
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
// # ORDER IS THE SAFETY PROPERTY (#350)
//
// This is the one command whose job is to MOVE the user's own words rather
// than to add logmind's, so the single unrecoverable outcome is a source
// stubbed while its content exists nowhere else. That used to be reachable:
// the stub write ran inside the loop and the AGENTS.md write ran after it,
// with the collected content living only in a slice the error return
// discarded. A symlinked AGENTS.md — where the guard fires, correctly, at
// the append write — left two per-tool files holding the stub and the user's
// paragraphs in neither place.
//
// The three phases below are separate, and MUST stay in this order:
//
//  1. PLAN     — read, classify and refuse every source. Writes nothing.
//  2. PRESERVE — write AGENTS.md. Every source still holds its own bytes.
//  3. STUB     — only now replace the sources.
//
// EVERY REFUSAL RUNS BEFORE THE FIRST SOURCE IS TOUCHED, which is what the
// old shape got wrong — not the refusals themselves, whose messages are
// good. A per-tool file's symlink refusal moved up into phase 1 (it used to
// live in that file's own stub write, so a link on the fourth file was found
// after three had been consolidated) — but only after this run has decided,
// via the classification right above it, that the file is one phase 3 will
// actually replace: a symlink to an already-migrated stub, or to a file
// carrying another component's marker, was never going to be written and so
// is never refused (#351). AGENTS.md's symlink refusal and its read both sit
// in phase 2, ahead of all of phase 3.
//
// But the invariant is carried by the ORDER, not by any of those checks. A
// check that a path is writable is stale the moment it returns, and nothing
// here can predict a full disk, a mode change, or a link planted in the
// window between the check and the write — none of it has to. Every phase-2
// failure, predicted or not, happens while the sources are intact, and every
// phase-3 failure happens after AGENTS.md already holds the content. The
// user's words exist in at least one place at every instant. The checks buy
// a better error, not the invariant.
//
// ALL OR NOTHING. A source that cannot be read aborts the whole migration
// (phase 1) rather than being skipped. It costs nothing — nothing has been
// written yet — and half a consolidation, with no record of which half
// moved, is a worse thing to hand back than "fix .cursorrules and re-run".
// The skip-and-carry-on this replaced was never a policy; it was the only
// move available once the loop had already started destroying things.
//
// What phase 3 can still leave behind is DUPLICATION, never loss: content in
// AGENTS.md AND in a source that failed to stub. That is legible from the
// error and fixable by hand; what it replaces is not.
//
// Returns a list of human-readable status messages — the caller
// prints them to stdout. Returning a slice (rather than a stream)
// matches Python's accumulator pattern; the migrate command renders
// the slice line-by-line. They are composed in phase 1, so they are
// INTENTIONS until every phase has run: on any error the slice comes back
// nil rather than describing writes that may not have happened. The second
// return is EnsureAgentsMD's refusal (#267): migrate still consolidates the
// per-agent files, but the caller must say that AGENTS.md's own block was
// left alone. The third is the set of files this run declined to claim
// (#336) — see the plan loop for which.
func MigrateToAgentsMD(repoRoot string) ([]string, *AgentsBlockRefusal, []RedirectRefusal, error) {
	var messages []string
	var refusals []RedirectRefusal
	_, declined, err := EnsureAgentsMD(repoRoot)
	if err != nil {
		return nil, declined, refusals, err
	}
	agentsPath := filepath.Join(repoRoot, agentsMDName)
	var appendedBlocks []string
	var planned []plannedStub

	// ---- PHASE 1: PLAN. Reads and refusals only; nothing is written. ----
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
			// ALL OR NOTHING — see the doc comment. fileExists just said this
			// path is there, so a read failure is a real fault (mode, I/O, a
			// concurrent unlink), not an absence, and consolidating the other
			// files around it would silently migrate some of the user's
			// instructions and leave the rest.
			return nil, declined, refusals, fmt.Errorf("cannot migrate %s: %w",
				filepath.Base(filePath), err)
		}
		content := string(data)
		if IsStub(content) {
			continue // already migrated
		}
		// ANOTHER COMPONENT'S FILE IS NOT THE USER'S CONTENT TO CONSOLIDATE
		// (protocol#77 row 2, #336). Migrate reads an UNMARKED file as the
		// user's own instructions and moves them into AGENTS.md — which
		// preserves every byte and is the whole point of the command, so an
		// absent marker is deliberately NOT refused here the way it is in
		// CreateAgentFile. A marker naming skdd is a different thing: folding
		// its pointer line into AGENTS.md under "## From Claude Code" and
		// stamping logmind's stub on the path re-owns a file logmind was told
		// not to touch, which is the silent re-ownership the ruling exists to
		// stop. One classifier, two commands, two policies stated out loud.
		if owner := ReadRedirectOwner(a, content); owner.Ownership == MarkerForeign {
			refusals = append(refusals, *refusalFor(a, owner))
			continue
		}

		remaining := strings.TrimSpace(stripLogmindBlock(content))
		if remaining != "" {
			appendedBlocks = append(appendedBlocks,
				fmt.Sprintf("## From %s\n\n%s\n", a.Display, remaining))
			messages = append(messages,
				fmt.Sprintf("✓ Migrated %s (%s) content into AGENTS.md",
					a.Display, filepath.Base(filePath)))
		}
		// Refuse a symlink HERE, not ahead of the classification above: this
		// run has now decided the file is neither an already-migrated stub
		// nor another component's marked file, so it is one of the sources
		// phase 3 is actually going to replace. Checking any earlier — the
		// shape this replaced — asked "is this a symlink" before asking "was
		// this run ever going to write here", and refused a link the two
		// classifications above would themselves have skipped or left alone
		// (a stub with nothing left to consolidate, a foreign file this
		// command declines on purpose). The reason for refusing at all is
		// still CreateAgentFile's: the ownership verdict just above is made
		// from the file's bytes, os.ReadFile resolves the final component,
		// and a link pointing outside the repository has some other file
		// answering "whose content is .cursorrules?" — but that question
		// only needs answering for a path this run is actually about to stub.
		if err := atomicio.RefuseSymlink(filePath); err != nil {
			return nil, declined, refusals, err
		}
		planned = append(planned, plannedStub{path: filePath})
		messages = append(messages,
			fmt.Sprintf("✓ %s replaced with stub", filepath.Base(filePath)))
	}

	// ---- PHASE 2: PRESERVE. The sources are all still intact here. ----
	//
	// AGENTS.md's own refusals are NOT hoisted above this block, and there is
	// no pre-flight atomicio.RefuseSymlink(agentsPath) here on purpose: the
	// refusal inside migrateWrite already runs before the first source is
	// touched, because this phase does, and a second copy of the check a few
	// lines earlier would be unobservable — same error, same untouched tree —
	// while reading as though the invariant rested on it. It rests on the
	// order. Note also that this whole phase is skipped when there is nothing
	// to consolidate, so a repo that is already migrated stays a no-op rather
	// than acquiring a new reason to fail.
	if len(appendedBlocks) > 0 {
		existing, err := os.ReadFile(agentsPath)
		if err != nil {
			return nil, declined, refusals, err
		}
		// Match Python: existing.rstrip() + "\n\n" + "\n".join(appended).
		body := strings.TrimRight(string(existing), " \t\n\r") +
			"\n\n" + strings.Join(appendedBlocks, "\n")
		if err := migrateWrite(agentsPath, []byte(body), 0o644); err != nil {
			return nil, declined, refusals, err
		}
	}

	// ---- PHASE 3: STUB. Destructive, and reached only once AGENTS.md
	// holds every byte phase 1 collected. ----
	for _, p := range planned {
		// The path resolved and read cleanly in phase 1, which also refused a
		// symlink at it, so this is a per-agent artifact the user owns —
		// written through atomicio (over the bare os.WriteFile this replaced)
		// so a link planted in the window since is refused, #300, rather than
		// silently written through.
		if err := migrateWrite(p.path, []byte(templates.Stub()), 0o644); err != nil {
			return nil, declined, refusals, err
		}
	}
	return messages, declined, refusals, nil
}

// plannedStub is one per-agent file phase 1 decided to replace with the
// stub. A named type rather than a bare []string because the plan is the
// thing phase 3 is not allowed to re-derive: re-walking agents.All() there
// would re-read the sources and could act on a set phase 1 never approved.
type plannedStub struct {
	path string
}

// migrateWrite is atomicio.WriteFile, held as a package-level var so
// MigrateToAgentsMD's ordering invariant can be tested against the failures
// no pre-check can predict — a full disk, a mode change since the plan
// phase read the file. Those are exactly the cases where the invariant has to come from
// the ORDER rather than from the refusal, and the symlink case (which a
// pre-check CAN see) cannot stand in for them. Same seam, and the same
// reason, as internal/skill/sync.go's atomicWriteFile and atomicio's own
// syncFile. Other write sites in this package call atomicio.WriteFile
// directly; only migrate needs the injectable failure.
var migrateWrite func(path string, data []byte, perm os.FileMode) error = atomicio.WriteFile

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
