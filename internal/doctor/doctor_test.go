// doctor_test.go — exercises the doctor package against in-tmpdir
// fixtures. Mirrors the Python tests/test_doctor.py shape: each
// scenario constructs a known-state working tree, runs CollectStatus,
// and asserts the drift category for the expected probe rows.
//
// Network probes are skipped via offline=true. The PATH-resolution
// probe is exercised in its own test that constructs a fake `logmind`
// binary on PATH.
package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thrillmade/logmind/internal/claudehook"
	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/version"
)

// freshRepo creates a directory tree that looks like a logmind-init'd
// project (without actually invoking init). Returns the absolute path.
func freshRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "git:\n  auto_commit: true\n")
	mustWrite(t, filepath.Join(dir, "docs", "decisions.md"), "# Decisions\n")
	mustWrite(t, filepath.Join(dir, "docs", "timeline.md"), "# Timeline\n")
	// Every freshRepo-based test that asserts Overall is inherently sensitive
	// to the live probePathResolution probe, so isolate PATH by default.
	isolatePathHermetic(t)
	return dir
}

// isolatePathHermetic points PATH at an empty temp dir (restored on Cleanup)
// so probePathResolution — the sole live-subprocess probe CollectStatus runs
// — resolves NO `logmind` and reports the benign "missing", instead of
// picking up whatever real, possibly-STALE logmind sits on the developer's
// PATH (e.g. a pyenv `logmind 1.2.0` shim). Without this, an Overall
// assertion is machine-dependent: the host binary alone can flip Overall to
// DRIFT. Tests that WANT a specific on-PATH binary override this afterward
// via prependFakeBinDir (which prepends onto this empty PATH). This mirrors
// the inline `os.Setenv("PATH", t.TempDir())` guard the DRIFT-asserting tests
// already use. The #214 regex fix (parsing the real `logmind <ver> (spec
// <ver>)` line the old pattern couldn't) is what unmasked this latent
// non-hermeticity: before it, every host binary silently classified
// markerless and never perturbed Overall.
func isolatePathHermetic(t *testing.T) {
	t.Helper()
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", t.TempDir())
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func TestCollectStatus_FreshRepoListsAllProbes(t *testing.T) {
	dir := freshRepo(t)
	// Make PATH probe deterministic — point at an empty dir so the
	// PATH-resolution probe yields "missing" regardless of the host
	// environment. Without this, a stale `logmind` on the developer's
	// PATH would flip the test to DRIFT vs CI's clean OK.
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", t.TempDir())

	r := CollectStatus(dir, true)
	if r.NetworkUsed {
		t.Errorf("offline=true; got NetworkUsed=true")
	}
	if len(r.Tools) != 1 {
		t.Fatalf("Tools = %d; want 1 (logmind only)", len(r.Tools))
	}
	tool := r.Tools[0]
	if tool.Name != "logmind" {
		t.Errorf("tool.Name = %q; want logmind", tool.Name)
	}
	// Each shipped probe should appear in the row list — that's the
	// contract that drives the renderer.
	names := workflowNames(tool.Workflows)
	for _, want := range []string{
		"regen-timeline.yml", "check-doc-links.yml",
		"logmind-self-update.yml", "check-decisions.yml",
		"AGENTS.md", ".gitattributes (merge driver)",
		"git config (merge driver)", "post-merge hook",
		"post-rewrite hook", "commit-msg hook",
		"pre-commit hook (derived-docs pin)",
		"Claude Code PreToolUse guard",
		"logmind on PATH",
	} {
		if !contains(names, want) {
			t.Errorf("workflow row %q missing; got %v", want, names)
		}
	}
	// Missing workflows are NOT stale (the renderer prints them as
	// "—"), so a fresh repo with no installed workflows should land
	// on OK overall — not DRIFT. DRIFT means a stale marker / version
	// mismatch, exercised by the *_DriftsToStale tests below.
	if r.Overall != "OK" {
		t.Errorf("Overall = %q; want OK on fresh repo with missing-only probes", r.Overall)
	}
}

// TestCollectStatus_OfflineParamIsNoOp pins the post-cleanup contract:
// the `offline` parameter is retained for signature/back-compat but doctor
// no longer makes network calls, so it must NEVER re-enable network use.
// Without this, a regression re-introducing `NetworkUsed: !offline` would
// pass every other test (they all call with offline=true).
func TestCollectStatus_OfflineParamIsNoOp(t *testing.T) {
	for _, offline := range []bool{true, false} {
		if r := CollectStatus(t.TempDir(), offline); r.NetworkUsed {
			t.Errorf("CollectStatus(offline=%v): NetworkUsed=true; want false", offline)
		}
	}
}

func TestCollectStatus_StaleWorkflowFlipsToDrift(t *testing.T) {
	dir := freshRepo(t)
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", t.TempDir())
	// Install a workflow with a known-wrong marker so the comparison
	// against the bundled marker yields stale.
	body := "# logmind-template-version: v0-FAKE\n# rest of file\n"
	mustWrite(t, filepath.Join(dir, ".github", "workflows", "regen-timeline.yml"), body)

	r := CollectStatus(dir, true)
	if r.Overall != "DRIFT" {
		t.Errorf("Overall = %q; want DRIFT with stale workflow marker", r.Overall)
	}

	// Pin the v1 brew/curl remediation stanza so a regression that
	// reverts to the legacy `pip install --upgrade logmind` suggestion
	// (or otherwise drops one of the three lines) is caught here. The
	// renderer emits each Suggestions entry on its own indented line,
	// so we assert each one's exact text.
	wantSuggestions := []string{
		"brew install thrillmade/tap/logmind",
		"# or: curl -fsSL https://logmind.dev/install.sh | bash",
		"# then re-run: logmind init",
	}
	if len(r.Suggestions) != len(wantSuggestions) {
		t.Fatalf("Suggestions = %v; want %d entries (brew/curl stanza)",
			r.Suggestions, len(wantSuggestions))
	}
	for i, want := range wantSuggestions {
		if r.Suggestions[i] != want {
			t.Errorf("Suggestions[%d] = %q; want %q", i, r.Suggestions[i], want)
		}
	}
}

// TestStaleCount_MatchesDriftClassification: StaleCount must count exactly
// the "stale"-classified rows and nothing else (missing rows on a fresh
// repo — e.g. every git hook, every workflow — must NOT be counted).
func TestStaleCount_MatchesDriftClassification(t *testing.T) {
	dir := freshRepo(t)
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", t.TempDir())

	if n := StaleCount(dir); n != 0 {
		t.Errorf("StaleCount(fresh repo) = %d; want 0 (missing != stale)", n)
	}

	body := "# logmind-template-version: v0-FAKE\n# rest of file\n"
	mustWrite(t, filepath.Join(dir, ".github", "workflows", "regen-timeline.yml"), body)

	if n := StaleCount(dir); n != 1 {
		t.Errorf("StaleCount(one stale workflow) = %d; want 1", n)
	}
}

// TestCollectLogmindStatusFast_ExcludesSubprocessProbes pins the shape
// contract of the hot-path probe subset: the PATH-resolution row and the
// git-config merge-driver row must NEVER appear (both fork a subprocess),
// while every file-read-only probe row still does.
func TestCollectLogmindStatusFast_ExcludesSubprocessProbes(t *testing.T) {
	dir := freshRepo(t)
	rows := collectLogmindStatusFast(dir)
	names := workflowNames(rows)
	if contains(names, "logmind on PATH") {
		t.Errorf("collectLogmindStatusFast must not include the PATH-resolution probe row; got %v", names)
	}
	if contains(names, "git config (merge driver)") {
		t.Errorf("collectLogmindStatusFast must not include the git-config merge-driver probe row; got %v", names)
	}
	for _, want := range []string{
		"regen-timeline.yml", "check-doc-links.yml", "logmind-self-update.yml", "check-decisions.yml",
		"AGENTS.md", ".gitattributes (merge driver)",
		"post-merge hook", "post-rewrite hook", "commit-msg hook",
		"pre-commit hook (derived-docs pin)",
		"Claude Code PreToolUse guard",
	} {
		if !contains(names, want) {
			t.Errorf("collectLogmindStatusFast missing expected file-read probe %q; got %v", want, names)
		}
	}
}

// prependFakeBinDir puts dir FIRST on PATH (keeping the rest of the real
// PATH after it) and restores the original PATH on test cleanup. Prepending
// rather than replacing matters here: several of these fixtures write
// wrapper shell scripts that themselves shell out to `sleep` / `touch` —
// external binaries the script's own PATH lookup needs to find. Replacing
// PATH wholesale (as a naive `os.Setenv("PATH", dir)` would) leaves those
// binaries unresolvable, so the wrapper's shell fails instantly with
// "command not found" instead of actually blocking — a false pass that
// would silently defeat the whole point of these hang-proof tests.
func prependFakeBinDir(t *testing.T, dir string) {
	t.Helper()
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)
}

