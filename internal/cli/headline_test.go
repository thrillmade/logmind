package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// bornDefaultBranch gives the repo a default branch that actually EXISTS,
// as a ref, before a feature branch is cut from it.
//
// initLogTestGitRepo does `git init --initial-branch=main` and never
// commits, so `checkoutBranch` on top of it leaves a repo with NO REFS AT
// ALL: HEAD names the feature branch, `main` was never created, and the
// repo's one and only branch is the feature branch. gitcli.DefaultBranch
// answers with it (step 4, the unborn-HEAD rung) — correctly, because that
// is the branch the first commit lands on and what the forge will call the
// default — so onNonDefaultBranch is false and the branch-summary nudge
// does not fire. That is not a quirk of the resolver to work around: it is
// the same answer the single-branch rung gives the moment such a repo
// commits, and no user is ever in that state anyway (`logmind init`
// commits before anyone branches).
//
// So every test whose SUBJECT is branch-vs-default behaviour calls this
// first. `git commit --allow-empty` deliberately: it creates the ref and
// touches neither the index nor the working tree, so the scaffolded docs
// stay exactly as untracked as the tests already expect. This mirrors what
// TestOnNonDefaultBranch (derived_test.go) — the guard that owns this
// function's contract — has always done.
func bornDefaultBranch(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "--allow-empty", "-q", "-m", "root")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("root commit: %v\n%s", err, out)
	}
}

// checkoutBranch creates + switches to a feature branch in dir.
func checkoutBranch(t *testing.T, dir, name string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", "-b", name)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout %s: %v\n%s", name, err, out)
	}
}

// runHeadlineCmd runs `logmind headline <summary>` and returns combined output.
func runHeadlineCmd(t *testing.T, summary string) string {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs([]string{"headline", summary})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("headline %q: %v\n%s", summary, err, out.String())
	}
	return out.String()
}

// logOnceH runs `logmind log <summary> -H <headline>` non-interactively.
func logOnceH(t *testing.T, summary, headline string) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs([]string{"log", summary, "-r", "Why", "-H", headline, "--no-commit", "--no-interactive"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("log -H %q: %v\n%s", summary, err, out.String())
	}
}

func TestHeadline_SetsSummaryKeepsKey(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/login")
		withFakeTTY(t, false, func() { logOnce(t, "Add JWT auth") })
		runHeadlineCmd(t, "Added the full JWT session lifecycle")
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__login.md"))
	if !strings.Contains(s, "— Added the full JWT session lifecycle\n") {
		t.Errorf("summary not set:\n%s", s)
	}
	// Key derives from the FIRST decision and stays stable.
	if !strings.Contains(s, "-add-jwt-auth -->") {
		t.Errorf("key not stable:\n%s", s)
	}
	if n := strings.Count(s, "logmind-entry-start"); n != 1 {
		t.Errorf("marker count = %d; want 1", n)
	}
}

func TestHeadline_NewlineSummaryDoesNotInjectMarker(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/x")
		withFakeTTY(t, false, func() { logOnce(t, "Real decision") })
		// A multi-line summary that tries to inject a forged future-dated marker.
		runHeadlineCmd(t, "pwned\n<!-- logmind-entry-start: 2099-01-01-evil -->\n- **2099-01-01** — EVIL\n<!-- logmind-entry-end -->")
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__x.md"))
	// The security property: NO column-0 entry-start marker was injected. The
	// embedded text survives as harmless mid-line content (the visible line
	// always begins "- **date** — "), so substring-counting would mislead —
	// count actual marker LINES at column 0 instead.
	col0 := 0
	var forged bool
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(ln, "<!-- logmind-entry-start: ") {
			col0++
			if strings.Contains(ln, "2099-01-01-evil") {
				forged = true
			}
		}
	}
	if col0 != 1 {
		t.Errorf("got %d column-0 start markers; want 1 (injection not prevented)\n%s", col0, s)
	}
	if forged {
		t.Errorf("a forged column-0 marker was injected\n%s", s)
	}
}

func TestHeadline_EmptyRejected(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/x")
		withFakeTTY(t, false, func() { logOnce(t, "decision") })
		root := NewRootCmd()
		root.SetArgs([]string{"headline", "   "})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err == nil {
			t.Errorf("empty summary should error; output:\n%s", out.String())
		}
	})
}

