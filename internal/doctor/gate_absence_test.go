// gate_absence_test.go — exercises collectGateAbsences / the
// CollectStatus.GateAbsences field: a §3.4/§6.2 enforcement surface that
// `logmind init` installed and something later removed.
//
// Unlike every other list on StatusReport, these are NOT advisory. A gate
// that is gone is an enforcement point failing open, permanently, and
// SPEC §3.4 says "Failing open MUST NOT be silent" — so Overall must flip
// to DRIFT and `logmind doctor` must exit non-zero.
//
// Measured on the release candidate, and the reason this file exists:
// `logmind init`, then `rm .git/hooks/commit-msg .claude/settings.json
// .github/workflows/check-decisions.yml`, then `logmind doctor` printed
// "Stack status: OK" and exited 0. The same repo with one stale template
// marker correctly printed DRIFT and exited 1 — doctor was not blind, it
// simply never counted gone.
package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/claudehook"
	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/templates"
	"github.com/thrillmade/logmind/internal/testgit"
)

// gatedRepo is freshRepo plus the state `logmind init` leaves behind: a
// .git directory and all three enforcement surfaces installed. Tests
// delete from it, which is the shape the bug lives in — freshRepo has no
// .git at all, and a directory that cannot make a commit has no gate
// failing open (see collectGateAbsences).
func gatedRepo(t *testing.T) string {
	t.Helper()
	dir := freshRepo(t)
	// A REAL repository, because the hook installers and the hook probes
	// both resolve their directory through git now (hooks.Dir) — against a
	// hand-made `.git` every Install* below no-ops and every probe reports
	// missing, which is the fixture measuring the bug rather than the code.
	realGitDir(t, dir)
	if _, err := hooks.InstallCommitMsg(dir); err != nil {
		t.Fatalf("InstallCommitMsg: %v", err)
	}
	if _, err := claudehook.EnsurePreToolUseGuard(dir); err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	// All four workflows, as init writes them in one pass — the other three
	// are what tell collectGateAbsences that this repository is on GitHub
	// Actions at all (gateSurfaces.siblings).
	for _, name := range LogmindWorkflows {
		mustWrite(t, filepath.Join(dir, ".github", "workflows", name),
			templates.Workflow(name+".template"))
	}
	return dir
}

// gatePaths are the files gatedRepo installs, keyed by the sentence
// fragment collectGateAbsences reports them under.
var gatePaths = []string{
	filepath.Join(".claude", "settings.json"),
	filepath.Join(".git", "hooks", "commit-msg"),
	filepath.Join(".github", "workflows", "check-decisions.yml"),
}

// TestGateAbsences_FullyInstalled_IsSilent is the control every other case
// in this file is measured against. Without it, a collector that reported
// all three unconditionally would score identically on the case below.
func TestGateAbsences_FullyInstalled_IsSilent(t *testing.T) {
	dir := gatedRepo(t)
	r := CollectStatus(dir, true)
	if len(r.GateAbsences) != 0 {
		t.Errorf("GateAbsences = %v; want none — all three surfaces are installed", r.GateAbsences)
	}
	if r.Overall != "OK" {
		t.Errorf("Overall = %q; want OK", r.Overall)
	}
}

// TestGateAbsences_DeletedGatesAreDrift is the regression pin, and it pins
// the OUTPUT the operator saw: the rendered "Stack status:" line and the
// verdict driving the process exit, not the collector underneath them.
func TestGateAbsences_DeletedGatesAreDrift(t *testing.T) {
	dir := gatedRepo(t)
	for _, rel := range gatePaths {
		if err := os.Remove(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("remove %s: %v", rel, err)
		}
	}

	r := CollectStatus(dir, true)
	if len(r.GateAbsences) != 3 {
		t.Fatalf("GateAbsences = %v; want all three surfaces reported", r.GateAbsences)
	}
	if r.Overall != "DRIFT" {
		t.Fatalf("Overall = %q; want DRIFT — three enforcement gates are gone and "+
			"SPEC §3.4 forbids failing open silently", r.Overall)
	}

	// Each absence must name the FILE, or the reader cannot act on it.
	joined := strings.Join(r.GateAbsences, "\n")
	for _, rel := range gatePaths {
		if !strings.Contains(joined, filepath.ToSlash(rel)) {
			t.Errorf("GateAbsences = %v; want %s named", r.GateAbsences, rel)
		}
	}

	// And it has to reach the surface a person reads. The rendered table
	// is what `logmind doctor` prints; a report field nothing renders is
	// exactly as silent as the bug was.
	body := RenderStatus(r)
	if !strings.Contains(body, "Stack status: DRIFT") {
		t.Errorf("rendered body says OK over three deleted gates:\n%s", body)
	}
	for _, want := range []string{
		"Enforcement gates absent (3)",
		"Failing open MUST NOT be silent",
		"logmind doctor --fix",
		"git.enforce_commits: false",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body missing %q:\n%s", want, body)
		}
	}
}

