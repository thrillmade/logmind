package skill

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner is a tiny gitRunner/ghRunner stub. Tests pre-load a
// scripted response per [cmd-prefix → reply] entry. Calls that don't
// match a scripted prefix return success with empty stdout — keeps
// happy-path tests terse.
type fakeRunner struct {
	calls   []runCall
	replies map[string]runReply
}

type runCall struct {
	Dir  string
	Args []string
}

type runReply struct {
	Stdout string
	Stderr string
	Err    error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{replies: map[string]runReply{}}
}

// scriptKey concatenates args with a single space — runners look up
// the exact arg sequence so different sub-verbs (e.g. "config --get"
// vs "log -1") can be wired separately.
func scriptKey(args ...string) string { return strings.Join(args, " ") }

func (f *fakeRunner) When(args []string, r runReply) {
	f.replies[scriptKey(args...)] = r
}

func (f *fakeRunner) Run(dir string, args ...string) (string, string, error) {
	f.calls = append(f.calls, runCall{Dir: dir, Args: append([]string{}, args...)})
	if r, ok := f.replies[scriptKey(args...)]; ok {
		return r.Stdout, r.Stderr, r.Err
	}
	return "", "", nil
}

// writeSampleSkill drops a minimal-but-valid SKILL.md (+ companion
// file) into <root>/.claude/skills/<name>/.
func writeSampleSkill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Title\n\nBody text.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func TestPush_DryRun_HappyPath(t *testing.T) {
	root := t.TempDir()
	writeSampleSkill(t, root, "demo", "A precise trigger.")

	// Add a companion file so the file list isn't just SKILL.md.
	if err := os.WriteFile(
		filepath.Join(root, ".claude", "skills", "demo", "REFERENCES.md"),
		[]byte("# References\n"), 0o644,
	); err != nil {
		t.Fatalf("write companion: %v", err)
	}

	git := newFakeRunner()
	// Provide source-repo provenance so the dry-run preview includes it.
	git.When([]string{"config", "--get", "remote.origin.url"},
		runReply{Stdout: "https://github.com/thrillmade/logmind.git"})
	git.When([]string{"rev-parse", "HEAD"},
		runReply{Stdout: "abcdef1234567890\n"})
	git.When([]string{"log", "-1", "--format=%an <%ae>"},
		runReply{Stdout: "Ada <ada@example.com>\n"})

	gh := newFakeRunner() // unused on dry-run

	var stdout bytes.Buffer
	res, err := pushWith(PushOptions{
		SkillName:      "demo",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         true,
		SourceRepoRoot: root,
		Now:            time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
		Stdout:         &stdout,
	}, git, gh)
	if err != nil {
		t.Fatalf("pushWith dry-run: %v", err)
	}

	out := stdout.String()
	mustContain := func(s string) {
		t.Helper()
		if !strings.Contains(out, s) {
			t.Errorf("stdout missing %q:\n%s", s, out)
		}
	}
	mustContain("→ Pushing skill 'demo' to thrillmade/agent-skills")
	mustContain("thrillmade/logmind @ abcdef1")
	mustContain("REFERENCES.md")
	mustContain("Dry-run: skipping clone, push, and PR creation.")
	mustContain("ok skill: push demo dry-run")

	if res.PRURL != "" {
		t.Errorf("dry-run should not set PRURL; got %q", res.PRURL)
	}
	if res.Branch == "" {
		t.Errorf("dry-run should still compute Branch")
	}
	if !strings.HasPrefix(res.Branch, "skill/demo-from-thrillmade-logmind-abcdef1") {
		t.Errorf("Branch shape wrong: %q", res.Branch)
	}
	if got, want := res.CatalogTarget, "thrillmade/agent-skills"; got != want {
		t.Errorf("CatalogTarget = %q; want %q", got, want)
	}

	// Dry-run must NOT touch gh.
	if len(gh.calls) != 0 {
		t.Errorf("dry-run touched gh; calls: %+v", gh.calls)
	}
}

func TestPush_MissingSkill_ReturnsSentinel(t *testing.T) {
	root := t.TempDir()
	var stdout bytes.Buffer
	_, err := pushWith(PushOptions{
		SkillName:      "ghost",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         true,
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, newFakeRunner(), newFakeRunner())
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("missing skill: want ErrSkillNotFound, got %v", err)
	}
	if !strings.Contains(stdout.String(), "Error: skill 'ghost' not found at") {
		t.Errorf("missing skill: stdout = %q", stdout.String())
	}
}

