package hooks

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// update mirrors the snapshot pattern from internal/cli — `make
// snapshot` passes it to regenerate testdata/ from the current Go
// output. The hook bodies are byte-identical against Python v0.6.14
// modulo the version-marker line; the golden files capture the Go
// shape (with `1.0.0-dev`) and the parity test below regenerates
// the Python shape on the fly and normalises the marker line.
var update = flag.Bool("update", false, "regenerate testdata/*.golden files from current Go output")

// TestPostMergeBody_MatchesGolden pins the Go-rendered post-merge
// hook body to the canonical bytes captured in testdata. Any drift
// in the embedded shell script breaks this assertion — the goldens
// are CHECKED IN so a refactor that re-orders a comment or trims a
// trailing newline trips CI loudly.
func TestPostMergeBody_MatchesGolden(t *testing.T) {
	checkGolden(t, "post-merge.golden", BuildPostMergeBody())
}

// TestPostRewriteBody_MatchesGolden — same role for the post-rewrite
// hook.
func TestPostRewriteBody_MatchesGolden(t *testing.T) {
	checkGolden(t, "post-rewrite.golden", BuildPostRewriteBody())
}

// TestCommitMsgBody_MatchesGolden pins the v2.0.0 enforcing commit-msg
// hook body. Unlike post-merge/post-rewrite this one has no Python
// ancestor to stay byte-identical with (the warn-only v0.6.16 body it
// replaces was Go-only already) — the golden here exists purely so a
// future refactor that reflows the shell script trips CI loudly instead
// of silently drifting every consumer's next `doctor --fix` upgrade.
func TestCommitMsgBody_MatchesGolden(t *testing.T) {
	checkGolden(t, "commit-msg.golden", BuildCommitMsgBody())
}

// TestCommitMsgBody_DelegatesToGuardCommit pins the enforcement contract
// as INTENT, distinct from the byte-golden above: the body must locate
// the message file, delegate the actual decision to
// `logmind guard-commit --layer git-hook`, and fail OPEN (exit 0) when
// logmind isn't on PATH — never fail closed on a missing binary.
//
// Stale-binary hardening: the body no longer does a blind `exit $?` relay
// of guard-commit's exit code (see BuildCommitMsgBody's doc comment) — it
// checks the captured `$rc` against EXACTLY 65 (guard-commit's distinctive
// EX_DATAERR block signal) before aborting, and falls through to fail-open
// otherwise. Pin both the capture and the specific comparison so a future
// edit can't silently regress back to relaying an arbitrary exit code.
func TestCommitMsgBody_DelegatesToGuardCommit(t *testing.T) {
	body := BuildCommitMsgBody()
	for _, must := range []string{
		"logmind guard-commit --layer git-hook",
		"--msg-file \"$MSG_FILE\"",
		"command -v logmind",
		"rc=$?",
		`"$rc" -eq 65`,
	} {
		if !strings.Contains(body, must) {
			t.Errorf("commit-msg body missing %q", must)
		}
	}
	// The old blind relay must be gone: `exit $?` right after the
	// guard-commit invocation would abort on ANY nonzero code, including a
	// stale binary's unrelated error — exactly the bug this hardening
	// fixes.
	if strings.Contains(body, "exit $?") {
		t.Errorf("commit-msg body still does a blind `exit $?` relay; want an explicit rc==65 check")
	}
	// The fail-open path: outside the `command -v logmind` branch (and
	// outside the rc==65 block), the script must still exit 0 (never fail
	// closed on a missing binary or a non-65 rc from a stale logmind).
	if !regexp.MustCompile(`(?m)^exit 0\s*(#.*)?$`).MatchString(body) {
		t.Errorf("commit-msg body has no top-level `exit 0` fail-open fallback")
	}
}

// TestPreCommitBody_MatchesGolden pins the L2a pre-commit hook body — the
// derived-docs pin-preservation guardrail. No Python ancestor (this hook is
// v2.0.0+/Go-only), same as commit-msg's golden: exists purely so a future
// refactor that reflows the shell script trips CI loudly instead of
// silently drifting every consumer's next `doctor --fix`/`init` upgrade.
func TestPreCommitBody_MatchesGolden(t *testing.T) {
	checkGolden(t, "pre-commit.golden", BuildPreCommitBody())
}

