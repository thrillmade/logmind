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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/doctor"
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

		// AND IT SAYS ONLY THAT (#306 HIGH 6). The residual note printed
		// LATER IN THE SAME RUN used to contradict the refusal above outright:
		// one file, one --fix, and both "its marker is on line 2, not line 1,
		// so logmind cannot tell whether the file is yours or its own" and "it
		// carries no logmind marker … so SPEC §5.2 treats it as yours". SPEC.md
		// §5.2 hands the file to the user only when it carries "no marker at
		// all", which is exactly what this file does not do. Asserting the
		// first note alone left the second one unpinned, which is how it
		// survived.
		if strings.Contains(stderr, "carries no logmind marker") {
			t.Errorf("doctor --fix says this file carries no logmind marker, having just reported "+
				"the marker and the line it sits on:\n%s", stderr)
		}
		if strings.Contains(stderr, "treats it as yours") {
			t.Errorf("doctor --fix claims SPEC §5.2 user ownership over a file carrying a logmind "+
				"marker; §5.2 grants that only to an artifact carrying no marker at all:\n%s", stderr)
		}
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
	// "markerless+displaced" is a fifth CASE over four drift values: one
	// verdict, two different facts, and only one of them is an ownership
	// claim (#306 HIGH 6).
	cases := map[string]doctor.WorkflowStatus{
		"stale":      {Drift: "stale"},
		"foreign":    {Drift: "foreign"},
		"markerless": {Drift: "markerless"},
		"displaced":  {Drift: "markerless", Displaced: true},
		"unreadable": {Drift: "unreadable"},
	}
	cause := map[string]string{}
	byText := map[string]string{}
	for _, d := range []string{"stale", "foreign", "markerless", "displaced", "unreadable"} {
		c := residualCause(cases[d])
		cause[d] = c
		if prev, dup := byText[c]; dup {
			t.Errorf("case %q and case %q report the IDENTICAL cause %q; one of them is wrong "+
				"about what happened", prev, d, c)
		}
		byText[c] = d
	}

	// SPEC.md §5.2 gives the artifact to the user only when it carries "no
	// marker at all". A marker on line 2 is not that, so the displaced case
	// must NOT claim user ownership — saying it does contradicts, word for
	// word, the declineDisplaced note printed for the same file in the same
	// `doctor --fix` run.
	if strings.Contains(cause["displaced"], "treats it as yours") {
		t.Errorf("a DISPLACED marker is reported under SPEC §5.2's no-marker-at-all ownership rule, "+
			"which does not cover it: %q", cause["displaced"])
	}
	if strings.Contains(cause["displaced"], "carries no logmind marker") {
		t.Errorf("the displaced note says the file carries no marker; it carries one, on the wrong "+
			"line, and that difference is the whole reason the file was refused: %q", cause["displaced"])
	}
	// And it says what IS true of it, in the same terms the write-side refusal
	// uses, so the two notes read as one account rather than two.
	mustContain(t, cause["displaced"], "not on line 1")

	// The ownership sentence belongs to "markerless" and to nothing else.
	const ownership = "treats it as yours"
	if !strings.Contains(cause["markerless"], ownership) {
		t.Errorf("markerless no longer states the SPEC §5.2 ownership verdict: %q", cause["markerless"])
	}
	for _, d := range []string{"stale", "foreign", "unreadable", "displaced"} {
		if strings.Contains(cause[d], ownership) {
			t.Errorf("case %q claims SPEC §5.2 user ownership, which is only true of an artifact "+
				"carrying no marker at all: %q", d, cause[d])
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

// TestInit_RefusedWorkflowDoesNotCostTheOtherThree is the #306 HIGH 4 user
// symptom, measured on the artifacts that reached the disk.
//
// installWorkflowTemplates used to `return nil, nil, nil, err` from inside its
// loop. check-decisions.yml sorts FIRST of the four bundled templates, so a
// symlink there — a path the write correctly refuses — aborted the loop before
// check-doc-links, logmind-self-update and regen-timeline were ever attempted.
// `init` then exited 0, printed "logmind initialized successfully!" and a
// receipt listing `workflows`, and a re-run reported "All workflow templates
// already current." One refused artifact silently cost three unrelated ones.
//
// The assertion is on the FILES, not on the return value: a test on the
// installer's error would pass while the user still lost three workflows.
func TestInit_RefusedWorkflowDoesNotCostTheOtherThree(t *testing.T) {
	skipWithoutSymlinks(t)
	outside := t.TempDir()

	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(outside, "escaped.yml"), rel); err != nil {
			t.Fatalf("plant dangling symlink: %v", err)
		}

		root := NewRootCmd()
		root.SetArgs([]string{"init"})
		var out, errOut strings.Builder
		root.SetOut(&out)
		root.SetErr(&errOut)
		_ = root.Execute()
		stdout, stderr := out.String(), errOut.String()

		// THE HARM: the other three templates must be on disk.
		for _, name := range []string{"check-doc-links.yml", "logmind-self-update.yml", "regen-timeline.yml"} {
			p := filepath.Join(".github", "workflows", name)
			if _, err := os.Stat(p); err != nil {
				t.Errorf("%s was never installed — one refused artifact abandoned the rest (%v)", name, err)
			}
		}
		// The refusal is still refused: nothing outside the repo, link intact.
		if _, err := os.Stat(filepath.Join(outside, "escaped.yml")); err == nil {
			t.Error("init wrote through the dangling symlink, outside the repository")
		}
		if fi, err := os.Lstat(rel); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("the planted symlink was replaced rather than refused (lstat err=%v)", err)
		}

		// AND THE SUMMARY TELLS THE TRUTH. Both lines used to be
		// unconditional, which is what made the loss silent.
		if strings.Contains(stdout, "logmind initialized successfully!") {
			t.Errorf("init reported unqualified success having failed to write a workflow:\n%s", stdout)
		}
		if strings.Contains(stdout, "ok initialized: docs/ .logmind/ workflows @v") {
			t.Errorf("the receipt lists `workflows` as written when one of them was not:\n%s", stdout)
		}
		mustContain(t, stdout, "could NOT be written")
		mustContain(t, stdout, "workflows=3/4")
		// And the path that did not land is NAMED, not merely counted.
		mustContain(t, stderr, "check-decisions.yml")
		mustContain(t, stderr, "symlink")
	})
}

