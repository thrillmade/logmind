// auto_test.go — exercises `logmind auto <profile>` end to end.
//
// Everything here is pinned on what a user observes: the bytes on stdout
// and stderr, the exit code, and the set of paths the run left behind.
// Nothing asserts on a helper in auto.go — the regressions worth catching
// (a fallback profile, a silently-rewritten directive, a command that
// starts the mode) all show up in exactly those three places.
package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// runAutoCmd drives `logmind auto <args...>` in the current cwd and
// returns (stdout, stderr, err).
func runAutoCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(append([]string{"auto"}, args...))
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// treeSnapshot maps every regular file under dir to a hash of its
// contents, plus every directory to "". Comparing two snapshots answers
// both "did anything change" and "did anything appear".
func treeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			snap[filepath.ToSlash(rel)+"/"] = ""
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		snap[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return snap
}

func snapshotDiff(before, after map[string]string) (added, changed, removed []string) {
	for path, sum := range after {
		old, ok := before[path]
		switch {
		case !ok:
			added = append(added, path)
		case old != sum:
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			removed = append(removed, path)
		}
	}
	sort.Strings(added)
	sort.Strings(changed)
	sort.Strings(removed)
	return added, changed, removed
}

// TestAuto_UnknownProfileRefusesAndNamesTheKnownOnes — the anti-fallback
// guard at the CLI surface: exit non-zero, say what is known, and write
// nothing. A setup verb that quietly configured a different autonomy
// policy than the one asked for is the same defect as #267 and #286.
func TestAuto_UnknownProfileRefusesAndNamesTheKnownOnes(t *testing.T) {
	for _, name := range []string{"skdd", "night", "nonsense"} {
		t.Run(name, func(t *testing.T) {
			dir := withTempCwd(t, func(d string) {
				out, errOut, err := runAutoCmd(t, name)
				if !errors.Is(err, ErrSilent) {
					t.Fatalf("err = %v; want ErrSilent (exit 1)\nstdout:%s\nstderr:%s", err, out, errOut)
				}
				mustContain(t, errOut, "unknown profile")
				mustContain(t, errOut, name)
				mustContain(t, errOut, "Known profiles: unattended")
				// SPEC §2.7: no `ok` receipt on a non-zero exit.
				if strings.Contains(out, "ok auto") {
					t.Errorf("a refused run printed an ok receipt:\n%s", out)
				}
			})
			if _, err := os.Stat(filepath.Join(dir, ".logmind", "auto.yml")); err == nil {
				t.Errorf("a refused profile still wrote .logmind/auto.yml")
			}
		})
	}
}

// TestAuto_RetiredNameExplainsTheRename — `night` is refused like any
// unknown name, but the refusal teaches the vocabulary the policy
// actually uses. It must never resolve.
func TestAuto_RetiredNameExplainsTheRename(t *testing.T) {
	withTempCwd(t, func(_ string) {
		_, errOut, err := runAutoCmd(t, "night")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v; want ErrSilent", err)
		}
		mustContain(t, errOut, "renamed `unattended-operation`")
		mustContain(t, errOut, "never by the clock")
	})
}

// TestAuto_KnownProfileIsIdempotent — the second run must change nothing
// on disk and must say so rather than reporting a fresh install.
func TestAuto_KnownProfileIsIdempotent(t *testing.T) {
	withTempCwd(t, func(dir string) {
		mustWriteUnder(t, dir, "docs/plan.md", "# Plan\n")

		out1, errOut1, err := runAutoCmd(t, "unattended")
		if err != nil {
			t.Fatalf("first run: %v\n%s", err, errOut1)
		}
		mustContain(t, out1, "✓ Created .logmind/auto.yml")
		mustContain(t, out1, "directive=created")
		afterFirst := treeSnapshot(t, dir)

		out2, errOut2, err := runAutoCmd(t, "unattended")
		if err != nil {
			t.Fatalf("second run: %v\n%s", err, errOut2)
		}
		afterSecond := treeSnapshot(t, dir)

		added, changed, removed := snapshotDiff(afterFirst, afterSecond)
		if len(added)+len(changed)+len(removed) != 0 {
			t.Errorf("second run was not a no-op: added=%v changed=%v removed=%v", added, changed, removed)
		}
		mustContain(t, out2, "already current")
		mustContain(t, out2, "directive=current")
		if strings.Contains(out2, "✓ Created") {
			t.Errorf("second run reported a fresh install:\n%s", out2)
		}
	})
}

