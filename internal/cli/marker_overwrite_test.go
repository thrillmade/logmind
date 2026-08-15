// marker_overwrite_test.go — logmind#297 and #299 at the level the user was
// harmed: "my file lost its content".
//
// Both defects wrote something partial over the whole of a user-owned file.
// #297: `self-update` handed a BLOCK BODY to a function whose first parameter
// is a WHOLE FILE, then wrote the fragment that came back over AGENTS.md.
// #299: `doctor` classified a workflow as markerless (SPEC §5.2 — the user's,
// MUST NOT be overwritten) using a first-line-only extractor, while
// `doctor --fix` re-classified it as versioned-and-stale using an any-line
// extractor and overwrote it.
//
// The assertions here are on OUTCOMES — the bytes of a realistic file after
// the real command ran — not on which arguments a helper received. An
// argument assertion passes its own mutation and still goes green the day the
// same fragment gets written by a different route.
package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/inserter"
	"github.com/thrillmade/logmind/internal/templates"
)

// runSelfUpdateCapture runs `self-update` in the current cwd and returns
// (stdout, stderr).
func runSelfUpdateCapture(t *testing.T) (string, string) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs([]string{"self-update"})
	var out, errOut strings.Builder
	root.SetOut(&out)
	root.SetErr(&errOut)
	if err := root.Execute(); err != nil {
		t.Fatalf("self-update: %v\nstderr=%s", err, errOut.String())
	}
	return out.String(), errOut.String()
}

// TestSelfUpdate_PreservesAgentsMDOutsideBlock is #297's user symptom.
//
// An AGENTS.md with a STALE logmind block and repository prose both above and
// below it. `self-update` must move the block forward and leave every other
// byte alone. When the fragment-write ships, this file becomes the block body
// alone and every sentinel below disappears.
func TestSelfUpdate_PreservesAgentsMDOutsideBlock(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)

		const above = "# AGENTS.md\n\nThis is the canonical instruction file for AI coding agents.\n\n"
		const below = "\n\n## Project Overview\n\n**logmind** is a decision-logging CLI (Go).\n\n" +
			"## Development Commands\n\n```bash\ngo build ./cmd/logmind\n```\n\n" +
			"## Release infrastructure\n\nCross-repo writes are signed by the steward App.\n"

		// A stale-but-refreshable block: the marker orders BEFORE the bundled
		// one and carries the slim flavour, so the refresh path engages
		// (rather than refusing, which would prove nothing about writing).
		staleBlock := "\n<!-- logmind-block-version: v1-pointer -->\nSTALE BLOCK BODY\n"
		original := above + "<!-- logmind-start -->" + staleBlock + "<!-- logmind-end -->" + below
		writeRel(t, "AGENTS.md", original, 0o644)

		runSelfUpdateCapture(t)

		got := readRel(t, "AGENTS.md")

		// SURVIVAL FIRST, so a failure names the harm the user reported
		// ("my AGENTS.md lost its content") rather than a symptom of it.
		for _, sentinel := range []string{
			"This is the canonical instruction file for AI coding agents.",
			"## Project Overview",
			"**logmind** is a decision-logging CLI (Go).",
			"go build ./cmd/logmind",
			"## Release infrastructure",
			"Cross-repo writes are signed by the steward App.",
		} {
			if !strings.Contains(got, sentinel) {
				t.Errorf("self-update destroyed content outside the marker block: %q is gone", sentinel)
			}
		}

		freshBody, ok := inserter.ExtractMarkerBlock(templates.AgentsSlimTemplate())
		if !ok {
			t.Fatal("bundled slim template carries no marker block")
		}
		if want := above + "<!-- logmind-start -->" + freshBody + "<!-- logmind-end -->" + below; got != want {
			t.Errorf("self-update did not rewrite the block in place:\n got: %q\nwant: %q", got, want)
		}

		// Vacuity control LAST: if the block never moved forward, nothing was
		// written and the survival assertions above proved nothing.
		if strings.Contains(got, "STALE BLOCK BODY") {
			t.Error("self-update did not refresh the stale block; the survival " +
				"assertions above would prove nothing")
		}
	})
}

