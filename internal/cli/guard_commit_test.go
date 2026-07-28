package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeGuardCommitConfig writes a minimal .logmind/config.yml under repo
// so tests can exercise git.enforce_commits / git.commit_line_threshold
// without hand-rolling the full config shape.
func writeGuardCommitConfig(t *testing.T, repo, body string) {
	t.Helper()
	dir := filepath.Join(repo, ".logmind")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .logmind: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
}

// bigLines returns n newline-terminated lines, enough to trip a small
// threshold regardless of diff mode.
func bigLines(n int) string {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		b.WriteString("line\n")
	}
	return b.String()
}

// --- runGuardCommit: --layer harness ------------------------------------

func TestRunGuardCommit_Harness_AllowsSmallChange(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "small.go"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git commit -m x`, repo)

	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty (harness allow prints nothing)", stdout.String())
	}
}

// TestRunGuardCommit_Harness_RestoresDerivedDocsOnNonDefaultBranch_IntegrationPointMode
// is the L2b proof, UPDATED for the B6 adoption gate: with
// `derived_docs: {mode: integration-point}` explicitly declared, the
// harness layer restores docs/timeline.md to its committed (HEAD) content
// on a non-default branch — BEFORE evaluating the allow/block decision —
// the same restore L1 (`logmind log`) and L2a (the pre-commit git hook)
// perform, but running BEFORE git itself. That's what lets this layer
// catch `git commit --no-verify` (which skips every git hook, including
// L2a) and work in a fresh clone (git hooks aren't cloned;
// .claude/settings.json, which invokes this binary, is). See the
// DriverMode sibling test below for the (now default) non-restoring case.
func TestRunGuardCommit_Harness_RestoresDerivedDocsOnNonDefaultBranch_IntegrationPointMode(t *testing.T) {
	repo := initRepo(t)
	docsDir := filepath.Join(repo, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	timelinePath := filepath.Join(docsDir, "timeline.md")
	original := "# Timeline\n\noriginal\n"
	if err := os.WriteFile(timelinePath, []byte(original), 0o644); err != nil {
		t.Fatalf("write timeline.md: %v", err)
	}
	writeGuardCommitConfig(t, repo, "derived_docs:\n  mode: integration-point\n")
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "add docs"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGitIn(t, repo, "checkout", "-b", "feat/restore-test")

	// Dirty docs/timeline.md — simulates `logmind warp` (or any stray
	// write) pulling in a different copy.
	dirty := "# Timeline\n\nDIRTY\n"
	if err := os.WriteFile(timelinePath, []byte(dirty), 0o644); err != nil {
		t.Fatalf("dirty timeline.md: %v", err)
	}

	payload := harnessJSON(t, "Bash", `git commit -am x`, repo)
	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0 (restored file leaves nothing substantive to block)", exitCode)
	}

	got, err := os.ReadFile(timelinePath)
	if err != nil {
		t.Fatalf("read timeline.md: %v", err)
	}
	if string(got) != original {
		t.Fatalf("docs/timeline.md not restored by the harness layer\nwant %q\ngot %q", original, string(got))
	}
}

// TestRunGuardCommit_Harness_SkipsAlreadyStagedDerivedDoc is the L2b
// companion to TestLog_PreservesManuallyStagedRepairOfDivergedBranch and
// TestWarpThenLog_PreservesRepairAcrossCommit (derived_repair_test.go) —
// v2.0.0 4b-quater. Unlike the sibling test above (which dirties the
// WORKING TREE only via os.WriteFile, simulating an accidental stray
// write), here docs/timeline.md is explicitly `git add`ed BEFORE the
// harness runs — simulating `logmind warp`'s merge-base repair, which
// stages its fix via `git checkout <merge-base> -- <path>` (see runWarp,
// warp.go). The harness's L2b restore must skip an already-staged path:
// since this layer fires BEFORE the pending Bash command (`git commit -am
// x`) even executes, anything already staged at check time can only be a
// prior, deliberate action — never something the pending command itself is
// about to do — so it is left alone rather than reverted to HEAD.
func TestRunGuardCommit_Harness_SkipsAlreadyStagedDerivedDoc(t *testing.T) {
	repo := initRepo(t)
	docsDir := filepath.Join(repo, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	timelinePath := filepath.Join(docsDir, "timeline.md")
	original := "# Timeline\n\noriginal\n"
	if err := os.WriteFile(timelinePath, []byte(original), 0o644); err != nil {
		t.Fatalf("write timeline.md: %v", err)
	}
	writeGuardCommitConfig(t, repo, "derived_docs:\n  mode: integration-point\n")
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "add docs"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGitIn(t, repo, "checkout", "-b", "feat/staged-repair")

	// Simulate `logmind warp`'s repair: write the repaired content AND
	// stage it — exactly what `git checkout <merge-base> -- <path>` does.
	repaired := "# Timeline\n\nREPAIRED (simulated warp merge-base fix)\n"
	if err := os.WriteFile(timelinePath, []byte(repaired), 0o644); err != nil {
		t.Fatalf("write repaired timeline.md: %v", err)
	}
	runGitIn(t, repo, "add", "docs/timeline.md")

	payload := harnessJSON(t, "Bash", `git commit -am x`, repo)
	var stdout, stderr bytes.Buffer
	if _, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &stdout, &stderr); err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}

	got, err := os.ReadFile(timelinePath)
	if err != nil {
		t.Fatalf("read timeline.md: %v", err)
	}
	if string(got) != repaired {
		t.Fatalf("harness restored an ALREADY-STAGED derived doc (undid the simulated warp repair)\nwant (preserved) %q\ngot %q", repaired, string(got))
	}
}

