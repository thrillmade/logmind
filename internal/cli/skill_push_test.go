// skill_push_test.go — exercises the `logmind skill push` CLI wiring.
//
// The core promotion mechanics (clone, copy, commit, gh pr create) are
// unit-tested via the injectable runner in internal/skill/push_test.go.
// These tests focus on the surface the user sees: flag parsing,
// config-target resolution, ErrSilent translation, and dry-run output.
package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/skill"
)

// initGitRepo runs the minimal git plumbing to make the repo at root
// look like a real source repo. We need:
//
//   - a working `git rev-parse HEAD` (commit exists)
//   - a remote.origin.url so resolveSourceRepoSlug returns a value
//
// Skipped on environments without git on PATH so CI without git stays
// clean (logmind doesn't require git to run, just to push).
func initGitRepo(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping integration-style test")
	}
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	runGit("init", "-q", "-b", "main")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")
	runGit("config", "commit.gpgsign", "false")
	runGit("remote", "add", "origin", "https://github.com/thrillmade/logmind.git")
	// Seed a commit so HEAD exists.
	dummy := filepath.Join(root, ".seed")
	if err := os.WriteFile(dummy, []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	runGit("add", ".seed")
	runGit("commit", "-q", "-m", "seed")
}

func TestSkillPush_RegisteredOnTree(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"skill", "push", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("skill push --help: %v", err)
	}
	s := out.String()
	for _, want := range []string{
		"Publish a local SKILL.md to a catalog repo",
		"--catalog",
		"--dry-run",
		// "downstream of every consumer" pins the direction-of-promotion
		// language — drift here would mean we accidentally documented
		// the wrong flow (catalog→local instead of local→catalog).
		"downstream of every consumer",
		"thrillmade/agent-skills",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("skill push --help missing %q:\n%s", want, s)
		}
	}
}

func TestSkillPush_DryRun_PrintsPlan(t *testing.T) {
	withTempCwd(t, func(dir string) {
		initGitRepo(t, dir)
		writeSampleSkillCLI(t, dir, "demo", "A precise trigger.")

		root := NewRootCmd()
		root.SetArgs([]string{"skill", "push", "demo", "--dry-run"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("skill push: %v", err)
		}
		s := out.String()
		for _, want := range []string{
			"→ Pushing skill 'demo' to thrillmade/agent-skills",
			"thrillmade/logmind @ ",
			"Dry-run: skipping clone, push, and PR creation.",
			"ok skill: push demo dry-run",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("dry-run output missing %q:\n%s", want, s)
			}
		}
	})
}

func TestSkillPush_CatalogFlagOverridesConfig(t *testing.T) {
	withTempCwd(t, func(dir string) {
		initGitRepo(t, dir)
		writeSampleSkillCLI(t, dir, "demo", "A trigger.")

		// Write a config that names a custom catalog.
		writeConfig(t, dir, "catalog_target: acme/private-skills\n")

		// --catalog flag wins over config.
		root := NewRootCmd()
		root.SetArgs([]string{"skill", "push", "demo", "--dry-run", "--catalog", "other/repo"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("skill push: %v", err)
		}
		if !strings.Contains(out.String(), "→ Pushing skill 'demo' to other/repo") {
			t.Errorf("--catalog override didn't apply:\n%s", out.String())
		}
	})
}

func TestSkillPush_ConfigCatalogTargetUsed(t *testing.T) {
	withTempCwd(t, func(dir string) {
		initGitRepo(t, dir)
		writeSampleSkillCLI(t, dir, "demo", "A trigger.")
		writeConfig(t, dir, "catalog_target: acme/private-skills\n")

		root := NewRootCmd()
		root.SetArgs([]string{"skill", "push", "demo", "--dry-run"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("skill push: %v", err)
		}
		if !strings.Contains(out.String(), "to acme/private-skills") {
			t.Errorf("config catalog_target not applied:\n%s", out.String())
		}
	})
}

func TestSkillPush_DefaultCatalogTarget(t *testing.T) {
	withTempCwd(t, func(dir string) {
		initGitRepo(t, dir)
		writeSampleSkillCLI(t, dir, "demo", "A trigger.")
		// No config file, no flag → default thrillmade/agent-skills.

		root := NewRootCmd()
		root.SetArgs([]string{"skill", "push", "demo", "--dry-run"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("skill push: %v", err)
		}
		if !strings.Contains(out.String(), "to thrillmade/agent-skills") {
			t.Errorf("default catalog target not applied:\n%s", out.String())
		}
	})
}

func TestSkillPush_MissingSkillReturnsErrSilent(t *testing.T) {
	withTempCwd(t, func(dir string) {
		var out bytes.Buffer
		err := runSkillPush(dir, "ghost", "", true, &out)
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("expected ErrSilent; got %v", err)
		}
		if !strings.Contains(out.String(), "Error: skill 'ghost' not found at") {
			t.Errorf("missing skill: stdout = %q", out.String())
		}
	})
}

func TestSkillPush_InvalidCatalogReturnsErrSilent(t *testing.T) {
	withTempCwd(t, func(dir string) {
		writeSampleSkillCLI(t, dir, "demo", "desc")
		var out bytes.Buffer
		err := runSkillPush(dir, "demo", "not-a-slug", true, &out)
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("expected ErrSilent; got %v", err)
		}
		if !strings.Contains(out.String(), "is not a valid <owner>/<repo>") {
			t.Errorf("invalid catalog: stdout = %q", out.String())
		}
	})
}

func TestResolveCatalogTarget_PrecedenceFlagWinsConfig(t *testing.T) {
	withTempCwd(t, func(dir string) {
		writeConfig(t, dir, "catalog_target: from-config/repo\n")
		got := resolveCatalogTarget(dir, "from-flag/repo")
		if got != "from-flag/repo" {
			t.Errorf("flag should win; got %q", got)
		}
	})
}

func TestResolveCatalogTarget_ConfigWinsWhenNoFlag(t *testing.T) {
	withTempCwd(t, func(dir string) {
		writeConfig(t, dir, "catalog_target: from-config/repo\n")
		got := resolveCatalogTarget(dir, "")
		if got != "from-config/repo" {
			t.Errorf("config should apply; got %q", got)
		}
	})
}

func TestResolveCatalogTarget_DefaultWhenNothingConfigured(t *testing.T) {
	withTempCwd(t, func(dir string) {
		got := resolveCatalogTarget(dir, "")
		if got != "thrillmade/agent-skills" {
			t.Errorf("default catalog target = %q; want thrillmade/agent-skills", got)
		}
	})
}

// --- helpers ------------------------------------------------------------

// writeSampleSkillCLI mirrors writeSampleSkill in the skill package but
// keeps cli-package tests self-contained.
func writeSampleSkillCLI(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Title\n\nBody text.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func writeConfig(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".logmind")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Compile-time guards so this test file's helpers stay aligned with
// the skill package contract.
var _ = skill.ErrSkillNotFound
var _ = skill.PushOptions{}
