// headline_agreement_test.go — `logmind headline "x"` and `logmind log … -H
// "x"` must be interchangeable, because the AGENTS.md block this binary ships
// presents them that way: "set a one-sentence summary … `logmind headline
// "<one sentence>"`, or bundle it into a decision with `logmind log "..." -H
// "<one sentence>"`".
//
// They were not. On the default branch the marker write in log.go gated on
// isBranchFile alone while `logmind headline` also required a non-default
// branch, so `logmind log -H "x"` set main.md's headline and `logmind headline
// "x"` refused and left the seeded placeholder standing. Both now route
// through branchSummaryApplies, the single owner of that question.
//
// The unprompted nudge keeps its own, narrower predicate
// (branchSummaryNudgeApplies) — "can a summary be set here" and "should
// logmind interrupt to ask for one" are different questions, and it was
// collapsing them that produced the bug.
package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/templates"
)

// headlineLine returns the visible summary line of the §1.6.3 marker in path.
func headlineLine(t *testing.T, path string) string {
	t.Helper()
	for _, line := range strings.Split(readFileStr(t, path), "\n") {
		if strings.HasPrefix(line, "- **") {
			return line
		}
	}
	t.Fatalf("no marker summary line in %s", path)
	return ""
}

// TestHeadlineAndLogH_AgreeOnTheDefaultBranch is the regression: run each form
// on `main` and require the same observable effect.
func TestHeadlineAndLogH_AgreeOnTheDefaultBranch(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		withFakeTTY(t, false, func() { logOnce(t, "A decision on main") })
		mainFile := filepath.Join(d, "docs", "decisions-branches", "main.md")

		// Form A — the standalone command. This is the one that refused.
		runHeadlineCmd(t, "Set by the headline command")
		if got := headlineLine(t, mainFile); !strings.Contains(got, "Set by the headline command") {
			t.Fatalf("`logmind headline` did not set main.md's summary; marker line is %q", got)
		}

		// Form B — the bundled flag, which always worked here.
		withFakeTTY(t, false, func() { logOnceH(t, "Another decision", "Set by log -H") })
		if got := headlineLine(t, mainFile); !strings.Contains(got, "Set by log -H") {
			t.Fatalf("`logmind log -H` did not set main.md's summary; marker line is %q", got)
		}

		// One marker throughout — neither form stacked a second.
		if n := strings.Count(readFileStr(t, mainFile), "logmind-entry-start"); n != 1 {
			t.Errorf("marker count = %d; want 1", n)
		}
	})
}

// TestHeadline_StillWorksOnAFeatureBranch is the control: the widened gate did
// not break the case that already worked, so the test above is evidence about
// the default branch specifically.
func TestHeadline_StillWorksOnAFeatureBranch(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/agree")
		withFakeTTY(t, false, func() { logOnce(t, "A decision on the branch") })

		runHeadlineCmd(t, "Set on the feature branch")
		got := headlineLine(t, filepath.Join(d, "docs", "decisions-branches", "feat__agree.md"))
		if !strings.Contains(got, "Set on the feature branch") {
			t.Fatalf("marker line is %q", got)
		}
	})
}

// TestHeadline_RefusesWhereThereIsNoBranchFile: the gate still exists, it just
// asks a different question. Where `logmind log` would not write a branch file
// at all there is no §1.6.3 marker to set, and `headline` says so — naming the
// real reason rather than blaming the default branch.
func TestHeadline_RefusesWhereThereIsNoBranchFile(t *testing.T) {
	withTempCwd(t, func(d string) {
		// Not a git repo → resolveDecisionsPath returns docs/decisions.md,
		// isBranchFile=false.
		mustMkdir(t, filepath.Join(d, "docs"))

		var out, errBuf strings.Builder
		if err := runHeadline(d, "A summary", "", false, &out, &errBuf); err != nil {
			t.Fatalf("runHeadline: %v", err)
		}
		body := out.String() + errBuf.String()
		mustContain(t, body, "Branch summaries live in a branch decision file")
		mustNotContain(t, body, "the default branch has no in-flight work")
	})
}

// TestAgentsTemplate_BranchSummaryClaimIsTrue closes the loop the finding
// opened: the shipped block asserted the two forms were interchangeable while
// the code disagreed. Pin the claim here, in the package whose behaviour has
// to honour it, so a future narrowing of branchSummaryApplies trips a test
// that names the promise rather than one that only names a boolean.
func TestAgentsTemplate_BranchSummaryClaimIsTrue(t *testing.T) {
	// The template hard-wraps its prose, so compare on a whitespace-collapsed
	// copy — the claim is about the sentence, not about where it breaks.
	body := templates.AgentsTemplate()
	flat := strings.Join(strings.Fields(body), " ")
	mustContain(t, flat, "It applies on every branch — the default branch is a branch like any other.")
	// The retracted claim must not come back alongside it.
	mustNotContain(t, body, "no-op on the")
	mustNotContain(t, body, "On a feature branch, set a one-sentence")
}
