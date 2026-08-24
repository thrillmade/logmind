// gate_function_test.go — an enforcement surface is ABSENT when it cannot
// enforce, not only when the file is gone. Companion to
// gate_absence_test.go, which covers the deleted-file half.
//
// Three states measured on the release candidate, every one reporting
// `Stack status: OK`, `gate_absences: null`, exit 0:
//
//	: > .git/hooks/commit-msg                  → 30-line raw commit through
//	chmod -x .git/hooks/commit-msg             → 30-line raw commit through
//	delete check-decisions.yml's jobs: block   → §6.2 merge gate defanged
//
// plus a fourth, which is the worst of them because the tool causes it:
// with core.hooksPath set, doctor reported a working hook missing, and its
// own `--fix` then wrote a replacement to .git/hooks — a path git never
// reads — after which doctor reported OK over a repository with no gate.
package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/templates"
)

// TestGateAbsences_InertCommitMsgHookIsReported — present, and enforcing
// nothing. Each case carries the CONTROL in the same run: the untouched
// hook must be silent, or a check that reports everything scores the same.
func TestGateAbsences_InertCommitMsgHookIsReported(t *testing.T) {
	cases := []struct {
		name string
		// break_ turns the installed hook into an inert one.
		break_ func(t *testing.T, path string)
		want   string
	}{
		{
			name: "emptied",
			break_: func(t *testing.T, path string) {
				mustWrite(t, path, "")
				if err := os.Chmod(path, 0o755); err != nil {
					t.Fatalf("chmod: %v", err)
				}
			},
			want: "no longer runs",
		},
		{
			name: "replaced with exit 0",
			break_: func(t *testing.T, path string) {
				mustWrite(t, path, "#!/bin/sh\n# logmind commit-msg hook\nexit 0\n")
				if err := os.Chmod(path, 0o755); err != nil {
					t.Fatalf("chmod: %v", err)
				}
			},
			want: "no longer runs",
		},
		{
			name: "execute bit cleared",
			break_: func(t *testing.T, path string) {
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatalf("chmod: %v", err)
				}
			},
			want: "not executable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := gatedRepo(t)
			path := filepath.Join(hooksDirOf(t, dir), "commit-msg")

			// CONTROL: as installed, nothing is reported.
			if r := CollectStatus(dir, true); len(r.GateAbsences) != 0 {
				t.Fatalf("control: GateAbsences = %v; want none before the hook is broken", r.GateAbsences)
			}

			tc.break_(t, path)
			r := CollectStatus(dir, true)
			if len(r.GateAbsences) != 1 || !strings.Contains(r.GateAbsences[0], tc.want) {
				t.Fatalf("GateAbsences = %v; want one row saying %q — the hook is present and stops nothing",
					r.GateAbsences, tc.want)
			}
			if !strings.Contains(r.GateAbsences[0], "commit-msg") {
				t.Errorf("GateAbsences = %v; the row must name the file", r.GateAbsences)
			}
			if r.Overall != "DRIFT" {
				t.Errorf("Overall = %q; want DRIFT — SPEC §3.4: failing open MUST NOT be silent", r.Overall)
			}
		})
	}
}

// TestGateAbsences_DefangedMergeGateIsReported — the §6.2 gate with its
// marker intact and its job gone. probeWorkflow's verdict comes from the
// marker alone, so this reported `drift: current` and nothing else.
//
// The last case is the FENCE against over-reporting, and it is not
// hypothetical: an earlier draft required the job to invoke `logmind
// check-decisions`, and logmind's own repository — whose installed copy is
// the pre-v5 bash gate that trips without calling the verb — was reported
// as judging no pull request at all. It does judge; it judges by a second
// list. That is a different fault and this check does not claim it.
func TestGateAbsences_DefangedMergeGateIsReported(t *testing.T) {
	full := templates.Workflow("check-decisions.yml.template")

	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "jobs block deleted, marker intact",
			content: full[:strings.Index(full, "\njobs:")+1],
			want:    "defines no step that runs anything",
		},
		{
			name:    "no pull-request trigger",
			content: strings.ReplaceAll(full, "pull_request_target:", "workflow_dispatch:"),
			want:    "subscribes to no `pull_request` event",
		},
		{
			name:    "customised but still a gate",
			content: strings.ReplaceAll(full, "logmind check-decisions", "./scripts/our-own-gate"),
			want:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := gatedRepo(t)
			path := filepath.Join(dir, ".github", "workflows", "check-decisions.yml")

			if r := CollectStatus(dir, true); len(r.GateAbsences) != 0 {
				t.Fatalf("control: GateAbsences = %v; want none before the workflow is edited", r.GateAbsences)
			}
			// The premise of every case: line 1 still carries the marker
			// probeWorkflow judges by, so only a functional check can catch
			// anything here.
			if !strings.HasPrefix(tc.content, "# logmind-template-version:") {
				t.Fatalf("fixture lost the line-1 marker, so this case no longer tests what it claims")
			}
			mustWrite(t, path, tc.content)

			r := CollectStatus(dir, true)
			if tc.want == "" {
				if len(r.GateAbsences) != 0 {
					t.Fatalf("GateAbsences = %v; want none — a repository may run its own gate on "+
						"a pull request, and logmind cannot read whether a job enforces", r.GateAbsences)
				}
				return
			}
			if len(r.GateAbsences) != 1 || !strings.Contains(r.GateAbsences[0], tc.want) {
				t.Fatalf("GateAbsences = %v; want one row saying %q", r.GateAbsences, tc.want)
			}
			if r.Overall != "DRIFT" {
				t.Errorf("Overall = %q; want DRIFT", r.Overall)
			}
		})
	}
}

