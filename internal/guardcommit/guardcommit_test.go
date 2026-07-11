package guardcommit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/gitcli"
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
	for _, args := range [][]string{
		{"init", "-q"},
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

func TestEvaluate_DecisionFileStaged(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	writeFile(t, dir, "docs/decisions.md", "## decision\n")
	run(t, dir, "add", "big.go", "docs/decisions.md")
	for _, mode := range []DiffMode{StagedOnly, WorkingTreeUnion} {
		d := Evaluate(dir, "subject", 20, mode)
		if !d.Allow || d.CarveOut != CarveOutDecisionFileStaged {
			t.Fatalf("mode=%v: Decision = %+v; want Allow via CarveOutDecisionFileStaged", mode, d)
		}
	}
}

func TestEvaluate_BranchDecisionFileStaged(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "big.go", linesOf(50))
	writeFile(t, dir, "docs/decisions-branches/feature.md", "## decision\n")
	run(t, dir, "add", "big.go", "docs/decisions-branches/feature.md")
	d := Evaluate(dir, "subject", 20, StagedOnly)
	if !d.Allow || d.CarveOut != CarveOutDecisionFileStaged {
		t.Fatalf("Decision = %+v; want Allow via CarveOutDecisionFileStaged", d)
	}
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
	writeFile(t, dir, "docs/decisions.md", "## decision\n")
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
		t.Fatalf("Decision = %+v; want Allow via CarveOutMergeInProgress (checked before decision-file-staged)", d)
	}
}

// --- IsDecisionFile / SubstantiveLines (moved verbatim from
// check_decisions.go — behavior-preserving unit coverage lives here now) --

func TestIsDecisionFile(t *testing.T) {
	cases := map[string]bool{
		"docs/decisions.md":                       true,
		"nested/path/decisions.md":                true,
		"docs/decisions-branches/feature.md":      true,
		"docs/decisions-branches/nested/other.md": true,
		"docs/decisions-archive.md":               false,
		"README.md":                               false,
		"src/decisions.txt":                       false,
	}
	for path, want := range cases {
		if got := IsDecisionFile(path); got != want {
			t.Errorf("IsDecisionFile(%q) = %v; want %v", path, got, want)
		}
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
