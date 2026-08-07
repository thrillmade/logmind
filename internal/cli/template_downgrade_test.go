// template_downgrade_test.go — logmind#286: a refresh must never walk a
// workflow template BACKWARDS. The pre-#286 code tested marker INEQUALITY
// ("different" == "stale"), so a released binary bundling v4 silently
// overwrote a repo already carrying v11 and reported it as
// "↻ Refreshed … to current template".
package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/templates"
)

// TestParseTemplateVersion covers the ordering key: bare vN, the -pointer
// flavour suffix, the -FAKE markers other tests plant, and every shape that
// must report ok=false so the caller falls back to refresh-on-inequality.
func TestParseTemplateVersion(t *testing.T) {
	for _, tc := range []struct {
		marker string
		want   int
		wantOK bool
	}{
		{"v4", 4, true},
		{"v11", 11, true},
		{"v9", 9, true},
		{"v10", 10, true},
		{"v0", 0, true},
		{"v9-pointer", 9, true},
		{"v11-pointer", 11, true},
		{"v0-FAKE", 0, true}, // planted by refresh_test.go / doctor_test.go
		{" v11 ", 11, true},
		// Unparseable — ok=false, caller keeps the pre-#286 behaviour.
		{"", 0, false},
		{"v", 0, false},
		{"v-3", 0, false},
		{"v+5", 0, false},
		{"vNOPE", 0, false},
		{"v 4", 0, false},
		{"11", 0, false},
		{"latest", 0, false},
	} {
		got, ok := parseTemplateVersion(tc.marker)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("parseTemplateVersion(%q) = (%d, %v); want (%d, %v)",
				tc.marker, got, ok, tc.want, tc.wantOK)
		}
	}
}

// TestParseTemplateVersion_Ordering pins the compare in BOTH directions.
// v4 vs v11 is the trap: lexically "v11" < "v4", so a string compare gets
// the #286 case exactly backwards. v9 vs v10 is the same trap one decade
// down.
func TestParseTemplateVersion_Ordering(t *testing.T) {
	for _, tc := range []struct {
		older, newer string
	}{
		{"v4", "v11"},
		{"v9", "v10"},
		{"v4", "v9-pointer"},
		{"v9-pointer", "v11"},
	} {
		o, ook := parseTemplateVersion(tc.older)
		n, nok := parseTemplateVersion(tc.newer)
		if !ook || !nok {
			t.Fatalf("both markers must parse: %q ok=%v, %q ok=%v", tc.older, ook, tc.newer, nok)
		}
		if !(n > o) {
			t.Errorf("%s must order AFTER %s; got %d vs %d", tc.newer, tc.older, n, o)
		}
		if !(o < n) {
			t.Errorf("%s must order BEFORE %s; got %d vs %d", tc.older, tc.newer, o, n)
		}
	}
	// Equal markers are neither newer nor older.
	a, _ := parseTemplateVersion("v11")
	b, _ := parseTemplateVersion("v11")
	if a != b {
		t.Errorf("equal markers must compare equal; got %d vs %d", a, b)
	}
	// The trap itself, asserted: a string compare disagrees with the
	// numeric one for exactly the #286 pair.
	if !("v11" < "v4") {
		t.Fatal("premise changed: \"v11\" is no longer lexically less than \"v4\"")
	}
}

// bundledTemplateVersion returns the `# logmind-template-version:` marker
// this binary ships for one workflow template.
func bundledTemplateVersion(t *testing.T, name string) string {
	t.Helper()
	v := extractTemplateVersion(templates.Workflow(name + ".template"))
	if v == "" {
		t.Fatalf("bundled %s carries no template-version marker", name)
	}
	return v
}

// plantNewerWorkflow writes .github/workflows/check-decisions.yml carrying
// the marker #286 reported on disk (v11) against a binary that bundles v4.
// Returns (installedMarker, bundledMarker).
func plantNewerWorkflow(t *testing.T) (string, string) {
	t.Helper()
	const installed = "v11"
	bundled := bundledTemplateVersion(t, "check-decisions.yml")
	bn, ok := parseTemplateVersion(bundled)
	in, _ := parseTemplateVersion(installed)
	if !ok || bn >= in {
		t.Fatalf("the #286 reproduction needs a bundled check-decisions.yml marker below %s; "+
			"it is now %s — re-point this test at a marker the binary is behind", installed, bundled)
	}
	writeRel(t, filepath.Join(".github", "workflows", "check-decisions.yml"),
		"# logmind-template-version: "+installed+"\n# hand-taken ahead of the release\n", 0o644)
	return installed, bundled
}

// TestInstallWorkflowTemplates_RefusesDowngrade is the unit-level #286
// reproduction: refresh mode leaves a newer-on-disk template byte-untouched
// and reports the refusal upward (not as a "refreshed" entry).
func TestInstallWorkflowTemplates_RefusesDowngrade(t *testing.T) {
	withTempCwd(t, func(dir string) {
		installed, bundled := plantNewerWorkflow(t)
		before := readRel(t, filepath.Join(".github", "workflows", "check-decisions.yml"))

		_, refreshed, declined, err := installWorkflowTemplates(dir, true)
		if err != nil {
			t.Fatalf("installWorkflowTemplates: %v", err)
		}

		after := readRel(t, filepath.Join(".github", "workflows", "check-decisions.yml"))
		if after != before {
			t.Errorf("newer template was overwritten:\n got: %q\nwant: %q", after, before)
		}
		for _, r := range refreshed {
			if strings.Contains(r, "check-decisions.yml") {
				t.Errorf("a refused downgrade must not be reported as refreshed: %v", refreshed)
			}
		}
		if len(declined) != 1 {
			t.Fatalf("declined = %v; want exactly one entry", declined)
		}
		if declined[0].Installed != installed || declined[0].Bundled != bundled {
			t.Errorf("declined[0] = %+v; want Installed=%s Bundled=%s", declined[0], installed, bundled)
		}
		if !strings.Contains(declined[0].Path, "check-decisions.yml") {
			t.Errorf("declined[0].Path = %q; want the check-decisions.yml path", declined[0].Path)
		}
	})
}

