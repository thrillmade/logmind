// pulse_hotpath_test.go — subprocess-level proofs for the pulse hot-path
// hardening (adversarial-review findings, items 1 and 2 of the pulse fix):
//
//   - item 1 (hang-proof): `logmind log`'s drift pulse must never touch the
//     subprocess-bearing PATH-resolution probe. Proven end-to-end against
//     the REAL built binary with a hostile `logmind` planted FIRST on
//     PATH — one variant that just sleeps, and a "daemonizing wrapper"
//     variant that backgrounds a sleeping grandchild and exits immediately
//     itself (the shape that defeats a naive context-timeout-only bound;
//     see internal/doctor/doctor_test.go's WaitDelay tests for the
//     on-demand `doctor` path's version of this same fix). Both must
//     complete in well under the ctx timeout doctor's PATH probe would
//     have used (5s) — comfortably inside 3s — because the pulse's
//     drift-count call (doctor.StaleCountFast) never invokes that probe.
//
//   - item 2 (TZ skew): the spec pulse compares decisions.Iter-parsed
//     entry dates against gitcli.LastCommitTime's real instant.
//     decisions.Iter parses `## YYYY-MM-DD HH:MM` headers with a zoneless
//     time.Parse (Go labels these UTC by default), but those headers are
//     LOCAL wall-clock time by construction — log.go's buildDecisionEntry
//     writes time.Now().Format("2006-01-02 15:04"). Comparing the
//     UTC-mislabeled value directly against a real instant skews the
//     comparison by the full local UTC offset: a false fire in
//     positive-offset zones, a missed fire in negative-offset zones.
//     Reproducing this needs a process whose time.Local actually reflects
//     a target zone — Go resolves time.Local once, at process
//     initialization, from the TZ environment variable, and setting
//     TZ inside an already-running test process does NOT reload it. Hence
//     real `go build` + subprocess invocations with TZ set on the
//     child's environment, not t.Setenv in-process.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// requireHotpathSubprocessTools skips the calling test when the go
// toolchain or git aren't available, or when -short was requested — same
// gating convention as guard_commit_test.go / version_test.go's subprocess
// tests. Returns the resolved `go` binary path.
func requireHotpathSubprocessTools(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping subprocess test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; skipping subprocess test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping subprocess test")
	}
	if runtime.GOOS == "windows" {
		t.Skip("hostile-PATH-binary fixtures below are POSIX shell scripts")
	}
	return goBin
}

// prependPathEnv builds a subprocess environment with dir placed FIRST on
// PATH (the rest of the real PATH follows). A single PATH entry is
// synthesized (rather than appending a second PATH= to os.Environ()) so
// there's no ambiguity about which one a libc getenv-style lookup honors
// inside the child.
func prependPathEnv(dir string) []string {
	out := []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") {
			out = append(out, kv)
		}
	}
	return out
}

// TestLogBinary_HangProof_HostilePathBinary is the release-bar proof for
// item 1: with the REAL built `logmind` binary, `logmind log
// --no-interactive --no-push` must complete in well under 15s even when a
// hostile `logmind` shell script sits FIRST on PATH — whether it just
// hangs, or daemonizes (forks a sleeping grandchild and exits). The pulse
// must ALSO still be functional: a stale post-merge hook (a file-read-only
// probe) must still surface a drift line, proving the fix didn't
// accidentally disable the whole pulse rather than just its
// subprocess-bearing probe.
func TestLogBinary_HangProof_HostilePathBinary(t *testing.T) {
	goBin := requireHotpathSubprocessTools(t)
	binPath := buildGuardCommitBinary(t, goBin)

	scenarios := []struct {
		name string
		body string
	}{
		{
			name: "hard sleep",
			body: "#!/bin/sh\nsleep 30\n",
		},
		{
			name: "daemonizing wrapper (grandchild sleeps, wrapper exits immediately)",
			body: "#!/bin/sh\n(sleep 30 &)\nexit 0\n",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			repo := t.TempDir()
			initLogTestGitRepo(t, repo)

			initCmd := exec.Command(binPath, "init", "--no-git")
			initCmd.Dir = repo
			if out, err := initCmd.CombinedOutput(); err != nil {
				t.Fatalf("logmind init --no-git: %v\n%s", err, out)
			}
			writeStaleHook(t, repo) // stale post-merge hook — proves file-read probes still run

			fakeBinDir := t.TempDir()
			fake := filepath.Join(fakeBinDir, "logmind")
			if err := os.WriteFile(fake, []byte(sc.body), 0o755); err != nil {
				t.Fatalf("write hostile logmind: %v", err)
			}

			cmd := exec.Command(binPath, "log", "hang proof test", "-r", "why", "--no-interactive", "--no-push")
			cmd.Dir = repo
			cmd.Env = prependPathEnv(fakeBinDir)
			cmd.Stdin = strings.NewReader("")
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			start := time.Now()
			runErr := cmd.Run()
			elapsed := time.Since(start)

			// Bound is deliberately well BELOW the 30s hostile sleep (so a
			// regression that shells the PATH `logmind` still trips it) yet
			// loose enough not to flake on a saturated CI host, where a
			// correct run's file-only work can still take a few seconds.
			if elapsed > 15*time.Second {
				t.Fatalf("logmind log took %v with a hostile logmind FIRST on PATH (%s); want < 15s (the hostile sleep is 30s) — the pulse must never touch the PATH-resolution subprocess.\nstdout=%q\nstderr=%q",
					elapsed, sc.name, stdout.String(), stderr.String())
			}
			if runErr != nil {
				t.Fatalf("logmind log failed (%v): %v\nstdout=%s\nstderr=%s", sc.name, runErr, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "component") || !strings.Contains(stderr.String(), "stale") {
				t.Fatalf("expected a drift pulse line (proving the file-read probes still ran) on stderr; got stdout=%q stderr=%q",
					stdout.String(), stderr.String())
			}
		})
	}
}

