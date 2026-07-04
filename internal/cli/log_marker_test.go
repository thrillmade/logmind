package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thrillmade/logmind/internal/timeline"
)

// optIntoMainCanonical overwrites the scaffolded config to enable the
// main-canonical timeline. Call after scaffoldDocs, before the first log.
//
// NOTE post-v1.0-flip: main-canonical is now the DEFAULT, so this call is
// belt-and-suspenders on a scaffolded repo — it makes the mode explicit and
// keeps these tests decoupled from any future default change.
func optIntoMainCanonical(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".logmind", "config.yml"),
		[]byte("timeline:\n  canonical: main-canonical\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// pinBranchDivergent writes the DEPRECATED-but-supported branch-divergent
// opt-out into dir's config. Symmetric to optIntoMainCanonical. Post-v1.0
// flip, branch-divergent-specific behavior (no entry-block marker, doctor
// no-ops, the v0.1.x byte floor) must EXPLICITLY opt out of the new default.
// Creates .logmind/ if absent so it works on a bare TempDir or after
// scaffoldDocs (overriding the scaffolded default).
func pinBranchDivergent(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".logmind"), 0o755); err != nil {
		t.Fatalf("mkdir .logmind: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".logmind", "config.yml"),
		[]byte("timeline:\n  canonical: branch-divergent\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func logOnce(t *testing.T, summary string) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs([]string{"log", summary, "-r", "Why", "--no-commit", "--no-interactive"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("log %q: %v\n%s", summary, err, out.String())
	}
}

// TestLog_MainCanonical_WritesAndPreservesMarker: the first log on a branch
// writes ONE §1.6.3 marker between the header and the first entry; a second
// log preserves it (no duplicate).
func TestLog_MainCanonical_WritesAndPreservesMarker(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		optIntoMainCanonical(t, d)
		cmd := exec.Command("git", "checkout", "-b", "feat/login")
		cmd.Dir = d
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("checkout: %v\n%s", err, out)
		}
		withFakeTTY(t, false, func() {
			logOnce(t, "Add JWT session auth")
			logOnce(t, "Refresh token rotation")
		})
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__login.md"))

	if n := strings.Count(s, "<!-- logmind-entry-start: "); n != 1 {
		t.Fatalf("marker count = %d; want 1 (written once, preserved on append)\n%s", n, s)
	}
	// Key slug derives from the FIRST summary.
	if !strings.Contains(s, "-add-jwt-session-auth -->") {
		t.Errorf("marker key missing slug add-jwt-session-auth\n%s", s)
	}
	// Visible line is the link-free headline (no detail link baked into the
	// branch file — that would break check-links from this dir).
	if !strings.Contains(s, "— Add JWT session auth\n") {
		t.Errorf("marker line missing/!link-free\n%s", s)
	}
	if strings.Contains(s[:strings.Index(s, "logmind-entry-end")], "[detail]") {
		t.Errorf("marker body must NOT carry a detail link (check-links)\n%s", s)
	}
	// Ordering: header < marker < first decision entry.
	hdr := strings.Index(s, "← back to")
	mark := strings.Index(s, "<!-- logmind-entry-start:")
	entry := strings.Index(s, "\n## ")
	if !(hdr >= 0 && hdr < mark && mark < entry) {
		t.Errorf("expected header < marker < first-entry; got %d,%d,%d\n%s", hdr, mark, entry, s)
	}
}

// TestLog_BranchDivergent_NoMarker: in the branch-divergent opt-out, branch
// files are byte-stable — NO entry-block marker is written (the v0.1.x parity
// guard at the log layer). Post-v1.0 flip this must explicitly pin
// branch-divergent, since main-canonical (which DOES write a marker) is now
// the default; see TestLog_DefaultIsMainCanonical_WritesMarker.
func TestLog_BranchDivergent_NoMarker(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		pinBranchDivergent(t, d) // explicit opt-out of the new default
		cmd := exec.Command("git", "checkout", "-b", "feat/plain")
		cmd.Dir = d
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("checkout: %v\n%s", err, out)
		}
		withFakeTTY(t, false, func() { logOnce(t, "Plain decision") })
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__plain.md"))
	if strings.Contains(s, "logmind-entry-start") {
		t.Errorf("branch-divergent mode wrote a marker; want none\n%s", s)
	}
}

