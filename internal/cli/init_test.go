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
	"strconv"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/timeline"
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
		mustContain(t, out.String(), "✓ Created docs/file-structure.md")
		mustContain(t, out.String(), "✓ Created docs/timeline.md")
		mustContain(t, out.String(), "✓ Created docs/timeline-archive.md")
		mustContain(t, out.String(), "✓ Created .logmind/config.yml")
		mustContain(t, out.String(), "logmind initialized successfully!")
	})

	// .logmind/config.yml's decisions: comment restates the SPEC §3.3 bound
	// via the __LOGMIND_RECENT_LIMIT__ placeholder (config.yml.template is a
	// one-time seed, never refreshed, so this is the ONLY place it is ever
	// substituted). Assert the substitution landed the live constant, and
	// that the raw placeholder never reaches disk — the same "never lands
	// on disk" shape TestInit_WritesWorkflowTemplates already holds
	// __LOGMIND_VERSION__ / __LOGMIND_DEFAULT_BRANCH__ to.
	cfgBody, err := os.ReadFile(filepath.Join(dir, ".logmind", "config.yml"))
	if err != nil {
		t.Fatalf("read .logmind/config.yml: %v", err)
	}
	mustContain(t, string(cfgBody), "carries the "+strconv.Itoa(timeline.RecentLimit)+" most recent entries")
	if strings.Contains(string(cfgBody), "__LOGMIND_RECENT_LIMIT__") {
		t.Errorf(".logmind/config.yml still contains __LOGMIND_RECENT_LIMIT__ placeholder")
	}

	// Verify file contents on disk.
	for _, rel := range []string{
		"docs/file-structure.md", "docs/timeline.md", "docs/timeline-archive.md",
		".logmind/config.yml", "AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected %s after init; %v", rel, err)
		}
	}
	// SPEC §3.2: nothing is archived, so init must not scaffold an archive
	// back into existence. (docs/decisions.md IS written here — this init is
	// --no-git, so there is no branch to name a file after and the first
	// decision takes the branchless fallback. TestInit_InGitRepo_FirstDecision
	// GoesToMainBranchFile covers the case that has a branch.)
	if _, err := os.Stat(filepath.Join(dir, "docs/decisions-archive.md")); err == nil {
		t.Errorf("init created docs/decisions-archive.md — that path is gone under §3.2")
	}
}

// TestInit_InGitRepo_FirstDecisionGoesToMainBranchFile pins SPEC §3.2's one
// path rule at the very first decision a repository ever records: on `main`,
// `logmind init` writes it to docs/decisions-branches/main.md — the file
// named for the branch it was made on — and creates no separate main log.
//
// Pinned on the files on disk after a real `logmind init`, not on
// resolveDecisionsPath: a helper-level test would pass just as happily with
// init still writing docs/decisions.md behind it.
func TestInit_InGitRepo_FirstDecisionGoesToMainBranchFile(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		runQuiet(t, []string{"init", "--no-git"})
	})

	body, err := os.ReadFile(filepath.Join(dir, "docs", "decisions-branches", "main.md"))
	if err != nil {
		t.Fatalf("read docs/decisions-branches/main.md: %v", err)
	}
	if !strings.Contains(string(body), "Initialize logmind decision tracking") {
		t.Errorf("main.md missing the first decision; body:\n%s", body)
	}
	// A branch file, opened like any other: backlink header + timeline marker.
	if !strings.Contains(string(body), "← back to [docs/timeline.md](../timeline.md)") {
		t.Errorf("main.md missing the branch-file backlink header; body:\n%s", body)
	}
	if !strings.Contains(string(body), "<!-- logmind-entry-start: ") {
		t.Errorf("main.md missing its timeline entry-block marker; body:\n%s", body)
	}
	mustRouteNoDecisionsTo(t, filepath.Join(dir, "docs", "decisions.md"),
		"init routed the first decision to docs/decisions.md on a repo that has a branch — main is a branch like any other (§3.2)")
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
		// Same for the SPEC §3.3 bound (RecentLimit): check-doc-links.yml and
		// regen-timeline.yml both restate it in a changelog comment.
		if strings.Contains(body, "__LOGMIND_RECENT_LIMIT__") {
			t.Errorf("%s still contains __LOGMIND_RECENT_LIMIT__ placeholder", name)
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
		// --no-git means there is no branch, so init's first decision lands in
		// the branchless fallback file.
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

// TestInit_CreatesDependabotConfig — v1.1.0 ships a
// .github/dependabot.yml from `logmind init` so consumer repos get
// automatic Dependabot bumps for the thrillmade/setup-logmind action
// ref. Asserts both that the file lands and that it contains the
// thrillmade group with the correct pattern.
func TestInit_CreatesDependabotConfig(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("init: %v\n%s", err, errOut.String())
		}
		mustContain(t, out.String(), "✓ Created .github/dependabot.yml")
	})
	body, err := os.ReadFile(filepath.Join(dir, ".github", "dependabot.yml"))
	if err != nil {
		t.Fatalf("read dependabot.yml: %v", err)
	}
	mustContain(t, string(body), "package-ecosystem: \"github-actions\"")
	mustContain(t, string(body), "thrillmade:")
	mustContain(t, string(body), "- \"thrillmade/*\"")
}

