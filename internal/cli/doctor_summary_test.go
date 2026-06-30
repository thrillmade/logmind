package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWriteUnder(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestHeadline_FileFlag_TargetsArbitraryBranch: `logmind headline --file <path>`
// summarizes an OLD/non-current branch (stay on the default branch, target the
// file explicitly) — the affordance the doctor advisory points agents at.
func TestHeadline_FileFlag_TargetsArbitraryBranch(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		optIntoMainCanonical(t, d)
		mustWriteUnder(t, d, "docs/decisions-branches/feat__old.md",
			"← back\n\n## 2026-06-10 09:00 - Old work\n\n---\n")
		root := NewRootCmd()
		root.SetArgs([]string{"headline", "--file", "docs/decisions-branches/feat__old.md", "Did the old thing well"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("headline --file: %v\n%s", err, out.String())
		}
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__old.md"))
	if !strings.Contains(s, "— Did the old thing well\n") {
		t.Errorf("--file did not set the summary on the targeted branch:\n%s", s)
	}
	if n := strings.Count(s, "logmind-entry-start"); n != 1 {
		t.Errorf("marker count = %d; want 1", n)
	}
}

// TestBackfillBranchSummaries: the doctor --fix backfill inserts a marker into
// markerless branch files, skips already-markered ones, and is idempotent.
func TestBackfillBranchSummaries(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, ".logmind/config.yml", "timeline:\n  canonical: main-canonical\n")
		mustWriteUnder(t, d, "docs/decisions-branches/feat__old.md",
			"← back\n\n## 2026-06-10 09:00 - Old\n\n---\n")
		mustWriteUnder(t, d, "docs/decisions-branches/feat__has.md",
			"← back\n\n<!-- logmind-entry-start: 2026-06-11-x -->\n- **2026-06-11** — X\n<!-- logmind-entry-end -->\n\n## 2026-06-11 10:00 - X\n\n---\n")
		if n := backfillBranchSummaries(d); n != 1 {
			t.Errorf("backfillBranchSummaries = %d; want 1 (only the markerless one)", n)
		}
	})
	if n := strings.Count(readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__old.md")), "logmind-entry-start"); n != 1 {
		t.Errorf("markerless file not backfilled (markers=%d)", n)
	}
	// Idempotent: a second pass changes nothing.
	if n := backfillBranchSummaries(dir); n != 0 {
		t.Errorf("second backfill = %d; want 0 (idempotent)", n)
	}
}

// TestBackfillBranchSummaries_DefaultModeNoop: branch-divergent mode never
// backfills (the feature is main-canonical-only → default repos byte-stable).
func TestBackfillBranchSummaries_DefaultModeNoop(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, ".logmind/config.yml", "git:\n  auto_commit: true\n")
		mustWriteUnder(t, d, "docs/decisions-branches/feat__old.md",
			"← back\n\n## 2026-06-10 09:00 - Old\n\n---\n")
		if n := backfillBranchSummaries(d); n != 0 {
			t.Errorf("default-mode backfill = %d; want 0", n)
		}
	})
	if strings.Contains(readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__old.md")), "logmind-entry-start") {
		t.Errorf("default mode wrote a marker; want none")
	}
}
