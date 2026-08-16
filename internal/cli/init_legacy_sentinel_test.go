// init_legacy_sentinel_test.go — the tree this binary scaffolds must stay
// recognisable as "already initialised" to logmind v1.2.0.
//
// v1.2.0 is the NEWEST RELEASE: what `brew install thrillmade/tap/logmind`
// and the setup-logmind action put on a machine today. During the v2 rollout
// the two binaries meet in the same repository constantly, and the meeting is
// not symmetric — this side can be changed and v1.2.0 cannot. So the
// compatibility obligation runs one way: whatever v2 leaves on disk, a v1.2.0
// `logmind init` must read as initialised and go to refresh mode.
//
// The stake is not cosmetic. When v1.2.0's sentinel answers false it does not
// refresh; it takes the entire FRESH-INSTALL path, and step one of that path
// writes .logmind/config.yml from its own bundled template — silently
// discarding every key v1.2.0's template does not have. Measured on a
// §3.2-scaffolded tree before the fix: `git.enforce_commits: true` and
// `git.commit_line_threshold: 20` both gone, with `logmind init` reporting
// success.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/templates"
)

// TestRepoDecisionsPointer_IsInstalledHere is the dogfooding gate, same shape
// as TestRepoGitattributes_RegistersEveryDefaultLine: logmind's own repository
// must carry the compatibility file logmind installs in every consumer repo.
//
// This branch is precisely where the exposure arrives. Before it, this repo's
// docs/decisions.md was a real 116-line main log and v1.2.0 read the repo as
// initialised. The branch moves that log to docs/decisions-branches/main.md
// (§3.2) and leaves no docs/decisions.md behind — so the day this merges, a
// v1.2.0 binary run against `dev` reads logmind's own repository as
// UNINITIALISED and re-scaffolds it, discarding git.enforce_commits and
// git.commit_line_threshold from .logmind/config.yml. The hazard would arrive
// silently with the merge rather than existing to be noticed, which is the
// worse of the two.
//
// Byte-equality, not just existence: this file is the pointer and nothing
// writes it, so any drift from the template is drift.
func TestRepoDecisionsPointer_IsInstalledHere(t *testing.T) {
	path := filepath.Join(repoRootFromCaller(t), "docs", "decisions.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("logmind's own %s is missing: %v\n"+
			"`logmind init` installs it in every consumer repo so a pre-v2.0 binary still "+
			"recognises the repository as initialised (ensureLegacyPointer). A repo this "+
			"project scaffolds but does not scaffold itself is untested where it matters.", path, err)
	}
	if want := templates.DecisionsPointerTemplate(); string(body) != want {
		t.Errorf("logmind's own %s has drifted from templates.DecisionsPointerTemplate().\n--- want ---\n%s\n--- got ---\n%s",
			path, want, body)
	}
	// And it must still be inert: the sentinel must not have become a source.
	mustRouteNoDecisionsTo(t, path, "the repo's own pointer file holds no decisions")
}

// v120AlreadyInitialised is logmind v1.2.0's install sentinel, transcribed
// from the released binary's internal/cli/init.go:
//
//	alreadyInit := pathExists(filepath.Join(docsPath, "decisions.md")) &&
//		pathExists(configPath)
//
// Transcribed rather than imported because v1.2.0 is a shipped artifact this
// module has no way to call and no way to amend. Keeping the predicate here,
// spelled out, is what makes this an actual cross-version test rather than a
// restatement of whatever the current sentinel happens to be: docsScaffolded
// could change freely and this would keep asking v1.2.0's question.
func v120AlreadyInitialised(repoRoot string) bool {
	return pathExists(filepath.Join(repoRoot, "docs", "decisions.md")) &&
		pathExists(filepath.Join(repoRoot, ".logmind", "config.yml"))
}

// TestInit_ScaffoldStaysRecognisableToV120 is the regression fence for the
// mixed-version hazard: run THIS binary's `init`, then ask v1.2.0's question
// of the result.
func TestInit_ScaffoldStaysRecognisableToV120(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)

		if !v120AlreadyInitialised(d) {
			t.Fatalf("a tree scaffolded by this binary reads as UNINITIALISED to logmind v1.2.0.\n"+
				"That binary would take the fresh-install path and rewrite .logmind/config.yml.\n"+
				"docs/ contains: %v", lsDir(t, filepath.Join(d, "docs")))
		}

		// CONTROL — the predicate discriminates, so a pass above is evidence.
		// Delete the one file it tests and it must flip to false; if it does
		// not, this test would pass on a tree that has the bug.
		pointer := filepath.Join(d, "docs", "decisions.md")
		stash, err := os.ReadFile(pointer)
		if err != nil {
			t.Fatalf("read %s: %v", pointer, err)
		}
		if err := os.Remove(pointer); err != nil {
			t.Fatalf("remove %s: %v", pointer, err)
		}
		if v120AlreadyInitialised(d) {
			t.Fatal("control failed: v120AlreadyInitialised answers true with docs/decisions.md absent, so it cannot be detecting anything")
		}
		if err := os.WriteFile(pointer, stash, 0o644); err != nil {
			t.Fatalf("restore %s: %v", pointer, err)
		}

		// The config keys that were being destroyed. Named explicitly so a
		// future template edit that drops one trips here rather than in a
		// user's repo.
		cfgBody := mustReadFile(t, filepath.Join(d, ".logmind", "config.yml"))
		for _, key := range []string{"enforce_commits:", "commit_line_threshold:"} {
			if !strings.Contains(cfgBody, key) {
				t.Errorf(".logmind/config.yml is missing %q — the key whose loss this whole file exists to prevent", key)
			}
		}
	})
}

