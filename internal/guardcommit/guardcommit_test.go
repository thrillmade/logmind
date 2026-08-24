package guardcommit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/agents"
	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/gitcli"
	"github.com/thrillmade/logmind/internal/templates"
	"github.com/thrillmade/logmind/internal/testgit"
)

// initRepo creates a fresh git repo at t.TempDir(), commits an initial
// README so HEAD is born, and returns the path. Skips the test if `git`
// isn't on PATH. Mirrors internal/gitcli's own initRepo test helper —
// duplicated rather than exported so this package's tests don't reach
// into gitcli's internals.
func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	testgit.InitRepo(t, dir, "-q")
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
	} {
		run(t, dir, args...)
	}
	writeFile(t, dir, "README.md", "hello\n")
	run(t, dir, "add", "README.md")
	run(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

// linesOf returns a string with n newline-terminated lines — enough
// "added" lines to trip a small threshold regardless of diff mode.
func linesOf(n int) string {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		b.WriteString("line\n")
	}
	return b.String()
}

// --- carve-out matrix -------------------------------------------------

func TestEvaluate_NotARepo_Allows(t *testing.T) {
	dir := t.TempDir() // no `git init`
	for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
		d := Evaluate(dir, "subject", 20, mode)
		if !d.Allow {
			t.Fatalf("mode=%v: Allow = false; want true (not a repo)", mode)
		}
		if d.CarveOut != CarveOutNone {
			t.Fatalf("mode=%v: CarveOut = %q; want empty", mode, d.CarveOut)
		}
	}
}

func TestEvaluate_EnvAllow(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	run(t, dir, "add", "big.go")
	t.Setenv("LOGMIND_ALLOW_GIT_COMMIT", "1")
	for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
		d := Evaluate(dir, "subject", 20, mode)
		if !d.Allow || d.CarveOut != CarveOutEnvAllow {
			t.Fatalf("mode=%v: Decision = %+v; want Allow via CarveOutEnvAllow", mode, d)
		}
	}
}

func TestEvaluate_EnvAllow_CaseInsensitiveTruthyValues(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	run(t, dir, "add", "big.go")
	for _, val := range []string{"1", "true", "TRUE", "yes", "Yes"} {
		t.Setenv("LOGMIND_ALLOW_GIT_COMMIT", val)
		d := Evaluate(dir, "subject", 20, StagedOnly)
		if !d.Allow || d.CarveOut != CarveOutEnvAllow {
			t.Fatalf("val=%q: Decision = %+v; want Allow via CarveOutEnvAllow", val, d)
		}
	}
	for _, val := range []string{"0", "false", "no", ""} {
		t.Setenv("LOGMIND_ALLOW_GIT_COMMIT", val)
		d := Evaluate(dir, "subject", 20, StagedOnly)
		if d.CarveOut == CarveOutEnvAllow {
			t.Fatalf("val=%q: CarveOut = env-allow; want NOT env-allow", val)
		}
	}
}

func TestEvaluate_SkipLogmindMarker(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	run(t, dir, "add", "big.go")
	for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
		d := Evaluate(dir, "wip: quick fix [skip-logmind]", 20, mode)
		if !d.Allow || d.CarveOut != CarveOutSkipLogmind {
			t.Fatalf("mode=%v: Decision = %+v; want Allow via CarveOutSkipLogmind", mode, d)
		}
	}
}

func TestEvaluate_RebaseInProgress(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	run(t, dir, "add", "big.go")
	gitDir, err := gitcli.GitDir(dir)
	if err != nil {
		t.Fatalf("GitDir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "rebase-merge"), 0o755); err != nil {
		t.Fatalf("mkdir rebase-merge: %v", err)
	}
	for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
		d := Evaluate(dir, "subject", 20, mode)
		if !d.Allow || d.CarveOut != CarveOutRebaseInProgress {
			t.Fatalf("mode=%v: Decision = %+v; want Allow via CarveOutRebaseInProgress", mode, d)
		}
	}
}

func TestEvaluate_RebaseApplyInProgress(t *testing.T) {
	dir := initRepo(t)
	gitDir, err := gitcli.GitDir(dir)
	if err != nil {
		t.Fatalf("GitDir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "rebase-apply"), 0o755); err != nil {
		t.Fatalf("mkdir rebase-apply: %v", err)
	}
	d := Evaluate(dir, "subject", 20, StagedOnly)
	if !d.Allow || d.CarveOut != CarveOutRebaseInProgress {
		t.Fatalf("Decision = %+v; want Allow via CarveOutRebaseInProgress", d)
	}
}

func TestEvaluate_MergeInProgress(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	run(t, dir, "add", "big.go")
	gitDir, err := gitcli.GitDir(dir)
	if err != nil {
		t.Fatalf("GitDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"), []byte("abc123\n"), 0o644); err != nil {
		t.Fatalf("write MERGE_HEAD: %v", err)
	}
	for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
		d := Evaluate(dir, "subject", 20, mode)
		if !d.Allow || d.CarveOut != CarveOutMergeInProgress {
			t.Fatalf("mode=%v: Decision = %+v; want Allow via CarveOutMergeInProgress", mode, d)
		}
	}
}

func TestEvaluate_CherryPickInProgress(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	run(t, dir, "add", "big.go")
	gitDir, err := gitcli.GitDir(dir)
	if err != nil {
		t.Fatalf("GitDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "CHERRY_PICK_HEAD"), []byte("abc123\n"), 0o644); err != nil {
		t.Fatalf("write CHERRY_PICK_HEAD: %v", err)
	}
	for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
		d := Evaluate(dir, "subject", 20, mode)
		if !d.Allow || d.CarveOut != CarveOutCherryPickOrRevert {
			t.Fatalf("mode=%v: Decision = %+v; want Allow via CarveOutCherryPickOrRevert", mode, d)
		}
	}
}