func TestPush_InvalidCatalogTarget_ReturnsSentinel(t *testing.T) {
	root := t.TempDir()
	writeSampleSkill(t, root, "demo", "desc")
	var stdout bytes.Buffer
	_, err := pushWith(PushOptions{
		SkillName:      "demo",
		CatalogTarget:  "not-a-slug",
		DryRun:         true,
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, newFakeRunner(), newFakeRunner())
	if !errors.Is(err, ErrInvalidCatalogTarget) {
		t.Fatalf("bad catalog: want ErrInvalidCatalogTarget, got %v", err)
	}
	if !strings.Contains(stdout.String(),
		"Error: catalog target 'not-a-slug' is not a valid <owner>/<repo>") {
		t.Errorf("bad catalog: stdout = %q", stdout.String())
	}
}

func TestPush_BadFrontmatter_Rejected(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills", "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No frontmatter at all.
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Title\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	_, err := pushWith(PushOptions{
		SkillName:      "broken",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         true,
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, newFakeRunner(), newFakeRunner())
	if err == nil {
		t.Fatalf("bad frontmatter: expected non-nil error")
	}
	if !strings.Contains(stdout.String(), "must start with YAML frontmatter") {
		t.Errorf("expected frontmatter error printed; got %q", stdout.String())
	}
}

func TestPush_NoSourceRemote_FallsBackToLocal(t *testing.T) {
	root := t.TempDir()
	writeSampleSkill(t, root, "demo", "desc")

	git := newFakeRunner()
	// remote.origin.url not configured → exec returns non-zero.
	git.When([]string{"config", "--get", "remote.origin.url"},
		runReply{Err: errors.New("no remote")})
	// Still need a HEAD sha (otherwise the branch suffix is "unknown").
	git.When([]string{"rev-parse", "HEAD"},
		runReply{Stdout: "0123456abcdef\n"})

	var stdout bytes.Buffer
	res, err := pushWith(PushOptions{
		SkillName:      "demo",
		CatalogTarget:  "acme/private-skills",
		DryRun:         true,
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, git, newFakeRunner())
	if err != nil {
		t.Fatalf("no remote: %v", err)
	}
	if res.SourceRepo != "" {
		t.Errorf("SourceRepo should be empty; got %q", res.SourceRepo)
	}
	out := stdout.String()
	if !strings.Contains(out, "(local, no remote)") {
		t.Errorf("missing local fallback marker:\n%s", out)
	}
	// Branch should still be valid (uses "local" suffix).
	if !strings.Contains(res.Branch, "-from-local-") {
		t.Errorf("branch fallback suffix wrong: %q", res.Branch)
	}
}

func TestPush_GhNotAuthed_NotDryRun_ReturnsSentinel(t *testing.T) {
	root := t.TempDir()
	writeSampleSkill(t, root, "demo", "desc")

	git := newFakeRunner()
	git.When([]string{"config", "--get", "remote.origin.url"},
		runReply{Stdout: "https://github.com/thrillmade/logmind.git"})
	git.When([]string{"rev-parse", "HEAD"},
		runReply{Stdout: "abcdef1234\n"})

	gh := newFakeRunner()
	gh.When([]string{"auth", "status"}, runReply{
		Stderr: "You are not logged in to any GitHub hosts. Run gh auth login to authenticate.",
		Err:    errors.New("exit 1"),
	})

	var stdout bytes.Buffer
	_, err := pushWith(PushOptions{
		SkillName:      "demo",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         false,
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, git, gh)
	if !errors.Is(err, ErrGhNotAuthed) {
		t.Fatalf("expected ErrGhNotAuthed; got %v", err)
	}
}

func TestListCompanionFiles_SkipsHiddenAndProvenance(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"SKILL.md":            "skill body",
		"PROVENANCE.md":       "counter",
		".DS_Store":           "noise",
		"REFERENCES.md":       "refs",
		"scripts/helper.sh":   "#!/bin/sh\n",
		"references/note.txt": "note",
	}
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := listCompanionFiles(dir)
	if err != nil {
		t.Fatalf("listCompanionFiles: %v", err)
	}
	want := []string{"REFERENCES.md", "references/note.txt", "scripts/helper.sh"}
	if len(got) != len(want) {
		t.Fatalf("companion list = %v; want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestParseGitHubSlug(t *testing.T) {
	cases := map[string]string{
		"https://github.com/owner/repo":       "owner/repo",
		"https://github.com/owner/repo.git":   "owner/repo",
		"git@github.com:owner/repo.git":       "owner/repo",
		"ssh://git@github.com/owner/repo.git": "owner/repo",
		"git://github.com/owner/repo":         "owner/repo",
		"https://gitlab.com/owner/repo":       "",
		"":                                    "",
		"not-a-url":                           "",
	}
	for in, want := range cases {
		if got := parseGitHubSlug(in); got != want {
			t.Errorf("parseGitHubSlug(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSafeBranchSegment(t *testing.T) {
	cases := map[string]string{
		"thrillmade/logmind":   "thrillmade-logmind",
		"foo bar/baz":          "foo-bar-baz",
		"critical-issues-only": "critical-issues-only",
		"//slashes//":          "slashes",
		"":                     "",
	}
	for in, want := range cases {
		if got := safeBranchSegment(in); got != want {
			t.Errorf("safeBranchSegment(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSummarizeDescription_ExtractsDescriptionLine(t *testing.T) {
	body := "---\nname: foo\ndescription: A focused one-liner.\n---\n\n# body\n"
	if got := summarizeDescription(body); got != "A focused one-liner." {
		t.Errorf("summarizeDescription = %q", got)
	}
}

func TestSummarizeDescription_EmptyOnMissingFrontmatter(t *testing.T) {
	if got := summarizeDescription("plain body"); got != "" {
		t.Errorf("summarizeDescription on no frontmatter = %q; want empty", got)
	}
}

func TestRenderProvenance_ContainsKeyFields(t *testing.T) {
	res := PushResult{
		SourceRepo:   "thrillmade/logmind",
		SourceCommit: "abcdef1234",
		SourceAuthor: "Ada <ada@example.com>",
	}
	out := renderProvenance("demo", res, time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), "A trigger.")
	for _, want := range []string{
		"skill: demo",
		`description: "A trigger."`,
		"source_repo: thrillmade/logmind",
		"source_commit: abcdef1234",
		`source_author: "Ada <ada@example.com>"`,
		"pushed_at: 2026-06-03T00:00:00Z",
		"pushed_via: logmind skill push",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("renderProvenance missing %q:\n%s", want, out)
		}
	}
}

func TestRenderPRBody_LinksSourceRepoAndCommit(t *testing.T) {
	res := PushResult{
		SourceRepo:   "thrillmade/logmind",
		SourceCommit: "abcdef1234567890",
	}
	body := renderPRBody("demo", res, time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), "A trigger.")
	for _, want := range []string{
		"## Skill proposal: `demo`",
		"> A trigger.",
		"https://github.com/thrillmade/logmind",
		"https://github.com/thrillmade/logmind/commit/abcdef1234567890",
		"Acceptance checklist:",
		"`logmind skill push`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("renderPRBody missing %q:\n%s", want, body)
		}
	}
}

// Compile-time check that fakeRunner satisfies both runner interfaces.
var _ gitRunner = (*fakeRunner)(nil)
var _ ghRunner = (*fakeRunner)(nil)

// Smoke: io.Discard is the default stdout — ensure pushWith doesn't
// nil-deref when callers omit it.
func TestPush_NilStdoutDefaultsToDiscard(t *testing.T) {
	root := t.TempDir()
	writeSampleSkill(t, root, "demo", "desc")
	git := newFakeRunner()
	git.When([]string{"rev-parse", "HEAD"}, runReply{Stdout: "abcdef1234567\n"})
	_, err := pushWith(PushOptions{
		SkillName:      "demo",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         true,
		SourceRepoRoot: root,
		Stdout:         nil, // explicit nil
	}, git, newFakeRunner())
	if err != nil {
		t.Fatalf("nil stdout: %v", err)
	}
	// (Test relies on the absence of a panic. No further assertion.)
	_ = io.Discard
}
