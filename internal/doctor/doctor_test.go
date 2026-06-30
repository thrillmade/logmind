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
	return dir
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
	// Construct a fake `logmind` shell script that prints an old version.
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "logmind")
	body := "#!/bin/sh\necho 'logmind, version 0.1.0'\n"
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
	body := "#!/bin/sh\necho 'logmind, version " + version.Version + "'\n"
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

func TestProbePathResolution_MarkerlessOnGarbageVersion(t *testing.T) {
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
	if row.Drift != "markerless" {
		t.Errorf("Drift = %q; want markerless", row.Drift)
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

// TestCollectStatus_SummariesNeeded_DefaultModeEmpty: in the default
// branch-divergent mode the check is OFF entirely (doctor output unchanged).
func TestCollectStatus_SummariesNeeded_DefaultModeEmpty(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"), "git:\n  auto_commit: true\n")
	mustWrite(t, filepath.Join(dir, "docs", "decisions-branches", "feat__x.md"),
		"← back\n\n## 2026-06-10 09:00 - X\n\n---\n")
	if r := CollectStatus(dir, true); len(r.SummariesNeeded) != 0 {
		t.Errorf("branch-divergent SummariesNeeded = %v; want empty (main-canonical-only feature)", r.SummariesNeeded)
	}
}
