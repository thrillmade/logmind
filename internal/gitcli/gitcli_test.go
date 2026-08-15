package gitcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/testgit"
)

// initRepo creates a fresh git repo at t.TempDir(), commits an initial
// README so HEAD is born, and returns the path. Skips the test if `git`
// is not on PATH (CI runners that strip it after compilation, locked-
// down sandboxes, etc.).
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
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestIsRepo_TrueInsideRepo(t *testing.T) {
	if !IsRepo(initRepo(t)) {
		t.Fatalf("IsRepo() = false inside fresh repo")
	}
}

func TestIsRepo_FalseOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	if IsRepo(dir) {
		t.Fatalf("IsRepo(%q) = true; want false (no .git/)", dir)
	}
}

func TestRevParseTopLevel_ReturnsRepoRoot(t *testing.T) {
	dir := initRepo(t)
	top, err := RevParseTopLevel(dir)
	if err != nil {
		t.Fatalf("RevParseTopLevel: %v", err)
	}
	// macOS prepends /private to TempDir paths once resolved; compare
	// via filepath.EvalSymlinks so the test is portable.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("evalsymlinks(%q): %v", dir, err)
	}
	got, err := filepath.EvalSymlinks(top)
	if err != nil {
		t.Fatalf("evalsymlinks(%q): %v", top, err)
	}
	if got != want {
		t.Fatalf("RevParseTopLevel = %q; want %q", got, want)
	}
}

func TestDiffCachedNames_ReturnsStagedPaths(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}
	if err := AddPaths(dir, "new.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	names := DiffCachedNames(dir)
	if len(names) != 1 || names[0] != "new.txt" {
		t.Fatalf("DiffCachedNames = %v; want [new.txt]", names)
	}
}

func TestDiffCachedNumstat_ParsesRows(t *testing.T) {
	dir := initRepo(t)
	var body bytes.Buffer
	for i := 0; i < 10; i++ {
		body.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "code.go"), body.Bytes(), 0o644); err != nil {
		t.Fatalf("write code.go: %v", err)
	}
	if err := AddPaths(dir, "code.go"); err != nil {
		t.Fatalf("add: %v", err)
	}
	rows := DiffCachedNumstat(dir)
	if len(rows) != 1 {
		t.Fatalf("DiffCachedNumstat: got %d rows; want 1 (%+v)", len(rows), rows)
	}
	if rows[0].Path != "code.go" {
		t.Fatalf("row.Path = %q; want code.go", rows[0].Path)
	}
	if rows[0].Added != "10" || rows[0].Removed != "0" {
		t.Fatalf("row counts = (%q, %q); want (10, 0)", rows[0].Added, rows[0].Removed)
	}
}

func TestConfigGet_Set_Roundtrip(t *testing.T) {
	dir := initRepo(t)
	if err := ConfigSet(dir, "merge.logmind-test.driver", "echo test"); err != nil {
		t.Fatalf("ConfigSet: %v", err)
	}
	val, ok := ConfigGet(dir, "merge.logmind-test.driver")
	if !ok || val != "echo test" {
		t.Fatalf("ConfigGet = (%q, %v); want (echo test, true)", val, ok)
	}
}

func TestConfigGet_MissingKey(t *testing.T) {
	dir := initRepo(t)
	val, ok := ConfigGet(dir, "merge.does-not-exist.driver")
	if ok || val != "" {
		t.Fatalf("ConfigGet(missing) = (%q, %v); want ('', false)", val, ok)
	}
}

func TestIsTrackedFile_TrueForCommittedFile(t *testing.T) {
	dir := initRepo(t)
	if !IsTrackedFile(dir, "README.md") {
		t.Fatalf("IsTrackedFile(README.md) = false; want true (committed by initRepo)")
	}
}

func TestIsTrackedFile_FalseForUntrackedFile(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "untracked.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write untracked.md: %v", err)
	}
	if IsTrackedFile(dir, "untracked.md") {
		t.Fatalf("IsTrackedFile(untracked.md) = true; want false (never added)")
	}
}

func TestIsTrackedFile_FalseOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write x.md: %v", err)
	}
	if IsTrackedFile(dir, "x.md") {
		t.Fatalf("IsTrackedFile outside a repo = true; want false")
	}
}