func TestEvaluate_RevertInProgress(t *testing.T) {
	dir := initRepo(t)
	gitDir, err := gitcli.GitDir(dir)
	if err != nil {
		t.Fatalf("GitDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "REVERT_HEAD"), []byte("abc123\n"), 0o644); err != nil {
		t.Fatalf("write REVERT_HEAD: %v", err)
	}
	d := Evaluate(dir, "subject", 20, StagedOnly)
	if !d.Allow || d.CarveOut != CarveOutCherryPickOrRevert {
		t.Fatalf("Decision = %+v; want Allow via CarveOutCherryPickOrRevert", d)
	}
}

// wellFormedEntry is what a change that RECORDED a decision writes: a
// title, a timestamp, and a reasoning section that is not empty (SPEC
// §3.1, as §3.4 requires the gate to check). Tests that mean "a decision
// was recorded" stage this; tests that mean "a decision-shaped file was
// staged" stage something else.
const wellFormedEntry = "## 2026-08-07 14:30 - Share one evaluation\n\n" +
	"**Reasoning:** Two callers of one path predicate gave two answers.\n\n" +
	"---\n"

// TestEvaluate_DecisionFileStaged — the legacy pre-§3.2 shape, and the
// case the fix for the sentinel hole must NOT regress: a repository whose
// docs/decisions.md is a real decision log clears the gate by appending a
// real entry to it. Those repositories exist and their commits still pass.
func TestEvaluate_DecisionFileStaged(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	writeFile(t, dir, "docs/decisions.md", wellFormedEntry)
	run(t, dir, "add", "big.go", "docs/decisions.md")
	for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
		d := Evaluate(dir, "subject", 20, mode)
		if !d.Allow || d.CarveOut != CarveOutDecisionRecorded {
			t.Fatalf("mode=%v: Decision = %+v; want Allow via CarveOutDecisionRecorded", mode, d)
		}
	}
}

func TestEvaluate_BranchDecisionFileStaged(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	writeFile(t, dir, "docs/decisions-branches/feature.md", wellFormedEntry)
	run(t, dir, "add", "big.go", "docs/decisions-branches/feature.md")
	d := Evaluate(dir, "subject", 20, StagedOnly)
	if !d.Allow || d.CarveOut != CarveOutDecisionRecorded {
		t.Fatalf("Decision = %+v; want Allow via CarveOutDecisionRecorded", d)
	}
}

// TestEvaluate_StagedDecisionFileWithNoEntryStillBlocks is the regression
// pin for the hole a round-14 panel found: carve-out 5 asked only whether
// a decision-shaped PATH was staged, so `git add docs/decisions.md` — the
// content-free v1.2.0 install sentinel `logmind init` itself writes, which
// says in its own body that it holds no decisions — cleared the commit
// gate for any amount of code.
//
// Measured on the PR head with the shipped binary: 302 lines of new Go,
// sentinel staged, `guard-commit --layer git-hook` exit 0 "allowed
// (decision-file-staged)"; with the same sentinel UNSTAGED, exit 65. The
// gate's answer turned on whether a file logmind wrote was named in the
// index.
//
// Both diff modes, because both hook layers are served by this one
// function and only the line count differs between them.
func TestEvaluate_StagedDecisionFileWithNoEntryStillBlocks(t *testing.T) {
	// The shipped sentinel's REAL bytes — the template `logmind init`
	// installs — not a paraphrase of them, so this pins the file rather
	// than the test author's memory of it. If a future edit ever gives
	// the sentinel a `## <date> <time> - <title>` line and a reasoning
	// section, this is what says so.
	sentinel := templates.DecisionsPointerTemplate()

	for _, tc := range []struct{ name, path, body string }{
		{"the v1.2.0 install sentinel", "docs/decisions.md", sentinel},
		{"a single meaningless line", "docs/decisions.md", ".\n"},
		{"a branch file with a header but no reasoning", "docs/decisions-branches/feature.md",
			"## 2026-08-07 14:30 - Untitled thought\n\n---\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
				dir := initRepo(t)
				writeFile(t, dir, "big.go", linesOf(50))
				writeFile(t, dir, tc.path, tc.body)
				run(t, dir, "add", "big.go", tc.path)

				d := Evaluate(dir, "subject", 20, mode)
				if d.Allow {
					t.Fatalf("mode=%v: Decision = %+v; want Block — staging %s records no decision, "+
						"and %d lines of code would land undocumented", mode, d, tc.path, 50)
				}
				// The message the author actually reads has to say which
				// file was staged in vain, or the block reads as spurious
				// with the decision file sitting right there in the index.
				if !strings.Contains(d.Reason, tc.path) {
					t.Errorf("mode=%v: Reason does not name the staged-but-empty decision file %q; got %q",
						mode, tc.path, d.Reason)
				}
				for _, hatch := range []string{"logmind log", "skip-logmind", "LOGMIND_ALLOW_GIT_COMMIT"} {
					if !strings.Contains(d.Reason, hatch) {
						t.Errorf("mode=%v: Reason drops the %q escape hatch (SPEC §3.4); got %q", mode, hatch, d.Reason)
					}
				}
			}
		})
	}
}

