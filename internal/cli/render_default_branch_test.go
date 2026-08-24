package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/gitcli"
	"github.com/thrillmade/logmind/internal/templates"
	"github.com/thrillmade/logmind/internal/testgit"
)

// gitTry runs a git command in dir and hands back its combined output and
// error for the caller to judge. gitIn (search_clone_shapes_test.go) is the
// must-succeed wrapper over it, so the two contracts — "inspect the failure"
// and "fail the test" — are two call shapes rather than two copies of
// exec.Command.
//
// It is NOT the package's only git-exec site: `grep -c 'exec.Command("git"'
// internal/cli/*_test.go` counts 31 across 14 files. An earlier revision of
// this comment claimed it was the one, which was a claim about a sweep nobody
// had run.
//
// It was called gitIn until #301 merged #314, at which point two files each
// held a different `gitIn` and the package stopped compiling. The comment
// that used to sit here claimed the helper was "local to this file so it
// does not collide" — Go has no file-local scope, so that was never true;
// it only looked true while nothing else claimed the name.
func gitTry(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestRenderWorkflowTemplate_SubstitutesDefaultBranch pins the scaffold
// half of the "don't assume the default branch is main" fix.
//
// A workflow's `on:` filter cannot take an expression — GitHub evaluates
// no context under `on:` — so the branch the regen triggers on has to be
// baked in when `logmind init` writes the file. That makes this function
// the single point where the assumption could come back, either by
// dropping the substitution or by defaulting it to "main" unconditionally.
func TestRenderWorkflowTemplate_SubstitutesDefaultBranch(t *testing.T) {
	const placeholder = "__LOGMIND_DEFAULT_BRANCH__"
	// Both templates that filter on a branch, not just the one where the
	// defect was noticed.
	for _, name := range []string{
		"regen-timeline.yml.template",
		"check-doc-links.yml.template",
	} {
		tmpl := templates.Workflow(name)
		if !strings.Contains(tmpl, placeholder) {
			t.Fatalf("%s no longer carries %s — either the placeholder was renamed "+
				"or the default branch was hardcoded again", name, placeholder)
		}

		// A repo whose default branch is NOT main is the whole point, so
		// that is what the test renders with.
		got := renderWorkflowTemplate(tmpl, "trunk")
		if strings.Contains(got, placeholder) {
			t.Errorf("%s: renderWorkflowTemplate left %s unsubstituted", name, placeholder)
		}
		if !strings.Contains(got, "branches: [trunk]") {
			t.Errorf("%s: rendered trigger should be `branches: [trunk]`; the scaffolded workflow "+
				"would otherwise never fire in a repo whose default branch is not main", name)
		}
		if !strings.Contains(got, "SCAFFOLDED_BRANCH: trunk") {
			t.Errorf("%s: rendered file should carry `SCAFFOLDED_BRANCH: trunk` so drift is "+
				"detectable", name)
		}
		// The runtime expression must survive rendering untouched — it is
		// what makes a stale scaffolded value cost a trigger rather than a
		// wrong-ref write.
		if !strings.Contains(got, "DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}") {
			t.Errorf("%s: rendering must not disturb the runtime default_branch expression", name)
		}
	}

	// Control on the substitution mechanism itself, driven by a synthetic
	// probe rather than by a template. Deliberately NOT sourced from a real
	// template: no template currently contains __LOGMIND_VERSION__, so a
	// template-sourced control would either skip (hiding this whole test
	// from the suite) or pass vacuously. The probe makes both replacements
	// observable no matter what the templates happen to contain.
	const probe = "a __LOGMIND_VERSION__ b __LOGMIND_DEFAULT_BRANCH__ c"
	rendered := renderWorkflowTemplate(probe, "trunk")
	if strings.Contains(rendered, "__LOGMIND_DEFAULT_BRANCH__") {
		t.Errorf("control: the default-branch substitution is a no-op (%q)", rendered)
	}
	if !strings.Contains(rendered, "trunk") {
		t.Errorf("control: the default-branch substitution did not insert the branch (%q)", rendered)
	}
	if strings.Contains(rendered, "__LOGMIND_VERSION__") {
		t.Errorf("control: renderWorkflowTemplate stopped substituting __LOGMIND_VERSION__ (%q)", rendered)
	}
}

// TestRenderedWorkflow_TriggersOnTheRepositorysOwnBranch drives the whole
// scaffold path through the REAL resolver, for both a `main` repo and a
// non-`main` one, and checks the trigger that comes out the other end.
//
// Deliberately NOT tested here: the "git can tell us nothing" case. It is
// not environment-independent — gitcli.DefaultBranch's step 5 reads
// ambient `init.defaultBranch`, and macOS's Command Line Tools ships a
// gitconfig that sets it to `main`, so on a developer machine step 5
// answers for ANY directory and the hard fallback below it is unreachable.
// A test asserting a value there would be asserting the toolchain's
// configuration, and making it deterministic would mean overriding ambient
// git config, which this lane was told not to do. gitcli owns that rung
// and its own tests already hedge on it (see gitcli_test.go's "version +
// init.defaultBranch config; both are valid answers").
func TestRenderedWorkflow_TriggersOnTheRepositorysOwnBranch(t *testing.T) {
	mainRepo := t.TempDir()
	seedRepo(t, mainRepo, "main")

	trunkRepo := t.TempDir()
	seedRepo(t, trunkRepo, "trunk")

	// A `master` repo carrying a stray local `main`. This is the case the
	// old `branches: [main, master]` literal covered by accident and a
	// single rendered name does not: the resolver has to pick the right one,
	// and a fixed main-before-master preference picked `main` here, wiring
	// the trigger to a branch nothing is ever pushed to. Rendering one name
	// is only better than guessing two if the one rendered is correct, so
	// the end-to-end render is pinned on exactly that repo shape.
	masterRepo := t.TempDir()
	seedRepo(t, masterRepo, "master")
	if out, err := gitTry(masterRepo, "branch", "main"); err != nil {
		t.Fatalf("create the stray local main: %v\n%s", err, out)
	}

	for _, tc := range []struct{ label, dir, want string }{
		{"default branch is main", mainRepo, "main"},
		{"default branch is trunk", trunkRepo, "trunk"},
		{"default branch is master, with a stray local main", masterRepo, "master"},
	} {
		got := gitcli.DefaultBranch(tc.dir)
		if got != tc.want {
			t.Errorf("%s: gitcli.DefaultBranch = %q, want %q", tc.label, got, tc.want)
			continue
		}
		for _, name := range []string{
			"regen-timeline.yml.template",
			"check-doc-links.yml.template",
		} {
			rendered := renderWorkflowTemplate(templates.Workflow(name), got)
			// An empty substitution would render `branches: []` — a filter
			// that matches no branch — installing the workflow dead rather
			// than merely wrong, and nothing downstream would report it.
			if strings.Contains(rendered, "branches: []") {
				t.Errorf("%s / %s: rendered an empty branch filter", tc.label, name)
			}
			if !strings.Contains(rendered, "branches: ["+tc.want+"]") {
				t.Errorf("%s / %s: rendered trigger is not `branches: [%s]`", tc.label, name, tc.want)
			}
		}
	}
}

// seedRepo makes dir a git repo whose default branch is `branch`, without
// touching init.defaultBranch — the branch is renamed after the first
// commit so gitcli.DefaultBranch resolves it the way it would in a real
// repository rather than via the config-only rung.
func seedRepo(t *testing.T, dir, branch string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		if out, err := gitTry(dir, args...); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	// Through testgit, never bare `git init`: a repo missing gc.auto=0 +
	// maintenance.auto=false lets `git commit` spawn a detached
	// `git maintenance` that is still writing into .git/objects when
	// t.TempDir()'s cleanup runs (#271).
	testgit.InitRepo(t, dir, "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
	run("branch", "-M", branch)
}

// TestInstallWorkflowTemplates_RendersRepoDefaultBranch is the end-to-end
// half: a repository whose default branch is not `main` must get its own
// branch name written into the scaffolded workflow, not a guess.
func TestInstallWorkflowTemplates_RendersRepoDefaultBranch(t *testing.T) {
	repo := t.TempDir()
	seedRepo(t, repo, "trunk")

	if _, _, _, err := installWorkflowTemplates(repo, false); err != nil {
		t.Fatalf("installWorkflowTemplates: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(repo, ".github", "workflows", "regen-timeline.yml"))
	if err != nil {
		t.Fatalf("read scaffolded workflow: %v", err)
	}
	got := string(body)
	if strings.Contains(got, "__LOGMIND_DEFAULT_BRANCH__") {
		t.Errorf("scaffolded workflow still contains the raw placeholder")
	}
	if !strings.Contains(got, "branches: [trunk]") {
		t.Errorf("scaffolded workflow should trigger on `trunk`, this repo's actual default branch.\n"+
			"got trigger region:\n%s", triggerRegion(got))
	}
	if strings.Contains(got, "branches: [main]") {
		t.Errorf("scaffolded workflow hardcoded `main` in a repo whose default branch is `trunk`")
	}

	// Every OTHER scaffolded workflow must have been rendered too — the
	// placeholder is not a regen-timeline feature. check-doc-links carried
	// the same class as `[main, master]` and is fixed the same way; a
	// template added later that forgets the substitution fails here.
	entries, err := os.ReadDir(filepath.Join(repo, ".github", "workflows"))
	if err != nil {
		t.Fatalf("read scaffolded workflows: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no workflows scaffolded at all — this check would be vacuous")
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(repo, ".github", "workflows", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		text := string(data)
		if strings.Contains(text, "__LOGMIND_DEFAULT_BRANCH__") {
			t.Errorf("%s: raw placeholder survived scaffolding", e.Name())
		}
		if strings.Contains(stripYAMLComments(text), "branches: [main") {
			t.Errorf("%s: scaffolded a `main` branch filter into a repo whose default branch is "+
				"`trunk` — the default-branch guess came back", e.Name())
		}
	}
}

// stripYAMLComments drops whole-line `#` comments so assertions bind what
// a workflow DOES rather than what its header says it used to do — the
// headers deliberately name the literals they replaced.
func stripYAMLComments(body string) string {
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// triggerRegion extracts a few lines around the `branches:` filter for a
// readable failure message.
func triggerRegion(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if strings.Contains(l, "branches:") {
			lo, hi := i-2, i+2
			if lo < 0 {
				lo = 0
			}
			if hi > len(lines) {
				hi = len(lines)
			}
			return strings.Join(lines[lo:hi], "\n")
		}
	}
	return "(no `branches:` line found)"
}

// TestInstallWorkflowTemplates_RendersUnbornRepoDefaultBranch is the
// regression for the README's own Quick Start — `mkdir proj && cd proj &&
// git init && logmind init` — on a repo whose default branch is not `main`.
//
// The end-to-end test above seeds a repo by COMMITTING and then renaming, so
// gitcli.DefaultBranch answers off the single-born-branch rung. That left the
// case the Quick Start actually produces untested: `git init -b trunk` and
// nothing else. An unborn repo has no refs at all, so every rung that reads
// one declined and the scaffold fell through to the hard "main" — writing
// `branches: [main]` into a repo that has never had a branch by that name.
// Byte-identical to what a repo really on `main` gets, at exit 0, with
// `doctor` calling the repo healthy: both workflows install DEAD, and a check
// that never runs reports nothing at all.
//
// The assertion walks BY LINE rather than checking the `on:` trigger alone.
// Each template carries the placeholder twice — the trigger and the
// SCAFFOLDED_BRANCH env the drift warning compares against the live default
// branch — and a substitution that reached only the first would leave the
// warning permanently firing on a correctly-wired repo. Line indices survive
// the render because every substitution is in-line, so this covers a third
// occurrence added later without being told about it.
func TestInstallWorkflowTemplates_RendersUnbornRepoDefaultBranch(t *testing.T) {
	const placeholder = "__LOGMIND_DEFAULT_BRANCH__"
	for _, tc := range []struct {
		label  string
		branch string
	}{
		// THE DEFECT, and THE CONTROL: the `main` repo's scaffold was
		// already right and must stay right, or the fix has only moved
		// which repos get a dead workflow.
		{"the defect: unborn trunk repo", "trunk"},
		{"control: unborn main repo", "main"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			repo := t.TempDir()
			// `git init -b <branch>` and NOTHING else — no config, no
			// commit. Through testgit for the maintenance-spawn reason
			// seedRepo documents above.
			testgit.InitRepo(t, repo, "-q", "-b", tc.branch)
			// Errorf, deliberately NOT Fatalf: this reports the CAUSE, and
			// the assertions below report the SYMPTOM the user actually saw
			// in the scaffolded file. Stopping here would leave the symptom
			// unpinned and this test unable to show it ever caught anything.
			if got := gitcli.DefaultBranch(repo); got != tc.branch {
				t.Errorf("gitcli.DefaultBranch = %q, want %q", got, tc.branch)
			}

			if _, _, _, err := installWorkflowTemplates(repo, false); err != nil {
				t.Fatalf("installWorkflowTemplates: %v", err)
			}

			substituted := 0
			for _, name := range templates.ListWorkflowTemplates() {
				tmplLines := strings.Split(templates.Workflow(name), "\n")
				body, err := os.ReadFile(filepath.Join(repo, ".github", "workflows",
					strings.TrimSuffix(name, ".template")))
				if err != nil {
					t.Fatalf("read scaffolded %s: %v", name, err)
				}
				outLines := strings.Split(string(body), "\n")
				if len(tmplLines) != len(outLines) {
					t.Fatalf("%s: template has %d lines, scaffolded file has %d — rendering is "+
						"no longer line-for-line, so this probe cannot locate the occurrences",
						name, len(tmplLines), len(outLines))
				}
				for i, line := range tmplLines {
					if !strings.Contains(line, placeholder) {
						continue
					}
					substituted++
					if strings.Contains(outLines[i], placeholder) {
						t.Errorf("%s line %d: raw placeholder survived: %q", name, i+1, outLines[i])
					}
					if !strings.Contains(outLines[i], tc.branch) {
						t.Errorf("%s line %d: %q does not carry this repo's default branch %q",
							name, i+1, outLines[i], tc.branch)
					}
				}
			}
			// The zero control: a renamed placeholder would make every
			// assertion above vacuous and this test silently green.
			if substituted < 4 {
				t.Fatalf("only %d placeholder occurrence(s) checked — expected at least 4 "+
					"(a trigger and a SCAFFOLDED_BRANCH in each of two workflows); "+
					"the placeholder was renamed or the templates stopped carrying it", substituted)
			}
		})
	}
}
