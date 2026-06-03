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
	if c.Decisions.MaxRecent != 20 {
		t.Errorf("Decisions.MaxRecent = %d; want 20", c.Decisions.MaxRecent)
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
	if cfg.Decisions.MaxRecent != 20 {
		t.Errorf("Decisions.MaxRecent = %d; want default 20", cfg.Decisions.MaxRecent)
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
