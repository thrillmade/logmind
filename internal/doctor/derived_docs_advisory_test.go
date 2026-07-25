// derived_docs_advisory_test.go — exercises collectDerivedDocsAdvisories /
// the CollectStatus.DerivedDocsAdvisories field (v2.0.0 B6, the
// derived-docs adoption gate's version floor). Every case is ADVISORY
// ONLY — Overall must stay OK regardless of what fires.
package doctor

import (
	"path/filepath"
	"testing"

	"github.com/thrillmade/logmind/internal/version"
)

func TestDerivedDocsAdvisories_DriverMode_NoAdvisory(t *testing.T) {
	dir := freshRepo(t)
	// No .logmind/config.yml at all — default "driver" mode.
	r := CollectStatus(dir, true)
	if len(r.DerivedDocsAdvisories) != 0 {
		t.Errorf("DerivedDocsAdvisories = %v; want none (driver mode, the default)", r.DerivedDocsAdvisories)
	}
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the derived-docs floor advisory must never be drift")
	}
}

func TestDerivedDocsAdvisories_DriverModeExplicit_MinBinarySet_NoAdvisory(t *testing.T) {
	dir := freshRepo(t)
	// Explicit "driver" with a min_binary set anyway — min_binary is
	// documented as ignored entirely outside integration-point mode.
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "derived_docs:\n  mode: driver\n  min_binary: \"99.0.0\"\n")
	r := CollectStatus(dir, true)
	if len(r.DerivedDocsAdvisories) != 0 {
		t.Errorf("DerivedDocsAdvisories = %v; want none (min_binary ignored in driver mode)", r.DerivedDocsAdvisories)
	}
}

func TestDerivedDocsAdvisories_IntegrationPoint_NoFloorDeclared_Advisory(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "derived_docs:\n  mode: integration-point\n")
	r := CollectStatus(dir, true)
	if len(r.DerivedDocsAdvisories) != 1 {
		t.Fatalf("DerivedDocsAdvisories = %v; want 1 (integration-point without a declared floor)", r.DerivedDocsAdvisories)
	}
	mustContainSubstr(t, r.DerivedDocsAdvisories[0], "min_binary")
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the derived-docs floor advisory must never be drift")
	}
}

func TestDerivedDocsAdvisories_IntegrationPoint_FloorSatisfied_NoAdvisory(t *testing.T) {
	dir := freshRepo(t)
	// A trivially-satisfied floor: the running test binary's version.Version
	// must be >= "0.0.1" (true for every real Version this repo ships).
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "derived_docs:\n  mode: integration-point\n  min_binary: \"0.0.1\"\n")
	r := CollectStatus(dir, true)
	if len(r.DerivedDocsAdvisories) != 0 {
		t.Errorf("DerivedDocsAdvisories = %v; want none (floor satisfied)", r.DerivedDocsAdvisories)
	}
}

// TestDerivedDocsAdvisories_IntegrationPoint_DevPrereleaseSatisfiesOwnFloor
// pins the exact self-adoption case (ruling 6): this repo's own dogfood
// binary reports "X.Y.Z-dev"; declaring min_binary at the CORE of that same
// version (stripping the prerelease suffix) must NOT warn about itself —
// see internal/version.SatisfiesMin's doc comment for why the suffix is
// stripped before comparing.
func TestDerivedDocsAdvisories_IntegrationPoint_DevPrereleaseSatisfiesOwnFloor(t *testing.T) {
	core, ok := coreVersionForTest(version.Version)
	if !ok {
		t.Skipf("version.Version %q isn't in a testable X.Y.Z(-suffix) shape", version.Version)
	}
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "derived_docs:\n  mode: integration-point\n  min_binary: \""+core+"\"\n")
	r := CollectStatus(dir, true)
	if len(r.DerivedDocsAdvisories) != 0 {
		t.Errorf("DerivedDocsAdvisories = %v; want none (running %q must satisfy its own core version %q as a floor)", r.DerivedDocsAdvisories, version.Version, core)
	}
}

func TestDerivedDocsAdvisories_IntegrationPoint_FloorViolated_Advisory(t *testing.T) {
	dir := freshRepo(t)
	// A floor no real released Version will ever satisfy.
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "derived_docs:\n  mode: integration-point\n  min_binary: \"9999.0.0\"\n")
	r := CollectStatus(dir, true)
	if len(r.DerivedDocsAdvisories) != 1 {
		t.Fatalf("DerivedDocsAdvisories = %v; want 1 (running binary older than the declared floor)", r.DerivedDocsAdvisories)
	}
	mustContainSubstr(t, r.DerivedDocsAdvisories[0], "9999.0.0")
	mustContainSubstr(t, r.DerivedDocsAdvisories[0], "older")
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the derived-docs floor advisory must never be drift")
	}
}

func TestDerivedDocsAdvisories_JSONFieldPresent(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "derived_docs:\n  mode: integration-point\n")
	r := CollectStatus(dir, true)
	js, err := r.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	mustContainSubstr(t, js, `"derived_docs_advisories"`)
	mustContainSubstr(t, js, "min_binary")
}

func TestDerivedDocsAdvisories_RenderStatus_HumanTable(t *testing.T) {
	dir := freshRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "derived_docs:\n  mode: integration-point\n  min_binary: \"9999.0.0\"\n")
	r := CollectStatus(dir, true)
	body := RenderStatus(r)
	mustContainSubstr(t, body, "Derived-docs version floor")
	mustContainSubstr(t, body, "9999.0.0")
	mustContainSubstr(t, body, "Stack status: OK")
}

// coreVersionForTest strips a "-prerelease"/"+build" suffix (and a leading
// "v") from a version string, returning ok=false if what's left isn't
// exactly three dot-separated integers — mirrors internal/version's
// unexported parseVersionCore closely enough for this test's purposes
// without reaching into that package's internals.
func coreVersionForTest(v string) (string, bool) {
	s := v
	if len(s) > 0 && s[0] == 'v' {
		s = s[1:]
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '-' || s[i] == '+' {
			s = s[:i]
			break
		}
	}
	parts := 1
	for _, c := range s {
		if c == '.' {
			parts++
		}
	}
	if parts != 3 {
		return "", false
	}
	return s, true
}