// TestPreCommitBody_GatesRestoreAndAlwaysExitsZero pins the L2a contract as
// INTENT, distinct from the byte-golden above: the restore only runs on a
// NON-default branch, and the hook exits 0 unconditionally — it must NEVER
// block a commit. The full guard line (with `!=`, not accidentally
// flippable to `==`/`||`) is pinned so an operator-bug regression trips
// this test instead of silently defeating the gate.
func TestPreCommitBody_GatesRestoreAndAlwaysExitsZero(t *testing.T) {
	body := BuildPreCommitBody()

	for _, must := range []string{
		`current=$(git rev-parse --abbrev-ref HEAD`,
		`default=$(git symbolic-ref --short refs/remotes/origin/HEAD`,
		`default=${default#origin/}`,
	} {
		if !strings.Contains(body, must) {
			t.Errorf("pre-commit body missing branch detection %q", must)
		}
	}

	// The non-default-branch guard itself, pinned in FULL (not just the
	// `!=` prefix) — the same operator-bug protection
	// TestPostRewriteHook_NoRegenOnFeatureBranch applies to the post-rewrite
	// hook's guard: a substitution like `A && B || true` would silently
	// widen when the restore fires.
	const guard = `if [ -n "$current" ] && [ "$current" != "$default" ]; then`
	gi := strings.Index(body, guard)
	if gi < 0 {
		t.Fatalf("pre-commit body missing the non-default-branch guard %q", guard)
	}

	// v2.0.0 4b-bis repair-path fix: the restore target is $target, resolved
	// via the SAME fallback chain as gitcli.DefaultBranchMergeBase —
	// merge-base(origin/$default, HEAD), then merge-base($default, HEAD),
	// then a bare HEAD if neither resolves — NOT a bare "HEAD" restore.
	// Restoring to HEAD unconditionally would silently re-affirm a branch
	// that already diverged before this commit instead of repairing it.
	for _, must := range []string{
		`target=$(git merge-base "origin/$default" HEAD 2>/dev/null || true)`,
		`[ -z "$target" ] && target=$(git merge-base "$default" HEAD 2>/dev/null || true)`,
		`[ -z "$target" ] && target=HEAD`,
	} {
		if !strings.Contains(body, must) {
			t.Errorf("pre-commit body missing merge-base target resolution %q", must)
		}
		if n := strings.Count(body, must); n != 1 {
			t.Errorf("%q appears %d time(s); want exactly 1", must, n)
		}
	}

	const restore = `git checkout "$target" -- docs/timeline.md docs/file-structure.md`
	ri := strings.Index(body, restore)
	if ri < 0 {
		t.Fatalf("pre-commit body missing the restore command %q", restore)
	}
	if n := strings.Count(body, restore); n != 1 {
		t.Errorf("restore command appears %d time(s); want exactly 1", n)
	}
	if ri < gi {
		t.Errorf("restore command appears BEFORE the non-default-branch guard — it would also run on the default branch")
	}
	if strings.Contains(body, "git checkout HEAD -- docs/timeline.md") {
		t.Errorf("pre-commit body still restores unconditionally to HEAD — the 4b-bis fix must target $target (the merge-base), not a bare HEAD")
	}

	// MUST NEVER block a commit: a top-level (unindented) `exit 0` outside
	// any conditional guarantees this regardless of the restore's outcome,
	// missing `.logmind/config.yml`, or a branch-detection failure.
	if !regexp.MustCompile(`(?m)^exit 0\s*(#.*)?$`).MatchString(body) {
		t.Errorf("pre-commit body has no top-level `exit 0` — it must NEVER block a commit")
	}
}

// TestPostMergeBody_RollupInvariants pins the Slice 2 roll-up contract as
// INTENT (distinct from the byte-golden): the post-merge hook MUST regenerate
// the timeline + file-structure (so a main-canonical repo rebuilds its §1.6.4
// union on every local merge — the regen command dispatches on config) and
// MUST NOT push to a branch (no push-to-default → no GITHUB_TOKEN stranding /
// self-trigger loop). A golden regen that silently violated either still
// trips THIS test. (The "leave regens unstaged" v0.6.7 behavior is pinned by
// the golden + the in-body comment, which itself names `git add`, so we don't
// substring-match that here.)
func TestPostMergeBody_RollupInvariants(t *testing.T) {
	body := BuildPostMergeBody()
	for _, must := range []string{
		"logmind timeline --write docs/timeline.md",
		"logmind file-structure --write docs/file-structure.md",
	} {
		if !strings.Contains(body, must) {
			t.Errorf("post-merge body missing roll-up regen %q", must)
		}
	}
	// Match an actual push invocation, not a stray mention: a `git push` at a
	// shell-command position (line start, after indentation). The body has
	// none — the roll-up never pushes.
	if regexp.MustCompile(`(?m)^\s*git push`).MatchString(body) {
		t.Errorf("post-merge body issues `git push` — the roll-up must NEVER push to a branch")
	}
}