// TestLogBinary_TZSkew_SpecPulse is the release-bar proof for item 2: the
// spec pulse's local-time comparison, exercised through the REAL built
// binary with TZ set on the subprocess environment (see file docstring for
// why this can't be an in-process t.Setenv test).
func TestLogBinary_TZSkew_SpecPulse(t *testing.T) {
	goBin := requireHotpathSubprocessTools(t)
	binPath := buildGuardCommitBinary(t, goBin)

	t.Run("UTC+12: decisions 2h BEFORE the spec commit must stay silent", func(t *testing.T) {
		loc := mustLoadTZLocation(t, "Pacific/Auckland")
		// NZ winter (June): NZST, a fixed UTC+12 — no DST straddling.
		specInstant := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		decisionInstant := specInstant.Add(-2 * time.Hour) // genuinely BEFORE the commit
		runTZSkewSpecPulseCase(t, binPath, "Pacific/Auckland", loc, specInstant, decisionInstant, false)
	})

	t.Run("UTC-7: decisions 2h AFTER the spec commit must fire", func(t *testing.T) {
		loc := mustLoadTZLocation(t, "America/Los_Angeles")
		// CA summer (July): PDT, a fixed UTC-7 — no DST straddling.
		specInstant := time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC)
		decisionInstant := specInstant.Add(2 * time.Hour) // genuinely AFTER the commit
		runTZSkewSpecPulseCase(t, binPath, "America/Los_Angeles", loc, specInstant, decisionInstant, true)
	})
}

func mustLoadTZLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("tzdata for %s unavailable on this host: %v", name, err)
	}
	return loc
}

// runTZSkewSpecPulseCase builds a repo with a spec file committed at
// specInstant (a real, TZ-independent instant) and specPulseThreshold
// decision entries whose headers carry the LOCAL wall-clock rendering of
// decisionInstant in `loc` — exactly what log.go's buildDecisionEntry would
// have written had `time.Now()` returned decisionInstant while the process
// ran under `loc`. It then runs the real `logmind log` binary with TZ=tz on
// its environment and asserts the spec pulse fires (or doesn't) per
// wantFire — the ground truth being decisionInstant.After(specInstant),
// which is independent of any timezone.
func runTZSkewSpecPulseCase(t *testing.T, binPath, tz string, loc *time.Location, specInstant, decisionInstant time.Time, wantFire bool) {
	t.Helper()
	repo := t.TempDir()
	initLogTestGitRepo(t, repo)

	initCmd := exec.Command(binPath, "init", "--no-git")
	initCmd.Dir = repo
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("logmind init --no-git: %v\n%s", err, out)
	}

	mustWriteUnder(t, repo, ".logmind/config.yml", "context:\n  spec_file: SPEC.md\n")
	commitFileWithDate(t, repo, "SPEC.md", "# Spec\n", specInstant.UTC().Format(time.RFC3339))

	// Overwrite the init-scaffolded decisions.md (which carries init's own
	// "first decision", logged at real current wall-clock time and
	// irrelevant to this reproduction) with `specPulseThreshold` entries,
	// all sharing the SAME contrived wall-clock header.
	wallClock := decisionInstant.In(loc).Format("2006-01-02 15:04")
	var b strings.Builder
	b.WriteString("# Decisions\n\n")
	for i := 0; i < specPulseThreshold; i++ {
		fmt.Fprintf(&b, "## %s - tz skew filler decision %d\n\n**Reasoning:** filler\n\n---\n\n", wallClock, i)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "decisions.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("overwrite decisions.md: %v", err)
	}

	cmd := exec.Command(binPath, "log", "tz skew probe", "-r", "why", "--no-commit", "--no-interactive")
	cmd.Dir = repo
	cmd.Env = envWithTZ(tz)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("logmind log: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	fired := strings.Contains(stderr.String(), "logmind: SPEC.md unchanged for")
	if fired != wantFire {
		t.Fatalf("TZ=%s: spec pulse fired=%v; want %v (ground truth: decisionInstant %s spec commit's instant).\nspecInstant=%s decisionInstant=%s wallClock(in %s)=%q\nstderr=%q",
			tz, fired, wantFire, map[bool]string{true: "IS after", false: "is NOT after"}[wantFire],
			specInstant, decisionInstant, tz, wallClock, stderr.String())
	}
}

// envWithTZ returns a subprocess environment identical to the current
// process's, except any existing TZ is dropped and replaced with a single
// "TZ=tz" entry — avoiding the ambiguity of two TZ= entries in Env (getenv
// implementations conventionally honor the FIRST match, so a naive append
// after os.Environ() risks being silently ignored if TZ is already set).
func envWithTZ(tz string) []string {
	out := []string{"TZ=" + tz}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "TZ=") {
			out = append(out, kv)
		}
	}
	return out
}
