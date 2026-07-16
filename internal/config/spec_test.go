package config

import (
	"path/filepath"
	"testing"
)

func TestSpecFile_DefaultUnset(t *testing.T) {
	if got := DefaultConfig().Context.SpecFile; got != "" {
		t.Errorf("default Context.SpecFile = %q; want \"\" (fold-in disabled, byte-parity)", got)
	}
}

func TestSpecFile_RoundTrip(t *testing.T) {
	cfg, err := LoadPath(writeTimelineCfg(t, "context:\n  spec_file: docs/spec.md\n"))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if cfg.Context.SpecFile != "docs/spec.md" {
		t.Errorf("Context.SpecFile = %q; want docs/spec.md", cfg.Context.SpecFile)
	}
	// Leafwise deep-merge guard: setting spec_file must not disturb the
	// sibling repomap default.
	if cfg.Context.Repomap {
		t.Errorf("setting spec_file flipped Repomap on; deep-merge should be leafwise")
	}
}

// TestResolveSpecFile_Unset covers the "" (never configured) case.
func TestResolveSpecFile_Unset(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	if _, ok := ResolveSpecFile(root, cfg); ok {
		t.Errorf("ResolveSpecFile with unset spec_file returned ok=true; want false")
	}
}

// TestResolveSpecFile_RelativeInRoot is the happy path: a plain
// repo-relative path resolves to repoRoot/<rel>.
func TestResolveSpecFile_RelativeInRoot(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.Context.SpecFile = "docs/spec.md"
	path, ok := ResolveSpecFile(root, cfg)
	if !ok {
		t.Fatalf("ResolveSpecFile: want ok=true for a plain relative path")
	}
	want := filepath.Join(root, "docs", "spec.md")
	if path != want {
		t.Errorf("path = %q; want %q", path, want)
	}
}

// TestResolveSpecFile_NoOutOfRootRead is the NORMATIVE security assertion:
// an absolute path, and a relative path that escapes repoRoot via `..`, MUST
// both be treated as unset — logmind must never read a spec_file outside
// the repo root, regardless of what a (possibly untrusted, possibly
// merge-conflicted) .logmind/config.yml says.
func TestResolveSpecFile_NoOutOfRootRead(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		rel  string
	}{
		{"parent-escape", "../evil.md"},
		{"deep-parent-escape", "../../../../etc/hosts"},
		{"absolute-etc-hosts", "/etc/hosts"},
		{"absolute-inside-root-looking", filepath.Join(root, "docs", "spec.md")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Context.SpecFile = tc.rel
			path, ok := ResolveSpecFile(root, cfg)
			if ok {
				t.Errorf("ResolveSpecFile(%q) = (%q, true); want unset (false) — must never escape/absolute-read", tc.rel, path)
			}
			if path != "" {
				t.Errorf("ResolveSpecFile(%q) returned non-empty path %q on the unset branch", tc.rel, path)
			}
		})
	}
}

// TestResolveSpecFile_SiblingDirNameNotConfusedForEscape guards against a
// naive string-prefix check: a sibling directory that merely SHARES a
// prefix with repoRoot (e.g. repoRoot="/tmp/repo" vs "/tmp/repo-evil") must
// not be treated as "inside" repoRoot just because HasPrefix would say so.
func TestResolveSpecFile_SiblingDirNameNotConfusedForEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	evilSibling := filepath.Join(parent, "repo-evil", "spec.md")
	rel, err := filepath.Rel(root, evilSibling)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	cfg := DefaultConfig()
	cfg.Context.SpecFile = rel
	if path, ok := ResolveSpecFile(root, cfg); ok {
		t.Errorf("ResolveSpecFile(%q) = (%q, true); want unset — resolves to a sibling dir, not inside root", rel, path)
	}
}
