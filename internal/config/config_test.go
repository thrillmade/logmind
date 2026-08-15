package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if !c.Git.AutoCommit {
		t.Errorf("Git.AutoCommit default = false; want true")
	}
	if !c.Git.AutoPush {
		t.Errorf("Git.AutoPush default = false; want true")
	}
	if c.Git.AutoRebase {
		t.Errorf("Git.AutoRebase default = true; want false")
	}
	if c.Git.CommitMessageTemplate != "logmind: {decision}" {
		t.Errorf("Git.CommitMessageTemplate = %q", c.Git.CommitMessageTemplate)
	}
	if !c.Decisions.BranchAware {
		t.Errorf("Decisions.BranchAware = false; want true")
	}
	if !c.FileStructure.AutoUpdate {
		t.Errorf("FileStructure.AutoUpdate = false; want true")
	}
	wantPatterns := []string{
		"__pycache__", ".git", "node_modules", "venv", ".venv",
		"env", ".env", "*.pyc", ".pytest_cache", ".mypy_cache",
		"dist", "build", "*.egg-info",
		// SPEC §1.2.1 additions: .next/, .turbo/, .DS_Store.
		".next", ".turbo", ".DS_Store",
	}
	if !reflect.DeepEqual(c.FileStructure.IgnorePatterns, wantPatterns) {
		t.Errorf("FileStructure.IgnorePatterns =\n  %q\nwant\n  %q", c.FileStructure.IgnorePatterns, wantPatterns)
	}
	if !c.Agents["claude"] || !c.Agents["cursor"] {
		t.Errorf("Agents claude/cursor should be true by default: %v", c.Agents)
	}
	if c.Agents["copilot"] || c.Agents["windsurf"] {
		t.Errorf("Agents copilot/windsurf should be false: %v", c.Agents)
	}
	// CatalogTarget default — per plan §"Skill suggestion cycle §4" the
	// default catalog destination is thrillmade/agent-skills. Tests pin
	// this so accidental edits to DefaultConfig() can't redirect every
	// user's `logmind skill push` somewhere else.
	if c.CatalogTarget != "thrillmade/agent-skills" {
		t.Errorf("CatalogTarget default = %q; want thrillmade/agent-skills", c.CatalogTarget)
	}
}

// TestDefaultConfig_IgnorePatterns_IncludesSpecRequiredDefaults pins the
// MINOR fix: SPEC §1.2.1 states the default file_structure.ignore_patterns
// "MUST include at least" .git/, node_modules/, venv/, __pycache__/,
// .next/, dist/, build/, .turbo/, *.pyc, .DS_Store. .next/.turbo/.DS_Store
// were missing entirely. (Checked here without the SPEC prose's trailing
// "/" — see the doc comment on DefaultConfig's IgnorePatterns literal for
// why: internal/tree's matcher does a literal component match, and every
// sibling default pattern already omits the trailing slash for the same
// reason.)
func TestDefaultConfig_IgnorePatterns_IncludesSpecRequiredDefaults(t *testing.T) {
	got := DefaultConfig().FileStructure.IgnorePatterns
	set := map[string]bool{}
	for _, p := range got {
		set[p] = true
	}
	for _, want := range []string{".next", ".turbo", ".DS_Store"} {
		if !set[want] {
			t.Errorf("DefaultConfig().FileStructure.IgnorePatterns = %q; missing SPEC §1.2.1-required default %q", got, want)
		}
	}
}

// TestDefaultMap_IgnorePatterns_MatchesDefaultConfig guards against the
// two hand-maintained pattern lists (DefaultConfig's typed slice and
// DefaultMap's `config list`/get/set-facing []any slice) drifting apart —
// `logmind config list` must show the same ignore_patterns file-structure
// tree generation actually uses.
func TestDefaultMap_IgnorePatterns_MatchesDefaultConfig(t *testing.T) {
	typed := DefaultConfig().FileStructure.IgnorePatterns
	fileStructure, ok := DefaultMap().Get("file_structure")
	if !ok {
		t.Fatal("DefaultMap() has no file_structure key")
	}
	om, ok := fileStructure.(*OrderedMap)
	if !ok {
		t.Fatalf("file_structure value is %T; want *OrderedMap", fileStructure)
	}
	rawPatterns, ok := om.Get("ignore_patterns")
	if !ok {
		t.Fatal("DefaultMap() file_structure has no ignore_patterns key")
	}
	anyPatterns, ok := rawPatterns.([]any)
	if !ok {
		t.Fatalf("ignore_patterns value is %T; want []any", rawPatterns)
	}
	if len(anyPatterns) != len(typed) {
		t.Fatalf("DefaultMap ignore_patterns has %d entries; DefaultConfig has %d", len(anyPatterns), len(typed))
	}
	for i, p := range anyPatterns {
		s, ok := p.(string)
		if !ok {
			t.Fatalf("ignore_patterns[%d] = %T; want string", i, p)
		}
		if s != typed[i] {
			t.Errorf("DefaultMap ignore_patterns[%d] = %q; DefaultConfig has %q", i, s, typed[i])
		}
	}
}