func TestLastCommitTime_ReturnsCommitterDate(t *testing.T) {
	dir := initRepo(t)
	cmd := exec.Command("git", "commit", "--allow-empty", "-q", "-m", "second")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE=2020-06-15T12:00:00Z",
		"GIT_COMMITTER_DATE=2020-06-15T12:00:00Z",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	// README.md's last touch is still the FIRST commit (initRepo), not the
	// empty second commit — LastCommitTime follows the file's own history,
	// not HEAD.
	got, ok := LastCommitTime(dir, "README.md")
	if !ok {
		t.Fatalf("LastCommitTime(README.md) ok=false; want true")
	}
	if got.IsZero() {
		t.Fatalf("LastCommitTime(README.md) returned zero time")
	}
}

func TestLastCommitTime_FalseForNeverCommittedPath(t *testing.T) {
	dir := initRepo(t)
	_, ok := LastCommitTime(dir, "never-existed.md")
	if ok {
		t.Fatalf("LastCommitTime(never-existed.md) ok=true; want false")
	}
}

func TestCurrentBranch_ReturnsBranchName(t *testing.T) {
	dir := initRepo(t)
	branch := CurrentBranch(dir)
	// `git init` default branch is "main" or "master" depending on git
	// version + init.defaultBranch config; both are valid answers.
	if branch != "main" && branch != "master" {
		t.Fatalf("CurrentBranch = %q; want main or master", branch)
	}
}

func TestGitDir_ResolvesAbsolutePath(t *testing.T) {
	dir := initRepo(t)
	gitDir, err := GitDir(dir)
	if err != nil {
		t.Fatalf("GitDir: %v", err)
	}
	if !filepath.IsAbs(gitDir) {
		t.Fatalf("GitDir = %q; want an absolute path", gitDir)
	}
	if _, err := os.Stat(filepath.Join(gitDir, "HEAD")); err != nil {
		t.Fatalf("GitDir %q doesn't look like a git dir (no HEAD): %v", gitDir, err)
	}
}

func TestGitDir_ErrorsOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := GitDir(dir); err == nil {
		t.Fatalf("GitDir(%q) = nil error; want an error (not a repo)", dir)
	}
}

func TestDiffNumstat_ParsesUnstagedTrackedRows(t *testing.T) {
	dir := initRepo(t)
	// README.md is tracked (committed by initRepo); modify WITHOUT staging.
	var body bytes.Buffer
	for i := 0; i < 7; i++ {
		body.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), body.Bytes(), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	rows := DiffNumstat(dir)
	if len(rows) != 1 || rows[0].Path != "README.md" {
		t.Fatalf("DiffNumstat = %+v; want one row for README.md", rows)
	}
	// DiffCachedNumstat must NOT see this unstaged change.
	if cached := DiffCachedNumstat(dir); len(cached) != 0 {
		t.Fatalf("DiffCachedNumstat = %+v; want empty (nothing staged)", cached)
	}
}

func TestUntrackedFiles_ListsUntrackedOnly(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}
	files := UntrackedFiles(dir)
	if len(files) != 1 || files[0] != "new.txt" {
		t.Fatalf("UntrackedFiles = %v; want [new.txt]", files)
	}
	// README.md is tracked+committed — must not show up as untracked.
	for _, f := range files {
		if f == "README.md" {
			t.Fatalf("UntrackedFiles included tracked file README.md: %v", files)
		}
	}
}

func TestUntrackedNumstat_TreatsFileAsAllAdded(t *testing.T) {
	dir := initRepo(t)
	var body bytes.Buffer
	for i := 0; i < 9; i++ {
		body.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "brand_new.go"), body.Bytes(), 0o644); err != nil {
		t.Fatalf("write brand_new.go: %v", err)
	}
	rows := UntrackedNumstat(dir)
	if len(rows) != 1 {
		t.Fatalf("UntrackedNumstat = %+v; want 1 row", rows)
	}
	if rows[0].Path != "brand_new.go" {
		t.Fatalf("row.Path = %q; want brand_new.go (the /dev/null side must be discarded)", rows[0].Path)
	}
	if rows[0].Added != "9" || rows[0].Removed != "0" {
		t.Fatalf("row counts = (%q, %q); want (9, 0)", rows[0].Added, rows[0].Removed)
	}
}

