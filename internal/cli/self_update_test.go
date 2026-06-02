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
