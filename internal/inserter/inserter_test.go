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
func TestFindOutdatedMarkerBlocks_DriftedDetected(t *testing.T) {
	dir := t.TempDir()
	driftedBody := "\n<!-- logmind-block-version: v7-pointer -->\nDIFFERENT CONTENT BUT SAME VERSION\n"
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

// TestFindOutdatedMarkerBlocks_NeverFlipsFlavour — installed v5 full
// + Go binary defaults to slim → DO NOT report as outdated. Matches
// the version-guard documented in matchingTemplate.
func TestFindOutdatedMarkerBlocks_NeverFlipsFlavour(t *testing.T) {
	dir := t.TempDir()
	// Install the FULL v5 template. We don't want the slim default to
	// silently rewrite this into v7-pointer.
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte(templates.AgentsTemplate()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("installed v5 vs default-slim must NOT be reported as outdated; got %v", out)
	}
}
