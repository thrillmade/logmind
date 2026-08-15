package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTimelineCfg writes content to a temp config.yml and returns its path.
func writeTimelineCfg(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestTimelineCanonical_UnknownKeyIgnored: the `timeline.canonical` config key
// was REMOVED in v2.0.0 (main-canonical is now the sole, unconditional
// timeline). A repo whose .logmind/config.yml still carries it MUST load
// cleanly — yaml.v3 struct unmarshal drops the now-unknown key rather than
// erroring, so no code path fails on a legacy config.
func TestTimelineCanonical_UnknownKeyIgnored(t *testing.T) {
	for _, body := range []string{
		"timeline:\n  canonical: main-canonical\n",
		"timeline:\n  canonical: branch-divergent\n",
		"timeline:\n  canonical: whatever\n",
	} {
		if _, err := LoadPath(writeTimelineCfg(t, body)); err != nil {
			t.Errorf("LoadPath with legacy %q errored; want the unknown key ignored: %v", body, err)
		}
	}
}

func TestFileStructureRootLabel_DefaultAndRoundTrip(t *testing.T) {
	if got := DefaultConfig().FileStructure.RootLabel; got != "" {
		t.Errorf("default RootLabel = %q; want \"\" (basename behavior, byte-parity)", got)
	}
	cfg, _ := LoadPath(writeTimelineCfg(t, "file_structure:\n  root_label: my-repo\n"))
	if cfg.FileStructure.RootLabel != "my-repo" {
		t.Errorf("RootLabel = %q; want my-repo", cfg.FileStructure.RootLabel)
	}
	// Leafwise deep-merge: setting root_label must not disturb its sibling
	// key. IgnorePatterns is §1.4's CONFIG source and this config sets no
	// patterns, so the correct value is EMPTY — a non-empty one here would
	// mean sixteen built-in defaults had been smuggled into the config
	// source, which is exactly the mis-ranking #303 fixed.
	//
	// That the defaults still apply to the walk is a claim about rendered
	// output, so it is pinned where it is observable — see
	// TestFileStructure_RootLabelOnlyConfigKeepsEveryDefault in
	// internal/tree (internal/config cannot import internal/tree; tree
	// imports config).
	if len(cfg.FileStructure.IgnorePatterns) != 0 {
		t.Errorf("ignore_patterns = %q after setting only root_label; want empty — the built-in defaults are DefaultIgnorePatterns, not the config source", cfg.FileStructure.IgnorePatterns)
	}
}

// TestPinVersion_DefaultAndRoundTrip pins the MINOR fix (SPEC §1.2.1 /
// §3.7): pinVersion is a top-level (not nested) config key, default "" so
// self-update runs normally, and a set value round-trips through
// LoadPath into the typed Config the CLI layer reads.
func TestPinVersion_DefaultAndRoundTrip(t *testing.T) {
	if got := DefaultConfig().PinVersion; got != "" {
		t.Errorf("default PinVersion = %q; want \"\" (self-update runs normally)", got)
	}
	cfg, err := LoadPath(writeTimelineCfg(t, "pinVersion: \"1.2.3\"\n"))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if cfg.PinVersion != "1.2.3" {
		t.Errorf("PinVersion = %q; want 1.2.3", cfg.PinVersion)
	}
	// A `pinVersion: null` config (the SPEC's own example) must resolve to
	// the same "" default, not error or leave a literal "null" string.
	nullCfg, err := LoadPath(writeTimelineCfg(t, "pinVersion: null\n"))
	if err != nil {
		t.Fatalf("LoadPath with pinVersion:null: %v", err)
	}
	if nullCfg.PinVersion != "" {
		t.Errorf("PinVersion with YAML null = %q; want \"\"", nullCfg.PinVersion)
	}
}

func TestDefaultMap_OmitsNewKeys_PreservesConfigListByteParity(t *testing.T) {
	// THE load-bearing guard for PR2: the new typed keys must NOT appear in
	// DefaultMap. DefaultMap is what `logmind config list` serializes, and
	// it round-trips byte-for-byte against the Python reference. Adding
	// `timeline`/`root_label` here would change that output — the silent
	// serialized-output change the plan forbids. Surfacing them in
	// config-list rides the coordinated v1.0 bump, not this slice.
	m := DefaultMap()
	if _, ok := m.Get("context"); ok {
		t.Errorf("DefaultMap contains `context` — breaks `config list` byte-parity")
	}
	// Explicit dotted-path checks (in addition to the whole-section check
	// above) so a future PR that adds a `context:` section for one reason
	// (e.g. surfacing `repomap`) can't silently smuggle `spec_file` in
	// alongside it without tripping this test.
	if _, ok := GetPath(m, "context.repomap"); ok {
		t.Errorf("DefaultMap exposes `context.repomap` — breaks `config list` byte-parity")
	}
	if _, ok := GetPath(m, "context.spec_file"); ok {
		t.Errorf("DefaultMap exposes `context.spec_file` — breaks `config list` byte-parity")
	}
	// derived_docs.mode / derived_docs.min_binary (the v2.0.0 B6 adoption-
	// signal gate, since removed entirely — the zero-conflict invariant is
	// now unconditional) never had a Python ancestor either — same rule as
	// enforce_commits / commit_line_threshold in GitConfig.
	if _, ok := m.Get("derived_docs"); ok {
		t.Errorf("DefaultMap contains `derived_docs` — breaks `config list` byte-parity")
	}
	if _, ok := GetPath(m, "derived_docs.mode"); ok {
		t.Errorf("DefaultMap exposes `derived_docs.mode` — breaks `config list` byte-parity")
	}
	if _, ok := GetPath(m, "derived_docs.min_binary"); ok {
		t.Errorf("DefaultMap exposes `derived_docs.min_binary` — breaks `config list` byte-parity")
	}
	fs, ok := m.Get("file_structure")
	if !ok {
		t.Fatal("file_structure missing from DefaultMap")
	}
	fsMap, ok := fs.(*OrderedMap)
	if !ok {
		t.Fatalf("file_structure is %T; want *OrderedMap", fs)
	}
	if _, ok := fsMap.Get("root_label"); ok {
		t.Errorf("DefaultMap file_structure contains `root_label` — breaks byte-parity")
	}
}
