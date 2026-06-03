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

	"github.com/thrillmade/logmind/internal/clierr"
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

// TestCopyTree_PreservesExecutableBit covers Bug 3 from PR #136 review:
// when copying skill files into the catalog clone, executable scripts
// (e.g., `scripts/helper.sh`) MUST keep their `+x` bit. Without this,
// downstream consumers cloning the catalog repo would silently get a
// non-runnable script and have to re-chmod after install.
//
// We exercise copyTree directly (rather than through pushWith) so the
// test stays focused on the perm-bit invariant and doesn't need to
// stub out git/gh.
func TestCopyTree_PreservesExecutableBit(t *testing.T) {
	if os.Getuid() == 0 {
		// Some CI runners run as root with permissive umasks that
		// confuse the perm-bit assertion. Skip rather than chase
		// platform-specific gymnastics.
		t.Skip("running as root; perm bits behave unpredictably")
	}

	src := t.TempDir()
	dst := t.TempDir()

	// Two files: one executable script and one plain markdown body.
	// listCompanionFiles emits slash-separated relative paths, so we
	// pass the same shape to copyTree.
	scriptRel := "scripts/helper.sh"
	mdRel := "REFERENCES.md"

	if err := os.MkdirAll(filepath.Join(src, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, scriptRel),
		[]byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("write helper.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, mdRel),
		[]byte("# refs\n"), 0o644); err != nil {
		t.Fatalf("write REFERENCES.md: %v", err)
	}

	if err := copyTree(src, dst, []string{scriptRel, mdRel}); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	// Load-bearing assertion: the script's executable bit survived.
	scriptInfo, err := os.Stat(filepath.Join(dst, scriptRel))
	if err != nil {
		t.Fatalf("stat dst script: %v", err)
	}
	// Mask to the user-execute bit since group/other vary by umask.
	if scriptInfo.Mode().Perm()&0o100 == 0 {
		t.Errorf("dest script lost executable bit; mode = %v", scriptInfo.Mode().Perm())
	}

	// Cross-check: the plain markdown file did NOT magically acquire
	// the executable bit — the fix preserves source mode, not blanket-grants.
	mdInfo, err := os.Stat(filepath.Join(dst, mdRel))
	if err != nil {
		t.Fatalf("stat dst md: %v", err)
	}
	if mdInfo.Mode().Perm()&0o100 != 0 {
		t.Errorf("dest md file unexpectedly executable; mode = %v", mdInfo.Mode().Perm())
	}
}

// TestPush_RejectsPathTraversalSkillName covers Bug 4 from PR #136
// review: a skill name carrying `..`, `/`, or `\` would have
// filepath.Join escape both the local skills tree (on read) and the
// catalog clone's skills tree (on write). pushWith MUST reject such
// names with ErrInvalidSkillName BEFORE any filesystem activity.
//
// We assert both: (a) the right sentinel comes back and (b) the
// pushWith call did NOT touch the supplied tempdir (no errant
// SKILL.md creation, no stat racing, no clone, etc.).
func TestPush_RejectsPathTraversalSkillName(t *testing.T) {
	cases := []struct {
		Name      string
		SkillName string
	}{
		{"dotdot", "../foo"},
		{"dotdot-deeper", "../../foo"},
		{"embedded-dotdot", "foo/../bar"},
		{"forward-slash", "foo/bar"},
		{"backslash", `foo\bar`},
		{"leading-dot", ".hidden"},
		{"empty", ""},
		{"uppercase", "Foo"},
		{"whitespace", "foo bar"},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			root := t.TempDir()

			// Pre-snapshot the tempdir so we can prove nothing was
			// created during the rejected call. We expect an empty
			// directory after pushWith returns ErrInvalidSkillName.
			entriesBefore, err := os.ReadDir(root)
			if err != nil {
				t.Fatalf("readdir before: %v", err)
			}

			var stdout bytes.Buffer
			_, err = pushWith(PushOptions{
				SkillName:      c.SkillName,
				CatalogTarget:  "thrillmade/agent-skills",
				DryRun:         true, // shouldn't matter — validation runs first
				SourceRepoRoot: root,
				Stdout:         &stdout,
			}, newFakeRunner(), newFakeRunner())

			if !errors.Is(err, ErrInvalidSkillName) {
				t.Fatalf("want ErrInvalidSkillName for %q; got %v",
					c.SkillName, err)
			}

			// The user-facing message should name the offending input
			// so the error is actionable.
			if !strings.Contains(stdout.String(), "is not a valid slug") {
				t.Errorf("expected actionable error line; got %q",
					stdout.String())
			}

			// No filesystem side-effects: the tempdir entry count is
			// unchanged.
			entriesAfter, err := os.ReadDir(root)
			if err != nil {
				t.Fatalf("readdir after: %v", err)
			}
			if len(entriesAfter) != len(entriesBefore) {
				t.Errorf("pushWith touched filesystem despite rejection; before=%d entries, after=%d",
					len(entriesBefore), len(entriesAfter))
			}
		})
	}
}

