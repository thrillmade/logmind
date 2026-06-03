package linkcheck

import (
	"os"
	"os/exec"
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
		"README.md": "# Project\n",
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

// TestCheck_ParityVsPython runs the same fixture through the Python
// link_check and asserts byte-equal report strings. Skipped when
// python3 isn't available.
func TestCheck_ParityVsPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH; skipping byte-identical-vs-Python check")
	}
	dir := setupFixture(t, map[string]string{
		"README.md":      "# Project\n[broken](missing.md)\n[ok](docs/guide.md)\n",
		"docs/guide.md":  "# Guide\n",
		"docs/orphan.md": "# Orphan\n",
	})
	broken, orphans, err := Check(dir, nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	goReport := FormatReport(broken, orphans)

	repoRoot := repoRootFromCaller(t)
	cmd := exec.Command("python3", "-c", `
import sys, os
sys.path.insert(0, 'src')
os.chdir(sys.argv[1])
from logmind.actions.link_check import check, format_report
broken, orphans = check(__import__('pathlib').Path('.'))
print(format_report(broken, orphans), end='')
`, dir)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("python3 link_check unavailable: %v\n%s", err, out)
	}
	pyReport := string(out)
	if goReport != pyReport {
		t.Fatalf("link-check report drift vs Python:\n--- go ---\n%s\n--- py ---\n%s",
			goReport, pyReport)
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

func repoRootFromCaller(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod walking up from %s", wd)
	return ""
}

// Sanity: keep strings imported (used by the build-tagged version).
var _ = strings.HasPrefix
