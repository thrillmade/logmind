// pulse_test.go — exercises the v2.0.0 "pulse" feature: `logmind log`'s
// stderr-only repo-health advisories (drift pulse + spec pulse + the
// derived-docs-on-main main-decisions pulse, pulse.go).
//
// Coverage:
//   - byte-exact §3.1 stdout contract holds even when a drift pulse fires
//     on stderr (the load-bearing "pulse never touches stdout" guarantee)
//   - drift pulse: fires with the correct count on a deliberately-staled
//     hook, stays silent on a fully-current (nothing-stale) repo
//   - spec pulse: fires exactly at the specPulseThreshold boundary (19
//     silent, 20 fires), with the correct count + path; stays silent when
//     the spec is untracked or context.spec_file is unset
//   - main-decisions pulse: fires with the correct singular/plural count on
//     a stale feature branch, stays silent on the default branch, stays
//     silent for a non-decision-touching origin advance, and is network-free
//     (never auto-fetches — see the "no auto-fetch" case below)
//   - ordering: drift pulse prints before spec pulse when both fire
//   - --quiet: the single `ok ...` stdout contract holds; pulse still
//     lands on stderr
//   - failure-safety: git absent from PATH → log still succeeds, pulse
//     silently skips, no error
package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/testgit"
)

// neutralizePathProbe points PATH at a directory containing ONLY a `git`
// symlink (resolved from the real PATH) so doctor's PATH-resolution probe
// deterministically reports "missing" instead of picking up whatever
// `logmind` binary (if any, and at whatever version) happens to be
// installed on the test runner's PATH. Without this, TestLog_Pulse tests
// that assert an EXACT stale count or an EXACT-empty stderr would be
// flaky across developer machines / CI images. `git` stays resolvable
// because `logmind log` itself needs it for the commit flow.
func neutralizePathProbe(t *testing.T) {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH; skipping")
	}
	binDir := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatalf("symlink git: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", binDir)
}

// writeStaleHook installs a `.git/hooks/post-merge` body carrying a
// deliberately-wrong `# logmind-hook-version:` marker, so doctor's
// probeHook classifies it "stale" (installed marker != the running
// binary's version.Version) — the one drift class the pulse counts.
func writeStaleHook(t *testing.T, dir string) {
	t.Helper()
	hookPath := filepath.Join(dir, ".git", "hooks", "post-merge")
	body := "#!/bin/sh\n# logmind post-merge hook\n" + hooks.HookVersionPrefix + "v0-FAKE\necho fake\n"
	if err := os.WriteFile(hookPath, []byte(body), 0o755); err != nil {
		t.Fatalf("write stale hook: %v", err)
	}
}

// commitFileWithDate writes content to relPath under dir, stages it, and
// commits it with a fixed author+committer date (RFC3339). Used to give a
// spec file a deterministic, far-in-the-past "last touched" commit so the
// spec pulse's decision-count comparison is reproducible.
func commitFileWithDate(t *testing.T, dir, relPath, content, date string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
	addCmd := exec.Command("git", "add", relPath)
	addCmd.Dir = dir
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add %s: %v\n%s", relPath, err, out)
	}
	commitCmd := exec.Command("git", "commit", "-q", "-m", "add "+relPath)
	commitCmd.Dir = dir
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_DATE="+date,
	)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit %s: %v\n%s", relPath, err, out)
	}
}

// addFillerDecisions runs `logmind log` n times against the process's
// current directory (set by withTempCwd) with --no-commit so each call is
// just a fast markdown-file write — no git overhead. Used to pad
// docs/decisions.md up to (or just under) specPulseThreshold.
func addFillerDecisions(t *testing.T, n int) {
	t.Helper()
	withFakeTTY(t, false, func() {
		for i := 0; i < n; i++ {
			root := NewRootCmd()
			root.SetArgs([]string{"log", fmt.Sprintf("filler decision %d", i), "-r", "why", "--no-commit", "--no-interactive"})
			var sink bytes.Buffer
			root.SetOut(&sink)
			root.SetErr(&sink)
			if err := root.Execute(); err != nil {
				t.Fatalf("filler log #%d: %v\n%s", i, err, sink.String())
			}
		}
	})
}