// TestLog_DefaultIsMainCanonical_WritesMarker is the v1.0-flip lock: with a
// DEFAULT scaffolded config (config.yml.template has NO `timeline` key, so the
// main-canonical default governs), the first log on a feature branch WRITES a
// §1.6.3 entry-block marker and `logmind timeline` renders the main-canonical
// (entry-block) format. Proves the flip is real end-to-end — no explicit opt-in.
func TestLog_DefaultIsMainCanonical_WritesMarker(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t) // default config — main-canonical (the v1.0 default)
		cmd := exec.Command("git", "checkout", "-b", "feat/default")
		cmd.Dir = d
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("checkout: %v\n%s", err, out)
		}
		withFakeTTY(t, false, func() { logOnce(t, "Default-mode decision") })
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__default.md"))
	if n := strings.Count(s, "<!-- logmind-entry-start: "); n != 1 {
		t.Fatalf("default (main-canonical) mode marker count = %d; want 1\n%s", n, s)
	}

	// The default config must resolve to main-canonical timeline mode, and a
	// --write render must emit the entry-block format (the same generator the
	// --check path uses — no false-stale wedge).
	if !canonicalEnabled(dir) {
		t.Errorf("default config did not resolve to main-canonical timeline mode")
	}
	target := filepath.Join(dir, "docs", "timeline.md")
	var stdout, stderr bytes.Buffer
	if err := runTimeline(dir, target, false, false, false, &stdout, &stderr); err != nil {
		t.Fatalf("runTimeline: %v (%s)", err, stderr.String())
	}
	got := readFileStr(t, target)
	if !timeline.HasEntryBlocks(got) {
		t.Errorf("default (main-canonical) timeline did not render entry-block format:\n%s", got)
	}
}

// TestLog_MainCanonical_InsertsMarkerWhenAbsent: a branch file created before
// the opt-in (no marker) gets one inserted after the header on the next log.
func TestLog_MainCanonical_InsertsMarkerWhenAbsent(t *testing.T) {
	dir := withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		cmd := exec.Command("git", "checkout", "-b", "feat/upgrade")
		cmd.Dir = d
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("checkout: %v\n%s", err, out)
		}
		// First log in DEFAULT mode → markerless branch file.
		withFakeTTY(t, false, func() { logOnce(t, "Pre-opt-in decision") })
		// Now opt in and log again → marker must be inserted.
		optIntoMainCanonical(t, d)
		withFakeTTY(t, false, func() { logOnce(t, "Post-opt-in decision") })
	})
	s := readFileStr(t, filepath.Join(dir, "docs", "decisions-branches", "feat__upgrade.md"))
	if n := strings.Count(s, "<!-- logmind-entry-start: "); n != 1 {
		t.Fatalf("marker count = %d; want 1 inserted on the post-opt-in log\n%s", n, s)
	}
	// Inserted right after the header, before any decision entry.
	hdr := strings.Index(s, "← back to")
	mark := strings.Index(s, "<!-- logmind-entry-start:")
	entry := strings.Index(s, "\n## ")
	if !(hdr < mark && mark < entry) {
		t.Errorf("inserted marker mis-positioned; got %d,%d,%d\n%s", hdr, mark, entry, s)
	}
}

func TestBuildTimelineMarker(t *testing.T) {
	d := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	got := buildTimelineMarker(d, "Add JWT session auth", " (#42)")
	// Key excludes the PR suffix (§1.6.3.1); visible line includes it.
	if !strings.Contains(got, "<!-- logmind-entry-start: 2026-06-29-add-jwt-session-auth -->") {
		t.Errorf("key wrong (PR suffix must NOT be in the key):\n%s", got)
	}
	if !strings.Contains(got, "- **2026-06-29** — Add JWT session auth (#42)\n") {
		t.Errorf("visible line wrong (must carry the #42 suffix):\n%s", got)
	}
	if strings.Contains(got, "[detail]") {
		t.Errorf("marker body must be link-free:\n%s", got)
	}
	if !strings.HasSuffix(got, "<!-- logmind-entry-end -->\n\n") {
		t.Errorf("marker must end with the close marker + blank line:\n%q", got)
	}
}

func TestPrSuffixFromEnv(t *testing.T) {
	for in, want := range map[string]string{"": "", "42": " (#42)", "#42": " (#42)", "  7 ": " (#7)"} {
		t.Setenv("LOGMIND_PR", in)
		if got := prSuffixFromEnv(); got != want {
			t.Errorf("prSuffixFromEnv(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestInsertMarkerAfterHeader(t *testing.T) {
	hdr := "← back to [docs/timeline.md](../timeline.md)\n\n"
	got := string(insertMarkerAfterHeader([]byte(hdr+"## body\n"), "MARK\n"))
	if got != hdr+"MARK\n## body\n" {
		t.Errorf("insert-after-header wrong:\n%q", got)
	}
	// No header → prepend (never drop).
	got2 := string(insertMarkerAfterHeader([]byte("hand-edited\n"), "MARK\n"))
	if got2 != "MARK\nhand-edited\n" {
		t.Errorf("no-header prepend wrong:\n%q", got2)
	}
}

func readFileStr(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
