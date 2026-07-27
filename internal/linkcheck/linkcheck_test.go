package linkcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheck_CleanRepo(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md": "# Project\n",
	})
	broken, orphans, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(broken) != 0 {
		t.Fatalf("broken = %v; want []", broken)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans = %v; want []", orphans)
	}
	report := FormatReport(broken, orphans)
	if report != "All markdown links resolve and no orphans found." {
		t.Fatalf("clean report drift:\n%s", report)
	}
}

func TestCheck_BrokenLink(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md": "# Project\n[broken](does-not-exist.md)\n",
	})
	broken, _, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(broken) != 1 {
		t.Fatalf("broken = %v; want 1 entry", broken)
	}
	want := "README.md: missing -> does-not-exist.md"
	if broken[0] != want {
		t.Fatalf("broken[0] = %q; want %q", broken[0], want)
	}
}

func TestCheck_OrphanMarkdown(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md":      "# Project\n",
		"docs/orphan.md": "# Orphan\n",
	})
	broken, orphans, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(broken) != 0 {
		t.Fatalf("broken = %v; want []", broken)
	}
	if len(orphans) != 1 || orphans[0] != "docs/orphan.md" {
		t.Fatalf("orphans = %v; want [docs/orphan.md]", orphans)
	}
}

func TestCheck_LinkedDocsAreNotOrphans(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md":     "# Project\n[guide](docs/guide.md)\n",
		"docs/guide.md": "# Guide\n",
	})
	_, orphans, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans = %v; want []", orphans)
	}
}

func TestCheck_DefaultAllowlistSkipsDecisions(t *testing.T) {
	// docs/decisions.md is on DefaultAllowOrphans — even unlinked,
	// it must NOT show up as an orphan.
	dir := setupFixture(t, map[string]string{
		"README.md":         "# Project\n",
		"docs/decisions.md": "# Decisions\n",
	})
	_, orphans, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans = %v; want [] (decisions.md is allowlisted)", orphans)
	}
}

func TestCheck_DirectoryPrefixAllowlist(t *testing.T) {
	// docs/decisions-branches/ trailing-slash entry: any .md under
	// it must be exempt.
	dir := setupFixture(t, map[string]string{
		"README.md":                          "# Project\n",
		"docs/decisions-branches/feature.md": "# Feature branch decisions\n",
	})
	_, orphans, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans = %v; want [] (branch decisions dir is allowlisted)", orphans)
	}
}

// TestCheck_ExcludesReviewWritebacks: `docs/reviews/PR-<n>.md` files
// are SPEC §6.2 review-writebacks emitted by clud-bug-app. They are
// append-only review telemetry, never cross-linked from anywhere — so
// the default allow-orphan list MUST exempt them. Without this exempt,
// every PR that picks up a clud-bug review would fail `check-links`
// with an orphan finding (the recurring regression that motivated this
// test).
func TestCheck_ExcludesReviewWritebacks(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md":             "# Project\n",
		"docs/reviews/PR-42.md": "# Review for PR #42\n",
	})
	_, orphans, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans = %v; want [] (docs/reviews/ is the SPEC §6.2 review-writeback path)", orphans)
	}
}

// TestCheck_ExcludesReviewWritebacks_MultipleFiles: confirm the
// directory-prefix exemption applies to every file under docs/reviews/,
// including a hypothetical README.md a maintainer might drop in for
// human orientation. The directory is owned by the App + sync pipeline,
// so any markdown under it is in-convention.
func TestCheck_ExcludesReviewWritebacks_MultipleFiles(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md":              "# Project\n",
		"docs/reviews/PR-1.md":   "# Review for PR #1\n",
		"docs/reviews/PR-147.md": "# Review for PR #147\n",
		"docs/reviews/README.md": "# Reviews directory\n",
	})
	_, orphans, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans = %v; want [] (entire docs/reviews/ directory is exempt)", orphans)
	}
}