// TestStaleCountFast_IgnoresStalePathBinary proves StaleCountFast's count
// can legitimately differ from StaleCount's: a stale on-PATH `logmind`
// binary flips the full probe set's count to 1, but the fast subset must
// stay at 0 since it never runs that probe at all.
func TestStaleCountFast_IgnoresStalePathBinary(t *testing.T) {
	dir := freshRepo(t)
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "logmind")
	// Real Go `--version` format (see internal/cli/root.go versionLine); #214
	// — the fixed regex only matches this, not the legacy Click `logmind,
	// version X` shape this fixture used to print.
	body := "#!/bin/sh\necho 'logmind 0.1.0 (spec 0.1.1)'\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	prependFakeBinDir(t, tmp)

	if n := StaleCount(dir); n != 1 {
		t.Fatalf("StaleCount (full probe set) = %d; want 1 (stale on-PATH binary)", n)
	}
	if n := StaleCountFast(dir); n != 0 {
		t.Errorf("StaleCountFast = %d; want 0 — PATH-resolution drift must never surface on the hot-path subset", n)
	}
}

// TestStaleCountFast_DoesNotBlockOnHungPathBinary is the unit-level
// hang-proof: with a `logmind` on PATH that sleeps for 30s, StaleCountFast
// must return near-instantly, proving it never touches probePathResolution
// (the subprocess probe). The full end-to-end proof — through `logmind
// log`'s actual pulse — lives in internal/cli/pulse_hotpath_test.go.
func TestStaleCountFast_DoesNotBlockOnHungPathBinary(t *testing.T) {
	dir := freshRepo(t)
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "logmind")
	body := "#!/bin/sh\nsleep 30\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	prependFakeBinDir(t, tmp)

	start := time.Now()
	StaleCountFast(dir)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("StaleCountFast took %v with a hung logmind on PATH; want near-instant (no subprocess probe on this path)", elapsed)
	}
}

