// refresh_test.go — exercises `logmind doctor --fix` (the shared
// applyRefresh remediation pass reached through the doctor command).
package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitInitCwd runs `git init` (+ identity) in the current working dir so
// merge-driver config and hook installation have a real repo to act on.
func gitInitCwd(t *testing.T) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "logmind-test"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func writeRel(t *testing.T, rel, content string, mode os.FileMode) {
	t.Helper()
	if dir := filepath.Dir(rel); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(rel, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readRel(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// runDoctorFixCmd runs `doctor --fix --offline` in the current cwd and
// returns (stdout, stderr). Fails on a non-nil error (a hard write fault).
func runDoctorFixCmd(t *testing.T) (string, string) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs([]string{"doctor", "--fix", "--offline"})
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --fix: %v\nstderr=%s", err, errOut.String())
	}
	return out.String(), errOut.String()
}

// TestDoctorFix_RefreshesStaleAndIsIdempotent: a stale workflow is brought
// current, and a second pass changes nothing (no over-reporting).
func TestDoctorFix_RefreshesStaleAndIsIdempotent(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		// Plant one stale workflow; the other three are absent (so the
		// first --fix both refreshes and creates).
		writeRel(t, filepath.Join(".github", "workflows", "regen-timeline.yml"),
			"# logmind-template-version: v0-FAKE\n# rest\n", 0o644)

		out1, _ := runDoctorFixCmd(t)
		mustContain(t, out1, "ok doctor-fix")
		// The stale marker must be gone (refreshed to the bundled template).
		if got := readRel(t, filepath.Join(".github", "workflows", "regen-timeline.yml")); contains(got, "v0-FAKE") {
			t.Errorf("--fix left the stale v0-FAKE marker in regen-timeline.yml:\n%s", got)
		}

		// Second pass: everything is current → no writes anywhere.
		out2, _ := runDoctorFixCmd(t)
		mustContain(t, out2, "workflows=0")
		mustContain(t, out2, "hooks=0")
		mustContain(t, out2, "agents-md=current")
		mustContain(t, out2, "gitattributes=current")
		mustContain(t, out2, "merge-driver=current")
	})
}

// TestDoctorFix_LeavesForeignHookAlone: a hand-written (markerless) hook is
// NEVER clobbered by --fix.
func TestDoctorFix_LeavesForeignHookAlone(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		foreign := "#!/bin/sh\necho my own custom post-merge hook\n"
		writeRel(t, filepath.Join(".git", "hooks", "post-merge"), foreign, 0o755)

		_, stderr := runDoctorFixCmd(t)

		if got := readRel(t, filepath.Join(".git", "hooks", "post-merge")); got != foreign {
			t.Errorf("--fix clobbered a foreign hook:\n got: %q\nwant: %q", got, foreign)
		}
		// The untouched foreign hook must surface as residual drift (the
		// markerless post-merge hook), not be silently ignored.
		mustContain(t, stderr, "post-merge hook")
	})
}

// TestDoctorFix_HardWriteErrorExitsNonZero: a genuine write fault (here,
// .github is a regular file so .github/workflows/ can't be created) makes
// --fix exit 1 (ErrSilent) with an error note — distinct from the exit-0
// residual path.
func TestDoctorFix_HardWriteErrorExitsNonZero(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		// Block .github/workflows creation by making .github a plain file.
		writeRel(t, ".github", "not a directory\n", 0o644)

		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--fix", "--offline"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		err := root.Execute()
		if err == nil {
			t.Fatalf("expected non-nil error on hard write fault; out=%s", out.String())
		}
		if !errors.Is(err, ErrSilent) {
			t.Errorf("err = %v; want ErrSilent", err)
		}
		mustContain(t, errOut.String(), "doctor --fix")
	})
}