// TestInstallWorkflowTemplates_StillUpgrades: the narrow fix must not
// change the forward direction. An older marker on disk is still refreshed
// to the bundled body.
func TestInstallWorkflowTemplates_StillUpgrades(t *testing.T) {
	withTempCwd(t, func(dir string) {
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		writeRel(t, rel, "# logmind-template-version: v1\n# ancient\n", 0o644)

		_, refreshed, declined, err := installWorkflowTemplates(dir, true)
		if err != nil {
			t.Fatalf("installWorkflowTemplates: %v", err)
		}
		if len(declined) != 0 {
			t.Errorf("declined = %+v; want none on an upgrade", declined)
		}
		want := bundledTemplateVersion(t, "check-decisions.yml")
		if got := extractTemplateVersion(readRel(t, rel)); got != want {
			t.Errorf("marker after refresh = %q; want %q", got, want)
		}
		found := false
		for _, r := range refreshed {
			if strings.Contains(r, "check-decisions.yml") {
				found = true
			}
		}
		if !found {
			t.Errorf("refreshed = %v; want it to include check-decisions.yml", refreshed)
		}
	})
}

// TestInstallWorkflowTemplates_UnparseableMarkerStillRefreshes: an
// unreadable marker must NOT become a way to pin a stale template forever —
// it falls back to the pre-#286 refresh-on-inequality behaviour.
func TestInstallWorkflowTemplates_UnparseableMarkerStillRefreshes(t *testing.T) {
	withTempCwd(t, func(dir string) {
		rel := filepath.Join(".github", "workflows", "check-decisions.yml")
		writeRel(t, rel, "# logmind-template-version: vNOPE\n# unreadable\n", 0o644)

		_, _, declined, err := installWorkflowTemplates(dir, true)
		if err != nil {
			t.Fatalf("installWorkflowTemplates: %v", err)
		}
		if len(declined) != 0 {
			t.Errorf("declined = %+v; want none for an unparseable marker", declined)
		}
		want := bundledTemplateVersion(t, "check-decisions.yml")
		if got := extractTemplateVersion(readRel(t, rel)); got != want {
			t.Errorf("marker after refresh = %q; want %q (unparseable must not pin)", got, want)
		}
	})
}

// TestInitRefresh_RefusesTemplateDowngrade — #286's reported scenario end to
// end through `logmind init` refresh mode: a repo carrying v11 refreshed by
// a binary bundling v4 keeps v11, and the refusal is named on stderr.
func TestInitRefresh_RefusesTemplateDowngrade(t *testing.T) {
	withTempCwd(t, func(_ string) {
		runQuiet(t, []string{"init", "--no-git"})
		installed, bundled := plantNewerWorkflow(t)
		before := readRel(t, filepath.Join(".github", "workflows", "check-decisions.yml"))

		out, errOut := runInitCapture(t, []string{"init", "--no-git"})

		after := readRel(t, filepath.Join(".github", "workflows", "check-decisions.yml"))
		if after != before {
			t.Errorf("init refresh downgraded a newer template:\n got: %q\nwant: %q", after, before)
		}
		if strings.Contains(out, "↻ Refreshed .github/workflows/check-decisions.yml") {
			t.Errorf("a downgrade must not be reported as a refresh:\n%s", out)
		}
		// The refusal MUST be visible, naming both markers and the direction.
		mustContain(t, errOut, "check-decisions.yml")
		mustContain(t, errOut, "installed template "+installed)
		mustContain(t, errOut, "NEWER")
		mustContain(t, errOut, bundled)
	})
}

// TestDoctorFix_RefusesTemplateDowngrade — the same refusal must surface
// through the other refresh caller, `logmind doctor --fix`.
func TestDoctorFix_RefusesTemplateDowngrade(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		installed, bundled := plantNewerWorkflow(t)
		before := readRel(t, filepath.Join(".github", "workflows", "check-decisions.yml"))

		out, stderr := runDoctorFixCmd(t)

		after := readRel(t, filepath.Join(".github", "workflows", "check-decisions.yml"))
		if after != before {
			t.Errorf("doctor --fix downgraded a newer template:\n got: %q\nwant: %q", after, before)
		}
		mustContain(t, out, "ok doctor-fix")
		mustContain(t, stderr, "check-decisions.yml")
		mustContain(t, stderr, "installed template "+installed)
		mustContain(t, stderr, "NEWER")
		mustContain(t, stderr, bundled)
	})
}

// runInitCapture runs a root command and returns (stdout, stderr) without
// failing on a non-nil error — refresh mode reports downgrade refusals on
// stderr while still exiting 0.
func runInitCapture(t *testing.T, args []string) (string, string) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(args)
	var out, errOut strings.Builder
	root.SetOut(&out)
	root.SetErr(&errOut)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v\nstdout=%s\nstderr=%s", args, err, out.String(), errOut.String())
	}
	return out.String(), errOut.String()
}