// TestProbePathResolution_DaemonizingWrapper_WaitDelayBounds exercises the
// on-demand `doctor` path's hardening (item 1(b)): a PATH binary that
// forks a background grandchild (inheriting the CombinedOutput pipe) and
// exits immediately itself. Without cmd.WaitDelay, CombinedOutput's
// internal Wait blocks reading that pipe until EOF — which never arrives
// while the grandchild lives — regardless of the 5s context timeout
// (which only kills the direct child). WaitDelay bounds the wait to a
// couple of seconds after the direct child exits.
func TestProbePathResolution_DaemonizingWrapper_WaitDelayBounds(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess timing test; skip in -short mode")
	}
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "logmind")
	// The wrapper backgrounds a long sleep (inheriting stdout/stderr, i.e.
	// the shared pipe) and returns immediately itself — the daemonizing
	// pattern that defeats a naive ctx-timeout-only bound.
	body := "#!/bin/sh\n(sleep 30 &)\nexit 0\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	prependFakeBinDir(t, tmp)

	start := time.Now()
	_ = probePathResolution()
	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Fatalf("probePathResolution with a daemonizing PATH wrapper took %v; want bounded (WaitDelay=2s after the near-instant direct-child exit), not the grandchild's full 30s sleep", elapsed)
	}
}

func TestCollectStatus_InstalledWorkflowMarkerMatchesBundled(t *testing.T) {
	dir := freshRepo(t)
	// Write an installed workflow whose marker matches what we ship.
	bundled := bundledLogmindMarker("regen-timeline.yml")
	if bundled == nil {
		t.Skip("bundled marker missing — skipping (would be a templates regression)")
	}
	body := "# logmind-template-version: " + *bundled + "\n# rest of file\n"
	mustWrite(t, filepath.Join(dir, ".github", "workflows", "regen-timeline.yml"), body)

	r := CollectStatus(dir, true)
	// Find the regen-timeline row + assert it's current.
	for _, wf := range r.Tools[0].Workflows {
		if wf.Name == "regen-timeline.yml" {
			if wf.Drift != "current" {
				t.Errorf("regen-timeline.yml drift = %q; want current", wf.Drift)
			}
			if wf.Marker == nil || *wf.Marker != *bundled {
				t.Errorf("marker = %v; want %v", wf.Marker, bundled)
			}
			return
		}
	}
	t.Fatal("regen-timeline.yml row not found")
}

func TestCollectStatus_InstalledWorkflowMarkerDriftsToStale(t *testing.T) {
	dir := freshRepo(t)
	body := "# logmind-template-version: v0-FAKE\n# rest of file\n"
	mustWrite(t, filepath.Join(dir, ".github", "workflows", "regen-timeline.yml"), body)

	r := CollectStatus(dir, true)
	for _, wf := range r.Tools[0].Workflows {
		if wf.Name == "regen-timeline.yml" {
			if wf.Drift != "stale" {
				t.Errorf("drift = %q; want stale", wf.Drift)
			}
			return
		}
	}
	t.Fatal("row not found")
}

