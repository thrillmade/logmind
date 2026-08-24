package version

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

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

// --- site/app/page.tsx mirror -------------------------------------------
//
// TypeScript can't import a Go package, so site/app/page.tsx hand-copies
// a few of this package's facts into its own constants. Nothing checked
// the copy agreed with the original before the tests below: version.go
// is the source of truth in every pair; the site is the mirror, and a
// flagged mismatch is fixed by editing the site, never this file.
//
// One pair — AREAS — needs a qualifier the others don't: it sits beside
// the site's CURRENT_SPEC and CURRENT_RELEASE_DATE, both claims about
// the RELEASED binary, not this tree's still-moving dev Version. CI runs
// `make test` with GO_TEST_FLAGS empty (no -v), so a skip's reason is
// invisible in the normal run — which means a skip is only acceptable
// here when there is provably nothing to check, never merely as a
// quieter way of saying "can't verify this." See
// TestAreasMirroredInSitePage's own doc comment for the three cases.

// repoRootFromCaller walks up from the process's working directory (via
// os.Getwd, not the caller's file location — despite the name) to the
// directory holding go.mod. Mirrors internal/cli/version_test.go's
// helper of the same name and purpose: `go test` itself always sets that
// working directory to the package under test, so this locates
// site/app/page.tsx correctly whether run via `go test ./internal/version`
// from the repo root, from inside the package (`cd internal/version &&
// go test`), or from an IDE's own runner. A precompiled test binary
// executed from an unrelated directory (`go test -c` then run elsewhere)
// is not covered by that guarantee — it fails loudly here (Fatalf below),
// which is the correct outcome, not a silent wrong guess.
func repoRootFromCaller(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod walking up from %s", wd)
	return "" // unreachable
}

// readSitePageTSX returns site/app/page.tsx's contents. It skips — loudly,
// with the resolved path in the message — only when the whole site/
// subproject is absent, the one legitimate reason not to run the mirror
// checks below. Any other failure (site/ present but app/page.tsx missing
// or unreadable) is a hard failure: that means this test's own locator is
// broken, not that there is nothing left to guard, and a broken locator
// must not read as a passing (or silently skipped) test.
func readSitePageTSX(t *testing.T) string {
	t.Helper()
	repoRoot := repoRootFromCaller(t)

	siteDir := filepath.Join(repoRoot, "site")
	if _, err := os.Stat(siteDir); os.IsNotExist(err) {
		t.Skipf("site/ not found at %s — skipping the site/binary version mirror check (expected only if the site subproject has been removed; if site/ still exists on disk, this skip means this test's locator is wrong, not that there's nothing to check)", siteDir)
	}

	pagePath := filepath.Join(siteDir, "app", "page.tsx")
	data, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatalf("site/ exists at %s but %s could not be read (%v) — site/app/page.tsx may have moved; fix this test's path, not the constants it checks", siteDir, pagePath, err)
	}
	return string(data)
}

// extractTSConst pulls `const <name> = "value";` (an optional `: string`
// type annotation tolerated) out of TypeScript source. Not a real TS
// parser — the tests below only need the literal string value of a
// handful of top-level const declarations.
func extractTSConst(src, name string) (string, error) {
	re := regexp.MustCompile(`(?m)^const\s+` + regexp.QuoteMeta(name) + `(?:\s*:\s*string)?\s*=\s*"([^"]*)"`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return "", fmt.Errorf(`no "const %s = \"...\";" declaration found`, name)
	}
	return m[1], nil
}

