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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/gitcli"
	"github.com/thrillmade/logmind/internal/templates"
	"github.com/thrillmade/logmind/internal/testgit"
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
//
// And TRACKED, not just on disk. Every consumer of this repository gets it by
// cloning, so an untracked docs/decisions.md here would satisfy os.ReadFile in
// this working tree and be absent from every one of them — the exact failure
// mode this file exists to fence, dogfooded.
func TestRepoDecisionsPointer_IsInstalledHere(t *testing.T) {
	repoRoot := repoRootFromCaller(t)
	path := filepath.Join(repoRoot, "docs", "decisions.md")

	if !gitcli.IsTrackedFile(repoRoot, legacyPointerRel) {
		t.Errorf("logmind's own %s is not tracked by git.\n"+
			"Consumers get this repository by cloning, and a clone carries the index — not the "+
			"working tree — so an untracked sentinel is one that no consumer, and no v1.2.0 "+
			"binary run against a clone, ever sees.", legacyPointerRel)
	}

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

// TestInit_LegacyPointerIsCommitted_NotJustWritten is the assertion the rest
// of this file could not make.
//
// Every other test here drives `logmind init --no-git` (scaffoldDocs) and then
// asks pathExists. Both halves are wrong for this artifact. --no-git means the
// commit path never runs, and pathExists asks about the WORKING TREE — but the
// sentinel's entire job is to be there when a v1.2.0 binary runs in somebody
// else's checkout, and a file that was written and never committed is in
// nobody else's checkout. The suite was green for a whole review round while
// `logmind init` wrote docs/decisions.md and left it untracked: the commit list
// in runInit is a hand-kept restatement of what init writes, and it had lost
// the entry (base dev carried "docs/decisions.md"; this branch re-added the
// write and dropped the commit).
//
// So this runs init WITH git and asks the question at the two layers that
// matter: is the path in the index, and does it survive `git clone`. The clone
// is not belt-and-braces — it is the literal scenario, since v1.2.0 asks its
// question in a checkout it did not create.
func TestInit_LegacyPointerIsCommitted_NotJustWritten(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		// NOT --no-git. The commit is the thing under test.
		runQuiet(t, []string{"init"})

		if !gitcli.IsTrackedFile(d, legacyPointerRel) {
			t.Errorf("`logmind init` wrote %s but never committed it — it is not in the index.\n"+
				"An untracked sentinel reaches no clone, so a v1.2.0 binary reads the repository as "+
				"UNINITIALISED and rewrites .logmind/config.yml over its settings.\n"+
				"git status: %s", legacyPointerRel, gitcli.StatusPorcelain(d, legacyPointerRel))
		}

		// CONTROL — the probe discriminates. Without this, "tracked" above
		// could be an IsTrackedFile that answers true for anything.
		const untrackedControl = "docs/untracked-control.md"
		mustWrite(t, filepath.Join(d, untrackedControl), "written, never committed\n")
		if gitcli.IsTrackedFile(d, untrackedControl) {
			t.Fatal("control failed: IsTrackedFile answers true for a file that was never committed, " +
				"so the assertion above is not measuring tracking")
		}

		// The scenario itself: v1.2.0's question, asked of a fresh clone.
		clone := filepath.Join(t.TempDir(), "clone")
		testgit.CloneRepo(t, clone, "-q", d)

		if !v120AlreadyInitialised(clone) {
			t.Errorf("a CLONE of a repository this binary scaffolded reads as UNINITIALISED to logmind v1.2.0.\n"+
				"clone docs/ contains: %v", lsDir(t, filepath.Join(clone, "docs")))
		}

		// CONTROL — the clone carries committed content only, so the check
		// above is a statement about the index and not about the source
		// working tree being copied wholesale.
		if pathExists(filepath.Join(clone, untrackedControl)) {
			t.Fatal("control failed: the clone carries a file that was never committed, " +
				"so its contents prove nothing about what init tracked")
		}
	})
}

