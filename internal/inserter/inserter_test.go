package inserter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/templates"
)

// TestReplaceMarkerBlock_RoundTrip is the byte-identical invariant
// proof: extract the original body, replace with garbage, replace
// back with the original body, and the result must equal the input
// byte-for-byte. Content outside the markers is preserved.
//
// This is the load-bearing property the B4 PR description claims.
// Without it, any `agents update --apply` could silently corrupt
// content above or below the markers (user content the agent or
// human added between sessions).
func TestReplaceMarkerBlock_RoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"full template", templates.AgentsTemplate()},
		{"slim template", templates.AgentsSlimTemplate()},
		{"template + user content above", "# My Project\n\nNotes.\n\n" + templates.AgentsSlimTemplate()},
		{"template + user content below", templates.AgentsSlimTemplate() + "\n## My Notes\n\nExtra info.\n"},
		{"template + user content both", "# My Project\n\nIntro.\n\n" + templates.AgentsSlimTemplate() + "\n## Extra\n\nBottom.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig, ok := ExtractMarkerBlock(tc.content)
			if !ok {
				t.Fatalf("ExtractMarkerBlock returned !ok for input with valid markers")
			}
			// Replace with garbage.
			garbageBody := "\n!! WRECK !!\n"
			wrecked := ReplaceMarkerBlock(tc.content, garbageBody)
			if !strings.Contains(wrecked, garbageBody) {
				t.Fatalf("ReplaceMarkerBlock didn't insert garbage body")
			}
			if wrecked == tc.content {
				t.Fatalf("ReplaceMarkerBlock didn't change content")
			}
			// Restore.
			restored := ReplaceMarkerBlock(wrecked, orig)
			if restored != tc.content {
				t.Fatalf("round-trip drift:\n--- want ---\n%s\n--- got ---\n%s",
					tc.content, restored)
			}
		})
	}
}

// TestReplaceMarkerBlock_PreservesOuterBytes verifies that content
// above the start marker and below the end marker is untouched even
// when the new body has a completely different length / shape.
func TestReplaceMarkerBlock_PreservesOuterBytes(t *testing.T) {
	above := "# Heading\n\nThis is the prologue.\n\n"
	below := "\n## Below\n\nSee references.\n"
	body := startMarker + "\nold body\n" + endMarker
	c0 := above + body + below

	c1 := ReplaceMarkerBlock(c0, "\nNEW BODY OF DIFFERENT LENGTH\n")
	if !strings.HasPrefix(c1, above) {
		t.Fatalf("prefix bytes were modified:\n--- want ---\n%q\n--- got ---\n%q",
			above, c1[:len(above)])
	}
	if !strings.HasSuffix(c1, below) {
		t.Fatalf("suffix bytes were modified:\n--- want ---\n%q\n--- got ---\n%q",
			below, c1[len(c1)-len(below):])
	}
}

// TestReplaceMarkerBlock_NoMarkers returns content unchanged when
// markers are absent. Used to guard against silent corruption when
// the file format drifts.
func TestReplaceMarkerBlock_NoMarkers(t *testing.T) {
	in := "# Plain markdown\n\nNo markers here.\n"
	out := ReplaceMarkerBlock(in, "NEW")
	if out != in {
		t.Fatalf("input was mutated despite missing markers:\n--- want ---\n%q\n--- got ---\n%q",
			in, out)
	}
}

// TestExtractMarkerBlock_MissingMarkers — both markers absent → !ok.
func TestExtractMarkerBlock_MissingMarkers(t *testing.T) {
	_, ok := ExtractMarkerBlock("nothing here\n")
	if ok {
		t.Fatalf("ExtractMarkerBlock returned ok=true for content without markers")
	}
}

// TestUpdateWorkflowPin_DoubleQuoted bumps the canonical
// pip install "logmind==X.Y.Z" form.
func TestUpdateWorkflowPin_DoubleQuoted(t *testing.T) {
	in := "      run: pip install \"logmind==0.6.13\"\n"
	out, old := UpdateWorkflowPin(in, "0.6.14")
	if old != "0.6.13" {
		t.Errorf("previous = %q; want 0.6.13", old)
	}
	want := "      run: pip install \"logmind==0.6.14\"\n"
	if out != want {
		t.Errorf("output = %q; want %q", out, want)
	}
}