func TestCheck_SkipsExternalLinks(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md": "# Project\n[ext](https://example.com)\n[mail](mailto:x@y.z)\n[anchor](#section)\n",
	})
	broken, _, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(broken) != 0 {
		t.Fatalf("broken = %v; want []", broken)
	}
}

func TestCheck_SkipsLinksInFencedCodeBlock(t *testing.T) {
	// Issue #60 regression test: fenced code blocks must be stripped
	// before the link scan, so example markdown syntax inside them
	// doesn't trip the checker.
	body := "# Project\n\n" +
		"```markdown\n" +
		"[broken-in-fence](does-not-exist.md)\n" +
		"```\n"
	dir := setupFixture(t, map[string]string{
		"README.md": body,
	})
	broken, _, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(broken) != 0 {
		t.Fatalf("broken = %v; want [] (link is inside fenced block)", broken)
	}
}

func TestCheck_SkipsLinksInInlineCodeSpan(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md": "# Project\n\nThe syntax is `[text](path)` — see docs.\n",
	})
	broken, _, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(broken) != 0 {
		t.Fatalf("broken = %v; want [] (link is inside inline-code span)", broken)
	}
}

// TestCheck_UnmatchedBacktickDoesNotEatBrokenLink — PR #83 review
// fix from link_check.py: an unmatched backtick mid-sentence must
// NOT consume real broken links several lines later.
func TestCheck_UnmatchedBacktickDoesNotEatBrokenLink(t *testing.T) {
	body := "# Project\n\n" +
		"Bare `mention without close.\n\n" +
		"[broken](does-not-exist.md)\n"
	dir := setupFixture(t, map[string]string{
		"README.md": body,
	})
	broken, _, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(broken) != 1 {
		t.Fatalf("broken = %v; want 1 entry (unmatched backtick must not eat the link)", broken)
	}
}

func TestFormatReport_BrokenAndOrphans(t *testing.T) {
	out := FormatReport(
		[]string{"a.md: missing -> b.md"},
		[]string{"docs/c.md"},
	)
	want := "Broken links (1):\n  - a.md: missing -> b.md\n\nOrphan markdown files (1):\n  - docs/c.md"
	if out != want {
		t.Fatalf("FormatReport drift:\n--- want ---\n%s\n--- got ---\n%s", want, out)
	}
}

// --- helpers -------------------------------------------------------------

func setupFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// Sanity: keep strings imported (used by the build-tagged version).
var _ = strings.HasPrefix

// --- CheckReport tests (v1.2.0, plan §8.7) -------------------------------

// TestCheckWithReport_CleanRepo: parity with Check on a clean fixture.
// Report carries empty slices + HasIssues==false.
func TestCheckWithReport_CleanRepo(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md": "# Project\n",
	})
	report, err := CheckWithReport(dir, nil, nil)
	if err != nil {
		t.Fatalf("CheckWithReport: %v", err)
	}
	if report.HasIssues() {
		t.Fatalf("clean repo HasIssues = true; want false")
	}
	if len(report.Broken) != 0 || len(report.Orphans) != 0 {
		t.Fatalf("clean report has entries: %+v", report)
	}
}

// TestCheckWithReport_BrokenLinkFix_TimelineRegen: a broken link FROM
// docs/timeline.md TO docs/decisions-branches/<branch>.md is the
// signature stale-derived-doc case. SuggestedFix must point at
// `logmind timeline --write docs/timeline.md`.
func TestCheckWithReport_BrokenLinkFix_TimelineRegen(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md": "# Project\n[t](docs/timeline.md)\n",
		"docs/timeline.md": "# Timeline\n\n" +
			"- 2026-01-01 [link](decisions-branches/missing.md)\n",
	})
	report, err := CheckWithReport(dir, nil, nil)
	if err != nil {
		t.Fatalf("CheckWithReport: %v", err)
	}
	if len(report.Broken) != 1 {
		t.Fatalf("expected 1 broken; got %d: %+v", len(report.Broken), report.Broken)
	}
	if !strings.Contains(report.Broken[0].SuggestedFix, "logmind timeline --write") {
		t.Fatalf("SuggestedFix should suggest timeline regen; got: %q",
			report.Broken[0].SuggestedFix)
	}
}

