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
// a few of this file's constants (its own comment says as much). Nothing
// checked the copy agreed with the original before the tests below:
// version.go is the source of truth in every pair; the site is the
// mirror, and a mismatch is fixed by editing the site, never this file.

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

// TestAreasMirroredInSitePage closes the gap site/app/page.tsx's own
// comment admits to: its AREAS constant is "a hand-maintained mirror" of
// this package's Areas, and nothing checked the two agreed. Areas is the
// source of truth — it's the second `--version` line the binary actually
// prints, SPEC §7.3 — so the site's copy must match it, never the
// reverse.
func TestAreasMirroredInSitePage(t *testing.T) {
	src := readSitePageTSX(t)

	siteAreas, err := extractTSConst(src, "AREAS")
	if err != nil {
		t.Fatalf("site/app/page.tsx: %v — its version-truth comment block above AREAS may have been restructured; update this test's extraction, not version.go's Areas", err)
	}

	if siteAreas != Areas {
		t.Fatalf("site/app/page.tsx's AREAS = %q does not match internal/version/version.go's Areas = %q.\n"+
			"internal/version/version.go's Areas is authoritative (it's what `logmind --version` actually prints, SPEC §7.3) — "+
			"edit the AREAS constant in site/app/page.tsx to match it. Do not change Areas in version.go to make this pass.",
			siteAreas, Areas)
	}
}

// TestNextVersionMirrorsGoDevVersion applies the same "Go owns it, the
// site mirrors it" rule to site/app/page.tsx's NEXT_VERSION.
//
// NEXT_VERSION names, per the site's own comment, "the release the
// 'enforced' section and hero badge describe ahead of time" — exactly
// what Version's un-tagged default ("2.0.0-dev": see that var's doc
// comment, "Bumped at release") names too, spelled two ways: Go carries a
// "-dev" prerelease suffix on its default; the site names the bare
// release the dev binary is building toward. parseVersionCore is the
// same tolerant major.minor.patch parse SatisfiesMin and SameMajor
// already use to strip that suffix before comparing, so this reuses it
// rather than a raw string compare that would never agree by
// construction.
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