// TestUpdateWorkflowPin_SingleQuoted is the v0.6.11 widened-pattern
// case — the reporulez convention.
func TestUpdateWorkflowPin_SingleQuoted(t *testing.T) {
	in := "      run: pip install 'logmind==0.6.13'\n"
	out, old := UpdateWorkflowPin(in, "0.6.14")
	if old != "0.6.13" {
		t.Errorf("previous = %q; want 0.6.13", old)
	}
	want := "      run: pip install 'logmind==0.6.14'\n"
	if out != want {
		t.Errorf("output = %q; want %q", out, want)
	}
}

// TestUpdateWorkflowPin_NoQuotes bumps the unquoted form.
func TestUpdateWorkflowPin_NoQuotes(t *testing.T) {
	in := "      run: pip install logmind==0.6.13\n"
	out, old := UpdateWorkflowPin(in, "0.6.14")
	if old != "0.6.13" {
		t.Errorf("previous = %q; want 0.6.13", old)
	}
	want := "      run: pip install logmind==0.6.14\n"
	if out != want {
		t.Errorf("output = %q; want %q", out, want)
	}
}

// TestUpdateWorkflowPin_Idempotent — running the same version twice
// returns content unchanged and previous = the version itself.
func TestUpdateWorkflowPin_Idempotent(t *testing.T) {
	in := "      run: pip install \"logmind==0.6.14\"\n"
	out, old := UpdateWorkflowPin(in, "0.6.14")
	if old != "0.6.14" {
		t.Errorf("previous = %q; want 0.6.14", old)
	}
	if out != in {
		t.Errorf("idempotent re-pin changed content:\n--- want ---\n%q\n--- got ---\n%q",
			in, out)
	}
}

// TestUpdateWorkflowPin_NoPin — no pip-install line → previous = "".
func TestUpdateWorkflowPin_NoPin(t *testing.T) {
	in := "name: regen-timeline\non: workflow_dispatch\n"
	out, old := UpdateWorkflowPin(in, "0.6.14")
	if old != "" {
		t.Errorf("previous = %q; want \"\" for no-pin content", old)
	}
	if out != in {
		t.Errorf("content was changed despite no pin")
	}
}

// TestFindOutdatedWorkflowPins_NoWorkflowsDir — returns nil when
// .github/workflows/ is absent.
func TestFindOutdatedWorkflowPins_NoWorkflowsDir(t *testing.T) {
	dir := t.TempDir()
	out, err := FindOutdatedWorkflowPins(dir, "0.6.14")
	if err != nil {
		t.Fatalf("FindOutdatedWorkflowPins: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result; got %v", out)
	}
}

