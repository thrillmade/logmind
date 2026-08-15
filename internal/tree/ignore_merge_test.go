package tree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The SPEC §1.4 contract these tests pin:
//
//	Patterns come from three sources, merged: the built-in defaults, the
//	repository's .gitignore with leading and trailing "/" trimmed, and
//	file_structure.ignore_patterns from config. A !pattern re-includes a
//	path an earlier pattern excluded.
//
// Every assertion here is on the RENDERED file-structure document, not on
// ResolveRules' return value — the bug in #269 was invisible at the helper
// level (ResolveRules faithfully merged whatever it was handed; the config
// loader had already thrown the defaults away) and only showed up as
// node_modules/ appearing in docs/file-structure.md.

// namedRepo lays out a fixture under a fixed "myrepo" directory so the
// rendered root line is deterministic (same trick as TestFileStructureTemplate).
func namedRepo(t *testing.T, layout map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "myrepo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range layout {
		full := filepath.Join(root, rel)
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// writeIgnoreConfig drops a .logmind/config.yml whose only setting is
// file_structure.ignore_patterns — the shape a repository that wants one
// extra pattern would hand-write.
func writeIgnoreConfig(t *testing.T, root string, patterns ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("file_structure:\n  ignore_patterns:\n")
	for _, p := range patterns {
		b.WriteString("    - \"" + p + "\"\n")
	}
	dir := filepath.Join(root, ".logmind")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustGenerate(t *testing.T, root string) string {
	t.Helper()
	got, err := GenerateFileStructure(root, -1)
	if err != nil {
		t.Fatalf("GenerateFileStructure: %v", err)
	}
	return got
}

// mustHide fails when any of `names` survives into the rendered tree.
func mustHide(t *testing.T, got string, names ...string) {
	t.Helper()
	for _, n := range names {
		if strings.Contains(got, n) {
			t.Errorf("rendered tree still contains %q (should be ignored):\n%s", n, got)
		}
	}
}

// mustShow fails when any of `names` is missing from the rendered tree.
func mustShow(t *testing.T, got string, names ...string) {
	t.Helper()
	for _, n := range names {
		if !strings.Contains(got, n) {
			t.Errorf("rendered tree is missing %q (should be kept):\n%s", n, got)
		}
	}
}

// TestFileStructure_ConfigPatternsMergeWithDefaults is the #269 regression:
// a repository that sets ONE ignore pattern used to lose all sixteen
// built-in defaults, because config.LoadPath unmarshals YAML over
// DefaultConfig() and yaml.Unmarshal REPLACES a slice rather than appending.
// node_modules/, dist/ and the rest then flooded docs/file-structure.md.
func TestFileStructure_ConfigPatternsMergeWithDefaults(t *testing.T) {
	root := namedRepo(t, map[string]string{
		"node_modules/lib.js": "n",
		"dist/bundle.js":      "d",
		".venv/pyvenv.cfg":    "v",
		"scratch.tmp":         "s",
		"docs/readme.md":      "r",
	})
	writeIgnoreConfig(t, root, "*.tmp")

	got := mustGenerate(t, root)
	// The custom pattern still applies...
	mustHide(t, got, "scratch.tmp")
	// ...and so do the built-in defaults it used to displace.
	mustHide(t, got, "node_modules", "dist", ".venv")
	// Control: the fixture really was walked.
	mustShow(t, got, "docs", "readme.md")
}

// TestFileStructure_ConfigNegationReincludesDefault covers the escape hatch
// that makes an unconditional merge safe: a repository that genuinely wants
// dist/ on its map says so with "!dist". Without this, merging the defaults
// in would remove capability from every repo that had opted out by listing
// its own patterns.
func TestFileStructure_ConfigNegationReincludesDefault(t *testing.T) {
	root := namedRepo(t, map[string]string{
		"dist/bundle.js":      "d",
		"node_modules/lib.js": "n",
		"docs/readme.md":      "r",
	})
	writeIgnoreConfig(t, root, "!dist")

	got := mustGenerate(t, root)
	mustShow(t, got, "dist", "bundle.js")
	// Negating one default must not disturb the others.
	mustHide(t, got, "node_modules")
}

// TestFileStructure_GitignoreSourceHonoured pins the second of §1.4's three
// sources, including the leading- and trailing-"/" trim: ".gitignore" entries
// are written "/coverage/" and "reports/", and both forms must ignore.
func TestFileStructure_GitignoreSourceHonoured(t *testing.T) {
	root := namedRepo(t, map[string]string{
		"coverage/index.html": "c",
		"reports/out.xml":     "x",
		"docs/readme.md":      "r",
		".gitignore":          "# comment\n\n/coverage/\nreports/\n",
	})

	got := mustGenerate(t, root)
	mustHide(t, got, "coverage", "reports")
	mustShow(t, got, "docs", "readme.md")
}

// TestFileStructure_GitignoreNegationReincludesDefault covers a negation
// arriving from the second source: ".gitignore" re-includes a path the
// built-in defaults excluded.
//
// It is also the guard against the trap that positional last-match-wins
// resolution would have sprung. This repository configures NOTHING, yet
// config.Load still hands ResolveRules the sixteen defaults as the config
// source — so a positional resolver would let that re-appended `dist`
// override the `!dist` here, hiding a directory the repository asked to
// see. See IgnoreRules.Matches.
func TestFileStructure_GitignoreNegationReincludesDefault(t *testing.T) {
	root := namedRepo(t, map[string]string{
		"dist/bundle.js":      "d",
		"node_modules/lib.js": "n",
		".gitignore":          "!dist\n",
	})

	got := mustGenerate(t, root)
	mustShow(t, got, "dist", "bundle.js")
	mustHide(t, got, "node_modules")
}

// TestFileStructure_NegationSurvivesACustomConfig pins the two sources
// interacting: the negation comes from .gitignore, the extra pattern from
// config, and both must land. Drop either source from the merge and one
// half of this goes red.
func TestFileStructure_NegationSurvivesACustomConfig(t *testing.T) {
	root := namedRepo(t, map[string]string{
		"dist/bundle.js":      "d",
		"node_modules/lib.js": "n",
		"scratch.tmp":         "s",
		"docs/readme.md":      "r",
		".gitignore":          "!dist\n",
	})
	writeIgnoreConfig(t, root, "*.tmp")

	got := mustGenerate(t, root)
	mustShow(t, got, "dist", "bundle.js")
	mustHide(t, got, "node_modules", "scratch.tmp")
	mustShow(t, got, "docs")
}

// TestFileStructure_NoConfigHidesEveryDefault is the byte-parity guard for
// the fleet: docs/file-structure.md sits behind a byte comparison in CI, so
// a repository with no config and no .gitignore must render exactly what it
// rendered before the merge landed. The fixture holds one entry per built-in
// default; only docs/ and keep.txt may survive.
func TestFileStructure_NoConfigHidesEveryDefault(t *testing.T) {
	root := namedRepo(t, map[string]string{
		"__pycache__/m.pyc":      "p",
		".git/HEAD":              "h",
		"node_modules/lib.js":    "n",
		"venv/pyvenv.cfg":        "v",
		".venv/pyvenv.cfg":       "v",
		"env/pyvenv.cfg":         "v",
		".env":                   "SECRET=1",
		"stale.pyc":              "p",
		".pytest_cache/CACHEDIR": "c",
		".mypy_cache/cache.json": "c",
		"dist/bundle.js":         "d",
		"build/out.o":            "o",
		"logmind.egg-info/PKG":   "e",
		".next/build-manifest":   "b",
		".turbo/cookies":         "c",
		".DS_Store":              "d",
		"docs/readme.md":         "r",
		"keep.txt":               "k",
	})

	got := mustGenerate(t, root)
	const wantTree = "```\nmyrepo\n├── docs\n│   └── readme.md\n└── keep.txt\n```\n"
	if !strings.Contains(got, wantTree) {
		t.Errorf("rendered tree changed for a repo with no config:\n=== got ===\n%s\n=== want tree ===\n%s", got, wantTree)
	}
}
