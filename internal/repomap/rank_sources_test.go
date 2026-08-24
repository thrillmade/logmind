// rank_sources_test.go — the repomap's decision corpus must cover EVERY
// decision file, not a hand-kept copy of the list.
//
// decisionLinkedPaths spelled its own literal {decisions.md, timeline.md,
// timeline-archive.md} and dropped docs/decisions-archive.md — in the same
// change that declared decisions.NonBranchSources() the single owner of that
// set. A file named only in a repo's legacy archive silently stopped counting
// as load-bearing, so under a token budget it got packed out of the map first.
package repomap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/tree"
)

// TestRank_DecisionLinked_CoversEveryDecisionSource ranks the same repo once
// per decision file, moving the ONLY mention of `lonely/lonely.go` between
// them. Whichever file holds it, it must outrank the higher-fan-in `hub`.
//
// The per-source table is generated from decisions.NonBranchSources() plus a
// branch file, so a source added to the owner is automatically covered here
// and a source dropped from the corpus fails.
func TestRank_DecisionLinked_CoversEveryDecisionSource(t *testing.T) {
	var relPaths []string
	for _, src := range decisions.NonBranchSources() {
		relPaths = append(relPaths, "docs/"+src.File)
	}
	relPaths = append(relPaths, "docs/decisions-branches/feat__x.md")

	for _, rel := range relPaths {
		t.Run(rel, func(t *testing.T) {
			dir := t.TempDir()
			write := func(r, src string) {
				p := filepath.Join(dir, filepath.FromSlash(r))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write("go.mod", "module example.com/m\n\ngo 1.22\n")
			write("hub/hub.go", "package hub\nfunc H() {}\n")
			write("a/a.go", "package a\nimport \"example.com/m/hub\"\nfunc A() { hub.H() }\n")
			write("b/b.go", "package b\nimport \"example.com/m/hub\"\nfunc B() { hub.H() }\n")
			write("lonely/lonely.go", "package lonely\nfunc L() {}\n")
			// The ONLY mention of lonely/lonely.go, in this run's source.
			write(rel, "## 2025-01-01 09:00 - X\n\nWe refactored lonely/lonely.go for good reasons.\n")

			rules, err := tree.ResolveRules(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			files, err := Extract(dir, rules)
			if err != nil {
				t.Fatal(err)
			}

			linked := decisionLinkedPaths(dir, files)
			if !linked["lonely/lonely.go"] {
				t.Fatalf("a decision recorded in %s did not make lonely/lonely.go decision-linked — that source is not in the repomap's corpus", rel)
			}
			// CONTROL, same repo: a file NOT named in any decision must not be
			// linked, so the assertion above cannot pass by linking everything.
			if linked["b/b.go"] {
				t.Errorf("b/b.go is named in no decision but was reported decision-linked")
			}
			// And the ranking actually moves: decision-linked outranks fan-in.
			if got, want := paths(Rank(dir, files)), "lonely/lonely.go,hub/hub.go,a/a.go,b/b.go"; got != want {
				t.Errorf("rank order = %q; want %q (decision recorded in %s)", got, want, rel)
			}
		})
	}
}