// TestFindOutdatedWorkflowPins_DetectsStale seeds a stale pin in
// regen-timeline.yml and asserts it's surfaced.
func TestFindOutdatedWorkflowPins_DetectsStale(t *testing.T) {
	dir := t.TempDir()
	workflowsDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wfPath := filepath.Join(workflowsDir, "regen-timeline.yml")
	body := "      run: pip install \"logmind==0.6.13\"\n"
	if err := os.WriteFile(wfPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedWorkflowPins(dir, "0.6.14")
	if err != nil {
		t.Fatalf("FindOutdatedWorkflowPins: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 outdated entry; got %d: %v", len(out), out)
	}
	if out[0].OldVersion != "0.6.13" || out[0].NewVersion != "0.6.14" {
		t.Errorf("version mismatch: %+v", out[0])
	}
}

// TestFindOutdatedWorkflowPins_SkipsUnknownWorkflows — custom user
// workflows are NOT in pinWorkflows; even a stale pin there must be
// ignored. Critical safety: we shouldn't rewrite files the user owns.
func TestFindOutdatedWorkflowPins_SkipsUnknownWorkflows(t *testing.T) {
	dir := t.TempDir()
	workflowsDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wfPath := filepath.Join(workflowsDir, "user-workflow.yml")
	body := "      run: pip install \"logmind==0.6.13\"\n"
	if err := os.WriteFile(wfPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedWorkflowPins(dir, "0.6.14")
	if err != nil {
		t.Fatalf("FindOutdatedWorkflowPins: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("user workflow shouldn't be swept; got %v", out)
	}
}

// TestEnsureAgentsMD_MissingCreates writes the canonical template
// when AGENTS.md doesn't exist.
func TestEnsureAgentsMD_MissingCreates(t *testing.T) {
	dir := t.TempDir()
	msg, err := EnsureAgentsMD(dir)
	if err != nil {
		t.Fatalf("EnsureAgentsMD: %v", err)
	}
	if msg != "Created AGENTS.md (canonical agent instructions)" {
		t.Errorf("status = %q; want Created", msg)
	}
	body, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(body) != templates.AgentsSlimTemplate() {
		t.Errorf("AGENTS.md content doesn't match slim template")
	}
}

// TestEnsureAgentsMD_NoOpWhenCurrent — running twice in a row, the
// second call returns "" (no-op).
func TestEnsureAgentsMD_NoOpWhenCurrent(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureAgentsMD(dir); err != nil {
		t.Fatalf("first EnsureAgentsMD: %v", err)
	}
	msg, err := EnsureAgentsMD(dir)
	if err != nil {
		t.Fatalf("second EnsureAgentsMD: %v", err)
	}
	if msg != "" {
		t.Errorf("second call status = %q; want empty (no-op)", msg)
	}
}

// TestEnsureAgentsMD_NoMarkersInserts — file exists without markers
// → insert the section.
func TestEnsureAgentsMD_NoMarkersInserts(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("# AGENTS\n\nNo markers.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	msg, err := EnsureAgentsMD(dir)
	if err != nil {
		t.Fatalf("EnsureAgentsMD: %v", err)
	}
	if msg != "Added logmind section to existing AGENTS.md" {
		t.Errorf("status = %q; want Added section", msg)
	}
	body, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), startMarker) {
		t.Errorf("marker not inserted; body = %s", body)
	}
}

// TestEnsureAgentsMD_FullBlockRefreshesFullNotSlim pins the flavour-
// preserving refresh contract for the `doctor --fix` / `init`-refresh
// path (both go through EnsureAgentsMD). A repo that shipped a FULL block
// (here a stale v6 full body) must refresh IN PLACE to the CURRENT full
// body — NOT get silently flipped to the current slim default.
// Second run is a no-op (idempotent). Mirrors invariant #3 documented on
// agentsMDTemplate and the version-guard in matchingTemplate.
//
// Deliberately generalized: rather than hardcoding a specific target
// version marker, this compares against templates.AgentsTemplate() /
// AgentsSlimTemplate() dynamically, so it keeps passing across future
// version bumps without an update every time — only the DEDICATED
// vN→vN+1 tests below (e.g. TestEnsureAgentsMD_V7FullRefreshesToV8) need
// to change per bump.
func TestEnsureAgentsMD_FullBlockRefreshesFullNotSlim(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "AGENTS.md")
	// A stale v6 full block: correct FULL marker flavour, differing bytes.
	staleFull := ReplaceMarkerBlock(templates.AgentsTemplate(),
		"\n<!-- logmind-block-version: v6 -->\n## Decision Logging (logmind) — REQUIRED for substantive commits\nstale full body\n")
	if err := os.WriteFile(agentsPath, []byte(staleFull), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	msg, err := EnsureAgentsMD(dir)
	if err != nil {
		t.Fatalf("EnsureAgentsMD: %v", err)
	}
	if msg == "" {
		t.Fatalf("stale full block should have been refreshed; got no-op")
	}
	got, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	wantBlock, ok := ExtractMarkerBlock(templates.AgentsTemplate())
	if !ok {
		t.Fatalf("could not extract marker block from templates.AgentsTemplate()")
	}
	gotBlock, ok := ExtractMarkerBlock(string(got))
	if !ok {
		t.Fatalf("could not extract marker block from refreshed AGENTS.md")
	}
	if strings.TrimSpace(gotBlock) != strings.TrimSpace(wantBlock) {
		t.Errorf("full block must refresh to the current full body;\ngot  %q\nwant %q", gotBlock, wantBlock)
	}
	slimBlock, ok := ExtractMarkerBlock(templates.AgentsSlimTemplate())
	if ok && strings.TrimSpace(gotBlock) == strings.TrimSpace(slimBlock) {
		t.Errorf("full block must NOT be flipped to the slim default")
	}
	if !strings.Contains(string(got), "Branch summary (headline)") {
		t.Errorf("refreshed full body must carry the branch-summary convention")
	}

	// Idempotent: a second pass over the now-current full body is a no-op.
	msg2, err := EnsureAgentsMD(dir)
	if err != nil {
		t.Fatalf("second EnsureAgentsMD: %v", err)
	}
	if msg2 != "" {
		t.Errorf("second EnsureAgentsMD over the current full body must be a no-op; got %q", msg2)
	}
}

// TestGetAgentStatus_AllAbsent — no files anywhere → every entry has
// exists=false, configured=false.
func TestGetAgentStatus_AllAbsent(t *testing.T) {
	dir := t.TempDir()
	rows := GetAgentStatus(dir)
	if len(rows) != 11 {
		t.Fatalf("expected 11 rows; got %d", len(rows))
	}
	for _, row := range rows {
		if row.Exists {
			t.Errorf("%s: Exists=true unexpected", row.Name)
		}
		if row.Configured {
			t.Errorf("%s: Configured=true unexpected", row.Name)
		}
	}
}

// TestGetAgentStatus_StubCountsAsConfigured covers the stub path: a
// file containing the stub marker reports configured=true.
func TestGetAgentStatus_StubCountsAsConfigured(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(templates.Stub()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows := GetAgentStatus(dir)
	var claude *AgentStatus
	for i := range rows {
		if rows[i].Name == "claude" {
			claude = &rows[i]
			break
		}
	}
	if claude == nil {
		t.Fatalf("claude row missing")
	}
	if !claude.Configured {
		t.Errorf("stub CLAUDE.md should be configured")
	}
}

// TestMigrateToAgentsMD_PreservesUserContent: a CLAUDE.md with user
// content above + logmind block + user content below should
// consolidate user content under "## From Claude Code" and replace
// CLAUDE.md with the stub.
func TestMigrateToAgentsMD_PreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "CLAUDE.md")
	body := "# CLAUDE\n\nProject overview.\n\n" + startMarker +
		"\nlogmind block\n" + endMarker + "\n\nMore notes.\n"
	if err := os.WriteFile(claudePath, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	msgs, err := MigrateToAgentsMD(dir)
	if err != nil {
		t.Fatalf("MigrateToAgentsMD: %v", err)
	}
	if len(msgs) < 2 {
		t.Fatalf("expected migration messages; got %v", msgs)
	}
	// CLAUDE.md is now a stub.
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !IsStub(string(got)) {
		t.Errorf("CLAUDE.md is not a stub after migrate")
	}
	// AGENTS.md has the consolidated content.
	agentsBody, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agentsBody), "## From Claude Code") {
		t.Errorf("AGENTS.md missing consolidated section; got %s", agentsBody)
	}
}