// TestPostMergeBody_IntegrationPointModeSkipsFeatureBranchRegen pins the
// v2.0.0 B6 adoption gate for the post-merge hook: on a non-default branch,
// the hook exits BEFORE the roll-up regen ONLY when THIS repo declared
// `derived_docs: {mode: integration-point}` (detected via
// derivedDocsIntegrationPointGrep against .logmind/config.yml). "driver"
// (the default) falls through and regenerates regardless of branch — see
// TestPostMergeBody_DriverModeNoLongerUnconditionallySkipsNonDefaultBranch
// for the companion proof that the old unconditional skip is gone.
//
// Approach: same as the post-rewrite sibling test — no in-package helper
// fires a real post-merge hook against a live .logmind/config.yml, so this
// asserts on the BODY STRING. It proves the gate structurally: the
// mode-check's `exit 0` sits INSIDE the non-default-branch guard, strictly
// BEFORE the roll-up regen block (itself guarded only by `[ -d docs ]`, no
// branch condition — see the sibling test below).
func TestPostMergeBody_IntegrationPointModeSkipsFeatureBranchRegen(t *testing.T) {
	body := BuildPostMergeBody()

	const branchGuard = `if [ -n "$current" ] && [ "$current" != "$default" ]; then`
	bgi := strings.Index(body, branchGuard)
	if bgi < 0 {
		t.Fatalf("post-merge body missing the non-default-branch guard %q", branchGuard)
	}

	modeCheck := "if " + derivedDocsIntegrationPointGrep + "; then"
	mci := strings.Index(body, modeCheck)
	if mci < 0 {
		t.Fatalf("post-merge body missing the integration-point mode check %q", modeCheck)
	}
	if mci < bgi {
		t.Errorf("mode check appears BEFORE the non-default-branch guard %q — it must be nested inside it", branchGuard)
	}
	// No bare `exit 0` between the guard opening and the mode check — that
	// would be the OLD unconditional skip lingering alongside the new gate.
	between := body[bgi:mci]
	if strings.Contains(between, "exit 0") {
		t.Errorf("post-merge body has an exit 0 between the branch guard and the mode check — the old unconditional skip must be fully replaced, not just supplemented")
	}

	exitRel := strings.Index(body[mci:], "exit 0")
	if exitRel < 0 {
		t.Fatalf("post-merge body's mode check has no exit 0")
	}
	exitIdx := mci + exitRel

	for _, action := range []string{
		"logmind timeline --write docs/timeline.md",
		"logmind file-structure --write docs/file-structure.md",
	} {
		if n := strings.Count(body, action); n != 1 {
			t.Errorf("action %q appears %d time(s); want exactly 1", action, n)
			continue
		}
		if strings.Index(body, action) < exitIdx {
			t.Errorf("action %q appears BEFORE the integration-point mode's exit 0", action)
		}
	}
}

// TestPostMergeBody_DriverModeNoLongerUnconditionallySkipsNonDefaultBranch
// pins the inversion ruling 3 requires: the roll-up regen actions
// (BuildPostMergeBody's final `[ -d docs ]` block) must NOT be nested
// inside any `current != default` OR `current == default` branch
// condition — driver mode (the default) regenerates after every local
// merge regardless of branch, matching the pre-v2.0.0 behavior. Only the
// integration-point mode check (pinned above) may skip it, and only via
// its own explicit `exit 0`.
func TestPostMergeBody_DriverModeNoLongerUnconditionallySkipsNonDefaultBranch(t *testing.T) {
	body := BuildPostMergeBody()
	const finalGuard = `if [ -d docs ]; then`
	fgi := strings.LastIndex(body, finalGuard)
	if fgi < 0 {
		t.Fatalf("post-merge body missing the final regen guard %q", finalGuard)
	}
	// The two roll-up regen calls must sit inside THIS `[ -d docs ]` guard —
	// not additionally re-gated on a branch-equality condition. We already
	// proved (in the sibling test) that reaching this point at all requires
	// falling through both the non-default-branch mode check AND the
	// default-branch fast-forward check without hitting either `exit 0`; the
	// only condition left immediately guarding the actions is `-d docs`.
	for _, action := range []string{
		"logmind timeline --write docs/timeline.md",
		"logmind file-structure --write docs/file-structure.md",
	} {
		idx := strings.LastIndex(body, action)
		if idx < fgi {
			t.Errorf("action %q does not appear after the final `[ -d docs ]` guard %q", action, finalGuard)
		}
	}
}

