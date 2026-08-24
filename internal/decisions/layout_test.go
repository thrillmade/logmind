// layout_test.go — the WRITER and the commit GATE ask Layout the same
// question, so the tests that matter here are the ones that would catch the
// two answering differently. The end-to-end pin is
// TestLogWritesWhereTheCommitGateLooks (internal/cli), which drives
// `logmind log` and then `guard-commit` over a real index; these are the
// same properties one level down, where each rule can be isolated.
package decisions

import (
	"os"
	"path/filepath"
	"testing"
)

// foldsCase reports whether dir's filesystem answers to a spelling that is
// not the one on disk. Measured, not assumed from GOOS: a case-sensitive
// volume can be mounted on a case-folding platform and the rules that
// matter here are the volume's.
func foldsCase(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatalf("mkdir probe: %v", err)
	}
	defer func() { _ = os.RemoveAll(probe) }()
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

// TestLayout_PredicateIsTheImageOfTheWriter is the fence: for every path
// Layout hands the WRITER, the same Layout must accept the repo-relative
// spelling from the GATE. It carries no copy of the two shapes — it asks
// for them — so a rename, a re-root or a re-spelling moves both sides
// together or fails here.
//
// Run over each layout a repository can actually be in, because the
// property held for the plain one and not for the other two, which is
// precisely how a decision logmind had just written became uncommittable.
func TestLayout_PredicateIsTheImageOfTheWriter(t *testing.T) {
	cases := []struct {
		name string
		// setup prepares the repository and returns the directory the
		// WRITER resolves from — which is not always the repository root.
		setup func(t *testing.T, repo string) string
	}{
		{
			name: "docs at the repository root",
			setup: func(t *testing.T, repo string) string {
				mkdirAll(t, filepath.Join(repo, "docs"))
				return repo
			},
		},
		{
			name: "a Docs directory the repository already had",
			setup: func(t *testing.T, repo string) string {
				if !foldsCase(t, repo) {
					t.Skip("filesystem is case-sensitive: `Docs/` and `docs/` are two directories here, so this configuration cannot arise")
				}
				mkdirAll(t, filepath.Join(repo, "Docs"))
				return repo
			},
		},
		{
			name: "a logmind project below the repository root",
			setup: func(t *testing.T, repo string) string {
				project := filepath.Join(repo, "pkg", "api")
				mkdirAll(t, filepath.Join(project, "docs"))
				mkdirAll(t, filepath.Join(project, ".logmind"))
				writeFileT(t, filepath.Join(project, ".logmind", "config.yml"), "decisions:\n  branch_aware: true\n")
				return project
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			project := tc.setup(t, repo)

			writer := ResolveLayout(project)
			gate := ResolveLayout(repo)
			for _, abs := range []string{writer.LegacyFile(), writer.BranchFile("main"), writer.BranchFile("fix__gate")} {
				// Written for real: the gate judges paths git reports, and
				// git reports what is on disk.
				mkdirAll(t, filepath.Dir(abs))
				writeFileT(t, abs, "## 2026-01-01 00:00 - probe\n")

				rel, err := filepath.Rel(repo, abs)
				if err != nil {
					t.Fatalf("Rel(%q): %v", abs, err)
				}
				rel = filepath.ToSlash(rel)
				if !gate.IsDecisionRel(rel) {
					t.Errorf("IsDecisionRel(%q) = false; the writer puts a decision there", rel)
				}
			}
		})
	}
}

