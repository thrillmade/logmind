package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/templates"
)

// TestAgentsList_FreshRepo: empty directory → every agent shows
// "not configured", and the supported-agents footer renders the
// canonical order.
//
// Golden is byte-identical to Python's CliRunner output (verified
// against the running Python build in the B4 PR description).
func TestAgentsList_FreshRepo(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runAgentsList(dir, &stdout); err != nil {
		t.Fatalf("runAgentsList: %v", err)
	}
	checkGolden(t, "agents_list_fresh.golden", stdout.String())
}

// TestAgentsList_StubInstalled: writing a stub CLAUDE.md flips its
// row to "configured" — exercises the IsStub() + Configured branch
// in GetAgentStatus.
func TestAgentsList_StubInstalled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"),
		[]byte(templates.Stub()), 0o644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	var stdout bytes.Buffer
	if err := runAgentsList(dir, &stdout); err != nil {
		t.Fatalf("runAgentsList: %v", err)
	}
	checkGolden(t, "agents_list_claude_stub.golden", stdout.String())
}

// TestAgentsList_ForeignFile: a CLAUDE.md without logmind markers
// gets the "exists (no logmind)" row — yellow ~ in Python, plain
// glyph here.
func TestAgentsList_ForeignFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"),
		[]byte("# Custom CLAUDE\n\nNo markers.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	if err := runAgentsList(dir, &stdout); err != nil {
		t.Fatalf("runAgentsList: %v", err)
	}
	checkGolden(t, "agents_list_claude_foreign.golden", stdout.String())
}

// TestAgentsAdd_Unknown: invalid name → "Error: Unknown agent ..."
// + exit 1 (ErrSilent).
func TestAgentsAdd_Unknown(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	err := runAgentsAdd(dir, "unknown_agent", false, &stdout, io.Discard)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runAgentsAdd err = %v; want ErrSilent", err)
	}
	checkGolden(t, "agents_add_unknown.golden", stdout.String())
}

// TestAgentsAdd_NewCursor: writes the stub at .cursorrules and
// reports "✓ Created .cursorrules ..."
func TestAgentsAdd_NewCursor(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runAgentsAdd(dir, "cursor", true, &stdout, io.Discard); err != nil {
		t.Fatalf("runAgentsAdd: %v", err)
	}
	checkGolden(t, "agents_add_cursor_new.golden", stdout.String())
	// Confirm the file body matches the stub byte-for-byte.
	body, err := os.ReadFile(filepath.Join(dir, ".cursorrules"))
	if err != nil {
		t.Fatalf("read .cursorrules: %v", err)
	}
	if string(body) != templates.Stub() {
		t.Errorf(".cursorrules body drift:\n--- want ---\n%s\n--- got ---\n%s",
			templates.Stub(), body)
	}
}

// TestAgentsAdd_ExistingForeign: CLAUDE.md exists without logmind →
// inserter splices the section in and prints "✓ Added logmind
// instructions to CLAUDE.md".
func TestAgentsAdd_ExistingForeign(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"),
		[]byte("# Custom\n\nProject notes.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	if err := runAgentsAdd(dir, "claude", true, &stdout, io.Discard); err != nil {
		t.Fatalf("runAgentsAdd: %v", err)
	}
	checkGolden(t, "agents_add_claude_foreign.golden", stdout.String())
	// CLAUDE.md now contains the marker.
	body, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), "<!-- logmind-start -->") {
		t.Errorf("CLAUDE.md missing logmind marker after add")
	}
}

// TestAgentsAdd_ExistingWithMarker: CLAUDE.md already has the
// marker → "already has logmind instructions" + no file change.
func TestAgentsAdd_ExistingWithMarker(t *testing.T) {
	dir := t.TempDir()
	original := "# CLAUDE\n\n<!-- logmind-start -->\nblock\n<!-- logmind-end -->\n"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(original), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	if err := runAgentsAdd(dir, "claude", true, &stdout, io.Discard); err != nil {
		t.Fatalf("runAgentsAdd: %v", err)
	}
	checkGolden(t, "agents_add_claude_already.golden", stdout.String())
	// File untouched.
	body, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != original {
		t.Errorf("file mutated when already configured")
	}
}

// TestAgentsRemove_Unknown — exit 1 + Error message.
func TestAgentsRemove_Unknown(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	err := runAgentsRemove(dir, "unknown_agent", true, true, strings.NewReader(""), &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runAgentsRemove err = %v; want ErrSilent", err)
	}
	checkGolden(t, "agents_remove_unknown.golden", stdout.String())
}