// TestPostRewriteHook_IntegrationPointModeSkipsFeatureBranchRegen pins the
// v2.0.0 derived-docs-on-main invariant for the post-rewrite hook, UPDATED
// for the B6 adoption gate: a rebase/amend on a non-default branch skips
// the regen (and `git add`) of docs/timeline.md + docs/file-structure.md
// ONLY when THIS repo declared `derived_docs: {mode: integration-point}`
// (detected via derivedDocsIntegrationPointGrep against
// .logmind/config.yml). "driver" (the default) regenerates regardless of
// branch — the exact pre-v2.0.0 behavior; see
// TestPostRewriteHook_DriverModeNoLongerGatesOnBranch for the companion
// proof that the OLD unconditional branch guard is gone.
//
// Approach: the hooks package has no in-test helper to install + fire a real
// post-rewrite hook against a live .logmind/config.yml (the
// initRepoWithLogmind / installHooks / isStagedOrDirty helpers the plan
// sketches live in internal/cli), and the task forbids inventing
// duplicates — so this asserts on the BODY STRING instead. It proves the
// gate structurally: the mode-check's `exit 0` sits INSIDE the
// non-default-branch guard, and strictly BEFORE the (now
// branch-unconditional) regen/add actions — so an interpreter reading
// top-to-bottom exits before ever reaching those actions when (current !=
// default) AND the repo's config matches integration-point mode.
func TestPostRewriteHook_IntegrationPointModeSkipsFeatureBranchRegen(t *testing.T) {
	body := BuildPostRewriteBody()

	// The branch-detection the non-default-branch guard depends on (mirrors
	// post-merge).
	for _, must := range []string{
		`current=$(git rev-parse --abbrev-ref HEAD`,
		`default=$(git symbolic-ref --short refs/remotes/origin/HEAD`,
		`default=${default#origin/}`,
	} {
		if !strings.Contains(body, must) {
			t.Errorf("post-rewrite body missing branch detection %q", must)
		}
	}

	const branchGuard = `if [ -n "$current" ] && [ "$current" != "$default" ]; then`
	bgi := strings.Index(body, branchGuard)
	if bgi < 0 {
		t.Fatalf("post-rewrite body missing the non-default-branch guard %q", branchGuard)
	}

	modeCheck := "if " + derivedDocsIntegrationPointGrep + "; then"
	mci := strings.Index(body, modeCheck)
	if mci < 0 {
		t.Fatalf("post-rewrite body missing the integration-point mode check %q", modeCheck)
	}
	if mci < bgi {
		t.Errorf("mode check appears BEFORE the non-default-branch guard %q — it must be nested inside it", branchGuard)
	}

	exitRel := strings.Index(body[mci:], "exit 0")
	if exitRel < 0 {
		t.Fatalf("post-rewrite body's mode check has no exit 0")
	}
	exitIdx := mci + exitRel

	// Every regen / stage action must appear EXACTLY ONCE and ONLY AFTER the
	// mode check's exit 0 — so a non-default branch in integration-point
	// mode exits before ever reaching these actions, while driver mode (no
	// grep match) falls through to them regardless of branch.
	for _, action := range []string{
		"logmind timeline --write docs/timeline.md",
		"logmind file-structure --write docs/file-structure.md",
		"git add docs/timeline.md docs/file-structure.md",
	} {
		if n := strings.Count(body, action); n != 1 {
			t.Errorf("action %q appears %d time(s); want exactly 1", action, n)
			continue
		}
		if strings.Index(body, action) < exitIdx {
			t.Errorf("action %q appears BEFORE the integration-point mode's exit 0", action)
		}
	}
}

// TestPostRewriteHook_DriverModeNoLongerGatesOnBranch pins the inversion
// ruling 3 requires: the OLD unconditional `current == default` guard
// around the regen/add actions must be GONE. Driver mode (the default —
// including a repo with no `derived_docs:` section at all) must regenerate
// on every branch after a rebase/amend, matching the pre-v2.0.0 behavior;
// only an explicit integration-point-mode grep match (pinned by
// TestPostRewriteHook_IntegrationPointModeSkipsFeatureBranchRegen above)
// may skip it.
func TestPostRewriteHook_DriverModeNoLongerGatesOnBranch(t *testing.T) {
	body := BuildPostRewriteBody()
	if strings.Contains(body, `[ "$current" = "$default" ]`) {
		t.Errorf("post-rewrite body still gates the regen on current==default — driver mode must regenerate on every branch regardless of default-branch equality")
	}
}

func TestInstallPostMerge_FreshInstall(t *testing.T) {
	repo := tempRepoWithHooks(t)
	changed, err := InstallPostMerge(repo)
	if err != nil {
		t.Fatalf("InstallPostMerge: %v", err)
	}
	if !changed {
		t.Fatalf("InstallPostMerge returned changed=false on a fresh repo; want true")
	}
	body, err := os.ReadFile(filepath.Join(repo, ".git", "hooks", "post-merge"))
	if err != nil {
		t.Fatalf("read post-merge: %v", err)
	}
	if string(body) != BuildPostMergeBody() {
		t.Fatalf("installed body drifts from BuildPostMergeBody()")
	}
}

func TestInstallPostMerge_Idempotent(t *testing.T) {
	repo := tempRepoWithHooks(t)
	if _, err := InstallPostMerge(repo); err != nil {
		t.Fatalf("first install: %v", err)
	}
	changed, err := InstallPostMerge(repo)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if changed {
		t.Fatalf("InstallPostMerge changed=true on identical second install; want false")
	}
}