// TestDecisionRecorded_IsTheOnlyQuestion covers the shared primitive
// directly, including the two halves that must BOTH hold and the error
// propagation the CI gate depends on (an unresolvable ref must not read as
// an empty diff).
func TestDecisionRecorded_IsTheOnlyQuestion(t *testing.T) {
	entry := []gitcli.AddedHunk{strings.Split(strings.TrimRight(wellFormedEntry, "\n"), "\n")}

	t.Run("a well-formed entry in a decision file records a decision", func(t *testing.T) {
		ev, err := DecisionRecorded([]string{"src/main.go", "docs/decisions-branches/feature.md"},
			func(path string) ([]gitcli.AddedHunk, error) { return entry, nil })
		if err != nil || !ev.Recorded {
			t.Fatalf("DecisionRecorded = %+v, %v; want Recorded", ev, err)
		}
		if len(ev.Touched) != 0 {
			t.Errorf("Touched = %v; want empty when a decision was recorded", ev.Touched)
		}
	})

	t.Run("the same entry in a NON-decision file records nothing", func(t *testing.T) {
		ev, err := DecisionRecorded([]string{"README.md"},
			func(path string) ([]gitcli.AddedHunk, error) { return entry, nil })
		if err != nil || ev.Recorded {
			t.Fatalf("DecisionRecorded = %+v, %v; want not Recorded", ev, err)
		}
	})

	t.Run("a decision file with nothing well-formed added is Touched, not Recorded", func(t *testing.T) {
		ev, err := DecisionRecorded([]string{"docs/decisions.md"},
			func(path string) ([]gitcli.AddedHunk, error) {
				return []gitcli.AddedHunk{{"# Decision Log", "no entries here"}}, nil
			})
		if err != nil || ev.Recorded {
			t.Fatalf("DecisionRecorded = %+v, %v; want not Recorded", ev, err)
		}
		if len(ev.Touched) != 1 || ev.Touched[0] != "docs/decisions.md" {
			t.Errorf("Touched = %v; want [docs/decisions.md]", ev.Touched)
		}
	})

	t.Run("the reader's error is propagated, not swallowed", func(t *testing.T) {
		want := errors.New("bad ref")
		ev, err := DecisionRecorded([]string{"docs/decisions.md"},
			func(path string) ([]gitcli.AddedHunk, error) { return nil, want })
		if !errors.Is(err, want) {
			t.Fatalf("err = %v; want %v — a diff git could not read must not report as 'no decision'", err, want)
		}
		if ev.Recorded {
			t.Errorf("Recorded = true on an error; want false")
		}
	})

	t.Run("only decision files are read at all", func(t *testing.T) {
		var asked []string
		if _, err := DecisionRecorded([]string{"src/main.go", "docs/plan.md", "docs/decisions.md"},
			func(path string) ([]gitcli.AddedHunk, error) { asked = append(asked, path); return nil, nil }); err != nil {
			t.Fatalf("DecisionRecorded: %v", err)
		}
		if len(asked) != 1 || asked[0] != "docs/decisions.md" {
			t.Errorf("read %v; want only docs/decisions.md", asked)
		}
	})
}

func TestEvaluate_UnderThreshold_Allows(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "small.go", linesOf(5))
	run(t, dir, "add", "small.go")
	d := Evaluate(dir, "subject", 20, StagedOnly)
	if !d.Allow || d.CarveOut != CarveOutUnderThreshold {
		t.Fatalf("Decision = %+v; want Allow via CarveOutUnderThreshold", d)
	}
	if d.Lines != 5 {
		t.Fatalf("Lines = %d; want 5", d.Lines)
	}
}

func TestEvaluate_OverThreshold_Blocks(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(25))
	run(t, dir, "add", "big.go")
	d := Evaluate(dir, "subject", 20, StagedOnly)
	if d.Allow {
		t.Fatalf("Decision = %+v; want Block", d)
	}
	if d.Lines != 25 {
		t.Fatalf("Lines = %d; want 25", d.Lines)
	}
	if d.Reason == "" {
		t.Fatalf("Reason is empty; want a block explanation")
	}
	if !strings.Contains(d.Reason, "logmind log") || !strings.Contains(d.Reason, "skip-logmind") || !strings.Contains(d.Reason, "LOGMIND_ALLOW_GIT_COMMIT") {
		t.Fatalf("Reason = %q; want it to mention logmind log, skip-logmind, and LOGMIND_ALLOW_GIT_COMMIT", d.Reason)
	}
}

func TestEvaluate_DocsOnlyChange_UnderThreshold(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "docs/random.md", linesOf(50))
	run(t, dir, "add", "docs/random.md")
	d := Evaluate(dir, "subject", 20, StagedOnly)
	if !d.Allow || d.CarveOut != CarveOutUnderThreshold || d.Lines != 0 {
		t.Fatalf("Decision = %+v; want Allow/under-threshold with Lines=0 (docs/ is exempt)", d)
	}
}

// --- the key StagedOnly vs. WorkingTreeUnion regression ----------------

// TestEvaluate_UnstagedChange_BlockUnderUnion_AllowUnderStaged is THE
// regression this package exists to prevent: a `git add -A && git commit`
// shape where, at the moment the harness's PreToolUse hook fires, the
// change under review is still sitting entirely UNSTAGED. StagedOnly
// (correct for the git commit-msg hook, where the index is already final)
// sees an empty index and allows; WorkingTreeUnion (correct for the
// harness, which runs BEFORE `git add -A` stages anything) must still
// catch it.
func TestEvaluate_UnstagedChange_BlockUnderUnion_AllowUnderStaged(t *testing.T) {
	dir := initRepo(t)
	// Modify a TRACKED file (README.md starts as the 1-line "hello\n" from
	// initRepo) without staging it: 30 added + 1 removed = 31 changed lines.
	writeFile(t, dir, "README.md", linesOf(30))

	staged := Evaluate(dir, "subject", 20, StagedOnly)
	if !staged.Allow {
		t.Fatalf("StagedOnly: Decision = %+v; want Allow (nothing staged yet)", staged)
	}

	union := Evaluate(dir, "subject", 20, WorkingTreeUnion)
	if union.Allow {
		t.Fatalf("WorkingTreeUnion: Decision = %+v; want Block (unstaged change must still be caught)", union)
	}
	if union.Lines != 31 {
		t.Fatalf("WorkingTreeUnion: Lines = %d; want 31 (30 added + 1 removed)", union.Lines)
	}
}

