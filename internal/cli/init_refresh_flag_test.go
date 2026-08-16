package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/inserter"
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
		// The output must name the file AND disclose that its previous
		// contents are gone — see TestInitRefresh_DisclosesThatItDiscards
		// for why the second half is not decoration.
		if !strings.Contains(out, "Re-rendered .github/workflows/"+name) {
			t.Errorf("%s: --refresh rewrote the file but did not say so.\noutput:\n%s", name, out)
		}
	}

	// Idempotent: a second --refresh has nothing to do and must not claim
	// otherwise, or the line stops meaning anything.
	again := runInitIn(t, dir, "--no-git", "--refresh")
	for _, claim := range []string{"Refreshed .github/workflows/", "Re-rendered .github/workflows/"} {
		if strings.Contains(again, claim) {
			t.Errorf("a second --refresh reported a rewrite (%q) with nothing to rewrite:\n%s", claim, again)
		}
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

// tryInitIn is runInitIn's sibling for the paths that are SUPPOSED to fail:
// it returns the error instead of calling t.Fatalf on it, because the exit
// code is part of what is being asserted.
func tryInitIn(t *testing.T, dir string, args ...string) (string, error) {
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
	root.SilenceUsage = true
	root.SilenceErrors = true
	execErr := root.Execute()
	return sink.String(), execErr
}

// TestInitRefresh_DisclosesThatItDiscards covers the edge of the ownership
// principle rather than its centre.
//
// A workflow whose marker MATCHES the bundled one but whose bytes differ is
// logmind's to rewrite — the marker says so, and that is the whole point of
// `--refresh`, which exists to repair a stale scaffold-time render. But the
// same condition is produced by a user editing the file, and that user got
// nothing: the content vanished under a "↻ Refreshed … to current template"
// line that reads exactly like the no-op it was not, and the flag help
// ("Re-render the … files logmind owns") never said edits were discarded.
//
// Overwriting stays — refusing would break the repair the flag exists for,
// and a diff-and-prompt is not available to a non-interactive scaffold. The
// fix is disclosure, before the fact and at the moment it happens.
func TestInitRefresh_DisclosesThatItDiscards(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")
	runInitIn(t, dir, "--no-git")

	const name = "check-doc-links.yml"
	path := filepath.Join(dir, ".github", "workflows", name)
	before := readWorkflow(t, dir, name)
	const userEdit = "# MY IMPORTANT USER EDIT\n"
	edited := before + userEdit
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	// The marker is untouched, so this is the "version-current, content
	// differs" case and not a version bump.
	if inserter.ExtractTemplateMarker(edited).Version != inserter.ExtractTemplateMarker(before).Version {
		t.Fatalf("setup: the edit moved the marker — this test would be measuring a version bump")
	}

	out := runInitIn(t, dir, "--no-git", "--refresh")

	// The overwrite itself is the documented behaviour, not the defect.
	if got := readWorkflow(t, dir, name); strings.Contains(got, userEdit) {
		t.Fatalf("setup no longer holds: --refresh did not overwrite %s, so there is nothing to disclose", name)
	}
	// THE DEFECT: it happened silently.
	if !strings.Contains(out, "Re-rendered .github/workflows/"+name) {
		t.Errorf("--refresh discarded the user's edit to %s without naming the file:\n%s", name, out)
	}
	if !strings.Contains(out, "discarded") {
		t.Errorf("--refresh discarded the user's edit to %s and said nothing about it. The line it "+
			"printed reads like the no-op re-render it was not:\n%s", name, out)
	}

	// …and the same warning has to be available BEFORE the fact, or the
	// only place it appears is after the content is gone.
	root := NewRootCmd()
	initCmd, _, err := root.Find([]string{"init"})
	if err != nil {
		t.Fatalf("find init: %v", err)
	}
	help := initCmd.Flags().Lookup("refresh").Usage
	if !strings.Contains(strings.ToUpper(help), "DISCARD") {
		t.Errorf("`--refresh`'s flag help does not warn that it discards edits, so the only warning "+
			"a user gets arrives after the content is gone:\n%s", help)
	}
}

// TestInitRefresh_OneUnwritableWorkflowDoesNotStopTheRest is the regression
// for a loop that abandoned everything after its first failure while the
// command still exited 0 and printed a summary of what it had managed.
//
// The probe is a symlink, because that is the case with two defects in it:
// a bare `os.WriteFile` FOLLOWS one, writing logmind's bytes wherever it
// points — outside the repository if the link says so — and the abort that
// followed the refusal then cost every remaining workflow as well.
//
// Three things are asserted together, because any one of them alone leaves
// the user misled: the other files are still written, the link's target is
// untouched, and the command reports the shortfall AND exits non-zero. A
// false success line is what made the original defect dangerous.
func TestInitRefresh_OneUnwritableWorkflowDoesNotStopTheRest(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")
	runInitIn(t, dir, "--no-git")

	// Somewhere logmind must never write.
	outside := filepath.Join(t.TempDir(), "outside.txt")
	const sentinel = "NOT LOGMIND'S TO WRITE\n"
	if err := os.WriteFile(outside, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}

	// check-decisions sorts first among the bundled templates, so a plain
	// abort here costs every workflow after it — which is the point.
	names := templates.ListWorkflowTemplates()
	if len(names) < 2 {
		t.Fatalf("setup: only %d workflow templates bundled — an abort could not be distinguished "+
			"from a complete run", len(names))
	}
	victim := strings.TrimSuffix(names[0], ".template")
	victimPath := filepath.Join(dir, ".github", "workflows", victim)
	if err := os.Remove(victimPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, victimPath); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	// Force every OTHER workflow to need a rewrite, so "the rest were still
	// processed" is a claim with something behind it.
	if out, err := gitIn(dir, "branch", "-M", "release2"); err != nil {
		t.Fatalf("rename the default branch: %v\n%s", err, out)
	}

	out, execErr := tryInitIn(t, dir, "--no-git", "--refresh")

	// 1. The link's target is untouched — the write was refused, not
	//    followed.
	if got, err := os.ReadFile(outside); err != nil || string(got) != sentinel {
		t.Errorf("logmind wrote through a symlink and out of the repository: %s now holds %q (err %v)",
			outside, string(got), err)
	}
	// 2. Every other workflow was still processed.
	for _, n := range names[1:] {
		wf := strings.TrimSuffix(n, ".template")
		body := readWorkflow(t, dir, wf)
		if strings.Contains(body, "branches: [release]\n") {
			t.Errorf("%s still carries the stale trigger — the loop abandoned it after the first "+
				"refusal instead of continuing.\noutput:\n%s", wf, out)
		}
	}
	// 3. The shortfall is reported, and the command does not claim success.
	if !strings.Contains(out, victim) {
		t.Errorf("the workflow that could not be written (%s) is not named in the output:\n%s", victim, out)
	}
	if execErr == nil {
		t.Errorf("`init --refresh` exited 0 after failing to write %s. The summary above it lists "+
			"only what DID get written, so exit 0 makes that summary read as the whole story:\n%s",
			victim, out)
	}
}

// TestInitRefresh_EveryWriteFailureIsRecordedNotAborted is the sibling of
// the symlink test above, and it exists because that one did not kill the
// mutation it was supposed to.
//
// The symlink is refused BEFORE the read, so it never reaches the write
// call at all — reverting the force-render write site to `return …, err`
// left the symlink test green. What is under test here is the write itself:
// with the workflows directory made unwritable, EVERY file fails at the
// same site, so an abort names one file and a record-and-continue names
// them all. That difference is the assertion.
func TestInitRefresh_EveryWriteFailureIsRecordedNotAborted(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")
	runInitIn(t, dir, "--no-git")

	// Force every workflow to need a rewrite.
	if out, err := gitIn(dir, "branch", "-M", "release2"); err != nil {
		t.Fatalf("rename the default branch: %v\n%s", err, out)
	}

	// The workflows that now NEED a rewrite — the ones carrying the stale
	// trigger. A template whose render is unchanged is a no-op, not a
	// failure, and asserting over those would measure nothing.
	var needWrite []string
	for _, n := range templates.ListWorkflowTemplates() {
		wf := strings.TrimSuffix(n, ".template")
		if strings.Contains(readWorkflow(t, dir, wf), "branches: [release]") {
			needWrite = append(needWrite, wf)
		}
	}
	if len(needWrite) < 2 {
		t.Fatalf("setup: only %d workflow(s) need a rewrite (%v) — an abort at the first failure "+
			"would be indistinguishable from a complete run", len(needWrite), needWrite)
	}

	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.Chmod(wfDir, 0o555); err != nil {
		t.Skipf("cannot make the workflows directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(wfDir, 0o755) })
	// Control: root ignores the mode bits, and CI sometimes runs as root.
	// Without this the assertions below would pass against a tree where
	// nothing failed at all.
	if probe, err := os.CreateTemp(wfDir, "probe-*"); err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("the read-only directory is still writable here (running as root?) — the probe cannot fail")
	}

	out, execErr := tryInitIn(t, dir, "--no-git", "--refresh")

	// Assert against needWrite, not the full template set: a workflow whose
	// render is unchanged (check-decisions.yml, logmind-self-update.yml —
	// neither carries the branch literal) is correctly skipped as a no-op
	// BEFORE any write is attempted, so it is never named in the output
	// either way. Asserting over the full set here would fail regardless of
	// whether the record-and-continue behaviour under test still holds —
	// exactly the false negative this test exists to avoid.
	var missing []string
	for _, wf := range needWrite {
		if !strings.Contains(out, wf) {
			missing = append(missing, wf)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d of %d unwritable workflows were never named: %v. The loop stopped at the first "+
			"failure instead of recording it and continuing, so the summary describes a fraction of "+
			"what was attempted:\n%s", len(missing), len(needWrite), missing, out)
	}
	if execErr == nil {
		t.Errorf("`init --refresh` exited 0 having written no workflow at all:\n%s", out)
	}
	if strings.Contains(out, "All workflow templates already current.") {
		t.Errorf("`init --refresh` claimed every template current after failing to write all of them:\n%s", out)
	}
}

// TestInitRefresh_SymlinkedWorkflowIsSkippedWhileOthersForceRender closes a
// gap that OneUnwritableWorkflowDoesNotStopTheRest and
// EveryWriteFailureIsRecordedNotAborted leave between them.
//
// OneUnwritableWorkflowDoesNotStopTheRest plants its symlink and then
// forces drift via a BRANCH RENAME — but only two of the four bundled
// templates carry the `branches: [__LOGMIND_DEFAULT_BRANCH__]` literal
// (check-doc-links.yml, regen-timeline.yml); check-decisions.yml and
// logmind-self-update.yml render byte-identical either way, so that setup
// can never show "the other three ARE re-rendered" — one or two of them
// are always a correct no-op instead. Neither existing test checks that
// the symlink OBJECT itself survives untouched — only that whatever it
// points at does; RefuseSymlink's contract ("leave it exactly as found")
// covers the link too, and a caller that deleted-then-recreated it while
// still refusing to follow it would pass both existing tests.
//
// This test drives drift directly (a content edit to each of the other
// three, marker left alone) so all three definitely reach the
// force-render write regardless of which trigger shape they carry, and
// adds the missing assertion on the link itself.
func TestInitRefresh_SymlinkedWorkflowIsSkippedWhileOthersForceRender(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")
	runInitIn(t, dir, "--no-git")

	names := templates.ListWorkflowTemplates()
	if len(names) != 4 {
		t.Fatalf("setup: expected 4 bundled workflow templates, found %d: %v — this test's \"other three\" "+
			"framing no longer matches the bundle", len(names), names)
	}
	var all []string
	for _, n := range names {
		all = append(all, strings.TrimSuffix(n, ".template"))
	}
	victim := all[0]
	others := all[1:]

	// Drift the OTHER three directly: version-current, content differs —
	// exactly the condition workflowForceRender exists to repair. Untied
	// from the branch literal, so it exercises every template regardless
	// of which trigger shape it carries (setup below confirms the marker
	// itself did not move).
	const userEdit = "# EXISTING CONTENT logmind must re-render over\n"
	for _, wf := range others {
		before := readWorkflow(t, dir, wf)
		edited := before + userEdit
		if inserter.ExtractTemplateMarker(edited).Version != inserter.ExtractTemplateMarker(before).Version {
			t.Fatalf("setup: editing %s moved its marker — this test would measure a version bump, not force-render", wf)
		}
		if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", wf), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The victim: a DANGLING symlink (the target does not exist yet)
	// pointing outside the repository, in place of a file whose marker
	// (before removal) matched the bundle — --refresh is the only mode
	// that would even ATTEMPT a rewrite of a version-current file, so this
	// setup isolates it from the version-ordered refresh path. Dangling
	// specifically because that is the shape atomicio.go's own doc comment
	// names as the sharper case: "a bare os.WriteFile opens through the
	// link ... and lands its body wherever the link points, dangling
	// target or not" — an ENOENT read on a dangling link is what would
	// route a caller without the refusal into the CREATE branch, not the
	// overwrite one.
	victimPath := filepath.Join(dir, ".github", "workflows", victim)
	if err := os.Remove(victimPath); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if _, err := os.Lstat(outside); err == nil {
		t.Fatalf("setup: %s already exists — the dangling-symlink premise doesn't hold", outside)
	}
	if err := os.Symlink(outside, victimPath); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	wantTarget, err := os.Readlink(victimPath)
	if err != nil {
		t.Fatalf("setup: could not read back the planted symlink: %v", err)
	}

	out, execErr := tryInitIn(t, dir, "--no-git", "--refresh")

	// 1. Nothing was created outside the repository through the link — the
	// dangling target must still be absent.
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Errorf("logmind wrote through the symlink and created %s outside the repository (err %v)",
			outside, err)
	}

	// 2. The link ITSELF is untouched: still a symlink, still pointing at
	// the same target. Replacing it with a regular file (even an empty or
	// failed-write one) or repointing it would violate "leave it exactly
	// as found" just as much as following it would.
	fi, err := os.Lstat(victimPath)
	if err != nil {
		t.Fatalf("%s: Lstat after refresh: %v", victim, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s was replaced by a regular file — RefuseSymlink must leave a refused symlink exactly as found", victim)
	} else if got, err := os.Readlink(victimPath); err != nil || got != wantTarget {
		t.Errorf("%s's symlink target changed: got %q (err %v), want %q", victim, got, err, wantTarget)
	}

	// 3. The other three WERE re-rendered — the drift they carried is gone,
	// and the run said so.
	for _, wf := range others {
		body := readWorkflow(t, dir, wf)
		if strings.Contains(body, userEdit) {
			t.Errorf("%s: --refresh left the drifted content in place — the victim's refusal must not cost "+
				"the force-render write for the other three.\noutput:\n%s", wf, out)
		}
		if !strings.Contains(out, "Re-rendered .github/workflows/"+wf) {
			t.Errorf("%s: --refresh rewrote the file but did not report it as re-rendered.\noutput:\n%s", wf, out)
		}
	}

	// 4. The shortfall is reported honestly: the victim is named, and the
	// command does not exit 0 as though nothing had gone wrong.
	if !strings.Contains(out, victim) {
		t.Errorf("the skipped workflow (%s) is not named in the output:\n%s", victim, out)
	}
	if execErr == nil {
		t.Errorf("`init --refresh` exited 0 after skipping %s — the summary above lists only what DID get "+
			"written, so exit 0 makes that summary read as the whole story:\n%s", victim, out)
	}
	if strings.Contains(out, "All workflow templates already current.") {
		t.Errorf("--refresh claimed every template current while %s was skipped as a symlink:\n%s", victim, out)
	}
}

// TestInitRefresh_DoesNotClaimAllCurrentOverAFileItSkipped pins the second
// half of the same honesty rule. A markerless workflow is the user's and is
// correctly left alone — but "All workflow templates already current" is a
// claim about the whole set, and printing it over a file logmind just
// declined to look at is false.
func TestInitRefresh_DoesNotClaimAllCurrentOverAFileItSkipped(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")
	runInitIn(t, dir, "--no-git")

	// CONTROL: with nothing skipped, the line is true and must still print.
	// Without this leg, a change that simply deleted the line would pass.
	if base := runInitIn(t, dir, "--no-git", "--refresh"); !strings.Contains(base, "All workflow templates already current.") {
		t.Fatalf("control: with every workflow owned and current, the summary line must still appear:\n%s", base)
	}

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
		t.Fatalf("setup: %s carried no marker to strip", markerless)
	}
	if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", markerless), []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runInitIn(t, dir, "--no-git", "--refresh")
	if strings.Contains(out, "All workflow templates already current.") {
		t.Errorf("--refresh reported every template current while %s was skipped for having no "+
			"logmind marker — the one file it did not bring to the bundled template is the one the "+
			"line claims about:\n%s", markerless, out)
	}
	if !strings.Contains(out, markerless) {
		t.Errorf("--refresh skipped %s and never named it:\n%s", markerless, out)
	}
}

// TestInitRefresh_CreateWriteFailureDoesNotAbortTheRest pins the CREATE-arm
// write failure at installWorkflowTemplatesMode's os.ReadFile ErrNotExist
// branch (#306's create-write site, line ~694): a file that does not exist
// yet, whose write to bring it into existence fails.
//
// Reached only via injection — there is no repository state that makes a
// brand-new file's write fail on its own; a read-only target directory is
// the same class of failure the two already-pinned write sites (RefuseSymlink,
// force-write) use, and it is unavoidable for this site specifically because
// the site exists only to handle "the write itself failed", which is
// necessarily a filesystem condition rather than a content one.
//
// The directory is pre-created and chmod'd BEFORE `init` runs, so
// installWorkflowTemplatesMode's own os.MkdirAll — which only needs to see
// the directory already exists, not write into it — still succeeds, and
// every template (the directory is empty) hits the CREATE branch rather than
// a version-compare branch below it. A `return` at the write-failure site
// would abandon every template after the first alphabetically before it was
// ever attempted at all — the fresh-install analogue of
// TestInitRefresh_EveryWriteFailureIsRecordedNotAborted.
func TestInitRefresh_CreateWriteFailureDoesNotAbortTheRest(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")

	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(wfDir, 0o555); err != nil {
		t.Skipf("cannot make the workflows directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(wfDir, 0o755) })
	// Control: root ignores the mode bits, and CI sometimes runs as root.
	if probe, err := os.CreateTemp(wfDir, "probe-*"); err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("the read-only directory is still writable here (running as root?) — the probe cannot fail")
	}

	names := templates.ListWorkflowTemplates()
	if len(names) < 2 {
		t.Fatalf("setup: only %d workflow template(s) bundled — an abort at the first failure would be "+
			"indistinguishable from a complete run", len(names))
	}

	out := runInitIn(t, dir, "--no-git")

	var missing []string
	for _, n := range names {
		wf := strings.TrimSuffix(n, ".template")
		if !strings.Contains(out, wf) {
			missing = append(missing, wf)
		}
		if _, err := os.Stat(filepath.Join(wfDir, wf)); err == nil {
			t.Errorf("%s exists despite its create-write having been made to fail", wf)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d of %d workflows that failed to be CREATED were never named: %v. The loop stopped "+
			"at the first create failure instead of recording it and continuing:\n%s",
			len(missing), len(names), missing, out)
	}
	if !strings.Contains(out, "could NOT be written") {
		t.Errorf("`init` did not disclose that every workflow template failed to be created:\n%s", out)
	}
	if strings.Contains(out, "logmind initialized successfully!") {
		t.Errorf("`init` claimed success while every workflow template failed to be created:\n%s", out)
	}
}

// TestInitRefresh_ReadErrorDoesNotAbortTheRest pins the read-error branch at
// installWorkflowTemplatesMode's ownership read (#306's read-error site,
// line ~705): os.ReadFile fails with something other than ErrNotExist.
//
// A DIRECTORY in place of the target reaches this branch without touching
// filesystem permissions at all: Lstat sees a non-symlink (RefuseSymlink
// passes it through unchanged) and the subsequent os.ReadFile then fails
// with "is a directory", which is not fs.ErrNotExist — landing on this arm
// rather than the CREATE one. Portable (no root-skip needed) and reachable
// without contrived permission bits, unlike the create-write and
// refresh-write sites this PR also pins.
func TestInitRefresh_ReadErrorDoesNotAbortTheRest(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")
	runInitIn(t, dir, "--no-git")

	names := templates.ListWorkflowTemplates()
	if strings.TrimSuffix(names[0], ".template") != "check-decisions.yml" {
		t.Fatalf("setup: expected check-decisions.yml to sort first, got %q — this test's ordering "+
			"premise no longer holds", names[0])
	}

	// Force every OTHER workflow to need a rewrite, so "the rest were still
	// processed" is a claim with something behind it.
	if out, err := gitIn(dir, "branch", "-M", "release2"); err != nil {
		t.Fatalf("rename the default branch: %v\n%s", err, out)
	}

	// The victim: a directory where a regular file belongs.
	const victim = "check-decisions.yml"
	victimPath := filepath.Join(dir, ".github", "workflows", victim)
	if err := os.Remove(victimPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(victimPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// --refresh: a version-ordered refresh does not repair a stale
	// scaffold-time trigger at all (see
	// TestInitRefresh_RepairsAStaleScaffoldedTrigger) — only the
	// force-render path does, so this is what actually gives the other
	// three templates something to do.
	out, execErr := tryInitIn(t, dir, "--no-git", "--refresh")

	// 1. The failed read is named in the output.
	if !strings.Contains(out, victim) {
		t.Errorf("the workflow that could not be READ (%s) is not named in the output:\n%s", victim, out)
	}
	// 2. Every other workflow — all of which sort AFTER the victim — was
	// still processed, not abandoned.
	for _, n := range names[1:] {
		wf := strings.TrimSuffix(n, ".template")
		body := readWorkflow(t, dir, wf)
		if strings.Contains(body, "branches: [release]\n") {
			t.Errorf("%s still carries the stale trigger — the read error on %s (which sorts first) "+
				"abandoned every template after it instead of recording the failure and continuing:\n%s",
				wf, victim, out)
		}
	}
	// 3. The directory planted as the victim was left exactly as found.
	if fi, err := os.Stat(victimPath); err != nil || !fi.IsDir() {
		t.Errorf("%s: the directory planted in place of the workflow was disturbed by the failed read (err %v)",
			victim, err)
	}
	// 4. The shortfall is reported honestly: exit non-zero.
	if execErr == nil {
		t.Errorf("`init` exited 0 after failing to read %s:\n%s", victim, out)
	}
}

// TestInitRefresh_DowngradeRefusalDoesNotAbortTheRest pins the downgrade
// decline at installWorkflowTemplatesMode's version-order check (#306's
// declineDowngrade site, line ~771) — the site the panel found matters most,
// because it is not a failure-injection corner but the exact scenario the
// code's own comment documents: a released binary bundling an OLDER
// template than a repo it is scaffolding already carries (SPEC-documented,
// #286).
//
// TestInitRefresh_DoesNotWidenOwnership already covers the DECISION (refuse,
// don't downgrade) but not the LOOP: its "ahead" marker sits on
// regen-timeline.yml, which sorts LAST among the four bundled templates, so
// mutating that site's `continue` to `return` loses nothing in that test —
// nothing comes after regen-timeline.yml. This test puts the ahead marker on
// check-decisions.yml, which sorts FIRST, and forces every other template to
// need re-creating, so a `return` at the decline site would abandon all
// three of them — reproducing, end to end, the scenario the panel manually
// verified against the built binary.
func TestInitRefresh_DowngradeRefusalDoesNotAbortTheRest(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")
	runInitIn(t, dir, "--no-git")

	names := templates.ListWorkflowTemplates()
	if strings.TrimSuffix(names[0], ".template") != "check-decisions.yml" {
		t.Fatalf("setup: expected check-decisions.yml to sort first, got %q — this test's ordering "+
			"premise no longer holds", names[0])
	}

	// The AHEAD marker — the scenario installWorkflowTemplatesMode's
	// installedVer != bundledVer comment documents by name.
	const ahead = "check-decisions.yml"
	future := strings.Replace(readWorkflow(t, dir, ahead), "# logmind-template-version: v",
		"# logmind-template-version: v99", 1)
	if !strings.Contains(future, "# logmind-template-version: v99") {
		t.Fatalf("setup: could not raise %s's marker", ahead)
	}
	if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", ahead), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}

	// Delete the other three, so this run's only way to bring them back is
	// the CREATE branch reached AFTER the downgrade decline for `ahead` —
	// exactly what the panel's manual repro forced.
	var others []string
	for _, n := range names[1:] {
		wf := strings.TrimSuffix(n, ".template")
		others = append(others, wf)
		if err := os.Remove(filepath.Join(dir, ".github", "workflows", wf)); err != nil {
			t.Fatal(err)
		}
	}

	out := runInitIn(t, dir, "--no-git")

	if got := readWorkflow(t, dir, ahead); got != future {
		t.Errorf("`init` downgraded %s from a NEWER template marker", ahead)
	}
	if !strings.Contains(out, "refusing to downgrade") {
		t.Errorf("the newer marker on %s was not reported as refused:\n%s", ahead, out)
	}
	for _, wf := range others {
		path := filepath.Join(dir, ".github", "workflows", wf)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was never recreated — the downgrade refusal on %s (which sorts first) abandoned "+
				"every template after it instead of recording the refusal and continuing:\n%s", wf, ahead, out)
			continue
		}
		if !strings.Contains(out, "✓ Created .github/workflows/"+wf) {
			t.Errorf("%s was recreated but not reported as created:\n%s", wf, out)
		}
	}
}

// TestInitRefresh_VersionOrderedWriteFailureDoesNotAbortTheRest pins the
// version-ordered refresh write at installWorkflowTemplatesMode's
// installedVer != bundledVer branch (#306's refresh-write site, line ~788)
// — the ordinary "brew install bundles an older template than the repo
// already has" shape, distinct from declineDowngrade's mirror image and
// distinct from force-write (line ~814, already pinned), which only fires
// under workflowForceRender.
//
// Two markers are rolled BACKWARDS — at both ends of the alphabetical list
// — so a `return` at the write-failure site would abandon the second
// (regen-timeline.yml) while only ever reporting the first
// (check-decisions.yml).
func TestInitRefresh_VersionOrderedWriteFailureDoesNotAbortTheRest(t *testing.T) {
	dir := t.TempDir()
	seedRepo(t, dir, "release")
	runInitIn(t, dir, "--no-git")

	rollBack := func(name string) string {
		body := readWorkflow(t, dir, name)
		marker := inserter.ExtractTemplateMarker(body)
		if marker.Version == "" {
			t.Fatalf("setup: %s carries no marker to roll back", name)
		}
		older := strings.Replace(body, "# logmind-template-version: "+marker.Version,
			"# logmind-template-version: v1", 1)
		if !strings.Contains(older, "# logmind-template-version: v1") {
			t.Fatalf("setup: could not roll back %s's marker", name)
		}
		if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", name), []byte(older), 0o644); err != nil {
			t.Fatal(err)
		}
		return older
	}
	first := rollBack("check-decisions.yml") // sorts first
	last := rollBack("regen-timeline.yml")   // sorts last

	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.Chmod(wfDir, 0o555); err != nil {
		t.Skipf("cannot make the workflows directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(wfDir, 0o755) })
	// Control: root ignores the mode bits, and CI sometimes runs as root.
	if probe, err := os.CreateTemp(wfDir, "probe-*"); err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("the read-only directory is still writable here (running as root?) — the probe cannot fail")
	}

	out, execErr := tryInitIn(t, dir, "--no-git")

	for _, name := range []string{"check-decisions.yml", "regen-timeline.yml"} {
		if !strings.Contains(out, name) {
			t.Errorf("%s's version-ordered refresh write failed but was never named in the output — "+
				"the loop stopped at an earlier failure instead of recording it and continuing:\n%s", name, out)
		}
	}
	if got := readWorkflow(t, dir, "check-decisions.yml"); got != first {
		t.Errorf("check-decisions.yml changed despite its write failing")
	}
	if got := readWorkflow(t, dir, "regen-timeline.yml"); got != last {
		t.Errorf("regen-timeline.yml changed despite its write failing")
	}
	if execErr == nil {
		t.Errorf("`init` exited 0 after failing to refresh both rolled-back workflows:\n%s", out)
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