// TestLog_Pulse_ByteExactStdout_NonTTY_DriftPresent is the release-bar
// proof that the pulse NEVER touches stdout: a drift pulse fires on
// stderr, and stdout is still the §3.1 three-line contract, byte-for-byte
// (bytes.Equal, not a substring match — a rogue extra byte anywhere would
// fail this).
func TestLog_Pulse_ByteExactStdout_NonTTY_DriftPresent(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		neutralizePathProbe(t)
		writeStaleHook(t, d)

		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "byte exact decision", "-r", "why", "--no-push", "--no-interactive"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log: %v\nstderr:\n%s", err, errBuf.String())
			}

			wantStdout := []byte("ℹ Staging all changes (use --stage scoped to limit)\n" +
				`✓ Logged decision: "byte exact decision"` + "\n" +
				"✓ Committed changes\n")
			gotStdout := out.Bytes()
			if !bytes.Equal(gotStdout, wantStdout) {
				t.Fatalf("stdout not byte-exact.\nwant (%d bytes) % x\nwant text: %q\ngot  (%d bytes) % x\ngot  text: %q",
					len(wantStdout), wantStdout, wantStdout, len(gotStdout), gotStdout, gotStdout)
			}

			wantStderr := "logmind: 1 component stale — run 'logmind doctor --fix'\n"
			if errBuf.String() != wantStderr {
				t.Fatalf("stderr = %q; want exactly %q", errBuf.String(), wantStderr)
			}
		})
	})
}

// TestLog_Pulse_NoDriftLine_WhenNothingStale: a freshly-scaffolded repo
// (nothing installed beyond docs/) has zero STALE components — "missing"
// isn't drift — so the drift pulse must stay silent. No spec_file is
// configured either, so stderr should be entirely empty.
func TestLog_Pulse_NoDriftLine_WhenNothingStale(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		neutralizePathProbe(t)

		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "clean decision", "-r", "why", "--no-push", "--no-interactive"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log: %v\nstderr:\n%s", err, errBuf.String())
			}
			if errBuf.Len() != 0 {
				t.Fatalf("stderr should be empty (nothing stale, no spec configured); got %q", errBuf.String())
			}
		})
	})
}

// TestLog_Pulse_SpecLine_ThresholdBoundary pins the exact
// specPulseThreshold gate: scaffoldDocs already logs one decision (init's
// "first decision"); 17 filler entries bring the total to 18, so the next
// log call lands the 19th entry (still below threshold — must stay
// silent) and the one after that lands the 20th (meets the threshold —
// must fire, with the exact count and the spec's path).
func TestLog_Pulse_SpecLine_ThresholdBoundary(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t) // logs 1 pre-existing decision (init's own "first decision")
		neutralizePathProbe(t)
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: SPEC.md\n")
		commitFileWithDate(t, d, "SPEC.md", "# Spec\n", "2020-01-01T00:00:00Z")

		addFillerDecisions(t, 17) // 1 (init) + 17 = 18 total so far

		withFakeTTY(t, false, func() {
			// 19th entry — one short of specPulseThreshold (20). Must stay silent.
			root := NewRootCmd()
			root.SetArgs([]string{"log", "checkpoint 19", "-r", "why", "--no-commit", "--no-interactive"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log (19th entry): %v\n%s", err, errBuf.String())
			}
			if strings.Contains(errBuf.String(), "unchanged for") {
				t.Fatalf("spec pulse fired at 19 decisions (below threshold); stderr:\n%s", errBuf.String())
			}

			// 20th entry — meets specPulseThreshold. Must fire with count=20.
			root = NewRootCmd()
			root.SetArgs([]string{"log", "checkpoint 20", "-r", "why", "--no-commit", "--no-interactive"})
			out.Reset()
			errBuf.Reset()
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log (20th entry): %v\n%s", err, errBuf.String())
			}
			mustContain(t, errBuf.String(), "logmind: SPEC.md unchanged for 20 decisions — still accurate?")
		})
	})
}

// TestLog_Pulse_SpecUntracked_Silent: context.spec_file resolves to a real
// file on disk, but it was never `git add`ed/committed. Untracked/
// uncommitted has no deterministic last-touched date across clones, so the
// pulse must skip it — regardless of how many decisions have piled up.
func TestLog_Pulse_SpecUntracked_Silent(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		neutralizePathProbe(t)
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: SPEC.md\n")
		if err := os.WriteFile(filepath.Join(d, "SPEC.md"), []byte("# Spec\n"), 0o644); err != nil {
			t.Fatalf("write SPEC.md: %v", err)
		}
		// Deliberately NOT added/committed — SPEC.md stays untracked.

		// Comfortably over specPulseThreshold, so if this test fails it's
		// unambiguously because untracked status was ignored — not because
		// the count was too low.
		addFillerDecisions(t, 25)

		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "untracked spec test", "-r", "why", "--no-commit", "--no-interactive"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log: %v\n%s", err, errBuf.String())
			}
			if strings.Contains(errBuf.String(), "unchanged for") {
				t.Fatalf("spec pulse fired for an untracked spec file; stderr:\n%s", errBuf.String())
			}
		})
	})
}