// TestCheckWithReport_BrokenLinkFix_Generic: a broken link with no
// timeline/file-structure heuristic match falls back to the generic
// "remove or restore" suggestion.
func TestCheckWithReport_BrokenLinkFix_Generic(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md": "# Project\n[broken](nope.md)\n",
	})
	report, err := CheckWithReport(dir, nil, nil)
	if err != nil {
		t.Fatalf("CheckWithReport: %v", err)
	}
	if len(report.Broken) != 1 {
		t.Fatalf("expected 1 broken; got %d", len(report.Broken))
	}
	if !strings.Contains(report.Broken[0].SuggestedFix, "remove the dead link") {
		t.Fatalf("generic SuggestedFix expected; got: %q", report.Broken[0].SuggestedFix)
	}
}

// TestCheckWithReport_OrphanFix_KnownWellKnown: docs/timeline.md as
// an orphan gets the canonical AGENTS.md suggestion (logmind
// convention).
func TestCheckWithReport_OrphanFix_KnownWellKnown(t *testing.T) {
	// timeline.md is not on DefaultAllowOrphans, so when no doc links
	// to it the orphan check fires.
	dir := setupFixture(t, map[string]string{
		"README.md":        "# Project\n",
		"docs/timeline.md": "# Timeline\n",
	})
	report, err := CheckWithReport(dir, nil, nil)
	if err != nil {
		t.Fatalf("CheckWithReport: %v", err)
	}
	if len(report.Orphans) != 1 {
		t.Fatalf("expected 1 orphan; got %d: %+v", len(report.Orphans), report.Orphans)
	}
	if !strings.Contains(report.Orphans[0].SuggestedFix, "AGENTS.md") {
		t.Fatalf("expected AGENTS.md suggestion for timeline.md; got: %q",
			report.Orphans[0].SuggestedFix)
	}
}

// TestCheckWithReport_OrphanFix_GenericReadme: an arbitrary orphan
// under docs/ gets the README.md heuristic when a root README exists.
func TestCheckWithReport_OrphanFix_GenericReadme(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md":       "# Project\n",
		"docs/install.md": "# Install\n",
	})
	report, err := CheckWithReport(dir, nil, nil)
	if err != nil {
		t.Fatalf("CheckWithReport: %v", err)
	}
	if len(report.Orphans) != 1 {
		t.Fatalf("expected 1 orphan; got %d", len(report.Orphans))
	}
	fix := report.Orphans[0].SuggestedFix
	// Either README.md (parent doc by path heuristic) or AGENTS.md
	// fallback — both are valid resolutions of the heuristic. Test
	// that ONE of the two canonical suggestions fires.
	if !strings.Contains(fix, "README.md") && !strings.Contains(fix, "AGENTS.md") {
		t.Fatalf("expected README.md or AGENTS.md suggestion; got: %q", fix)
	}
}

// TestCheckWithReport_FindingFields: HasIssues + every Finding carries
// non-empty Path, Reason, SuggestedFix.
func TestCheckWithReport_FindingFields(t *testing.T) {
	dir := setupFixture(t, map[string]string{
		"README.md":       "# Project\n[broken](nope.md)\n",
		"docs/install.md": "# Install\n",
	})
	report, err := CheckWithReport(dir, nil, nil)
	if err != nil {
		t.Fatalf("CheckWithReport: %v", err)
	}
	if !report.HasIssues() {
		t.Fatalf("expected HasIssues=true")
	}
	for _, f := range append(report.Broken, report.Orphans...) {
		if f.Path == "" {
			t.Errorf("Finding.Path is empty: %+v", f)
		}
		if f.Reason == "" {
			t.Errorf("Finding.Reason is empty: %+v", f)
		}
		if f.SuggestedFix == "" {
			t.Errorf("Finding.SuggestedFix is empty: %+v", f)
		}
	}
}
