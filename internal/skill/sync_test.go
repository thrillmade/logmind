package skill

import (
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
