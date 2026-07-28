// quiet_test.go — the LOGMIND_QUIET output discipline (token-killer Phase 1b).
//
// Per wired verb we assert the three invariants:
//
//   - QUIET emits EXACTLY ONE `ok <k=v>` line to stdout and suppresses the
//     ✓-progress chatter / rendered body.
//   - Errors still land on stderr under QUIET (never swallowed).
//   - The DEFAULT (non-quiet) path is unchanged — the legacy trailer + body
//     are still there. (The timeline/tree/cli goldens are the byte-parity
//     guard; these are belt-and-suspenders contrast checks.)
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// assertSingleOK checks that out is exactly one line, an `ok ` receipt whose
// trailer contains each wanted key=value token.
func assertSingleOK(t *testing.T, out string, wantTokens ...string) {
	t.Helper()
	trimmed := strings.TrimRight(out, "\n")
	if strings.Contains(trimmed, "\n") {
		t.Fatalf("quiet stdout has >1 line:\n%q", out)
	}
	if !strings.HasPrefix(trimmed, "ok ") {
		t.Fatalf("quiet stdout is not an `ok` line: %q", out)
	}
	for _, tok := range wantTokens {
		if !strings.Contains(trimmed, tok) {
			t.Errorf("quiet ok line %q missing token %q", trimmed, tok)
		}
	}
}

// TestQuietFlagHelp_DoesNotOverpromiseUnwiredVerbs guards the MINOR fix:
// cobra renders --quiet's Usage string identically under every
// subcommand's "Global Flags" section (it's a persistent flag on root),
// including the 13 verbs that don't read it at all. The help text must not
// make the unqualified "one ok line per verb" promise; it must instead
// name the actual wired subset (or otherwise scope the claim) so a verb
// that ignores --quiet doesn't look broken against its own --help output.
func TestQuietFlagHelp_DoesNotOverpromiseUnwiredVerbs(t *testing.T) {
	root := NewRootCmd()
	f := root.PersistentFlags().Lookup(quietFlagName)
	if f == nil {
		t.Fatal("--quiet flag not registered on root")
	}
	usage := f.Usage

	// The unqualified promise this fix removes: "per verb" / "every verb"
	// with no scoping. cobra shows this same sentence on ALL 22
	// subcommands, but only 9 currently wire quietEnabled/qout in.
	if strings.Contains(usage, "per verb") && !strings.Contains(usage, "read/emit verb") {
		t.Errorf("usage still says %q; the unqualified 'per verb' promise must be scoped to the verbs that actually honor it", usage)
	}

	// The wired set the fix documents (see quiet.go's newQout callers +
	// doctor.go's inline quietEnabled use) must be named somewhere in the
	// help text, so an agent reading --help learns the real surface.
	for _, verb := range []string{"doctor", "file-structure", "guard-commit", "headline", "log", "repomap", "search", "show", "timeline"} {
		if !strings.Contains(usage, verb) {
			t.Errorf("usage %q does not name wired verb %q", usage, verb)
		}
	}

	// And it must signal, in some form, that other verbs are out of scope —
	// otherwise a reader still can't tell the list above is exhaustive.
	if !strings.Contains(strings.ToLower(usage), "other verbs") && !strings.Contains(strings.ToLower(usage), "ignore this flag") {
		t.Errorf("usage %q does not disclose that unwired verbs ignore --quiet", usage)
	}
}

func TestQuietEnvSet(t *testing.T) {
	cases := map[string]bool{
		"1": true, "true": true, "TRUE": true, "yes": true, "on": true, "anything": true,
		"": false, "0": false, "false": false, "no": false, "off": false, "  ": false,
	}
	for v, want := range cases {
		t.Setenv(quietEnvVar, v)
		if got := quietEnvSet(); got != want {
			t.Errorf("quietEnvSet(%q) = %v; want %v", v, got, want)
		}
	}
}

// TestQuietEnabled_ExplicitFlagBeatsEnv pins the precedence contract: an
// explicit --quiet / --quiet=false on the command line wins over the
// LOGMIND_QUIET env var; the env var only decides when the flag is absent.
func TestQuietEnabled_ExplicitFlagBeatsEnv(t *testing.T) {
	mk := func(args ...string) *cobra.Command {
		c := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
		c.Flags().Bool(quietFlagName, false, "")
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		return c
	}

	t.Setenv(quietEnvVar, "1") // env says quiet ON
	if quietEnabled(mk("--quiet=false")) {
		t.Error("--quiet=false must override LOGMIND_QUIET=1 (explicit flag wins)")
	}
	if !quietEnabled(mk()) {
		t.Error("LOGMIND_QUIET=1 with no flag should enable quiet")
	}

	t.Setenv(quietEnvVar, "0") // env says quiet OFF
	if !quietEnabled(mk("--quiet")) {
		t.Error("--quiet should enable quiet even with LOGMIND_QUIET=0")
	}
	if quietEnabled(mk()) {
		t.Error("no flag + env off should be the default (non-quiet)")
	}
}

