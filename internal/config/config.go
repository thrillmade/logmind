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
//     returns the defaults — matching the Python “except Exception:
//     return defaults“ branch in core/config.py:104-110. Logmind treats
//     a broken config as user fixing it later; we never abort a derived-
//     doc command just because YAML failed.
//
//   - Deep merge: user keys override defaults LEAFWISE, not whole-section.
//     If a user only sets “file_structure.auto_update: false“, the
//     “ignore_patterns“ list still comes from defaults. Matches Python
//     _deep_update (core/config.py:115-127).
//
//   - This package never WRITES config — that's `logmind config set`
//     territory (B5). B3 only reads.
package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/thrillmade/logmind/internal/atomicio"
)

// Config is the parsed contents of .logmind/config.yml with defaults
// merged in. Only the subsections B3 actually reads are typed —
// extending this struct for B4+B5+B6 is purely additive.
type Config struct {
	Git           GitConfig           `yaml:"git"`
	Decisions     DecisionsConfig     `yaml:"decisions"`
	FileStructure FileStructureConfig `yaml:"file_structure"`
	// Context gates what `logmind context` folds into the cold-start payload
	// (token-killer). Additive: the default reproduces today's payload.
	Context ContextConfig   `yaml:"context"`
	Agents  map[string]bool `yaml:"agents"`
	// CatalogTarget is the default `<owner>/<repo>` slug `logmind skill
	// push` opens PRs against. Overrideable on the CLI via --catalog.
	// Per plan §"Skill suggestion cycle §4" + §"Skill catalog
	// architecture": skills live IN the consumer repo first; this is the
	// downstream catalog destination, never an inbound source.
	CatalogTarget string `yaml:"catalog_target"`

	// PrivacyScanner is the §8.2 wave-2 layer-3 (content-scanner)
	// config. Empty by default — the scanner's hardcoded baseline
	// (credential prefixes + canonical internal-process keywords)
	// fires regardless; config purely WIDENS the deny set. See
	// `internal/skill/scanner.go` for the baseline definitions.
	PrivacyScanner PrivacyScannerConfig `yaml:"privacy_scanner"`

	// AllowPromoteFromPrivate is the §8.2 wave-2 layer-4 opt-out flag.
	// When false (default), pushing a skill from a private source repo
	// to a public catalog is rejected. When true, the cross-visibility
	// check records the visibility shape but doesn't block; layers
	// 1-3 still run unchanged.
	AllowPromoteFromPrivate bool `yaml:"allow_promote_from_private"`

	// PinVersion is the SPEC §1.2.1 / §3.7 self-update floor: when set to
	// a non-empty version string, `logmind self-update` MUST no-op instead
	// of refreshing templates/hooks to the running binary's version. A
	// top-level key (not nested under a section), camelCase per the SPEC's
	// own naming — matches how the SPEC's example config.yml renders it
	// (`pinVersion: null`). Default "" (unset) = self-update runs normally.
	PinVersion string `yaml:"pinVersion"`
}

// PrivacyScannerConfig is the typed shape of the `privacy_scanner:`
// section. Mirrors `skill.ScannerConfig` — kept as a separate type
// here so the YAML tags + the zero-value defaults stay in the config
// package while the scanner code keeps a clean interface free of
// YAML tag noise.
type PrivacyScannerConfig struct {
	// Keywords is an additive list of substrings. Each one is matched
	// case-insensitively against SKILL.md body text. Default severity
	// is "block" (same as the baseline keyword category). Merge with
	// the hardcoded baseline — config can WIDEN, never weaken.
	Keywords []string `yaml:"keywords"`

	// OrgDomains lists internal-domain TLD-bearing strings (e.g.
	// "thrillmade.internal", "thrillmade.local"). Each entry is
	// wrapped in a regex matching `<host>.<domain>` references in
	// the body. Default severity is "warn" — promote via
	// SeverityOverrides to widen.
	OrgDomains []string `yaml:"org_domains"`

	// SeverityOverrides maps category Kind ("credential", "keyword",
	// "org-domain", "local-path") to severity ("block" or "warn").
	// Used to WIDEN baseline-warn categories. Attempting to weaken
	// baseline-block categories (credential, keyword) is silently
	// ignored — the baseline stays in force.
	SeverityOverrides map[string]string `yaml:"severity_overrides"`
}