// TestLog_Pulse_SpecUncommittedWorkingTreeEdit_Silent: the spec file IS
// tracked and has a real last-commit date, but it also has an uncommitted
// working-tree edit right now (e.g. this same `logmind log` was invoked
// with --no-commit, or --stage scoped, while the author is mid-edit on the
// spec). The pulse must skip — the log that's touching the spec shouldn't
// turn around and ask whether the spec is still accurate.
func TestLog_Pulse_SpecUncommittedWorkingTreeEdit_Silent(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		neutralizePathProbe(t)
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: SPEC.md\n")
		commitFileWithDate(t, d, "SPEC.md", "# Spec\n", "2020-01-01T00:00:00Z")

		// Comfortably over specPulseThreshold against the 2020 commit date,
		// so if this test fails it's unambiguously because the uncommitted
		// edit was ignored — not because the count was too low.
		addFillerDecisions(t, 25)

		// Now edit SPEC.md WITHOUT committing — a dirty working tree.
		if err := os.WriteFile(filepath.Join(d, "SPEC.md"), []byte("# Spec\n\nmid-edit.\n"), 0o644); err != nil {
			t.Fatalf("edit SPEC.md: %v", err)
		}

		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			// --no-commit: nothing (including the SPEC.md edit) gets
			// committed, so the edit is still dirty when emitPulse runs.
			root.SetArgs([]string{"log", "mid-spec-edit test", "-r", "why", "--no-commit", "--no-interactive"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log: %v\n%s", err, errBuf.String())
			}
			if strings.Contains(errBuf.String(), "unchanged for") {
				t.Fatalf("spec pulse fired while SPEC.md has an uncommitted working-tree edit; stderr:\n%s", errBuf.String())
			}
		})
	})
}

// TestLog_Pulse_SpecUncommittedStagedEdit_Silent: same as the working-tree
// variant above, but the spec edit is STAGED (git add'ed) rather than left
// in the working tree — `git status --porcelain` reports it either way, so
// the pulse must skip here too.
func TestLog_Pulse_SpecUncommittedStagedEdit_Silent(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		neutralizePathProbe(t)
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: SPEC.md\n")
		commitFileWithDate(t, d, "SPEC.md", "# Spec\n", "2020-01-01T00:00:00Z")

		addFillerDecisions(t, 25)

		if err := os.WriteFile(filepath.Join(d, "SPEC.md"), []byte("# Spec\n\nstaged edit.\n"), 0o644); err != nil {
			t.Fatalf("edit SPEC.md: %v", err)
		}
		addCmd := exec.Command("git", "add", "SPEC.md")
		addCmd.Dir = d
		if out, err := addCmd.CombinedOutput(); err != nil {
			t.Fatalf("git add SPEC.md: %v\n%s", err, out)
		}
		// Deliberately NOT committed — staged only.

		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "staged-spec-edit test", "-r", "why", "--no-commit", "--no-interactive"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log: %v\n%s", err, errBuf.String())
			}
			if strings.Contains(errBuf.String(), "unchanged for") {
				t.Fatalf("spec pulse fired while SPEC.md has a staged-but-uncommitted edit; stderr:\n%s", errBuf.String())
			}
		})
	})
}

// TestLog_Pulse_SpecUnset_Silent: context.spec_file is left at its default
// ("") — config.ResolveSpecFile reports unset, so the pulse must never
// even attempt the git/decision-count work.
func TestLog_Pulse_SpecUnset_Silent(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		neutralizePathProbe(t)

		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "spec unset test", "-r", "why", "--no-push", "--no-interactive"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log: %v\n%s", err, errBuf.String())
			}
			if strings.Contains(errBuf.String(), "unchanged for") {
				t.Fatalf("spec pulse fired with spec_file unset; stderr:\n%s", errBuf.String())
			}
		})
	})
}

