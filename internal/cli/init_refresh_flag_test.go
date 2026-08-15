package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/templates"
)

// runInitIn drives the real CLI (`logmind init …`) with cwd set to dir and
// returns combined stdout+stderr. Going through NewRootCmd rather than
// calling runInit is deliberate: the defect being fixed here was that the
// FLAG did not exist, so a test that bypasses flag parsing would have gone
// green against the broken tree.
func runInitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	origin, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origin) }()

	root := NewRootCmd()
	root.SetArgs(append([]string{"init"}, args...))
	var sink bytes.Buffer
	root.SetOut(&sink)
	root.SetErr(&sink)
	if err := root.Execute(); err != nil {
		t.Fatalf("logmind init %v: %v\n%s", args, err, sink.String())
	}
	return sink.String()
}

func readWorkflow(t *testing.T, dir, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}

// TestInitRefresh_RepairsAStaleScaffoldedTrigger is the regression for a
// remediation message that named a command which did not exist.
//
// Every workflow logmind scaffolds that filters on a branch carries the
// repository's default branch as a LITERAL — an `on:` filter takes no
// expression, so it is rendered at scaffold time. Rename the default branch
// and that literal is stale: the workflow stops firing, silently. The
// shipped workflows detect this and print `Fix: run 'logmind init --refresh'
// to rewrite the trigger.`
//
// Two things were wrong. `--refresh` was not a flag (`Error: unknown flag`),
// and bare `logmind init` cannot substitute for it: its refresh only rewrites
// a workflow whose template marker is OLDER than the bundled one, and a
// stale trigger at the current version moves no marker. So the repo reported
// "All workflow templates already current" while wired to a branch that no
// longer existed.
//
// The bare-init leg below is the control: without it, `--refresh` passing
// would prove nothing about whether it was needed.
func TestInitRefresh_RepairsAStaleScaffoldedTrigger(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")

	runInitIn(t, dir, "--no-git")
	for _, name := range []string{"regen-timeline.yml", "check-doc-links.yml"} {
		if !strings.Contains(readWorkflow(t, dir, name), "branches: [release]") {
			t.Fatalf("setup: %s was not scaffolded for `release`", name)
		}
	}

	// The default branch is renamed. Nothing about the template version
	// changed, so nothing a version-ordered refresh looks at changed either.
	if out, err := gitIn(dir, "branch", "-M", "release2"); err != nil {
		t.Fatalf("rename the default branch: %v\n%s", err, out)
	}

	// CONTROL: bare `logmind init` leaves the stale trigger in place. If this
	// leg ever starts repairing it, the assertions below stop measuring
	// --refresh.
	bare := runInitIn(t, dir, "--no-git")
	if !strings.Contains(bare, "All workflow templates already current.") {
		t.Errorf("control: bare `init` was expected to report the templates current; got:\n%s", bare)
	}
	for _, name := range []string{"regen-timeline.yml", "check-doc-links.yml"} {
		if !strings.Contains(readWorkflow(t, dir, name), "branches: [release]") {
			t.Fatalf("control: bare `init` rewrote %s's trigger — this test no longer isolates --refresh", name)
		}
	}

	// THE FIX: the command the shipped warning names.
	out := runInitIn(t, dir, "--no-git", "--refresh")
	for _, name := range []string{"regen-timeline.yml", "check-doc-links.yml"} {
		body := readWorkflow(t, dir, name)
		if !strings.Contains(body, "branches: [release2]") {
			t.Errorf("%s: `init --refresh` did not rewrite the trigger to the live default branch.\n"+
				"got trigger region:\n%s\ncommand output:\n%s", name, triggerRegion(body), out)
		}
		if strings.Contains(body, "branches: [release]\n") {
			t.Errorf("%s: the stale `release` trigger survived --refresh", name)
		}
		if !strings.Contains(body, "SCAFFOLDED_BRANCH: release2") {
			t.Errorf("%s: the drift sentinel was not re-rendered, so the next rename goes unreported", name)
		}
		if !strings.Contains(out, "Refreshed .github/workflows/"+name) {
			t.Errorf("%s: --refresh rewrote the file but did not say so.\noutput:\n%s", name, out)
		}
	}

	// Idempotent: a second --refresh has nothing to do and must not claim
	// otherwise, or the line stops meaning anything.
	again := runInitIn(t, dir, "--no-git", "--refresh")
	if strings.Contains(again, "Refreshed .github/workflows/") {
		t.Errorf("a second --refresh reported a rewrite with nothing to rewrite:\n%s", again)
	}
	if !strings.Contains(again, "All workflow templates already current.") {
		t.Errorf("a second --refresh should report the templates current; got:\n%s", again)
	}
}