// TestEvaluate_UntrackedOnlyLargeFile_BlockUnderUnion covers the other
// half of the harness's blind spot: a brand-new file that was never even
// `git add`ed. StagedOnly sees nothing at all (correctly, for its use
// case); WorkingTreeUnion must count it.
func TestEvaluate_UntrackedOnlyLargeFile_BlockUnderUnion(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "brand_new.go", linesOf(40))

	staged := Evaluate(dir, "subject", 20, StagedOnly)
	if !staged.Allow {
		t.Fatalf("StagedOnly: Decision = %+v; want Allow (untracked file is invisible to --cached)", staged)
	}

	union := Evaluate(dir, "subject", 20, WorkingTreeUnion)
	if union.Allow {
		t.Fatalf("WorkingTreeUnion: Decision = %+v; want Block (untracked-only large file must be caught)", union)
	}
	if union.Lines != 40 {
		t.Fatalf("WorkingTreeUnion: Lines = %d; want 40", union.Lines)
	}
}

// TestEvaluate_WorkingTreeUnion_SumsAllThreeSources sanity-checks that
// WorkingTreeUnion really does UNION staged + unstaged-tracked +
// untracked, rather than only catching one of the three.
func TestEvaluate_WorkingTreeUnion_SumsAllThreeSources(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "staged.go", linesOf(3))
	run(t, dir, "add", "staged.go")
	// README.md starts as the 1-line "hello\n" from initRepo, so replacing
	// it with 4 lines is 4 added + 1 removed = 5 changed lines.
	writeFile(t, dir, "README.md", linesOf(4)) // tracked, unstaged
	writeFile(t, dir, "untracked.go", linesOf(5))

	d := Evaluate(dir, "subject", 1, WorkingTreeUnion)
	if d.Allow {
		t.Fatalf("Decision = %+v; want Block", d)
	}
	if d.Lines != 13 {
		t.Fatalf("Lines = %d; want 13 (3 staged + 5 unstaged-tracked[4add+1rm] + 5 untracked)", d.Lines)
	}
}

// --- carve-out precedence (first match wins) ----------------------------

func TestEvaluate_CarveOutPrecedence_EnvBeatsSkipMarkerCheck(t *testing.T) {
	// Not a meaningful ordering distinction on its own (both allow), but
	// confirms env is checked ahead of subject parsing without requiring
	// the subject to even be examined.
	dir := initRepo(t)
	t.Setenv("LOGMIND_ALLOW_GIT_COMMIT", "1")
	d := Evaluate(dir, "", 20, StagedOnly)
	if !d.Allow || d.CarveOut != CarveOutEnvAllow {
		t.Fatalf("Decision = %+v; want Allow via CarveOutEnvAllow even with empty subject", d)
	}
}

func TestEvaluate_InProgressStateBeatsDecisionFileCheck(t *testing.T) {
	dir := initRepo(t)
	// A RECORDED decision, so both carve-outs genuinely apply and the
	// assertion below is about their order rather than about only one of
	// them being able to fire.
	writeFile(t, dir, "docs/decisions.md", wellFormedEntry)
	run(t, dir, "add", "docs/decisions.md")
	gitDir, err := gitcli.GitDir(dir)
	if err != nil {
		t.Fatalf("GitDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"), []byte("abc\n"), 0o644); err != nil {
		t.Fatalf("write MERGE_HEAD: %v", err)
	}
	d := Evaluate(dir, "subject", 20, StagedOnly)
	// Either carve-out would Allow; assert the DOCUMENTED order actually
	// fires (merge-in-progress, checked before the staged-files loop).
	if !d.Allow || d.CarveOut != CarveOutMergeInProgress {
		t.Fatalf("Decision = %+v; want Allow via CarveOutMergeInProgress (checked before decision-recorded)", d)
	}
}

// --- isDecisionFile / SubstantiveLines (moved verbatim from
// check_decisions.go — behavior-preserving unit coverage lives here now) --
//
// isDecisionFile is HALF of the gate's question (see DecisionRecorded, and
// TestEvaluate_StagedDecisionFileWithNoEntryStillBlocks for what shipping
// the half alone cost). The path rules below are still worth pinning: a
// path this predicate rejects can never record a decision no matter what
// the change wrote into it.
//
// The two `true` rows are the ONLY two shapes resolveDecisionsPath
// produces. Every `false` row below except README.md / src/decisions.txt
// used to be `true`: the predicate accepted a `/decisions.md` suffix in any
// directory and any depth under docs/decisions-branches/, which is the hole
// TestEvaluate_DecoyDecisionFileElsewhereStillBlocks measures end to end.

func TestIsDecisionFile(t *testing.T) {
	cases := map[string]bool{
		"docs/decisions.md":                  true,
		"docs/decisions-branches/feature.md": true,
		// Named for a branch with a slash in it — sanitizeBranchName turns
		// `fix/gate` into `fix__gate`, so this is a real write target.
		"docs/decisions-branches/fix__gate.md": true,

		// The reproduced bypass: a decision-shaped file anywhere else.
		"internal/x/decisions.md":  false,
		"nested/path/decisions.md": false,
		"decisions.md":             false,
		"vendor/docs/decisions.md": false,
		// Under the branch dir but not a file any read path enumerates —
		// ListBranchFiles skips subdirectories.
		"docs/decisions-branches/nested/other.md": false,
		// Under the branch dir but not markdown.
		"docs/decisions-branches/notes.txt": false,

		"docs/decisions-archive.md": false,
		"README.md":                 false,
		"src/decisions.txt":         false,
	}
	for path, want := range cases {
		if got := isDecisionFile(path); got != want {
			t.Errorf("isDecisionFile(%q) = %v; want %v", path, got, want)
		}
	}
}