// TestLog_Pulse_OrderingDriftBeforeSpec: when both pulses fire in the same
// invocation, the drift line must print FIRST, spec line SECOND — and
// nothing else lands on stderr.
func TestLog_Pulse_OrderingDriftBeforeSpec(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		neutralizePathProbe(t)
		writeStaleHook(t, d)
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: SPEC.md\n")
		commitFileWithDate(t, d, "SPEC.md", "# Spec\n", "2020-01-01T00:00:00Z")
		addFillerDecisions(t, 25) // comfortably over specPulseThreshold

		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "ordering test", "-r", "why", "--no-commit", "--no-interactive"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log: %v\n%s", err, errBuf.String())
			}
			stderr := errBuf.String()
			driftIdx := strings.Index(stderr, "component")
			specIdx := strings.Index(stderr, "unchanged for")
			if driftIdx == -1 || specIdx == -1 {
				t.Fatalf("expected both drift and spec pulses to fire; stderr:\n%s", stderr)
			}
			if driftIdx > specIdx {
				t.Fatalf("drift pulse must print BEFORE spec pulse; stderr:\n%s", stderr)
			}
			lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
			if len(lines) != 2 {
				t.Fatalf("expected exactly 2 pulse lines on stderr; got %d:\n%s", len(lines), stderr)
			}
		})
	})
}

// TestLog_Pulse_QuietMode_StdoutSingleOK_StderrHasDriftPulse: --quiet's
// single-`ok`-line stdout contract must hold exactly as it does without a
// pulse; the drift pulse still lands on stderr, since stderr sits outside
// that contract.
func TestLog_Pulse_QuietMode_StdoutSingleOK_StderrHasDriftPulse(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		neutralizePathProbe(t)
		writeStaleHook(t, d)

		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "quiet pulse test", "-r", "why", "--no-push", "--no-interactive", "--quiet"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log --quiet: %v\n%s", err, errBuf.String())
			}
			assertSingleOK(t, out.String(), "logged", "committed=true", "pushed=false")
			mustContain(t, errBuf.String(), "logmind: 1 component stale — run 'logmind doctor --fix'")
		})
	})
}

// TestLog_Pulse_GitAbsentFromPath_NoErrorNoPulse is the failure-safety
// proof: a spec file tracked+committed long ago, comfortably over
// specPulseThreshold decisions on top — every ingredient the spec pulse
// needs — but with `git` removed from PATH for the actual `logmind log`
// invocation. gitcli.IsTrackedFile / LastCommitTime fail closed (best-
// effort false/zero-value, never a panic), so the pulse silently skips
// and the log still succeeds with exit 0. Proves the "never fail or slow
// the log" guarantee holds even when the underlying git calls error out.
func TestLog_Pulse_GitAbsentFromPath_NoErrorNoPulse(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: SPEC.md\n")
		commitFileWithDate(t, d, "SPEC.md", "# Spec\n", "2020-01-01T00:00:00Z")
		addFillerDecisions(t, 25) // over threshold — would fire if git worked

		// Strip PATH down to nothing — no git, no logmind. Must happen AFTER
		// the setup above, which needs a real git on PATH.
		origPath := os.Getenv("PATH")
		t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
		_ = os.Setenv("PATH", t.TempDir())

		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "git-absent test", "-r", "why", "--no-commit", "--no-interactive"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log should succeed even with git absent from PATH: %v\nstderr:\n%s", err, errBuf.String())
			}
			mustContain(t, out.String(), `✓ Logged decision: "git-absent test"`)
			if strings.Contains(errBuf.String(), "logmind:") {
				t.Fatalf("pulse should be silent when git is absent from PATH; stderr:\n%s", errBuf.String())
			}
		})
	})
}

