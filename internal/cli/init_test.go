// init_test.go — exercises `logmind init` end-to-end against an empty
// tmpdir. Goal: verify that every file the Python init writes (a) gets
// created, (b) has the expected content shape, and (c) refresh-mode
// is idempotent.
//
// Full byte-identical comparison against Python output is documented
// in the wave B6 PR description as a known gap — the YAML indent
// delta in `config.yml` round-trips and the date-stamped first
// decision line both differ deterministically.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_CreatesDocsAndConfig(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("init execute: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
		}
		mustContain(t, out.String(), "✓ Created docs/")
		mustContain(t, out.String(), "✓ Created docs/decisions.md")
		mustContain(t, out.String(), "✓ Created docs/decisions-archive.md")
		mustContain(t, out.String(), "✓ Created docs/file-structure.md")
		mustContain(t, out.String(), "✓ Created docs/timeline.md")
		mustContain(t, out.String(), "✓ Created .logmind/config.yml")
		mustContain(t, out.String(), "logmind initialized successfully!")
	})

	// Verify file contents on disk.
	for _, rel := range []string{
		"docs/decisions.md", "docs/decisions-archive.md",
		"docs/file-structure.md", "docs/timeline.md",
		".logmind/config.yml", "AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected %s after init; %v", rel, err)
		}
	}
}

func TestInit_WritesWorkflowTemplates(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var sink bytes.Buffer
		root.SetOut(&sink)
		root.SetErr(&sink)
		if err := root.Execute(); err != nil {
			t.Fatalf("init: %v\n%s", err, sink.String())
		}
	})
	for _, name := range []string{
		"regen-timeline.yml",
		"check-doc-links.yml",
		"logmind-self-update.yml",
		"check-decisions.yml",
	} {
		p := filepath.Join(dir, ".github", "workflows", name)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("missing %s: %v", p, err)
			continue
		}
		body := string(data)
		// Every template carries a `# logmind-template-version: vN` marker.
		mustContain(t, body, "logmind-template-version")
		// Pin substitution should have happened — __LOGMIND_VERSION__ never lands on disk.
		if strings.Contains(body, "__LOGMIND_VERSION__") {
			t.Errorf("%s still contains __LOGMIND_VERSION__ placeholder", name)
		}
	}
}

func TestInit_GitignoreAndGitattributesBlocks(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		// Pre-create .gitignore so we exercise the append path.
		if err := os.WriteFile(".gitignore", []byte("# pre-existing\n*.tmp\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var sink bytes.Buffer
		root.SetOut(&sink)
		root.SetErr(&sink)
		if err := root.Execute(); err != nil {
			t.Fatalf("init: %v\n%s", err, sink.String())
		}
	})
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	body := string(gi)
	// Pre-existing content preserved.
	mustContain(t, body, "# pre-existing\n*.tmp\n")
	// Logmind block appended with sentinels.
	mustContain(t, body, "# >>> logmind >>>")
	mustContain(t, body, ".logmind/cache/")
	mustContain(t, body, "# <<< logmind <<<")
}

func TestInit_RefreshMode_LeavesDocsAlone(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		// First init.
		runQuiet(t, []string{"init", "--no-git"})
		// Stash a custom decision line so we can confirm refresh leaves it.
		dpath := "docs/decisions.md"
		body, _ := os.ReadFile(dpath)
		marker := "\n\n## 2099-12-31 — Sentinel decision\n"
		body = append(body, []byte(marker)...)
		if err := os.WriteFile(dpath, body, 0o644); err != nil {
			t.Fatal(err)
		}
		// Second init runs in refresh mode.
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("refresh init: %v\n%s", err, errOut.String())
		}
		mustContain(t, out.String(), "logmind is already initialized")
		mustContain(t, out.String(), "Done. docs/ and .logmind/ left untouched.")
	})
	after, err := os.ReadFile(filepath.Join(dir, "docs", "decisions.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "Sentinel decision") {
		t.Errorf("refresh nuked the sentinel decision; expected it preserved")
	}
}

func TestInit_AgentsFlagOverridesDefault(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git", "--agents", "claude"})
		var sink bytes.Buffer
		root.SetOut(&sink)
		root.SetErr(&sink)
		if err := root.Execute(); err != nil {
			t.Fatalf("init: %v\n%s", err, sink.String())
		}
	})
	// claude file should exist; cursor (default) should NOT because we
	// explicitly restricted to claude.
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Errorf("expected CLAUDE.md after --agents claude: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursorrules")); err == nil {
		t.Errorf("did NOT expect .cursorrules when --agents claude only")
	}
}

// runQuiet runs a root command silencing stdout/stderr; used to drive
// setup steps in multi-stage tests without polluting test logs.
func runQuiet(t *testing.T, args []string) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(args)
	var sink bytes.Buffer
	root.SetOut(&sink)
	root.SetErr(&sink)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v\n%s", args, err, sink.String())
	}
}
