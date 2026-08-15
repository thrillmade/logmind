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
// This is the case that killed positional resolution the first time, and it
// passes now for a structural reason rather than because negation cheats.
// The repository configures NOTHING. It used to still hand ResolveRules the
// sixteen built-in defaults as the CONFIG source, because DefaultConfig
// seeded FileStructure.IgnorePatterns — so that re-appended `dist` sat at
// position 3, AFTER the `!dist` here at position 2, and a positional
// resolver correctly-but-uselessly let it win. The defaults are now their
// own source (config.DefaultIgnorePatterns, seeded first by ResolveRules)
// and the config source is empty for a repository that configured nothing,
// so `!dist` is the last rule matching dist and nothing follows it.
//
// Change ResolveRules to seed from config.DefaultConfig().FileStructure
// .IgnorePatterns again and this goes red.
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

// TestFileStructure_ConfigReExcludesWhatGitignoreNegated pins the capability
// positional resolution buys, and that negation-wins resolution could not
// express at all: config is §1.4's LAST source, so a repository whose
// .gitignore re-includes dist/ can still keep it off its own map by naming it
// in file_structure.ignore_patterns.
//
// Under the negation-wins rule this PR replaced, the `!dist` won from
// wherever it sat and this was unreachable — there was no way to overrule a
// .gitignore negation from config.
func TestFileStructure_ConfigReExcludesWhatGitignoreNegated(t *testing.T) {
	root := namedRepo(t, map[string]string{
		"dist/bundle.js": "d",
		"docs/readme.md": "r",
		".gitignore":     "!dist\n",
	})
	writeIgnoreConfig(t, root, "dist")

	got := mustGenerate(t, root)
	mustHide(t, got, "dist", "bundle.js")
	mustShow(t, got, "docs", "readme.md")
}

// TestFileStructure_RootLabelOnlyConfigKeepsEveryDefault is the rendered-
// output half of internal/config's TestFileStructureRootLabel_DefaultAndRoundTrip.
//
// A config that sets only root_label must not cost the repository the
// built-in defaults. That used to be checked by asserting the typed config's
// IgnorePatterns was non-empty — but the defaults no longer live there
// (they are config.DefaultIgnorePatterns, seeded by ResolveRules), so the
// claim is only observable here, on the document a user actually reads.
// internal/config cannot make it: tree imports config, not the reverse.
func TestFileStructure_RootLabelOnlyConfigKeepsEveryDefault(t *testing.T) {
	root := namedRepo(t, map[string]string{
		"node_modules/lib.js": "n",
		"dist/bundle.js":      "d",
		".venv/pyvenv.cfg":    "v",
		"docs/readme.md":      "r",
	})
	dir := filepath.Join(root, ".logmind")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("file_structure:\n  root_label: myrepo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := mustGenerate(t, root)
	mustHide(t, got, "node_modules", "dist", ".venv")
	mustShow(t, got, "docs", "readme.md")
}

// TestResolveRules_ExtrasApplyAndRankLast covers ResolveRules' variadic
// `extras`, the ad-hoc fourth source kept for parity with Python's
// `extra_ignore` on generate_tree.
//
// It had no test and no production caller, so dropping it entirely from the
// merge went unnoticed — found by mutating the append away and watching the
// suite stay green. Position matters now, so this pins both halves: an extra
// ignores, and an extra ranks AFTER config, so it can re-exclude what a
// config `!pattern` re-included.
func TestResolveRules_ExtrasApplyAndRankLast(t *testing.T) {
	root := namedRepo(t, map[string]string{
		"vendored/lib.js": "v",
		"dist/bundle.js":  "d",
		"docs/readme.md":  "r",
	})

	// An extra excludes a path no other source mentions.
	rules, err := ResolveRules(root, nil, "vendored")
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderWithLabel(root, rules, -1, "myrepo")
	if err != nil {
		t.Fatal(err)
	}
	mustHide(t, got, "vendored", "lib.js")
	mustShow(t, got, "docs", "readme.md")

	// Config re-includes dist; the extra, ranking after it, takes it back.
	rules, err = ResolveRules(root, []string{"!dist"}, "dist")
	if err != nil {
		t.Fatal(err)
	}
	got, err = RenderWithLabel(root, rules, -1, "myrepo")
	if err != nil {
		t.Fatal(err)
	}
	mustHide(t, got, "dist", "bundle.js")

	// Control: without the extra, the config negation stands — so the
	// assertion above is about the extra's POSITION, not about dist being
	// hidden by the built-in default all along.
	rules, err = ResolveRules(root, []string{"!dist"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = RenderWithLabel(root, rules, -1, "myrepo")
	if err != nil {
		t.Fatal(err)
	}
	mustShow(t, got, "dist", "bundle.js")
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