func TestInstallPostMerge_LeavesForeignHook(t *testing.T) {
	repo := tempRepoWithHooks(t)
	hookPath := filepath.Join(repo, ".git", "hooks", "post-merge")
	custom := "#!/bin/sh\n# user's custom hook\necho hi\n"
	if err := os.WriteFile(hookPath, []byte(custom), 0o755); err != nil {
		t.Fatalf("seed custom hook: %v", err)
	}
	changed, err := InstallPostMerge(repo)
	if err != nil {
		t.Fatalf("InstallPostMerge: %v", err)
	}
	if changed {
		t.Fatalf("InstallPostMerge changed=true on foreign hook; want false (leave alone)")
	}
	got, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("re-read hook: %v", err)
	}
	if string(got) != custom {
		t.Fatalf("foreign hook was modified:\n%s", got)
	}
}

func TestInstallPostMerge_MissingHooksDir(t *testing.T) {
	dir := t.TempDir()
	// No .git/hooks here — installer should return (false, nil), not
	// error. Matches the Python install_post_merge_hook line 229-230.
	changed, err := InstallPostMerge(dir)
	if err != nil {
		t.Fatalf("InstallPostMerge: %v", err)
	}
	if changed {
		t.Fatalf("changed=true with no .git/hooks/; want false")
	}
}

func TestInstallCommitMsg_FreshInstall(t *testing.T) {
	repo := tempRepoWithHooks(t)
	changed, err := InstallCommitMsg(repo)
	if err != nil {
		t.Fatalf("InstallCommitMsg: %v", err)
	}
	if !changed {
		t.Fatalf("InstallCommitMsg returned changed=false on a fresh repo; want true")
	}
	body, err := os.ReadFile(filepath.Join(repo, ".git", "hooks", "commit-msg"))
	if err != nil {
		t.Fatalf("read commit-msg: %v", err)
	}
	if string(body) != BuildCommitMsgBody() {
		t.Fatalf("installed body drifts from BuildCommitMsgBody()")
	}
}

func TestInstallCommitMsg_Idempotent(t *testing.T) {
	repo := tempRepoWithHooks(t)
	if _, err := InstallCommitMsg(repo); err != nil {
		t.Fatalf("first install: %v", err)
	}
	changed, err := InstallCommitMsg(repo)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if changed {
		t.Fatalf("InstallCommitMsg changed=true on identical second install; want false")
	}
}

// TestInstallCommitMsg_UpgradesWarnOnlyV0616Body pins the "no new install
// wiring needed" contract from the PR spec: an existing repo's v0.6.16
// warn-only commit-msg hook (same CommitMsgMarker, different body) MUST
// be recognized as OURS and overwritten with the current enforcing body
// — installHook's existing "ours + body differs → overwrite" path is
// what carries out this upgrade on the next init/doctor --fix, with zero
// new code.
func TestInstallCommitMsg_UpgradesWarnOnlyV0616Body(t *testing.T) {
	repo := tempRepoWithHooks(t)
	hookPath := filepath.Join(repo, ".git", "hooks", "commit-msg")
	legacyWarnOnly := "#!/bin/sh\n" +
		"# logmind commit-msg hook\n" +
		HookVersionPrefix + "0.6.16\n" +
		"MSG_FILE=\"$1\"\n" +
		"if [ -z \"$MSG_FILE\" ] || [ ! -f \"$MSG_FILE\" ]; then exit 0; fi\n" +
		"if grep -q '\\[skip-logmind\\]' \"$MSG_FILE\"; then\n" +
		"    echo 'logmind: [skip-logmind] detected' >&2\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(hookPath, []byte(legacyWarnOnly), 0o755); err != nil {
		t.Fatalf("seed legacy warn-only hook: %v", err)
	}

	changed, err := InstallCommitMsg(repo)
	if err != nil {
		t.Fatalf("InstallCommitMsg: %v", err)
	}
	if !changed {
		t.Fatalf("InstallCommitMsg changed=false over a legacy warn-only body; want true (upgrade)")
	}
	got, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("re-read hook: %v", err)
	}
	if string(got) != BuildCommitMsgBody() {
		t.Fatalf("hook was not upgraded to the current enforcing body")
	}

	fi, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("stat upgraded hook: %v", err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o; want 0755 (executable bit must survive the atomic rewrite)", fi.Mode().Perm())
	}

	// Guards the atomic-write fix: overwriting this EXISTING hook must go
	// through atomicio's temp-sibling-plus-rename, leaving no ".tmp-*"
	// residue in .git/hooks/ behind.
	entries, err := os.ReadDir(filepath.Dir(hookPath))
	if err != nil {
		t.Fatalf("read hooks dir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file after hook upgrade: %s", e.Name())
		}
	}
}

