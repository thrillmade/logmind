// marker_overwrite_test.go — logmind#297 and #299 at the level the user was
// harmed: "my file lost its content".
//
// Both defects wrote something partial over the whole of a user-owned file.
// #297: `self-update` handed a BLOCK BODY to a function whose first parameter
// is a WHOLE FILE, then wrote the fragment that came back over AGENTS.md.
// #299: `doctor` classified a workflow as markerless (SPEC §1.1 — the user's,
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

// TestSelfUpdate_LeavesMarkerlessAgentsMDContentIntact — SPEC §1.1 for the
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

// TestDoctorFix_LeavesUnmarkedWorkflowAlone — the plain SPEC §1.1 case: a
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

// TestSelfUpdate_HasNoSecondAgentsMDWriter guards the structural half of the
// #297 fix. SPEC §1.1: "Exactly one automation owns any generated or copied
// path. Two refreshers MUST NOT write the same path." The deleted loop was a
// second refresher of AGENTS.md, and being unreachable is precisely why its
// wrong-argument write survived untested — so the absence of a second writer
// is the thing worth pinning, not the correctness of one that should not
// exist.
func TestSelfUpdate_HasNoSecondAgentsMDWriter(t *testing.T) {
	src, err := os.ReadFile("self_update.go")
	if err != nil {
		t.Fatalf("read self_update.go: %v", err)
	}
	// Control: prove the probe can actually find a call in this file, so a
	// pass means "absent", not "the search was wrong".
	if !strings.Contains(string(src), "inserter.EnsureAgentsMD") {
		t.Fatal("control failed: self_update.go no longer calls inserter.EnsureAgentsMD, " +
			"so this file is not the one that owns the AGENTS.md refresh — re-point this test")
	}
	for _, banned := range []string{
		"inserter.FindOutdatedMarkerBlocks",
		"os.WriteFile(entry.Path",
	} {
		if strings.Contains(string(src), banned) {
			t.Errorf("self-update grew a second AGENTS.md writer (%q); EnsureAgentsMD owns that path", banned)
		}
	}
}