// TestInit_DependabotMergeWithExistingGomodEntry — when the consumer
// repo already has a .github/dependabot.yml that pins a different
// ecosystem (gomod, npm, ...), `logmind init` MERGES the github-actions
// block in rather than clobbering the file. Existing entries survive
// byte-for-byte.
func TestInit_DependabotMergeWithExistingGomodEntry(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		// Pre-create a hand-rolled dependabot.yml under .github/.
		if err := os.MkdirAll(".github", 0o755); err != nil {
			t.Fatal(err)
		}
		preExisting := "version: 2\nupdates:\n  - package-ecosystem: \"gomod\"\n    directory: \"/\"\n    schedule:\n      interval: \"weekly\"\n"
		if err := os.WriteFile(".github/dependabot.yml", []byte(preExisting), 0o644); err != nil {
			t.Fatal(err)
		}
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("init: %v\n%s", err, errOut.String())
		}
		mustContain(t, out.String(), "✓ Merged thrillmade group into .github/dependabot.yml")
	})
	body, err := os.ReadFile(filepath.Join(dir, ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	// Pre-existing entry preserved.
	mustContain(t, string(body), "gomod")
	// Our block appended.
	mustContain(t, string(body), "thrillmade:")
}

// TestInit_DependabotHandsOffUserOwnedGithubActionsBlock — if the
// consumer already has a `package-ecosystem: "github-actions"` entry
// in dependabot.yml, `logmind init` must NOT modify it (Dependabot
// rejects duplicate ecosystem+directory pairs). Surface a nudge instead.
func TestInit_DependabotHandsOffUserOwnedGithubActionsBlock(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		if err := os.MkdirAll(".github", 0o755); err != nil {
			t.Fatal(err)
		}
		preExisting := "version: 2\nupdates:\n  - package-ecosystem: \"github-actions\"\n    directory: \"/\"\n    schedule:\n      interval: \"weekly\"\n"
		if err := os.WriteFile(".github/dependabot.yml", []byte(preExisting), 0o644); err != nil {
			t.Fatal(err)
		}
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("init: %v\n%s", err, errOut.String())
		}
		// Should NOT print Created or Merged. SHOULD print the
		// opt-in hint mentioning the existing github-actions
		// coverage.
		got := out.String()
		if strings.Contains(got, "✓ Created .github/dependabot.yml") {
			t.Errorf("init wrongly clobbered user-owned dependabot.yml")
		}
		if !strings.Contains(got, "already covers github-actions") {
			t.Errorf("missing opt-in nudge for user-owned dependabot.yml; got:\n%s", got)
		}
	})
	// File body still pristine.
	body, err := os.ReadFile(filepath.Join(dir, ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "thrillmade:") {
		t.Errorf("init mutated a user-owned ecosystem block")
	}
}

// TestInit_WorkflowsUseSetupLogmindActionAfterInit — the rendered
// workflow files on disk (post `logmind init`) should ship the
// setup-logmind action ref and NOT carry the legacy curl/pip pattern.
// This is the consumer-facing parity check: in addition to the embed
// template asserting in templates_test.go, also assert that the
// renderer didn't smuggle the old pattern back in via placeholder
// substitution.
func TestInit_WorkflowsUseSetupLogmindActionAfterInit(t *testing.T) {
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
	for _, name := range []string{"check-doc-links.yml", "regen-timeline.yml"} {
		body, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(body)
		mustContain(t, text, "thrillmade/setup-logmind@v")
		if strings.Contains(text, "pip install \"logmind==") {
			t.Errorf("%s should not contain legacy pip install line after init", name)
		}
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

// TestInit_InstallsClaudePreToolUseGuardByDefault: `init --no-git` still
// installs the Layer 1 Claude Code PreToolUse guard (.claude/settings.json)
// because claude is in agents.DefaultEnabled() — and the install happens
// even though --no-git skips the git-hook installers, since
// .claude/settings.json is repo content, not git-clone state.
func TestInit_InstallsClaudePreToolUseGuardByDefault(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("init: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
		}
		mustContain(t, out.String(), "✓ Installed Claude Code guard-commit hook (.claude/settings.json)")
	})
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("expected .claude/settings.json after init: %v", err)
	}
	mustContain(t, string(body), "logmind guard-commit --layer harness")
	mustContain(t, string(body), "logmind-hook-version")
}