// TestInitRefresh_RefusedWorkflowIsNotReportedAsAllCurrent is the same defect
// on init's OTHER surface. With the loop aborting, the refresh pass returned an
// empty created/refreshed/declined triple, and the "all three lists are empty"
// test for the reassuring line was therefore satisfied — so a repo that had
// just failed to write a workflow was told every template was already current.
func TestInitRefresh_RefusedWorkflowIsNotReportedAsAllCurrent(t *testing.T) {
	skipWithoutSymlinks(t)
	outside := t.TempDir()

	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		runInitCapture(t, []string{"init"}) // first pass: fresh init
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		if err := os.Remove(rel); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(outside, "escaped.yml"), rel); err != nil {
			t.Fatalf("plant dangling symlink: %v", err)
		}

		// tryInitCapture, not runInitCapture: refresh mode returns a non-zero
		// exit when a workflow could not be written (#313), so the helper that
		// fatals on any error would fail here before asserting anything about
		// the output. The exit code is not merely tolerated — it is asserted
		// below, because it is the other half of the same rule this test is
		// about: a run that could not write a workflow must not look like a
		// clean run, on stdout OR to the caller's `$?`.
		stdout, stderr, execErr := tryInitCapture(t, []string{"init"}) // second pass: refresh mode

		if strings.Contains(stdout, "All workflow templates already current.") {
			t.Errorf("refresh reported every template current over a workflow it could not write:\n%s", stdout)
		}
		mustContain(t, stderr, "check-decisions.yml")
		mustContain(t, stderr, "was NOT written")
		if execErr == nil {
			t.Errorf("`init` in refresh mode exited 0 having failed to write check-decisions.yml — "+
				"the summary above lists only what DID get written, so exit 0 makes it read as the "+
				"whole story:\nstdout=%s\nstderr=%s", stdout, stderr)
		}
	})
}

// TestSelfUpdate_ReportsARefusedAgentsMDRefresh is #306 HIGH 5.
//
// `if …, err := inserter.EnsureAgentsMD(cwd); err == nil {` had no else, so a
// refused AGENTS.md write was discarded whole: self-update printed
// "✓ logmind templates are up to date." over a block that was still stale,
// exited 0, and said nothing on stderr. The error became reachable on this very
// branch, when the AGENTS.md write learned to refuse a symlink — the fix made
// the failure possible and then dropped it.
//
// Measured through the real command, on the three things the user can see:
// the block on disk, stdout, and the exit code.
func TestSelfUpdate_ReportsARefusedAgentsMDRefresh(t *testing.T) {
	skipWithoutSymlinks(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "AGENTS.md")

	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		// Init first so the hooks and .claude/settings.json this command also
		// refreshes are already current. Without that they report changes of
		// their own, `updated` goes true, and the stdout assertions below pass
		// for the wrong reason.
		runInitCapture(t, []string{"init"})

		// The same stale-but-REFRESHABLE block shape
		// TestSelfUpdate_PreservesAgentsMDOutsideBlock uses: the marker orders
		// before the bundled one and carries the slim flavour, so the refresh
		// genuinely engages. An unrecognised marker would be refused by
		// planBlockRefresh before any write was attempted, and would prove
		// nothing about a discarded write error.
		stale := "# AGENTS.md\n\nUSER_SENTINEL_OUTSIDE\n\n" +
			"<!-- logmind-start -->\n<!-- logmind-block-version: v1-pointer -->\nSTALE BLOCK BODY\n<!-- logmind-end -->\n"
		if err := os.WriteFile(target, []byte(stale), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove("AGENTS.md"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, "AGENTS.md"); err != nil {
			t.Fatalf("plant symlink: %v", err)
		}

		root := NewRootCmd()
		root.SetArgs([]string{"self-update"})
		var out, errOut strings.Builder
		root.SetOut(&out)
		root.SetErr(&errOut)
		err := root.Execute()
		stdout, stderr := out.String(), errOut.String()

		// The refusal held — the file outside the repo is untouched.
		if got := readRel(t, target); got != stale {
			t.Fatalf("self-update wrote through the symlink to a file outside the repo")
		}
		// And it was REPORTED rather than dropped.
		if strings.Contains(stdout, "✓ logmind templates are up to date.") {
			t.Errorf("self-update called a stale, unrefreshed block up to date:\n%s", stdout)
		}
		if strings.Contains(stdout, "ok self-update applied") {
			t.Errorf("self-update issued its applied receipt for a refresh that failed:\n%s", stdout)
		}
		if err == nil {
			t.Error("self-update exited 0 having failed to refresh the block; the failure was silent")
		}
		mustContain(t, stderr, "AGENTS.md")
		mustContain(t, stderr, "symlink")
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
