package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// wellFormedEntry is a decision entry that satisfies SPEC §3.4's gate
// check — a title, a timestamp, and a reasoning section that is not
// empty. Tests that mean "the change carries a decision" write this;
// tests that mean "the file was merely touched" write something else.
const wellFormedEntry = "## 2026-08-07 14:30 - Test decision\n\n" +
	"**Reasoning:** The gate reads what a change wrote, not which files it named.\n\n" +
	"---\n"

// stagedOpts is the default check-decisions scope: the index, threshold
// left to config (which defaults to 20).
func stagedOpts(cwd string) checkDecisionsOpts {
	return checkDecisionsOpts{cwd: cwd, threshold: 20}
}

// TestCheckDecisions_NotARepo asserts the early-skip path: not a git
// repo → "Not a git repository, skipping check." + exit 0.
func TestCheckDecisions_NotARepo(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if err := runCheckDecisions(stagedOpts(dir), &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_not_a_repo.golden", stdout.String())
}

// TestCheckDecisions_NoChanges runs against a fresh repo with no
// staged changes. Expected: "✓ 0 lines changed (below 20-line threshold)."
func TestCheckDecisions_NoChanges(t *testing.T) {
	repo := initRepo(t)
	var stdout bytes.Buffer
	if err := runCheckDecisions(stagedOpts(repo), &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_clean.golden", stdout.String())
}

// TestCheckDecisions_OverThreshold stages 25 lines of non-docs code.
// Expected: warning + exit 1 (ErrSilent).
func TestCheckDecisions_OverThreshold(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "source.txt", 25)
	var stdout bytes.Buffer
	err := runCheckDecisions(stagedOpts(repo), &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runCheckDecisions err = %v; want ErrSilent", err)
	}
	checkGolden(t, "check_decisions_over_threshold.golden", stdout.String())
}

// TestCheckDecisions_OverThresholdNoFail verifies the same warning
// text but exit 0 when --no-fail is set.
func TestCheckDecisions_OverThresholdNoFail(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "source.txt", 25)
	opts := stagedOpts(repo)
	opts.noFail = true
	var stdout bytes.Buffer
	if err := runCheckDecisions(opts, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_over_threshold_nofail.golden", stdout.String())
}

// TestCheckDecisions_DocsStaged stages a well-formed docs/decisions.md
// entry — must be recognised as a written decision and short-circuit to
// the green path.
func TestCheckDecisions_DocsStaged(t *testing.T) {
	repo := initRepo(t)
	stageFile(t, repo, "docs/decisions.md", wellFormedEntry)
	var stdout bytes.Buffer
	if err := runCheckDecisions(stagedOpts(repo), &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_docs_staged.golden", stdout.String())
}

// TestCheckDecisions_BranchDecisionStaged covers the branch-aware
// path: a staged docs/decisions-branches/<branch>.md must also be
// recognised as a decision file.
func TestCheckDecisions_BranchDecisionStaged(t *testing.T) {
	repo := initRepo(t)
	stageFile(t, repo, "docs/decisions-branches/feature.md", wellFormedEntry)
	var stdout bytes.Buffer
	if err := runCheckDecisions(stagedOpts(repo), &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	// Reuses the docs-staged golden — the success message is the same
	// regardless of which decision file matched the predicate.
	checkGolden(t, "check_decisions_docs_staged.golden", stdout.String())
}

// TestCheckDecisions_CustomThreshold passes --threshold 50, stages
// 25 lines: must report "✓ 25 lines changed (below 50-line threshold)."
func TestCheckDecisions_CustomThreshold(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "source.txt", 25)
	opts := stagedOpts(repo)
	opts.threshold, opts.thresholdExplicit = 50, true
	var stdout bytes.Buffer
	if err := runCheckDecisions(opts, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_under_threshold.golden", stdout.String())
}

// TestCheckDecisions_DocsLOCIsExempt — a 25-line change to a
// docs/*.md file MUST be excluded from the LOC count, because docs
// changes don't represent decisions-being-made.
func TestCheckDecisions_DocsLOCIsExempt(t *testing.T) {
	repo := initRepo(t)
	stageLines(t, repo, "docs/random.md", 25)
	var stdout bytes.Buffer
	if err := runCheckDecisions(stagedOpts(repo), &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_clean.golden", stdout.String())
}

// --- SPEC §3.4 regression pins ----------------------------------------

// TestCheckDecisions_TouchedButMalformedDecisionDoesNotClear is the pin
// for §3.4: "A decision clears the gate by being written, not by
// existing. ... MUST NOT be satisfied by the decision file merely
// appearing in the diff. A test that asks only whether the file was
// touched is passed by a single meaningless line."
//
// The old implementation returned green the moment ANY staged path
// matched IsDecisionFile, so this exact input — one junk line in
// docs/decisions.md alongside 25 lines of code — passed. It must now
// fall through to the line count and fail.
func TestCheckDecisions_TouchedButMalformedDecisionDoesNotClear(t *testing.T) {
	repo := initRepo(t)
	stageFile(t, repo, "docs/decisions.md", ".\n")
	stageLines(t, repo, "source.txt", 25)
	var stdout bytes.Buffer
	err := runCheckDecisions(stagedOpts(repo), &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runCheckDecisions err = %v; want ErrSilent — a touched-but-empty decision file must not clear the gate (got %q)", err, stdout.String())
	}
	checkGolden(t, "check_decisions_over_threshold.golden", stdout.String())
}

// TestGateAndHookAgreeOnWhatRecordsADecision is the class-level pin for
// the hole a round-14 panel found: `logmind guard-commit` and
// `check-decisions` are two consumers of ONE rule, and they had grown two
// answers to it. The gate asked whether the change ADDED a well-formed
// entry; the commit hook asked only whether a decision-shaped PATH was
// staged — so `git add docs/decisions.md`, the content-free v1.2.0 install
// sentinel `logmind init` writes and which says in its own body that it
// holds no decisions, cleared the commit gate for any amount of code while
// CI still blocked the same change.
//
// Measured on the PR head: sentinel staged beside 302 lines of new Go,
// `guard-commit --layer git-hook` exit 0 "allowed (decision-file-staged)",
// `check-decisions` exit 1.
//
// So the assertion is AGREEMENT, not two independent expectations. A fix
// that closes the hook's hole by teaching it a second copy of the rule
// passes a per-surface test and fails this one the moment the copies
// drift; both surfaces route through guardcommit.DecisionRecorded.
func TestGateAndHookAgreeOnWhatRecordsADecision(t *testing.T) {
	// The shipped sentinel's shape: prose, and no `## <date> <time> -
	// <title>` header anywhere in it.
	const sentinel = "# Decision Log\n\nDecisions are not kept in this file. Since SPEC §3.2 every\n" +
		"decision lands in `docs/decisions-branches/<branch>.md`.\n\n" +
		"It is not written to, and it holds no decisions of its own.\n"

	cases := []struct {
		name        string
		path, body  string // decision file staged alongside the code; "" for none
		wantBlocked bool
	}{
		{name: "the v1.2.0 install sentinel records nothing",
			path: "docs/decisions.md", body: sentinel, wantBlocked: true},
		{name: "a legacy decisions.md with a real entry records a decision",
			path: "docs/decisions.md", body: wellFormedEntry, wantBlocked: false},
		{name: "the branch file logmind log writes records a decision",
			path: "docs/decisions-branches/feature.md", body: wellFormedEntry, wantBlocked: false},
		{name: "a header with no reasoning records nothing",
			path: "docs/decisions-branches/feature.md",
			body: "## 2026-08-07 14:30 - Untitled thought\n\n---\n", wantBlocked: true},
		{name: "no decision file at all",
			wantBlocked: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initRepo(t)
			stageLines(t, repo, "source.txt", 25)
			if tc.path != "" {
				stageFile(t, repo, tc.path, tc.body)
			}

			var gateOut bytes.Buffer
			gateBlocked := errors.Is(runCheckDecisions(stagedOpts(repo), &gateOut), ErrSilent)

			msgFile := filepath.Join(repo, "COMMIT_EDITMSG")
			if err := os.WriteFile(msgFile, []byte("feat: land it\n"), 0o644); err != nil {
				t.Fatalf("write msg file: %v", err)
			}
			var hookOut, hookErr bytes.Buffer
			exitCode, err := runGuardCommit(repo, "git-hook", msgFile, 20, false, true,
				strings.NewReader(""), &hookOut, &hookErr)
			if err != nil {
				t.Fatalf("runGuardCommit: %v", err)
			}
			hookBlocked := exitCode != 0

			if gateBlocked != hookBlocked {
				t.Fatalf("the gate and the commit hook DISAGREE about whether this change recorded a decision.\n"+
					"  check-decisions blocked = %v (out: %q)\n"+
					"  guard-commit    blocked = %v (exit %d, stderr: %q)\n"+
					"One rule, two consumers — a change either recorded a decision or it did not.",
					gateBlocked, gateOut.String(), hookBlocked, exitCode, hookErr.String())
			}
			if gateBlocked != tc.wantBlocked {
				t.Fatalf("both surfaces agree on blocked=%v, but want %v (gate out: %q, hook stderr: %q)",
					gateBlocked, tc.wantBlocked, gateOut.String(), hookErr.String())
			}
			// The hook's block signal is 65 (EX_DATAERR) specifically — the
			// commit-msg hook body treats every OTHER nonzero code as "not
			// our block signal" and falls open, so an agreement test that
			// only asked "nonzero" would pass on a gate that no longer aborts
			// the commit.
			if hookBlocked && exitCode != exGitHookBlock {
				t.Errorf("guard-commit blocked with exit %d; want %d — the commit-msg hook only aborts on that code",
					exitCode, exGitHookBlock)
			}
		})
	}
}

// TestCheckDecisions_ExcludedSurfacesDoNotCount walks SPEC §3.4's
// exclusion table end-to-end through the verb, one path per row, at a
// size that would otherwise trip the gate.
func TestCheckDecisions_ExcludedSurfacesDoNotCount(t *testing.T) {
	for _, path := range []string{
		"docs/plan.md",                    // 1. everything under docs/
		"AGENTS.md",                       // 2. the §1.1 instruction file
		"CLAUDE.md",                       // 2. a §1.2 per-tool file
		".github/copilot-instructions.md", // 2. a §1.2 per-tool file in a subdirectory
		".logmind/config.yml",             // 3. the toolchain's own configuration
	} {
		t.Run(path, func(t *testing.T) {
			repo := initRepo(t)
			stageLines(t, repo, path, 25)
			var stdout bytes.Buffer
			if err := runCheckDecisions(stagedOpts(repo), &stdout); err != nil {
				t.Fatalf("runCheckDecisions: %v (out: %q)", err, stdout.String())
			}
			checkGolden(t, "check_decisions_clean.golden", stdout.String())
		})
	}
}

// TestCheckDecisions_BinaryDoesNotCount pins exclusion 4 — "any file the
// forge reports as binary ... a line count means nothing there." The
// staged blob carries a NUL byte so git's numstat reports "-".
func TestCheckDecisions_BinaryDoesNotCount(t *testing.T) {
	repo := initRepo(t)
	stageFile(t, repo, "assets/blob.bin", strings.Repeat("\x00\x01\x02\x03\n", 40))
	var stdout bytes.Buffer
	if err := runCheckDecisions(stagedOpts(repo), &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_clean.golden", stdout.String())
}

// TestCheckDecisions_MarkdownOutsideDocsCounts is the negative half of
// the table, and the one that matters most. §3.4: "A skill file counts.
// So does an agent definition. ... Excluding markdown wholesale switches
// the rule off in the repositories where writing *is* the work."
func TestCheckDecisions_MarkdownOutsideDocsCounts(t *testing.T) {
	for _, path := range []string{
		".claude/skills/logmind/SKILL.md",
		".claude/agents/reviewer.md",
		"README.md",
	} {
		t.Run(path, func(t *testing.T) {
			repo := initRepo(t)
			stageLines(t, repo, path, 25)
			var stdout bytes.Buffer
			err := runCheckDecisions(stagedOpts(repo), &stdout)
			if !errors.Is(err, ErrSilent) {
				t.Fatalf("runCheckDecisions err = %v; want ErrSilent — %s is not excluded by §3.4 (out: %q)", err, path, stdout.String())
			}
		})
	}
}

// TestCheckDecisions_ThresholdFromConfig pins §3.4's "the threshold is
// git.commit_line_threshold, and defaults to 20": with no --threshold
// flag, the repo's config decides. 25 staged lines pass under a
// configured 50 and fail under a configured 10.
func TestCheckDecisions_ThresholdFromConfig(t *testing.T) {
	cases := []struct {
		configured int
		wantFail   bool
	}{
		{configured: 50, wantFail: false},
		{configured: 10, wantFail: true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("threshold_%d", tc.configured), func(t *testing.T) {
			repo := initRepo(t)
			writeConfig(t, repo, fmt.Sprintf("git:\n  commit_line_threshold: %d\n", tc.configured))
			stageLines(t, repo, "source.txt", 25)
			var stdout bytes.Buffer
			err := runCheckDecisions(stagedOpts(repo), &stdout)
			if tc.wantFail != errors.Is(err, ErrSilent) {
				t.Fatalf("commit_line_threshold %d: err = %v, wantFail = %v (out: %q)",
					tc.configured, err, tc.wantFail, stdout.String())
			}
			if !tc.wantFail && !strings.Contains(stdout.String(), fmt.Sprintf("below %d-line threshold", tc.configured)) {
				t.Fatalf("output %q does not report the configured threshold %d", stdout.String(), tc.configured)
			}
		})
	}
}

// TestCheckDecisions_ExplicitThresholdBeatsConfig keeps the documented
// precedence: an explicit --threshold still overrides config.
func TestCheckDecisions_ExplicitThresholdBeatsConfig(t *testing.T) {
	repo := initRepo(t)
	writeConfig(t, repo, "git:\n  commit_line_threshold: 10\n")
	stageLines(t, repo, "source.txt", 25)
	opts := stagedOpts(repo)
	opts.threshold, opts.thresholdExplicit = 50, true
	var stdout bytes.Buffer
	if err := runCheckDecisions(opts, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_under_threshold.golden", stdout.String())
}

// --- range mode (--base/--head), the CI shape -------------------------

// TestCheckDecisions_RangeMode exercises the base...head scope the
// check-decisions gate of §6.2 needs: a branch whose commits are already
// made (nothing staged) is judged against its base ref.
func TestCheckDecisions_RangeMode(t *testing.T) {
	repo := initRepo(t)
	base := revParse(t, repo, "HEAD")

	stageLines(t, repo, "source.txt", 25)
	commit(t, repo, "add source [skip-logmind]")
	head := revParse(t, repo, "HEAD")

	opts := stagedOpts(repo)
	opts.base, opts.head = base, head
	var stdout bytes.Buffer
	err := runCheckDecisions(opts, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runCheckDecisions err = %v; want ErrSilent (out: %q)", err, stdout.String())
	}
	checkGolden(t, "check_decisions_over_threshold.golden", stdout.String())

	// The staged index is empty, so today's default scope sees nothing —
	// which is precisely why CI needs the range.
	var staged bytes.Buffer
	if err := runCheckDecisions(stagedOpts(repo), &staged); err != nil {
		t.Fatalf("staged-scope runCheckDecisions: %v", err)
	}
	checkGolden(t, "check_decisions_clean.golden", staged.String())
}

// TestCheckDecisions_RangeModeDecisionClears — the same range, with a
// well-formed decision committed alongside the code, clears.
func TestCheckDecisions_RangeModeDecisionClears(t *testing.T) {
	repo := initRepo(t)
	base := revParse(t, repo, "HEAD")

	stageLines(t, repo, "source.txt", 25)
	stageFile(t, repo, "docs/decisions-branches/feature.md", wellFormedEntry)
	commit(t, repo, "add source with a decision [skip-logmind]")

	opts := stagedOpts(repo)
	opts.base, opts.head = base, revParse(t, repo, "HEAD")
	var stdout bytes.Buffer
	if err := runCheckDecisions(opts, &stdout); err != nil {
		t.Fatalf("runCheckDecisions: %v (out: %q)", err, stdout.String())
	}
	checkGolden(t, "check_decisions_docs_staged.golden", stdout.String())
}

// TestCheckDecisions_RangeModeMalformedDecisionDoesNotClear — the gate's
// well-formedness check applies to the range scope too, reading the
// lines the range ADDED rather than the file's whole contents. The
// decision file here already holds a valid entry from before the base,
// and the range adds only junk to it.
func TestCheckDecisions_RangeModeMalformedDecisionDoesNotClear(t *testing.T) {
	repo := initRepo(t)
	stageFile(t, repo, "docs/decisions-branches/feature.md", wellFormedEntry)
	commit(t, repo, "seed the decision file [skip-logmind]")
	base := revParse(t, repo, "HEAD")

	stageFile(t, repo, "docs/decisions-branches/feature.md", wellFormedEntry+".\n")
	stageLines(t, repo, "source.txt", 25)
	commit(t, repo, "append junk [skip-logmind]")

	opts := stagedOpts(repo)
	opts.base, opts.head = base, revParse(t, repo, "HEAD")
	var stdout bytes.Buffer
	err := runCheckDecisions(opts, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runCheckDecisions err = %v; want ErrSilent — a pre-existing entry must not clear a range that added none (out: %q)", err, stdout.String())
	}
	checkGolden(t, "check_decisions_over_threshold.golden", stdout.String())
}

// TestCheckDecisions_RangeModeBadRefFails — an unresolvable ref must be
// reported, not silently counted as an empty diff. "git couldn't resolve
// the range" and "the range changed nothing" produce identical numbers,
// and only one of them should let a pull request through.
func TestCheckDecisions_RangeModeBadRefFails(t *testing.T) {
	repo := initRepo(t)
	opts := stagedOpts(repo)
	opts.base, opts.head = "no-such-ref", "HEAD"
	var stdout bytes.Buffer
	if err := runCheckDecisions(opts, &stdout); err == nil {
		t.Fatalf("runCheckDecisions returned nil for an unresolvable base ref (out: %q)", stdout.String())
	}
}

// TestCheckDecisions_RangeModeRequiresBothRefs — half a range is a usage
// error, not a silent fallback to the staged scope.
func TestCheckDecisions_RangeModeRequiresBothRefs(t *testing.T) {
	repo := initRepo(t)
	for _, opts := range []checkDecisionsOpts{
		{cwd: repo, threshold: 20, base: "HEAD"},
		{cwd: repo, threshold: 20, head: "HEAD"},
	} {
		var stdout bytes.Buffer
		if err := runCheckDecisions(opts, &stdout); err == nil {
			t.Fatalf("runCheckDecisions(%+v) = nil; want a usage error", opts)
		}
	}
}

// stageLines writes `count` lines of "line N\n" to `name` inside
// repo, then `git add` it.
func stageLines(t *testing.T, repo, name string, count int) {
	t.Helper()
	var b bytes.Buffer
	for i := 1; i <= count; i++ {
		b.WriteString("line\n")
	}
	stageFile(t, repo, name, b.String())
}

// stageFile writes body to `name` inside repo (creating parents) and
// `git add`s it.
func stageFile(t *testing.T, repo, name, body string) {
	t.Helper()
	full := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := exec.Command("git", "add", name)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add %s: %v\n%s", name, err, out)
	}
}

// commit records whatever is staged. The subject carries
// [skip-logmind] so a developer machine with logmind's own commit-msg
// hook installed globally can't interfere — these repos are t.TempDir()
// scratch and the hook is not what's under test here.
func commit(t *testing.T, repo, subject string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "-m", subject)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func revParse(t *testing.T, repo, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

// TestCheckDecisions_RangeMode_RefusesLocalAllowances pins the BLOCKING
// finding from PR #287's adversarial review. SPEC §3.4 lists six local
// allowances and says of the gate: "every allowance above that depends on
// local process state — the environment variable, a git operation in
// progress, running outside a repository, a marker in a commit subject —
// is invisible to it and MUST NOT be honoured there. Exactly two things
// clear the gate."
//
// The not-a-repo arm ran BEFORE the range-mode split, so a gate invoked
// from the wrong directory — `working-directory:` pointing off the
// checkout, or `actions/checkout` with a `path:` — printed "skipping
// check" and exited 0. Every pull request would have gone green
// regardless of size, while reporting success.
func TestCheckDecisions_RangeMode_RefusesLocalAllowances(t *testing.T) {
	// Created at PARENT scope on purpose. t.TempDir() names the directory
	// after the calling test, so creating it inside a subtest named
	// "--no-fail …" yields a path containing "--no-fail" — and the
	// not-a-repo error echoes opts.cwd. That is how the original version of
	// this test passed while the guard it claimed to cover was deleted.
	dir := t.TempDir()

	t.Run("not a git repository is a hard error in range mode", func(t *testing.T) {
		dir := t.TempDir() // deliberately not a git repo
		var out bytes.Buffer
		err := runCheckDecisions(checkDecisionsOpts{
			cwd: dir, base: "origin/main", head: "HEAD",
		}, &out)
		if err == nil {
			t.Fatalf("range mode outside a repository returned nil (gate would pass); stdout=%q", out.String())
		}
	})

	t.Run("not a git repository still skips gracefully in local mode", func(t *testing.T) {
		dir := t.TempDir()
		var out bytes.Buffer
		if err := runCheckDecisions(checkDecisionsOpts{cwd: dir}, &out); err != nil {
			t.Fatalf("local mode outside a repository must not error, got %v", err)
		}
		if !strings.Contains(out.String(), "Not a git repository") {
			t.Errorf("local mode lost its skip message; got %q", out.String())
		}
	})

	t.Run("--no-fail is refused in range mode", func(t *testing.T) {
		// dir comes from the PARENT scope deliberately. t.TempDir() names the
		// directory after the subtest, so a subtest called "--no-fail …"
		// produces a path CONTAINING "--no-fail" — and the not-a-repo error
		// echoes opts.cwd. An adversarial review found the original assertion
		// matching that path rather than the guard: deleting the guard
		// entirely left this test green.
		var out bytes.Buffer
		err := runCheckDecisions(checkDecisionsOpts{
			cwd: dir, base: "a", head: "b", noFail: true,
		}, &out)
		if err == nil {
			t.Fatal("--no-fail with --base/--head returned nil; it is a third thing clearing the gate")
		}
		// Assert on text unique to the guard, not on a flag name that can
		// appear in an incidental path.
		if !strings.Contains(err.Error(), "cannot be combined with --base/--head") {
			t.Errorf("error is not the guard's refusal, got %v", err)
		}
		if !strings.Contains(err.Error(), "--no-fail") {
			t.Errorf("refusal should name the flag, got %v", err)
		}
	})
}

// TestCheckDecisions_RangeMode_RefusesThreshold pins the fourth
// gate-clearer. Its sibling --no-fail refusal was tested; this one shipped
// untested and an adversarial review caught the gap.
//
// SPEC §3.4 pins the gate's threshold to git.commit_line_threshold and
// nothing else. --threshold is worse than --no-fail as an escape because
// it does not look like one: `--threshold 999999` reads as configuration
// rather than a bypass, so it is the version a reviewer waves through.
func TestCheckDecisions_RangeMode_RefusesThreshold(t *testing.T) {
	dir := t.TempDir()

	t.Run("--threshold is refused in range mode", func(t *testing.T) {
		var out bytes.Buffer
		err := runCheckDecisions(checkDecisionsOpts{
			cwd: dir, base: "a", head: "b",
			threshold: 999999, thresholdExplicit: true,
		}, &out)
		if err == nil {
			t.Fatal("--threshold with --base/--head returned nil; it is a third thing clearing the gate")
		}
		if !strings.Contains(err.Error(), "cannot be combined with --base/--head") {
			t.Errorf("error is not the guard's refusal, got %v", err)
		}
		if !strings.Contains(err.Error(), "--threshold") {
			t.Errorf("refusal should name the flag, got %v", err)
		}
	})

	t.Run("--threshold is still allowed locally", func(t *testing.T) {
		var out bytes.Buffer
		// Not a repo, so the local path returns its skip message rather
		// than evaluating — enough to prove the flag is not refused.
		if err := runCheckDecisions(checkDecisionsOpts{
			cwd: dir, threshold: 999999, thresholdExplicit: true,
		}, &out); err != nil {
			t.Fatalf("--threshold alone must not be refused, got %v", err)
		}
	})
}