// TestIsDecisionFile_AgreesWithResolveDecisionsPath is the fence that keeps
// the gate and the writer from drifting apart again. It cannot import
// internal/cli (that would be an import cycle), so it asserts the two
// shapes against the LAYOUT CONSTANTS resolveDecisionsPath builds from — if
// a future edit renames docs/, decisions.md or decisions-branches/ in one
// place only, this fails.
func TestIsDecisionFile_AgreesWithResolveDecisionsPath(t *testing.T) {
	// resolveDecisionsPath's branchlessPath: filepath.Join(docsPath, LegacyFileName).
	branchless := decisions.DocsDirName + "/" + decisions.LegacyFileName
	// resolveDecisionsPath's branchFile: filepath.Join(docsPath, BranchDirName, sanitized+".md").
	branchFile := decisions.DocsDirName + "/" + decisions.BranchDirName + "/main.md"
	for _, p := range []string{branchless, branchFile} {
		if !isDecisionFile(p) {
			t.Errorf("isDecisionFile(%q) = false; resolveDecisionsPath writes there", p)
		}
	}
}

// TestEvaluate_DecoyDecisionFileElsewhereStillBlocks is the regression pin
// for the reproduced bypass, and it pins the OUTPUT the operator saw rather
// than the helper underneath it: a well-formed §3.1 entry written to
// internal/x/decisions.md — a path no read path enumerates and `logmind
// log` will never write — alongside 302 lines of new Go cleared
// `guard-commit --layer git-hook` with exit 0 "allowed (decision-recorded)"
// while the identical index without the decoy was refused with exit 65.
//
// The CONTROL is in the same test on purpose. "Blocked" is only evidence
// if the same tree minus the decoy is also blocked for the same reason and
// the same tree with a REAL entry passes — otherwise a predicate that
// rejects everything scores identically.
func TestEvaluate_DecoyDecisionFileElsewhereStillBlocks(t *testing.T) {
	decoys := []string{
		"internal/x/decisions.md",
		"decisions.md",
		"docs/decisions-branches/nested/other.md",
	}
	for _, decoy := range decoys {
		t.Run(decoy, func(t *testing.T) {
			for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
				dir := initRepo(t)
				writeFile(t, dir, "big.go", linesOf(302))
				writeFile(t, dir, decoy, wellFormedEntry)
				run(t, dir, "add", "big.go", decoy)

				if d := Evaluate(dir, "subject", 20, mode); d.Allow {
					t.Fatalf("mode=%v: Decision = %+v; want Block — %s is not a file logmind "+
						"records decisions in, so 302 lines of Go would land undocumented",
						mode, d, decoy)
				}
			}
		})
	}

	// CONTROL: the same 302 lines with the same entry written where
	// `logmind log` actually writes it still passes. Without this, the test
	// above is also passed by a predicate that answers false to everything.
	for _, real := range []string{"docs/decisions.md", "docs/decisions-branches/feature.md"} {
		t.Run("control "+real, func(t *testing.T) {
			dir := initRepo(t)
			writeFile(t, dir, "big.go", linesOf(302))
			writeFile(t, dir, real, wellFormedEntry)
			run(t, dir, "add", "big.go", real)

			d := Evaluate(dir, "subject", 20, StagedOnly)
			if !d.Allow || d.CarveOut != CarveOutDecisionRecorded {
				t.Fatalf("Decision = %+v; want Allow via CarveOutDecisionRecorded — an entry "+
					"appended to %s IS a recorded decision", d, real)
			}
		})
	}
}

func TestSubstantiveLines_SkipsDocsAndBinaryAndUnparseable(t *testing.T) {
	rows := []gitcli.NumstatLine{
		{Added: "10", Removed: "2", Path: "main.go"},
		{Added: "5", Removed: "0", Path: "docs/readme.md"}, // skipped: docs/
		{Added: "-", Removed: "-", Path: "image.png"},      // skipped: binary
		{Added: "x", Removed: "y", Path: "weird.go"},       // skipped: unparseable
		{Added: "3", Removed: "1", Path: "other.go"},
	}
	got := SubstantiveLines(rows)
	want := 10 + 2 + 3 + 1
	if got != want {
		t.Fatalf("SubstantiveLines = %d; want %d", got, want)
	}
}

// --- SPEC §3.4's exclusion table, one case per row --------------------
//
// "Four things are excluded from the count, and nothing else is."
// Everything below is a regression PIN, not incidental coverage: each
// row is a rule the SPEC states, and a change that flips one has changed
// what the gate enforces.

// TestIsExcludedPath_DocsTree pins exclusion 1 — "everything under
// docs/", the decision record and the documents derived from it.
func TestIsExcludedPath_DocsTree(t *testing.T) {
	for _, p := range []string{
		"docs/plan.md",
		"docs/timeline.md",
		"docs/decisions-branches/feature__x.md",
		"docs/nested/deep/thing.go",
	} {
		if !IsExcludedPath(p) {
			t.Errorf("IsExcludedPath(%q) = false; want true (SPEC §3.4: everything under docs/)", p)
		}
	}
}

// TestIsExcludedPath_InstructionAndPerToolFiles pins exclusion 2 —
// AGENTS.md and the per-tool files of SPEC §1.2. The expected set is
// read from the agents registry rather than retyped here, because a
// second copy of the list is exactly the defect §3.4 warns about ("two
// lists that mean the same thing are two lists that will disagree") and
// a test that carries its own copy would pass while the two drifted.
func TestIsExcludedPath_InstructionAndPerToolFiles(t *testing.T) {
	if !IsExcludedPath("AGENTS.md") {
		t.Errorf("IsExcludedPath(%q) = false; want true (SPEC §1.1 instruction file)", "AGENTS.md")
	}
	patterns := agents.FilePatterns()
	if len(patterns) == 0 {
		t.Fatal("agents.FilePatterns() is empty — the exclusion set has no source")
	}
	for _, p := range patterns {
		if !IsExcludedPath(p) {
			t.Errorf("IsExcludedPath(%q) = false; want true (SPEC §1.2 per-tool file)", p)
		}
	}
}