// TestSelfUpdate_LeavesMarkerlessAgentsMDContentIntact — SPEC §5.2 for the
// self-update surface. An AGENTS.md the user wrote with no logmind block at
// all gets the block ADDED (an insert preserves their content); what must
// never happen is their prose being replaced.
func TestSelfUpdate_LeavesMarkerlessAgentsMDContentIntact(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)

		original := "# My own AGENTS.md\n\nI wrote every line of this myself.\n\n" +
			"## House rules\n\nUSER_SENTINEL_ONE\n\n## More\n\nUSER_SENTINEL_TWO\n"
		writeRel(t, "AGENTS.md", original, 0o644)

		runSelfUpdateCapture(t)

		got := readRel(t, "AGENTS.md")
		for _, sentinel := range []string{
			"I wrote every line of this myself.",
			"## House rules",
			"USER_SENTINEL_ONE",
			"## More",
			"USER_SENTINEL_TWO",
		} {
			if !strings.Contains(got, sentinel) {
				t.Errorf("self-update destroyed user content: %q is gone", sentinel)
			}
		}
	})
}

// plantDisplacedMarkerWorkflow writes the #299 reproduction: a workflow whose
// `# logmind-template-version:` marker sits on line 2, under a header the
// user added. Returns the rel path and the original bytes.
func plantDisplacedMarkerWorkflow(t *testing.T) (string, string) {
	t.Helper()
	rel := filepath.Join(".github", "workflows", "check-decisions.yml")
	body := "# my org header — do not remove\n" +
		"# logmind-template-version: v1\n" +
		"name: check-decisions\n" +
		"jobs: {}\n" +
		"USER_SENTINEL_LINE\n"
	writeRel(t, rel, body, 0o644)
	return rel, body
}

// TestDoctorFix_LeavesDisplacedMarkerWorkflowAlone is #299's user symptom,
// measured exactly as the issue measured it: doctor prints "markerless", and
// then --fix must NOT overwrite the file it just called the user's.
func TestDoctorFix_LeavesDisplacedMarkerWorkflowAlone(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		rel, before := plantDisplacedMarkerWorkflow(t)

		_, stderr := runDoctorFixCmd(t)

		if after := readRel(t, rel); after != before {
			t.Errorf("doctor --fix overwrote a file it classified as the user's:\n got: %q\nwant: %q",
				after, before)
		}
		// The refusal has to be visible and actionable: name the file, the
		// marker, the line it is on, and both ways out.
		mustContain(t, stderr, "check-decisions.yml")
		mustContain(t, stderr, "left unchanged")
		mustContain(t, stderr, "line 2")
		mustContain(t, stderr, "not line 1")
	})
}

// TestDoctorFix_LeavesUnmarkedWorkflowAlone — the plain SPEC §5.2 case: a
// workflow with no logmind marker anywhere is the user's, and --fix must both
// leave it alone AND say that it did.
func TestDoctorFix_LeavesUnmarkedWorkflowAlone(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		before := "name: my own check-decisions\njobs: {}\nUSER_SENTINEL_LINE\n"
		writeRel(t, rel, before, 0o644)

		_, stderr := runDoctorFixCmd(t)

		if after := readRel(t, rel); after != before {
			t.Errorf("doctor --fix overwrote a markerless (user-owned) workflow:\n got: %q\nwant: %q",
				after, before)
		}
		mustContain(t, stderr, "check-decisions.yml")
		mustContain(t, stderr, "carries no `# logmind-template-version:` marker")
		mustContain(t, stderr, "treats it as yours")
	})
}

// TestDoctorFix_UnmarkedWorkflowMessageNamesAWorkingRemedy is the #306 HIGH:
// a panel found the declineUnmarked refusal told the user to make
// `# logmind-template-version: <bundled>` the file's first line — but
// installWorkflowTemplates only rewrites a file whose marker DIFFERS from
// bundled, so pasting the CURRENT marker in by hand makes the file match on
// the very next read and never refresh again, while doctor reports it
// "current" forever. The remedy must be one that actually works
// (delete-and-regenerate), and must NOT still offer the dead-end one.
func TestDoctorFix_UnmarkedWorkflowMessageNamesAWorkingRemedy(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		writeRel(t, rel, "jobs: {}\nUSER_SENTINEL\n", 0o644)

		_, stderr := runDoctorFixCmd(t)

		relSlash := filepath.ToSlash(rel)
		mustContain(t, stderr, "delete "+relSlash)
		mustContain(t, stderr, "logmind doctor --fix")
		if strings.Contains(stderr, "make `# logmind-template-version:") {
			t.Errorf("message still tells the user to paste the marker in by hand — that freezes "+
				"the file at \"current\" without ever actually matching the bundled template:\n%s", stderr)
		}
	})
}