func TestUntrackedNumstat_BinaryFileMarkedWithDash(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0, 1, 2, 0, 3}, 0o644); err != nil {
		t.Fatalf("write bin.dat: %v", err)
	}
	rows := UntrackedNumstat(dir)
	if len(rows) != 1 {
		t.Fatalf("UntrackedNumstat = %+v; want 1 row", rows)
	}
	if rows[0].Added != "-" || rows[0].Removed != "-" {
		t.Fatalf("row counts = (%q, %q); want (\"-\", \"-\") for a binary file", rows[0].Added, rows[0].Removed)
	}
}

// TestUntrackedFiles_UnicodeName guards the -z / core.quotepath fix: a
// non-ASCII untracked filename must be returned RAW (not octal-escaped),
// so the downstream `git diff --no-index` in UntrackedNumstat can open it
// and count its lines.
func TestUntrackedFiles_UnicodeName(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "é.go"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write é.go: %v", err)
	}
	files := UntrackedFiles(dir)
	if len(files) != 1 || files[0] != "é.go" {
		t.Fatalf("UntrackedFiles = %v; want [é.go] (raw, not octal-escaped)", files)
	}
}

func TestUntrackedNumstat_UnicodeNameCounted(t *testing.T) {
	dir := initRepo(t)
	var body bytes.Buffer
	for i := 0; i < 12; i++ {
		body.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "é.go"), body.Bytes(), 0o644); err != nil {
		t.Fatalf("write é.go: %v", err)
	}
	rows := UntrackedNumstat(dir)
	if len(rows) != 1 {
		t.Fatalf("UntrackedNumstat = %+v; want 1 row for the unicode file", rows)
	}
	if rows[0].Path != "é.go" || rows[0].Added != "12" {
		t.Fatalf("row = %+v; want path=é.go added=12", rows[0])
	}
}

func TestTopLevel_ResolvesFromSubdir(t *testing.T) {
	dir := initRepo(t)
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	top, ok := TopLevel(sub)
	if !ok {
		t.Fatalf("TopLevel(%q) ok=false; want true", sub)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("evalsymlinks(%q): %v", dir, err)
	}
	got, err := filepath.EvalSymlinks(top)
	if err != nil {
		t.Fatalf("evalsymlinks(%q): %v", top, err)
	}
	if got != want {
		t.Fatalf("TopLevel(subdir) = %q; want repo root %q", got, want)
	}
}

func TestTopLevel_FalseOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	if top, ok := TopLevel(dir); ok || top != "" {
		t.Fatalf("TopLevel(%q) = (%q, %v); want (\"\", false)", dir, top, ok)
	}
}