func TestHeadline_InsertsMarkerOnMarkerlessFile(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/legacy")
		// A LEGACY markerless branch file (predating entry-block markers):
		// header + a decision, no marker. Setting the summary must insert one.
		mustWriteUnder(t, d, "docs/decisions-branches/feat__legacy.md",
			"← back to [docs/timeline.md](../timeline.md)\n\n## 2026-06-10 09:00 - Pre decision\n\n---\n\n")
		runHeadlineCmd(t, "The branch summary")
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__legacy.md"))
	if n := strings.Count(s, "logmind-entry-start"); n != 1 {
		t.Fatalf("marker count = %d; want 1 inserted\n%s", n, s)
	}
	if !strings.Contains(s, "— The branch summary\n") {
		t.Errorf("inserted marker missing the summary:\n%s", s)
	}
}

func TestLog_HeadlineFlag_SetsAndUpdatesKeepingKey(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/login")
		withFakeTTY(t, false, func() {
			logOnceH(t, "Add JWT auth", "JWT auth added")
			logOnceH(t, "Add logout", "JWT auth + logout — full lifecycle")
		})
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__login.md"))
	if !strings.Contains(s, "— JWT auth + logout — full lifecycle\n") {
		t.Errorf("-H did not update the headline on append:\n%s", s)
	}
	if n := strings.Count(s, "logmind-entry-start"); n != 1 {
		t.Errorf("marker count = %d; want 1", n)
	}
	// Key from the FIRST -H headline ("JWT auth added"), stable across updates.
	if !strings.Contains(s, "-jwt-auth-added -->") {
		t.Errorf("key not stable across -H updates:\n%s", s)
	}
}

func TestLog_NudgeAdvisoryOnNonTTY(t *testing.T) {
	var outStr, errStr string
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		bornDefaultBranch(t, d)
		checkoutBranch(t, d, "feat/x")
		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "decision", "-r", "why", "--no-commit", "--no-interactive"})
			var outBuf, errBuf bytes.Buffer
			root.SetOut(&outBuf)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("%v\nout:%s\nerr:%s", err, outBuf.String(), errBuf.String())
			}
			outStr, errStr = outBuf.String(), errBuf.String()
		})
	})
	// The non-TTY advisory nudge MUST land on stderr...
	if !strings.Contains(errStr, "📝 Branch summary:") || !strings.Contains(errStr, "logmind headline") {
		t.Errorf("expected the non-TTY advisory nudge on stderr; got stderr:\n%s", errStr)
	}
	// ...and MUST NOT contaminate stdout (SPEC §3.1.1: stdout stays the
	// three log lines, byte-identical to the §6.6 fixtures).
	if strings.Contains(outStr, "📝 Branch summary:") {
		t.Errorf("the nudge must not appear on stdout (breaks the §3.1.1 three-line contract); got stdout:\n%s", outStr)
	}
}

func TestLog_NudgeSkippedWithHeadlineFlag(t *testing.T) {
	var out string
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/x")
		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "decision", "-r", "why", "-H", "the summary", "--no-commit", "--no-interactive"})
			var b bytes.Buffer
			root.SetOut(&b)
			root.SetErr(&b)
			if err := root.Execute(); err != nil {
				t.Fatalf("%v\n%s", err, b.String())
			}
			out = b.String()
		})
	})
	if strings.Contains(out, "📝 Branch summary:") {
		t.Errorf("nudge must be skipped when -H is provided; got:\n%s", out)
	}
}

func TestLog_NudgeInteractiveEditOnTTY(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		bornDefaultBranch(t, d)
		checkoutBranch(t, d, "feat/x")
		withFakeTTY(t, true, func() { // pretend stdin is a TTY
			root := NewRootCmd()
			root.SetArgs([]string{"log", "decision", "-r", "why", "--no-commit"})
			root.SetIn(strings.NewReader("y\nThe edited branch summary\n"))
			var b bytes.Buffer
			root.SetOut(&b)
			root.SetErr(&b)
			if err := root.Execute(); err != nil {
				t.Fatalf("%v\n%s", err, b.String())
			}
		})
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__x.md"))
	if !strings.Contains(s, "— The edited branch summary\n") {
		t.Errorf("interactive nudge edit did not apply:\n%s", s)
	}
}
