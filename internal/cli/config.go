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
//   - `config get` prints booleans as `True`/`False` (Python's str()
//     output), ints as decimal integers, strings verbatim. Matches the
//     Python click.echo() default rendering of these types.
//   - `config set` echoes `Set k = v` in green; coerced value uses
//     Python's str() shape (True/False, not true/false).
//
// Error semantics:
//   - Missing key → red "Key '<k>' not found" to stderr, exit 1.
//   - I/O errors on save → bubble up via cobra; user sees default
//     cobra error formatting.
package cli

import (
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
			fmt.Fprintln(cmd.OutOrStdout(), formatValuePythonRepr(val))
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
			"Writes to .logmind/config.yml, creating it if missing.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, raw := args[0], args[1]
			repoRoot := "."
			merged, err := config.LoadAsMap(repoRoot)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", err)
			}
			parsed := coerceConfigValue(raw)
			config.SetPath(merged, key, parsed)
			if err := config.SaveMap(configPath(repoRoot), merged); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, formatValuePythonRepr(parsed))
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

// formatValuePythonRepr renders v the way Python's str() does for the
// CLI output paths (`config get <k>` and `Set <k> = <v>` echo). Bools
// become Title-case (True/False), nil is empty string, everything
// else falls through to fmt's default Go formatting.
func formatValuePythonRepr(v any) string {
	switch typed := v.(type) {
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case string:
		return typed
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}