// TestPush_AcceptsValidSlugs is the positive complement to
// TestPush_RejectsPathTraversalSkillName: kebab + dot + underscore
// names that match SPEC §1.10.1 must continue to work. We exercise
// just the dry-run preflight so the test stays self-contained.
func TestPush_AcceptsValidSlugs(t *testing.T) {
	cases := []string{
		"critical-issues-only",
		"foo",
		"a",
		"foo.bar",
		"foo_bar",
		"foo-bar-1",
		"123abc",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeSampleSkill(t, root, name, "desc")

			git := newFakeRunner()
			git.When([]string{"rev-parse", "HEAD"},
				runReply{Stdout: "abcdef1234567\n"})

			_, err := pushWith(PushOptions{
				SkillName:      name,
				CatalogTarget:  "thrillmade/agent-skills",
				DryRun:         true,
				SourceRepoRoot: root,
				Stdout:         io.Discard,
			}, git, newFakeRunner())
			if err != nil {
				t.Errorf("valid slug %q rejected: %v", name, err)
			}
		})
	}
}

// --- §8.2 privacy gate, layers 1+2 ----------------------------------
//
// First slice of the master plan's belt-and-braces privacy gate. Layer
// 3 (content scanner) + layer 4 (repo-visibility check) are queued for
// wave-2 and intentionally NOT exercised here. These tests pin the
// rejection paths (frontmatter markers + directory convention) and
// assert no filesystem side-effects fire when the gate trips.
//
// Shared assertion helper: privacy-gate rejections must satisfy BOTH
// errors.Is(err, ErrPrivateSkill) and errors.Is(err, clierr.ErrSilent)
// so the cli layer can translate to exit-1-silent without re-printing,
// and downstream code can recognise the rejection category by name.
func assertPrivateSkillError(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrPrivateSkill) {
		t.Fatalf("want errors.Is(err, ErrPrivateSkill); got %v", err)
	}
	if !errors.Is(err, clierr.ErrSilent) {
		t.Fatalf("want errors.Is(err, clierr.ErrSilent); got %v", err)
	}
}

// writePrivateSkillUnderSkillsPrivate drops a SKILL.md at
// `.claude/skills-private/<name>/SKILL.md` so the layer-2 directory
// convention check can fire. extraFrontmatter is appended verbatim
// inside the YAML block — used by the override test to insert a
// `private: false` line and prove placement still wins.
func writePrivateSkillUnderSkillsPrivate(t *testing.T, root, name, description, extraFrontmatter string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "skills-private", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skills-private dir: %v", err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n"
	if extraFrontmatter != "" {
		body += extraFrontmatter
		if !strings.HasSuffix(extraFrontmatter, "\n") {
			body += "\n"
		}
	}
	body += "---\n\n# Title\n\nBody text.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

// noFilesystemSideEffects pre-snapshots tempdir entries and asserts
// the count is unchanged after pushWith returns. We deliberately
// check entry count rather than diff trees — the directory the test
// pre-created (e.g., `.claude/skills/demo/`) must of course still be
// there; what we're proving is that pushWith did NOT create the
// catalog cache dir, mutate the skill body, or otherwise touch the
// filesystem AFTER the gate tripped.
func noFilesystemSideEffects(t *testing.T, root string, before []os.DirEntry) {
	t.Helper()
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("readdir after: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("pushWith touched filesystem despite privacy-gate rejection: before=%d, after=%d",
			len(before), len(after))
	}
}

