// config.go — `logmind config list|get|set` subcommand tree.
//
// Wave B6: replaces Python cli.config_list / config_get / config_set.
// The Python CLI exposes three sub-commands under a `config` group:
//
//	logmind config list       — dump the merged config as YAML to stdout
//	logmind config get <key>  — print one leaf value via dot notation
//	logmind config set <k> <v> — type-coerce v and write to .logmind/config.yml
//
// Type-coercion rules (matched from Python):
//   - "true"/"false" (case-insensitive) → bool
//   - all-digits → int
//   - all-digits with exactly one decimal point → float
//   - everything else → string
//
// Output parity notes:
//   - `config list` emits 2-space YAML indent (matches Python's
//     yaml.dump default_flow_style=False sort_keys=False) but sequence
//     items are indented one extra level vs Python (yaml.v3 vs PyYAML
//     convention). Acceptable delta — both parse the same.
//   - `config get` and `config set`'s echo print the value the way the
//     FILE spells it — `true`/`false`, not Python's `True`/`False`, and
//     a section or a list as YAML rather than Go's `%v`. §1.6 requires
//     the non-interactive form to be scriptable; see formatConfigValue.
//
// SPEC §1.6 refusal:
//   - `config set` refuses an agent-initiated write that WEAKENS
//     git.enforce_commits / review.strict_mode / review.auto_fix. The
//     key table lives in internal/config (blocking.go); how a person is
//     told from an agent, and what defeats it, is config_blocking.go.
//
// Error semantics:
//   - Missing key → red "Key '<k>' not found" to stderr, exit 1.
//   - I/O errors on save → bubble up via cobra; user sees default
//     cobra error formatting.
package cli

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/thrillmade/logmind/internal/config"
)

// configPath returns the .logmind/config.yml path the CLI reads/writes.
// Centralised so the test harness can override repo root via Cobra's
// Dir flag in the future.
func configPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".logmind", "config.yml")
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and modify logmind configuration",
		Long: "View and modify the logmind configuration stored in\n" +
			".logmind/config.yml. The Python `logmind config` group is\n" +
			"preserved with three sub-commands: list, get, and set.",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newConfigListCmd())
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigSetCmd())
	return cmd
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show all configuration settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoRoot := "."
			merged, err := config.LoadAsMap(repoRoot)
			if err != nil {
				// Mirror Python's silent-degrade — log a warning but
				// still dump the defaults so the user can see what's
				// in effect rather than failing on a broken file.
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", err)
			}
			return dumpMap(cmd.OutOrStdout(), merged)
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value by key (dot notation)",
		Long: "Print one configuration value identified by dot-separated\n" +
			"keys (e.g. `git.auto_push`). Missing keys exit non-zero.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			repoRoot := "."
			merged, err := config.LoadAsMap(repoRoot)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", err)
			}
			val, ok := config.GetPath(merged, key)
			if !ok || val == nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Key '%s' not found\n", key)
				return ErrSilent
			}
			fmt.Fprintln(cmd.OutOrStdout(), formatConfigValue(val))
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: "Coerces the value: \"true\"/\"false\" become booleans,\n" +
			"all-digit strings become ints, all-digit-with-one-dot\n" +
			"strings become floats, everything else stays a string.\n" +
			"Writes to .logmind/config.yml, creating it if missing.\n" +
			"\n" +
			"Three settings are a person's to change, not an agent's\n" +
			"(SPEC §1.6): git.enforce_commits, review.strict_mode and\n" +
			"review.auto_fix. A write that WEAKENS one of them — turning\n" +
			"a gate off, or turning auto-fix on — is refused unless a\n" +
			"person retypes the key at a terminal. Turning a gate ON is\n" +
			"never refused, and every other setting is written as usual.\n" +
			"How logmind tells a person from an agent, and what defeats\n" +
			"that check, is documented in internal/cli/config_blocking.go.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, raw := args[0], args[1]
			repoRoot := "."
			parsed := coerceConfigValue(raw)
			// Before the load, before the write: a refused write must
			// leave the file exactly as it found it (and must not
			// CREATE one in a repo that had none).
			if err := guardBlockingWrite(cmd, key, parsed); err != nil {
				return err
			}
			merged, err := config.LoadAsMap(repoRoot)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", err)
			}
			config.SetPath(merged, key, parsed)
			if err := config.SaveMap(configPath(repoRoot), merged); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, formatConfigValue(parsed))
			return nil
		},
	}
}

// dumpMap writes m to w using 2-space YAML indent (matches Python's
// yaml.dump default options). Errors propagate to the caller.
func dumpMap(w io.Writer, m *config.OrderedMap) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return err
	}
	return enc.Close()
}

// coerceConfigValue maps a raw string from the CLI argv into a Go
// value matching Python's coercion rules (see file docstring).
func coerceConfigValue(raw string) any {
	lower := strings.ToLower(raw)
	switch lower {
	case "true":
		return true
	case "false":
		return false
	}
	if isAllDigits(raw) {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return int(n)
		}
	}
	if isPythonFloatShape(raw) {
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			return f
		}
	}
	return raw
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isPythonFloatShape mirrors Python's check:
//
//	value.replace(".", "", 1).isdigit() and value.count(".") == 1
//
// i.e. exactly one decimal point and the remaining characters are all
// digits. Excludes signed/exponent forms — same as Python.
func isPythonFloatShape(s string) bool {
	if strings.Count(s, ".") != 1 {
		return false
	}
	withoutDot := strings.Replace(s, ".", "", 1)
	return isAllDigits(withoutDot)
}

// formatConfigValue renders v the way the config FILE spells it, so
// `config get <k>` is comparable against .logmind/config.yml and against the
// documented default — SPEC §1.6 requires the non-interactive form to be
// scriptable, "the same command in a terminal and in a workflow", and a
// workflow doing `[ "$(logmind config get git.enforce_commits)" = "false" ]`
// takes the wrong branch on anything else.
//
// It replaces formatValuePythonRepr, which rendered through Python's str():
// bools came out `True`/`False` while the file held `false`, and everything
// non-scalar fell through to Go's %v — so `config get file_structure.
// ignore_patterns` printed `[a b c]` and `config get git` printed a struct
// pointer, `&{[auto_commit ...] map[...]}`. None of the three was parseable
// by anything.
//
//	bool    → true / false     — YAML, and what the file holds
//	int     → decimal          — unchanged
//	float   → %v               — unchanged
//	string  → verbatim         — a caller doing `$(logmind config get
//	                             git.commit_message_template)` wants the
//	                             string, not a YAML scalar to unquote
//	anything else (a section, a list) → YAML block at 2-space indent, the
//	                             same bytes `config list` prints for that
//	                             subtree
//
// `config set`'s echo uses this too, so what the command says it wrote and
// what the file holds cannot disagree.
func formatConfigValue(v any) string {
	switch typed := v.(type) {
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case string:
		return typed
	case nil:
		return ""
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprintf("%v", v)
	default:
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(v); err != nil {
			// Nothing better to fall back to, and a get should not fail
			// on a value the file already round-trips.
			return fmt.Sprintf("%v", v)
		}
		if err := enc.Close(); err != nil {
			return fmt.Sprintf("%v", v)
		}
		// The encoder ends every document with a newline; the caller
		// Fprintln's, so hand it back without one.
		return strings.TrimRight(buf.String(), "\n")
	}
}
