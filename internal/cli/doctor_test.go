// doctor_test.go — exercises the `logmind doctor` cobra command.
//
// Heavier unit-level coverage lives in internal/doctor/doctor_test.go
// — these tests verify the cobra→doctor wiring (flags, exit codes,
// JSON-vs-text output selection).
package cli

import (
	"bytes"
	"errors"
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