func TestInstallCommitMsg_LeavesForeignHook(t *testing.T) {
	repo := tempRepoWithHooks(t)
	hookPath := filepath.Join(repo, ".git", "hooks", "commit-msg")
	custom := "#!/bin/sh\n# user's custom commit-msg hook\necho hi\n"
	if err := os.WriteFile(hookPath, []byte(custom), 0o755); err != nil {
		t.Fatalf("seed custom hook: %v", err)
	}
	changed, err := InstallCommitMsg(repo)
	if err != nil {
		t.Fatalf("InstallCommitMsg: %v", err)
	}
	if changed {
		t.Fatalf("InstallCommitMsg changed=true on foreign hook; want false (leave alone)")
	}
	got, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("re-read hook: %v", err)
	}
	if string(got) != custom {
		t.Fatalf("foreign hook was modified:\n%s", got)
	}
}

func TestInstallPreCommit_FreshInstall(t *testing.T) {
	repo := tempRepoWithHooks(t)
	changed, err := InstallPreCommit(repo)
	if err != nil {
		t.Fatalf("InstallPreCommit: %v", err)
	}
	if !changed {
		t.Fatalf("InstallPreCommit returned changed=false on a fresh repo; want true")
	}
	body, err := os.ReadFile(filepath.Join(repo, ".git", "hooks", "pre-commit"))
	if err != nil {
		t.Fatalf("read pre-commit: %v", err)
	}
	if string(body) != BuildPreCommitBody() {
		t.Fatalf("installed body drifts from BuildPreCommitBody()")
	}
}

func TestInstallPreCommit_Idempotent(t *testing.T) {
	repo := tempRepoWithHooks(t)
	if _, err := InstallPreCommit(repo); err != nil {
		t.Fatalf("first install: %v", err)
	}
	changed, err := InstallPreCommit(repo)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if changed {
		t.Fatalf("InstallPreCommit changed=true on identical second install; want false")
	}
}

// TestInstallPreCommit_LeavesForeignHook pins the conservative-interop rule
// (ruling 2): a pre-commit hook that already exists WITHOUT PreCommitMarker
// is left completely untouched — no clobber, no append. Two distinct
// foreign shapes are exercised: a hand-written hook, and the legacy
// `logmind install-hook`-installed `check-decisions` body (a DIFFERENT
// logmind-owned hook that predates this feature and carries no
// PreCommitMarker of its own — see PreCommitMarker's doc comment).
func TestInstallPreCommit_LeavesForeignHook(t *testing.T) {
	cases := map[string]string{
		"hand-written": "#!/bin/sh\n# user's custom pre-commit hook\necho hi\n",
		"legacy check-decisions": "#!/bin/sh\n" +
			"# logmind check-decisions — hang-guarded (issue #213): run under a\n" +
			"# deadline so a wedged logmind binary can never stall `git commit`.\n" +
			"if command -v logmind >/dev/null 2>&1; then\n" +
			"    logmind check-decisions &\n" +
			"    __lm_pid=$!\n" +
			"    wait \"$__lm_pid\" 2>/dev/null\n" +
			"    __lm_rc=$?\n" +
			"    exit \"$__lm_rc\"\n" +
			"fi\n",
	}
	for name, custom := range cases {
		t.Run(name, func(t *testing.T) {
			repo := tempRepoWithHooks(t)
			hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
			if err := os.WriteFile(hookPath, []byte(custom), 0o755); err != nil {
				t.Fatalf("seed foreign hook: %v", err)
			}
			changed, err := InstallPreCommit(repo)
			if err != nil {
				t.Fatalf("InstallPreCommit: %v", err)
			}
			if changed {
				t.Fatalf("InstallPreCommit changed=true on foreign hook; want false (leave alone)")
			}
			got, err := os.ReadFile(hookPath)
			if err != nil {
				t.Fatalf("re-read hook: %v", err)
			}
			if string(got) != custom {
				t.Fatalf("foreign hook was modified:\n%s", got)
			}
		})
	}
}

