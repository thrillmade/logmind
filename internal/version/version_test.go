package version

import "testing"

// TestSatisfiesMin_DevPrereleaseSatisfiesItsOwnFloor pins the deliberate
// departure from strict semver precedence (see SatisfiesMin's doc comment):
// this repo's own dogfood binary reports "2.0.0-dev" — a caller comparing
// that against a "2.0.0" floor must have the prerelease suffix stripped
// before comparing, or a self-referential version check would perpetually
// warn about itself.
func TestSatisfiesMin_DevPrereleaseSatisfiesItsOwnFloor(t *testing.T) {
	if !SatisfiesMin("2.0.0-dev", "2.0.0") {
		t.Errorf("SatisfiesMin(%q, %q) = false; want true (prerelease suffix stripped before compare)", "2.0.0-dev", "2.0.0")
	}
}

func TestSatisfiesMin_OlderVersionFails(t *testing.T) {
	cases := []struct{ v, min string }{
		{"1.9.9", "2.0.0"},
		{"2.0.0-dev", "2.0.1"},
		{"1.2.3", "1.2.4"},
		{"0.9.0", "1.0.0"},
	}
	for _, c := range cases {
		if SatisfiesMin(c.v, c.min) {
			t.Errorf("SatisfiesMin(%q, %q) = true; want false", c.v, c.min)
		}
	}
}

func TestSatisfiesMin_NewerOrEqualSucceeds(t *testing.T) {
	cases := []struct{ v, min string }{
		{"2.0.0", "2.0.0"},
		{"2.0.1", "2.0.0"},
		{"3.0.0", "2.9.9"},
		{"v2.0.0", "2.0.0"}, // leading "v" tolerated on either side
		{"2.0.0", "v2.0.0"},
	}
	for _, c := range cases {
		if !SatisfiesMin(c.v, c.min) {
			t.Errorf("SatisfiesMin(%q, %q) = false; want true", c.v, c.min)
		}
	}
}

// TestSameMajor_ReportsOnlyAMajorDifference pins the reporting boundary the
// commit-msg hook's engine-skew notice uses (issue #270). A dev prerelease
// and its release share a major; so do a patch and a minor apart. Only a
// major step across is skew worth interrupting a commit to mention — a
// per-patch notice would fire on every commit in every repo whose hooks
// predate the last release, and be ignored within a day.
func TestSameMajor_ReportsOnlyAMajorDifference(t *testing.T) {
	same := []struct{ a, b string }{
		{"2.0.0-dev", "2.0.0"},
		{"2.0.0", "2.9.9"},
		{"v2.1.0", "2.0.0"},
		{"1.2.0", "1.0.1"},
	}
	for _, c := range same {
		if !SameMajor(c.a, c.b) {
			t.Errorf("SameMajor(%q, %q) = false; want true", c.a, c.b)
		}
	}
	differ := []struct{ a, b string }{
		{"1.2.0", "2.0.0-dev"},
		{"2.0.0-dev", "3.0.0"},
		{"0.6.16", "2.0.0"},
	}
	for _, c := range differ {
		if SameMajor(c.a, c.b) {
			t.Errorf("SameMajor(%q, %q) = true; want false", c.a, c.b)
		}
	}
}

// TestSameMajor_UnparseableFailsOpen: same stance as SatisfiesMin. A skew
// notice fired off a version string this helper cannot read would be a false
// alarm, and the case that actually disables the gate — an engine that
// cannot run `guard-commit` at all — is caught by the hook body's own loud
// fail-open, not by this compare.
func TestSameMajor_UnparseableFailsOpen(t *testing.T) {
	cases := []struct{ a, b string }{
		{"", "2.0.0"},
		{"2.0.0", ""},
		{"not-a-version", "2.0.0"},
		{"2.0", "2.0.0"},
	}
	for _, c := range cases {
		if !SameMajor(c.a, c.b) {
			t.Errorf("SameMajor(%q, %q) = false; want true (fail open on unparseable input)", c.a, c.b)
		}
	}
}

// TestSatisfiesMin_UnparseableFailsOpen: a floor check is advisory-only —
// an unparseable version string (either side) must never manufacture a
// bogus warning, so SatisfiesMin returns true (satisfied) rather than
// erroring or panicking.
func TestSatisfiesMin_UnparseableFailsOpen(t *testing.T) {
	cases := []struct{ v, min string }{
		{"", "2.0.0"},
		{"2.0.0", ""},
		{"not-a-version", "2.0.0"},
		{"2.0.0", "not-a-version"},
		{"2.0", "2.0.0"},     // only two components
		{"2.0.0.1", "2.0.0"}, // four components
		{"2.x.0", "2.0.0"},   // non-numeric component
	}
	for _, c := range cases {
		if !SatisfiesMin(c.v, c.min) {
			t.Errorf("SatisfiesMin(%q, %q) = false; want true (fail open on unparseable input)", c.v, c.min)
		}
	}
}
