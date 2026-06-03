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
	// CatalogTarget is the default `<owner>/<repo>` slug `logmind skill
	// push` opens PRs against. Overrideable on the CLI via --catalog.
	// Per plan §"Skill suggestion cycle §4" + §"Skill catalog
	// architecture": skills live IN the consumer repo first; this is the
	// downstream catalog destination, never an inbound source.
	CatalogTarget string `yaml:"catalog_target"`
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
		// Default catalog target. The catalog is downstream of every
		// consumer repo — never the other way around (End State #5).
		CatalogTarget: "thrillmade/agent-skills",
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

// DefaultMap returns the same default values as DefaultConfig but
// encoded as the loose `map[string]any` shape `logmind config list/get/set`
// uses for dot-notation lookups. Mirrors Python's DEFAULT_CONFIG dict
// shape so the serialised output round-trips byte-for-byte against
// a Python-generated config file (modulo agent-section additions Go
// has to know about). Key insertion order is preserved by using
// yaml.MapSlice-like ordering via a small helper struct.
//
// Note: the Python defaults list 11 base agents. We keep parity by
// listing them in the same insertion order — yaml.v3's marshal on a
// plain map sorts alphabetically, which would change file content
// between Python and Go installs. The OrderedMap shape we expose
// avoids that.
func DefaultMap() *OrderedMap {
	root := NewOrderedMap()

	git := NewOrderedMap()
	git.Set("auto_commit", true)
	git.Set("auto_push", true)
	git.Set("commit_message_template", "logmind: {decision}")
	git.Set("auto_rebase", false)
	root.Set("git", git)

	decisions := NewOrderedMap()
	decisions.Set("max_recent", 20)
	decisions.Set("branch_aware", true)
	root.Set("decisions", decisions)

	fileStructure := NewOrderedMap()
	fileStructure.Set("auto_update", true)
	patterns := []any{
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
	}
	fileStructure.Set("ignore_patterns", patterns)
	root.Set("file_structure", fileStructure)

	agents := NewOrderedMap()
	for _, name := range []string{
		"claude", "cursor", "copilot", "windsurf", "aider",
		"continue", "cody", "zed", "amazonq", "cline", "codex",
	} {
		enabled := name == "claude" || name == "cursor"
		agents.Set(name, enabled)
	}
	root.Set("agents", agents)

	// catalog_target: where `logmind skill push` opens PRs. Listed last
	// because it's a single scalar — keeping the multi-line sections
	// grouped at the top reads better in `logmind config list`.
	root.Set("catalog_target", "thrillmade/agent-skills")

	return root
}

// LoadAsMap loads .logmind/config.yml under repoRoot into an
// OrderedMap shape with defaults merged in (user values override
// defaults leafwise). Falls back to defaults on missing file or
// parse error — matching the Python behaviour.
func LoadAsMap(repoRoot string) (*OrderedMap, error) {
	return LoadPathAsMap(filepath.Join(repoRoot, ".logmind", "config.yml"))
}

// LoadPathAsMap is the path-explicit equivalent of LoadAsMap.
func LoadPathAsMap(path string) (*OrderedMap, error) {
	merged := DefaultMap()
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return merged, nil
		}
		return merged, readErr
	}
	user := NewOrderedMap()
	if err := yaml.Unmarshal(data, user); err != nil {
		return DefaultMap(), err
	}
	deepUpdate(merged, user)
	return merged, nil
}

// SaveMap writes m as YAML to path (creating parent dirs), using
// 2-space indentation to match Python's yaml.dump(default_flow_style=
// False) output. Caller's responsibility to ensure m is an OrderedMap
// the YAML encoder can serialise.
//
// Atomic write semantics: encode to a sibling temp file then rename
// over the destination. Without this, an encode/close error after the
// destination was already truncated would leave the user with an empty
// config.yml — silently losing every setting they had. The rename is
// atomic on POSIX so concurrent readers either see the old file or
// the new file, never a half-written intermediate state.
func SaveMap(path string, m *OrderedMap) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Belt + suspenders cleanup: on any error path below, remove the
	// orphan temp file. The `tmp = nil` after the successful rename
	// disarms this so we don't accidentally wipe the (now-renamed)
	// destination.
	cleanup := func() {
		if tmp != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}
	defer cleanup()

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	tmp = nil // disarm the deferred cleanup
	return os.Rename(tmpName, path)
}

// deepUpdate recursively folds src into dst — matches Python
// Config._deep_update (core/config.py:115-127). If both src and dst
// have a sub-map at the same key, descend; otherwise overwrite.
func deepUpdate(dst, src *OrderedMap) {
	for _, key := range src.Keys() {
		srcVal, _ := src.Get(key)
		if srcMap, ok := srcVal.(*OrderedMap); ok {
			if existing, ok := dst.Get(key); ok {
				if dstMap, ok := existing.(*OrderedMap); ok {
					deepUpdate(dstMap, srcMap)
					continue
				}
			}
		}
		dst.Set(key, srcVal)
	}
}

// GetPath walks the dot-separated path through m and returns the leaf
// value. Returns (nil, false) when any segment is missing or hits a
// non-map. Mirrors Python Config.get.
func GetPath(m *OrderedMap, dotted string) (any, bool) {
	keys := splitDotted(dotted)
	var current any = m
	for _, key := range keys {
		om, ok := current.(*OrderedMap)
		if !ok {
			return nil, false
		}
		v, ok := om.Get(key)
		if !ok {
			return nil, false
		}
		current = v
	}
	return current, true
}

// SetPath sets the leaf at dotted path to value, creating intermediate
// OrderedMaps as needed. Mirrors Python Config.set.
func SetPath(m *OrderedMap, dotted string, value any) {
	keys := splitDotted(dotted)
	if len(keys) == 0 {
		return
	}
	current := m
	for i, key := range keys {
		if i == len(keys)-1 {
			current.Set(key, value)
			return
		}
		existing, ok := current.Get(key)
		if !ok {
			next := NewOrderedMap()
			current.Set(key, next)
			current = next
			continue
		}
		next, ok := existing.(*OrderedMap)
		if !ok {
			// Existing leaf can't be descended; replace with map.
			next = NewOrderedMap()
			current.Set(key, next)
		}
		current = next
	}
}

func splitDotted(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
