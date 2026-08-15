// config_test.go — exercises `logmind config list/get/set` against
// in-tmpdir fixtures. Snapshot-driven: each test compares the
// command's stdout/stderr against expected strings. Parity gaps
// (yaml.v3 sequence indent vs PyYAML, etc.) are documented in
// config.go's file-level comment.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempCwd runs fn inside a fresh temp directory, restoring the
// caller's cwd on exit. Returns the temp dir path so tests can poke
// at created files after fn returns.
func withTempCwd(t *testing.T, fn func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	origin, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origin) })
	fn(dir)
	return dir
}

func TestConfigList_DefaultsMatchPython(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"config", "list"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
		// Spot-check the head of the output — full byte-identical
		// comparison would lock the test to PyYAML's specific sequence
		// indent (documented delta).
		got := out.String()
		mustContain(t, got, "git:\n")
		mustContain(t, got, "  auto_commit: true\n")
		mustContain(t, got, "  auto_push: true\n")
		mustContain(t, got, "  commit_message_template: 'logmind: {decision}'\n")
		mustContain(t, got, "decisions:\n")
		mustContain(t, got, "  branch_aware: true\n")
		mustContain(t, got, "agents:\n")
		mustContain(t, got, "  claude: true\n")
		mustContain(t, got, "  cursor: true\n")
		mustContain(t, got, "  copilot: false\n")
	})
}

func TestConfigGet_Boolean(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"config", "get", "git.auto_push"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got := strings.TrimSpace(out.String()); got != "True" {
			t.Errorf("get git.auto_push = %q; want True", got)
		}
	})
}

func TestConfigGet_Int(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"config", "get", "git.commit_line_threshold"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got := strings.TrimSpace(out.String()); got != "20" {
			t.Errorf("get git.commit_line_threshold = %q; want 20", got)
		}
	})
}

func TestConfigGet_MissingKeyExitsNonZero(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"config", "get", "does.not.exist"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		err := root.Execute()
		if err == nil {
			t.Fatalf("execute: want non-nil error (ErrSilent), got nil")
		}
		mustContain(t, errOut.String(), "Key 'does.not.exist' not found")
	})
}

func TestConfigSet_BooleanCoercion(t *testing.T) {
	withTempCwd(t, func(dir string) {
		root := NewRootCmd()
		root.SetArgs([]string{"config", "set", "git.auto_push", "false"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
		mustContain(t, out.String(), "Set git.auto_push = False")
		// File should exist + contain the new value.
		data, err := os.ReadFile(filepath.Join(dir, ".logmind", "config.yml"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		mustContain(t, string(data), "auto_push: false")
	})
}

func TestConfigSet_IntCoercion(t *testing.T) {
	withTempCwd(t, func(dir string) {
		root := NewRootCmd()
		root.SetArgs([]string{"config", "set", "git.commit_line_threshold", "42"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
		mustContain(t, out.String(), "Set git.commit_line_threshold = 42")
		data, err := os.ReadFile(filepath.Join(dir, ".logmind", "config.yml"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		mustContain(t, string(data), "commit_line_threshold: 42")
	})
}

func TestConfigSet_StringFallback(t *testing.T) {
	withTempCwd(t, func(dir string) {
		root := NewRootCmd()
		root.SetArgs([]string{"config", "set", "git.commit_message_template", "release: {decision}"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, ".logmind", "config.yml"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		mustContain(t, string(data), "release: {decision}")
	})
}

func TestConfigSet_PreservesUnrelatedKeys(t *testing.T) {
	withTempCwd(t, func(dir string) {
		// First set: auto_push=false
		runCommand(t, []string{"config", "set", "git.auto_push", "false"})
		// Second set: a new agent key
		runCommand(t, []string{"config", "set", "agents.claude", "false"})
		data, err := os.ReadFile(filepath.Join(dir, ".logmind", "config.yml"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		body := string(data)
		mustContain(t, body, "auto_push: false")
		mustContain(t, body, "claude: false")
		// Defaults should still be merged in.
		mustContain(t, body, "branch_aware: true")
		mustContain(t, body, "cursor: true")
	})
}

func TestCoerceConfigValue(t *testing.T) {
	cases := []struct {
		raw  string
		want any
	}{
		{"true", true},
		{"True", true},
		{"false", false},
		{"FALSE", false},
		{"42", int(42)},
		{"0", int(0)},
		{"3.14", float64(3.14)},
		{"hello", "hello"},
		{"1.2.3", "1.2.3"}, // multiple dots → not float per Python rule
		{"", ""},
	}
	for _, c := range cases {
		got := coerceConfigValue(c.raw)
		if got != c.want {
			t.Errorf("coerceConfigValue(%q) = %v (%T); want %v (%T)", c.raw, got, got, c.want, c.want)
		}
	}
}

// runCommand fires a root command with the given args and asserts no error.
func runCommand(t *testing.T, args []string) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(args)
	var sink bytes.Buffer
	root.SetOut(&sink)
	root.SetErr(&sink)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
}

func mustContain(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Errorf("output missing %q\n--- got ---\n%s", needle, body)
	}
}