// TestDoctorFix_NeverTouchesDocs: --fix is install-state only; it must not
// rewrite decision content or derived docs.
func TestDoctorFix_NeverTouchesDocs(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		sentinels := map[string]string{
			filepath.Join("docs", "decisions.md"):      "# Decision Log\n\n## 2026-01-01 00:00 - keep me\n\n---\n",
			filepath.Join("docs", "timeline.md"):       "# Timeline\n\n(hand-written sentinel)\n",
			filepath.Join("docs", "file-structure.md"): "# File Structure\n\n(hand-written sentinel)\n",
			filepath.Join(".logmind", "config.yml"):    "git:\n  auto_commit: false\n",
		}
		for rel, content := range sentinels {
			writeRel(t, rel, content, 0o644)
		}

		runDoctorFixCmd(t)

		for rel, want := range sentinels {
			if got := readRel(t, rel); got != want {
				t.Errorf("--fix modified %s:\n got: %q\nwant: %q", rel, got, want)
			}
		}
	})
}

func contains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}

// TestDoctorFix_InstallsClaudePreToolUseGuardByDefault: with no
// .logmind/config.yml at all (so agents.claude falls back to the
// default-true behavior), --fix installs the Layer 1 Claude Code
// PreToolUse guard alongside the rest of the remediation pass, and a
// second pass reports it as already current (no re-write).
func TestDoctorFix_InstallsClaudePreToolUseGuardByDefault(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)

		out1, _ := runDoctorFixCmd(t)
		mustContain(t, out1, "claude-hook=changed")
		body := readRel(t, filepath.Join(".claude", "settings.json"))
		mustContain(t, body, "logmind guard-commit --layer harness")

		out2, _ := runDoctorFixCmd(t)
		mustContain(t, out2, "claude-hook=current")
	})
}

// TestDoctorFix_ClaudeDisabledInConfigSkipsPreToolUseGuard: an explicit
// `agents.claude: false` in .logmind/config.yml must prevent --fix from
// installing the Layer 1 guard at all.
func TestDoctorFix_ClaudeDisabledInConfigSkipsPreToolUseGuard(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		writeRel(t, filepath.Join(".logmind", "config.yml"), "agents:\n  claude: false\n", 0o644)

		out, _ := runDoctorFixCmd(t)
		mustContain(t, out, "claude-hook=current")
		if _, err := os.Stat(filepath.Join(".claude", "settings.json")); err == nil {
			t.Errorf("did NOT expect .claude/settings.json when agents.claude is false")
		}
	})
}

// TestDoctorFix_MalformedClaudeSettingsDegradesGracefully: a user's
// malformed (e.g. JSONC-style trailing-comma) .claude/settings.json must
// NOT hard-fail `doctor --fix` — EnsurePreToolUseGuard's refusal is
// swallowed like a foreign git hook's, so --fix still exits 0, still
// prints the `ok doctor-fix` summary, still applies the rest of the
// remediation pass (hooks etc.), and leaves the malformed file untouched.
func TestDoctorFix_MalformedClaudeSettingsDegradesGracefully(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		malformed := "{\n  \"hooks\": {\n    \"PreToolUse\": [],\n  },\n}\n" // trailing commas — JSONC, not JSON
		writeRel(t, filepath.Join(".claude", "settings.json"), malformed, 0o644)

		// runDoctorFixCmd fails the test on a non-nil Execute() error, so
		// reaching the assertions below already proves exit 0.
		out, _ := runDoctorFixCmd(t)
		mustContain(t, out, "ok doctor-fix")
		mustContain(t, out, "claude-hook=current") // no write happened

		// The rest of the remediation still ran: the git hooks were
		// installed on this fresh repo.
		if _, err := os.Stat(filepath.Join(".git", "hooks", "commit-msg")); err != nil {
			t.Errorf("expected commit-msg hook installed despite malformed settings.json: %v", err)
		}

		// The malformed file is user content --fix can't repair — byte-untouched.
		if got := readRel(t, filepath.Join(".claude", "settings.json")); got != malformed {
			t.Errorf("malformed settings.json was modified:\n got: %q\nwant: %q", got, malformed)
		}
	})
}