func TestProbePathResolution_RunningBinaryNotFound(t *testing.T) {
	// Override PATH to an empty dir so exec.LookPath fails. Restore on exit.
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	tmp := t.TempDir()
	_ = os.Setenv("PATH", tmp)

	row := probePathResolution()
	if row.Name != "logmind on PATH" {
		t.Errorf("Name = %q", row.Name)
	}
	if row.Drift != "missing" {
		t.Errorf("Drift = %q; want missing", row.Drift)
	}
	if row.BundledMarker == nil || *row.BundledMarker != version.Version {
		t.Errorf("BundledMarker = %v; want %v", row.BundledMarker, version.Version)
	}
}

func TestProbePathResolution_StaleVersionDrift(t *testing.T) {
	// Construct a fake `logmind` shell script that prints an old version in
	// the REAL Go `--version` format (`logmind <ver> (spec <ver>)`) — issue
	// #214: the legacy Click `logmind, version X` shape this test used to
	// assert never occurs in practice, and the fixed regex only matches the
	// real one.
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "logmind")
	body := "#!/bin/sh\necho 'logmind 0.1.0 (spec 0.1.1)'\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", tmp)

	row := probePathResolution()
	if row.Drift != "stale" {
		t.Errorf("Drift = %q; want stale", row.Drift)
	}
	if row.Marker == nil || !strings.Contains(*row.Marker, "0.1.0") {
		t.Errorf("Marker = %v; want it to contain 0.1.0", row.Marker)
	}
	if row.Marker == nil || !strings.Contains(*row.Marker, version.Version) {
		t.Errorf("Marker = %v; want it to contain running %s", row.Marker, version.Version)
	}
}

func TestProbePathResolution_MatchingVersionIsCurrent(t *testing.T) {
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "logmind")
	// Real Go `--version` format (see internal/cli/root.go versionLine) — the
	// only shape the fixed #214 regex matches.
	body := "#!/bin/sh\necho 'logmind " + version.Version + " (spec " + version.SpecVersion + ")'\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", tmp)

	row := probePathResolution()
	if row.Drift != "current" {
		t.Errorf("Drift = %q; want current", row.Drift)
	}
}

// TestProbePathResolution_RealVersionFormatNotMarkerless is the direct #214
// regression: a PATH binary whose `--version` prints the genuine
// `logmind <ver> (spec <ver>)` line (root.go's versionLine) MUST classify as
// current/stale — never the unparseable fallback. The pre-fix regex
// (`version\s+(\S+)`) found no "version" token in that line, so a real
// on-PATH Go logmind was always mis-labeled and the PATH-drift row went blind.
func TestProbePathResolution_RealVersionFormatNotMarkerless(t *testing.T) {
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "logmind")
	// A DIFFERENT version than the running binary → the real format must
	// resolve to a concrete stale classification, not the markerless fallback.
	body := "#!/bin/sh\necho 'logmind 9.9.9-onpath (spec " + version.SpecVersion + ")'\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", tmp)

	row := probePathResolution()
	if row.Drift == "unreadable" {
		t.Fatalf("Drift = unreadable on a real `logmind <ver> (spec <ver>)` line; #214 regressed (marker=%v)", row.Marker)
	}
	if row.Drift != "stale" {
		t.Errorf("Drift = %q; want stale (9.9.9-onpath != running %s)", row.Drift, version.Version)
	}
	if row.Marker == nil || !strings.Contains(*row.Marker, "9.9.9-onpath") {
		t.Errorf("Marker = %v; want it to contain the parsed on-PATH version 9.9.9-onpath", row.Marker)
	}
}

// TestProbePathResolution_LegacyClickVersionClassified is the dual-review
// follow-up to #214: a stale PYTHON (Click) binary on PATH prints
// `logmind, version X`. The re-anchored regex must still parse it and
// classify it stale/DRIFT — not silently degrade it to the unparseable
// fallback, which would blind the drift row to exactly the stale binary it
// exists to catch.
func TestProbePathResolution_LegacyClickVersionClassified(t *testing.T) {
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "logmind")
	body := "#!/bin/sh\necho 'logmind, version 0.6.16'\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", tmp)

	row := probePathResolution()
	if row.Drift == "unreadable" {
		t.Fatalf("Drift = unreadable on legacy Click `logmind, version 0.6.16`; a stale Python binary must be classified, not blinded (marker=%v)", row.Marker)
	}
	if row.Drift != "stale" {
		t.Errorf("Drift = %q; want stale (0.6.16 != running %s)", row.Drift, version.Version)
	}
	if row.Marker == nil || !strings.Contains(*row.Marker, "0.6.16") {
		t.Errorf("Marker = %v; want it to contain the parsed version 0.6.16", row.Marker)
	}
}

