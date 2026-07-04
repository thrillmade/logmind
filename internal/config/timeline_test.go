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
	// Leafwise deep-merge: setting root_label must NOT wipe the default
	// ignore_patterns (regression guard for the typed-config merge).
	if len(cfg.FileStructure.IgnorePatterns) == 0 {
		t.Errorf("ignore_patterns lost when only root_label was set")
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
