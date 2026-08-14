package guardcommit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/agents"
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
		added []string
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
			if got := WellFormedDecisionAdded(tc.added); got != tc.want {
				t.Fatalf("WellFormedDecisionAdded(%q) = %v; want %v", tc.added, got, tc.want)
			}
		})
	}
}

// TestHasReasoning_SectionEndsAtNextHeader pins the hole an adversarial
// review found in PR #287: the body accumulator stopped only at a blank
// line, so an entry whose next section header is NOT blank-separated had
// that header's own text swallowed as the previous section's body —
// making an empty **Reasoning:** read as non-empty. SPEC §3.1 defines
// that shape as an empty section, and §3.4 rejects exactly this "single
// meaningless line". One blank line was all that separated a caught case
// from an uncaught one.
func TestHasReasoning_SectionEndsAtNextHeader(t *testing.T) {
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasReasoning(tc.raw); got != tc.want {
				t.Errorf("hasReasoning() = %v, want %v\nraw:\n%s", got, tc.want, tc.raw)
			}
		})
	}
}

// TestIsSectionHeader covers the shape test directly. It is deliberately
// shape-based rather than a fixed list of known section names, because
// SPEC §3.1 says a consumer "MUST NOT require a section order beyond the
// title coming first" — a hand-written entry may carry a section this
// build has never heard of.
func TestIsSectionHeader(t *testing.T) {
	yes := []string{"**Reasoning:**", "**Alternatives considered:** none", "**Anything At All:**"}
	no := []string{"", "plain text", "**bold but no colon**", "*single star:*", "**unterminated:"}
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