// TestIsExcludedPath_ToolchainConfig pins exclusion 3 — "the toolchain's
// own configuration."
func TestIsExcludedPath_ToolchainConfig(t *testing.T) {
	for _, p := range []string{".logmind/config.yml", ".logmind/nested/whatever.yml"} {
		if !IsExcludedPath(p) {
			t.Errorf("IsExcludedPath(%q) = false; want true (SPEC §3.4: the toolchain's own configuration)", p)
		}
	}
}

// TestSubstantiveLines_BinaryRowsExcluded pins exclusion 4 — "any file
// the forge reports as binary," which git's numstat marks with "-"
// rather than a count. It is a numstat rule, not a path rule, so it
// lives on SubstantiveLines and not on IsExcludedPath.
func TestSubstantiveLines_BinaryRowsExcluded(t *testing.T) {
	if IsExcludedPath("assets/logo.png") {
		t.Error("IsExcludedPath(\"assets/logo.png\") = true; binary is a numstat rule, not a path rule")
	}
	rows := []gitcli.NumstatLine{{Added: "-", Removed: "-", Path: "assets/logo.png"}}
	if got := SubstantiveLines(rows); got != 0 {
		t.Fatalf("SubstantiveLines(binary row) = %d; want 0", got)
	}
}

// TestSubstantiveLines_MarkdownOutsideDocsCounts is the load-bearing
// negative of the table: markdown is NOT excluded for being markdown.
//
// SPEC §3.4: "A skill file counts. So does an agent definition. ...
// Excluding markdown wholesale switches the rule off in the repositories
// where writing *is* the work." The `*.md` arm the CI workflow carries
// today is the bug this pins against.
func TestSubstantiveLines_MarkdownOutsideDocsCounts(t *testing.T) {
	rows := []gitcli.NumstatLine{
		{Added: "40", Removed: "0", Path: ".claude/skills/logmind/SKILL.md"},
		{Added: "30", Removed: "0", Path: ".claude/agents/reviewer.md"},
		{Added: "20", Removed: "0", Path: "README.md"},
		{Added: "10", Removed: "0", Path: ".github/PULL_REQUEST_TEMPLATE.md"},
	}
	for _, row := range rows {
		if IsExcludedPath(row.Path) {
			t.Errorf("IsExcludedPath(%q) = true; want false — markdown outside docs/ counts (SPEC §3.4)", row.Path)
		}
	}
	if got, want := SubstantiveLines(rows), 100; got != want {
		t.Fatalf("SubstantiveLines = %d; want %d", got, want)
	}
}

// --- §3.4's well-formedness requirement on the gate -------------------

// TestWellFormedDecisionAdded pins "a decision clears the gate by being
// written, not by existing": title + timestamp + non-empty reasoning in
// the lines the diff ADDED, and nothing less.
func TestWellFormedDecisionAdded(t *testing.T) {
	cases := []struct {
		name  string
		added gitcli.AddedHunk
		want  bool
	}{
		{
			name: "complete entry",
			added: []string{
				"## 2026-08-07 14:30 - Share one evaluation",
				"",
				"**Reasoning:** Two lists that mean the same thing will disagree.",
				"",
				"---",
			},
			want: true,
		},
		{
			// The shape §3.4 names outright: "a test that asks only
			// whether the file was touched is passed by a single
			// meaningless line."
			name:  "single meaningless line",
			added: []string{"."},
			want:  false,
		},
		{
			name:  "header only, no reasoning",
			added: []string{"## 2026-08-07 14:30 - Untitled thought", "", "---"},
			want:  false,
		},
		{
			name:  "reasoning header with nothing under it",
			added: []string{"## 2026-08-07 14:30 - Empty", "", "**Reasoning:**", "", "---"},
			want:  false,
		},
		{
			name:  "reasoning without a decision header",
			added: []string{"**Reasoning:** orphaned prose with no entry above it"},
			want:  false,
		},
		{
			name:  "malformed timestamp",
			added: []string{"## 2026-13-45 99:99 - Bad clock", "", "**Reasoning:** nope.", "", "---"},
			want:  false,
		},
		{
			name:  "no seconds allowed — §3.1 forbids them, so the header is not one",
			added: []string{"## 2026-08-07 14:30:11 - Seconds", "", "**Reasoning:** nope.", "", "---"},
			want:  false,
		},
		{
			name:  "nothing added at all",
			added: nil,
			want:  false,
		},
		{
			name: "reasoning wrapped onto the following line",
			added: []string{
				"## 2026-08-07 14:30 - Wrapped",
				"",
				"**Reasoning:**",
				"the prose starts on the next line instead.",
				"",
				"---",
			},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WellFormedDecisionAdded([]gitcli.AddedHunk{tc.added}); got != tc.want {
				t.Fatalf("WellFormedDecisionAdded(%q) = %v; want %v", tc.added, got, tc.want)
			}
		})
	}
}

