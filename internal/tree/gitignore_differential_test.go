package tree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Differential tests against real git.
//
// SPEC §1.4 says a `!pattern` "re-includes a path an earlier pattern
// excluded" — positional last-match-wins, which is git's own rule inside a
// .gitignore. Prose can be read two ways; `git check-ignore -v` cannot. It is
// the strongest oracle available for this behaviour, so the .gitignore source
// is compared against it directly rather than against our own expectations.
//
// SCOPE: the oracle covers §1.4's SECOND source only. git knows nothing about
// logmind's built-in defaults or file_structure.ignore_patterns, so these
// fixtures resolve `readGitignoreRules` alone. The three-way merge is pinned
// on rendered output in ignore_merge_test.go. Every fixture also uses
// top-level paths, where a path and its basename coincide, so the comparison
// isolates RESOLUTION rather than re-testing glob matching.
//
// The repos are hermetic: GIT_CONFIG_GLOBAL / GIT_CONFIG_SYSTEM are pointed
// at /dev/null so a developer's ~/.gitignore or core.excludesFile cannot
// change the verdict. Nothing is staged — `git check-ignore` reads the
// exclude rules, and leaving the index empty keeps it that way.

// runGit runs git in repoRoot with global/system config neutralised and
// returns stdout plus the exit code. A code other than the caller's expected
// set is a harness failure, not a verdict.
func runGit(t *testing.T, repoRoot string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.Output()
	if err == nil {
		return string(out), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code == 1 {
			return string(out), code
		}
		t.Fatalf("git %v in %s: exit %d (stderr: %s)", args, repoRoot, exitErr.ExitCode(), exitErr.Stderr)
	}
	t.Fatalf("git %v in %s: %v", args, repoRoot, err)
	return "", -1
}

// gitCheckIgnore reports whether real git considers path ignored in repoRoot,
// plus the "<file>:<line>:<pattern>" attribution.
//
// The verdict and the attribution come from SEPARATE invocations on purpose.
// `git check-ignore -v` changes what the exit code means: it becomes "some
// pattern MATCHED", so a path re-included by a trailing `!important.log`
// still exits 0 while printing `.gitignore:2:!important.log`. Reading that as
// "ignored" inverts every negation case in the table — which is precisely the
// behaviour under test, so the oracle would have agreed with a broken
// resolver. Measured on git 2.39.5:
//
//	$ printf '*.log\n!important.log\n' > .gitignore
//	$ git check-ignore -v important.log ; echo $?
//	.gitignore:2:!important.log	important.log
//	0
//	$ git check-ignore important.log ; echo $?
//	1
//
// So: plain run for the verdict (0 = ignored, 1 = not), -v run for the
// attribution only, which is diagnostic and never consulted for truth.
func gitCheckIgnore(t *testing.T, repoRoot, path string) (bool, string) {
	t.Helper()
	_, code := runGit(t, repoRoot, "check-ignore", "--no-index", path)
	verbose, _ := runGit(t, repoRoot, "check-ignore", "-v", "--no-index", path)
	return code == 0, strings.TrimSpace(verbose)
}

// gitignoreRepo creates a hermetic git repo whose .gitignore holds body, and
// materialises each path in files so the fixture is a real working tree.
func gitignoreRepo(t *testing.T, body string, files ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		full := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in %s: %v (%s)", root, err, out)
	}
	return root
}

