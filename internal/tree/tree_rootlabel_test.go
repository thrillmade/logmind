package tree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func mustWriteTree(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRenderWithLabel_DeterministicAcrossDirNames is the churn fix: the same
// content in two differently-named checkout dirs + the same label produces
// identical bytes (without a label, the root line would differ).
func TestRenderWithLabel_DeterministicAcrossDirNames(t *testing.T) {
	build := func(name string) string {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		mustWriteTree(t, filepath.Join(dir, "a.txt"), "x")
		mustWriteTree(t, filepath.Join(dir, "sub", "b.txt"), "y")
		out, err := RenderWithLabel(dir, IgnoreRules{}, -1, "myrepo")
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	a := build("checkout-one")
	b := build("a-completely-different-name")
	if a != b {
		t.Errorf("not deterministic across dir names:\n--- A ---\n%s\n--- B ---\n%s", a, b)
	}
	if !strings.HasPrefix(a, "myrepo\n") {
		t.Errorf("root line = %q; want it to start with the label", a)
	}
}

// TestRenderWithLabel_EmptyLabelMatchesRender is the byte-parity guard:
// label=="" must reproduce Render's basename behavior exactly.
func TestRenderWithLabel_EmptyLabelMatchesRender(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "somerepo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteTree(t, filepath.Join(dir, "a.txt"), "x")

	withEmpty, err := RenderWithLabel(dir, IgnoreRules{}, -1, "")
	if err != nil {
		t.Fatal(err)
	}
	viaRender, err := Render(dir, IgnoreRules{}, -1)
	if err != nil {
		t.Fatal(err)
	}
	if withEmpty != viaRender {
		t.Errorf("label=\"\" diverged from Render (byte-parity broken):\n%q\nvs\n%q", withEmpty, viaRender)
	}
	if !strings.HasPrefix(viaRender, "somerepo\n") {
		t.Errorf("default root line = %q; want the basename 'somerepo'", viaRender)
	}
}

// TestResolveRootLabel covers the config→label mapping (auto resolution is
// exercised in the gitcli RemoteRepoName test).
func TestResolveRootLabel(t *testing.T) {
	if got := resolveRootLabel(t.TempDir(), ""); got != "" {
		t.Errorf("empty → %q; want \"\"", got)
	}
	if got := resolveRootLabel(t.TempDir(), "fixed-name"); got != "fixed-name" {
		t.Errorf("verbatim → %q; want fixed-name", got)
	}
	// "auto" in a non-git dir → "" (degrades to basename downstream).
	if got := resolveRootLabel(t.TempDir(), "auto"); got != "" {
		t.Errorf("auto in non-git dir → %q; want \"\"", got)
	}
}

// TestResolveRootLabel_WorktreeMatchesMainCheckout is the regression pin
// for logmind#285: a `git worktree` checkout must resolve to the SAME
// root label as the main checkout it was created from, even though the
// two live in differently-named directories (a real repo's checkout dir
// vs. ".../agent-<id>"). Before the fix, resolveRootLabel's default
// ("", unconfigured) case fell straight through to "", and
// RenderWithLabel's basename fallback then picked up whichever directory
// the command happened to run from — silently corrupting
// docs/file-structure.md's root line on every regen inside a worktree.
func TestResolveRootLabel_WorktreeMatchesMainCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping integration test")
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
		}
	}

	base := t.TempDir()
	main := filepath.Join(base, "myrepo")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	run(main, "init", "-q")
	run(main, "config", "user.email", "test@test.com")
	run(main, "config", "user.name", "test")
	mustWriteTree(t, filepath.Join(main, "README.md"), "hello\n")
	run(main, "add", "README.md")
	run(main, "commit", "-q", "-m", "init")

	// Checkout dir name deliberately unrelated to the repo name — mirrors
	// the parallel-agent worktree layout the issue reports.
	worktree := filepath.Join(base, "agent-deadbeef123")
	run(main, "worktree", "add", "-q", worktree, "-b", "wt-branch")

	mainLabel := resolveRootLabel(main, "")
	worktreeLabel := resolveRootLabel(worktree, "")

	if mainLabel != "myrepo" {
		t.Errorf("resolveRootLabel(main checkout, \"\") = %q; want %q", mainLabel, "myrepo")
	}
	if worktreeLabel != mainLabel {
		t.Errorf("worktree root label = %q; main checkout root label = %q — the two checkouts of the same repo must agree (logmind#285)", worktreeLabel, mainLabel)
	}
}