// TestWellFormedDecisionAdded_HunksAreJudgedSeparately is the regression
// pin for the round-16 bypass. The reader returned ONE flat list of added
// lines, so `-U0` hunks that git had every reason to keep apart —
// separated by content the change never touched — arrived concatenated.
// A section opened at the end of one hunk was then satisfied by the first
// prose in the next, and the gate reported an entry that is not in the
// file.
//
// Measured on the PR head, hook installed and the freshly built binary
// first on PATH: an empty `**Reasoning:**` in hunk 1 plus one unrelated
// bullet added further down the same file → `git commit` exit 0 "allowed
// (decision-recorded)" and `check-decisions --base --head` exit 0, for
// 302 lines of new Go whose reasoning section is visibly empty on disk.
// The identical change minus that second hunk → exit 65 and exit 1.
//
// The three rows are the same content cut three ways, so what they
// isolate is the boundary and nothing else.
func TestWellFormedDecisionAdded_HunksAreJudgedSeparately(t *testing.T) {
	openedSection := gitcli.AddedHunk{
		"## 2026-08-14 09:00 - Collapse the decision layout",
		"",
		reasoningMarker,
		"",
	}
	unrelatedProse := gitcli.AddedHunk{"- whether the parser reads a section the way a person does"}

	t.Run("prose from a later hunk does not fill an earlier hunk's section", func(t *testing.T) {
		if WellFormedDecisionAdded([]gitcli.AddedHunk{openedSection, unrelatedProse}) {
			t.Fatal("an empty reasoning section was satisfied by a line added elsewhere in the file — " +
				"302 lines of code clear both gates on an entry that documents nothing")
		}
	})

	t.Run("control: the same lines with nothing after the header still fail", func(t *testing.T) {
		if WellFormedDecisionAdded([]gitcli.AddedHunk{openedSection}) {
			t.Fatal("an empty reasoning section cleared the gate on its own; the row above measures nothing")
		}
	})

	t.Run("control: the same lines IN ONE hunk pass, because then they are in the file", func(t *testing.T) {
		together := append(append(gitcli.AddedHunk{}, openedSection...), unrelatedProse...)
		if !WellFormedDecisionAdded([]gitcli.AddedHunk{together}) {
			t.Fatal("a reasoning body written directly under its header was refused — " +
				"the fix has stopped judging adjacency and started refusing everything")
		}
	})
}

// TestWellFormedDecisionAdded_ShippedSentinel is the control on the
// table above: the §3.2 install sentinel's REAL bytes, split the way a
// diff hands them over, must not clear the gate. Round 14 closed that
// hole; every later loosening of the section-boundary rule has to prove
// it is still closed, and against the file rather than a paraphrase.
func TestWellFormedDecisionAdded_ShippedSentinel(t *testing.T) {
	split := func(body string) []gitcli.AddedHunk {
		return []gitcli.AddedHunk{strings.Split(strings.TrimRight(body, "\n"), "\n")}
	}
	if WellFormedDecisionAdded(split(templates.DecisionsPointerTemplate())) {
		t.Errorf("the shipped docs/decisions.md sentinel clears the gate; it carries no entry:\n%s",
			templates.DecisionsPointerTemplate())
	}
	// Control on the control: the same call on a known-passing input, so
	// the line above is evidence about the sentinel rather than about a
	// predicate that says no to everything.
	if !WellFormedDecisionAdded(split(wellFormedEntry)) {
		t.Fatal("the control entry does not clear the gate either — the check above measures nothing")
	}
}

// TestHasReasoning_SectionBoundaries pins where a §3.1 section ends —
// at the next section header or at the entry terminator, and at NEITHER
// a blank line nor a fixed offset from the marker. Two holes, opposite
// directions, and the rows below hold both shut at once.
//
// Too loose (PR #287, found by an adversarial review): the body
// accumulator stopped only at a blank line, so an entry whose next
// section header is NOT blank-separated had that header's own text
// swallowed as the previous section's body — making an empty
// **Reasoning:** read as non-empty. §3.1 defines that shape as an empty
// section and §3.4 rejects exactly this "single meaningless line".
//
// Too tight (PR #301 round 14, found in the field): the accumulator ALSO
// stopped at a blank line, so an ordinary entry whose body sits below one
// read as an EMPTY section and the commit was refused. Measured across
// four identically-built repos: reasoning inline → exit 0; wrapped with
// no blank → 0; a blank after the marker → 65; a bullet list after a
// blank → 65. §3.1 forbids the assumption behind it outright — a consumer
// "MUST NOT require a section order beyond the title coming first and
// `---` terminating the entry."
//
// One blank line was all that separated a caught case from an uncaught
// one in the first hole, and a working entry from a blocked one in the
// second.
func TestHasReasoning_SectionBoundaries(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "empty reasoning, next header not blank-separated",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\n**Alternatives considered:** none\n\n---\n",
			want: false,
		},
		{
			name: "empty reasoning, entry terminator immediately after",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\n---\n",
			want: false,
		},
		{
			name: "real reasoning on the marker line",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:** because\n\n---\n",
			want: true,
		},
		{
			name: "real reasoning wrapped onto following lines",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\nit wrapped onto\nthe next line\n\n---\n",
			want: true,
		},
		{
			name: "wrapped reasoning still ends at an unseparated next header",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\nreal content\n**Implications:** x\n\n---\n",
			want: true,
		},
		// --- a blank line is a PARAGRAPH BREAK, not the end of a section.
		// These four are the round-14 regression: the scan stopped at the
		// first blank line, so an ordinary entry whose body sits below one
		// read as an empty section and its commit was refused (exit 65,
		// measured). The two `want: false` rows are the control — the
		// loosening must not let an actually-empty section through.
		{
			name: "reasoning body below a blank line",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\n\nThree sentences that say exactly why.\n\n---\n",
			want: true,
		},
		{
			name: "reasoning body is a bullet list below a blank line",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\n\n- one\n- two\n\n---\n",
			want: true,
		},
		{
			name: "empty reasoning, blank line, then the next header",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\n\n**Alternatives considered:** none\n\n---\n",
			want: false,
		},
		{
			name: "empty reasoning, blank line, then the terminator",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\n\n---\n",
			want: false,
		},
		{
			// §3.1: a consumer "MUST NOT require a section order beyond
			// the title coming first". Reasoning last, below a section
			// this build has never heard of, still counts.
			name: "reasoning last, after an unknown section",
			raw:  "## 2026-08-07 10:00 - t\n\n**Provenance:** a section logmind never emits\n\n**Reasoning:**\n\nwhy, at the bottom\n\n---\n",
			want: true,
		},
		// --- a bolded LEAD-IN is body, not the next section's header.
		// These are the round-16 false rejection: the boundary test
		// matched anything that opened a bold run and carried a colon,
		// so the most ordinary way to start a reasoning paragraph read
		// as an empty section and the commit was refused. Measured, with
		// the first content line under a bare **Reasoning:**: plain prose
		// → exit 0; `**Root cause:** ...` → exit 65; `- **Latency:** ...`
		// → exit 0. The last row is the control the loosening must not
		// break — a header §3.1 NAMES still ends the section.
		{
			name: "reasoning body opening with a bolded lead-in",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\n\n**Root cause:** the parser ended a section at the first blank line.\n\n---\n",
			want: true,
		},
		{
			name: "bolded lead-in with no blank line above it",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\n**Root cause:** the section boundary, again.\n\n---\n",
			want: true,
		},
		{
			name: "empty reasoning still ends at a NAMED section, blank line or not",
			raw:  "## 2026-08-07 10:00 - t\n\n**Reasoning:**\n\n**Implications:**\n- the gate would read this bullet as the reason\n\n---\n",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasReasoning(tc.raw); got != tc.want {
				t.Errorf("hasReasoning() = %v, want %v\nraw:\n%s", got, tc.want, tc.raw)
			}
		})
	}
}

