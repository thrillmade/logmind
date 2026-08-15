// auto_advisory_test.go — exercises collectAutoAdvisories / the
// CollectStatus.AutoAdvisories field (#241): a standing directive that
// predates the bundled policy, one naming a profile this binary does not
// know, and one carrying no ownership marker at all.
//
// Every case is ADVISORY ONLY — Overall must stay OK. `logmind auto` is
// an explicit opt-in verb whose directive carries policy a human
// authored, so none of this is drift and none of it is auto-fixable.
package doctor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/auto"
)

func writeDirective(t *testing.T, dir, content string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, ".logmind", "auto.yml"), content)
}

// bundledUnattendedMarker is the marker this binary ships for the
// `unattended` profile — read from the registry rather than hardcoded, so
// bumping the template's version does not silently rot these tests.
func bundledUnattendedMarker(t *testing.T) string {
	t.Helper()
	p, ok := auto.Lookup("unattended")
	if !ok {
		t.Fatalf("the `unattended` profile is not registered")
	}
	marker, ok := auto.BundledMarker(p)
	if !ok {
		t.Fatalf("the `unattended` template carries no version marker")
	}
	return marker
}

// TestAutoAdvisories_NoDirective_NoAdvisory — the common case. A repo that
// never ran `logmind auto` is not drifted; it simply never opted in.
func TestAutoAdvisories_NoDirective_NoAdvisory(t *testing.T) {
	dir := freshRepo(t)
	r := CollectStatus(dir, true)
	if len(r.AutoAdvisories) != 0 {
		t.Errorf("AutoAdvisories = %v; want none (no directive installed)", r.AutoAdvisories)
	}
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; an absent auto directive must never be drift")
	}
}

func TestAutoAdvisories_CurrentDirective_NoAdvisory(t *testing.T) {
	dir := freshRepo(t)
	writeDirective(t, dir, "# logmind-auto-version: "+bundledUnattendedMarker(t)+"\nprofile: unattended\n")
	r := CollectStatus(dir, true)
	if len(r.AutoAdvisories) != 0 {
		t.Errorf("AutoAdvisories = %v; want none (directive is current)", r.AutoAdvisories)
	}
}

// TestAutoAdvisories_StaleDirective — the drift this feature exists to
// notice: the policy the directive restates has moved on. Advisory only.
func TestAutoAdvisories_StaleDirective(t *testing.T) {
	dir := freshRepo(t)
	writeDirective(t, dir, "# logmind-auto-version: v0\nprofile: unattended\n")
	r := CollectStatus(dir, true)
	if len(r.AutoAdvisories) != 1 {
		t.Fatalf("AutoAdvisories = %v; want exactly 1", r.AutoAdvisories)
	}
	got := r.AutoAdvisories[0]
	for _, want := range []string{".logmind/auto.yml", "v0", bundledUnattendedMarker(t), "logmind auto unattended"} {
		if !strings.Contains(got, want) {
			t.Errorf("advisory = %q; want it to mention %q", got, want)
		}
	}
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the auto advisory must never be drift")
	}
	// It must also show up in the rendered table, or it reaches nobody.
	if !strings.Contains(RenderStatus(r), got) {
		t.Errorf("RenderStatus omitted the auto advisory")
	}
}

// TestAutoAdvisories_UnknownProfile — a directive naming a profile this
// binary has never heard of. logmind reports it and names what it knows;
// it never guesses what the file should contain (#267's lesson).
func TestAutoAdvisories_UnknownProfile(t *testing.T) {
	dir := freshRepo(t)
	writeDirective(t, dir, "# logmind-auto-version: v1\nprofile: skdd\n")
	r := CollectStatus(dir, true)
	if len(r.AutoAdvisories) != 1 {
		t.Fatalf("AutoAdvisories = %v; want exactly 1", r.AutoAdvisories)
	}
	got := r.AutoAdvisories[0]
	for _, want := range []string{`"skdd"`, "does not know", "unattended"} {
		if !strings.Contains(got, want) {
			t.Errorf("advisory = %q; want it to mention %q", got, want)
		}
	}
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the auto advisory must never be drift")
	}
}

// TestAutoAdvisories_MarkerlessDirective — SPEC §5.2: an artifact with no
// marker belongs to the user. Doctor says so, because otherwise the file
// silently never refreshes and nobody knows why.
func TestAutoAdvisories_MarkerlessDirective(t *testing.T) {
	dir := freshRepo(t)
	writeDirective(t, dir, "profile: unattended\nhard_stops:\n  repo: [never touch prod]\n")
	r := CollectStatus(dir, true)
	if len(r.AutoAdvisories) != 1 {
		t.Fatalf("AutoAdvisories = %v; want exactly 1", r.AutoAdvisories)
	}
	got := r.AutoAdvisories[0]
	for _, want := range []string{"logmind-auto-version", "belongs to you"} {
		if !strings.Contains(got, want) {
			t.Errorf("advisory = %q; want it to mention %q", got, want)
		}
	}
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the auto advisory must never be drift")
	}
}