// TestProbePathResolution_UnreadableOnGarbageVersion — #306: a PATH binary
// whose output carries no parseable version is "unreadable", NOT "markerless".
// "markerless" is SPEC §5.2's OWNERSHIP verdict ("an artifact carrying no
// marker at all belongs to the user and MUST NOT be overwritten") and callers
// act on it as such — `doctor --fix` refuses to write the path and the
// residual note tells the user logmind is leaving their file alone. A binary
// on PATH is not a user-owned markerless artifact; reusing the ownership token
// for it made --fix state a true drift with a false cause.
func TestProbePathResolution_UnreadableOnGarbageVersion(t *testing.T) {
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "logmind")
	// Output that doesn't include the `version ` token at all so the
	// regex matches nothing. Mirrors Python's same fallback path.
	body := "#!/bin/sh\necho some unrelated output\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", tmp)

	row := probePathResolution()
	if row.Drift != "unreadable" {
		t.Errorf("Drift = %q; want unreadable", row.Drift)
	}
	if row.Drift == "markerless" {
		t.Error("Drift = markerless: an unreadable PATH binary is being reported under SPEC §5.2's " +
			"user-ownership verdict, which is a different fact about a different kind of artifact")
	}
}

func TestToJSON_StableFieldShape(t *testing.T) {
	r := CollectStatus(t.TempDir(), true)
	js, err := r.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	// Spot-check key field names a downstream consumer would parse.
	for _, want := range []string{
		`"project_root"`,
		`"tools"`,
		`"overall"`,
		`"network_used"`,
		`"suggestions"`,
		`"installed_version"`,
		`"latest_version"`,
		`"workflows"`,
		`"drift"`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("JSON missing field marker %s", want)
		}
	}
}

func TestRenderStatus_HumanReadable(t *testing.T) {
	r := CollectStatus(t.TempDir(), true)
	body := RenderStatus(r)
	if !strings.Contains(body, "Stack status:") {
		t.Errorf("rendered body missing 'Stack status:'")
	}
	if !strings.HasPrefix(body, "logmind ") {
		t.Errorf("rendered body should start with `logmind `; got %q", firstLineForTest(body))
	}
}