// TestIsSectionHeader covers the section-boundary test directly. It is a
// list of the section names SPEC §3.1 NAMES, not a shape: §3.1's "MUST
// NOT require a section order beyond the title coming first" argues
// against requiring an ORDER, and §3.1 names the sections in its own
// template. Matching by name keeps `**Alternatives considered:**` ending
// the section above it while leaving a bolded lead-in like `**Root
// cause:**` as body, which is what the shape version got wrong.
func TestIsSectionHeader(t *testing.T) {
	yes := []string{
		"**Reasoning:**",
		"**Alternatives considered:** none",
		"**Implications:**",
	}
	no := []string{
		"", "plain text", "**bold but no colon**", "*single star:*", "**unterminated:",
		// The round-16 false rejection: a bolded lead-in opening a
		// reasoning paragraph is BODY. Treated as a header, the section
		// above it read as empty and the commit was refused (measured:
		// exit 65 on the hook, exit 1 on the gate).
		"**Root cause:** the parser ended a section at the first blank line.",
		"**Provenance:** a section logmind never emits",
		// A named marker in a bullet is body too — a list item, not the
		// start of a section.
		"- **Implications:** it would end the section from inside a list",
	}
	for _, s := range yes {
		if !isSectionHeader(s) {
			t.Errorf("isSectionHeader(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isSectionHeader(s) {
			t.Errorf("isSectionHeader(%q) = true, want false", s)
		}
	}
}

// TestSectionMarkersMatchTheSpec pins the list against §3.1's template
// rather than against the test author's memory of it: the three headers
// §3.1 prints, in §3.1's spelling. A marker that drifts from the SPEC
// stops ending the section it names, and `logmind log`'s own output is
// what drifts first.
func TestSectionMarkersMatchTheSpec(t *testing.T) {
	want := []string{"**Reasoning:**", "**Alternatives considered:**", "**Implications:**"}
	if len(sectionMarkers) != len(want) {
		t.Fatalf("sectionMarkers = %q; want the %d headers SPEC §3.1 names: %q", sectionMarkers, len(want), want)
	}
	for i, m := range want {
		if sectionMarkers[i] != m {
			t.Errorf("sectionMarkers[%d] = %q; want %q (SPEC §3.1's spelling)", i, sectionMarkers[i], m)
		}
	}
}

// TestSubstantiveLines_RenameOutOfDocsIsCounted pins the gate hole a
// retrospective panel found on `dev` after PR #287 unified the numstat
// flag lists and dropped --no-renames in the process.
//
// With rename detection ON, git renders a cross-directory rename as ONE
// row whose path field is `old => new`:
//
//	150	0	docs/notes.md => src/payload.go
//
// IsExcludedPath prefix-matches that whole string, so the row was
// excluded as `docs/...` and 550 lines of new Go counted zero — the gate
// passed exactly the change it exists to stop. gitcli.numstatFlags now
// carries --no-renames on every count, which splits the row into a
// deletion under docs/ (excluded) and an addition under src/ (counted).
//
// This test pins the CONSUMER side: whatever git hands us, a path that
// still carries the `=>` rename rendering must not be silently treated as
// living under an excluded prefix.
func TestSubstantiveLines_RenameOutOfDocsIsCounted(t *testing.T) {
	// The exact shape git emits when rename detection is on.
	rows := []gitcli.NumstatLine{
		{Added: "150", Removed: "0", Path: "docs/notes.md => src/payload.go"},
	}
	if got := SubstantiveLines(rows); got == 0 {
		t.Errorf("a rename OUT of docs/ counted 0 lines — the gate would pass "+
			"550 lines of new code; got %d, want non-zero", got)
	}

	// Control: a genuine docs-only row must still be excluded, or the fix
	// has simply broken the exclusion it was meant to preserve.
	docsOnly := []gitcli.NumstatLine{
		{Added: "550", Removed: "0", Path: "docs/notes.md"},
	}
	if got := SubstantiveLines(docsOnly); got != 0 {
		t.Errorf("a docs-only change must stay excluded (SPEC §3.4); got %d, want 0", got)
	}

	// Control: the compact rename rendering, which git uses when the two
	// paths share a prefix. It never looked like a bare docs/ path, so it
	// was never part of the hole — but it must not regress either.
	compact := []gitcli.NumstatLine{
		{Added: "150", Removed: "0", Path: "{docs => src}/sub/notes.md"},
	}
	if got := SubstantiveLines(compact); got == 0 {
		t.Errorf("compact rename rendering counted 0; got %d, want non-zero", got)
	}
}