// TestDoctorFix_DeleteAndRerunRegeneratesUnmarkedWorkflow proves the remedy
// TestDoctorFix_UnmarkedWorkflowMessageNamesAWorkingRemedy asserts the text
// of actually works end to end: delete the refused file, re-run --fix, and
// the file comes back as a real, current, working template — not the same
// inert `jobs: {}` body the user started with.
func TestDoctorFix_DeleteAndRerunRegeneratesUnmarkedWorkflow(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		writeRel(t, rel, "jobs: {}\nUSER_SENTINEL\n", 0o644)
		runDoctorFixCmd(t) // first pass: refused, file unchanged

		if err := os.Remove(rel); err != nil {
			t.Fatalf("remove refused file: %v", err)
		}

		runDoctorFixCmd(t) // second pass: file is gone -> CREATE branch, always writes

		got := readRel(t, rel)
		wantVersion := inserter.ExtractTemplateMarker(
			templates.Workflow("check-decisions.yml.template")).Version
		gotMarker := inserter.ExtractTemplateMarker(got)
		if !gotMarker.Writable() || gotMarker.Version != wantVersion {
			t.Errorf("delete-and-rerun did not regenerate the workflow: got %+v; want version %q",
				gotMarker, wantVersion)
		}
		if strings.Contains(got, "USER_SENTINEL") || strings.Contains(got, "jobs: {}") {
			t.Error("file still carries the old inert body — the remedy did not actually regenerate it")
		}
	})
}

// TestDoctorFix_ResidualNoteNamesOwnershipNotPathOrHook is the #306 LOW: the
// residual note printed after --fix used one static sentence for every
// still-drifted row — "(PATH/version, or a hand-written hook)" — which is
// simply wrong for an unmarked WORKFLOW: it is neither PATH/version drift
// (that's the on-PATH binary or a hook trailing the running version) nor a
// hand-written git HOOK (a workflow has no such concept; "foreign" is
// reserved for that). Before #306's ownership-first write refusal, this row
// never reached the residual list at all — a markerless file was silently
// overwritten into "current" instead, which is the very bug this branch
// fixes — so the wrong attribution was invisible until the write refusal
// made the row reachable.
func TestDoctorFix_ResidualNoteNamesOwnershipNotPathOrHook(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		writeRel(t, rel, "jobs: {}\nUSER_SENTINEL\n", 0o644)

		_, stderr := runDoctorFixCmd(t)

		mustContain(t, stderr, "check-decisions.yml")
		mustContain(t, stderr, "no logmind marker")
		if strings.Contains(stderr, `"check-decisions.yml" still drifted — not auto-fixable by `+"`doctor --fix`"+` (PATH/version, or a hand-written hook)`) {
			t.Errorf("residual note still blames PATH/version or a hand-written hook for a plain "+
				"unmarked workflow, neither of which happened here:\n%s", stderr)
		}
	})
}

// TestDoctor_ReportsDisplacedMarkerRatherThanBareMarkerless — the reader's
// half of #299. Reporting only "markerless" would hide that a marker IS
// present, which is the one fact the user needs to move the file into either
// camp deliberately.
func TestDoctor_ReportsDisplacedMarkerRatherThanBareMarkerless(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		plantDisplacedMarkerWorkflow(t)

		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline"})
		var out, errOut strings.Builder
		root.SetOut(&out)
		root.SetErr(&errOut)
		_ = root.Execute() // doctor exits non-zero on drift; the body is what matters

		body := out.String()
		mustContain(t, body, "check-decisions.yml")
		mustContain(t, body, "on line 2, not line 1")
	})
}

// TestDoctorFix_StillRefreshesAWorkflowItOwns is the CONTROL on every refusal
// above: the strict first-line rule must not have turned --fix into a no-op.
// A marker on line 1 is still ours, and still gets refreshed.
func TestDoctorFix_StillRefreshesAWorkflowItOwns(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		writeRel(t, rel, "# logmind-template-version: v1\n# ancient\n", 0o644)

		runDoctorFixCmd(t)

		want := inserter.ExtractTemplateMarker(
			templates.Workflow("check-decisions.yml.template")).Version
		got := inserter.ExtractTemplateMarker(readRel(t, rel))
		if !got.Writable() || got.Version != want {
			t.Errorf("a workflow logmind owns was not refreshed: got %+v; want version %q", got, want)
		}
	})
}