func firstLineForTest(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func workflowNames(workflows []WorkflowStatus) []string {
	out := make([]string, len(workflows))
	for i, w := range workflows {
		out[i] = w.Name
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestCollectStatus_SummariesNeeded classifies branch files in main-canonical
// mode: markerless → "no summary", marker==first-title → "placeholder",
// enriched → excluded. The advisory MUST NOT flip Overall to DRIFT.
func TestCollectStatus_SummariesNeeded(t *testing.T) {
	dir := t.TempDir()
	isolatePathHermetic(t) // Overall must be driven by the fixtures, not a stale host `logmind`.
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "timeline:\n  canonical: main-canonical\n")
	mustWrite(t, filepath.Join(dir, "docs", "decisions.md"), "# D\n")
	// markerless
	mustWrite(t, filepath.Join(dir, "docs", "decisions-branches", "feat__cache.md"),
		"← back\n\n## 2026-06-10 09:00 - Add caching\n\n---\n")
	// placeholder — marker headline equals the first decision's title
	mustWrite(t, filepath.Join(dir, "docs", "decisions-branches", "feat__login.md"),
		"← back\n\n<!-- logmind-entry-start: 2026-06-12-add-login -->\n- **2026-06-12** — Add login\n<!-- logmind-entry-end -->\n\n## 2026-06-12 10:00 - Add login\n\n---\n")
	// enriched — marker headline differs from the first title
	mustWrite(t, filepath.Join(dir, "docs", "decisions-branches", "feat__api.md"),
		"← back\n\n<!-- logmind-entry-start: 2026-06-14-add-api -->\n- **2026-06-14** — Built the full REST API with auth and pagination\n<!-- logmind-entry-end -->\n\n## 2026-06-14 10:00 - Add API\n\n---\n")

	r := CollectStatus(dir, true)
	if r.Overall == "DRIFT" {
		t.Errorf("Overall = DRIFT; the summary advisory must never be drift")
	}
	if len(r.SummariesNeeded) != 2 {
		t.Fatalf("SummariesNeeded = %d (%v); want 2 (markerless cache + placeholder login; enriched api excluded)", len(r.SummariesNeeded), r.SummariesNeeded)
	}
	joined := strings.Join(r.SummariesNeeded, "\n")
	if !strings.Contains(joined, "feat__cache.md") || !strings.Contains(joined, "no summary") {
		t.Errorf("missing markerless advisory: %v", r.SummariesNeeded)
	}
	if !strings.Contains(joined, "feat__login.md") || !strings.Contains(joined, "placeholder") {
		t.Errorf("missing placeholder advisory: %v", r.SummariesNeeded)
	}
	if strings.Contains(joined, "feat__api.md") {
		t.Errorf("enriched branch must NOT be listed: %v", r.SummariesNeeded)
	}
}

// --- probeClaudePreToolUseHook (Layer 1 / v2.0.0 enforcement) ------------

func TestProbeClaudePreToolUseHook_MissingOnFreshRepo(t *testing.T) {
	dir := freshRepo(t)
	row := probeClaudePreToolUseHook(dir)
	if row.Drift != "missing" {
		t.Errorf("Drift = %q; want missing", row.Drift)
	}
	if row.BundledMarker == nil || *row.BundledMarker != version.Version {
		t.Errorf("BundledMarker = %v; want %v", row.BundledMarker, version.Version)
	}
}

func TestProbeClaudePreToolUseHook_CurrentAfterInstall(t *testing.T) {
	dir := freshRepo(t)
	if _, err := claudehook.EnsurePreToolUseGuard(dir); err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	row := probeClaudePreToolUseHook(dir)
	if row.Drift != "current" {
		t.Errorf("Drift = %q; want current", row.Drift)
	}
	if row.Marker == nil || *row.Marker != version.Version {
		t.Errorf("Marker = %v; want %v", row.Marker, version.Version)
	}
}

func TestProbeClaudePreToolUseHook_StaleOnRevertedMarker(t *testing.T) {
	dir := freshRepo(t)
	if _, err := claudehook.EnsurePreToolUseGuard(dir); err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	revertClaudeHookMarker(t, dir)

	row := probeClaudePreToolUseHook(dir)
	if row.Drift != "stale" {
		t.Errorf("Drift = %q; want stale", row.Drift)
	}
}

// TestCollectStatus_StaleClaudeHookFlipsToDrift mirrors
// TestCollectStatus_StaleWorkflowFlipsToDrift for the new Layer 1 probe:
// a stale marker must flip Overall to DRIFT exactly like every other
// hook/workflow probe (classifyLogmindDrift treats them uniformly).
func TestCollectStatus_StaleClaudeHookFlipsToDrift(t *testing.T) {
	dir := freshRepo(t)
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", t.TempDir())
	if _, err := claudehook.EnsurePreToolUseGuard(dir); err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	revertClaudeHookMarker(t, dir)

	r := CollectStatus(dir, true)
	if r.Overall != "DRIFT" {
		t.Errorf("Overall = %q; want DRIFT with stale Claude Code PreToolUse marker", r.Overall)
	}
}

// revertClaudeHookMarker rewrites the installed hook-version marker in
// dir/.claude/settings.json to a known-stale value.
func revertClaudeHookMarker(t *testing.T, dir string) {
	t.Helper()
	path := claudehook.SettingsPath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	reverted := strings.ReplaceAll(string(data), version.Version, "0.1.0-FAKE")
	if err := os.WriteFile(path, []byte(reverted), 0o644); err != nil {
		t.Fatalf("write reverted settings.json: %v", err)
	}
}

// --- probeCommitMsgHook (the §3.4 commit gate) ---------------------------

// probeHook's whole drift signal rests on the hook body being a pure
// function of internal/version.Version: it byte-compares the installed file
// against hooks.BuildCommitMsgBody(). Issue #270 weighed pinning the
// installing binary's absolute PATH into that body and rejected it for
// exactly this reason — a body that varied by machine would make every
// correctly-installed hook look like content drift. These two tests are the
// pin on that reasoning: a hook this binary just wrote is CURRENT, and one
// byte of hand-editing is STALE.

func TestProbeCommitMsgHook_CurrentAfterInstall(t *testing.T) {
	dir := freshRepo(t)
	fakeGitDir(t, dir)
	if _, err := hooks.InstallCommitMsg(dir); err != nil {
		t.Fatalf("InstallCommitMsg: %v", err)
	}
	row := probeCommitMsgHook(dir)
	if row.Drift != "current" {
		t.Errorf("Drift = %q; want current — a correctly-installed hook must not be reported as drifted", row.Drift)
	}
	if row.Marker == nil || *row.Marker != version.Version {
		t.Errorf("Marker = %v; want %v", row.Marker, version.Version)
	}

	r := CollectStatus(dir, true)
	if r.Overall != "OK" {
		t.Errorf("Overall = %q; want OK", r.Overall)
	}
}

// TestProbeCommitMsgHook_ContentDriftOnHandEdit hand-edits ONE line of the
// installed hook — the fail-open branch, i.e. the line someone silencing the
// gate would reach for — leaving the version marker untouched, so only the
// byte-compare can catch it.
func TestProbeCommitMsgHook_ContentDriftOnHandEdit(t *testing.T) {
	dir := freshRepo(t)
	fakeGitDir(t, dir)
	if _, err := hooks.InstallCommitMsg(dir); err != nil {
		t.Fatalf("InstallCommitMsg: %v", err)
	}
	path := filepath.Join(dir, ".git", "hooks", "commit-msg")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read commit-msg: %v", err)
	}
	edited := strings.Replace(string(data), `if [ "$rc" -eq 65 ]; then`, `if [ "$rc" -eq 66 ]; then`, 1)
	if edited == string(data) {
		t.Fatalf("fixture is stale: the rc==65 block guard is no longer in the installed body")
	}
	mustWrite(t, path, edited)

	row := probeCommitMsgHook(dir)
	if row.Drift != "stale" {
		t.Errorf("Drift = %q; want stale — a hand-edited hook body must be detected", row.Drift)
	}
	if row.Marker == nil || !strings.Contains(*row.Marker, "content drift") {
		t.Errorf("Marker = %v; want it to say content drift (the version marker is untouched)", row.Marker)
	}

	r := CollectStatus(dir, true)
	if r.Overall != "DRIFT" {
		t.Errorf("Overall = %q; want DRIFT with a hand-edited commit-msg hook", r.Overall)
	}
}

// --- probePreCommitHook (L2a / v2.0.0 derived-docs pin-preservation) -----

// fakeGitDir plants a bare `.git/hooks/` directory under dir — enough for
// probePreCommitHook's (and probeHook's) `.git`-presence check, without
// needing a real `git init`. File-read-only probes only ever stat/read
// paths, so this is a faithful, hermetic fixture.
func fakeGitDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir .git/hooks: %v", err)
	}
}

