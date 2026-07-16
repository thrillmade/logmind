// init_spec_test.go — exercises `logmind init --spec` (H2 of the
// canonical-spec-file feature): docs/spec.md scaffolding + context.spec_file
// wiring, in both fresh-install and refresh mode, and idempotency of both.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/config"
)

// TestInitSpec_FreshInstall_CreatesFileAndSetsConfig: a fresh `init --spec`
// scaffolds docs/spec.md from the template and sets context.spec_file in
// .logmind/config.yml.
func TestInitSpec_FreshInstall_CreatesFileAndSetsConfig(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git", "--spec"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("init --spec: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
		}
		mustContain(t, out.String(), "✓ Created docs/spec.md")
		mustContain(t, out.String(), "✓ Set context.spec_file: docs/spec.md in .logmind/config.yml")
	})

	body, err := os.ReadFile(filepath.Join(dir, "docs", "spec.md"))
	if err != nil {
		t.Fatalf("read docs/spec.md: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"<!--", "-->",
		"# <Project> — Spec",
		"**Status:** Draft",
		"## What this project is building toward",
		"## Current contract",
		"## Open questions / not yet decided",
		"## Non-goals",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("docs/spec.md missing %q:\n%s", want, text)
		}
	}

	merged, err := config.LoadAsMap(dir)
	if err != nil {
		t.Fatalf("LoadAsMap: %v", err)
	}
	got, ok := config.GetPath(merged, "context.spec_file")
	if !ok || got != "docs/spec.md" {
		t.Errorf("context.spec_file = %v (ok=%v); want docs/spec.md", got, ok)
	}
}

// TestInitSpec_WithoutFlag_NoSpecArtifacts: plain `init` (no --spec) leaves
// docs/spec.md unwritten and context.spec_file unset — the flag is opt-in.
func TestInitSpec_WithoutFlag_NoSpecArtifacts(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		runQuiet(t, []string{"init", "--no-git"})
	})
	if _, err := os.Stat(filepath.Join(dir, "docs", "spec.md")); err == nil {
		t.Errorf("docs/spec.md should not exist without --spec")
	}
	merged, err := config.LoadAsMap(dir)
	if err != nil {
		t.Fatalf("LoadAsMap: %v", err)
	}
	if _, ok := config.GetPath(merged, "context.spec_file"); ok {
		t.Errorf("context.spec_file should not be set without --spec")
	}
}

// TestInitSpec_Idempotent_SecondRunTouchesNothing: running `init --spec`
// again after hand-editing docs/spec.md must not overwrite it, and must not
// disturb an already-set context.spec_file (even a customised one).
func TestInitSpec_Idempotent_SecondRunTouchesNothing(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		runQuiet(t, []string{"init", "--no-git", "--spec"})
		// Hand-edit the scaffolded spec — a second --spec run must preserve it.
		specPath := filepath.Join(".", "docs", "spec.md")
		if err := os.WriteFile(specPath, []byte("# My Project — Spec\n\nHand-written content.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git", "--spec"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("second init --spec: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
		}
		// Second pass must NOT report re-creating the file or re-setting config.
		if strings.Contains(out.String(), "✓ Created docs/spec.md") {
			t.Errorf("second --spec run re-created docs/spec.md; want idempotent no-op:\n%s", out.String())
		}
		if strings.Contains(out.String(), "✓ Set context.spec_file") {
			t.Errorf("second --spec run re-set context.spec_file; want idempotent no-op:\n%s", out.String())
		}
	})
	body, err := os.ReadFile(filepath.Join(dir, "docs", "spec.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Hand-written content.") {
		t.Errorf("second --spec run overwrote hand-edited docs/spec.md:\n%s", body)
	}
}

// TestInitSpec_DoesNotOverrideExplicitSpecFile: if the user already pointed
// context.spec_file at a DIFFERENT path, `--spec` must not clobber that
// choice, even though docs/spec.md itself may still get scaffolded (it's
// independently "only if absent").
func TestInitSpec_DoesNotOverrideExplicitSpecFile(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		runQuiet(t, []string{"init", "--no-git"})
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: docs/custom-spec.md\n")
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git", "--spec"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("init --spec: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
		}
		if strings.Contains(out.String(), "✓ Set context.spec_file") {
			t.Errorf("--spec overrode an already-set context.spec_file:\n%s", out.String())
		}
	})
	merged, err := config.LoadAsMap(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := config.GetPath(merged, "context.spec_file")
	if got != "docs/custom-spec.md" {
		t.Errorf("context.spec_file = %v; want the user's custom docs/custom-spec.md preserved", got)
	}
}

// TestInitSpec_RefreshMode_WorksAfterAlreadyInitialized: --spec must work
// when applied to an ALREADY-initialized repo (refresh mode), not just fresh
// installs — a repo that ran `logmind init` before this feature existed can
// still opt in later.
func TestInitSpec_RefreshMode_WorksAfterAlreadyInitialized(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		// First init WITHOUT --spec.
		runQuiet(t, []string{"init", "--no-git"})
		if _, err := os.Stat("docs/spec.md"); err == nil {
			t.Fatalf("docs/spec.md should not exist after the plain first init")
		}
		// Second call is refresh mode (docs/ + .logmind/ already exist); adds --spec.
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git", "--spec"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("refresh init --spec: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
		}
		mustContain(t, out.String(), "logmind is already initialized — running in refresh mode.")
		mustContain(t, out.String(), "✓ Created docs/spec.md")
		mustContain(t, out.String(), "✓ Set context.spec_file: docs/spec.md in .logmind/config.yml")
	})
	if _, err := os.Stat(filepath.Join(dir, "docs", "spec.md")); err != nil {
		t.Errorf("expected docs/spec.md after refresh --spec: %v", err)
	}
	merged, err := config.LoadAsMap(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := config.GetPath(merged, "context.spec_file")
	if !ok || got != "docs/spec.md" {
		t.Errorf("context.spec_file = %v (ok=%v); want docs/spec.md after refresh --spec", got, ok)
	}
}

// TestInitSpec_RefreshMode_IdempotentSecondRun: refresh-mode --spec run
// twice in a row must be a no-op the second time (same idempotency contract
// as fresh-install).
func TestInitSpec_RefreshMode_IdempotentSecondRun(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		runQuiet(t, []string{"init", "--no-git"})           // fresh, no spec
		runQuiet(t, []string{"init", "--no-git", "--spec"}) // refresh, adds spec
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git", "--spec"}) // refresh again
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("third init --spec: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
		}
		if strings.Contains(out.String(), "✓ Created docs/spec.md") {
			t.Errorf("third --spec run re-created docs/spec.md:\n%s", out.String())
		}
		if strings.Contains(out.String(), "✓ Set context.spec_file") {
			t.Errorf("third --spec run re-set context.spec_file:\n%s", out.String())
		}
	})
	_ = dir
}