// TestGateAbsences_EachSurfaceCountsOnItsOwn — one deletion, one row. A
// collector that only fired when ALL of them were gone would pass the test
// above and still miss the single-gate case, which is the common one (a
// fresh clone has no commit-msg hook).
func TestGateAbsences_EachSurfaceCountsOnItsOwn(t *testing.T) {
	for _, rel := range gatePaths {
		t.Run(rel, func(t *testing.T) {
			dir := gatedRepo(t)
			if err := os.Remove(filepath.Join(dir, rel)); err != nil {
				t.Fatalf("remove %s: %v", rel, err)
			}
			r := CollectStatus(dir, true)
			if len(r.GateAbsences) != 1 {
				t.Fatalf("GateAbsences = %v; want exactly 1 (%s deleted)", r.GateAbsences, rel)
			}
			if !strings.Contains(r.GateAbsences[0], filepath.ToSlash(rel)) {
				t.Errorf("GateAbsences[0] = %q; want it to name %s", r.GateAbsences[0], rel)
			}
			if r.Overall != "DRIFT" {
				t.Errorf("Overall = %q; want DRIFT (%s is gone)", r.Overall, rel)
			}
		})
	}
}

// TestGateAbsences_EnforceCommitsFalseIsRespected — the deliberate
// opt-out. `git.enforce_commits: false` already means "logmind does not
// gate commits here" to guard-commit; doctor must not nag a repo that has
// said so, or the escape hatch does not exist.
func TestGateAbsences_EnforceCommitsFalseIsRespected(t *testing.T) {
	dir := gatedRepo(t)
	for _, rel := range gatePaths {
		if err := os.Remove(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("remove %s: %v", rel, err)
		}
	}
	// CONTROL first: without the key, this exact tree is DRIFT.
	if r := CollectStatus(dir, true); r.Overall != "DRIFT" {
		t.Fatalf("control: Overall = %q; want DRIFT before the opt-out is written", r.Overall)
	}

	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"),
		"git:\n  auto_commit: true\n  enforce_commits: false\n")

	r := CollectStatus(dir, true)
	if len(r.GateAbsences) != 0 {
		t.Errorf("GateAbsences = %v; want none — git.enforce_commits is false", r.GateAbsences)
	}
	if r.Overall != "OK" {
		t.Errorf("Overall = %q; want OK — the repo opted out deliberately", r.Overall)
	}
}

// TestGateAbsences_ClaudeAgentDisabled — the reader must agree with the
// writer. `logmind init` never installs the PreToolUse guard for a repo
// with `agents.claude: false`, so doctor must not report it absent; the
// OTHER two surfaces still count, or the flag would silence more than it
// covers.
func TestGateAbsences_ClaudeAgentDisabled(t *testing.T) {
	dir := gatedRepo(t)
	mustWrite(t, filepath.Join(dir, ".logmind", "config.yml"),
		"git:\n  auto_commit: true\nagents:\n  claude: false\n")
	if err := os.Remove(filepath.Join(dir, ".claude", "settings.json")); err != nil {
		t.Fatalf("remove settings.json: %v", err)
	}

	r := CollectStatus(dir, true)
	if len(r.GateAbsences) != 0 {
		t.Fatalf("GateAbsences = %v; want none — this repo never asked for the guard", r.GateAbsences)
	}

	// The flag covers the guard and nothing else.
	if err := os.Remove(filepath.Join(hooksDirOf(t, dir), "commit-msg")); err != nil {
		t.Fatalf("remove commit-msg: %v", err)
	}
	r = CollectStatus(dir, true)
	if len(r.GateAbsences) != 1 || !strings.Contains(r.GateAbsences[0], "commit-msg") {
		t.Fatalf("GateAbsences = %v; want the commit-msg hook alone — agents.claude "+
			"silences the harness guard, not the whole set", r.GateAbsences)
	}
}

// TestGateAbsences_NeverInitialised_IsSilent — absence is only drift where
// something was installed to lose. A directory that never ran `logmind
// init` has no .logmind/config.yml and must never be nagged, which is the
// line between "a choice" and "drift".
func TestGateAbsences_NeverInitialised_IsSilent(t *testing.T) {
	dir := t.TempDir()
	isolatePathHermetic(t)
	realGitDir(t, dir)
	r := CollectStatus(dir, true)
	if len(r.GateAbsences) != 0 {
		t.Errorf("GateAbsences = %v; want none — this directory never ran `logmind init`", r.GateAbsences)
	}
	if r.Overall != "OK" {
		t.Errorf("Overall = %q; want OK", r.Overall)
	}
}