func TestProbePreCommitHook_MissingOnFreshRepo(t *testing.T) {
	dir := freshRepo(t)
	fakeGitDir(t, dir)
	row := probePreCommitHook(dir)
	if row.Drift != "missing" {
		t.Errorf("Drift = %q; want missing", row.Drift)
	}
	if row.Installed {
		t.Errorf("Installed = true; want false (no pre-commit hook on disk)")
	}
}

// TestProbePreCommitHook_ForeignHookLeftAlone pins the "foreign, not
// markerless/stale" classification (ruling 5): a pre-commit hook that
// predates PreCommitMarker — here, the legacy `logmind install-hook`
// check-decisions body — is reported as Installed=true, Drift="foreign", so
// classifyLogmindDrift treats it exactly like a missing hook (benign, never
// flips Overall to DRIFT) instead of "stale" (which WOULD flip it).
func TestProbePreCommitHook_ForeignHookLeftAlone(t *testing.T) {
	dir := freshRepo(t)
	fakeGitDir(t, dir)
	foreign := "#!/bin/sh\n# logmind check-decisions — hang-guarded (issue #213)\nlogmind check-decisions\n"
	mustWrite(t, filepath.Join(dir, ".git", "hooks", "pre-commit"), foreign)

	row := probePreCommitHook(dir)
	if row.Drift != "foreign" {
		t.Errorf("Drift = %q; want foreign", row.Drift)
	}
	if !row.Installed {
		t.Errorf("Installed = false; want true (a foreign hook IS present on disk)")
	}

	r := CollectStatus(dir, true)
	if r.Overall != "OK" {
		t.Errorf("Overall = %q; want OK — a foreign pre-commit hook must not flip Overall to DRIFT", r.Overall)
	}
}

func TestProbePreCommitHook_CurrentAfterInstall(t *testing.T) {
	dir := freshRepo(t)
	fakeGitDir(t, dir)
	if _, err := hooks.InstallPreCommit(dir); err != nil {
		t.Fatalf("InstallPreCommit: %v", err)
	}
	row := probePreCommitHook(dir)
	if row.Drift != "current" {
		t.Errorf("Drift = %q; want current", row.Drift)
	}
	if row.Marker == nil || *row.Marker != version.Version {
		t.Errorf("Marker = %v; want %v", row.Marker, version.Version)
	}
}

func TestProbePreCommitHook_StaleOnRevertedMarker(t *testing.T) {
	dir := freshRepo(t)
	fakeGitDir(t, dir)
	if _, err := hooks.InstallPreCommit(dir); err != nil {
		t.Fatalf("InstallPreCommit: %v", err)
	}
	path := filepath.Join(dir, ".git", "hooks", "pre-commit")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pre-commit: %v", err)
	}
	reverted := strings.ReplaceAll(string(data), version.Version, "0.1.0-FAKE")
	if err := os.WriteFile(path, []byte(reverted), 0o755); err != nil {
		t.Fatalf("write reverted pre-commit: %v", err)
	}

	row := probePreCommitHook(dir)
	if row.Drift != "stale" {
		t.Errorf("Drift = %q; want stale", row.Drift)
	}

	r := CollectStatus(dir, true)
	if r.Overall != "DRIFT" {
		t.Errorf("Overall = %q; want DRIFT with stale pre-commit marker", r.Overall)
	}
}