// TestPreCommitHook_EndToEnd_RawGitCommitDoesNotCarryDirtyDerivedDoc is the
// L2a proof (ruling task 3b): with the pre-commit hook installed, a raw
// `git commit -am` (bypassing `logmind log` and its own L1 restore) on a
// feature branch must NOT let a dirtied docs/timeline.md ride into the
// commit. This is the exact gap L2a exists to close — the named trigger
// scenario is `logmind warp` writing the default branch's newer copy into
// the working tree, but any raw commit reaches the same hook.
//
// Needs a REAL git repository (not just the `.git/hooks/` shell
// tempRepoWithHooks builds) so `git commit` actually invokes the installed
// hook — see initRealGitRepo.
func TestPreCommitHook_EndToEnd_RawGitCommitDoesNotCarryDirtyDerivedDoc(t *testing.T) {
	repo := initRealGitRepo(t)

	docsDir := filepath.Join(repo, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	timelinePath := filepath.Join(docsDir, "timeline.md")
	original := "# Timeline\n\noriginal content\n"
	if err := os.WriteFile(timelinePath, []byte(original), 0o644); err != nil {
		t.Fatalf("write timeline.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "file-structure.md"), []byte("# File structure\n"), 0o644); err != nil {
		t.Fatalf("write file-structure.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".logmind"), 0o755); err != nil {
		t.Fatalf("mkdir .logmind: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".logmind", "config.yml"), []byte("git: {}\n"), 0o644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	gitCommitAll(t, repo, "initial scaffold")

	// Install the pre-commit hook — mirrors the init/doctor --fix wiring.
	if _, err := InstallPreCommit(repo); err != nil {
		t.Fatalf("InstallPreCommit: %v", err)
	}

	// Move to a feature branch — the restore only fires when current !=
	// default, and DefaultBranch resolves to whatever local branch `git
	// init` created (main or master), so any freshly-created branch name
	// differs from it.
	gitIn(t, repo, "checkout", "-b", "feat/dirty-timeline")

	// Dirty docs/timeline.md — simulates `logmind warp` (or any stray
	// write) pulling in a different copy.
	dirty := "# Timeline\n\nDIRTY simulated warp content\n"
	if err := os.WriteFile(timelinePath, []byte(dirty), 0o644); err != nil {
		t.Fatalf("dirty timeline.md: %v", err)
	}

	// Raw `git commit -am` — NOT `logmind log`. The pre-commit hook must
	// fire and restore docs/timeline.md to HEAD before the commit is built.
	gitIn(t, repo, "commit", "-am", "raw commit bypassing logmind log")

	committed, ok := gitShowFile(repo, "HEAD", "docs/timeline.md")
	if !ok {
		t.Fatalf("git show HEAD:docs/timeline.md failed")
	}
	if committed != original {
		t.Fatalf("commit carried the dirtied docs/timeline.md; L2a pre-commit hook did not restore it\nwant:\n%s\ngot:\n%s", original, committed)
	}

	// `git checkout HEAD --` restores both the index AND the working tree —
	// confirm the on-disk copy was restored too, not just what got committed.
	onDisk, err := os.ReadFile(timelinePath)
	if err != nil {
		t.Fatalf("read timeline.md: %v", err)
	}
	if string(onDisk) != original {
		t.Fatalf("working tree docs/timeline.md not restored either\nwant:\n%s\ngot:\n%s", original, onDisk)
	}
}

// TestPreCommitHook_EndToEnd_RepairsAlreadyDivergedBranch is the v2.0.0
// 4b-bis repair-path proof for L2a specifically: unlike the sibling test
// above (which dirties the WORKING TREE only), here a commit ALREADY on the
// branch's HEAD carries a diverged copy of docs/timeline.md — simulating an
// old binary's local regen, or a hand edit, having landed BEFORE the
// pre-commit hook was installed (or before this fix existed at all; either
// way, the hook can't have prevented it). The user then upgrades/installs
// the hook (e.g. via `logmind doctor --fix`), follows the CI gate's own
// repair advice (`git checkout main -- docs/timeline.md`, the local-repo
// stand-in for `git checkout origin/main -- ...` when there's no origin
// remote), and commits raw — bypassing `logmind log` entirely, so this
// exercises the pre-commit hook body itself, not internal/cli's Go-level L1
// restore. Before the 4b-bis fix (a bare `git checkout HEAD --`), this hook
// would silently undo the repair by re-checking out the stale diverged HEAD
// copy right before the commit was built.
func TestPreCommitHook_EndToEnd_RepairsAlreadyDivergedBranch(t *testing.T) {
	repo := initRealGitRepo(t)

	docsDir := filepath.Join(repo, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	timelinePath := filepath.Join(docsDir, "timeline.md")
	mainContent := "# Timeline\n\nmain content\n"
	if err := os.WriteFile(timelinePath, []byte(mainContent), 0o644); err != nil {
		t.Fatalf("write timeline.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "file-structure.md"), []byte("# File structure\n"), 0o644); err != nil {
		t.Fatalf("write file-structure.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".logmind"), 0o755); err != nil {
		t.Fatalf("mkdir .logmind: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".logmind", "config.yml"), []byte("git: {}\n"), 0o644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	gitCommitAll(t, repo, "initial scaffold")

	// No pre-commit hook installed yet — this commit represents the state
	// BEFORE the fix (or before the hook was ever set up), so nothing stops
	// the divergence from landing.
	gitIn(t, repo, "checkout", "-b", "feat/already-diverged")
	stale := "# Timeline\n\nSTALE diverged content\n"
	if err := os.WriteFile(timelinePath, []byte(stale), 0o644); err != nil {
		t.Fatalf("write stale timeline.md: %v", err)
	}
	gitIn(t, repo, "commit", "-am", "bad regen (pre-existing divergence, no hook yet)")

	// NOW install the hook — e.g. the user ran `logmind doctor --fix` (or
	// upgraded to a version that installs it) AFTER the divergence already
	// landed. This is the realistic order: the hook can only guard commits
	// made after it exists.
	if _, err := InstallPreCommit(repo); err != nil {
		t.Fatalf("InstallPreCommit: %v", err)
	}

	// The repair: follow the CI gate's own advice, using the local `main`
	// branch as the no-origin-remote stand-in for `origin/main`.
	gitIn(t, repo, "checkout", "main", "--", "docs/timeline.md")
	if got, err := os.ReadFile(timelinePath); err != nil || string(got) != mainContent {
		t.Fatalf("test setup: working tree after manual repair = %q, err=%v; want %q", got, err, mainContent)
	}

	// Commit the fix RAW — bypassing `logmind log` — so only the pre-commit
	// hook (L2a) is in play. Before the 4b-bis fix this would silently
	// restore docs/timeline.md back to the stale diverged HEAD content.
	gitIn(t, repo, "commit", "-am", "repair the diverged derived docs")

	committed, ok := gitShowFile(repo, "HEAD", "docs/timeline.md")
	if !ok {
		t.Fatalf("git show HEAD:docs/timeline.md failed")
	}
	if committed != mainContent {
		t.Fatalf("pre-commit hook undid the repair: committed docs/timeline.md = %q; want main's content %q (the merge-base) — got the stale content back = %v",
			committed, mainContent, committed == stale)
	}
	onDisk, err := os.ReadFile(timelinePath)
	if err != nil {
		t.Fatalf("read timeline.md: %v", err)
	}
	if string(onDisk) != mainContent {
		t.Fatalf("working tree docs/timeline.md after the repair commit = %q; want %q", onDisk, mainContent)
	}
}

func TestExtractVersion_FromInstalledHook(t *testing.T) {
	repo := tempRepoWithHooks(t)
	if _, err := InstallPostMerge(repo); err != nil {
		t.Fatalf("install: %v", err)
	}
	v, ok := ExtractVersion(filepath.Join(repo, ".git", "hooks", "post-merge"))
	if !ok {
		t.Fatalf("ExtractVersion returned ok=false on hook we just installed")
	}
	if v == "" {
		t.Fatalf("ExtractVersion returned empty string")
	}
}

func TestExtractVersion_MissingFile(t *testing.T) {
	dir := t.TempDir()
	v, ok := ExtractVersion(filepath.Join(dir, "does-not-exist"))
	if ok || v != "" {
		t.Fatalf("ExtractVersion(missing) = (%q, %v); want ('', false)", v, ok)
	}
}

func TestExtractVersion_PreV0610Hook(t *testing.T) {
	// Pre-v0.6.10 hooks didn't embed the version marker — extractor
	// must return ok=false (NOT a default-empty-string). Mirrors
	// Python gitattributes.extract_hook_version line 273-279.
	dir := t.TempDir()
	old := "#!/bin/sh\n# logmind post-merge hook\necho old\n"
	path := filepath.Join(dir, "post-merge")
	if err := os.WriteFile(path, []byte(old), 0o755); err != nil {
		t.Fatalf("write old hook: %v", err)
	}
	v, ok := ExtractVersion(path)
	if ok || v != "" {
		t.Fatalf("ExtractVersion(pre-v0.6.10) = (%q, %v); want ('', false)", v, ok)
	}
}

// --- helpers -------------------------------------------------------------

func tempRepoWithHooks(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir .git/hooks: %v", err)
	}
	return dir
}

// initRealGitRepo creates a fresh, REAL git working repo (not just the
// `.git/hooks/` shell tempRepoWithHooks builds) rooted at t.TempDir(),
// configured with a deterministic identity and gpg signing disabled.
// Needed by TestPreCommitHook_EndToEnd_* — those tests run an actual
// `git commit` so the installed pre-commit hook fires for real. Deliberately
// uses plain `os/exec` rather than importing internal/gitcli, keeping this
// package's import graph small (see the package doc comment).
func initRealGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "config", "user.email", "test@example.com")
	gitIn(t, dir, "config", "user.name", "Test")
	gitIn(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

// gitIn runs `git <args>` against dir, failing the test on a non-zero exit.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitCommitAll stages everything and commits it in dir.
func gitCommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", msg)
}

// gitShowFile returns the content of path at ref (`git show <ref>:<path>`)
// within dir. ("", false) on any failure.
func gitShowFile(dir, ref, path string) (string, bool) {
	cmd := exec.Command("git", "show", ref+":"+path)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	return string(out), true
}

func checkGolden(t *testing.T, name, body string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `make snapshot` to create it)", path, err)
	}
	if string(want) != body {
		t.Fatalf("hook body drift vs %s\n--- want ---\n%s\n--- got ---\n%s",
			path, want, body)
	}
}

// repoRootFromCaller walks up from the test file's cwd to the
// directory holding go.mod. Lets us locate the Python source no
// matter where `go test` was launched from.
func repoRootFromCaller(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod walking up from %s", wd)
	return ""
}