// GitConfig mirrors the `git:` section.
type GitConfig struct {
	AutoCommit            bool   `yaml:"auto_commit"`
	AutoPush              bool   `yaml:"auto_push"`
	AutoRebase            bool   `yaml:"auto_rebase"`
	CommitMessageTemplate string `yaml:"commit_message_template"`
	// EnforceCommits gates the v2.0 guard-commit decision engine (see
	// internal/guardcommit + `logmind guard-commit`). Default true:
	// substantive commits are steered through `logmind log` unless a
	// carve-out applies. Set to false as a full repo off-ramp — when
	// false, guard-commit allows unconditionally (exit 0) regardless
	// of layer or change size.
	EnforceCommits bool `yaml:"enforce_commits"`
	// CommitLineThreshold overrides guard-commit's substantive-change
	// line threshold (how many changed lines trigger the "log this
	// decision" gate). A `--threshold` flag passed explicitly to
	// `logmind guard-commit` wins over this; this value wins over the
	// hardcoded fallback of 20.
	CommitLineThreshold int `yaml:"commit_line_threshold"`
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
	// RootLabel overrides the file-structure tree's root line. Default ""
	// = the checkout directory's basename (today's behavior); a fixed
	// string makes file-structure.md deterministic across checkouts /
	// worktrees, and "auto" resolves to the git remote repo name (wired in
	// Slice 2 PR5). Additive: "" preserves byte-parity.
	RootLabel string `yaml:"root_label"`
}

// ContextConfig mirrors the `context:` section — knobs for the `logmind
// context` cold-start payload (token-killer Phase 2).
type ContextConfig struct {
	// Repomap folds the Go signature skeleton (see internal/repomap) into the
	// context payload as a third <document>, inserted stable-first (after the
	// file tree, before the volatile timeline). Default false preserves the
	// current two-doc payload byte-for-byte; the v1.0 flip turns it on.
	Repomap bool `yaml:"repomap"`

	// SpecFile designates a repo-relative path to the project's canonical,
	// forward-looking spec — a hand-authored doc distinct from the derived
	// file-structure ("what") and timeline ("why"): it's the "where this is
	// headed" doc, edited via normal PRs and never regenerated. When set (and
	// the resolved file exists and is non-empty), `logmind context` folds it
	// in as a <document type="spec">, first in the payload — it's the most
	// stable doc, so it makes the best cache prefix. Default "" preserves the
	// existing payload byte-for-byte. See ResolveSpecFile for the path-safety
	// rule: an absolute path, or one that escapes the repo root, is UNSET.
	SpecFile string `yaml:"spec_file"`
}

