package skill

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedNow is the timestamp used by every test that exercises
// last-refined. Pinned to a known instant so the YAML rewrite is
// deterministic across CI shards.
var fixedNow = time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)

// reviewWith builds a NORMATIVE-template review body with the given
// SHA and citation list. We don't bother with all the optional
// severity buckets — sync only reads the SHA + citation block. Keeps
// fixtures small enough to read inline.
func reviewWith(sha string, citations []string) string {
	body := "# clud-bug review — PR #1\n"
	body += "<!-- protocol-version: 0.1.0 -->\n"
	body += "<!-- written-by: clud-bug[bot] -->\n"
	body += "<!-- review-sha: " + sha + " -->\n\n"
	body += "**Summary:** 1 critical · 0 minor · 0 preexisting\n\n"
	if len(citations) > 0 {
		body += "**Skills cited:**\n"
		for _, c := range citations {
			body += "- " + c + "\n"
		}
		body += "\n"
	}
	body += "**Findings:**\n\n"
	body += "### 🔴 Critical\n"
	body += "- **foo.go:10** — example-skill: Something went wrong\n"
	body += "\n---\n\n"
	body += "[Link to PR](https://github.com/example/example/pull/1)\n"
	return body
}

// writeReview puts the body at docs/reviews/<filename> under root.
func writeReview(t *testing.T, root, filename, body string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir reviews: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(body), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
}

// scaffoldSkillWithProvenance creates .claude/skills/<name>/{SKILL.md,
// PROVENANCE.md} so sync has something to update. Returns the
// PROVENANCE.md path for assertion.
func scaffoldSkillWithProvenance(t *testing.T, root, name string) string {
	t.Helper()
	if _, err := ScaffoldBasic(root, name, "desc"); err != nil {
		t.Fatalf("scaffold %s: %v", name, err)
	}
	skillMD := MDPath(root, name)
	if err := WriteProvenanceSkeleton(skillMD, name); err != nil {
		t.Fatalf("provenance %s: %v", name, err)
	}
	return filepath.Join(SkillDir(root, name), "PROVENANCE.md")
}

// TestSync_Empty: missing docs/reviews dir → zero-work summary, no
// error. This is the freshly-init'd-repo path.
func TestSync_Empty(t *testing.T) {
	dir := t.TempDir()
	got, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.ReviewsScanned != 0 || got.SkillsUpdated != 0 || got.CitationsAdded != 0 {
		t.Errorf("expected zero-work summary; got %+v", got)
	}
}

// TestSync_SingleCitation_HappyPath: one PR, one skill, counter goes
// from 0 → N and applied-review-shas records the head SHA.
func TestSync_SingleCitation_HappyPath(t *testing.T) {
	dir := t.TempDir()
	provPath := scaffoldSkillWithProvenance(t, dir, "example-skill")

	sha := strings.Repeat("a", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha, []string{
		"example-skill (3 findings)",
	}))

	got, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.SkillsUpdated != 1 {
		t.Errorf("SkillsUpdated = %d; want 1", got.SkillsUpdated)
	}
	if got.CitationsAdded != 3 {
		t.Errorf("CitationsAdded = %d; want 3", got.CitationsAdded)
	}
	if got.ReviewsApplied != 1 {
		t.Errorf("ReviewsApplied = %d; want 1", got.ReviewsApplied)
	}

	body, err := os.ReadFile(provPath)
	if err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "cited-by-clud-bug: 3") {
		t.Errorf("counter not updated; body:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, `last-refined: "2026-06-03"`) {
		t.Errorf("last-refined not updated; body:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, sha) {
		t.Errorf("applied-review-shas missing %s; body:\n%s", sha, bodyStr)
	}
}

// TestSync_MultipleCitationsSameSkill: two PRs cite the same skill;
// counter sums + both SHAs land in the applied list.
func TestSync_MultipleCitationsSameSkill(t *testing.T) {
	dir := t.TempDir()
	provPath := scaffoldSkillWithProvenance(t, dir, "shared-skill")

	sha1 := strings.Repeat("1", 40)
	sha2 := strings.Repeat("2", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha1, []string{
		"shared-skill (2 findings)",
	}))
	writeReview(t, dir, "PR-2.md", reviewWith(sha2, []string{
		"shared-skill (4 findings)",
	}))

	got, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.SkillsUpdated != 1 {
		t.Fatalf("SkillsUpdated = %d; want 1", got.SkillsUpdated)
	}
	if got.CitationsAdded != 6 {
		t.Errorf("CitationsAdded = %d; want 6", got.CitationsAdded)
	}
	if got.ReviewsApplied != 2 {
		t.Errorf("ReviewsApplied = %d; want 2", got.ReviewsApplied)
	}

	body, _ := os.ReadFile(provPath)
	if !strings.Contains(string(body), "cited-by-clud-bug: 6") {
		t.Errorf("counter not summed; body:\n%s", body)
	}
	if !strings.Contains(string(body), sha1) || !strings.Contains(string(body), sha2) {
		t.Errorf("both SHAs not present; body:\n%s", body)
	}
}

