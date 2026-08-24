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

	"github.com/thrillmade/logmind/internal/testgit"
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
		// A REAL repository with a stale logmind hook in it. `git init`
		// rather than a hand-made `.git/hooks`: the installers resolve
		// their target with `git rev-parse --git-path hooks` (hooks.Dir)
		// now, so a directory git will not answer about gets no hook
		// written to it at all.
		testgit.InitRepo(t, "", "-q", "--initial-branch=main")
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

// TestSelfUpdate_PinVersionSet_NoOps pins the MINOR fix (SPEC §1.2.1 /
// §3.7): a `.logmind/config.yml` with a non-empty top-level `pinVersion`
// makes self-update a complete no-op — not even a stale, clearly-outdated
// hook gets refreshed. Reuses TestSelfUpdate_RefreshesStaleHook's fixture
// shape so the contrast is direct: same stale hook, but pinned this time.
func TestSelfUpdate_PinVersionSet_NoOps(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		if err := os.MkdirAll(".logmind", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(".logmind", "config.yml"), []byte("pinVersion: \"1.2.3\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		testgit.InitRepo(t, "", "-q", "--initial-branch=main")
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
			t.Fatalf("self-update: %v\n%s\n%s", err, out.String(), errOut.String())
		}
		if !strings.Contains(out.String(), "pinned to 1.2.3") {
			t.Errorf("expected a clear pinned-version message; got\n%s", out.String())
		}
		if strings.Contains(out.String(), "Refreshed") {
			t.Errorf("pinned self-update must not refresh anything; got\n%s", out.String())
		}
		if strings.Contains(out.String(), "ok self-update applied") {
			t.Errorf("pinned self-update must not emit the normal-run ok line; got\n%s", out.String())
		}
	})
	// The stale hook body must be untouched — no refresh happened at all.
	body, err := os.ReadFile(filepath.Join(dir, ".git", "hooks", "post-merge"))
	if err != nil {
		t.Fatalf("read post-merge: %v", err)
	}
	if !strings.Contains(string(body), "echo stale") {
		t.Errorf("pinned self-update must leave the stale hook alone; got\n%s", string(body))
	}
	// No .claude/settings.json either — Layer 1 install is also skipped.
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.json")); err == nil {
		t.Errorf("pinned self-update must not install .claude/settings.json")
	}
}

// TestSelfUpdate_PinVersionUnset_RunsNormally is the direct contrast: the
// default (no pinVersion key at all) must still refresh exactly as before
// — the fix must not accidentally gate the unset case.
func TestSelfUpdate_PinVersionUnset_RunsNormally(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		testgit.InitRepo(t, "", "-q", "--initial-branch=main")
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
			t.Fatalf("self-update: %v\n%s\n%s", err, out.String(), errOut.String())
		}
		if !strings.Contains(out.String(), "ok self-update applied") {
			t.Errorf("expected the normal-run ok line; got\n%s", out.String())
		}
	})
	body, err := os.ReadFile(filepath.Join(dir, ".git", "hooks", "post-merge"))
	if err != nil {
		t.Fatalf("read post-merge: %v", err)
	}
	if strings.Contains(string(body), "echo stale") {
		t.Errorf("stale body not replaced when pinVersion is unset; got\n%s", string(body))
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