// TestDoctorFix_RefusesDanglingWorkflowSymlink is the #306 live escape,
// measured through the real command. os.ReadFile FOLLOWS a symlink, so a
// DANGLING one at .github/workflows/check-decisions.yml returns fs.ErrNotExist
// and the file reads as ABSENT — the same ENOENT-as-absent shape already fixed
// in inserter.EnsureDependabot. installWorkflowTemplates then took its CREATE
// branch and a bare os.WriteFile followed the link, landing the rendered
// workflow wherever it pointed (a panel measured 6419 bytes written OUTSIDE
// the repository) while --fix reported `workflows=1` as if it had installed
// one. The refusal must be loud, and nothing may appear at the link target.
func TestDoctorFix_RefusesDanglingWorkflowSymlink(t *testing.T) {
	skipWithoutSymlinks(t)
	outside := t.TempDir()
	escapeTarget := filepath.Join(outside, "escaped-workflow.yml")

	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(escapeTarget, rel); err != nil {
			t.Fatalf("plant dangling symlink: %v", err)
		}

		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--fix", "--offline"})
		var out, errOut strings.Builder
		root.SetOut(&out)
		root.SetErr(&errOut)
		err := root.Execute()

		// THE HARM FIRST: nothing may exist outside the repository.
		if _, statErr := os.Lstat(escapeTarget); statErr == nil {
			body, _ := os.ReadFile(escapeTarget)
			t.Fatalf("doctor --fix wrote %d bytes OUTSIDE the repo, through a dangling symlink, to %s",
				len(body), escapeTarget)
		}
		// The link itself is untouched — refusing means leaving both the link
		// and whatever it names exactly as found.
		fi, lerr := os.Lstat(rel)
		if lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("the planted symlink at %s was replaced rather than refused (lstat err=%v)", rel, lerr)
		}
		// And the refusal is REPORTED, not silent — a silent no-op here reads
		// to the user exactly like a successful install.
		if err == nil {
			t.Error("doctor --fix exited 0 on a workflow it could not write; the refusal was silent")
		}
		mustContain(t, errOut.String(), "symlink")
	})
}

// TestDoctorFix_RefusesSymlinkedExistingWorkflow is the same escape through
// installWorkflowTemplates' OTHER branch. A NON-dangling symlink reads fine
// through os.ReadFile, so the file looks present and its (stale, line-1,
// therefore logmind-owned) marker routes control to the REFRESH write — where
// a bare os.WriteFile rewrites whatever the link points at. The dangling case
// above never reaches this branch, so without this test the refresh write can
// be reverted to a raw primitive and every behavioural assertion stays green.
func TestDoctorFix_RefusesSymlinkedExistingWorkflow(t *testing.T) {
	skipWithoutSymlinks(t)
	outside := t.TempDir()
	realTarget := filepath.Join(outside, "someone-elses-workflow.yml")
	// Marker on line 1 (so it is "ours" and writable) but an old version (so
	// the refresh branch engages), with user content underneath to detect a
	// rewrite.
	const userContent = "# logmind-template-version: v0-ANCIENT\nname: not ours\nUSER_SENTINEL_OUTSIDE\n"
	if err := os.WriteFile(realTarget, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}

	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realTarget, rel); err != nil {
			t.Fatalf("plant symlink: %v", err)
		}

		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--fix", "--offline"})
		var out, errOut strings.Builder
		root.SetOut(&out)
		root.SetErr(&errOut)
		err := root.Execute()

		got, readErr := os.ReadFile(realTarget)
		if readErr != nil {
			t.Fatalf("the file outside the repo vanished: %v", readErr)
		}
		if string(got) != userContent {
			t.Fatalf("doctor --fix wrote through the symlink and rewrote a file outside the repo:\n got: %q\nwant: %q",
				string(got), userContent)
		}
		if err == nil {
			t.Error("doctor --fix exited 0 on a workflow it could not write; the refusal was silent")
		}
		mustContain(t, errOut.String(), "symlink")

		// VACUITY CONTROL: the refresh branch has to actually be the branch
		// under test. A marker logmind does NOT own would be refused earlier,
		// by the ownership gate, and would prove nothing about the write.
		if m := inserter.ExtractTemplateMarker(userContent); !m.Writable() {
			t.Fatalf("fixture marker is not writable (%+v) — control never reached the refresh write", m)
		}
	})
}

