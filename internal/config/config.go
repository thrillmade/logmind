// Package config loads .logmind/config.yml with built-in defaults.
//
// Mirrors src/logmind/core/config.py — the file shape, default-value
// constants, and deep-merge semantics all match Python v0.6.14 so the
// Go binary picks up user-edited config.yml without surprise.
//
// Design choices:
//
//   - Defaults are hard-coded in DefaultConfig(). When the user's
//     .logmind/config.yml is missing or unparseable, the loader silently
//     returns the defaults — matching the Python ``except Exception:
//     return defaults`` branch in core/config.py:104-110. Logmind treats
//     a broken config as user fixing it later; we never abort a derived-
//     doc command just because YAML failed.
//
//   - Deep merge: user keys override defaults LEAFWISE, not whole-section.
//     If a user only sets ``file_structure.auto_update: false``, the
//     ``ignore_patterns`` list still comes from defaults. Matches Python
//     _deep_update (core/config.py:115-127).
//
//   - This package never WRITES config — that's `logmind config set`
//     territory (B5). B3 only reads.
package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the parsed contents of .logmind/config.yml with defaults
// merged in. Only the subsections B3 actually reads are typed —
// extending this struct for B4+B5+B6 is purely additive.
type Config struct {
	Git           GitConfig           `yaml:"git"`
	Decisions     DecisionsConfig     `yaml:"decisions"`
	FileStructure FileStructureConfig `yaml:"file_structure"`
	Agents        map[string]bool     `yaml:"agents"`
}

// GitConfig mirrors the `git:` section.
type GitConfig struct {
	AutoCommit            bool   `yaml:"auto_commit"`
	AutoPush              bool   `yaml:"auto_push"`
	AutoRebase            bool   `yaml:"auto_rebase"`
	CommitMessageTemplate string `yaml:"commit_message_template"`
}

// DecisionsConfig mirrors the `decisions:` section.
type DecisionsConfig struct {
	MaxRecent   int  `yaml:"max_recent"`
	BranchAware bool `yaml:"branch_aware"`
}

// FileStructureConfig mirrors the `file_structure:` section.
type FileStructureConfig struct {
	AutoUpdate     bool     `yaml:"auto_update"`
	IgnorePatterns []string `yaml:"ignore_patterns"`
}

// DefaultConfig returns a fresh Config populated with the same values
// hard-coded into Python's DEFAULT_CONFIG (core/config.py:14-67).
func DefaultConfig() Config {
	return Config{
		Git: GitConfig{
			AutoCommit:            true,
			AutoPush:              true,
			AutoRebase:            false,
			CommitMessageTemplate: "logmind: {decision}",
		},
		Decisions: DecisionsConfig{
			MaxRecent:   20,
			BranchAware: true,
		},
		FileStructure: FileStructureConfig{
			AutoUpdate: true,
			IgnorePatterns: []string{
				"__pycache__",
				".git",
				"node_modules",
				"venv",
				".venv",
				"env",
				".env",
				"*.pyc",
				".pytest_cache",
				".mypy_cache",
				"dist",
				"build",
				"*.egg-info",
			},
		},
		Agents: map[string]bool{
			"claude":   true,
			"cursor":   true,
			"copilot":  false,
			"windsurf": false,
			"aider":    false,
			"continue": false,
			"cody":     false,
			"zed":      false,
			"amazonq":  false,
			"cline":    false,
			"codex":    false,
		},
	}
}

// Load reads .logmind/config.yml under repoRoot, merges it with the
// built-in defaults, and returns the result. A missing file silently
// degrades to defaults — matching Python's "no config = first run".
func Load(repoRoot string) (Config, error) {
	return LoadPath(filepath.Join(repoRoot, ".logmind", "config.yml"))
}

// LoadPath loads a config file from an arbitrary path. Used by tests
// to point at fixtures without faking the .logmind/ directory layout.
func LoadPath(path string) (Config, error) {
	cfg := DefaultConfig()

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, readErr
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), err
	}
	return cfg, nil
}