// TestInit_PreExistingUntrackedPointerIsCommitted drives the branch every
// other test in this file leaves cold: ensureLegacyPointer's "it was already
// there" early return.
//
// Every other case starts from a repo with no docs/decisions.md, so
// `pointerRel` is always the freshly-created return and a mutant that dropped
// the path on the already-present branch survives the whole file. What it
// breaks is not hypothetical — it is the fleet's common shape. A repository
// that predates §3.2 carries a REAL decision log at docs/decisions.md; if it
// is untracked when `logmind init` runs (a fresh checkout of a directory
// someone had been keeping locally, a repo whose .logmind/ was removed and
// re-inited), the file must still end up in the index. Otherwise the clone
// everyone else works from has no docs/decisions.md at all, and a v1.2.0
// binary run there reads the repository as UNINITIALISED and rewrites
// .logmind/config.yml over its settings — the exact failure this whole file
// exists to fence, arrived by the one route nothing tested.
//
// The fresh-install path is genuinely reachable in that state: the install
// sentinel is `.logmind/config.yml` AND docsScaffolded, so a repo with
// docs/decisions.md and no .logmind/ is not "already initialised".
func TestInit_PreExistingUntrackedPointerIsCommitted(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)

		// A pre-§3.2 main decision log, written and never committed.
		legacy := "# Decisions\n\n## 2019-05-05 11:11 - Chose Postgres over MySQL\n\n" +
			"**Reasoning:** pre-upgrade-rationale\n\n---\n"
		pointer := filepath.Join(d, "docs", "decisions.md")
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, pointer, legacy)
		if gitcli.IsTrackedFile(d, legacyPointerRel) {
			t.Fatal("fixture precondition: the pointer must start UNTRACKED")
		}

		// NOT --no-git: the commit is the thing under test.
		runQuiet(t, []string{"init"})

		if got := mustReadFile(t, pointer); got != legacy {
			t.Fatalf("`logmind init` rewrote a pre-§3.2 decision log.\nwant:\n%s\ngot:\n%s", legacy, got)
		}
		if !gitcli.IsTrackedFile(d, legacyPointerRel) {
			t.Errorf("`logmind init` left a pre-existing %s untracked.\n"+
				"\"Present\" is not the obligation — TRACKED is: an untracked sentinel reaches no clone, "+
				"so a v1.2.0 binary reads the repository as UNINITIALISED and rewrites .logmind/config.yml.\n"+
				"git status: %s", legacyPointerRel, gitcli.StatusPorcelain(d, legacyPointerRel))
		}

		// The scenario itself: v1.2.0's question, asked of a fresh clone,
		// which carries the index and not the working tree.
		clone := filepath.Join(t.TempDir(), "clone")
		testgit.CloneRepo(t, clone, "-q", d)
		if !v120AlreadyInitialised(clone) {
			t.Errorf("a CLONE of a repository whose docs/decisions.md was already on disk reads as "+
				"UNINITIALISED to logmind v1.2.0.\nclone docs/ contains: %v",
				lsDir(t, filepath.Join(clone, "docs")))
		}
		// And the log itself travelled, not just a path of that name.
		if got := mustReadFile(t, filepath.Join(clone, "docs", "decisions.md")); got != legacy {
			t.Errorf("the clone's docs/decisions.md is not the log that was on disk.\nwant:\n%s\ngot:\n%s",
				legacy, got)
		}
	})
}

// TestInit_PreExistingPointerIsNotReportedAsCreated: the receipt must match
// what happened. `✓ Created docs/decisions.md` used to be unconditional, and
// on the merge base that was harmless — the old sentinel tested for
// decisions.md itself, so the fresh path was unreachable with the file
// present. Widening the sentinel (docsScaffolded) made the branch reachable
// and the line false: content preserved, receipt lying.
func TestInit_PreExistingPointerIsNotReportedAsCreated(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"), "# Decisions\n\nheld locally\n")

		out := initCapturingOutput(t)
		if strings.Contains(out, "✓ Created docs/decisions.md") {
			t.Errorf("init reported creating a file that was already on disk.\n--- stdout ---\n%s", out)
		}
		if !strings.Contains(out, "docs/decisions.md already present") {
			t.Errorf("init said nothing about the file it staged.\n--- stdout ---\n%s", out)
		}
	})

	// CONTROL — the line IS printed when init really does create it, so the
	// assertion above is measuring the branch and not the absence of any
	// mention at all.
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		out := initCapturingOutput(t)
		if !strings.Contains(out, "✓ Created docs/decisions.md") {
			t.Errorf("control failed: a genuine fresh install does not report creating the sentinel, "+
				"so the negative assertion above proves nothing.\n--- stdout ---\n%s", out)
		}
	})
}

// initCapturingOutput runs `logmind init --no-git` against the current cwd
// and returns stdout. Sibling of refreshCapturingOutput below, for the
// fresh-install path's own receipt lines.
func initCapturingOutput(t *testing.T) string {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs([]string{"init", "--no-git"})
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	return out.String()
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
//
// pathExists is the RIGHT question here, unlike in the fresh-install case
// above, and the reason is worth stating: refresh commits nothing at all — not
// workflows, not .gitattributes, not this — so the file it restores is
// untracked by design and stays untracked until the user commits it. Which
// makes the restore only half a fix, and the other half is telling the user.
// That disclosure is asserted here, because a silent write leaves a file on
// disk that LOOKS like the fix and does not reach a single clone.
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

		out := refreshCapturingOutput(t) // second run — takes the refresh path

		if !v120AlreadyInitialised(d) {
			t.Fatal("`logmind init` in refresh mode left the repo unrecognisable to v1.2.0")
		}
		if !strings.Contains(out, "Restored docs/decisions.md") {
			t.Errorf("refresh restored the sentinel and said nothing about it.\n"+
				"It commits nothing, so the file is untracked and reaches no clone — the user is "+
				"the only one who can close that, and cannot act on a write they were not told about.\n"+
				"--- stdout ---\n%s", out)
		}
		if !strings.Contains(out, "Commit it") {
			t.Errorf("refresh named the restored file but not the action it needs.\n--- stdout ---\n%s", out)
		}

		// CONTROL — the line is conditional on an actual restore, not printed
		// unconditionally. A third run has nothing to restore and must be
		// silent about it, or the assertions above would pass on a constant.
		if again := refreshCapturingOutput(t); strings.Contains(again, "Restored docs/decisions.md") {
			t.Errorf("control failed: refresh reports a restore when the pointer was already present, "+
				"so the disclosure above is unconditional output and proves nothing.\n--- stdout ---\n%s", again)
		}
	})
}

// refreshCapturingOutput runs `logmind init --no-git` against an
// already-initialised cwd and returns everything it wrote to stdout. Same
// invocation as scaffoldDocs, which discards output; the refresh-mode
// disclosures are assertions, so they need to be readable.
func refreshCapturingOutput(t *testing.T) string {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs([]string{"init", "--no-git"})
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	if err := root.Execute(); err != nil {
		t.Fatalf("init (refresh): %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	return out.String()
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