// TestAuto_PrintsTheHandoverAndStartsNothing is the central ruling:
// `logmind auto` sets a repository up and hands the invocation to a
// human. unattended-operation begins only from an explicit handover, so a
// tool that started it would break the policy it had just installed.
//
// "Starts nothing" is pinned on the file system, not on an implementation
// detail: after the run the ONLY new paths under the repo are the
// directive and its parent. Anything that launched a loop — a scheduler
// lock, a checkpoint, a log — would land here and fail this.
func TestAuto_PrintsTheHandoverAndStartsNothing(t *testing.T) {
	withTempCwd(t, func(dir string) {
		mustWriteUnder(t, dir, "docs/plan.md", "# Plan\n")
		before := treeSnapshot(t, dir)

		out, errOut, err := runAutoCmd(t, "unattended")
		if err != nil {
			t.Fatalf("auto: %v\n%s", err, errOut)
		}

		// The handover is PRINTED, and says it is not being performed.
		mustContain(t, out, "logmind does NOT start unattended operation")
		mustContain(t, out, "never from a clock, a scheduled wake, or a tool")
		// All five slots unattended-operation requires a handover to name.
		mustContain(t, out, "scope:")
		mustContain(t, out, "hard stops:")
		mustContain(t, out, "you may:")
		mustContain(t, out, "wake:")
		mustContain(t, out, "at a fork:")
		// The checkpoint the resumed session will read.
		mustContain(t, out, "park work in docs/plan.md")

		added, changed, removed := snapshotDiff(before, treeSnapshot(t, dir))
		wantAdded := []string{".logmind/", ".logmind/auto.yml"}
		if strings.Join(added, ",") != strings.Join(wantAdded, ",") {
			t.Errorf("auto created %v; want exactly %v — anything else means it started something", added, wantAdded)
		}
		if len(changed) != 0 || len(removed) != 0 {
			t.Errorf("auto mutated existing paths: changed=%v removed=%v", changed, removed)
		}
	})
}

// TestAuto_DirectiveOnDiskCarriesTheSkillRules — the file an agent will
// actually read, not the template constant.
func TestAuto_DirectiveOnDiskCarriesTheSkillRules(t *testing.T) {
	withTempCwd(t, func(dir string) {
		mustWriteUnder(t, dir, "docs/plan.md", "# Plan\n")
		if _, errOut, err := runAutoCmd(t, "unattended"); err != nil {
			t.Fatalf("auto: %v\n%s", err, errOut)
		}
		body := readFileStr(t, filepath.Join(dir, ".logmind", "auto.yml"))

		for _, want := range []string{
			"# logmind-auto-version:",                             // SPEC §5.2 ownership marker
			"profile: unattended",                                 // one owner for the profile name
			"requires_human_handover: true",                       // unattended-operation: entry
			"the ceiling, minus the cost of the largest dispatch", // session-heartbeat: threshold
			"path: docs/plan.md",                                  // session-heartbeat: checkpoint
			"a sha, never a branch name",                          // session-heartbeat: checkpoint slots
			"can it be undone before they are back",               // unattended-operation: the test
			"pushing to a shared ref",                             // unattended-operation: far side
			"the second consecutive failure of the same fix",      // unattended-operation: hard stops
			"where the work stands now — one line",                // unattended-operation: digest
			"must_be_git_ignored: true",                           // unattended-operation: scheduler state
			"skills/session-heartbeat",                            // the owning skill, linked
			"skills/unattended-operation",                         // the owning skill, linked
		} {
			mustContain(t, body, want)
		}
	})
}