// TestMigrateToAgentsMD_Idempotent — running on an already-stubbed
// tree is a no-op (zero messages besides AGENTS.md creation).
func TestMigrateToAgentsMD_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(templates.Stub()), 0o644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	msgs, err := MigrateToAgentsMD(dir)
	if err != nil {
		t.Fatalf("MigrateToAgentsMD: %v", err)
	}
	// No "Migrated" / "replaced with stub" messages — claude is already a stub.
	for _, m := range msgs {
		if strings.Contains(m, "CLAUDE.md replaced") || strings.Contains(m, "Migrated Claude") {
			t.Errorf("idempotent migrate produced action message: %q", m)
		}
	}
}

// TestCreateAgentFile_Stub creates CLAUDE.md as a stub for the
// default markdown agent.
func TestCreateAgentFile_Stub(t *testing.T) {
	dir := t.TempDir()
	path, err := CreateAgentFile("claude", dir)
	if err != nil {
		t.Fatalf("CreateAgentFile: %v", err)
	}
	if path != filepath.Join(dir, "CLAUDE.md") {
		t.Errorf("path = %q; want CLAUDE.md", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != templates.Stub() {
		t.Errorf("body doesn't match stub template")
	}
}

// TestCreateAgentFile_Codex creates AGENTS.md (slim).
func TestCreateAgentFile_Codex(t *testing.T) {
	dir := t.TempDir()
	path, err := CreateAgentFile("codex", dir)
	if err != nil {
		t.Fatalf("CreateAgentFile: %v", err)
	}
	if path != filepath.Join(dir, "AGENTS.md") {
		t.Errorf("path = %q; want AGENTS.md", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != templates.AgentsSlimTemplate() {
		t.Errorf("codex AGENTS.md doesn't match slim template")
	}
}

// TestCreateAgentFile_UnknownReturnsEmpty mirrors Python's None for
// unknown agents — we return ("", nil).
func TestCreateAgentFile_UnknownReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path, err := CreateAgentFile("unknown", dir)
	if err != nil {
		t.Fatalf("CreateAgentFile: %v", err)
	}
	if path != "" {
		t.Errorf("path = %q; want empty for unknown agent", path)
	}
}

// TestRemoveAgentFile_DeletesExisting writes CLAUDE.md, then removes it.
func TestRemoveAgentFile_DeletesExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ok, err := RemoveAgentFile("claude", dir)
	if err != nil {
		t.Fatalf("RemoveAgentFile: %v", err)
	}
	if !ok {
		t.Errorf("expected ok=true for existing file")
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Errorf("file still exists after removal")
	}
}

// TestRemoveAgentFile_Missing — absent file → (false, nil).
func TestRemoveAgentFile_Missing(t *testing.T) {
	dir := t.TempDir()
	ok, err := RemoveAgentFile("claude", dir)
	if err != nil {
		t.Fatalf("RemoveAgentFile: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false for missing file")
	}
}

// TestDetectTemplateDrift_AgentsMDMissing — no AGENTS.md → "AGENTS.md missing".
func TestDetectTemplateDrift_AgentsMDMissing(t *testing.T) {
	dir := t.TempDir()
	drift, err := DetectTemplateDrift(dir, nil)
	if err != nil {
		t.Fatalf("DetectTemplateDrift: %v", err)
	}
	if len(drift) != 1 || drift[0] != "AGENTS.md missing" {
		t.Errorf("drift = %v; want [AGENTS.md missing]", drift)
	}
}

// TestDetectTemplateDrift_NoMarkers — AGENTS.md exists without markers.
func TestDetectTemplateDrift_NoMarkers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# AGENTS\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	drift, err := DetectTemplateDrift(dir, nil)
	if err != nil {
		t.Fatalf("DetectTemplateDrift: %v", err)
	}
	want := "AGENTS.md present but missing logmind block"
	found := false
	for _, d := range drift {
		if d == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("drift = %v; want to contain %q", drift, want)
	}
}

// TestFindOutdatedMarkerBlocks_NoFile returns nil when AGENTS.md is
// absent (nothing to update, distinct error path from "exists but
// current").
func TestFindOutdatedMarkerBlocks_NoFile(t *testing.T) {
	dir := t.TempDir()
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result; got %v", out)
	}
}

