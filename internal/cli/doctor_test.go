// doctor_test.go — exercises the `logmind doctor` cobra command.
//
// Heavier unit-level coverage lives in internal/doctor/doctor_test.go
// — these tests verify the cobra→doctor wiring (flags, exit codes,
// JSON-vs-text output selection).
package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctor_OfflineRendersTextByDefault(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline", "--exit-zero"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor: %v\n%s", err, errOut.String())
		}
		body := out.String()
		mustContain(t, body, "Stack status:")
		// Non-JSON output should not start with `{`.
		trimmed := strings.TrimSpace(body)
		if strings.HasPrefix(trimmed, "{") {
			t.Errorf("default mode emitted JSON; want text")
		}
	})
}

func TestDoctor_JSONOutput(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline", "--exit-zero", "--json"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor: %v\n%s", err, errOut.String())
		}
		body := strings.TrimSpace(out.String())
		if !strings.HasPrefix(body, "{") || !strings.HasSuffix(body, "}") {
			t.Errorf("--json should emit a JSON document; got %q", firstLineForTest(body))
		}
		mustContain(t, body, `"project_root"`)
		mustContain(t, body, `"tools"`)
	})
}

func TestDoctor_DriftExitsNonZero(t *testing.T) {
	withTempCwd(t, func(_ string) {
		// Plant a stale workflow so DRIFT is deterministic across
		// developer machines and CI runners.
		if err := os.MkdirAll(filepath.Join(".github", "workflows"), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "# logmind-template-version: v0-FAKE\n# rest\n"
		if err := os.WriteFile(filepath.Join(".github", "workflows", "regen-timeline.yml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		err := root.Execute()
		if err == nil {
			t.Fatalf("expected non-nil error on DRIFT; output=%s", out.String())
		}
		if !errors.Is(err, ErrSilent) {
			t.Errorf("err = %v; want ErrSilent", err)
		}
	})
}

func TestDoctor_ExitZeroSilencesDrift(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline", "--exit-zero"})
		var sink bytes.Buffer
		root.SetOut(&sink)
		root.SetErr(&sink)
		if err := root.Execute(); err != nil {
			t.Errorf("--exit-zero should suppress DRIFT exit; got %v", err)
		}
	})
}

func firstLineForTest(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// TestDoctor_SpecAdvisory_HumanTableAndJSON: the `logmind doctor` cobra
// wiring surfaces the H2 spec advisory in BOTH the human table and --json,
// and Overall stays OK (advisory, not drift) end-to-end through the command.
func TestDoctor_SpecAdvisory_HumanTableAndJSON(t *testing.T) {
	// Isolate PATH so the live probePathResolution probe finds NO `logmind`
	// (benign "missing") rather than a real, possibly-STALE host binary (e.g.
	// a pyenv `logmind 1.2.0` shim) that would flip Overall to DRIFT and break
	// the OK assertions below. Process-global, so it covers both sub-closures.
	// The #214 regex fix (now parsing the real `logmind <ver> (spec <ver>)`
	// line) is what makes the host binary visible to this probe at all.
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", t.TempDir())

	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: docs/spec.md\n")

		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline", "--exit-zero"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor: %v\n%s", err, errOut.String())
		}
		body := out.String()
		mustContain(t, body, "Canonical spec file")
		mustContain(t, body, "missing")
		mustContain(t, body, "Stack status: OK")
	})

	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: docs/spec.md\n")

		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline", "--exit-zero", "--json"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor --json: %v\n%s", err, errOut.String())
		}
		body := out.String()
		mustContain(t, body, `"spec_advisories"`)
		mustContain(t, body, "missing")
		mustContain(t, body, `"overall": "OK"`)
	})
}