func TestQuiet_Timeline_StdoutSingleOK(t *testing.T) {
	cwd := makeDocs(t,
		"## 2026-06-04 14:00 - Newest\n## 2026-06-01 08:00 - Oldest\n", "", nil)
	var out, errBuf bytes.Buffer
	if err := runTimeline(cwd, "", false, true, &out, &errBuf); err != nil {
		t.Fatalf("runTimeline quiet: %v", err)
	}
	assertSingleOK(t, out.String(), "timeline", "bytes=", "mode=canonical")
	if strings.Contains(out.String(), "Newest") || strings.Contains(out.String(), "✓") {
		t.Errorf("quiet stdout leaked the rendered body / chatter: %q", out.String())
	}
}

func TestQuiet_Timeline_DefaultUnchanged(t *testing.T) {
	cwd := makeDocs(t,
		"## 2026-06-04 14:00 - Newest\n## 2026-06-01 08:00 - Oldest\n", "", nil)
	var out, errBuf bytes.Buffer
	if err := runTimeline(cwd, "", false, false, &out, &errBuf); err != nil {
		t.Fatalf("runTimeline default: %v", err)
	}
	// Default mode keeps the legacy trailer AND the rendered body.
	if !strings.Contains(out.String(), "ok timeline: ") {
		t.Errorf("default mode dropped the legacy `ok timeline:` trailer: %q", out.String())
	}
	if !strings.Contains(out.String(), "Newest") {
		t.Errorf("default mode dropped the rendered body: %q", out.String())
	}
}