// TestFindOutdatedMarkerBlocks_CurrentNoop — fresh template install →
// nothing outdated.
func TestFindOutdatedMarkerBlocks_CurrentNoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte(templates.AgentsSlimTemplate()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("current template should not be outdated; got %v", out)
	}
}

// TestFindOutdatedMarkerBlocks_DriftedDetected swaps the block body
// for content that still carries the version marker but differs from
// the template. Asserts FindOutdatedMarkerBlocks reports it.
//
// Why we preserve the marker: matchingTemplate returns "" when the
// installed body has no recognized version marker (the version-guard).
// Drift detection only applies when the flavour matches.
//
// v0.6.16 bumped the slim marker v7-pointer→v8-pointer; the drifted
// body must carry the CURRENT marker so the template-flavour selector
// (matchingTemplate) returns the slim template rather than "" — that's
// what triggers the byte-compare that detects drift.
func TestFindOutdatedMarkerBlocks_DriftedDetected(t *testing.T) {
	dir := t.TempDir()
	driftedBody := "\n<!-- logmind-block-version: v8-pointer -->\nDIFFERENT CONTENT BUT SAME VERSION\n"
	wrecked := ReplaceMarkerBlock(templates.AgentsSlimTemplate(), driftedBody)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(wrecked), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 outdated entry; got %d", len(out))
	}
	if !strings.Contains(out[0].OldBody, "DIFFERENT CONTENT") {
		t.Errorf("OldBody = %q; want to contain DIFFERENT CONTENT", out[0].OldBody)
	}
}

// TestMatchingTemplate_PointerCheckOrderingGuardsAgainstV8Collision is a
// DIRECT unit test of matchingTemplate's marker-ordering contract — the
// exact collision the stale-binary-hardening / enforcement wave's v8/
// v8-pointer bump introduced: "logmind-block-version: v8-pointer" contains
// "logmind-block-version: v8" as a literal substring (the bare "v8" marker
// is a PREFIX of "v8-pointer"). If the bare-"v8" full-flavour branch were
// ever checked BEFORE the "-pointer" slim branches, a slim v8-pointer body
// would incorrectly match the full check too and get mis-classified.
// matchingTemplate's actual ordering (every "-pointer" variant checked
// first) prevents that; this test proves it two ways:
//
//  1. A body carrying the slim "v8-pointer" marker resolves to the SLIM
//     template — not full — even though it contains "v8" as a substring.
//  2. A hypothetical FULL body carrying a bare "v8" marker (no "-pointer"
//     suffix) resolves to the FULL template.
func TestMatchingTemplate_PointerCheckOrderingGuardsAgainstV8Collision(t *testing.T) {
	slimV8PointerBody := "\n<!-- logmind-block-version: v8-pointer -->\n## Decision logging\nslim body\n"
	got := matchingTemplate(slimV8PointerBody)
	if got != templates.AgentsSlimTemplate() {
		t.Errorf("matchingTemplate(v8-pointer body) did not resolve to the slim template — " +
			"the bare-v8 full check must not have run before the -pointer check")
	}

	hypotheticalFullV8Body := "\n<!-- logmind-block-version: v8 -->\n## Decision Logging (logmind) — REQUIRED for substantive commits\nfull body\n"
	got = matchingTemplate(hypotheticalFullV8Body)
	if got != templates.AgentsTemplate() {
		t.Errorf("matchingTemplate(bare v8 body) did not resolve to the full template")
	}
}