func TestPush_PrivateFrontmatter_RejectedBeforeClone(t *testing.T) {
	// Layer 1 marker: `private: true`. Drop into the SKILL.md
	// frontmatter so the existing CheckFrontmatter still passes (name +
	// description present); the gate then catches the private flag and
	// rejects before any clone work runs.
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: demo\ndescription: A trigger.\nprivate: true\n---\n\n# body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-snapshot the .claude/ subtree state so we can prove pushWith
	// didn't, e.g., create a catalog cache dir AT ALL. We snapshot the
	// .claude/skills/demo/ dir specifically because that's where the
	// test setup already wrote files; the root tempdir has the same
	// shape before and after as long as the gate fires correctly.
	skillDirEntries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	git := newFakeRunner()
	gh := newFakeRunner()
	var stdout bytes.Buffer
	_, err = pushWith(PushOptions{
		SkillName:      "demo",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         false, // not a dry-run: prove the gate beats real-clone path
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, git, gh)

	assertPrivateSkillError(t, err)

	// Error message must point at the offending field by name + the
	// catalog target so the user knows what to edit.
	wantInOutput := []string{
		"skill demo is marked private",
		"private: true",
		"not pushing to thrillmade/agent-skills",
		"Remove the marker OR move the skill to a different catalog target",
	}
	for _, w := range wantInOutput {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q:\n%s", w, stdout.String())
		}
	}

	// Layer 1 fires AFTER reading SKILL.md but BEFORE clone — assert no
	// `git clone` (or any other git call past the provenance preview)
	// fired. The gate is supposed to reject before the gh-auth check
	// too, so no `gh auth status` should have happened.
	for _, c := range git.calls {
		if len(c.Args) > 0 && c.Args[0] == "clone" {
			t.Errorf("layer-1 gate let git clone fire: %+v", c)
		}
	}
	if len(gh.calls) != 0 {
		t.Errorf("layer-1 gate touched gh; calls: %+v", gh.calls)
	}

	// Skill dir entries unchanged (no in-place edits to SKILL.md or
	// creation of sibling provenance files).
	afterEntries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEntries) != len(skillDirEntries) {
		t.Errorf("layer-1 gate mutated skill dir: before=%d, after=%d",
			len(skillDirEntries), len(afterEntries))
	}
}

func TestPush_DoNotPromoteFrontmatter_Rejected(t *testing.T) {
	// Alternate spelling: `do-not-promote: true`. Same rejection path
	// as `private: true` — the gate honours both names because the
	// catalog ecosystem hasn't settled on a single canonical field.
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: demo\ndescription: A trigger.\ndo-not-promote: true\n---\n\n# body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	_, err := pushWith(PushOptions{
		SkillName:      "demo",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         true,
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, newFakeRunner(), newFakeRunner())

	assertPrivateSkillError(t, err)

	// The error message points at the do-not-promote field specifically,
	// not at the generic "private" alias — the field that triggered the
	// gate is what the user needs to find in their SKILL.md to fix.
	if !strings.Contains(stdout.String(), "do-not-promote: true") {
		t.Errorf("expected do-not-promote field named in error; got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "skill demo is marked private") {
		t.Errorf("expected canonical rejection phrasing; got %q", stdout.String())
	}
}

func TestPush_SkillsPrivateDir_RejectedByConvention(t *testing.T) {
	// Layer 2: skill lives under .claude/skills-private/<name>/. No
	// frontmatter private-flag needed — the directory placement alone
	// is the signal. Reject without ever reading the body's frontmatter
	// (we can't even know it's well-formed; the gate fires earlier).
	root := t.TempDir()
	writePrivateSkillUnderSkillsPrivate(t, root, "secret-skill", "A trigger.", "")

	// Snapshot tempdir BEFORE — prove the layer-2 gate doesn't create a
	// catalog cache dir or anything else after rejection. The
	// .claude/skills-private/secret-skill/ subtree already exists from
	// the writer above and is the baseline we compare against.
	entriesBefore, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	git := newFakeRunner()
	gh := newFakeRunner()
	var stdout bytes.Buffer
	_, err = pushWith(PushOptions{
		SkillName:      "secret-skill",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         false, // not a dry-run; prove the gate beats real-clone path
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, git, gh)

	assertPrivateSkillError(t, err)

	wantInOutput := []string{
		"skill secret-skill lives under .claude/skills-private/",
		"treated as private by convention",
		"Move to .claude/skills/secret-skill/",
		"add private: false explicit override",
	}
	for _, w := range wantInOutput {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q:\n%s", w, stdout.String())
		}
	}

	// Layer 2 fires BEFORE the SKILL.md read — so no git or gh calls at
	// all. Assert both runners stayed idle.
	if len(git.calls) != 0 {
		t.Errorf("layer-2 gate touched git; calls: %+v", git.calls)
	}
	if len(gh.calls) != 0 {
		t.Errorf("layer-2 gate touched gh; calls: %+v", gh.calls)
	}

	// No filesystem mutation after the gate trips.
	noFilesystemSideEffects(t, root, entriesBefore)
}