// TestLayout_RejectsWhatTheWriterNeverProduces is the other half — without
// it, a predicate that answers true to everything scores identically above.
//
// Every row except README.md and src/decisions.txt used to be accepted: the
// predicate this replaces took a `/decisions.md` suffix in ANY directory,
// and a well-formed entry at internal/x/decisions.md alongside 302 lines of
// new Go cleared the commit gate.
func TestLayout_RejectsWhatTheWriterNeverProduces(t *testing.T) {
	repo := t.TempDir()
	mkdirAll(t, filepath.Join(repo, "docs"))
	layout := ResolveLayout(repo)

	for _, rel := range []string{
		"internal/x/decisions.md",
		"nested/path/decisions.md",
		"decisions.md",
		"vendor/docs/decisions.md",
		// A docs/ shape with no logmind project behind it. The nearest miss
		// there is: the decoy only has to add one directory level.
		"internal/x/docs/decisions.md",
		"internal/x/docs/decisions-branches/main.md",
		// Under the branch dir but not a file any read path enumerates —
		// ListBranchFiles skips subdirectories.
		"docs/decisions-branches/nested/other.md",
		// Under the branch dir but not markdown, and the empty stem.
		"docs/decisions-branches/notes.txt",
		"docs/decisions-branches/.md",
		"docs/decisions-archive.md",
		"README.md",
		"src/decisions.txt",
		// Nothing may reach outside the repository to find a project root.
		"../elsewhere/docs/decisions.md",
	} {
		if layout.IsDecisionRel(rel) {
			t.Errorf("IsDecisionRel(%q) = true; `logmind log` never writes there", rel)
		}
	}
}

// TestLayout_NestedProjectNeedsAProjectBehindIt pins the ONE thing that
// separates the two nearest-miss paths above and below: pkg/api/docs/... is
// accepted because `logmind init` ran in pkg/api, and internal/x/docs/... is
// not because nothing did. Both rows in one test, because "accepted" and
// "rejected" are only evidence as a pair.
func TestLayout_NestedProjectNeedsAProjectBehindIt(t *testing.T) {
	repo := t.TempDir()
	mkdirAll(t, filepath.Join(repo, "docs"))
	layout := ResolveLayout(repo)

	const rel = "pkg/api/docs/decisions.md"
	if layout.IsDecisionRel(rel) {
		t.Fatalf("IsDecisionRel(%q) = true before pkg/api is a logmind project — a decision-shaped path is not a decision record", rel)
	}
	mkdirAll(t, filepath.Join(repo, "pkg", "api", ".logmind"))
	writeFileT(t, filepath.Join(repo, "pkg", "api", ".logmind", "config.yml"), "decisions:\n  branch_aware: true\n")
	if !layout.IsDecisionRel(rel) {
		t.Fatalf("IsDecisionRel(%q) = false after `logmind init` ran in pkg/api — `logmind log` there writes exactly this path", rel)
	}
}

// TestResolveLayout_ReportsTheSpellingOnDisk pins the half the operator
// reads rather than the half the gate does: `logmind log`'s receipt names
// the file it wrote, and on a case-folding volume the literal join names one
// that does not exist as spelled.
func TestResolveLayout_ReportsTheSpellingOnDisk(t *testing.T) {
	repo := t.TempDir()
	if !foldsCase(t, repo) {
		t.Skip("filesystem is case-sensitive: `Docs/` and `docs/` are two directories here, so this configuration cannot arise")
	}
	mkdirAll(t, filepath.Join(repo, "Docs"))

	layout := ResolveLayout(repo)
	if got, want := layout.Dir(), filepath.Join(repo, "Docs"); got != want {
		t.Errorf("Dir() = %q; want %q — the directory the write actually lands in", got, want)
	}
	// And the gate takes either spelling here, because the filesystem
	// cannot tell them apart and git may report the index's rather than the
	// working tree's.
	for _, rel := range []string{"Docs/decisions.md", "docs/decisions.md"} {
		if !layout.IsDecisionRel(rel) {
			t.Errorf("IsDecisionRel(%q) = false; on this volume that is the same file logmind wrote", rel)
		}
	}
}

// TestResolveLayout_DoesNotInventADocsDirectory: a repository with no docs
// directory at all still gets the documented name, because `logmind init`
// is about to create it and `logmind log` prints its absence as an error
// naming `docs/`.
func TestResolveLayout_DoesNotInventADocsDirectory(t *testing.T) {
	repo := t.TempDir()
	layout := ResolveLayout(repo)
	if got, want := layout.Dir(), filepath.Join(repo, DocsDirName); got != want {
		t.Errorf("Dir() = %q; want %q", got, want)
	}
	if !layout.IsDecisionRel(DocsDirName + "/" + LegacyFileName) {
		t.Error("the repository-root record is not recognised before docs/ exists")
	}
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func writeFileT(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