// TestRestorePathsToHead_RevertsBranchEdit: RestorePathsToHead discards an
// unstaged edit and puts the working-tree copy back to HEAD's content — the
// primitive the L1 zero-conflict guard relies on.
func TestRestorePathsToHead_RevertsBranchEdit(t *testing.T) {
	repo := initRepo(t) // git init + committed README.md
	writeAndCommit(t, repo, "a.txt", "v1", "add a.txt")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("v2-edited"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := RestorePathsToHead(repo, "a.txt"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if string(got) != "v1" {
		t.Fatalf("want restored v1, got %q", string(got))
	}
}

// TestShowFile_ReadsRefContent: ShowFile reads a path's content at a ref
// without touching the working tree, and reports ok=false for a path that
// does not exist at that ref.
func TestShowFile_ReadsRefContent(t *testing.T) {
	repo := initRepo(t)
	writeAndCommit(t, repo, "a.txt", "v1", "add a.txt")
	got, ok := ShowFile(repo, "HEAD", "a.txt")
	if !ok || got != "v1" {
		t.Fatalf("ShowFile HEAD:a.txt = %q,%v want v1,true", got, ok)
	}
	if _, ok := ShowFile(repo, "HEAD", "missing.txt"); ok {
		t.Fatal("ShowFile of missing path should be false")
	}
}

// TestMergeBase_ReturnsCommonAncestor: MergeBase(repo, mainSha) resolves to a
// non-empty SHA when ref and HEAD share history — the primitive `warp` and the
// pulse main-compare probe both build on.
// TestMergeBase_FalseOnUnknownRef: an unresolvable ref is best-effort
// ("", false), not an error the caller must special-case.
// TestRestorePathsToRef_RestoresArbitraryRefContent: RestorePathsToRef
// discards a working-tree edit and restores an OLDER commit's content — not
// just HEAD's — the primitive the v2.0.0 4b-bis repair-path fix relies on
// (restoring derived docs to their merge-base with the default branch,
// rather than to HEAD, so an already-diverged branch self-repairs).
func TestRestorePathsToRef_RestoresArbitraryRefContent(t *testing.T) {
	repo := initRepo(t)
	writeAndCommit(t, repo, "a.txt", "v1", "add a.txt")
	v1Sha := strings.TrimSpace(revParse(t, repo, "HEAD"))
	writeAndCommit(t, repo, "a.txt", "v2", "update a.txt")

	if err := RestorePathsToRef(repo, v1Sha, "a.txt"); err != nil {
		t.Fatalf("RestorePathsToRef: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if string(got) != "v1" {
		t.Fatalf("want restored v1 (the older ref's content), got %q", string(got))
	}
}

// TestIsPathStaged_TrueForStagedChange: `git add`ing a modified tracked
// file makes IsPathStaged report true — this is the "deliberate" half of
// the staged-vs-unstaged distinction internal/cli's L1/L2b restores rely
// on (a `logmind warp` repair stages via `git checkout <ref> -- <path>`,
// the same index-writing primitive).
func TestIsPathStaged_TrueForStagedChange(t *testing.T) {
	repo := initRepo(t)
	writeAndCommit(t, repo, "a.txt", "v1", "add a.txt")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	runGit(t, repo, "add", "a.txt")
	if !IsPathStaged(repo, "a.txt") {
		t.Fatalf("IsPathStaged = false; want true for a staged change vs HEAD")
	}
}

// TestIsPathStaged_FalseForUnstagedChange: a working-tree edit that was
// never `git add`ed reports false — the "accidental" half of the
// distinction; L1/L2b restore paths in this state.
func TestIsPathStaged_FalseForUnstagedChange(t *testing.T) {
	repo := initRepo(t)
	writeAndCommit(t, repo, "a.txt", "v1", "add a.txt")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if IsPathStaged(repo, "a.txt") {
		t.Fatalf("IsPathStaged = true; want false for an unstaged working-tree edit")
	}
}

// TestIsPathStaged_FalseForCleanPath: a path with no change at all (index
// and working tree both match HEAD) reports false.
func TestIsPathStaged_FalseForCleanPath(t *testing.T) {
	repo := initRepo(t)
	writeAndCommit(t, repo, "a.txt", "v1", "add a.txt")
	if IsPathStaged(repo, "a.txt") {
		t.Fatalf("IsPathStaged = true; want false for a clean path")
	}
}

// TestIsPathStaged_FalseAfterStagedChangeCommitted: once a staged change is
// committed, the index and HEAD agree again — IsPathStaged must go back to
// false, not latch true forever.
func TestIsPathStaged_FalseAfterStagedChangeCommitted(t *testing.T) {
	repo := initRepo(t)
	writeAndCommit(t, repo, "a.txt", "v1", "add a.txt")
	writeAndCommit(t, repo, "a.txt", "v2", "update a.txt")
	if IsPathStaged(repo, "a.txt") {
		t.Fatalf("IsPathStaged = true; want false once the staged change is committed")
	}
}

// TestIsPathStaged_FalseOutsideRepo: best-effort — a non-repo directory
// must not panic or report a false "staged" positive.
func TestIsPathStaged_FalseOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	if IsPathStaged(dir, "a.txt") {
		t.Fatalf("IsPathStaged(%q) = true; want false outside a git repo", dir)
	}
}

// TestDefaultBranchMergeBase_ResolvesLocalDefaultBranch: no origin remote —
// DefaultBranchMergeBase falls back to the LOCAL default-branch ref and
// resolves the actual fork point, not just HEAD.
func TestDefaultBranchMergeBase_ResolvesLocalDefaultBranch(t *testing.T) {
	repo := initRepo(t) // git init --initial-branch left to git's own default
	forkSha := strings.TrimSpace(revParse(t, repo, "HEAD"))
	runGit(t, repo, "checkout", "-b", "feat/x")
	writeAndCommit(t, repo, "a.txt", "v1", "add a.txt on feat/x")

	got := DefaultBranchMergeBase(repo)
	if got != forkSha {
		t.Fatalf("DefaultBranchMergeBase = %q; want the fork commit %q", got, forkSha)
	}
}

// TestDefaultBranchMergeBase_FallsBackToHEAD_WhenUnresolvable: neither
// `origin/<default>` nor a local `<default>`-named branch resolves (no
// remote, and the two real local branches are named something else
// entirely) — DefaultBranchMergeBase degrades to the literal "HEAD" rather
// than erroring, so a caller that restores to it never breaks.
func TestDefaultBranchMergeBase_FallsBackToHEAD_WhenUnresolvable(t *testing.T) {
	repo := initRepo(t)
	runGit(t, repo, "branch", "-m", "onlybranch") // rename away from main/master
	runGit(t, repo, "checkout", "-b", "other")
	// Two local branches now (onlybranch, other), neither named main/master,
	// and no origin — DefaultBranch()'s step 1 (origin/HEAD) and step 2
	// (local main/master) both miss. Whatever it falls through to next
	// (init.defaultBranch or the hard "main" fallback) still won't match
	// either REAL branch name here, so neither MergeBase candidate resolves.
	def := DefaultBranch(repo)
	if def == "onlybranch" || def == "other" {
		t.Fatalf("test setup: DefaultBranch resolved to a real local branch (%q); need a name that resolves to neither", def)
	}

	if got := DefaultBranchMergeBase(repo); got != "HEAD" {
		t.Fatalf("DefaultBranchMergeBase = %q; want the \"HEAD\" fallback", got)
	}
}

// TestNumstat_RenameOutOfDocsIsSplit runs against a REAL git repository
// with a REAL rename, and is the pin that the earlier synthetic test was
// not.
//
// The regression it guards: #287 unified the numstat flag lists and
// dropped --no-renames. With rename detection on, git renders a
// cross-directory rename as ONE row whose path is `old => new`, so
// `docs/notes.md => src/payload.go` prefix-matched `docs/` and hundreds
// of lines of new code counted zero. An adversarial review found that the
// fix's own test — asserting on synthetic NumstatLine values — stayed
// GREEN when --no-renames was removed again, because it never invoked
// git. A test on the helper you fixed is not a test on the bug.
//
// This one drives the actual command. Remove --no-renames from
// numstatFlags and it goes red, because git's output shape changes.
//
// The file must be large enough that appending to it stays above git's
// ~50% rename-similarity threshold; a small file plus many added lines is
// simply not detected as a rename and the test would pass vacuously.
func TestNumstat_RenameOutOfDocsIsSplit(t *testing.T) {
	repo := initRepo(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	var big strings.Builder
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&big, "original line %d\n", i)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "notes.md"), []byte(big.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "add docs/notes.md")

	// Move it into src/ and append, keeping similarity high enough that
	// git still calls it a rename.
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	run("mv", "docs/notes.md", "src/payload.go")
	f, err := os.OpenFile(filepath.Join(repo, "src", "payload.go"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 150; i++ {
		fmt.Fprintf(f, "// added %d\n", i)
	}
	f.Close()
	run("add", "-A")

	// POSITIVE CONTROL FIRST. Assert that git, left to itself, DOES render
	// this as a rename. Without it the whole test is vacuous: an adversarial
	// review showed that under `diff.renames=false` in a user's gitconfig,
	// the "no => rows" assertion below passes even with --no-renames removed,
	// because git never emitted a rename in the first place. Absence of the
	// rendering only means something once we know it would otherwise appear.
	bare := exec.Command("git", "diff", "--cached", "--numstat")
	bare.Dir = repo
	bareOut, err := bare.Output()
	if err != nil {
		t.Fatalf("bare numstat: %v", err)
	}
	if !strings.Contains(string(bareOut), " => ") {
		t.Skipf("git did not render this as a rename (diff.renames disabled, or "+
			"similarity below threshold) — the test cannot prove anything about "+
			"--no-renames here. Bare numstat was:\n%s", bareOut)
	}

	rows := DiffCachedNumstat(repo)

	// Now the real assertion: our flags must suppress what the control just
	// proved git would otherwise emit.
	var renameRows int
	for _, r := range rows {
		if strings.Contains(r.Path, " => ") {
			renameRows++
		}
	}
	if renameRows > 0 {
		t.Fatalf("numstat still carries a rename rendering (%d rows) — --no-renames is not in effect:\n%+v", renameRows, rows)
	}

	// The substantive half must be attributed to src/, not swallowed by
	// the docs/ prefix.
	var sawSrc bool
	for _, r := range rows {
		if r.Path == "src/payload.go" {
			sawSrc = true
		}
	}
	if !sawSrc {
		t.Fatalf("no row for src/payload.go — the rename was not split:\n%+v", rows)
	}
}