func TestQuiet_Timeline_ErrorToStderr(t *testing.T) {
	cwd := t.TempDir() // no docs/
	var out, errBuf bytes.Buffer
	if err := runTimeline(cwd, "", false, true, &out, &errBuf); err == nil {
		t.Fatal("expected ErrSilent when docs/ missing")
	}
	if out.Len() != 0 {
		t.Errorf("quiet error path wrote to stdout: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "Error: docs/ directory not found") {
		t.Errorf("error not routed to stderr under quiet: %q", errBuf.String())
	}
}

func TestQuiet_FileStructure_StdoutSingleOK(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	if err := runFileStructure(cwd, "", false, 2, true, &out, &errBuf); err != nil {
		t.Fatalf("runFileStructure quiet: %v", err)
	}
	assertSingleOK(t, out.String(), "file-structure", "bytes=", "depth=2")
}

func TestQuiet_FileStructure_DefaultUnchanged(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	if err := runFileStructure(cwd, "", false, 2, false, &out, &errBuf); err != nil {
		t.Fatalf("runFileStructure default: %v", err)
	}
	if !strings.Contains(out.String(), "ok file-structure: ") {
		t.Errorf("default mode dropped the legacy trailer: %q", out.String())
	}
}

func TestQuiet_Headline_SkippedSingleOK(t *testing.T) {
	cwd := t.TempDir() // not a git repo → default-branch skip
	var out, errBuf bytes.Buffer
	if err := runHeadline(cwd, "A summary", "", true, &out, &errBuf); err != nil {
		t.Fatalf("runHeadline quiet: %v", err)
	}
	assertSingleOK(t, out.String(), "headline", "state=skipped", "reason=default-branch")
}

func TestQuiet_Headline_DefaultUnchanged(t *testing.T) {
	cwd := t.TempDir() // not a git repo → default-branch skip
	var out, errBuf bytes.Buffer
	if err := runHeadline(cwd, "A summary", "", false, &out, &errBuf); err != nil {
		t.Fatalf("runHeadline default: %v", err)
	}
	if strings.Contains(out.String(), "ok ") {
		t.Errorf("default mode emitted an ok line it never had before: %q", out.String())
	}
	if !strings.Contains(out.String(), "the default branch logs to docs/decisions.md") {
		t.Errorf("default mode dropped its guidance line: %q", out.String())
	}
}

func TestQuiet_Headline_SetSingleOK(t *testing.T) {
	cwd := t.TempDir()
	branchDir := filepath.Join(cwd, "docs", "decisions-branches")
	if err := os.MkdirAll(branchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(branchDir, "feat__x.md")
	// Markerless branch file → setHeadlineInFile inserts a marker (state=set).
	if err := os.WriteFile(target, []byte("## 2026-07-01 10:00 - First\n\n---\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	q := newQout(true, &out, &errBuf)
	if err := setHeadlineInFile(cwd, target, "Whole-branch summary", q); err != nil {
		t.Fatalf("setHeadlineInFile quiet: %v", err)
	}
	assertSingleOK(t, out.String(), "headline", "state=set", "docs/decisions-branches/feat__x.md")
	if strings.Contains(out.String(), "✓") {
		t.Errorf("quiet leaked the ✓ chatter: %q", out.String())
	}
}

// TestQuiet_Headline_SetDefaultReceipt closes the default-mode coverage gap:
// on the headline SET path, default (non-quiet) stdout must carry the legacy
// ✓ receipt + the `ok headline: <rel>` trailer, and NOT the quiet k=v form.
func TestQuiet_Headline_SetDefaultReceipt(t *testing.T) {
	cwd := t.TempDir()
	branchDir := filepath.Join(cwd, "docs", "decisions-branches")
	if err := os.MkdirAll(branchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(branchDir, "feat__x.md")
	if err := os.WriteFile(target, []byte("## 2026-07-01 10:00 - First\n\n---\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	q := newQout(false, &out, &errBuf) // DEFAULT (non-quiet)
	if err := setHeadlineInFile(cwd, target, "Whole-branch summary", q); err != nil {
		t.Fatalf("setHeadlineInFile default: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "✓ Branch summary set: Whole-branch summary") {
		t.Errorf("default mode dropped the ✓ receipt: %q", s)
	}
	if !strings.Contains(s, "ok headline: docs/decisions-branches/feat__x.md") {
		t.Errorf("default mode dropped the legacy `ok headline:` trailer: %q", s)
	}
	if strings.Contains(s, "state=set") {
		t.Errorf("default mode leaked the quiet k=v form: %q", s)
	}
}

func TestQuiet_Doctor_ReportSingleOK(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--quiet", "--offline", "--exit-zero"})
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor quiet: %v\n%s", err, errBuf.String())
		}
		assertSingleOK(t, out.String(), "doctor", "overall=", "drift=")
		if strings.Contains(out.String(), "Stack status:") {
			t.Errorf("quiet doctor leaked the human table: %q", out.String())
		}
	})
}

func TestQuiet_Doctor_EnvVarEnables(t *testing.T) {
	t.Setenv(quietEnvVar, "1")
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--offline", "--exit-zero"})
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor env-quiet: %v\n%s", err, errBuf.String())
		}
		assertSingleOK(t, out.String(), "doctor", "overall=")
	})
}

func TestQuiet_Log_SingleOK(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		withFakeTTY(t, false, func() {
			root := NewRootCmd()
			root.SetArgs([]string{"log", "Quiet decision", "-r", "Why", "--quiet", "--no-commit"})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			if err := root.Execute(); err != nil {
				t.Fatalf("log quiet: %v\n%s", err, errBuf.String())
			}
			assertSingleOK(t, out.String(), "logged", "path=docs/decisions.md", "committed=false")
			if strings.Contains(out.String(), "✓ Logged") {
				t.Errorf("quiet log leaked the ✓ chatter to stdout: %q", out.String())
			}
		})
	})
}

func TestQuiet_Log_ErrorToStderr(t *testing.T) {
	cwd := t.TempDir() // no docs/ → error path
	f := &logFlags{stage: "all"}
	var out, errBuf bytes.Buffer
	if err := runLog(cwd, "x", f, true, strings.NewReader(""), &out, &errBuf); err == nil {
		t.Fatal("expected ErrSilent when docs/ missing")
	}
	if out.Len() != 0 {
		t.Errorf("quiet log error path wrote to stdout: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "Error: docs/ directory not found") {
		t.Errorf("error not routed to stderr under quiet: %q", errBuf.String())
	}
}

func TestQuiet_Show_ErrorToStderr(t *testing.T) {
	cwd := t.TempDir() // no docs/ → error path
	var out, errBuf bytes.Buffer
	if err := runShow(cwd, false, false, false, true, &out, &errBuf); err == nil {
		t.Fatal("expected ErrSilent when docs/ missing")
	}
	if out.Len() != 0 {
		t.Errorf("quiet show error path wrote to stdout: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "Error: docs/ directory not found") {
		t.Errorf("error not routed to stderr under quiet: %q", errBuf.String())
	}
}

func TestQuiet_Search_ErrorToStderr(t *testing.T) {
	cwd := t.TempDir() // no docs/ → error path
	var out, errBuf bytes.Buffer
	if err := runSearch(cwd, "term", &searchFlags{}, true, &out, &errBuf); err == nil {
		t.Fatal("expected ErrSilent when docs/ missing")
	}
	if out.Len() != 0 {
		t.Errorf("quiet search error path wrote to stdout: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "Error: docs/ directory not found") {
		t.Errorf("error not routed to stderr under quiet: %q", errBuf.String())
	}
}