// TestGateAbsences_NonRepo_IsSilent — an initialised directory that is not
// a git repository cannot make a commit, so nothing is failing open there.
// probeHook reports "missing" for a deleted hook and for an absent .git
// identically, so this distinction is made in collectGateAbsences and has
// to be pinned separately.
func TestGateAbsences_NonRepo_IsSilent(t *testing.T) {
	dir := freshRepo(t) // .logmind/config.yml, but no .git
	r := CollectStatus(dir, true)
	if len(r.GateAbsences) != 0 {
		t.Errorf("GateAbsences = %v; want none — not a git repository", r.GateAbsences)
	}
	if r.Overall != "OK" {
		t.Errorf("Overall = %q; want OK", r.Overall)
	}
}

// TestGateAbsences_NonEnforcementMissingIsStillBenign — the scope fence.
// Turning every optional template into DRIFT is how doctor becomes noise
// people stop reading, so a missing NON-enforcement artifact must stay
// exactly as benign as it was.
func TestGateAbsences_NonEnforcementMissingIsStillBenign(t *testing.T) {
	dir := gatedRepo(t)
	for _, rel := range []string{
		filepath.Join(".github", "workflows", "regen-timeline.yml"),
		filepath.Join(".github", "workflows", "check-doc-links.yml"),
		filepath.Join(".github", "workflows", "logmind-self-update.yml"),
		"AGENTS.md",
		".gitattributes",
	} {
		mustWrite(t, filepath.Join(dir, rel), "placeholder\n")
		if err := os.Remove(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("remove %s: %v", rel, err)
		}
	}
	// The post-merge / post-rewrite / pre-commit hooks were never installed
	// by gatedRepo, so they are already absent here.
	r := CollectStatus(dir, true)
	if len(r.GateAbsences) != 0 {
		t.Errorf("GateAbsences = %v; want none — none of those is a §3.4/§6.2 "+
			"enforcement surface", r.GateAbsences)
	}
	if r.Overall != "OK" {
		t.Errorf("Overall = %q; want OK — missing optional templates are not drift", r.Overall)
	}
}

// TestGateAbsences_LinkedWorktreeSeesTheHooksGitReads — the false positive
// that shipped with this feature's first draft, now fixed at the cause
// rather than suppressed.
//
// In a linked git worktree `.git` is a FILE naming the common directory,
// the hooks live THERE, and they fire normally. The probe used to join
// <repoRoot>/.git/hooks/<name>, so it reported "missing" for a hook that
// was installed and running — measured in this repository's own agent
// worktrees. The first fix was to say nothing in a worktree at all, which
// bought silence at the price of never reporting a genuinely absent hook
// there. The probe now resolves the directory the way git does
// (hooks.Dir), so both halves are answerable, and both are asserted here:
// an installed hook is FOUND from the worktree, and a deleted one is
// REPORTED from the worktree.
func TestGateAbsences_LinkedWorktreeSeesTheHooksGitReads(t *testing.T) {
	main := gatedRepo(t)
	mustWrite(t, filepath.Join(main, "seed.txt"), "seed\n")
	gitInRepo(t, main, "config", "user.email", "test@example.com")
	gitInRepo(t, main, "config", "user.name", "Test")
	gitInRepo(t, main, "config", "commit.gpgsign", "false")
	gitInRepo(t, main, "add", "seed.txt")
	gitInRepo(t, main, "commit", "-q", "-m", "seed", "--no-verify")

	linked := filepath.Join(t.TempDir(), "wt")
	gitInRepo(t, main, "worktree", "add", "-q", "-b", "wt", linked)
	testgit.DisableMaintenance(t, linked)

	// The worktree is a checkout, not a logmind project — give it the same
	// surfaces gatedRepo gave the main checkout, minus the hook, which is
	// SHARED and is exactly what this test is about.
	mustWrite(t, filepath.Join(linked, ".logmind", "config.yml"), "git:\n  auto_commit: true\n")
	mustWrite(t, filepath.Join(linked, "docs", "decisions.md"), "# Decisions\n")
	if _, err := claudehook.EnsurePreToolUseGuard(linked); err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	for _, name := range LogmindWorkflows {
		mustWrite(t, filepath.Join(linked, ".github", "workflows", name),
			templates.Workflow(name+".template"))
	}

	// Precondition: `.git` really is a FILE here, or this test is a second
	// copy of the normal-checkout case.
	info, err := os.Lstat(filepath.Join(linked, ".git"))
	if err != nil || info.IsDir() {
		t.Fatalf("precondition: %s/.git is not a worktree pointer file (err=%v)", linked, err)
	}

	if r := CollectStatus(linked, true); len(r.GateAbsences) != 0 {
		t.Fatalf("GateAbsences = %v; want none — the commit-msg hook is installed in the "+
			"common directory and fires for commits made from this worktree", r.GateAbsences)
	}

	// …and the absence IS reported from here, which the previous
	// worktree-wide silence could never have said.
	hooksDir, ok := hooks.Dir(linked)
	if !ok {
		t.Fatal("hooks.Dir could not resolve the worktree's hooks directory")
	}
	if err := os.Remove(filepath.Join(hooksDir, "commit-msg")); err != nil {
		t.Fatalf("remove commit-msg: %v", err)
	}
	r := CollectStatus(linked, true)
	if len(r.GateAbsences) != 1 || !strings.Contains(r.GateAbsences[0], "commit-msg") {
		t.Fatalf("GateAbsences = %v; want the commit-msg hook reported — it is genuinely "+
			"gone from the directory git reads for this worktree", r.GateAbsences)
	}
}