// TestFindOutdatedMarkerBlocks_NeverFlipsFlavour — installed CURRENT full
// template + Go binary defaults to slim → DO NOT report as outdated.
// Matches the version-guard documented in matchingTemplate. Also serves as
// the "a repo already on the current full body is up-to-date" no-drift
// case. Dynamically installs templates.AgentsTemplate() so this keeps
// passing across future version bumps without needing an update.
func TestFindOutdatedMarkerBlocks_NeverFlipsFlavour(t *testing.T) {
	dir := t.TempDir()
	// Install the CURRENT full template. We don't want the slim default
	// to silently rewrite this into the slim body.
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte(templates.AgentsTemplate()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("installed current full vs default-slim must NOT be reported as outdated; got %v", out)
	}
}

// TestFindOutdatedMarkerBlocks_OldFullRefreshesToCurrent — installed v5
// full template MUST be reported as outdated so the `agents update
// --apply` workflow refreshes it forward to the CURRENT full body.
// Without this, existing full installs never see prose changes.
//
// Deliberately generalized (not pinned to a specific target version
// number): the refresh target is compared against
// templates.AgentsTemplate() dynamically, so this test keeps passing
// across future version bumps — only the DEDICATED vN→vN+1 tests below
// (e.g. TestFindOutdatedMarkerBlocks_V7RefreshesToV8) need to change per
// bump.
func TestFindOutdatedMarkerBlocks_OldFullRefreshesToCurrent(t *testing.T) {
	dir := t.TempDir()
	// Synthesize a v5-shaped block body. The byte content differs from
	// the current template, so the drift compare must fire.
	v5Body := "\n<!-- logmind-block-version: v5 -->\n## Decision Logging (logmind)\nold v5 body\n"
	wrecked := ReplaceMarkerBlock(templates.AgentsTemplate(), v5Body)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(wrecked), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("v5 block vs current template must be reported as outdated; got %d entries", len(out))
	}
	wantBlock, ok := ExtractMarkerBlock(templates.AgentsTemplate())
	if !ok {
		t.Fatalf("could not extract marker block from templates.AgentsTemplate()")
	}
	if out[0].NewBody != wantBlock {
		t.Errorf("refresh target NewBody must equal the current full template's block;\ngot  %q\nwant %q", out[0].NewBody, wantBlock)
	}
}

// TestFindOutdatedMarkerBlocks_PriorFullRefreshesToCurrent — a repo
// carrying the immediately-prior full flavour (shipped to consumer repos
// before the current bump) MUST be reported as outdated and its refresh
// target MUST be the CURRENT full body (carrying the branch-summary
// convention), NOT the slim body. This is what lets consumer repos pick
// up prose changes via `agents update` / `doctor --fix` drift detection.
//
// Originally pinned the Slice-2 branch-summary wave's v6→v7 bump;
// generalized (dynamic comparison against templates.AgentsTemplate(),
// no hardcoded target version) so it survives future bumps. See
// TestFindOutdatedMarkerBlocks_V7RefreshesToV8 for the CURRENT bump's
// dedicated regression test.
func TestFindOutdatedMarkerBlocks_PriorFullRefreshesToCurrent(t *testing.T) {
	dir := t.TempDir()
	// Synthesize a v6-shaped full block body (older marker, differing bytes).
	v6Body := "\n<!-- logmind-block-version: v6 -->\n## Decision Logging (logmind) — REQUIRED for substantive commits\nold v6 body without the branch-summary convention\n"
	wrecked := ReplaceMarkerBlock(templates.AgentsTemplate(), v6Body)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(wrecked), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("v6 block vs current template must be reported as outdated; got %d entries", len(out))
	}
	// Refresh target must be the CURRENT full body — not a full↔slim flip.
	wantBlock, ok := ExtractMarkerBlock(templates.AgentsTemplate())
	if !ok {
		t.Fatalf("could not extract marker block from templates.AgentsTemplate()")
	}
	if out[0].NewBody != wantBlock {
		t.Errorf("refresh target NewBody must equal the current full template's block;\ngot  %q\nwant %q", out[0].NewBody, wantBlock)
	}
	slimBlock, ok := ExtractMarkerBlock(templates.AgentsSlimTemplate())
	if ok && out[0].NewBody == slimBlock {
		t.Errorf("v6 full block must NOT refresh to the slim body")
	}
	if !strings.Contains(out[0].NewBody, "Branch summary (headline)") {
		t.Errorf("refresh target must carry the branch-summary (headline) convention")
	}
}