// ResolveSpecFile resolves cfg.Context.SpecFile against repoRoot per the
// canonical-spec-file path rule (NORMATIVE):
//
//   - "" (unset) → ("", false).
//   - An absolute path → treated as UNSET. We never honor an absolute
//     spec_file, even one that happens to live inside repoRoot — the config
//     value is documented as repo-relative, and accepting an absolute path
//     would invite a config that "looks relative" on one machine and points
//     somewhere else entirely on another.
//   - A relative path that, after filepath.Join(repoRoot, rel) + Clean,
//     resolves OUTSIDE repoRoot (e.g. "../evil.md") → treated as UNSET.
//
// Both "unset" reasons collapse to the same ("", false) result: a
// misconfigured or hostile spec_file must never cause a read outside
// repoRoot, and must degrade silently (no error, no partial read) rather
// than surface as a gate failure — `logmind context` is a convenience.
func ResolveSpecFile(repoRoot string, cfg Config) (path string, ok bool) {
	rel := cfg.Context.SpecFile
	if rel == "" {
		return "", false
	}
	if filepath.IsAbs(rel) {
		return "", false
	}
	root := filepath.Clean(repoRoot)
	joined := filepath.Join(root, rel) // filepath.Join already Cleans the result.
	relToRoot, err := filepath.Rel(root, joined)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", false
	}
	return joined, true
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
			EnforceCommits:        true,
			CommitLineThreshold:   20,
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
				// SPEC §1.2.1's default list MUST also include these three
				// (.next/, .turbo/, .DS_Store) — added without the SPEC
				// prose's trailing "/" because internal/tree's matcher
				// (patternSetMatches) does a literal filepath.Match against
				// the basename/path component; every OTHER default pattern
				// here is already written without a trailing slash for the
				// same reason (a literal ".next/" would never match the
				// directory component ".next" and so would silently ignore
				// nothing).
				".next",
				".turbo",
				".DS_Store",
			},
			RootLabel: "",
		},
		// Context.Repomap default false → `logmind context` emits today's
		// two-doc payload byte-for-byte. NOT added to DefaultMap (below); the
		// v1.0 flip turns it on and surfaces it in `config list`.
		// Context.SpecFile default "" → the spec fold-in stays disabled and
		// the payload byte-for-byte unchanged. Also NOT added to DefaultMap —
		// `logmind init --spec` sets it explicitly via SetPath/SaveMap.
		Context: ContextConfig{
			Repomap:  false,
			SpecFile: "",
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
		// PrivacyScanner: zero value — empty lists, no severity
		// overrides. The scanner's hardcoded baseline still fires;
		// users opt INTO additional patterns via .logmind/config.yml.
		PrivacyScanner: PrivacyScannerConfig{},
		// AllowPromoteFromPrivate: false. Safe-default: a private
		// source → public catalog push gets blocked unless the user
		// explicitly opts in.
		AllowPromoteFromPrivate: false,
		// PinVersion: "" (unset) — `logmind self-update` runs normally.
		// Deliberately NOT added to DefaultMap (below): like root_label /
		// context.repomap / context.spec_file, it's a key with no Python
		// v0.6.14 ancestor, and DefaultMap must stay byte-for-byte
		// Python-parity for `config list` (see
		// TestDefaultMap_OmitsNewKeys_PreservesConfigListByteParity).
		// Reads/writes via config.yml (LoadPath/GetPath/`config get
		// pinVersion`) work regardless of listing.
		PinVersion: "",
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
	// SPEC §1.2.1 documents both keys with these defaults and places them
	// here, after auto_rebase, in the section's example config. They are
	// parsed and honored (guard_commit.go), but were missing from this map
	// — so `config get git.enforce_commits` reported "not found" for a key
	// with a documented default. §1.2.1's MAY-omit-from-listing carve-out
	// covers context.repomap / file_structure.root_label / context.spec_file
	// / skill_suggest.*; these two are not in it, so they belong here.
	git.Set("enforce_commits", true)
	git.Set("commit_line_threshold", 20)
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
		".next",
		".turbo",
		".DS_Store",
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

	// privacy_scanner: §8.2 wave-2 layer-3 (content-scanner) config.
	// Empty defaults — the scanner's hardcoded baseline still fires
	// regardless. Users WIDEN the deny set via these keys; weakening
	// the baseline is impossible by design (see scanner.go contract).
	privacyScanner := NewOrderedMap()
	privacyScanner.Set("keywords", []any{})
	privacyScanner.Set("org_domains", []any{})
	privacyScanner.Set("severity_overrides", NewOrderedMap())
	root.Set("privacy_scanner", privacyScanner)

	// allow_promote_from_private: §8.2 wave-2 layer-4 opt-out. Safe
	// default is false — cross-visibility pushes (private source →
	// public catalog) get blocked unless the user explicitly opts in.
	root.Set("allow_promote_from_private", false)

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
// Atomic write semantics, via the shared internal/atomicio.WriteFile
// primitive: encode to an in-memory buffer, then write to a sibling
// temp file and rename over the destination. Without this, an
// encode error after the destination was already truncated would
// leave the user with an empty config.yml — silently losing every
// setting they had. The rename is atomic on POSIX so concurrent
// readers either see the old file or the new file, never a
// half-written intermediate state.
func SaveMap(path string, m *OrderedMap) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	// 0600: matches the permission this path has always written
	// (os.CreateTemp's mode, never explicitly widened) — unchanged by
	// the atomicio consolidation.
	return atomicio.WriteFile(path, buf.Bytes(), 0o600)
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