func TestPush_SkillsPrivateDir_BeatsExplicitFalseOverride(t *testing.T) {
	// Override precedence: skill is under skills-private/, AND its
	// frontmatter explicitly says `private: false`. Directory convention
	// MUST still win — the master plan §8.2 model is that placement is
	// the primary signal; the frontmatter override is for flipping a
	// PUBLIC skill TO private, not for un-marking a private path.
	//
	// This test specifically pins the precedence so a future contributor
	// can't accidentally "fix" the gate to defer to the frontmatter.
	root := t.TempDir()
	writePrivateSkillUnderSkillsPrivate(t, root, "secret-skill", "A trigger.",
		"private: false")

	var stdout bytes.Buffer
	_, err := pushWith(PushOptions{
		SkillName:      "secret-skill",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         true,
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, newFakeRunner(), newFakeRunner())

	assertPrivateSkillError(t, err)

	// The rejection wording is the layer-2 message — NOT the layer-1
	// frontmatter message — proving the dir gate fired first.
	if !strings.Contains(stdout.String(), "lives under .claude/skills-private/") {
		t.Errorf("expected layer-2 (dir-convention) rejection wording; got %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "frontmatter") {
		t.Errorf("layer-1 message leaked even though layer-2 fired first; got %q", stdout.String())
	}
}

func TestPush_HappyPath_NoPrivacyMarkers_Unaffected(t *testing.T) {
	// Belt-and-braces sanity: a plain skill with no `private:` /
	// `do-not-promote:` field, living under `.claude/skills/`, MUST
	// still flow through the dry-run preflight unchanged. The privacy
	// gate is purely additive — no regression on the existing path.
	root := t.TempDir()
	writeSampleSkill(t, root, "demo", "A precise trigger.")

	git := newFakeRunner()
	git.When([]string{"rev-parse", "HEAD"}, runReply{Stdout: "abcdef1234567\n"})

	var stdout bytes.Buffer
	_, err := pushWith(PushOptions{
		SkillName:      "demo",
		CatalogTarget:  "thrillmade/agent-skills",
		DryRun:         true,
		SourceRepoRoot: root,
		Stdout:         &stdout,
	}, git, newFakeRunner())
	if err != nil {
		t.Fatalf("happy path regressed under privacy gate: %v", err)
	}
	// Dry-run summary line should still fire — the gate doesn't
	// short-circuit valid skills.
	if !strings.Contains(stdout.String(), "ok skill: push demo dry-run") {
		t.Errorf("dry-run summary missing — gate may have over-fired: %q", stdout.String())
	}
}

func TestScanPrivateFrontmatterField_DetectsAllAcceptedSpellings(t *testing.T) {
	// Unit-level coverage on the parser so we don't have to spin up a
	// full pushWith for every boolean spelling. Both field names + all
	// three YAML boolean-true spellings should match; common
	// near-misses must NOT.
	cases := []struct {
		Name     string
		Body     string
		WantHit  bool
		WantName string
	}{
		// True-positives — each field × each accepted boolean spelling.
		{"private-true", "---\nname: a\ndescription: b\nprivate: true\n---\nbody", true, "private"},
		{"private-yes", "---\nname: a\ndescription: b\nprivate: yes\n---\nbody", true, "private"},
		{"private-on", "---\nname: a\ndescription: b\nprivate: on\n---\nbody", true, "private"},
		{"private-TRUE-caps", "---\nname: a\ndescription: b\nprivate: TRUE\n---\nbody", true, "private"},
		{"dnp-true", "---\nname: a\ndescription: b\ndo-not-promote: true\n---\nbody", true, "do-not-promote"},
		{"indented-private", "---\nname: a\ndescription: b\n  private: true\n---\nbody", true, "private"},
		// True-negatives — these must NOT trip the gate.
		{"private-false", "---\nname: a\ndescription: b\nprivate: false\n---\nbody", false, ""},
		{"private-no", "---\nname: a\ndescription: b\nprivate: no\n---\nbody", false, ""},
		{"private-numeric-one", "---\nname: a\ndescription: b\nprivate: 1\n---\nbody", false, ""},
		{"private-quoted-string", "---\nname: a\ndescription: b\nprivate: \"true\"\n---\nbody", false, ""},
		{"no-frontmatter", "no frontmatter at all", false, ""},
		{"unterminated-frontmatter", "---\nname: a\nprivate: true\n", false, ""},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got, hit := scanPrivateFrontmatterField(c.Body)
			if hit != c.WantHit {
				t.Errorf("hit = %v; want %v (body=%q)", hit, c.WantHit, c.Body)
			}
			if got != c.WantName {
				t.Errorf("field = %q; want %q", got, c.WantName)
			}
		})
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
