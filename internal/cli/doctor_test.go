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

	"github.com/thrillmade/logmind/internal/templates"
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

// TestDoctor_AbsentEnforcementGatesExitNonZero is the `logmind doctor`
// end of the B3 regression: an initialised repository whose §3.4/§6.2
// enforcement surfaces are gone must SAY so and must exit non-zero.
//
// Measured on the release candidate before the fix: `logmind init`, then
// `rm .git/hooks/commit-msg .claude/settings.json
// .github/workflows/check-decisions.yml`, then `logmind doctor` printed
// "Stack status: OK" and exited 0 — while the same repo with one stale
// template marker printed DRIFT and exited 1 (TestDoctor_DriftExitsNonZero
// above is that control, and it is what says doctor was never blind
// generally). SPEC §3.4: "Failing open MUST NOT be silent."
//
// The quiet receipt is asserted in the same test on purpose. `ok doctor
// overall=DRIFT drift=0` would report the failure and its own count as
// contradicting each other, and a caller that branches on drift= would
// read the repo as clean.
func TestDoctor_AbsentEnforcementGatesExitNonZero(t *testing.T) {
	// Isolate PATH so probePathResolution cannot supply the DRIFT verdict
	// from a stale host binary — the assertion below has to be about the
	// gates and nothing else.
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", t.TempDir())

	plant := func(t *testing.T, d string) {
		t.Helper()
		// `logmind init`'s sentinel: this repository was initialised.
		mustWriteUnder(t, d, ".logmind/config.yml", "git:\n  enforce_commits: true\n")
		// A git repository, so a commit can actually be made from it.
		if err := os.MkdirAll(filepath.Join(d, ".git", "hooks"), 0o755); err != nil {
			t.Fatalf("mkdir .git/hooks: %v", err)
		}
		// check-decisions.yml's SIBLING workflows, which is what says this
		// repository is on GitHub Actions at all — `logmind init
		// --github-actions=false` installs none of them and records that
		// choice nowhere, so the merge-gate row stays silent without them
		// (doctor.gateSurfaces). Deliberately NOT check-decisions.yml: that
		// is the gate this test is about, and it must be the missing one.
		for _, name := range []string{
			"regen-timeline.yml", "check-doc-links.yml", "logmind-self-update.yml",
		} {
			mustWriteUnder(t, d, ".github/workflows/"+name, templates.Workflow(name+".template"))
		}
	}

	withTempCwd(t, func(d string) {
		plant(t, d)
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		err := root.Execute()
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("doctor over three absent gates returned %v; want ErrSilent (exit 1)", err)
		}
		body := out.String()
		mustContain(t, body, "Stack status: DRIFT")
		mustContain(t, body, "Enforcement gates absent (3)")
		mustContain(t, body, ".git/hooks/commit-msg")
		mustContain(t, body, ".github/workflows/check-decisions.yml")
		mustContain(t, body, ".claude/settings.json")
		// The remedy AND the deliberate opt-out, or the reader is nagged
		// with no way to answer.
		mustContain(t, body, "logmind doctor --fix")
		mustContain(t, body, "git.enforce_commits: false")
	})

	withTempCwd(t, func(d string) {
		plant(t, d)
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--quiet", "--offline", "--exit-zero"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor --quiet: %v\n%s", err, errOut.String())
		}
		mustContain(t, out.String(), "overall=DRIFT drift=3")
	})

	// CONTROL: the same tree minus the initialised marker. Nothing was
	// installed there to lose, so doctor must stay quiet and exit 0 —
	// without this, a collector that reported unconditionally would pass
	// every assertion above.
	withTempCwd(t, func(d string) {
		if err := os.MkdirAll(filepath.Join(d, ".git", "hooks"), 0o755); err != nil {
			t.Fatalf("mkdir .git/hooks: %v", err)
		}
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor on an uninitialised directory: %v\n%s", err, errOut.String())
		}
		body := out.String()
		mustContain(t, body, "Stack status: OK")
		if strings.Contains(body, "Enforcement gates absent") {
			t.Errorf("a directory that never ran `logmind init` was nagged:\n%s", body)
		}
	})
}