// TestRunGuardCommit_Harness_DriverModeSkipsRestore: driver mode — both the
// implicit default (no .logmind/config.yml at all) and an EXPLICIT
// `derived_docs: {mode: driver}` — disables the harness-layer restore too,
// not just L2a's pre-commit hook install (see the cli/init_test.go
// counterpart for that half) — so a dirtied derived doc rides through
// untouched. This is the v2.0.0 B6 inversion: pin-preservation used to be
// unconditional-by-default (git.pin_derived_docs: true); now it's
// opt-in-by-default (derived_docs.mode: driver).
func TestRunGuardCommit_Harness_DriverModeSkipsRestore(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configBody string // "" == no .logmind/config.yml written at all
	}{
		{name: "implicit default (no config)", configBody: ""},
		{name: "explicit driver mode", configBody: "derived_docs:\n  mode: driver\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := initRepo(t)
			docsDir := filepath.Join(repo, "docs")
			if err := os.MkdirAll(docsDir, 0o755); err != nil {
				t.Fatalf("mkdir docs: %v", err)
			}
			timelinePath := filepath.Join(docsDir, "timeline.md")
			if err := os.WriteFile(timelinePath, []byte("# Timeline\n\noriginal\n"), 0o644); err != nil {
				t.Fatalf("write timeline.md: %v", err)
			}
			if tc.configBody != "" {
				writeGuardCommitConfig(t, repo, tc.configBody)
			}
			for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "add docs"}} {
				cmd := exec.Command("git", args...)
				cmd.Dir = repo
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
			}
			runGitIn(t, repo, "checkout", "-b", "feat/restore-test")

			dirty := "# Timeline\n\nDIRTY\n"
			if err := os.WriteFile(timelinePath, []byte(dirty), 0o644); err != nil {
				t.Fatalf("dirty timeline.md: %v", err)
			}

			payload := harnessJSON(t, "Bash", `git commit -am x`, repo)
			var stdout, stderr bytes.Buffer
			if _, err := runGuardCommit(repo, "harness", "", 20, true, false,
				strings.NewReader(payload), &stdout, &stderr); err != nil {
				t.Fatalf("runGuardCommit: %v", err)
			}

			got, err := os.ReadFile(timelinePath)
			if err != nil {
				t.Fatalf("read timeline.md: %v", err)
			}
			if string(got) != dirty {
				t.Fatalf("docs/timeline.md was restored despite driver mode\nwant %q (untouched)\ngot %q", dirty, string(got))
			}
		})
	}
}

