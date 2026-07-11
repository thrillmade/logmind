// show_test.go — exercises `logmind show` against tmpdir fixtures.
//
// Coverage:
//   - default branch streams docs/decisions.md
//   - feature branch streams docs/decisions-branches/<branch>.md, NOT
//     docs/decisions.md (the "current branch" contract SKILL.md/AGENTS.md
//     document)
//   - no decisions logged yet on the current branch → friendly message
//   - --all appends the archive under an ARCHIVED DECISIONS banner when the
//     archive file exists, and a "(no archive)" ok-suffix when it doesn't
//   - --quiet collapses stdout to exactly one `ok k=v` line
//   - docs/ missing → friendly error + ErrSilent
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustMkdir + mustWrite are tiny fixture helpers shared by show_test.go and
// search_test.go — both build docs/ trees directly (without going through
// `logmind init`/`logmind log`) so tests can pin exact file contents.
func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// runShowCmd runs `logmind show [extraArgs...]` and returns combined output.
func runShowCmd(t *testing.T, extraArgs ...string) string {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(append([]string{"show"}, extraArgs...))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("show %v: %v\n%s", extraArgs, err, out.String())
	}
	return out.String()
}

// TestShow_DefaultBranch_StreamsDecisionsMd: on the default branch, `show`
// streams docs/decisions.md verbatim and prints the ok-trailer.
func TestShow_DefaultBranch_StreamsDecisionsMd(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		withFakeTTY(t, false, func() { logOnce(t, "Use PostgreSQL") })

		body := runShowCmd(t)
		mustContain(t, body, "## ")
		mustContain(t, body, "Use PostgreSQL")
		mustContain(t, body, "ok show: docs/decisions.md")
		mustContain(t, body, "bytes")
	})
}

// TestShow_FeatureBranch_StreamsBranchFile: on a feature branch, `show`
// streams docs/decisions-branches/<branch>.md — the SAME file `logmind log`
// just wrote to — not docs/decisions.md. This is the "current branch"
// contract the docs promise.
func TestShow_FeatureBranch_StreamsBranchFile(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/login")
		withFakeTTY(t, false, func() { logOnce(t, "Add JWT auth") })

		body := runShowCmd(t)
		mustContain(t, body, "Add JWT auth")
		mustContain(t, body, "ok show: docs/decisions-branches/feat__login.md")
		// The default-branch decisions.md content must NOT leak in.
		if strings.Contains(body, "Initialize logmind decision tracking") {
			t.Errorf("feature-branch show leaked docs/decisions.md content:\n%s", body)
		}
	})
}

// TestShow_NoDecisionsYetOnBranch: a fresh feature branch with no `logmind
// log` yet has no decisions-branches file on disk — `show` reports the
// friendly empty message rather than erroring.
func TestShow_NoDecisionsYetOnBranch(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/empty")

		body := runShowCmd(t)
		mustContain(t, body, "No decisions logged yet on this branch.")
		mustContain(t, body, "ok show:")
	})
}

// TestShow_All_ArchiveVariants: --all appends the archive under a banner
// when it exists, and degrades gracefully (no banner, "(no archive)"
// ok-suffix) when it doesn't.
func TestShow_All_ArchiveVariants(t *testing.T) {
	cases := []struct {
		name         string
		writeArchive bool
		wantBanner   bool
		wantOkSuffix string
	}{
		{name: "archive present", writeArchive: true, wantBanner: true, wantOkSuffix: "+ archive"},
		{name: "archive absent", writeArchive: false, wantBanner: false, wantOkSuffix: "(no archive)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTempCwd(t, func(d string) {
				mustMkdir(t, filepath.Join(d, "docs"))
				mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
					"## 2026-06-01 10:00 - Main decision\n")
				if tc.writeArchive {
					mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
						"## 2025-01-01 09:00 - Archived decision\n")
				}

				body := runShowCmd(t, "--all")
				mustContain(t, body, "Main decision")
				if tc.wantBanner {
					mustContain(t, body, "ARCHIVED DECISIONS")
					mustContain(t, body, "Archived decision")
				} else if strings.Contains(body, "ARCHIVED DECISIONS") {
					t.Errorf("did not expect ARCHIVED DECISIONS banner:\n%s", body)
				}
				mustContain(t, body, tc.wantOkSuffix)
			})
		})
	}
}

// TestShow_Quiet_EmitsOneOkLine: --quiet suppresses the verbatim body —
// matching `logmind repomap`'s stdout-sink precedent — leaving exactly one
// `ok k=v` line.
func TestShow_Quiet_EmitsOneOkLine(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Quiet decision\n")

		body := runShowCmd(t, "--quiet")
		lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
		if len(lines) != 1 {
			t.Fatalf("quiet show: want exactly 1 line, got %d:\n%s", len(lines), body)
		}
		if !strings.HasPrefix(lines[0], "ok show ") {
			t.Errorf("quiet show line = %q; want prefix %q", lines[0], "ok show ")
		}
		if strings.Contains(body, "Quiet decision") {
			t.Errorf("quiet show leaked the decision body:\n%s", body)
		}
		mustContain(t, body, "path=docs/decisions.md")
	})
}

// TestShow_DocsMissingErrors: no docs/ → friendly error on the error
// channel + ErrSilent.
func TestShow_DocsMissingErrors(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"show"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err == nil {
			t.Fatalf("expected ErrSilent when docs/ missing")
		}
		mustContain(t, out.String(), "docs/ directory not found")
	})
}