// TestSync_MultipleSkillsOnePR: one PR cites two different skills;
// both PROVENANCE.md files are updated and the run reports both.
func TestSync_MultipleSkillsOnePR(t *testing.T) {
	dir := t.TempDir()
	p1 := scaffoldSkillWithProvenance(t, dir, "alpha-skill")
	p2 := scaffoldSkillWithProvenance(t, dir, "beta-skill")

	sha := strings.Repeat("b", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha, []string{
		"alpha-skill (1 finding)",
		"beta-skill (5 findings)",
	}))

	got, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.SkillsUpdated != 2 {
		t.Errorf("SkillsUpdated = %d; want 2", got.SkillsUpdated)
	}
	if got.CitationsAdded != 6 {
		t.Errorf("CitationsAdded = %d; want 6", got.CitationsAdded)
	}

	b1, _ := os.ReadFile(p1)
	b2, _ := os.ReadFile(p2)
	if !strings.Contains(string(b1), "cited-by-clud-bug: 1") {
		t.Errorf("alpha not updated; body:\n%s", b1)
	}
	if !strings.Contains(string(b2), "cited-by-clud-bug: 5") {
		t.Errorf("beta not updated; body:\n%s", b2)
	}
}

// TestSync_Idempotent: a second Sync over the same input is a no-op.
// The counter MUST NOT double-count. This is the load-bearing
// contract — wired into post-merge hooks etc.
func TestSync_Idempotent(t *testing.T) {
	dir := t.TempDir()
	provPath := scaffoldSkillWithProvenance(t, dir, "idem-skill")

	sha := strings.Repeat("c", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha, []string{
		"idem-skill (2 findings)",
	}))

	first, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if first.SkillsUpdated != 1 || first.CitationsAdded != 2 {
		t.Fatalf("first pass off: %+v", first)
	}

	// Snapshot the body after the first pass so we can compare.
	firstBody, err := os.ReadFile(provPath)
	if err != nil {
		t.Fatalf("read first: %v", err)
	}

	second, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if second.SkillsUpdated != 0 || second.CitationsAdded != 0 {
		t.Errorf("second pass should be no-op; got %+v", second)
	}

	secondBody, _ := os.ReadFile(provPath)
	if string(firstBody) != string(secondBody) {
		t.Errorf("provenance body changed on second pass; diff:\n--- first ---\n%s\n--- second ---\n%s",
			firstBody, secondBody)
	}
}

// TestSync_IdempotentThenNewReview: after the no-op second pass, a
// third pass with a NEW PR's worth of citations does fold them in
// (proves the dedup is per-SHA, not per-skill).
func TestSync_IdempotentThenNewReview(t *testing.T) {
	dir := t.TempDir()
	provPath := scaffoldSkillWithProvenance(t, dir, "growing-skill")

	sha1 := strings.Repeat("d", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha1, []string{
		"growing-skill (1 finding)",
	}))
	if _, err := Sync(dir, SyncOptions{Now: fixedNow}); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	sha2 := strings.Repeat("e", 40)
	writeReview(t, dir, "PR-2.md", reviewWith(sha2, []string{
		"growing-skill (4 findings)",
	}))

	got, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("third Sync: %v", err)
	}
	if got.SkillsUpdated != 1 || got.CitationsAdded != 4 {
		t.Errorf("only the new review should land; got %+v", got)
	}

	body, _ := os.ReadFile(provPath)
	if !strings.Contains(string(body), "cited-by-clud-bug: 5") {
		t.Errorf("counter should be 1+4=5; body:\n%s", body)
	}
}

// TestSync_DryRun: parses + reports but doesn't touch disk.
func TestSync_DryRun(t *testing.T) {
	dir := t.TempDir()
	provPath := scaffoldSkillWithProvenance(t, dir, "dry-skill")
	before, err := os.ReadFile(provPath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	sha := strings.Repeat("f", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha, []string{
		"dry-skill (7 findings)",
	}))

	got, err := Sync(dir, SyncOptions{Now: fixedNow, DryRun: true})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.SkillsUpdated != 1 || got.CitationsAdded != 7 {
		t.Errorf("dry-run summary should reflect what WOULD happen; got %+v", got)
	}

	after, err := os.ReadFile(provPath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("dry-run touched disk; before:\n%s\nafter:\n%s", before, after)
	}
}