// TestGateAbsences_UnresolvableGitDirIsSilent — the other side of the same
// rule. When git cannot say where the hooks live (a `.git` pointer at a
// common directory that is not there), logmind cannot establish that a hook
// is absent, and "cannot establish" must not be reported as "absent":
// `doctor --fix` would not clear it either, because the installer resolves
// the same way and no-ops for the same reason.
//
// The FILE-backed surfaces still report, or this guard would silence more
// than the unanswerable probe.
func TestGateAbsences_UnresolvableGitDirIsSilent(t *testing.T) {
	dir := gatedRepo(t)
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("rm .git: %v", err)
	}
	mustWrite(t, filepath.Join(dir, ".git"), "gitdir: /elsewhere/.git/worktrees/wt\n")

	r := CollectStatus(dir, true)
	for _, g := range r.GateAbsences {
		if strings.Contains(g, "commit-msg") {
			t.Errorf("GateAbsences = %v; git cannot resolve a hooks directory here, so "+
				"the hook's absence is not established and must not be claimed", r.GateAbsences)
		}
	}

	if err := os.Remove(filepath.Join(dir, ".github", "workflows", "check-decisions.yml")); err != nil {
		t.Fatalf("remove check-decisions.yml: %v", err)
	}
	r = CollectStatus(dir, true)
	if len(r.GateAbsences) != 1 || !strings.Contains(r.GateAbsences[0], "check-decisions.yml") {
		t.Fatalf("GateAbsences = %v; want the merge gate reported — an unanswerable git "+
			"hides the hooks, not the workflow", r.GateAbsences)
	}
}

// gitInRepo runs `git <args>` in dir, failing the test on a non-zero exit.
func gitInRepo(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// TestGateAbsences_NoWorkflowsAtAll_IsSilent — `logmind init
// --github-actions=false` installs no workflows and records that choice
// nowhere, so the merge-gate row must fall silent when its siblings are
// absent too. Otherwise a repository that is simply not on GitHub Actions
// is told forever that its gate is missing, and the only escape
// (git.enforce_commits: false) also silences the two LOCAL gates it still
// wants.
//
// The CONTROL is the reproduced case, and it is what stops this guard from
// swallowing the bug: with the sibling workflows present, deleting
// check-decisions.yml alone is still DRIFT.
func TestGateAbsences_NoWorkflowsAtAll_IsSilent(t *testing.T) {
	dir := gatedRepo(t)

	// CONTROL: siblings present, the gate alone deleted → reported.
	if err := os.Remove(filepath.Join(dir, ".github", "workflows", "check-decisions.yml")); err != nil {
		t.Fatalf("remove check-decisions.yml: %v", err)
	}
	r := CollectStatus(dir, true)
	if len(r.GateAbsences) != 1 || !strings.Contains(r.GateAbsences[0], "check-decisions.yml") {
		t.Fatalf("control: GateAbsences = %v; want the merge gate reported while its "+
			"siblings are installed", r.GateAbsences)
	}

	// Now no logmind workflows at all — this repo never took that path.
	if err := os.RemoveAll(filepath.Join(dir, ".github", "workflows")); err != nil {
		t.Fatalf("rm workflows: %v", err)
	}
	r = CollectStatus(dir, true)
	if len(r.GateAbsences) != 0 {
		t.Errorf("GateAbsences = %v; want none — no logmind workflow was ever "+
			"installed here", r.GateAbsences)
	}
	if r.Overall != "OK" {
		t.Errorf("Overall = %q; want OK", r.Overall)
	}

	// …and the LOCAL gates are unaffected by that: they are per-clone and
	// their absence still counts.
	if err := os.Remove(filepath.Join(hooksDirOf(t, dir), "commit-msg")); err != nil {
		t.Fatalf("remove commit-msg: %v", err)
	}
	r = CollectStatus(dir, true)
	if len(r.GateAbsences) != 1 || !strings.Contains(r.GateAbsences[0], "commit-msg") {
		t.Fatalf("GateAbsences = %v; want the commit-msg hook reported — no-workflows "+
			"is a statement about CI, not about the local interception points", r.GateAbsences)
	}
}