// TestFindOutdatedMarkerBlocks_V7RefreshesToV8 is the load-bearing case
// for the stale-binary-hardening / enforcement wave: a repo carrying a v7
// full block (the immediately-prior full flavour, shipped to consumer
// repos before this bump) MUST be reported as outdated and its refresh
// target MUST be the v8 full body — carrying the BLOCK enforcement prose
// and its carve-outs — NOT the slim v9-pointer body. This is what lets
// consumer repos pick up the enforcement-prose update via `agents update`
// / `doctor --fix` drift detection.
func TestFindOutdatedMarkerBlocks_V7RefreshesToV8(t *testing.T) {
	dir := t.TempDir()
	// Synthesize a v7-shaped full block body (older marker, pre-enforcement
	// prose, differing bytes).
	v7Body := "\n<!-- logmind-block-version: v7 -->\n## Decision Logging (logmind) — REQUIRED for substantive commits\nold v7 body: the commit-msg hook only warns\n"
	wrecked := ReplaceMarkerBlock(templates.AgentsTemplate(), v7Body)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(wrecked), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("v7 block vs v8 template must be reported as outdated; got %d entries", len(out))
	}
	// Refresh target must be the FULL v8 body — not a full↔slim flip.
	if !strings.Contains(out[0].NewBody, "logmind-block-version: v8 ") {
		t.Errorf("refresh target NewBody must carry the v8 marker; got %q", out[0].NewBody)
	}
	if strings.Contains(out[0].NewBody, "v9-pointer") {
		t.Errorf("v7 full block must NOT refresh to the slim v9-pointer body")
	}
	if !strings.Contains(out[0].NewBody, "BLOCK") {
		t.Errorf("v8 refresh target must carry the BLOCK enforcement prose")
	}
	if !strings.Contains(out[0].NewBody, "[skip-logmind]") ||
		!strings.Contains(out[0].NewBody, "LOGMIND_ALLOW_GIT_COMMIT=1") ||
		!strings.Contains(out[0].NewBody, "git.enforce_commits: false") {
		t.Errorf("v8 refresh target must carry all three enforcement carve-outs; got %q", out[0].NewBody)
	}
}

// TestFindOutdatedMarkerBlocks_OldSlimRefreshesToCurrent — same contract
// as TestFindOutdatedMarkerBlocks_OldFullRefreshesToCurrent for the slim
// variant: an installed v7-pointer block must refresh to the CURRENT slim
// body. Generalized (dynamic comparison, no hardcoded target version) so
// it survives future bumps. See
// TestFindOutdatedMarkerBlocks_V8PointerRefreshesToV9Pointer for the
// CURRENT bump's dedicated regression test.
func TestFindOutdatedMarkerBlocks_OldSlimRefreshesToCurrent(t *testing.T) {
	dir := t.TempDir()
	v7Body := "\n<!-- logmind-block-version: v7-pointer -->\n## Decision logging — `logmind log` is the commit primitive\nold v7 body\n"
	wrecked := ReplaceMarkerBlock(templates.AgentsSlimTemplate(), v7Body)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(wrecked), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("v7-pointer block vs current slim template must be reported as outdated; got %d entries", len(out))
	}
	wantBlock, ok := ExtractMarkerBlock(templates.AgentsSlimTemplate())
	if !ok {
		t.Fatalf("could not extract marker block from templates.AgentsSlimTemplate()")
	}
	if out[0].NewBody != wantBlock {
		t.Errorf("refresh target NewBody must equal the current slim template's block;\ngot  %q\nwant %q", out[0].NewBody, wantBlock)
	}
}