// TestInit_AgentsFlagExcludingClaudeSkipsPreToolUseGuard: when --agents
// excludes claude, Layer 1 must NOT be installed — the gate is
// slices.Contains(enabled, "claude"), not "always on".
func TestInit_AgentsFlagExcludingClaudeSkipsPreToolUseGuard(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git", "--agents", "cursor"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("init: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
		}
		if strings.Contains(out.String(), "Claude Code guard-commit hook") {
			t.Errorf("did NOT expect the Claude Code guard-commit hook line when --agents excludes claude:\n%s", out.String())
		}
	})
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.json")); err == nil {
		t.Errorf("did NOT expect .claude/settings.json when --agents excludes claude")
	}
}

// TestInit_RefreshMode_InstallsMissingClaudeHook: refresh mode (re-running
// `init` on an already-initialised repo) recomputes claudeAgentEnabled
// from the same --agents/--all-agents flags every time (it's not read
// back from a persisted config), so a repo whose .claude/settings.json
// was deleted (or never created, e.g. by an old logmind version) gets it
// installed on the next refresh pass.
func TestInit_RefreshMode_InstallsMissingClaudeHook(t *testing.T) {
	dir := withTempCwd(t, func(_ string) {
		// First init WITHOUT claude enabled, so no guard gets installed.
		runQuiet(t, []string{"init", "--no-git", "--agents", "cursor"})
		if _, err := os.Stat(".claude/settings.json"); err == nil {
			t.Fatalf("did not expect .claude/settings.json after the first (cursor-only) init")
		}

		// Second call is refresh mode; defaulting the agents list this time
		// (no --agents flag) re-enables claude, which should backfill the
		// hook that was never installed.
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("refresh init: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
		}
		mustContain(t, out.String(), "✓ Refreshed .claude/settings.json (Claude Code guard-commit hook)")
	})
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.json")); err != nil {
		t.Errorf("expected .claude/settings.json after refresh re-enabled claude: %v", err)
	}
}

// TestInit_RefreshMode_AlwaysInstallsPreCommitHook pins the removal of the
// v2.0.0 B6 `derived_docs.mode` adoption gate from L2a's pre-commit hook
// install (see internal/cli/derived.go and internal/cli/refresh.go): a
// refresh-mode `logmind init` installs the pre-commit hook UNCONDITIONALLY,
// alongside the other three git hooks (post-merge/post-rewrite/commit-msg)
// — no config declaration required, and a leftover legacy
// `derived_docs: {mode: driver}` section (from a repo that predates the
// gate's removal) doesn't suppress it either. Replaces the old
// DriverModeSkipsPreCommitHookInstall / IntegrationPointModeInstallsPreCommitHook
// pair, which pinned the now-removed per-repo gate and its inverse.
func TestInit_RefreshMode_AlwaysInstallsPreCommitHook(t *testing.T) {
	for _, tc := range []struct {
		name           string
		derivedDocsCfg string // "" leaves the config.yml `logmind init` scaffolds untouched
	}{
		{name: "no config declaration (config.yml as scaffolded)"},
		{name: "legacy derived_docs.mode: driver (now ignored)", derivedDocsCfg: "derived_docs:\n  mode: driver\n"},
		{name: "legacy derived_docs.mode: integration-point (now ignored)", derivedDocsCfg: "derived_docs:\n  mode: integration-point\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := withTempCwd(t, func(d string) {
				initLogTestGitRepo(t, d)
				// --no-git here scaffolds docs/ + .logmind/config.yml without
				// touching git hooks at all (this test is purely about the
				// refresh-mode install, exercised by the SECOND init call below).
				runQuiet(t, []string{"init", "--no-git"})

				if tc.derivedDocsCfg != "" {
					cfgPath := filepath.Join(d, ".logmind", "config.yml")
					if err := os.WriteFile(cfgPath, []byte(tc.derivedDocsCfg), 0o644); err != nil {
						t.Fatalf("write config.yml: %v", err)
					}
				}

				// Second call: already-initialized → refresh mode. No --no-git
				// this time, so applyRefresh's opts.git branch runs for real.
				root := NewRootCmd()
				root.SetArgs([]string{"init"})
				var out, errOut bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&errOut)
				if err := root.Execute(); err != nil {
					t.Fatalf("refresh init: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
				}
				if !strings.Contains(out.String(), "pre-commit") {
					t.Errorf("expected a pre-commit hook line (unconditional):\n%s", out.String())
				}
			})
			if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-commit")); err != nil {
				t.Errorf("expected .git/hooks/pre-commit (unconditional install): %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "post-merge")); err != nil {
				t.Errorf("expected .git/hooks/post-merge to still be installed: %v", err)
			}
		})
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