// TestLog_Pulse_SpecLine_TimezoneStable pins issue #222: the spec-staleness
// verdict must be identical regardless of the running machine's timezone (the
// #211 localize+instant compare flipped it by the full UTC offset). We pin
// time.Local to several extreme zones and assert specPulseLine returns the same
// (line, ok) for the same on-disk repo state, and that a decision on the SAME
// calendar day as the spec commit is NOT counted (same-day tie is not-after, so
// no false "still accurate?" nudge). The pre-fix code fired for the same-day
// fixture under negative-offset zones while staying silent under positive ones,
// so this test fails against it and passes against the calendar-day fix.
func TestLog_Pulse_SpecLine_TimezoneStable(t *testing.T) {
	origLocal := time.Local
	t.Cleanup(func() { time.Local = origLocal })

	zones := []*time.Location{
		time.UTC,
		time.FixedZone("UTC+14", 14*3600),
		time.FixedZone("UTC-12", -12*3600),
	}

	run := func(t *testing.T, decisionDay string, wantOK bool, wantCount int) {
		withTempCwd(t, func(d string) {
			initLogTestGitRepo(t, d)
			scaffoldDocs(t)
			mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: SPEC.md\n")
			commitFileWithDate(t, d, "SPEC.md", "# Spec\n", "2020-06-15T12:00:00Z")

			// Overwrite docs/decisions.md with exactly specPulseThreshold headers
			// dated decisionDay at noon — direct-write so the header dates are
			// fully controlled (not time.Now()).
			var b strings.Builder
			for i := 0; i < specPulseThreshold; i++ {
				fmt.Fprintf(&b, "## %s 12:00 - decision %d\n\n**Reasoning:** why\n\n---\n\n", decisionDay, i)
			}
			mustWriteUnder(t, d, "docs/decisions.md", b.String())

			for _, loc := range zones {
				time.Local = loc
				line, ok := specPulseLine(d)
				if ok != wantOK {
					t.Fatalf("TZ %s day=%s: specPulseLine ok=%v want %v (line=%q)", loc, decisionDay, ok, wantOK, line)
				}
				if wantOK {
					want := fmt.Sprintf("logmind: SPEC.md unchanged for %d decisions — still accurate?", wantCount)
					if line != want {
						t.Fatalf("TZ %s: line=%q want %q", loc, line, want)
					}
				}
			}
		})
	}

	// Day AFTER the spec commit day → all counted → fires with count=threshold.
	t.Run("day_after_fires", func(t *testing.T) {
		run(t, "2020-06-16", true, specPulseThreshold)
	})
	// SAME calendar day as the spec commit → same-day tie is not-after → 0
	// counted → below threshold → silent, in every zone.
	t.Run("same_day_silent", func(t *testing.T) {
		run(t, "2020-06-15", false, 0)
	})
}

// --- v2.0.0 derived-docs-on-main: main-decisions pulse (mainDecisionsPulseLine) ---
//
// initClonePair / commitOn / runGitIn are shared with warp_test.go (same
// package) — a real origin/repo clone pair is the natural fixture for
// exercising a probe that compares HEAD against a remote-tracking ref.

// TestMainDecisionsPulseLine_FiresOnStaleBranch: on a feature branch with a
// pre-fetched origin/main carrying exactly one decision-touching commit the
// branch lacks, the probe fires with the singular ("commit", not "commits")
// exact message.
func TestMainDecisionsPulseLine_FiresOnStaleBranch(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/decisions.md", "## decision one\n")
	runGitIn(t, repo, "fetch", "origin", "main")
	runGitIn(t, repo, "checkout", "-b", "feat/p")

	line, ok := mainDecisionsPulseLine(repo)
	if !ok {
		t.Fatal("expected the main-decisions pulse to fire")
	}
	want := "logmind: main has 1 new decision commit — run 'logmind warp' to catch up"
	if line != want {
		t.Fatalf("line = %q; want %q", line, want)
	}
}

// TestMainDecisionsPulseLine_PluralCount: two decision-touching commits
// ahead pluralizes ("commits").
func TestMainDecisionsPulseLine_PluralCount(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/decisions.md", "## decision one\n")
	commitOn(t, origin, "docs/decisions-branches/other.md", "## decision two\n")
	runGitIn(t, repo, "fetch", "origin", "main")
	runGitIn(t, repo, "checkout", "-b", "feat/p2")

	line, ok := mainDecisionsPulseLine(repo)
	if !ok {
		t.Fatal("expected the main-decisions pulse to fire")
	}
	want := "logmind: main has 2 new decision commits — run 'logmind warp' to catch up"
	if line != want {
		t.Fatalf("line = %q; want %q", line, want)
	}
}

// TestMainDecisionsPulseLine_SilentOnDefaultBranch: the probe is gated by
// onNonDefaultBranch — even with origin/main visibly ahead, staying on the
// default branch (where L0/L1 don't apply and the docs regenerate locally)
// must not fire.
func TestMainDecisionsPulseLine_SilentOnDefaultBranch(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "docs/decisions.md", "## decision one\n")
	runGitIn(t, repo, "fetch", "origin", "main")
	// repo stays on its default branch ("main") — no checkout.

	if _, ok := mainDecisionsPulseLine(repo); ok {
		t.Fatal("main-decisions pulse must stay silent on the default branch")
	}
}

