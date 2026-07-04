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

func TestTimelineCanonical_DefaultIsBranchDivergent(t *testing.T) {
	// An absent `timeline` key must yield the DefaultConfig default, with
	// main-canonical OFF.
	cfg, err := LoadPath(writeTimelineCfg(t, "git:\n  auto_commit: true\n"))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if cfg.Timeline.Canonical != "branch-divergent" {
		t.Errorf("Canonical = %q; want branch-divergent", cfg.Timeline.Canonical)
	}
	if cfg.Timeline.IsMainCanonical() {
		t.Errorf("IsMainCanonical() = true; want false on the default")
	}
	// A wholly missing file degrades to the same default.
	cfg2, _ := LoadPath(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if cfg2.Timeline.IsMainCanonical() {
		t.Errorf("missing-file default must be branch-divergent (main-canonical OFF)")
	}
}

func TestTimelineCanonical_OptIn(t *testing.T) {
	cfg, err := LoadPath(writeTimelineCfg(t, "timeline:\n  canonical: main-canonical\n"))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if !cfg.Timeline.IsMainCanonical() {
		t.Errorf("IsMainCanonical() = false; want true for `canonical: main-canonical`")
	}
}

func TestTimelineCanonical_FailSafeOnUnknownValue(t *testing.T) {
	// A typo / case-variant / unrelated value must NOT enable main-canonical
	// — only the exact string qualifies. This is the guard against a silent
	// output flip from a fat-fingered config.
	for _, v := range []string{"maincanonical", "main_canonical", "MAIN-CANONICAL", "main-canonical ", "branch-divergent", "", "true"} {
		cfg, _ := LoadPath(writeTimelineCfg(t, "timeline:\n  canonical: \""+v+"\"\n"))
		if cfg.Timeline.IsMainCanonical() {
			t.Errorf("canonical=%q enabled main-canonical; want fail-safe OFF", v)
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
	if _, ok := m.Get("timeline"); ok {
		t.Errorf("DefaultMap contains `timeline` — breaks `config list` byte-parity")
	}
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