// TestFindOutdatedMarkerBlocks_V8PointerRefreshesToV9Pointer is the
// dedicated regression test for the stale-binary-hardening / enforcement
// wave's slim bump: an installed v8-pointer block (pre-enforcement prose)
// MUST refresh to the v9-pointer body carrying the BLOCK enforcement
// prose and its carve-outs.
func TestFindOutdatedMarkerBlocks_V8PointerRefreshesToV9Pointer(t *testing.T) {
	dir := t.TempDir()
	v8Body := "\n<!-- logmind-block-version: v8-pointer -->\n## Decision logging — `logmind log` is the commit primitive\nold v8-pointer body: the commit-msg hook only warns\n"
	wrecked := ReplaceMarkerBlock(templates.AgentsSlimTemplate(), v8Body)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(wrecked), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("v8-pointer block vs v9-pointer template must be reported as outdated; got %d entries", len(out))
	}
	if !strings.Contains(out[0].NewBody, "logmind-block-version: v9-pointer") {
		t.Errorf("refresh target NewBody must carry v9-pointer marker; got %q", out[0].NewBody)
	}
	if !strings.Contains(out[0].NewBody, "BLOCK") {
		t.Errorf("v9-pointer refresh target must carry the BLOCK enforcement prose")
	}
	if !strings.Contains(out[0].NewBody, "[skip-logmind]") ||
		!strings.Contains(out[0].NewBody, "LOGMIND_ALLOW_GIT_COMMIT=1") ||
		!strings.Contains(out[0].NewBody, "git.enforce_commits: false") {
		t.Errorf("v9-pointer refresh target must carry all three enforcement carve-outs; got %q", out[0].NewBody)
	}
}

// TestReplaceMarkerBlock_InvertedMarkers — when end marker appears
// BEFORE start marker (malformed input), the function MUST return
// content unchanged. Without this guard, ReplaceMarkerBlock would
// silently corrupt by duplicating the inter-marker region.
//
// Flagged by clud-bug-review on PR #118 — ExtractMarkerBlock already
// had this guard; ReplaceMarkerBlock did not. Pinned by this test.
func TestReplaceMarkerBlock_InvertedMarkers(t *testing.T) {
	// End marker appears BEFORE start marker — corrupt file shape.
	content := "PREFIX\n<!-- /logmind-block-version -->\nMIDDLE\n<!-- logmind-block-version: v8-pointer -->\nSUFFIX"
	got := ReplaceMarkerBlock(content, "WOULD-OVERWRITE")
	if got != content {
		t.Errorf("ReplaceMarkerBlock on inverted markers must return content unchanged.\nWant: %q\nGot:  %q", content, got)
	}
}

// TestInsertLogmindSection_NoHeading — the path that runs when no
// `# ` heading is found in the file. The block must be prepended at
// position 0 (before all existing content). Without this test, that
// branch (where insertIndex stays at 0) was untested.
//
// Flagged by clud-bug-review on PR #118.
func TestInsertLogmindSection_NoHeading(t *testing.T) {
	dir := t.TempDir()
	// A `.cursorrules`-shaped file that goes straight into rules without
	// an H1 heading — common pattern for those agent files.
	original := "Rule 1: be terse.\nRule 2: cite sources.\n"
	target := filepath.Join(dir, ".cursorrules")
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changed, err := InsertLogmindSection(target)
	if err != nil {
		t.Fatalf("InsertLogmindSection: %v", err)
	}
	if !changed {
		t.Fatal("InsertLogmindSection returned changed=false on a file without the marker; want true")
	}

	out, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(out)
	// InsertLogmindSection inserts the section near position 0 for
	// headingless files (after a possible leading newline). The actual
	// marker shipped in templates/logmind-section.md is
	// `<!-- logmind-start -->`. Assert the marker appears BEFORE the
	// user content — the no-heading branch's contract.
	markerIdx := strings.Index(got, "<!-- logmind-start -->")
	rulesIdx := strings.Index(got, "Rule 1: be terse.")
	if markerIdx == -1 {
		t.Fatalf("logmind-start marker not found in output; first 200 chars: %q", got[:min(200, len(got))])
	}
	if rulesIdx == -1 {
		t.Errorf("original rules must be preserved after the inserted block; got:\n%s", got)
	}
	if markerIdx > rulesIdx {
		t.Errorf("logmind block must precede user content (markerIdx=%d > rulesIdx=%d)", markerIdx, rulesIdx)
	}
	// Idempotency: second call must be a no-op (marker already present).
	changed2, err := InsertLogmindSection(target)
	if err != nil {
		t.Fatalf("InsertLogmindSection (2nd call): %v", err)
	}
	if changed2 {
		t.Error("second InsertLogmindSection call must return changed=false; got true")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