// treeDescribesRelease reports whether siteVersion (site/app/page.tsx's
// CURRENT_VERSION) and treeVersion (this package's Version) name the
// same release, using parseVersionCore's tolerant major.minor.patch
// compare — the same parse SatisfiesMin and SameMajor already use, so a
// "-dev" suffix on treeVersion doesn't defeat the comparison. Also
// returns treeVersion's formatted core (e.g. "2.0.0-dev" -> "2.0.0") for
// callers building a message around it. A version string that fails to
// parse on either side is a hard failure, not a false/skip: that means
// something is broken enough that no comparison should be trusted
// silently.
func treeDescribesRelease(t *testing.T, siteVersion, treeVersion string) (aligned bool, treeCore string) {
	t.Helper()
	sc, ok := parseVersionCore(siteVersion)
	if !ok {
		t.Fatalf("site/app/page.tsx's CURRENT_VERSION = %q does not parse as major.minor.patch; fix CURRENT_VERSION, not this test", siteVersion)
	}
	tc, ok := parseVersionCore(treeVersion)
	if !ok {
		t.Fatalf("internal/version/version.go's Version = %q does not parse as major.minor.patch(-suffix); fix Version, not site/app/page.tsx", treeVersion)
	}
	return sc == tc, fmt.Sprintf("%d.%d.%d", tc[0], tc[1], tc[2])
}

// TestAreasMirroredInSitePage closes the gap that gave rise to this
// test: TypeScript can't import a Go package, so site/app/page.tsx
// hand-copies this package's Areas into its own AREAS constant, and
// nothing checked the two agreed. Areas is the source of truth — it's
// the second `--version` line the binary actually prints, SPEC §7.3 —
// so the site's copy must match it, never the reverse.
//
// AREAS is a claim about the RELEASED binary named by CURRENT_VERSION —
// the same kind of fact as the site's CURRENT_SPEC and
// CURRENT_RELEASE_DATE, neither of which is pinned to this tree's
// still-moving dev Version. version.go's Areas (and its neighbors:
// SpecVersion alone moved five times inside one still-unreleased cycle)
// can and does move mid-cycle without the release it'll ship in having
// changed. But a skip is invisible under `make test` (no -v in CI), so
// it is only defensible when there is nothing to check — never merely
// when this tree can't check it. Three cases, checked in this order:
//
//  1. CURRENT_VERSION is before AREAS_SINCE: the site's own
//     SHOWS_AREAS_LINE gate means the areas line does not render at all.
//     AREAS is dead content — skip, and say so.
//  2. CURRENT_VERSION is at or after AREAS_SINCE (the line DOES render)
//     but doesn't name the same release as this tree's Version: the
//     rendered claim can't be verified against anything this tree knows
//     — a rendered, unverifiable claim is exactly the drift this test
//     exists to catch, so this FAILS rather than passing through quietly.
//  3. CURRENT_VERSION names the same release as Version: assert equality
//     directly.
func TestAreasMirroredInSitePage(t *testing.T) {
	src := readSitePageTSX(t)

	siteAreas, err := extractTSConst(src, "AREAS")
	if err != nil {
		t.Fatalf("site/app/page.tsx: %v — its version-truth comment block above AREAS may have been restructured; update this test's extraction, not version.go's Areas", err)
	}

	currentVersion, err := extractTSConst(src, "CURRENT_VERSION")
	if err != nil {
		t.Fatalf("site/app/page.tsx: %v — its version-truth comment block above CURRENT_VERSION may have been restructured; update this test's extraction, not version.go's Areas", err)
	}

	areasSince, err := extractTSConst(src, "AREAS_SINCE")
	if err != nil {
		t.Fatalf("site/app/page.tsx: %v — its version-truth comment block above AREAS_SINCE may have been restructured; update this test's extraction, not version.go's Areas", err)
	}
	if _, ok := parseVersionCore(areasSince); !ok {
		t.Fatalf("site/app/page.tsx's AREAS_SINCE = %q does not parse as major.minor.patch; fix AREAS_SINCE, not this test", areasSince)
	}

	aligned, treeCore := treeDescribesRelease(t, currentVersion, Version)
	// renders reproduces site/app/page.tsx's own SHOWS_AREAS_LINE gate
	// (CURRENT_VERSION >= AREAS_SINCE) using SatisfiesMin — the same
	// numeric floor check already used elsewhere in this package, not a
	// second hand-rolled comparator.
	renders := SatisfiesMin(currentVersion, areasSince)

	switch {
	case !renders:
		t.Skipf("AREAS mirror check inactive: site/app/page.tsx's CURRENT_VERSION (v%s) is before AREAS_SINCE (v%s), so the areas line does not render on the page at this CURRENT_VERSION — AREAS is dead content, nothing to verify. Expected pre-v%s, not a bug.",
			currentVersion, areasSince, areasSince)

	case !aligned:
		t.Fatalf("site/app/page.tsx's CURRENT_VERSION (v%s) is at or after AREAS_SINCE (v%s), so the areas line DOES render — but this tree's Version (%s, core v%s) has moved past the release CURRENT_VERSION names, so nothing here can verify the rendered AREAS is still true of that release. A rendered, unverifiable claim must fail, not skip.\n"+
			"Either: record v%s's actual released areas line (not this dev tree's) and check against that instead — see the separately-tracked follow-up on sourcing \"the latest release\" for CI — or, if this IS release time, bump CURRENT_VERSION (with CURRENT_SPEC, CURRENT_RELEASE_DATE, and AREAS) together to describe v%s, the release this tree is actually building, so this test can verify it directly.",
			currentVersion, areasSince, Version, treeCore, currentVersion, treeCore)

	case siteAreas != Areas:
		t.Fatalf("site/app/page.tsx's AREAS = %q does not match internal/version/version.go's Areas = %q, and both describe the same release (v%s) right now.\n"+
			"internal/version/version.go's Areas is authoritative for that release (it's what `logmind --version` actually prints, SPEC §7.3) — "+
			"edit the AREAS constant in site/app/page.tsx to match it. Do not change Areas in version.go to make this pass.",
			siteAreas, Areas, currentVersion)
	}
}