// TestResidualCause_NamesADistinctCausePerDriftValue is the #306 HIGH on the
// residual note. residualProbes emits four drift values and they mean four
// different things; the note must not attribute one row's cause to another.
//
// The specific regression: probePathResolution filed BOTH "cannot exec
// --version" and "no version parsed" as drift="markerless", so `doctor --fix`
// told a user whose PATH binary was merely unreadable that logmind "treats it
// as yours and leaves it alone" — SPEC §5.2's OWNERSHIP verdict, applied to a
// binary that has no ownership marker concept at all.
func TestResidualCause_NamesADistinctCausePerDriftValue(t *testing.T) {
	drifts := []string{"stale", "foreign", "markerless", "unreadable"}
	cause := map[string]string{}
	byText := map[string]string{}
	for _, d := range drifts {
		c := residualCause(d)
		cause[d] = c
		if prev, dup := byText[c]; dup {
			t.Errorf("drift %q and drift %q report the IDENTICAL cause %q; one of them is wrong "+
				"about what happened", prev, d, c)
		}
		byText[c] = d
	}

	// The ownership sentence belongs to "markerless" and to nothing else.
	const ownership = "treats it as yours"
	if !strings.Contains(cause["markerless"], ownership) {
		t.Errorf("markerless no longer states the SPEC §5.2 ownership verdict: %q", cause["markerless"])
	}
	for _, d := range []string{"stale", "foreign", "unreadable"} {
		if strings.Contains(cause[d], ownership) {
			t.Errorf("drift %q claims SPEC §5.2 user ownership, which is only true of a markerless "+
				"artifact: %q", d, cause[d])
		}
	}

	// The citation itself, since one of these strings is user-facing stderr:
	// both sentences live at SPEC.md §5.2, never in §1.1.
	if strings.Contains(cause["markerless"], "§1.1") {
		t.Errorf("markerless cites SPEC §1.1; the marker-ownership rule is §5.2's: %q", cause["markerless"])
	}
	mustContain(t, cause["markerless"], "§5.2")

	// And "unreadable" says what actually happened.
	mustContain(t, cause["unreadable"], "on PATH")
	mustContain(t, cause["unreadable"], "--version")
}

// TestDoctorFix_UnreadablePathBinaryReportsItsOwnCause walks the whole route —
// a real binary on PATH whose --version output carries no version, through the
// real `doctor --fix` — because residualCause being right is only useful if
// the drift value reaching it is right too. Before #306 this row arrived as
// "markerless" and got the user-ownership sentence.
func TestDoctorFix_UnreadablePathBinaryReportsItsOwnCause(t *testing.T) {
	skipWithoutSymlinks(t) // shell-script fixture; same non-Windows condition
	fakeDir := t.TempDir()
	fake := filepath.Join(fakeDir, "logmind")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho some unrelated output\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// PREPEND rather than replace: doctor --fix shells out to git, which still
	// has to resolve.
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		_, stderr := runDoctorFixCmd(t)

		mustContain(t, stderr, "logmind on PATH")
		mustContain(t, stderr, "--version")
		if strings.Contains(stderr, `"logmind on PATH" still drifted — it carries no logmind marker`) {
			t.Errorf("the PATH binary is reported as a markerless user-owned artifact; it is neither:\n%s", stderr)
		}
	})
}

// skipWithoutSymlinks skips on platforms where an unprivileged process cannot
// create one (Windows), mirroring internal/inserter's helper of the same
// purpose.
func skipWithoutSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevation on Windows")
	}
}

// ---------------------------------------------------------------------------
// The structural half of the #297 fix: nothing may write a managed path except
// through the one primitive that owns it.
// ---------------------------------------------------------------------------

