// self_update_test.go — exercises `logmind self-update`.
//
// Coverage:
//   - Fresh repo with no logmind artifacts → self-update reports
//     "templates are up to date" (nothing to refresh).
//   - Repo with stale hooks → refresh rewrites them.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfUpdate_FreshRepoNoChanges(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"self-update"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("self-update: %v\n%s\n%s", err, out.String(), errOut.String())
		}
		// Empty repo has nothing to refresh — AGENTS.md doesn't exist
		// (EnsureAgentsMD creates it, then we'd report). Acceptable
		// either way; we only assert the trailing OK line is emitted.
		if !strings.Contains(out.String(), "ok self-update applied") {
			t.Errorf("missing ok line; output=\n%s", out.String())
		}
	})
}

func TestSelfUpdate_RefreshesStaleHook(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		// Simulate an existing .git/hooks/ dir with a stale logmind hook.
		if err := os.MkdirAll(filepath.Join(".git", "hooks"), 0o755); err != nil {
			t.Fatal(err)
		}
		stale := "#!/bin/sh\n# logmind post-merge hook\n# logmind-hook-version: 0.1.0\necho stale\n"
		if err := os.WriteFile(filepath.Join(".git", "hooks", "post-merge"), []byte(stale), 0o755); err != nil {
			t.Fatal(err)
		}
		root := NewRootCmd()
		root.SetArgs([]string{"self-update"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("self-update: %v\n%s", err, errOut.String())
		}
		if !strings.Contains(out.String(), "Refreshed .git/hooks/post-merge") {
			t.Errorf("expected refresh notice; got\n%s", out.String())
		}
	})
	// Confirm the hook now carries the current marker.
	body, err := os.ReadFile(filepath.Join(dir, ".git", "hooks", "post-merge"))
	if err != nil {
		t.Fatalf("read post-merge: %v", err)
	}
	if strings.Contains(string(body), "echo stale") {
		t.Errorf("stale body not replaced; got\n%s", string(body))
	}
}

// TestSelfUpdate_InstallsClaudePreToolUseGuard: self-update is a refresh
// path of its own (separate from init/doctor --fix), so it must also
// install Layer 1 — otherwise a repo that only ever runs self-update
// gets its commit-msg hook auto-upgraded to enforcing while the
// PreToolUse guard stays missing forever (and doctor never nudges,
// because "missing" is benign).
func TestSelfUpdate_InstallsClaudePreToolUseGuard(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"self-update"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("self-update: %v\n%s\n%s", err, out.String(), errOut.String())
		}
		if !strings.Contains(out.String(), "✓ Refreshed .claude/settings.json (Claude Code guard-commit hook)") {
			t.Errorf("expected the Claude guard refresh notice; got\n%s", out.String())
		}
	})
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("expected .claude/settings.json after self-update: %v", err)
	}
	if !strings.Contains(string(body), "logmind guard-commit --layer harness") {
		t.Errorf("settings.json missing the guard-commit entry:\n%s", body)
	}
}

// TestSelfUpdate_ClaudeDisabledInConfigSkipsGuard: agents.claude:false is
// the same opt-out doctor --fix honors — self-update must respect it too.
func TestSelfUpdate_ClaudeDisabledInConfigSkipsGuard(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		if err := os.MkdirAll(".logmind", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(".logmind", "config.yml"), []byte("agents:\n  claude: false\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		root := NewRootCmd()
		root.SetArgs([]string{"self-update"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("self-update: %v\n%s\n%s", err, out.String(), errOut.String())
		}
		if strings.Contains(out.String(), "Claude Code guard-commit hook") {
			t.Errorf("did NOT expect the Claude guard notice with agents.claude:false; got\n%s", out.String())
		}
	})
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.json")); err == nil {
		t.Errorf("did NOT expect .claude/settings.json when agents.claude is false")
	}
}