// TestClassifyMarker_OrderedNotEqual pins the bug a retrospective panel
// found in the combination of #289 and #291.
//
// #289 taught `installWorkflowTemplates` that template markers are ORDERED
// and refused to move one backwards. It did not teach doctor, which still
// compared for equality — so a repository AHEAD of the running binary was
// reported "STALE (latest: <older marker>)", with both the verdict and the
// "latest" label inverted. Worse, because #289's refusal is correct, the
// row became permanently unclearable: doctor said stale, --fix refused,
// and nothing the operator did could reconcile them.
//
// Two consumers of the same fact have to agree, or the tool contradicts
// itself.
func TestClassifyMarker_OrderedNotEqual(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := []struct {
		name            string
		marker, bundled *string
		want            string
	}{
		{"equal is current", s("v5"), s("v5"), "current"},
		{"older installed is stale", s("v1"), s("v5"), "stale"},
		{"newer installed is ahead, not stale", s("v99"), s("v5"), "ahead"},

		// The lexical trap: "v11" sorts BEFORE "v4" as a string, so an
		// equality-or-string-compare implementation looks correct on
		// single digits and inverts the moment a version reaches two.
		{"v11 vs v4 is ahead, not stale", s("v11"), s("v4"), "ahead"},
		{"v4 vs v11 is stale", s("v4"), s("v11"), "stale"},

		// Flavour suffixes must not defeat the parse.
		{"pointer suffix, newer", s("v10-pointer"), s("v9-pointer"), "ahead"},
		{"pointer suffix, older", s("v9-pointer"), s("v10-pointer"), "stale"},

		// Unparseable falls back to the old semantics: something differs
		// and we cannot say which way, so "stale" is the honest answer.
		{"unparseable installed falls back to stale", s("vNEXT"), s("v5"), "stale"},
		{"unparseable bundled falls back to stale", s("v5"), s("vNEXT"), "stale"},

		{"no marker is markerless", nil, s("v5"), "markerless"},
		{"no bundled is unknown", s("v5"), nil, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyMarker(tc.marker, tc.bundled); got != tc.want {
				t.Errorf("classifyMarker() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRenderStatus_AheadRowNamesTheDirection pins the STRING the user
// saw, not the helper behind it.
//
// An adversarial review found the first attempt at this test worthless:
// it constructed `Drift: "ahead"` literally and asserted the roll-up did
// not flip, so deleting the rendering branch entirely left the package
// green while the row silently fell back to a bare "ahead" via
// formatDrift's default. The reported symptom was the string
// `STALE (latest: v4)` — inverted verdict, inverted label — so the string
// is what has to be pinned.
func TestRenderStatus_AheadRowNamesTheDirection(t *testing.T) {
	bundled := "v5"
	installed := "v99"
	r := StatusReport{
		Overall: "OK",
		Tools: []ToolStatus{{
			Name:  "logmind",
			Drift: "ok",
			Workflows: []WorkflowStatus{{
				Name:          "check-decisions.yml",
				Marker:        &installed,
				BundledMarker: &bundled,
				Drift:         "ahead",
			}},
		}},
	}
	body := RenderStatus(r)

	if !strings.Contains(body, "ahead of this binary") {
		t.Errorf("rendered row does not name the direction; got:\n%s", body)
	}
	if !strings.Contains(body, "bundles: v5") {
		t.Errorf("rendered row does not name what this binary bundles; got:\n%s", body)
	}
	// The original defect: calling the OLDER marker "latest".
	if strings.Contains(body, "latest: v5") {
		t.Errorf("rendered row calls the older marker \"latest\" — the inverted label this fixes; got:\n%s", body)
	}
	if strings.Contains(body, "STALE") {
		t.Errorf("an ahead row must not read STALE; got:\n%s", body)
	}
}

// TestClassifyLogmindDrift_AheadDoesNotFlipDrift covers the roll-up.
// Deliberately kept alongside the rendering test above rather than
// instead of it: this one proves an ahead row does not send the operator
// to `--fix`, which correctly refuses; that one proves the row says why.
func TestClassifyLogmindDrift_AheadDoesNotFlipDrift(t *testing.T) {
	ahead := []WorkflowStatus{{Name: "check-decisions.yml", Drift: "ahead"}}
	if got := classifyLogmindDrift(ahead); got == "stale" {
		t.Errorf("an ahead workflow flipped the tool to stale; got %q", got)
	}
	stale := []WorkflowStatus{{Name: "check-decisions.yml", Drift: "stale"}}
	if got := classifyLogmindDrift(stale); got != "stale" {
		t.Errorf("a stale workflow must still flip the tool to stale; got %q", got)
	}
	mixed := []WorkflowStatus{
		{Name: "check-decisions.yml", Drift: "ahead"},
		{Name: "regen-timeline.yml", Drift: "stale"},
	}
	if got := classifyLogmindDrift(mixed); got != "stale" {
		t.Errorf("ahead masked a genuinely stale row; got %q, want stale", got)
	}
}
