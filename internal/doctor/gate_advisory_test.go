// gate_advisory_test.go — the other half of SPEC §1.6's sentence. The command
// refuses an agent-initiated weakening; nothing local can stop an agent
// editing the file, so doctor reports a blocking setting it finds already
// weakened. Advisory only: a person is allowed to have turned it off, so
// Overall must stay OK.
package doctor

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGateAdvisories_CleanRepoSaysNothing(t *testing.T) {
	dir := freshRepo(t)
	r := CollectStatus(dir, true)
	if len(r.GateAdvisories) != 0 {
		t.Errorf("GateAdvisories = %v; want none (nothing weakened)", r.GateAdvisories)
	}
}

// Control for the test above: the same probe DOES fire when the file says so.
func TestGateAdvisories_HandEditedConfigYML(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"),
		"git:\n  auto_commit: true\n  enforce_commits: false\n")
	r := CollectStatus(dir, true)
	if len(r.GateAdvisories) != 1 {
		t.Fatalf("GateAdvisories = %v; want exactly one", r.GateAdvisories)
	}
	got := r.GateAdvisories[0]
	for _, needle := range []string{
		"git.enforce_commits",
		".logmind/config.yml",
		"whether a substantive commit must carry a decision",
		"the weakened value",
		"logmind config set git.enforce_commits true",
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("advisory missing %q:\n%s", needle, got)
		}
	}
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; a weakened gate is a person's choice to report, not drift")
	}
}

// The two review keys live in .clud-bug.json per §1.6, and a hand edit there
// is the same defeat.
func TestGateAdvisories_HandEditedCludBugJSON(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "strict_mode off",
			body: `{"review": {"strict_mode": false}}`,
			want: []string{"review.strict_mode", ".claude/skills/.clud-bug.json",
				"whether a critical review finding blocks the merge"},
		},
		{
			name: "auto_fix on",
			body: `{"review": {"auto_fix": true}}`,
			want: []string{"review.auto_fix", "whether a reviewer may push a fix without a person",
				"clud-bug's to write, not logmind's"},
		},
		{
			name: "auto_fix given a round count",
			body: `{"review": {"auto_fix": 3}}`,
			want: []string{"review.auto_fix"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := freshRepo(t)
			mustWrite(t, filepath.Join(dir, ".claude", "skills", ".clud-bug.json"), c.body)
			r := CollectStatus(dir, true)
			if len(r.GateAdvisories) != 1 {
				t.Fatalf("GateAdvisories = %v; want exactly one", r.GateAdvisories)
			}
			for _, needle := range c.want {
				if !strings.Contains(r.GateAdvisories[0], needle) {
					t.Errorf("advisory missing %q:\n%s", needle, r.GateAdvisories[0])
				}
			}
			if r.Overall == "DRIFT" {
				t.Errorf("Overall = DRIFT; the gate advisory must never be drift")
			}
		})
	}
}

// Controls: the strengthened values, and a .clud-bug.json that configures
// something else entirely, must say nothing. Over-reporting trains a reader to
// ignore the list.
func TestGateAdvisories_StrengthenedAndUnrelatedStaySilent(t *testing.T) {
	cases := []struct{ name, path, body string }{
		{"enforce_commits on", filepath.Join(".logmind", "config.yml"),
			"git:\n  enforce_commits: true\n"},
		{"strict_mode on", filepath.Join(".claude", "skills", ".clud-bug.json"),
			`{"review": {"strict_mode": true}}`},
		{"auto_fix off", filepath.Join(".claude", "skills", ".clud-bug.json"),
			`{"review": {"auto_fix": false}}`},
		{"unrelated review keys", filepath.Join(".claude", "skills", ".clud-bug.json"),
			`{"review": {"trigger": "pre-push", "ci_checks": ["build"]}, "installed": ["x"]}`},
		{"unparseable json", filepath.Join(".claude", "skills", ".clud-bug.json"), `{not json`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := freshRepo(t)
			mustWrite(t, filepath.Join(dir, c.path), c.body)
			r := CollectStatus(dir, true)
			if len(r.GateAdvisories) != 0 {
				t.Errorf("GateAdvisories = %v; want none", r.GateAdvisories)
			}
		})
	}
}

// All three at once, so a reader who defeated every gate is told about every
// gate rather than the first one.
func TestGateAdvisories_ReportsAllThree(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"),
		"git:\n  enforce_commits: false\n")
	mustWrite(t, filepath.Join(dir, ".claude", "skills", ".clud-bug.json"),
		`{"review": {"strict_mode": false, "auto_fix": true}}`)
	r := CollectStatus(dir, true)
	if len(r.GateAdvisories) != 3 {
		t.Fatalf("GateAdvisories = %v; want three", r.GateAdvisories)
	}
	joined := strings.Join(r.GateAdvisories, "\n")
	for _, key := range []string{"git.enforce_commits", "review.strict_mode", "review.auto_fix"} {
		if !strings.Contains(joined, key) {
			t.Errorf("missing %s:\n%s", key, joined)
		}
	}
	if !strings.Contains(RenderStatus(r), "Blocking settings") {
		t.Errorf("RenderStatus does not surface the gate advisories:\n%s", RenderStatus(r))
	}
}