// TestMainDecisionsPulseLine_NonDecisionCommit_Silent: origin advancing on an
// unrelated path (not one of the three decision sources) must not fire —
// the probe is scoped to decision-touching commits only, not "any commit".
func TestMainDecisionsPulseLine_NonDecisionCommit_Silent(t *testing.T) {
	origin, repo := initClonePair(t)
	commitOn(t, origin, "README.md", "unrelated change\n")
	runGitIn(t, repo, "fetch", "origin", "main")
	runGitIn(t, repo, "checkout", "-b", "feat/p3")

	if _, ok := mainDecisionsPulseLine(repo); ok {
		t.Fatal("main-decisions pulse must stay silent when origin advanced on a non-decision path")
	}
}

// TestMainDecisionsPulseLine_NetworkFree_NoAutoFetch: the probe must NEVER
// fetch — it reads the local origin/<default> ref as of the last explicit
// fetch/warp only. Advancing origin WITHOUT fetching in repo must leave the
// probe silent (the stale local ref still matches HEAD).
func TestMainDecisionsPulseLine_NetworkFree_NoAutoFetch(t *testing.T) {
	origin, repo := initClonePair(t)
	runGitIn(t, repo, "checkout", "-b", "feat/q")
	// Advance origin AFTER the clone/checkout, WITHOUT ever fetching in repo.
	commitOn(t, origin, "docs/decisions.md", "## decision one\n")

	if _, ok := mainDecisionsPulseLine(repo); ok {
		t.Fatal("main-decisions pulse must be network-free: it saw origin's new commit without an explicit fetch")
	}
}

// initClonePairScaffolded builds a fully logmind-scaffolded origin
// (docs/decisions.md, docs/timeline.md, .logmind/config.yml, ... via
// `logmind init --no-git`) and a `git clone` of it. The plain initClonePair
// (warp_test.go) only seeds docs/timeline.md, which is enough for warp but
// not for a real `logmind log` invocation (TestLog_DocsMissingErrors shows
// log refuses to run against an unscaffolded repo).
func initClonePairScaffolded(t *testing.T) (origin, repo string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping integration test")
	}
	origin = t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	initLogTestGitRepo(t, origin)
	if err := os.Chdir(origin); err != nil {
		t.Fatalf("Chdir origin: %v", err)
	}
	scaffoldDocs(t)
	if err := os.Chdir(origWD); err != nil {
		t.Fatalf("Chdir back: %v", err)
	}
	commitAll(t, origin, "scaffold")

	repo = filepath.Join(t.TempDir(), "repo")
	// A clone does NOT inherit origin's gc.auto/maintenance.auto (local,
	// per-repo config `git clone` doesn't copy) — see testgit's package doc.
	testgit.CloneRepo(t, repo, "-q", origin)
	runGitIn(t, repo, "config", "user.email", "test@example.com")
	runGitIn(t, repo, "config", "user.name", "Test")
	runGitIn(t, repo, "config", "commit.gpgsign", "false")
	return origin, repo
}

// TestLog_Pulse_MainDecisionsLine_EndToEnd wires mainDecisionsPulseLine
// through the real `logmind log` command on a stale feature branch: the
// exact advisory string lands on stderr, positioned AFTER the drift/spec
// pulses (none fire here, but the ordering is emitPulse's contract) and the
// §3.1 stdout contract is untouched.
func TestLog_Pulse_MainDecisionsLine_EndToEnd(t *testing.T) {
	origin, repo := initClonePairScaffolded(t)
	commitOn(t, origin, "docs/decisions.md", "## upstream decision\n")
	runGitIn(t, repo, "fetch", "origin", "main")
	runGitIn(t, repo, "checkout", "-b", "feat/pulse-e2e")

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("Chdir repo: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	withFakeTTY(t, false, func() {
		root := NewRootCmd()
		root.SetArgs([]string{"log", "branch decision", "-r", "why", "--no-push", "--no-interactive"})
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		if err := root.Execute(); err != nil {
			t.Fatalf("log: %v\nstderr:\n%s", err, errBuf.String())
		}
		want := "logmind: main has 1 new decision commit — run 'logmind warp' to catch up"
		if !strings.Contains(errBuf.String(), want) {
			t.Fatalf("stderr missing the main-decisions pulse line; got %q", errBuf.String())
		}
		// The §3.1 stdout contract must stay untouched by the new probe.
		if strings.Contains(out.String(), "logmind:") {
			t.Fatalf("pulse output leaked onto stdout: %q", out.String())
		}
	})
}