// TestSync_MalformedReview_Skipped: a review file without a review-sha
// comment is skipped with a warning; siblings still process.
func TestSync_MalformedReview_Skipped(t *testing.T) {
	dir := t.TempDir()
	provPath := scaffoldSkillWithProvenance(t, dir, "ok-skill")

	// Good review.
	sha := strings.Repeat("a", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha, []string{
		"ok-skill (1 finding)",
	}))
	// Bad review — no review-sha comment, malformed header.
	writeReview(t, dir, "PR-2.md",
		"# garbage\n\n**Skills cited:**\n- ok-skill (99 findings)\n\n")

	var warnings []string
	got, err := Sync(dir, SyncOptions{
		Now:  fixedNow,
		Warn: func(s string) { warnings = append(warnings, s) },
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.ReviewsScanned != 2 {
		t.Errorf("ReviewsScanned = %d; want 2", got.ReviewsScanned)
	}
	if got.CitationsAdded != 1 {
		t.Errorf("CitationsAdded = %d; want 1 (only the good review)", got.CitationsAdded)
	}
	if len(warnings) == 0 {
		t.Errorf("expected at least one warning for the malformed file")
	}
	// Confirm the bad review's bogus 99-finding count didn't poison
	// the on-disk counter.
	body, _ := os.ReadFile(provPath)
	if !strings.Contains(string(body), "cited-by-clud-bug: 1") {
		t.Errorf("counter should be 1, not 100; body:\n%s", body)
	}
}

// TestSync_UnknownSkill_Warned: a review cites a skill that doesn't
// exist on disk. Sync warns + continues; no PROVENANCE.md is created
// (skill-scaffold is the user's job, not sync's).
func TestSync_UnknownSkill_Warned(t *testing.T) {
	dir := t.TempDir()
	sha := strings.Repeat("a", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha, []string{
		"never-installed (3 findings)",
	}))

	var warnings []string
	got, err := Sync(dir, SyncOptions{
		Now:  fixedNow,
		Warn: func(s string) { warnings = append(warnings, s) },
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.SkillsUpdated != 0 {
		t.Errorf("SkillsUpdated = %d; want 0", got.SkillsUpdated)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for unknown skill")
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "never-installed") {
		t.Errorf("warning should name the skill; got:\n%s", strings.Join(warnings, "\n"))
	}
}

// TestSync_NonPRFile_Ignored: stray markdown in docs/reviews/ doesn't
// break the run.
func TestSync_NonPRFile_Ignored(t *testing.T) {
	dir := t.TempDir()
	scaffoldSkillWithProvenance(t, dir, "kept")

	sha := strings.Repeat("a", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha, []string{
		"kept (1 finding)",
	}))
	writeReview(t, dir, "README.md",
		"# Reviews\n\nThis directory is managed by clud-bug.\n")

	got, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.ReviewsScanned != 1 {
		t.Errorf("ReviewsScanned = %d; want 1 (README.md must be skipped)", got.ReviewsScanned)
	}
	if got.SkillsUpdated != 1 {
		t.Errorf("SkillsUpdated = %d; want 1", got.SkillsUpdated)
	}
}

// TestParseReview_Tolerant: a clean PR with no citations is a valid
// parse — sync just skips it.
func TestParseReview_NoCitations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PR-7.md")
	sha := strings.Repeat("0", 40)
	body := reviewWith(sha, nil)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rv, err := ParseReview(path)
	if err != nil {
		t.Fatalf("ParseReview: %v", err)
	}
	if rv.PRNumber != 7 {
		t.Errorf("PRNumber = %d; want 7", rv.PRNumber)
	}
	if rv.HeadSHA != sha {
		t.Errorf("HeadSHA = %q; want %q", rv.HeadSHA, sha)
	}
	if len(rv.Citations) != 0 {
		t.Errorf("Citations = %v; want empty", rv.Citations)
	}
}

// TestParseReview_BadFilename: a file under docs/reviews/ that isn't
// `PR-<n>.md` errors out with ErrMalformedReview so callers can decide
// what to do.
func TestParseReview_BadFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-pr.md")
	if err := os.WriteFile(path, []byte("body\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ParseReview(path)
	if !errors.Is(err, ErrMalformedReview) {
		t.Errorf("expected ErrMalformedReview; got %v", err)
	}
}

// TestParseReview_MissingSHA: a review file without a review-sha
// comment errors out with ErrMalformedReview.
func TestParseReview_MissingSHA(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PR-3.md")
	body := "# review\n\n**Skills cited:**\n- foo (1 finding)\n\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ParseReview(path)
	if !errors.Is(err, ErrMalformedReview) {
		t.Errorf("expected ErrMalformedReview; got %v", err)
	}
}

// TestParseReview_CitationVariations covers single/plural "finding"
// and whitespace tolerance in the citation lines.
func TestParseReview_CitationVariations(t *testing.T) {
	cases := []struct {
		Name      string
		Line      string
		WantSkill string
		WantCount int
	}{
		{"plural", "- foo (3 findings)", "foo", 3},
		{"single", "- bar (1 finding)", "bar", 1},
		{"extra-spaces", "-    baz   (2   findings)", "baz", 2},
		{"kebab", "- my-kebab-skill (5 findings)", "my-kebab-skill", 5},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			body := "<!-- review-sha: " + strings.Repeat("a", 40) + " -->\n\n"
			body += "**Skills cited:**\n" + c.Line + "\n\n"
			dir := t.TempDir()
			path := filepath.Join(dir, "PR-1.md")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			rv, err := ParseReview(path)
			if err != nil {
				t.Fatalf("ParseReview: %v", err)
			}
			if rv.Citations[c.WantSkill] != c.WantCount {
				t.Errorf("Citations[%q] = %d; want %d (got %v)",
					c.WantSkill, rv.Citations[c.WantSkill], c.WantCount, rv.Citations)
			}
		})
	}
}

