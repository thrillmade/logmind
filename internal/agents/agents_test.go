package agents

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestNames_OrderMatchesPython pins the iteration order. The
// `agents list` output depends on this order — any drift here would
// change the byte-for-byte output of every position-stable test.
//
// Python source: src/logmind/core/inserter.AGENT_REGISTRY at v0.6.14.
func TestNames_OrderMatchesPython(t *testing.T) {
	want := []string{
		"claude", "cursor", "copilot", "windsurf", "aider",
		"continue", "cody", "zed", "amazonq", "cline", "codex",
	}
	got := Names()
	if len(got) != len(want) {
		t.Fatalf("len(Names) = %d; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// TestLookup_KnownAgents covers the registry contents for each agent.
// One-line per agent so additions / removals are obvious in diffs.
func TestLookup_KnownAgents(t *testing.T) {
	cases := []struct {
		name        string
		pattern     string
		displayName string
		isJSON      bool
	}{
		{"claude", "CLAUDE.md", "Claude Code", false},
		{"cursor", ".cursorrules", "Cursor", false},
		{"copilot", ".github/copilot-instructions.md", "GitHub Copilot", false},
		{"windsurf", ".windsurfrules", "Windsurf", false},
		{"aider", "CONVENTIONS.md", "Aider", false},
		{"continue", ".continuerules", "Continue", false},
		{"cody", ".sourcegraph/cody.json", "Sourcegraph Cody", true},
		{"zed", ".zed/settings.json", "Zed AI", true},
		{"amazonq", ".amazonq/rules.md", "Amazon Q", false},
		{"cline", ".clinerules", "Cline", false},
		{"codex", "AGENTS.md", "OpenAI Codex", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, ok := Lookup(tc.name)
			if !ok {
				t.Fatalf("Lookup(%q) returned !ok", tc.name)
			}
			if a.FilePattern != tc.pattern {
				t.Errorf("FilePattern = %q; want %q", a.FilePattern, tc.pattern)
			}
			if a.Display != tc.displayName {
				t.Errorf("Display = %q; want %q", a.Display, tc.displayName)
			}
			if a.IsJSON != tc.isJSON {
				t.Errorf("IsJSON = %v; want %v", a.IsJSON, tc.isJSON)
			}
		})
	}
}

// TestLookup_UnknownReturnsFalse — defensive: callers branch on the
// ok bool to print the "Unknown agent" error.
func TestLookup_UnknownReturnsFalse(t *testing.T) {
	if _, ok := Lookup("not-a-real-agent"); ok {
		t.Fatalf("Lookup of unknown agent returned ok=true")
	}
}

// TestFilePath_RoutesThroughFilepathJoin asserts the OS-native path
// separator wins. The pattern in the registry is forward-slash
// (logical paths); on disk we always want the OS form.
func TestFilePath_RoutesThroughFilepathJoin(t *testing.T) {
	repoRoot := filepath.Join("/", "tmp", "repo")
	if runtime.GOOS == "windows" {
		repoRoot = filepath.Join("C:\\", "tmp", "repo")
	}
	got, ok := FilePath("copilot", repoRoot)
	if !ok {
		t.Fatalf("FilePath(copilot) returned !ok")
	}
	want := filepath.Join(repoRoot, ".github", "copilot-instructions.md")
	if got != want {
		t.Fatalf("FilePath = %q; want %q", got, want)
	}
}

// TestDefaultEnabled — claude + cursor in that order.
func TestDefaultEnabled(t *testing.T) {
	got := DefaultEnabled()
	want := []string{"claude", "cursor"}
	if len(got) != len(want) {
		t.Fatalf("len(DefaultEnabled) = %d; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DefaultEnabled()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// TestIsJSON pins cody + zed → true, everything else → false.
func TestIsJSON(t *testing.T) {
	if !IsJSON("cody") {
		t.Errorf("IsJSON(cody) = false; want true")
	}
	if !IsJSON("zed") {
		t.Errorf("IsJSON(zed) = false; want true")
	}
	for _, name := range []string{"claude", "cursor", "codex", "unknown"} {
		if IsJSON(name) {
			t.Errorf("IsJSON(%q) = true; want false", name)
		}
	}
}