// TestInit_LegacyPointerHoldsNoDecisions: the compatibility file satisfies
// v1.2.0 WITHOUT becoming a source of phantom decisions. Measured through the
// same parser every read path uses, so a pass here is a pass for the timeline,
// `show`, `search` and `context` alike.
func TestInit_LegacyPointerHoldsNoDecisions(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)

		docsPath := filepath.Join(d, "docs")
		pointer := filepath.Join(docsPath, "decisions.md")

		_, raws, err := decisions.SplitRaw(pointer)
		if err != nil {
			t.Fatalf("SplitRaw(%s): %v", pointer, err)
		}
		if len(raws) != 0 {
			t.Fatalf("docs/decisions.md must contribute 0 decisions; got %d", len(raws))
		}

		all, err := decisions.Collect(docsPath, os.Stderr)
		if err != nil {
			t.Fatalf("Collect: %v", err)
		}
		for _, e := range all {
			if e.SourcePath == "decisions.md" {
				t.Errorf("Collect surfaced %q from the pointer file, which holds no decisions", e.Title)
			}
		}

		// CONTROL — Collect does read this path, so the absence above is
		// absence of entries and not absence of reading. Append one real
		// header and it must appear.
		body := mustReadFile(t, pointer)
		if err := os.WriteFile(pointer,
			[]byte(body+"\n## 2019-05-05 11:11 - control entry\n\n**Reasoning:** control\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", pointer, err)
		}
		after, err := decisions.Collect(docsPath, os.Stderr)
		if err != nil {
			t.Fatalf("Collect (control): %v", err)
		}
		found := false
		for _, e := range after {
			if e.Title == "control entry" {
				found = true
			}
		}
		if !found {
			t.Fatal("control failed: Collect does not read docs/decisions.md at all, so 'contributes no rows' proved nothing")
		}
	})
}

// TestInitRefresh_RestoresAMissingPointer: a repo scaffolded by a v2 build
// from before the pointer existed is exposed to exactly the same
// config-clobbering. Refresh is the upgrade path, so refresh closes it —
// ensureLegacyPointer is called from both routes into an initialised repo.
func TestInitRefresh_RestoresAMissingPointer(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)

		pointer := filepath.Join(d, "docs", "decisions.md")
		if err := os.Remove(pointer); err != nil {
			t.Fatalf("remove %s: %v", pointer, err)
		}
		if v120AlreadyInitialised(d) {
			t.Fatal("fixture precondition: with the pointer removed the repo must look uninitialised to v1.2.0")
		}

		scaffoldDocs(t) // second run — takes the refresh path

		if !v120AlreadyInitialised(d) {
			t.Fatal("`logmind init` in refresh mode left the repo unrecognisable to v1.2.0")
		}
	})
}

// TestInitRefresh_NeverOverwritesARealLegacyLog: the flip side, and the one
// that matters more. In a repo that predates §3.2, docs/decisions.md is a real
// decision log with real entries. Rewriting a user-owned artifact is not this
// code's business (SPEC line 1101), so ensureLegacyPointer writes only into
// absence — asserted on bytes, not on "the file still exists".
func TestInitRefresh_NeverOverwritesARealLegacyLog(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)

		pointer := filepath.Join(d, "docs", "decisions.md")
		legacy := "# Decisions\n\n## 2019-05-05 11:11 - Chose Postgres over MySQL\n\n**Reasoning:** pre-upgrade-rationale\n\n---\n"
		mustWrite(t, pointer, legacy)

		scaffoldDocs(t) // refresh path

		if got := mustReadFile(t, pointer); got != legacy {
			t.Fatalf("refresh rewrote a pre-§3.2 decision log.\nwant:\n%s\ngot:\n%s", legacy, got)
		}
	})
}