// TestAuto_ReportsMissingSkillsWithoutFetchingThem — logmind does not pull
// catalog items (§5.2's subscription model is Planned at skdd#6). It says
// which are absent and prints the command a human runs.
func TestAuto_ReportsMissingSkillsWithoutFetchingThem(t *testing.T) {
	withTempCwd(t, func(dir string) {
		mustWriteUnder(t, dir, ".claude/skills/session-heartbeat/SKILL.md", "---\nname: session-heartbeat\n---\n")

		out, errOut, err := runAutoCmd(t, "unattended")
		if err != nil {
			t.Fatalf("auto: %v\n%s", err, errOut)
		}
		mustContain(t, out, "✓ session-heartbeat")
		mustContain(t, out, "✗ unattended-operation — not installed")
		mustContain(t, out, "npx skills add https://github.com/thrillmade/agent-skills --skill unattended-operation")
		mustContain(t, out, "skills-missing=1")

		if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "unattended-operation")); err == nil {
			t.Errorf("auto fetched a skill; it must only print the install command")
		}
	})
}

// TestAuto_MissingPlanDocIsReportedNotCreated — a checkpoint with nowhere
// to land is the difference between a resumed session telling "paused"
// from "died". What a plan doc should SAY is a judgment call, so auto
// reports the absence rather than inventing one (the same boundary
// `doctor --fix` draws for docs/spec.md).
func TestAuto_MissingPlanDocIsReportedNotCreated(t *testing.T) {
	withTempCwd(t, func(dir string) {
		_, errOut, err := runAutoCmd(t, "unattended")
		if err != nil {
			t.Fatalf("auto: %v\n%s", err, errOut)
		}
		mustContain(t, errOut, "docs/plan.md does not exist yet")
		if _, statErr := os.Stat(filepath.Join(dir, "docs", "plan.md")); statErr == nil {
			t.Errorf("auto created the plan doc; it must report the absence instead")
		}
	})
}

// TestAuto_RefusesToRewriteADirectiveItDoesNotOwn — the file carries
// policy a human authored (repo hard stops, the wake mechanism). Every
// decline is REPORTED on stderr; silence would leave the operator
// believing the setup they just ran is in force.
func TestAuto_RefusesToRewriteADirectiveItDoesNotOwn(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		wantNote string
		wantOK   string
	}{
		{
			name:     "markerless",
			content:  "profile: unattended\nhard_stops:\n  repo: [never touch prod]\n",
			wantNote: "belongs to you",
			wantOK:   "directive=declined-markerless",
		},
		{
			name:     "another profile",
			content:  "# logmind-auto-version: v1\nprofile: skdd\n",
			wantNote: "refusing to overwrite it with",
			wantOK:   "directive=declined-other-profile",
		},
		{
			name:     "newer than this binary",
			content:  "# logmind-auto-version: v99\nprofile: unattended\n",
			wantNote: "refusing to downgrade",
			wantOK:   "directive=declined-newer",
		},
		{
			name:     "older than this binary",
			content:  "# logmind-auto-version: v0\nprofile: unattended\n",
			wantNote: "left unchanged, because it carries policy you authored",
			wantOK:   "directive=declined-stale",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withTempCwd(t, func(dir string) {
				path := filepath.Join(dir, ".logmind", "auto.yml")
				mustWriteUnder(t, dir, ".logmind/auto.yml", c.content)

				out, errOut, err := runAutoCmd(t, "unattended")
				if err != nil {
					t.Fatalf("auto: %v\n%s", err, errOut)
				}
				mustContain(t, errOut, c.wantNote)
				mustContain(t, out, c.wantOK)
				if got := readFileStr(t, path); got != c.content {
					t.Errorf("auto rewrote a directive it does not own:\n got: %q\nwant: %q", got, c.content)
				}
			})
		})
	}
}

// TestAuto_QuietEmitsOnlyTheReceipt — §2.7's quiet discipline: one
// chainable `ok <k=v>` line, no chatter.
func TestAuto_QuietEmitsOnlyTheReceipt(t *testing.T) {
	withTempCwd(t, func(dir string) {
		mustWriteUnder(t, dir, "docs/plan.md", "# Plan\n")
		out, errOut, err := runAutoCmd(t, "unattended", "--quiet")
		if err != nil {
			t.Fatalf("auto --quiet: %v\n%s", err, errOut)
		}
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 1 {
			t.Fatalf("quiet stdout = %d lines; want exactly 1:\n%s", len(lines), out)
		}
		mustContain(t, lines[0], "ok auto profile=unattended directive=created")
		mustContain(t, lines[0], "checkpoint=docs/plan.md")
	})
}