// TestHooksPath_ProbeAndInstallerAgreeWithGit — core.hooksPath, both
// halves. The probe must SEE a hook where git reads it, and the installer
// must WRITE there; a tool whose reader and writer disagree with git about
// one path is how `doctor --fix` came to manufacture an OK over a
// repository with no gate at all.
func TestHooksPath_ProbeAndInstallerAgreeWithGit(t *testing.T) {
	dir := gatedRepo(t)

	// Relocate, and MOVE the working hook the way a person would.
	relocated := filepath.Join(dir, ".githooks")
	if err := os.MkdirAll(relocated, 0o755); err != nil {
		t.Fatalf("mkdir .githooks: %v", err)
	}
	gitInRepo(t, dir, "config", "core.hooksPath", ".githooks")
	installed := filepath.Join(dir, ".git", "hooks", "commit-msg")
	body, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("read installed hook: %v", err)
	}
	if err := os.WriteFile(filepath.Join(relocated, "commit-msg"), body, 0o755); err != nil {
		t.Fatalf("write relocated hook: %v", err)
	}
	if err := os.Remove(installed); err != nil {
		t.Fatalf("remove old hook: %v", err)
	}

	// Precondition: git really reads .githooks now.
	if got := hooksDirOf(t, dir); filepath.Clean(got) != filepath.Clean(relocated) {
		t.Fatalf("hooks.Dir = %q; want %q — the fixture did not relocate anything", got, relocated)
	}

	// READER: no false absence over a hook that is installed and running.
	if r := CollectStatus(dir, true); len(r.GateAbsences) != 0 {
		t.Fatalf("GateAbsences = %v; want none — the commit-msg hook is in the directory "+
			"git reads and fires normally", r.GateAbsences)
	}

	// WRITER: --fix's install path must not resurrect the unread location.
	if err := os.Remove(filepath.Join(relocated, "commit-msg")); err != nil {
		t.Fatalf("remove relocated hook: %v", err)
	}
	changed, err := hooks.InstallCommitMsg(dir)
	if err != nil || !changed {
		t.Fatalf("InstallCommitMsg = %v, %v; want (true, nil)", changed, err)
	}
	if _, err := os.Stat(filepath.Join(relocated, "commit-msg")); err != nil {
		t.Errorf("the hook was not written to %s, the directory git reads: %v", relocated, err)
	}
	if _, err := os.Stat(installed); err == nil {
		t.Errorf("a hook was written to .git/hooks/commit-msg, which this repository never reads — " +
			"that is the write that manufactured a false OK")
	}
}

// TestGateAbsences_SerializeAsAListWhenEmpty — `gate_absences` is the one
// list on StatusReport whose emptiness a reader branches on (it is the only
// one that moves `overall` and therefore the exit code), so it must not
// arrive as `null` on a healthy repository and `[]` on a broken one.
func TestGateAbsences_SerializeAsAListWhenEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		repo func(t *testing.T) string
	}{
		{"fully installed", gatedRepo},
		{"never initialised", func(t *testing.T) string {
			dir := t.TempDir()
			isolatePathHermetic(t)
			realGitDir(t, dir)
			return dir
		}},
		{"not a git repository", freshRepo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.repo(t)
			r := CollectStatus(dir, true)
			if len(r.GateAbsences) != 0 {
				t.Fatalf("GateAbsences = %v; this case is supposed to have none", r.GateAbsences)
			}
			js, err := r.ToJSON()
			if err != nil {
				t.Fatalf("ToJSON: %v", err)
			}
			if !strings.Contains(js, `"gate_absences": []`) {
				t.Errorf("gate_absences did not serialize as an empty list; got:\n%s",
					gateAbsencesLine(js))
			}
		})
	}
}

func gateAbsencesLine(js string) string {
	for _, line := range strings.Split(js, "\n") {
		if strings.Contains(line, "gate_absences") {
			return line
		}
	}
	return "(no gate_absences key at all)"
}