// TestLoadUserOverride_CatalogTarget verifies a user-set catalog_target
// flows through Load() so `logmind skill push` picks it up without
// the --catalog flag.
func TestLoadUserOverride_CatalogTarget(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logmind")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "catalog_target: acme/private-skills\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CatalogTarget != "acme/private-skills" {
		t.Errorf("CatalogTarget = %q; want acme/private-skills", cfg.CatalogTarget)
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load missing config returned err: %v", err)
	}
	if !reflect.DeepEqual(cfg, DefaultConfig()) {
		t.Errorf("Load on missing file did not return defaults")
	}
}

func TestLoadUserOverride(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logmind")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `file_structure:
  auto_update: false
  ignore_patterns:
    - "*.log"
    - "secret/"
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FileStructure.AutoUpdate {
		t.Errorf("user override of auto_update=false did not take effect")
	}
	if !reflect.DeepEqual(cfg.FileStructure.IgnorePatterns, []string{"*.log", "secret/"}) {
		t.Errorf("ignore_patterns override = %q", cfg.FileStructure.IgnorePatterns)
	}
	if !cfg.Git.AutoCommit {
		t.Errorf("Git.AutoCommit should still be default true after user override")
	}
}

func TestLoadMalformedYaml(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logmind")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "git: not-a-mapping\n  - oops"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err == nil {
		t.Errorf("expected non-nil error on malformed yaml")
	}
	if !reflect.DeepEqual(cfg, DefaultConfig()) {
		t.Errorf("malformed config did not degrade to defaults")
	}
}

// TestDefaultConfig_PrivacyScannerEmpty pins the §8.2 wave-2 safe-defaults
// shape: empty Keywords / OrgDomains lists, nil SeverityOverrides map.
// The scanner's hardcoded baseline still fires regardless of these
// values — config purely WIDENS the deny set. If this test drifts, an
// accidental DefaultConfig() change has started seeding patterns from
// the config layer instead of the scanner package.
func TestDefaultConfig_PrivacyScannerEmpty(t *testing.T) {
	c := DefaultConfig()
	if len(c.PrivacyScanner.Keywords) != 0 {
		t.Errorf("PrivacyScanner.Keywords default = %v; want empty", c.PrivacyScanner.Keywords)
	}
	if len(c.PrivacyScanner.OrgDomains) != 0 {
		t.Errorf("PrivacyScanner.OrgDomains default = %v; want empty", c.PrivacyScanner.OrgDomains)
	}
	if c.PrivacyScanner.SeverityOverrides != nil {
		t.Errorf("PrivacyScanner.SeverityOverrides default = %v; want nil", c.PrivacyScanner.SeverityOverrides)
	}
}

// TestDefaultConfig_AllowPromoteFromPrivate_FalseByDefault pins the
// §8.2 wave-2 layer-4 safe default. A private source repo pushing to a
// public catalog gets blocked unless the user explicitly opts in by
// flipping this key. Drift on the default would silently disable the
// cross-visibility gate for every fresh install.
func TestDefaultConfig_AllowPromoteFromPrivate_FalseByDefault(t *testing.T) {
	c := DefaultConfig()
	if c.AllowPromoteFromPrivate {
		t.Errorf("AllowPromoteFromPrivate default = true; want false (safe default)")
	}
}

// TestLoadUserOverride_PrivacyScannerKeywords confirms a user-supplied
// keywords list flows through Load() so `logmind skill push` picks it
// up. Verifies the YAML deserialisation tag is wired correctly.
func TestLoadUserOverride_PrivacyScannerKeywords(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logmind")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "privacy_scanner:\n  keywords:\n    - acme-internal\n    - skunkworks\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"acme-internal", "skunkworks"}
	if !reflect.DeepEqual(cfg.PrivacyScanner.Keywords, want) {
		t.Errorf("PrivacyScanner.Keywords = %v; want %v", cfg.PrivacyScanner.Keywords, want)
	}
}