// TestWriteSurfaces_UseNoRawWritePrimitive is the rebuilt #297 second-writer
// guard, and it is deliberately NOT a scan for the AGENTS.md PATH.
//
// SPEC §5.2: "Exactly one automation owns any generated or copied path. Two
// refreshers MUST NOT write the same path." The deleted #297 loop was a second
// refresher of AGENTS.md, and being unreachable is precisely why its
// wrong-argument write survived untested — so the absence of a second writer is
// the thing worth pinning, not the correctness of one that should not exist.
//
// WHY THE PATH IS THE WRONG THING TO SCAN FOR. Two earlier versions of this
// guard matched on the target expression: first two exact banned substrings in
// one file, then a regexp for `WriteFile(agentsPath` / `WriteFile(filepath.
// Join(…"AGENTS.md"`. A review panel walked through both. The set of ways to
// spell a path is unbounded, and every one of these compiles and passed:
//
//	os.WriteFile(entry.Path, []byte(entry.NewBody), 0o644)  // the literal #297 loop
//	os.WriteFile(repoRoot+"/AGENTS.md", …)                  // concatenated literal
//	p := filepath.Join(root, "AGENTS.md"); os.WriteFile(p, …) // renamed variable
//	writeAgentsFile(agentsPath, …)                          // one-line wrapper
//	…and anything at all under cmd/, which the old scan root never visited.
//
// The first of those five is the worst case for a path scan: it never names
// AGENTS.md at all — it takes the path back OUT of the inserter API. And it is
// invisible to a behavioural test too, which is why one is not the answer
// here: after EnsureAgentsMD refreshes the block, FindOutdatedMarkerBlocks
// reports nothing, so the restored loop is a no-op and every outcome assertion
// above it stays green. Measured, not assumed — with the loop restored,
// TestSelfUpdate_PreservesAgentsMDOutsideBlock passes.
//
// WHAT IS SCANNED INSTEAD. The write PRIMITIVE, which unlike a path is a
// closed, enumerable set. Every one of those five evasions must ultimately
// call one, so banning the primitive catches all five however the path is
// spelled — and it generalises: the same guard is what would have caught the
// live symlink escape at installWorkflowTemplates, where a bare os.WriteFile
// followed a dangling symlink out of the repo (see
// TestDoctorFix_RefusesDanglingWorkflowSymlink).
//
// KNOWN BOUNDARY, stated rather than implied: a second writer that took the
// path from inserter.OutdatedMarkerEntry.Path AND routed it through
// atomicio.WriteFile would satisfy this guard. Closing that needs either
// interprocedural dataflow or the removal of the Path field, whose only
// consumer is runAgentsUpdate in internal/cli/agents.go.
func TestWriteSurfaces_UseNoRawWritePrimitive(t *testing.T) {
	root := moduleRoot(t)

	// SCOPE CONTROL FIRST. Evasion five was purely a scope failure — a
	// perfectly good scanner pointed at internal/ alone, so a second writer
	// under cmd/ was never read. Assert the walk actually reaches specific
	// NAMED files before trusting a clean result, one per directory the guard
	// claims to cover. Naming the files rather than counting them matters:
	// the cheapest way to make this guard green is to drop a directory from
	// writeSurfaceDirs, and a non-empty count would not notice.
	scanned := map[string]bool{}
	for _, dir := range writeSurfaceDirs {
		files, err := goSourceFiles(filepath.Join(root, dir))
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
		for _, f := range files {
			rel, relErr := filepath.Rel(root, f)
			if relErr != nil {
				t.Fatal(relErr)
			}
			scanned[filepath.ToSlash(rel)] = true
		}
	}
	for _, sentinel := range []string{
		"cmd/logmind/main.go",             // the binary's own package — evasion five's home
		"internal/cli/self_update.go",     // where the #297 second writer lived
		"internal/inserter/inserter.go",   // the owner of the AGENTS.md write primitive
		"internal/inserter/dependabot.go", // a sibling installer found by a later sweep
	} {
		if !scanned[sentinel] {
			t.Fatalf("scope control: %s was never read by the scan — the guard's coverage has "+
				"shrunk, so a clean result below proves nothing about it", sentinel)
		}
	}

	// DETECTION CONTROL. One synthetic fixture per evasion the panel found,
	// including one in a nested subdirectory (proving the walk recurses) and
	// one negative fixture that must NOT be flagged (proving the scanner
	// discriminates rather than flagging every file it reads).
	t.Run("control_detects_every_known_evasion", func(t *testing.T) {
		dir := t.TempDir()
		type fixture struct {
			rel, body string
			wantHit   bool
		}
		fixtures := []fixture{
			{"evasion1_inserter_supplied_path.go", `package evil

import (
	"os"

	"github.com/thrillmade/logmind/internal/inserter"
)

func f(cwd string) {
	entries, _, _ := inserter.FindOutdatedMarkerBlocks(cwd)
	for _, entry := range entries {
		_ = os.WriteFile(entry.Path, []byte(entry.NewBody), 0o644)
	}
}
`, true},
			{"evasion2_concatenated_literal.go", `package evil

import "os"

func g(repoRoot string) { _ = os.WriteFile(repoRoot+"/AGENTS.md", nil, 0o644) }
`, true},
			{"evasion3_renamed_variable.go", `package evil

import (
	"os"
	"path/filepath"
)

func h(repoRoot string) {
	p := filepath.Join(repoRoot, "AGENTS.md")
	_ = os.WriteFile(p, nil, 0o644)
}
`, true},
			{"evasion4_helper_wrapper.go", `package evil

import "os"

func writeAgentsFile(path string, data []byte) error { return os.WriteFile(path, data, 0o644) }
`, true},
			{"nested/evasion5_under_a_subdirectory.go", `package nested

import (
	"os"
	"path/filepath"
)

func k(repoRoot string) {
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	_ = os.WriteFile(agentsPath, nil, 0o644)
}
`, true},
			{"other_primitives.go", `package evil

import "os"

func m(path string) {
	f, _ := os.Create(path)
	_ = f
	g, _ := os.OpenFile(path, os.O_WRONLY, 0o644)
	_ = g
}
`, true},
			// NEGATIVE control: the sanctioned route. A guard that flags this
			// too would "pass" every fixture above while measuring nothing.
			{"sanctioned_route.go", `package evil

import (
	"path/filepath"

	"github.com/thrillmade/logmind/internal/atomicio"
)

func n(repoRoot string) {
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	_ = atomicio.WriteFile(agentsPath, nil, 0o644)
}
`, false},
			// NEGATIVE control: the primitive named in a comment and a string,
			// never called. An AST scan must not confuse mention with use — a
			// regexp over the bytes would flag this file and, by flagging the
			// codebase's own explanatory comments, force the guard to be
			// weakened until it stopped working.
			{"mentions_only.go", `package evil

// This function deliberately does not call os.WriteFile.
func p() string { return "os.WriteFile" }
`, false},
		}
		var wantHits []string
		for _, fx := range fixtures {
			full := filepath.Join(dir, fx.rel)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(fx.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if fx.wantHit {
				wantHits = append(wantHits, fx.rel)
			}
		}

		got, err := findRawWritePrimitives(dir, []string{"."})
		if err != nil {
			t.Fatalf("scan fixtures: %v", err)
		}
		hitFiles := map[string]bool{}
		for _, v := range got {
			hitFiles[v.File] = true
		}
		for _, want := range wantHits {
			if !hitFiles[filepath.ToSlash(want)] {
				t.Errorf("control: %s was NOT flagged — this evasion would ship undetected", want)
			}
		}
		for _, fx := range fixtures {
			if !fx.wantHit && hitFiles[filepath.ToSlash(fx.rel)] {
				t.Errorf("control: %s WAS flagged — the scanner does not discriminate, so a "+
					"clean tree result would only mean the allowlist is doing the work", fx.rel)
			}
		}
	})

	// THE REAL TREE.
	violations, err := findRawWritePrimitives(root, writeSurfaceDirs)
	if err != nil {
		t.Fatalf("scan the write surfaces: %v", err)
	}
	exercised := map[string]bool{}
	for _, v := range violations {
		if _, ok := rawWriteAllowlist[v.File]; ok {
			exercised[v.File] = true
			continue
		}
		t.Errorf("%s:%d calls %s directly — route it through atomicio.WriteFile (or an "+
			"inserter primitive), which refuses a symlink at the destination and writes "+
			"whole-file-or-nothing. If this path is one logmind already owns elsewhere, the "+
			"call is a SECOND writer of it, which SPEC §5.2 forbids outright.",
			v.File, v.Line, v.Call)
	}

	// STALENESS. An allowlist entry that no longer names a real violation is
	// either a fix nobody deleted the exemption for, or a file that moved out
	// from under it — and in the second case the exemption now covers nothing
	// while the moved file goes unguarded. Either way it has to be noticed, so
	// an unused entry fails rather than lingers.
	for path, reason := range rawWriteAllowlist {
		if !exercised[path] {
			t.Errorf("allowlist entry %q (%s) matched no raw write — delete the entry if the "+
				"call is gone, or update it if the file moved; a stale exemption widens this "+
				"guard without anyone deciding to", path, reason)
		}
	}
}

// writeSurfaceDirs are the trees this guard covers: the command layer, the
// artifact-installer package, and the binary's own main package. cmd/ is here
// because its absence from the previous scan root was one of the five ways a
// second writer got in.
var writeSurfaceDirs = []string{
	filepath.Join("internal", "cli"),
	filepath.Join("internal", "inserter"),
	"cmd",
}

// rawWriteAllowlist maps a REPO-RELATIVE PATH to the reason its raw write
// primitive is permitted. Paths, not base names: a base-name key silently
// exempts any future file that happens to share the name, which is the same
// "matches more than it means" failure that let five second writers past the
// previous version of this guard.
//
// Every entry is a standing finding, not an endorsement — and every entry is
// checked for staleness below, so one whose violation is fixed, or whose file
// moves, fails this test rather than quietly widening it.
var rawWriteAllowlist = map[string]string{
	// Opens a lock file for flock(2). No content is ever written through the
	// descriptor, so there is no torn-write or symlink-follow exposure to fix.
	"internal/cli/filelock_unix.go": "lock-file open, not a content write",

	// FINDING, not a fix: os.WriteFile on a CI workflow pin path. Same
	// ENOENT-as-absent / follow-the-symlink exposure as the two writes fixed
	// in init.go on this branch. Owned by another in-flight lane (PR #313),
	// so it is recorded here rather than routed around.
	"internal/cli/agents.go": "workflow-pin write; raw primitive owned by PR #313's lane",

	// FINDING, same as above: two raw hook writes, PR #313's lane.
	"internal/cli/install_hook.go": "git-hook write; raw primitive owned by PR #313's lane",
}

// rawWriteViolation is one call to a banned primitive.
type rawWriteViolation struct {
	File string // repo-relative, slash-separated
	Line int
	Call string // e.g. "os.WriteFile"
}

// bannedWritePrimitives are the stdlib calls that write (or truncate) a file
// at a path directly. Unlike a path expression, this set is CLOSED — which is
// the whole reason the guard scans for it instead of for "AGENTS.md".
//
// os.Rename is absent deliberately: it is the second half of atomicio's own
// temp-file-plus-rename, so banning it would ban the sanctioned route.
var bannedWritePrimitives = map[string]map[string]bool{
	"os":     {"WriteFile": true, "Create": true, "OpenFile": true, "Truncate": true},
	"ioutil": {"WriteFile": true},
}

// findRawWritePrimitives parses every non-test .go file under root and returns
// each CALL to a banned write primitive. Files whose base name is in allow are
// skipped.
//
// AST, not regexp, and that is load-bearing rather than fastidious: this
// codebase's comments discuss `os.WriteFile` constantly — describing exactly
// the defect this guard pins — so a byte-level scan would flag its own
// explanations, and the only way to make it green would be to weaken it.
func findRawWritePrimitives(root string, dirs []string) ([]rawWriteViolation, error) {
	var files []string
	for _, dir := range dirs {
		found, err := goSourceFiles(filepath.Join(root, dir))
		if err != nil {
			return nil, err
		}
		files = append(files, found...)
	}
	var out []rawWriteViolation
	fset := token.NewFileSet()
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if bannedWritePrimitives[pkg.Name][sel.Sel.Name] {
				out = append(out, rawWriteViolation{
					File: rel,
					Line: fset.Position(sel.Pos()).Line,
					Call: pkg.Name + "." + sel.Sel.Name,
				})
			}
			return true
		})
	}
	return out, nil
}

// goSourceFiles lists every non-test .go file under root, recursively.
func goSourceFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return out, err
}

// moduleRoot walks up from the test's working directory to the directory
// holding go.mod. Scanning from the module root — rather than from a relative
// "..", which is how cmd/ came to be excluded — is what makes the guard's
// coverage a property of the repository instead of of the test's location.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test's working directory")
		}
		dir = parent
	}
}