// TestGitignoreResolution_AgreesWithGitCheckIgnore is the differential. Each
// fixture's verdict comes from `git check-ignore -v`, never from a hand-written
// expectation, so a divergence shows up as disagreement rather than as a test
// that was quietly written to match whatever we shipped.
//
// wantIgnored is carried alongside purely as a control on the ORACLE: if git
// stopped answering (wrong flags, a stray global excludes file, a harness bug
// swallowing an exit code) every case would silently collapse to one verdict
// and the differential would pass while comparing nothing. The table holds
// both polarities and asserts git produced each.
func TestGitignoreResolution_AgreesWithGitCheckIgnore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; the differential oracle is unavailable")
	}

	cases := []struct {
		name string
		body string
		path string
		// wantIgnored is what git MUST say — a control on the oracle, not
		// the assertion under test (that one compares git to logmind).
		wantIgnored bool
	}{
		{
			// The case the adversarial panel blocked #303 on. The negation
			// sits EARLIER than *.log, so it re-includes nothing: git
			// attributes the verdict to `.gitignore:2:*.log`.
			name:        "negation_before_the_pattern_loses",
			body:        "!important.log\n*.log\n",
			path:        "important.log",
			wantIgnored: true,
		},
		{
			// Same two lines, swapped. Now the negation is last and wins.
			// Together with the case above this pins that ORDER, not the
			// mere presence of a negation, decides.
			name:        "negation_after_the_pattern_wins",
			body:        "*.log\n!important.log\n",
			path:        "important.log",
			wantIgnored: false,
		},
		{
			// A pattern repeated after its own negation. This is the case
			// that makes first-seen deduplication wrong: drop line 3 as a
			// duplicate of line 1 and `!important.log` becomes last,
			// flipping the verdict away from git's.
			name:        "pattern_repeated_after_its_negation",
			body:        "*.log\n!important.log\n*.log\n",
			path:        "important.log",
			wantIgnored: true,
		},
		{
			// A directory logmind also ships as a built-in default, so the
			// repository re-including it is the real-world escape hatch —
			// and the .gitignore source alone must already agree with git.
			name:        "directory_reincluded_by_a_later_negation",
			body:        "dist\n!dist\n",
			path:        "dist",
			wantIgnored: false,
		},
		{
			// The same two lines the other way round: a negation that an
			// exclusion follows does not survive.
			name:        "directory_reexcluded_by_a_later_pattern",
			body:        "!dist\ndist\n",
			path:        "dist",
			wantIgnored: true,
		},
		{
			// Control: an ordinary single-rule exclusion, no negation
			// anywhere. Establishes the fixture harness reaches a verdict
			// at all before the ordering cases are believed.
			name:        "plain_exclusion_no_negation",
			body:        "node_modules\n",
			path:        "node_modules",
			wantIgnored: true,
		},
		{
			// Control: a path no rule mentions. Proves the oracle can
			// return "not ignored" for reasons other than a negation.
			name:        "unmatched_path",
			body:        "node_modules\n",
			path:        "README.md",
			wantIgnored: false,
		},
	}

	sawIgnored, sawNotIgnored := false, false
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := gitignoreRepo(t, tc.body, tc.path)

			gitIgnored, attribution := gitCheckIgnore(t, root, tc.path)
			if gitIgnored != tc.wantIgnored {
				t.Fatalf("ORACLE CONTROL FAILED: git check-ignore says ignored=%v for %q under %q; the fixture expected %v. Fix the harness before reading the comparison below (attribution: %q)",
					gitIgnored, tc.path, tc.body, tc.wantIgnored, attribution)
			}

			rules, err := readGitignoreRules(root)
			if err != nil {
				t.Fatal(err)
			}
			logmindIgnored := IgnoreRules(rules).Matches(tc.path, tc.path)

			if logmindIgnored != gitIgnored {
				t.Errorf("DIVERGENCE from git for %q under .gitignore %q:\n  git       ignored=%v (%s)\n  logmind   ignored=%v\n  rules     %+v",
					tc.path, tc.body, gitIgnored, attribution, logmindIgnored, rules)
			}
		})
		if tc.wantIgnored {
			sawIgnored = true
		} else {
			sawNotIgnored = true
		}
	}

	if !sawIgnored || !sawNotIgnored {
		t.Errorf("the differential table is one-sided (sawIgnored=%v sawNotIgnored=%v); it cannot distinguish a working comparison from a stuck one", sawIgnored, sawNotIgnored)
	}
}

// TestGitignoreResolution_DedupKeepsTheLastOccurrence pins the reason dedup
// keeps the last copy of a repeated rule rather than the first.
//
// ResolveRules deduplicates so the defaults and a config that repeats them do
// not grow the list without bound. Under last-match-wins, dropping the LATER
// copy moves a rule earlier and changes verdicts; dropping the earlier one
// cannot, because the surviving copy sits where the original decision was
// made. This is the same fixture as the differential's
// "pattern_repeated_after_its_negation" case, run through the full merge so
// the property is pinned where dedup actually runs.
func TestGitignoreResolution_DedupKeepsTheLastOccurrence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n!important.log\n*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, err := ResolveRules(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rules.Matches("important.log", "important.log") {
		t.Errorf("important.log survived; want ignored — the trailing *.log is the last match, and dedup must not have dropped it in favour of the leading copy:\n%+v", rules)
	}
	// The surviving copy must be the LAST one, i.e. positioned after the
	// negation. Assert the list shape too, so a resolver that got the right
	// answer for the wrong reason still fails.
	negateAt, lastLogAt := -1, -1
	for i, r := range rules {
		if r.Pattern == "important.log" && r.Negate {
			negateAt = i
		}
		if r.Pattern == "*.log" && !r.Negate {
			lastLogAt = i
		}
	}
	if negateAt == -1 || lastLogAt == -1 {
		t.Fatalf("expected both rules to survive dedup; got %+v", rules)
	}
	if lastLogAt < negateAt {
		t.Errorf("*.log is at %d, before !important.log at %d; dedup kept the FIRST occurrence and moved the decision earlier:\n%+v", lastLogAt, negateAt, rules)
	}
}