// TestNextVersionMirrorsGoDevVersion applies the same "Go owns it, the
// site mirrors it" rule to site/app/page.tsx's NEXT_VERSION.
//
// NEXT_VERSION names the release the site's hero badge and install
// footnote describe ahead of time — generically, "there's a next release
// and it isn't out yet" (see site/app/page.tsx's own top-of-file
// comment for the exact sites this covers today; deliberately not
// quoted here — a paraphrase of the site's prose would silently go
// stale the moment that wording changes, which is the same
// hand-kept-copy problem this whole file exists to close, just one
// layer up in a comment instead of a constant). That's exactly what
// Version's un-tagged default ("2.0.0-dev": see that var's doc comment,
// "Bumped at release") names too, spelled two ways: Go carries a "-dev"
// prerelease suffix on its default; the site names the bare release the
// dev binary is building toward. Unlike AREAS above, NEXT_VERSION is
// unconditional — it never claims anything about a release that's
// already shipped, only about the one still coming, so there's no
// release-alignment window to gate it on. parseVersionCore is the same
// tolerant major.minor.patch parse SatisfiesMin and SameMajor already
// use to strip that suffix before comparing, so this reuses it rather
// than a raw string compare that would never agree by construction.
func TestNextVersionMirrorsGoDevVersion(t *testing.T) {
	src := readSitePageTSX(t)

	nextVersion, err := extractTSConst(src, "NEXT_VERSION")
	if err != nil {
		t.Fatalf("site/app/page.tsx: %v — its version-truth comment block above NEXT_VERSION may have been restructured; update this test's extraction, not version.go's Version", err)
	}

	core, ok := parseVersionCore(Version)
	if !ok {
		t.Fatalf("internal/version/version.go's Version = %q does not parse as major.minor.patch(-suffix); fix Version, not site/app/page.tsx", Version)
	}
	wantNext := fmt.Sprintf("%d.%d.%d", core[0], core[1], core[2])

	if nextVersion != wantNext {
		t.Fatalf("site/app/page.tsx's NEXT_VERSION = %q does not match internal/version/version.go's Version = %q (core %q, prerelease suffix stripped).\n"+
			"NEXT_VERSION names the release the dev binary is building toward; version.go's Version is authoritative — "+
			"edit NEXT_VERSION in site/app/page.tsx to %q. Do not change Version in version.go to make this pass.",
			nextVersion, Version, wantNext, wantNext)
	}
}
