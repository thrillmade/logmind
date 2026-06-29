package tree

import (
	"os"
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