// TestLoadUserOverride_PrivacyScannerOrgDomains — same shape as the
// keywords override, but for the org_domains list. Org domains have NO
// baseline (every org's internal-domain shape is different); the user
// must opt in by listing them here.
func TestLoadUserOverride_PrivacyScannerOrgDomains(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logmind")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "privacy_scanner:\n  org_domains:\n    - acme.internal\n    - acme.local\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"acme.internal", "acme.local"}
	if !reflect.DeepEqual(cfg.PrivacyScanner.OrgDomains, want) {
		t.Errorf("PrivacyScanner.OrgDomains = %v; want %v", cfg.PrivacyScanner.OrgDomains, want)
	}
}

// TestLoadUserOverride_PrivacyScannerSeverityOverrides verifies the
// severity-overrides map round-trips. Note: this test only pins that
// the YAML map deserialises into Go; the "can't weaken baseline"
// contract is enforced by the scanner package (see scanner_test.go).
func TestLoadUserOverride_PrivacyScannerSeverityOverrides(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logmind")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "privacy_scanner:\n  severity_overrides:\n    local-path: block\n    org-domain: block\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]string{
		"local-path": "block",
		"org-domain": "block",
	}
	if !reflect.DeepEqual(cfg.PrivacyScanner.SeverityOverrides, want) {
		t.Errorf("PrivacyScanner.SeverityOverrides = %v; want %v",
			cfg.PrivacyScanner.SeverityOverrides, want)
	}
}

// TestLoadUserOverride_AllowPromoteFromPrivate — the §8.2 wave-2 layer-4
// opt-out flag flips from false to true when the user sets it. This is
// the only way to push a private-source skill to a public catalog.
func TestLoadUserOverride_AllowPromoteFromPrivate(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logmind")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "allow_promote_from_private: true\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AllowPromoteFromPrivate {
		t.Errorf("AllowPromoteFromPrivate = false; want true (user override)")
	}
}

// TestDefaultMap_PrivacyScannerSubkeysExist guards against the
// `logmind config get privacy_scanner.keywords` 404 case on a fresh
// install. The OrderedMap shape backs the config-list/config-get/config-set
// commands; if DefaultMap() drifts and forgets the wave-2 sections, the
// CLI surface for those keys silently disappears.
func TestDefaultMap_PrivacyScannerSubkeysExist(t *testing.T) {
	m := DefaultMap()
	for _, path := range []string{
		"privacy_scanner",
		"privacy_scanner.keywords",
		"privacy_scanner.org_domains",
		"privacy_scanner.severity_overrides",
		"allow_promote_from_private",
	} {
		if _, ok := GetPath(m, path); !ok {
			t.Errorf("DefaultMap missing key %q — fresh install would 404 on `logmind config get %s`",
				path, path)
		}
	}
}

// TestLoad_RetiredMaxRecentKeyIsIgnoredAndRoundTripped covers SPEC §1.6's
// lenient-read rule for the key SPEC §3.2 removed: "An unrecognised key MUST
// NOT cause a failure, and any tool that rewrites the file MUST round-trip
// that key unchanged."
//
// `decisions.max_recent` capped docs/decisions.md and rotated the overflow
// into docs/decisions-archive.md. Both are gone, and DecisionsConfig no longer
// names the key — so every repo in the fleet is carrying a config the schema
// does not describe until someone tidies it. Loading such a config must
// succeed, the keys that DO exist must still resolve, and a `config set`
// must not silently drop the stale key on rewrite.
func TestLoad_RetiredMaxRecentKeyIsIgnoredAndRoundTripped(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logmind")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "decisions:\n  max_recent: 20\n  branch_aware: false\ngit:\n  auto_push: false\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load errored on a config carrying the retired decisions.max_recent key: %v", err)
	}
	// The neighbouring keys must still have been applied — an "ignored"
	// unknown key that also discards its siblings is not lenient, it is
	// silently lossy.
	if cfg.Decisions.BranchAware {
		t.Errorf("Decisions.BranchAware = true; want the file's false — the sibling key was dropped alongside the unknown one")
	}
	if cfg.Git.AutoPush {
		t.Errorf("Git.AutoPush = true; want the file's false — a whole section was dropped")
	}

	// Round-trip: the map view a `config set` rewrites through must carry the
	// unrecognised key back out unchanged.
	m, err := LoadAsMap(dir)
	if err != nil {
		t.Fatalf("LoadAsMap errored on the retired key: %v", err)
	}
	decisions, ok := m.Get("decisions")
	if !ok {
		t.Fatalf("LoadAsMap dropped the decisions section entirely")
	}
	section, ok := decisions.(*OrderedMap)
	if !ok {
		t.Fatalf("decisions section is %T; want *OrderedMap", decisions)
	}
	if _, ok := section.Get("max_recent"); !ok {
		t.Errorf("LoadAsMap dropped decisions.max_recent — §1.6 requires an unrecognised key to round-trip unchanged")
	}
}