func TestRunGuardCommit_Harness_BlocksOverThresholdUnstagedChange(t *testing.T) {
	repo := initRepo(t)
	// Unstaged (not `git add`ed) — this is the WorkingTreeUnion regression:
	// a compound `git add -A && git commit` hasn't staged anything yet
	// when the harness's PreToolUse hook fires.
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(bigLines(30)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git add -A && git commit -m x`, repo)

	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("exitCode = %d; want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "logmind log") {
		t.Fatalf("stderr = %q; want it to mention `logmind log`", stderr.String())
	}
}

func TestRunGuardCommit_Harness_NonBashToolAllowsWithoutInspection(t *testing.T) {
	repo := initRepo(t)
	payload := harnessJSON(t, "Read", `git commit -m x`, repo)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (non-Bash tool_name fails open)", exitCode, err)
	}
}

func TestRunGuardCommit_Harness_MalformedJSONFailsOpen(t *testing.T) {
	repo := initRepo(t)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader("not json"), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (unparseable payload fails open)", exitCode, err)
	}
}

func TestRunGuardCommit_Harness_NonCommitBashCommandAllows(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(50)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `npm test`, repo)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (not a git-commit shape)", exitCode, err)
	}
}

func TestRunGuardCommit_Harness_SkipLogmindMarkerAllows(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(50)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git commit -m "wip [skip-logmind]"`, repo)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil ([skip-logmind] extracted from -m)", exitCode, err)
	}
}

// TestRunGuardCommit_Harness_SkipLogmindInSecondMessageArg is the MINOR B
// regression: git concatenates multiple -m args, so a [skip-logmind]
// marker in a SECOND -m must be honored (reading only the first -m would
// over-block a commit the agent explicitly opted out of).
func TestRunGuardCommit_Harness_SkipLogmindInSecondMessageArg(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(50)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git commit -m "subject line" -m "body [skip-logmind]"`, repo)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil ([skip-logmind] in the SECOND -m must be honored)", exitCode, err)
	}
}

func TestExtractSubjectHint_CollectsAllMessageForms(t *testing.T) {
	cases := []struct {
		cmd  string
		want string // the joined hint must contain this marker
	}{
		{`git commit -m "a" -m "b [skip-logmind]"`, "[skip-logmind]"},
		{`git commit --message="[skip-logmind]"`, "[skip-logmind]"},
		{`git commit --message "[skip-logmind]"`, "[skip-logmind]"},
		{`git commit -m"[skip-logmind]"`, "[skip-logmind]"},
	}
	for _, c := range cases {
		got := extractSubjectHint(c.cmd)
		if !strings.Contains(got, c.want) {
			t.Errorf("extractSubjectHint(%q) = %q; want it to contain %q", c.cmd, got, c.want)
		}
	}
	// No message flag → empty hint (the [skip-logmind] carve-out just
	// won't apply; other carve-outs still work).
	if got := extractSubjectHint(`git commit`); got != "" {
		t.Errorf("extractSubjectHint(`git commit`) = %q; want empty", got)
	}
}

func TestRunGuardCommit_Harness_UsesPayloadCwdOverRepoRootFlag(t *testing.T) {
	repo := initRepo(t)
	other := t.TempDir() // not a git repo at all
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(50)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// payload.cwd points at the real repo; --repo-root points nowhere
	// useful. The real repo's cwd must win, so the big change is caught.
	payload := harnessJSON(t, "Bash", `git commit -m x`, repo)
	exitCode, err := runGuardCommit(other, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("exitCode = %d; want 2 (payload cwd should have been used, not the --repo-root fallback)", exitCode)
	}
}

// TestRunGuardCommit_Harness_UntrackedFromSubdir_Blocks is the MAJOR 2
// regression: an untracked-only ~40-line file, with the harness payload's
// cwd pointing at a SUBDIRECTORY of the repo. Before the toplevel-resolve
// fix, the git status/diff ops ran with cmd.Dir = the subdir, so the
// root-relative untracked paths never resolved and the change was silently
// allowed. Resolving payload.cwd → repo toplevel makes it Block.
func TestRunGuardCommit_Harness_UntrackedFromSubdir_Blocks(t *testing.T) {
	repo := initRepo(t)
	sub := filepath.Join(repo, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	// Untracked (never `git add`ed) ~40-line file at the repo root.
	if err := os.WriteFile(filepath.Join(repo, "brand_new.go"), []byte(bigLines(40)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Payload cwd = the SUBDIR, not the repo root.
	payload := harnessJSON(t, "Bash", `git add -A && git commit -m x`, sub)

	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit("", "harness", "", 20, true, false,
		strings.NewReader(payload), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("exitCode = %d; want 2 (untracked file must be caught even when evaluated from a subdir)", exitCode)
	}
	if !strings.Contains(stderr.String(), "logmind log") {
		t.Fatalf("stderr = %q; want it to mention `logmind log`", stderr.String())
	}
}

// TestRunGuardCommit_Harness_UnicodeUntrackedFromSubdir_Blocks combines
// MINOR A (unicode untracked filename via -z) with MAJOR 2 (subdir cwd):
// an untracked ~40-line file named "é.go" must still be counted and block.
func TestRunGuardCommit_Harness_UnicodeUntrackedFromSubdir_Blocks(t *testing.T) {
	repo := initRepo(t)
	sub := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "é.go"), []byte(bigLines(40)), 0o644); err != nil {
		t.Fatalf("write é.go: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git add -A && git commit -m x`, sub)

	exitCode, err := runGuardCommit("", "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("exitCode = %d; want 2 (unicode-named untracked file must be counted)", exitCode)
	}
}

