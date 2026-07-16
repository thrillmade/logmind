// spec_advisory_test.go — exercises collectSpecAdvisories / the
// CollectStatus.SpecAdvisories field (H2 of the canonical-spec-file
// feature): configured-but-missing, configured-but-empty,
// configured-with-an-unsafe-path, and the unset-but-a-conventional-file-
// exists nudge. Every case is ADVISORY ONLY — Overall must stay OK.
package doctor

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecAdvisories_Unset_NoConventionalFile_NoAdvisory(t *testing.T) {
	dir := freshRepo(t)
	r := CollectStatus(dir, true)
	if len(r.SpecAdvisories) != 0 {
		t.Errorf("SpecAdvisories = %v; want none (unset, nothing on disk)", r.SpecAdvisories)
	}
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the spec advisory must never be drift")
	}
}

// TestSpecAdvisories_Unset_NudgeChecksCandidates exercises the nudge's
// candidate scan. Note: SPEC.md and spec.md are DELIBERATELY not
// distinguished here — on the case-insensitive filesystems most developer
// machines use (macOS default, Windows), the two names refer to the same
// inode, so pinning one over the other would be testing OS behavior, not
// logmind's. What IS meaningfully testable regardless of filesystem
// case-sensitivity is: (a) docs/spec.md alone is found, and (b) a top-level
// candidate (SPEC.md/spec.md) takes priority over docs/spec.md when both
// exist.
func TestSpecAdvisories_Unset_NudgeChecksCandidates(t *testing.T) {
	cases := []struct {
		name        string
		filesUp     []string
		wantSubstr  string // matched case-insensitively
		mustNotHave string
	}{
		{name: "docs-spec-only", filesUp: []string{"docs/spec.md"}, wantSubstr: "docs/spec.md"},
		{name: "top-level-spec-only", filesUp: []string{"SPEC.md"}, wantSubstr: "spec.md"},
		{name: "top-level-wins-over-docs", filesUp: []string{"SPEC.md", "docs/spec.md"}, wantSubstr: "spec.md", mustNotHave: "docs/spec.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := freshRepo(t)
			for _, f := range tc.filesUp {
				mustWrite(t, filepath.Join(dir, filepath.FromSlash(f)), "content\n")
			}
			r := CollectStatus(dir, true)
			if len(r.SpecAdvisories) != 1 {
				t.Fatalf("SpecAdvisories = %v; want exactly 1 nudge", r.SpecAdvisories)
			}
			got := r.SpecAdvisories[0]
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tc.wantSubstr)) {
				t.Errorf("nudge = %q; want it to mention %q (case-insensitive)", got, tc.wantSubstr)
			}
			if tc.mustNotHave != "" && strings.Contains(got, tc.mustNotHave) {
				t.Errorf("nudge = %q; must not mention %q (a top-level candidate should win)", got, tc.mustNotHave)
			}
			mustContainSubstr(t, got, "context.spec_file")
			if r.Overall == "DRIFT" {
				t.Errorf("Overall = DRIFT; the spec advisory must never be drift")
			}
		})
	}
}

func TestSpecAdvisories_ConfiguredButMissing(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "context:\n  spec_file: docs/spec.md\n")
	r := CollectStatus(dir, true)
	if len(r.SpecAdvisories) != 1 {
		t.Fatalf("SpecAdvisories = %v; want 1 (configured but missing)", r.SpecAdvisories)
	}
	mustContainSubstr(t, r.SpecAdvisories[0], "missing")
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the spec advisory must never be drift")
	}
}

func TestSpecAdvisories_ConfiguredButEmpty(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, "docs", "spec.md"), "")
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "context:\n  spec_file: docs/spec.md\n")
	r := CollectStatus(dir, true)
	if len(r.SpecAdvisories) != 1 {
		t.Fatalf("SpecAdvisories = %v; want 1 (configured but empty)", r.SpecAdvisories)
	}
	mustContainSubstr(t, r.SpecAdvisories[0], "empty")
}

func TestSpecAdvisories_ConfiguredButWhitespaceOnly(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, "docs", "spec.md"), "   \n\t\n")
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "context:\n  spec_file: docs/spec.md\n")
	r := CollectStatus(dir, true)
	if len(r.SpecAdvisories) != 1 {
		t.Fatalf("SpecAdvisories = %v; want 1 (whitespace-only counts as empty)", r.SpecAdvisories)
	}
	mustContainSubstr(t, r.SpecAdvisories[0], "empty")
}

func TestSpecAdvisories_ConfiguredAbsolutePath(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, "docs", "spec.md"), "content\n")
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"),
		"context:\n  spec_file: "+filepath.Join(dir, "docs", "spec.md")+"\n")
	r := CollectStatus(dir, true)
	if len(r.SpecAdvisories) != 1 {
		t.Fatalf("SpecAdvisories = %v; want 1 (absolute path)", r.SpecAdvisories)
	}
	mustContainSubstr(t, r.SpecAdvisories[0], "absolute")
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the spec advisory must never be drift")
	}
}

func TestSpecAdvisories_ConfiguredOutOfRootEscape(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "context:\n  spec_file: ../evil.md\n")
	r := CollectStatus(dir, true)
	if len(r.SpecAdvisories) != 1 {
		t.Fatalf("SpecAdvisories = %v; want 1 (out-of-root escape)", r.SpecAdvisories)
	}
	mustContainSubstr(t, r.SpecAdvisories[0], "escapes the repo root")
}

func TestSpecAdvisories_ConfiguredAndPresentAndNonEmpty_NoAdvisory(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, "docs", "spec.md"), "# Spec\n\nReal content.\n")
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "context:\n  spec_file: docs/spec.md\n")
	r := CollectStatus(dir, true)
	if len(r.SpecAdvisories) != 0 {
		t.Errorf("SpecAdvisories = %v; want none (healthy configuration)", r.SpecAdvisories)
	}
}

func TestSpecAdvisories_JSONFieldPresent(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "context:\n  spec_file: docs/spec.md\n")
	r := CollectStatus(dir, true)
	js, err := r.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	mustContainSubstr(t, js, `"spec_advisories"`)
	mustContainSubstr(t, js, "missing")
}

func TestSpecAdvisories_RenderStatus_HumanTable(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "context:\n  spec_file: docs/spec.md\n")
	r := CollectStatus(dir, true)
	body := RenderStatus(r)
	mustContainSubstr(t, body, "Canonical spec file")
	mustContainSubstr(t, body, "missing")
	mustContainSubstr(t, body, "Stack status: OK")
}

func mustContainSubstr(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected %q to contain %q", haystack, needle)
	}
}