// TestInit_EmptyDocsDirIsNotAlreadyInitialised pins the other half of the
// sentinel. A repo carrying .logmind/config.yml beside an EMPTY docs/ is
// half-installed, and testing only that docs/ EXISTS reads that as
// initialised: init short-circuits into refresh mode and the repo is never
// scaffolded at all — no timeline, no file-structure, no decision file, and no
// error to say so.
func TestInit_EmptyDocsDirIsNotAlreadyInitialised(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		mustMkdir(t, filepath.Join(d, "docs"))
		mustMkdir(t, filepath.Join(d, ".logmind"))
		mustWrite(t, filepath.Join(d, ".logmind", "config.yml"), "git:\n  auto_push: false\n")

		scaffoldDocs(t)

		for _, rel := range []string{"timeline.md", "file-structure.md", "decisions.md"} {
			if !pathExists(filepath.Join(d, "docs", rel)) {
				t.Errorf("docs/%s missing — init took the refresh path against an empty docs/ and scaffolded nothing", rel)
			}
		}
	})
}

// TestInit_MigratedRepoStillRefreshes is the case the sentinel was widened FOR
// in the first place, kept pinned so the fix above cannot quietly undo it: a
// repo that moved its pre-§3.2 main log into docs/decisions-branches/main.md
// and deleted docs/decisions.md is INITIALISED, and init must refresh it
// rather than rewrite .logmind/config.yml over its settings.
func TestInit_MigratedRepoStillRefreshes(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)

		cfgPath := filepath.Join(d, ".logmind", "config.yml")
		sentinelValue := "# do-not-clobber-me\n"
		mustWrite(t, cfgPath, mustReadFile(t, cfgPath)+sentinelValue)
		if err := os.Remove(filepath.Join(d, "docs", "decisions.md")); err != nil {
			t.Fatalf("remove pointer: %v", err)
		}

		scaffoldDocs(t)

		if !strings.Contains(mustReadFile(t, cfgPath), sentinelValue) {
			t.Fatal("init rewrote .logmind/config.yml in a repo that had simply deleted docs/decisions.md")
		}
	})
}

// mustRouteNoDecisionsTo asserts that path carries no decision entries.
//
// It replaces the bare os.Stat("docs/decisions.md") that several §3.2 routing
// tests used to make. Absence of the FILE stopped being answerable once
// `logmind init` began scaffolding it as a compatibility pointer
// (ensureLegacyPointer) — but absence of the file was never what §3.2's one
// path rule is about. The rule is that a decision made on a branch lands in
// that branch's file, so "no decision was routed here" is the claim, and it is
// checked with the same parser every read path uses. A regression that routed
// a decision to docs/decisions.md still trips this: the entry would be there
// to find.
func mustRouteNoDecisionsTo(t *testing.T, path, why string) {
	t.Helper()
	_, raws, err := decisions.SplitRaw(path)
	if err != nil {
		t.Fatalf("SplitRaw(%s): %v", path, err)
	}
	if len(raws) != 0 {
		titles := make([]string, 0, len(raws))
		for _, r := range raws {
			titles = append(titles, r.Title)
		}
		t.Fatalf("%s holds %d decision(s) %v — %s", path, len(raws), titles, why)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func lsDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{"<unreadable: " + err.Error() + ">"}
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// TestRawDecisionTokens_CountsEveryListedSource: `logmind context --explain`
// divides the raw decision corpus by the timeline's size to report how much
// the timeline distills. rawDecisionTokens supplies the numerator, and it used
// to build its own file list — docs/decisions.md plus a glob of
// docs/decisions-branches/*.md — which silently omitted
// docs/decisions-archive.md. A repo that rotated under the retired
// `max_recent` cap therefore under-reported the ratio by exactly the history
// it had held longest. It routes through decisions.ListSources now, like every
// other read path.
func TestRawDecisionTokens_CountsEveryListedSource(t *testing.T) {
	withTempCwd(t, func(d string) {
		docsPath := filepath.Join(d, "docs")
		mustMkdir(t, filepath.Join(docsPath, "decisions-branches"))
		mustWrite(t, filepath.Join(docsPath, "decisions-branches", "main.md"),
			"## 2026-01-01 09:00 - branch decision\n\n**Reasoning:** b\n")

		before := rawDecisionTokens(d)
		if before == 0 {
			t.Fatal("fixture precondition: the branch file alone must estimate above zero")
		}

		// The file the hand-rolled list forgot.
		mustWrite(t, filepath.Join(docsPath, "decisions-archive.md"),
			"# Archive\n\n## 2019-01-01 09:00 - an archived decision with a good deal of body text in it\n\n"+
				"**Reasoning:** this is the history a rotated repo holds and the old glob never counted\n")

		after := rawDecisionTokens(d)
		if after <= before {
			t.Fatalf("docs/decisions-archive.md contributed nothing: %d tokens before, %d after", before, after)
		}
	})
}