// TestAgentsRemove_NotConfigured — file absent → yellow "not
// configured" message + exit 0.
func TestAgentsRemove_NotConfigured(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runAgentsRemove(dir, "claude", true, true, strings.NewReader(""), &stdout); err != nil {
		t.Fatalf("runAgentsRemove: %v", err)
	}
	checkGolden(t, "agents_remove_not_configured.golden", stdout.String())
}

// TestAgentsRemove_Forced: --force skips the prompt and removes the file.
func TestAgentsRemove_Forced(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	if err := runAgentsRemove(dir, "claude", true, true, strings.NewReader(""), &stdout); err != nil {
		t.Fatalf("runAgentsRemove: %v", err)
	}
	checkGolden(t, "agents_remove_forced.golden", stdout.String())
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Errorf("CLAUDE.md still exists after force-remove")
	}
}

// TestAgentsRemove_CancelOnNoConfirm: missing --force + stdin "" →
// "Cancelled." + file remains.
func TestAgentsRemove_CancelOnNoConfirm(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	if err := runAgentsRemove(dir, "claude", false, true, strings.NewReader("n\n"), &stdout); err != nil {
		t.Fatalf("runAgentsRemove: %v", err)
	}
	checkGolden(t, "agents_remove_cancelled.golden", stdout.String())
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Errorf("CLAUDE.md was removed despite 'n' answer")
	}
}

// TestAgentsUpdate_NoAgentsMD: empty repo → "No AGENTS.md in this repo".
func TestAgentsUpdate_NoAgentsMD(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runAgentsUpdate(dir, "1.0.0-dev", false, &stdout, io.Discard); err != nil {
		t.Fatalf("runAgentsUpdate: %v", err)
	}
	checkGolden(t, "agents_update_no_agents_md.golden", stdout.String())
}

// TestAgentsUpdate_AgentsMDNoBlock: AGENTS.md present, no markers →
// "exists but has no logmind marker block" message.
func TestAgentsUpdate_AgentsMDNoBlock(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte("# AGENTS\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	if err := runAgentsUpdate(dir, "1.0.0-dev", false, &stdout, io.Discard); err != nil {
		t.Fatalf("runAgentsUpdate: %v", err)
	}
	checkGolden(t, "agents_update_no_block.golden", stdout.String())
}

// TestAgentsUpdate_Current: slim template installed → "current" message.
func TestAgentsUpdate_Current(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte(templates.AgentsSlimTemplate()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	if err := runAgentsUpdate(dir, "1.0.0-dev", false, &stdout, io.Discard); err != nil {
		t.Fatalf("runAgentsUpdate: %v", err)
	}
	checkGolden(t, "agents_update_current.golden", stdout.String())
}

// TestAgentsMigrate_Empty: no per-agent files → "No agent files to
// migrate".
func TestAgentsMigrate_Empty(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runAgentsMigrate(dir, true, &stdout, io.Discard); err != nil {
		t.Fatalf("runAgentsMigrate: %v", err)
	}
	checkGolden(t, "agents_migrate_empty.golden", stdout.String())
}

// TestAgentsMigrate_WithClaude: CLAUDE.md with user content + marker
// block → migrate consolidates content into AGENTS.md and replaces
// CLAUDE.md with the stub.
func TestAgentsMigrate_WithClaude(t *testing.T) {
	dir := t.TempDir()
	claudeBody := "# CLAUDE\n\n## My notes\n\nKeep this.\n\n" +
		"<!-- logmind-start -->\nblock\n<!-- logmind-end -->\n"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"),
		[]byte(claudeBody), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	var stdout bytes.Buffer
	if err := runAgentsMigrate(dir, true, &stdout, io.Discard); err != nil {
		t.Fatalf("runAgentsMigrate: %v", err)
	}
	out := stdout.String()
	// Don't pin every line of stdout via golden — MigrateToAgentsMD's
	// message ordering depends on the registry iteration. Pin the
	// load-bearing assertions instead: the migration succeeded, both
	// status messages fired, and the side effects landed.
	if !strings.Contains(out, "Migrated Claude Code") {
		t.Errorf("missing migrated-content message; got:\n%s", out)
	}
	if !strings.Contains(out, "CLAUDE.md replaced with stub") {
		t.Errorf("missing stub-replace message; got:\n%s", out)
	}
	// CLAUDE.md is now a stub.
	body, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(body), "<!-- logmind-stub:") {
		t.Errorf("CLAUDE.md not stubbed after migrate")
	}
	// AGENTS.md contains the consolidated content.
	agentsBody, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agentsBody), "## From Claude Code") {
		t.Errorf("AGENTS.md missing consolidated section")
	}
	if !strings.Contains(string(agentsBody), "Keep this.") {
		t.Errorf("AGENTS.md missing user content")
	}
}