// TestInitRefresh_DoesNotWidenOwnership pins the half of the ruling that is
// a refusal rather than a capability: `--refresh` is a bigger hammer, not a
// wider claim.
//
// SPEC §5.2 makes the `# logmind-template-version:` marker the thing that
// makes a file in .github/workflows/ logmind's to rewrite. A file with no
// marker is the user's — they stripped it, or they wrote their own workflow
// under a name logmind happens to ship — and a marker NEWER than the
// bundled one belongs to a repo running ahead of this binary (#286). Neither
// may be touched, and "the user asked for --refresh" is not a licence.
func TestInitRefresh_DoesNotWidenOwnership(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")
	runInitIn(t, dir, "--no-git")

	if out, err := gitIn(dir, "branch", "-M", "release2"); err != nil {
		t.Fatalf("rename the default branch: %v\n%s", err, out)
	}

	wf := func(name string) string { return filepath.Join(dir, ".github", "workflows", name) }

	// (a) marker stripped → the file is the user's.
	const markerless = "check-doc-links.yml"
	body := readWorkflow(t, dir, markerless)
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "# logmind-template-version:") {
			kept = append(kept, line)
		}
	}
	stripped := strings.Join(kept, "\n")
	if stripped == body {
		t.Fatalf("setup: %s carried no marker to strip — the ownership rule would be untested", markerless)
	}
	if err := os.WriteFile(wf(markerless), []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}

	// (b) marker ahead of this binary → refuse to downgrade, and SAY so.
	const ahead = "regen-timeline.yml"
	future := strings.Replace(readWorkflow(t, dir, ahead), "# logmind-template-version: v", "# logmind-template-version: v99", 1)
	if !strings.Contains(future, "# logmind-template-version: v99") {
		t.Fatalf("setup: could not raise %s's marker", ahead)
	}
	if err := os.WriteFile(wf(ahead), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runInitIn(t, dir, "--no-git", "--refresh")

	if got := readWorkflow(t, dir, markerless); got != stripped {
		t.Errorf("--refresh rewrote %s, which carries no logmind marker — that file is the user's", markerless)
	}
	if got := readWorkflow(t, dir, ahead); got != future {
		t.Errorf("--refresh downgraded %s from a NEWER template marker", ahead)
	}
	if !strings.Contains(out, "refusing to downgrade") {
		t.Errorf("--refresh left a newer template alone but did not report the refusal:\n%s", out)
	}
	// Both were left alone, so neither may be claimed as refreshed.
	if strings.Contains(out, "Refreshed .github/workflows/") {
		t.Errorf("--refresh reported a rewrite it did not perform:\n%s", out)
	}
}

// TestShippedWorkflows_PrescribeCommandsThatExist is the CLASS guard behind
// the flag above, and the one that would have caught this before it shipped.
//
// A workflow template's remediation text is the only thing a repo owner has
// when a scaffolded check goes quiet, and it is prose: nothing linked
// `Fix: run 'logmind init --refresh'` to the CLI's actual flag set, so the
// message shipped — and two tests PINNED it — naming a flag that produced
// `Error: unknown flag: --refresh`. Adding the flag fixes today's message;
// this fixes the class, by resolving every `'logmind …'` command quoted in
// a shipped template against the real command tree.
func TestShippedWorkflows_PrescribeCommandsThatExist(t *testing.T) {
	var found []string
	for _, name := range templates.ListWorkflowTemplates() {
		for _, cmdline := range quotedLogmindCommands(templates.Workflow(name)) {
			found = append(found, cmdline)
			if err := resolvesAgainstCLI(cmdline); err != nil {
				t.Errorf("%s prescribes %q, which this binary cannot run: %v", name, cmdline, err)
			}
		}
	}

	// Control 1: the extractor actually found the commands. A regex that
	// matched nothing would make every assertion above vacuous.
	if len(found) == 0 {
		t.Fatalf("no `'logmind …'` command was extracted from any workflow template — " +
			"the probe is broken, not the tree")
	}
	if !containsString(found, "logmind init --refresh") {
		t.Errorf("the branch-drift remediation command was not among the extracted commands %q — "+
			"either it stopped being quoted or the extractor stopped seeing it", found)
	}

	// Control 2: the validator rejects what it should. Without this, a
	// resolvesAgainstCLI that returned nil unconditionally would pass.
	for _, bogus := range []string{
		"logmind init --no-such-flag",
		"logmind no-such-subcommand",
	} {
		if err := resolvesAgainstCLI(bogus); err == nil {
			t.Errorf("control: resolvesAgainstCLI(%q) returned nil — the validator does not validate", bogus)
		}
	}
}

// quotedLogmindCommands extracts every single-quoted `logmind …` command
// from a template body. Single quotes because that is how the shipped
// ::error / ::warning messages quote a command for the reader.
func quotedLogmindCommands(body string) []string {
	var out []string
	for _, chunk := range strings.Split(body, "'")[1:] {
		// Odd-indexed chunks after the split are the quoted spans; taking
		// every other one is fragile against apostrophes, so match on the
		// prefix instead and let non-commands fall through.
		if strings.HasPrefix(chunk, "logmind ") && !strings.Contains(chunk, "\n") {
			out = append(out, chunk)
		}
	}
	return out
}

// resolvesAgainstCLI reports whether `logmind <args>` names a real
// subcommand with real flags, WITHOUT running it: cobra's Find resolves the
// subcommand and ParseFlags validates every flag against that command's
// flag set. Positional arguments are left to the command's own Args
// validator at runtime.
func resolvesAgainstCLI(cmdline string) error {
	fields := strings.Fields(cmdline)
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	target, rest, err := root.Find(fields[1:])
	if err != nil {
		return err
	}
	if target == root {
		// `logmind <something>` that resolved to the root itself means the
		// subcommand does not exist.
		return &unknownCommandError{name: strings.Join(fields[1:], " ")}
	}
	return target.ParseFlags(rest)
}

type unknownCommandError struct{ name string }

func (e *unknownCommandError) Error() string { return "unknown subcommand: " + e.name }

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