// TestParseReview_ZeroFindingsRejected: a `(0 findings)` line is
// dropped — zero is not signal worth recording.
func TestParseReview_ZeroFindingsRejected(t *testing.T) {
	dir := t.TempDir()
	body := "<!-- review-sha: " + strings.Repeat("a", 40) + " -->\n\n"
	body += "**Skills cited:**\n- whatever (0 findings)\n\n"
	path := filepath.Join(dir, "PR-9.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rv, err := ParseReview(path)
	if err != nil {
		t.Fatalf("ParseReview: %v", err)
	}
	if len(rv.Citations) != 0 {
		t.Errorf("Citations should drop zero-count entries; got %v", rv.Citations)
	}
}

// TestParseProvenance_ReadsAppliedSHAs: covers parseProvenance's
// applied-review-shas extraction (used by re-runs to dedup).
func TestParseProvenance_ReadsAppliedSHAs(t *testing.T) {
	body := "# Provenance for skill: x\n\n```yaml\n" +
		"derived-from-decisions: []\n" +
		"cited-by-clud-bug: 7\n" +
		`last-refined: "2026-01-01"` + "\n" +
		"refinement-history: []\n" +
		`applied-review-shas: ["abc", "def"]` + "\n" +
		"```\n"
	st := parseProvenance(body)
	if st.CitedByCludBug != 7 {
		t.Errorf("CitedByCludBug = %d; want 7", st.CitedByCludBug)
	}
	if st.LastRefined != "2026-01-01" {
		t.Errorf("LastRefined = %q; want 2026-01-01", st.LastRefined)
	}
	if len(st.AppliedSHAs) != 2 || st.AppliedSHAs[0] != "abc" || st.AppliedSHAs[1] != "def" {
		t.Errorf("AppliedSHAs = %v; want [abc def]", st.AppliedSHAs)
	}
}

// TestRewriteProvenance_InjectsAppliedSHAs: a pristine skeleton (no
// applied-review-shas line) gets the line injected on first write.
func TestRewriteProvenance_InjectsAppliedSHAs(t *testing.T) {
	body := "# Provenance for skill: x\n\n```yaml\n" +
		"derived-from-decisions: []\n" +
		"cited-by-clud-bug: 0\n" +
		`last-refined: ""` + "\n" +
		"refinement-history: []\n" +
		"```\n\nfree-form notes here\n"
	upd := provenanceUpdate{
		NewCount:    3,
		LastRefined: "2026-06-03",
		AppliedSHAs: []string{"aaaa", "bbbb"},
	}
	got := rewriteProvenance(body, upd)
	if !strings.Contains(got, "cited-by-clud-bug: 3") {
		t.Errorf("counter not rewritten; got:\n%s", got)
	}
	if !strings.Contains(got, `last-refined: "2026-06-03"`) {
		t.Errorf("last-refined not rewritten; got:\n%s", got)
	}
	if !strings.Contains(got, `applied-review-shas: ["aaaa", "bbbb"]`) {
		t.Errorf("applied-review-shas not injected; got:\n%s", got)
	}
	if !strings.Contains(got, "free-form notes here") {
		t.Errorf("free-form notes below the YAML block were stripped; got:\n%s", got)
	}
}

// TestFilterNewSHAs_DedupesAndSorts: the dedup gate behaves on
// duplicates within incoming and on case differences.
func TestFilterNewSHAs(t *testing.T) {
	applied := []string{"AAAA", "bbbb"}
	incoming := []string{"aaaa", "BBBB", "cccc", "cccc"}
	got := filterNewSHAs(incoming, applied)
	want := []string{"cccc"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("filterNewSHAs = %v; want %v", got, want)
	}
}

// TestSync_WriteFailure_DoesNotCountSHA covers Bug 1 from PR #135 review:
// if atomicWriteFile fails for a given skill, the run MUST NOT count the
// SHA in ReviewsApplied (and MUST NOT bump SkillsUpdated / CitationsAdded)
// for that skill. Otherwise the summary inflates relative to what
// actually landed on disk and a downstream consumer reading the counter
// would believe a write happened when it didn't.
//
// Two skills are scaffolded; the writer is rigged to fail for the first
// skill's PROVENANCE.md and succeed for the second. We assert:
//   - the failed-write skill's old body is untouched
//   - the successful-write skill's body is bumped
//   - the summary counts only the second skill + only its SHA
func TestSync_WriteFailure_DoesNotCountSHA(t *testing.T) {
	dir := t.TempDir()
	failPath := scaffoldSkillWithProvenance(t, dir, "fail-skill")
	okPath := scaffoldSkillWithProvenance(t, dir, "ok-skill")

	// Snapshot the failing skill's pre-Sync body so we can prove the
	// disk wasn't touched even after the write was rejected.
	failBefore, err := os.ReadFile(failPath)
	if err != nil {
		t.Fatalf("read failPath: %v", err)
	}

	// Both skills are cited by the same PR — same SHA flows through
	// the loop twice. If Bug 1 regresses, the SHA gets added to
	// appliedReviewSet on the failed iteration too, inflating
	// ReviewsApplied. With the fix, the SHA only counts on the
	// ok-skill iteration (where the write actually persists).
	sha := strings.Repeat("a", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha, []string{
		"fail-skill (2 findings)",
		"ok-skill (3 findings)",
	}))

	// Swap atomicWriteFile for one that selectively fails for the
	// fail-skill path and delegates to the real writer otherwise.
	// t.Cleanup restores the original so other tests aren't affected.
	original := atomicWriteFile
	t.Cleanup(func() { atomicWriteFile = original })
	atomicWriteFile = func(path string, data []byte, perm os.FileMode) error {
		if strings.Contains(path, "fail-skill") {
			return errors.New("simulated write failure")
		}
		return original(path, data, perm)
	}

	var warnings []string
	got, err := Sync(dir, SyncOptions{
		Now:  fixedNow,
		Warn: func(s string) { warnings = append(warnings, s) },
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Only ok-skill should be counted as updated.
	if got.SkillsUpdated != 1 {
		t.Errorf("SkillsUpdated = %d; want 1 (only ok-skill persisted)", got.SkillsUpdated)
	}
	// Only ok-skill's 3 citations should be counted; fail-skill's 2 must NOT.
	if got.CitationsAdded != 3 {
		t.Errorf("CitationsAdded = %d; want 3 (fail-skill's 2 must not count)", got.CitationsAdded)
	}
	// THE LOAD-BEARING ASSERTION for Bug 1: the SHA must be counted
	// exactly once (from ok-skill's successful persist), not twice
	// (from both iterations regardless of write outcome).
	if got.ReviewsApplied != 1 {
		t.Errorf("ReviewsApplied = %d; want 1 (SHA must not count on failed write)", got.ReviewsApplied)
	}
	if len(got.Updates) != 1 || got.Updates[0].Name != "ok-skill" {
		t.Errorf("Updates should contain only ok-skill; got %+v", got.Updates)
	}

	// Disk-level cross-check: fail-skill's body is byte-identical to
	// the pre-Sync snapshot. (If the writer fix regresses by writing
	// before erroring out, this assertion catches it.)
	failAfter, err := os.ReadFile(failPath)
	if err != nil {
		t.Fatalf("read failPath after: %v", err)
	}
	if string(failBefore) != string(failAfter) {
		t.Errorf("fail-skill body changed despite write failure;\nbefore:\n%s\nafter:\n%s",
			failBefore, failAfter)
	}

	// ok-skill should have its counter bumped.
	okBody, _ := os.ReadFile(okPath)
	if !strings.Contains(string(okBody), "cited-by-clud-bug: 3") {
		t.Errorf("ok-skill counter not bumped; body:\n%s", okBody)
	}

	// A warning should have been emitted for the failed write so the
	// CLI surface still tells the user something went wrong.
	if len(warnings) == 0 {
		t.Errorf("expected warn for failed write; got none")
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "fail-skill") {
		t.Errorf("warning should name fail-skill; got:\n%s", strings.Join(warnings, "\n"))
	}
}

// TestSync_SHAcase_Canonicalised: hex casing in the review file
// doesn't leak into the on-disk SHA set — both `AAAA...` and `aaaa...`
// dedup against each other.
func TestSync_SHAcase_Canonicalised(t *testing.T) {
	dir := t.TempDir()
	provPath := scaffoldSkillWithProvenance(t, dir, "case-skill")

	upper := strings.Repeat("A", 40)
	lower := strings.Repeat("a", 40)
	// First pass with uppercase SHA.
	writeReview(t, dir, "PR-1.md", reviewWith(upper, []string{
		"case-skill (2 findings)",
	}))
	if _, err := Sync(dir, SyncOptions{Now: fixedNow}); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Rewrite the review with the lowercased SHA — same review, just
	// different casing. Should NOT bump the counter again.
	writeReview(t, dir, "PR-1.md", reviewWith(lower, []string{
		"case-skill (2 findings)",
	}))
	got, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if got.SkillsUpdated != 0 {
		t.Errorf("re-run with same SHA in different case must be no-op; got %+v", got)
	}
	body, _ := os.ReadFile(provPath)
	if !strings.Contains(string(body), "cited-by-clud-bug: 2") {
		t.Errorf("counter should still be 2; body:\n%s", body)
	}
}

// --- ParseSinceDuration --------------------------------------------------

// TestParseSinceDuration_Valid pins the accepted shapes: SPEC §3.9's own
// `<N>d` shorthand, plus a fallback to time.ParseDuration for ordinary
// Go duration strings.
func TestParseSinceDuration_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"1d", 24 * time.Hour},
		{"365d", 365 * 24 * time.Hour},
		{"24h", 24 * time.Hour},
		{"90m", 90 * time.Minute},
		{"1h30m", 90 * time.Minute},
	}
	for _, c := range cases {
		got, err := ParseSinceDuration(c.in)
		if err != nil {
			t.Errorf("ParseSinceDuration(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSinceDuration(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

// TestParseSinceDuration_Malformed pins the rejection path. A malformed
// --since MUST error rather than silently resolve to zero (which would
// mean "scan everything" while claiming to have honored the window —
// the exact lying-flag failure mode this build must avoid).
func TestParseSinceDuration_Malformed(t *testing.T) {
	cases := []string{
		"", "   ", "0d", "-5d", "7dd", "7 d", "7days", "garbage",
		"d7", "-24h", "0h", "7w", "1y", "d",
	}
	for _, in := range cases {
		if _, err := ParseSinceDuration(in); err == nil {
			t.Errorf("ParseSinceDuration(%q): expected error, got nil", in)
		}
	}
}

// --- Sync: --since --------------------------------------------------------

// TestSync_Since_ExcludesOldReviews: a review file backdated outside the
// --since window is excluded from the scan entirely — it doesn't count
// toward ReviewsScanned and its citations never reach PROVENANCE.md.
func TestSync_Since_ExcludesOldReviews(t *testing.T) {
	dir := t.TempDir()
	provPath := scaffoldSkillWithProvenance(t, dir, "since-skill")

	oldSHA := strings.Repeat("a", 40)
	newSHA := strings.Repeat("b", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(oldSHA, []string{"since-skill (2 findings)"}))
	writeReview(t, dir, "PR-2.md", reviewWith(newSHA, []string{"since-skill (5 findings)"}))

	oldTime := fixedNow.Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "docs", "reviews", "PR-1.md"), oldTime, oldTime); err != nil {
		t.Fatalf("chtimes PR-1: %v", err)
	}
	if err := os.Chtimes(filepath.Join(dir, "docs", "reviews", "PR-2.md"), fixedNow, fixedNow); err != nil {
		t.Fatalf("chtimes PR-2: %v", err)
	}

	got, err := Sync(dir, SyncOptions{Now: fixedNow, Since: 7 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.ReviewsScanned != 1 {
		t.Errorf("ReviewsScanned = %d; want 1 (old review excluded by --since)", got.ReviewsScanned)
	}
	if got.CitationsAdded != 5 {
		t.Errorf("CitationsAdded = %d; want 5 (only PR-2's citations)", got.CitationsAdded)
	}

	body, _ := os.ReadFile(provPath)
	if strings.Contains(string(body), oldSHA) {
		t.Errorf("old SHA must not be recorded when excluded by --since; body:\n%s", body)
	}
	if !strings.Contains(string(body), newSHA) {
		t.Errorf("new SHA missing; body:\n%s", body)
	}
}

// TestSync_Since_Zero_IsUnbounded: the zero value of Since (what every
// pre-existing caller passes) must behave exactly like the pre-flag
// code path — a backdated review is still picked up.
func TestSync_Since_Zero_IsUnbounded(t *testing.T) {
	dir := t.TempDir()
	scaffoldSkillWithProvenance(t, dir, "unbounded-skill")
	sha := strings.Repeat("c", 40)
	writeReview(t, dir, "PR-1.md", reviewWith(sha, []string{"unbounded-skill (1 findings)"}))
	oldTime := fixedNow.Add(-3650 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "docs", "reviews", "PR-1.md"), oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got, err := Sync(dir, SyncOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.ReviewsScanned != 1 || got.CitationsAdded != 1 {
		t.Errorf("Since=0 must not filter anything; got %+v", got)
	}
}

// --- UpdateProvenance -------------------------------------------------

// writeCludBugManifest drops a minimal SPEC §1.12.1-shaped
// .claude/skills/.clud-bug.json under root.
func writeCludBugManifest(t *testing.T, root string, usage map[string]cludBugUsageEntry) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .claude/skills: %v", err)
	}
	m := cludBugManifest{Usage: usage}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".clud-bug.json"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// scaffoldPlainSkill creates just .claude/skills/<name>/SKILL.md — no
// PROVENANCE.md at all — the "brand new skill" starting state for
// UpdateProvenance tests.
func scaffoldPlainSkill(t *testing.T, root, name string) {
	t.Helper()
	if _, err := ScaffoldBasic(root, name, "desc"); err != nil {
		t.Fatalf("scaffold %s: %v", name, err)
	}
}

// TestUpdateProvenance_FreshSkill_NoPriorFile: a skill with no
// PROVENANCE.md at all gets the SPEC §1.11.1 template written from
// scratch, reported as Migrated.
func TestUpdateProvenance_FreshSkill_NoPriorFile(t *testing.T) {
	dir := t.TempDir()
	scaffoldPlainSkill(t, dir, "fresh-skill")

	got, err := UpdateProvenance(dir, ProvenanceOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("UpdateProvenance: %v", err)
	}
	if got.SkillsScanned != 1 || got.SkillsRefreshed != 1 {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if !got.Updates[0].Migrated {
		t.Errorf("expected Migrated=true for a brand-new file; got %+v", got.Updates[0])
	}

	body, err := os.ReadFile(filepath.Join(SkillDir(dir, "fresh-skill"), "PROVENANCE.md"))
	if err != nil {
		t.Fatalf("read PROVENANCE.md: %v", err)
	}
	got0 := string(body)
	for _, want := range []string{
		"# Provenance for fresh-skill\n",
		"<!-- maintained-by: logmind sync -->\n",
		"**Source:** manual\n",
		"**Last refined:** 2026-06-03\n",
		"**Cited by clud-bug:** 0 times\n",
		"**Derived from decisions:**\n",
		"**Refinement history:**\n",
	} {
		if !strings.Contains(got0, want) {
			t.Errorf("PROVENANCE.md missing %q; body:\n%s", want, got0)
		}
	}
}

// TestUpdateProvenance_MigratesLegacySkeleton: a skill that already has
// the pre-existing non-SPEC `<!-- logmind:provenance v1 -->` skeleton
// (provenance.go) gets replaced with the SPEC §1.11.1 format, not
// merged with it — the two formats share no fields worth carrying over
// (the legacy skeleton's cited-by-clud-bug counter is sourced from
// docs/reviews/PR-*.md, a different source than this function reads).
func TestUpdateProvenance_MigratesLegacySkeleton(t *testing.T) {
	dir := t.TempDir()
	scaffoldSkillWithProvenance(t, dir, "legacy-skill")

	got, err := UpdateProvenance(dir, ProvenanceOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("UpdateProvenance: %v", err)
	}
	if got.SkillsRefreshed != 1 || !got.Updates[0].Migrated {
		t.Fatalf("expected one migrated refresh; got %+v", got)
	}
	body, _ := os.ReadFile(filepath.Join(SkillDir(dir, "legacy-skill"), "PROVENANCE.md"))
	if strings.Contains(string(body), "logmind:provenance v1") {
		t.Errorf("legacy marker should be gone after migration; body:\n%s", body)
	}
	if !strings.Contains(string(body), "<!-- maintained-by: logmind sync -->") {
		t.Errorf("missing SPEC marker after migration; body:\n%s", body)
	}
}

// TestUpdateProvenance_ReadsCludBugUsage: the citation count comes from
// .claude/skills/.clud-bug.json's usage[<slug>].citations field, per
// SPEC §1.11.1 / §1.12.
func TestUpdateProvenance_ReadsCludBugUsage(t *testing.T) {
	dir := t.TempDir()
	scaffoldPlainSkill(t, dir, "cited-skill")
	writeCludBugManifest(t, dir, map[string]cludBugUsageEntry{
		"cited-skill": {Citations: 7, LastCited: "2026-06-01T00:00:00Z"},
	})

	got, err := UpdateProvenance(dir, ProvenanceOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("UpdateProvenance: %v", err)
	}
	if got.Updates[0].NewCitations != 7 {
		t.Fatalf("NewCitations = %d; want 7", got.Updates[0].NewCitations)
	}
	body, _ := os.ReadFile(filepath.Join(SkillDir(dir, "cited-skill"), "PROVENANCE.md"))
	if !strings.Contains(string(body), "**Cited by clud-bug:** 7 times") {
		t.Errorf("citation count missing from body:\n%s", body)
	}
}

// TestUpdateProvenance_DerivedFromDecisions: a decision entry whose
// title mentions the skill by name (hyphenated slug form) surfaces as a
// SPEC §1.11.1 "Derived from decisions" anchor line.
func TestUpdateProvenance_DerivedFromDecisions(t *testing.T) {
	dir := t.TempDir()
	scaffoldPlainSkill(t, dir, "avoid-naked-fetch")
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	body := "# Decisions\n\n" +
		"## 2026-05-15 10:00 - Add avoid-naked-fetch skill\n\n" +
		"**Reasoning:** codify the timeout convention.\n\n" +
		"---\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write decisions.md: %v", err)
	}

	got, err := UpdateProvenance(dir, ProvenanceOptions{Now: fixedNow})
	if err != nil {
		t.Fatalf("UpdateProvenance: %v", err)
	}
	if got.Updates[0].DecisionRefs != 1 {
		t.Fatalf("DecisionRefs = %d; want 1; got %+v", got.Updates[0].DecisionRefs, got.Updates[0])
	}
	provBody, _ := os.ReadFile(filepath.Join(SkillDir(dir, "avoid-naked-fetch"), "PROVENANCE.md"))
	if !strings.Contains(string(provBody), "docs/decisions.md#add-avoid-naked-fetch-skill") {
		t.Errorf("expected decision anchor line; body:\n%s", provBody)
	}
}

// TestUpdateProvenance_Idempotent: re-running with no new signal is a
// no-op — SkillsRefreshed is 0 on the second pass and the file is
// byte-identical.
func TestUpdateProvenance_Idempotent(t *testing.T) {
	dir := t.TempDir()
	scaffoldPlainSkill(t, dir, "steady-skill")
	writeCludBugManifest(t, dir, map[string]cludBugUsageEntry{
		"steady-skill": {Citations: 2, LastCited: "2026-06-01T00:00:00Z"},
	})

	if _, err := UpdateProvenance(dir, ProvenanceOptions{Now: fixedNow}); err != nil {
		t.Fatalf("first UpdateProvenance: %v", err)
	}
	provPath := filepath.Join(SkillDir(dir, "steady-skill"), "PROVENANCE.md")
	before, err := os.ReadFile(provPath)
	if err != nil {
		t.Fatalf("read after first run: %v", err)
	}

	later := fixedNow.Add(24 * time.Hour)
	got, err := UpdateProvenance(dir, ProvenanceOptions{Now: later})
	if err != nil {
		t.Fatalf("second UpdateProvenance: %v", err)
	}
	if got.SkillsRefreshed != 0 {
		t.Errorf("re-run with no new signal must be a no-op; got %+v", got)
	}
	after, err := os.ReadFile(provPath)
	if err != nil {
		t.Fatalf("read after second run: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("unchanged re-run must leave the file byte-identical.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestUpdateProvenance_DryRun_WritesNothing: --dry-run reports what
// would change but never touches disk.
func TestUpdateProvenance_DryRun_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	scaffoldPlainSkill(t, dir, "dry-prov-skill")
	provPath := filepath.Join(SkillDir(dir, "dry-prov-skill"), "PROVENANCE.md")

	got, err := UpdateProvenance(dir, ProvenanceOptions{Now: fixedNow, DryRun: true})
	if err != nil {
		t.Fatalf("UpdateProvenance: %v", err)
	}
	if got.SkillsRefreshed != 1 {
		t.Errorf("dry-run summary should still report the would-be refresh; got %+v", got)
	}
	if _, err := os.Stat(provPath); !os.IsNotExist(err) {
		t.Errorf("dry-run must not create PROVENANCE.md; stat err = %v", err)
	}
}

// TestUpdateProvenance_Since_ExcludesStaleUsageAndOldDecisions: --since
// filters BOTH the usage-counter recency check (usage[<slug>].last_cited)
// and the decision-entry date filter. A skill whose only citation
// evidence is outside the window shows 0 citations for this run, and a
// decision entry outside the window doesn't appear in "Derived from
// decisions".
func TestUpdateProvenance_Since_ExcludesStaleUsageAndOldDecisions(t *testing.T) {
	dir := t.TempDir()
	scaffoldPlainSkill(t, dir, "stale-skill")
	writeCludBugManifest(t, dir, map[string]cludBugUsageEntry{
		"stale-skill": {Citations: 9, LastCited: "2025-01-01T00:00:00Z"}, // ancient
	})
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	body := "# Decisions\n\n" +
		"## 2025-01-02 10:00 - Old note about stale-skill\n\n" +
		"**Reasoning:** ancient.\n\n" +
		"---\n"
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write decisions.md: %v", err)
	}

	got, err := UpdateProvenance(dir, ProvenanceOptions{Now: fixedNow, Since: 30 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("UpdateProvenance: %v", err)
	}
	if got.SkillsRefreshed != 1 {
		// Even "nothing recent" is a meaningful first-write (Migrated),
		// so a refresh still happens — it should just show zeros.
		t.Fatalf("expected one refresh (fresh file, zeroed by --since); got %+v", got)
	}
	if got.Updates[0].NewCitations != 0 {
		t.Errorf("NewCitations = %d; want 0 (stale last_cited excluded by --since)", got.Updates[0].NewCitations)
	}
	if got.Updates[0].DecisionRefs != 0 {
		t.Errorf("DecisionRefs = %d; want 0 (old decision excluded by --since)", got.Updates[0].DecisionRefs)
	}
}

// --- WriteSkillDrafts ---------------------------------------------------

// decisionsBodyForDrafts builds three dated entries citing the same
// kebab-case token, matching the fixture shape
// TestSuggestFromDecisions_FindsPattern already pins in suggest_test.go
// — WriteSkillDrafts is a thin wrapper around SuggestFromDecisions, so
// reusing that fixture shape keeps the two tests honest about testing
// the same underlying heuristic.
func decisionsBodyForDrafts() string {
	return "# Decisions\n\n" +
		"## 2026-05-15 - first\n\n**Date**: 2026-05-15\n\n" +
		"We standardized on cache-invalidation everywhere.\n\n" +
		"## 2026-05-20 - second\n\n**Date**: 2026-05-20\n\n" +
		"More cache-invalidation work landed.\n\n" +
		"## 2026-05-25 - third\n\n**Date**: 2026-05-25\n\n" +
		"cache-invalidation again, third time.\n"
}

// TestWriteSkillDrafts_WritesConformantFrontmatter: the SPEC §1.9
// frontmatter contract (`source: logmind-derived`, `status: candidate`)
// is honored, and the file lands at the SPEC path
// docs/skills-derived/<name>.md.
func TestWriteSkillDrafts_WritesConformantFrontmatter(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(decisionsBodyForDrafts()), 0o644); err != nil {
		t.Fatalf("write decisions.md: %v", err)
	}

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	summary, err := WriteSkillDrafts(dir, DraftOptions{Now: now})
	if err != nil {
		t.Fatalf("WriteSkillDrafts: %v", err)
	}
	if summary.CandidatesConsidered != 1 {
		t.Fatalf("CandidatesConsidered = %d; want 1; summary=%+v", summary.CandidatesConsidered, summary)
	}
	if len(summary.DraftsWritten) != 1 || summary.DraftsWritten[0] != "cache-invalidation" {
		t.Fatalf("DraftsWritten = %v; want [cache-invalidation]", summary.DraftsWritten)
	}

	path := filepath.Join(dir, "docs", "skills-derived", "cache-invalidation.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	got := string(body)
	if !strings.HasPrefix(got, "---\nname: cache-invalidation\n") {
		t.Errorf("frontmatter doesn't open as expected; body starts:\n%s", got[:80])
	}
	if !strings.Contains(got, "\nsource: logmind-derived\n") {
		t.Errorf("missing source: logmind-derived; body:\n%s", got)
	}
	if !strings.Contains(got, "\nstatus: candidate\n") {
		t.Errorf("missing status: candidate; body:\n%s", got)
	}
}

// TestWriteSkillDrafts_DryRun_WritesNothing: --dry-run must not create
// docs/skills-derived/ at all, even though the summary still reports
// what would have been written.
func TestWriteSkillDrafts_DryRun_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "decisions.md"), []byte(decisionsBodyForDrafts()), 0o644); err != nil {
		t.Fatalf("write decisions.md: %v", err)
	}

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	summary, err := WriteSkillDrafts(dir, DraftOptions{Now: now, DryRun: true})
	if err != nil {
		t.Fatalf("WriteSkillDrafts: %v", err)
	}
	if len(summary.DraftsWritten) != 1 {
		t.Fatalf("DraftsWritten = %v; want 1 entry reported even in dry-run", summary.DraftsWritten)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "skills-derived")); !os.IsNotExist(err) {
		t.Errorf("dry-run must not create docs/skills-derived/; stat err = %v", err)
	}
}

// TestWriteSkillDrafts_NoCandidates: an empty/absent decision log is a
// clean zero-work summary, not an error.
func TestWriteSkillDrafts_NoCandidates(t *testing.T) {
	dir := t.TempDir()
	summary, err := WriteSkillDrafts(dir, DraftOptions{Now: time.Now()})
	if err != nil {
		t.Fatalf("WriteSkillDrafts: %v", err)
	}
	if summary.CandidatesConsidered != 0 || len(summary.DraftsWritten) != 0 {
		t.Errorf("expected zero-work summary; got %+v", summary)
	}
}