// TestRunGuardCommit_Harness_EnforceFalseFromSubdir_Allows is the MAJOR 3
// regression: git.enforce_commits:false in the repo's config must be
// honored even when the payload cwd is a SUBDIR (config must load from the
// resolved toplevel, not the subdir where no .logmind/config.yml exists).
func TestRunGuardCommit_Harness_EnforceFalseFromSubdir_Allows(t *testing.T) {
	repo := initRepo(t)
	writeGuardCommitConfig(t, repo, "git:\n  enforce_commits: false\n")
	sub := filepath.Join(repo, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(100)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git add -A && git commit -m x`, sub)

	exitCode, err := runGuardCommit("", "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (enforce_commits:false must load from the toplevel, not the subdir)", exitCode, err)
	}
}

// --- runGuardCommit: --layer git-hook ------------------------------------

func TestRunGuardCommit_GitHook_AllowsSmallStagedChange(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "small.go", 5)
	msgFile := writeMsgFile(t, repo, "a small change")

	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit(repo, "git-hook", msgFile, 20, true, false,
		nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGuardCommit: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
}

// TestRunGuardCommit_GitHook_BlocksOverThresholdStagedChange pins the
// stale-binary-hardening exit-code contract: the git-hook layer now
// blocks via a distinctive exit code (65, EX_DATAERR) rather than the
// generic cli.ErrSilent (which main.go turns into the same exit 1 a
// stale, unrelated logmind failure would also produce — see
// guardCommitGitHook's doc comment for why that collision is exactly the
// bug this hardening fixes).
func TestRunGuardCommit_GitHook_BlocksOverThresholdStagedChange(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "big.go", 25)
	msgFile := writeMsgFile(t, repo, "a big change")

	var stdout, stderr bytes.Buffer
	exitCode, err := runGuardCommit(repo, "git-hook", msgFile, 20, true, false,
		nil, &stdout, &stderr)
	if exitCode != 65 {
		t.Fatalf("exitCode = %d; want 65 (the git-hook layer's distinctive EX_DATAERR block signal)", exitCode)
	}
	if err != nil {
		t.Fatalf("err = %v; want nil (the block is signalled via exitCode, not an error)", err)
	}
	if !strings.Contains(stderr.String(), "logmind log") {
		t.Fatalf("stderr = %q; want it to mention `logmind log`", stderr.String())
	}
}

func TestRunGuardCommit_GitHook_UnstagedOnlyChangeAllows(t *testing.T) {
	// The StagedOnly half of the key regression: a large change that
	// hasn't been staged is invisible to the git-hook layer by design
	// (correct — the index IS final by the time commit-msg runs; an
	// unstaged change literally cannot end up in this commit).
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(bigLines(30)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	msgFile := writeMsgFile(t, repo, "subject")

	exitCode, err := runGuardCommit(repo, "git-hook", msgFile, 20, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (unstaged change invisible under StagedOnly)", exitCode, err)
	}
}

func TestRunGuardCommit_GitHook_MissingMsgFileFlag_PlainError(t *testing.T) {
	repo := initRepo(t)
	exitCode, err := runGuardCommit(repo, "git-hook", "", 20, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
	if err == nil {
		t.Fatalf("err = nil; want a plain error (missing --msg-file)")
	}
	if err == ErrSilent {
		t.Fatalf("err = ErrSilent; want a plain (non-ErrSilent) error for this misuse case")
	}
	if !strings.Contains(err.Error(), "--msg-file") {
		t.Fatalf("err = %v; want it to mention --msg-file", err)
	}
}

func TestRunGuardCommit_GitHook_MissingMsgFileOnDisk_PropagatesRealError(t *testing.T) {
	repo := initRepo(t)
	exitCode, err := runGuardCommit(repo, "git-hook", filepath.Join(repo, "does-not-exist.txt"), 20, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
	if err == nil || err == ErrSilent {
		t.Fatalf("err = %v; want a plain os.ReadFile error", err)
	}
}

// --- shared: --layer validation, threshold resolution, enforce_commits ---

func TestRunGuardCommit_InvalidLayer_PlainError(t *testing.T) {
	repo := initRepo(t)
	exitCode, err := runGuardCommit(repo, "bogus", "", 20, true, false,
		strings.NewReader("{}"), &bytes.Buffer{}, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("exitCode = %d; want 0", exitCode)
	}
	if err == nil || err == ErrSilent {
		t.Fatalf("err = %v; want a plain error naming the bad --layer value", err)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err = %v; want it to echo the invalid value", err)
	}
}

func TestRunGuardCommit_EnforceCommitsFalse_AllowsBothLayers(t *testing.T) {
	repo := initRepo(t)
	writeGuardCommitConfig(t, repo, "git:\n  enforce_commits: false\n")

	// Harness layer: a huge unstaged change that would otherwise block.
	if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(100)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	payload := harnessJSON(t, "Bash", `git commit -m x`, repo)
	exitCode, err := runGuardCommit(repo, "harness", "", 20, true, false,
		strings.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("harness: exitCode=%d err=%v; want 0,nil (enforce_commits:false is a full off-ramp)", exitCode, err)
	}

	// git-hook layer: stage the same huge change.
	stageLines(t, repo, "staged-big.go", 100)
	msgFile := writeMsgFile(t, repo, "subject")
	exitCode, err = runGuardCommit(repo, "git-hook", msgFile, 20, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("git-hook: exitCode=%d err=%v; want 0,nil (enforce_commits:false is a full off-ramp)", exitCode, err)
	}
}

func TestRunGuardCommit_ThresholdPrecedence(t *testing.T) {
	repo := initRepo(t)
	writeGuardCommitConfig(t, repo, "git:\n  commit_line_threshold: 5\n")
	stageLines(t, repo, "code.go", 10)
	msgFile := writeMsgFile(t, repo, "subject")

	// Config threshold (5) is below 10 changed lines → block, with no
	// --threshold flag override (flagExplicit=false, flagValue ignored).
	exitCode, err := runGuardCommit(repo, "git-hook", msgFile, 999, false, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("err = %v; want nil (block is signalled via exitCode)", err)
	}
	if exitCode != 65 {
		t.Fatalf("exitCode = %d; want 65 (config threshold of 5 should have applied and blocked)", exitCode)
	}

	// Explicit --threshold flag (50) wins over the config's 5.
	exitCode, err = runGuardCommit(repo, "git-hook", msgFile, 50, true, false,
		nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || exitCode != 0 {
		t.Fatalf("exitCode=%d err=%v; want 0,nil (explicit --threshold=50 should have won over config's 5)", exitCode, err)
	}
}

// --- black-box subprocess test: exit code 2, not 1 -----------------------

// TestGuardCommitBinary_Harness_ExitCode2NotJust1 builds the real binary
// and pipes a blocking harness JSON payload to
// `guard-commit --layer harness`, asserting the process exits 2 — NOT the
// generic exit-1 every other error path in this CLI uses. Exit code 2 is
// load-bearing: the Claude Code harness's PreToolUse protocol only treats
// exit 2 as "block this tool call"; exit 1 would silently let it through.
func TestGuardCommitBinary_Harness_ExitCode2NotJust1(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; skipping subprocess test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping subprocess test")
	}

	binPath := buildGuardCommitBinary(t, goBin)

	t.Run("blocks with exit code 2", func(t *testing.T) {
		repo := initRepo(t) // fresh repo per subtest — no cross-subtest state bleed
		if err := os.WriteFile(filepath.Join(repo, "big.go"), []byte(bigLines(30)), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		payload := harnessJSON(t, "Bash", `git add -A && git commit -m x`, repo)

		cmd := exec.Command(binPath, "guard-commit", "--layer", "harness")
		cmd.Stdin = strings.NewReader(payload)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()

		exitErr, ok := runErr.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected *exec.ExitError, got %T (%v); stdout=%q stderr=%q", runErr, runErr, stdout.String(), stderr.String())
		}
		if got := exitErr.ExitCode(); got != 2 {
			t.Fatalf("ExitCode() = %d; want 2 (not the generic exit 1)", got)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q; want empty on block", stdout.String())
		}
		if !strings.Contains(stderr.String(), "logmind log") {
			t.Fatalf("stderr = %q; want it to mention `logmind log`", stderr.String())
		}
	})

	t.Run("allows with exit code 0 and empty stdout", func(t *testing.T) {
		repo := initRepo(t) // fresh repo per subtest — no cross-subtest state bleed
		if err := os.WriteFile(filepath.Join(repo, "tiny.go"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		payload := harnessJSON(t, "Bash", `git commit -m x`, repo)

		cmd := exec.Command(binPath, "guard-commit", "--layer", "harness")
		cmd.Stdin = strings.NewReader(payload)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("logmind guard-commit --layer harness (allow case) failed: %v\nstdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q; want empty on allow", stdout.String())
		}
	})
}

// --- black-box subprocess test: exit code 65, the git-hook block signal --

// TestGuardCommitBinary_GitHook_ExitCode65 builds the real binary and runs
// `guard-commit --layer git-hook --msg-file <path>` directly against a
// staged over-threshold change, pinning the process's exit code at 65
// (EX_DATAERR) — the git-hook layer's distinctive block signal introduced
// by the stale-binary hardening (see guard_commit.go's guardCommitGitHook
// and its LOUD COMMENT). 65 must stay a code no ordinary failure produces
// by accident: internal/hooks.BuildCommitMsgBody's commit-msg hook body
// checks for EXACTLY 65 before aborting a commit, so a stale-but-present
// logmind's unrelated nonzero exit (1 for an old Cobra build's
// unknown-command error, 2 for the frozen Python CLI's argparse error)
// must never collide with it.
func TestGuardCommitBinary_GitHook_ExitCode65(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; skipping subprocess test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping subprocess test")
	}

	binPath := buildGuardCommitBinary(t, goBin)

	t.Run("blocks with exit code 65", func(t *testing.T) {
		repo := initRepo(t) // fresh repo per subtest — no cross-subtest state bleed
		stageLines(t, repo, "big.go", 25)
		msgFile := writeMsgFile(t, repo, "a big change")

		cmd := exec.Command(binPath, "guard-commit", "--layer", "git-hook", "--msg-file", msgFile, "--repo-root", repo)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()

		exitErr, ok := runErr.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected *exec.ExitError, got %T (%v); stdout=%q stderr=%q", runErr, runErr, stdout.String(), stderr.String())
		}
		if got := exitErr.ExitCode(); got != 65 {
			t.Fatalf("ExitCode() = %d; want 65 (EX_DATAERR, the git-hook layer's distinctive block signal)", got)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q; want empty on block", stdout.String())
		}
		if !strings.Contains(stderr.String(), "logmind log") {
			t.Fatalf("stderr = %q; want it to mention `logmind log`", stderr.String())
		}
	})

	t.Run("allows with exit code 0", func(t *testing.T) {
		repo := initRepo(t) // fresh repo per subtest — no cross-subtest state bleed
		stageLines(t, repo, "small.go", 5)
		msgFile := writeMsgFile(t, repo, "a small change")

		cmd := exec.Command(binPath, "guard-commit", "--layer", "git-hook", "--msg-file", msgFile, "--repo-root", repo)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("logmind guard-commit --layer git-hook (allow case) failed: %v\nstdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
	})
}

// buildGuardCommitBinary compiles ./cmd/logmind once per test run into a
// temp dir and returns the binary path.
func buildGuardCommitBinary(t *testing.T, goBin string) string {
	t.Helper()
	repoRoot := repoRootFromCaller(t)
	binDir := t.TempDir()
	binName := "logmind"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(binDir, binName)

	build := exec.Command(goBin, "build", "-o", binPath, "./cmd/logmind")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return binPath
}

// --- shared payload/file helpers ------------------------------------------

// harnessJSON builds a minimal PreToolUse-shaped JSON payload.
func harnessJSON(t *testing.T, toolName, command, cwd string) string {
	t.Helper()
	payload := struct {
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
		Cwd string `json:"cwd"`
	}{ToolName: toolName, Cwd: cwd}
	payload.ToolInput.Command = command
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal harness payload: %v", err)
	}
	return string(data)
}

// writeMsgFile writes a commit-message file (subject on the first line)
// and returns its path, mimicking what git hands a commit-msg hook.
func writeMsgFile(t *testing.T, repo, subject string) string {
	t.Helper()
	path := filepath.Join(repo, "COMMIT_EDITMSG")
	if err := os.WriteFile(path, []byte(subject+"\n\nbody text\n"), 0o644); err != nil {
		t.Fatalf("write msg file: %v", err)
	}
	return path
}
