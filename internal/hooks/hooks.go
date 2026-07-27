// Package hooks builds the canonical bodies for logmind's git hooks
// (`post-merge`, `post-rewrite`) and installs them under `.git/hooks/`.
//
// The hook BODIES are kept byte-identical to the Python v0.6.14 output
// of src/logmind/core/gitattributes._build_post_merge_hook_body and
// _build_post_rewrite_hook_body. That's a HARD contract: a fixture
// snapshot in hooks_test.go diffs the Go-generated body against a
// golden file extracted from the Python helper. Any drift trips CI.
//
// Why byte-identical: existing v0.6.x repos have these hooks installed.
// The Go binary's `init` / `log` / `install-hook` paths overwrite the
// file when the body has changed — so if the Go body is different by
// even one whitespace char, every repo will see a spurious "logmind
// hook updated" message on the next `logmind log` and incur a stat-
// diff cycle. Keeping the bytes identical lets the Python→Go switch
// be invisible to downstream repos.
//
// Version marker contract: each body embeds a line
//
//	# logmind-hook-version: <Version>
//
// where <Version> is read from internal/version.Version. logmind
// doctor reads this back to detect drift between the binary that
// wrote the hook and the binary currently running. The PREFIX
// constant is exported (HookVersionPrefix) because doctor consumes
// it too.
//
// Orphan-branch detection (v0.6.13): the post-merge body MUST include
// the `@{u}` upstream-check short-circuit. Without it, a feature
// branch that was just squash-merged and deleted upstream regenerates
// derived docs against stale state and blocks the next `git checkout
// main`. The Python issue #112 explains the bug surface in detail.
package hooks

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/thrillmade/logmind/internal/atomicio"
	"github.com/thrillmade/logmind/internal/version"
)

// HookVersionPrefix is the literal string prefixing the version marker
// in every installed hook body. Exported so logmind doctor (a later
// wave) can reuse the same constant when grepping installed hooks
// for drift detection — keeps the prefix in one place.
const HookVersionPrefix = "# logmind-hook-version: "

// PostMergeMarker identifies a logmind-installed post-merge hook.
// install-time inspection greps for this string to decide whether the
// existing hook is ours (refresh allowed) or a foreign one (leave
// alone). Mirrors src/logmind/core/gitattributes._POST_MERGE_HOOK_MARKER.
const PostMergeMarker = "# logmind post-merge hook"

// PostRewriteMarker — same role as PostMergeMarker, for the
// post-rewrite hook.
const PostRewriteMarker = "# logmind post-rewrite hook"

// CommitMsgMarker identifies a logmind-installed commit-msg hook.
// v0.6.16 introduced this hook as a warn-only surface for
// `[skip-logmind]` markers in commit subjects; v2.0.0 upgraded the body
// to Layer 2 of the commit-enforcement design (see BuildCommitMsgBody).
const CommitMsgMarker = "# logmind commit-msg hook"

// PreCommitMarker identifies a logmind-installed pre-commit hook — L2a of
// the v2.0.0 derived-docs pin-preservation design (see BuildPreCommitBody).
// Distinct from `install-hook`'s OWN, separate, opt-in pre-commit body
// (internal/cli/install_hook.go's preCommitMarker == "logmind
// check-decisions"): that body predates this one, serves a different
// purpose (blocking undocumented commits), and is intentionally left alone
// by installHook's marker check below — a repo running the legacy
// check-decisions hook is "foreign" from THIS marker's point of view, and
// vice versa. Both markers can coexist in principle, but installHook never
// merges two hook bodies into one file; see BuildPreCommitBody's doc
// comment for the conservative-interop rule this implies.
const PreCommitMarker = "# logmind pre-commit hook"

// hookVersion returns the version string embedded in each hook body.
// Reads from internal/version.Version — kept as a function (not a
// package-level var) so tests can override it via a build-tagged
// alternative file without touching the production code.
func hookVersion() string {
	return version.Version
}

// derivedDocsIntegrationPointGrep is the shell fragment every L0 hook body
// (BuildPostMergeBody / BuildPostRewriteBody) uses to detect this repo's
// derived-docs adoption signal (v2.0.0 B6) WITHOUT a `logmind` subprocess —
// git hooks already gate their whole body on `command -v logmind`, but the
// mode check itself has to work even when that binary can't be trusted to
// answer (or is momentarily absent) mid-hook, and reading one line out of a
// YAML file is cheap enough to inline directly. Matches `mode:
// integration-point`, optionally quoted, allowing leading indentation and
// trailing whitespace — the exact shape DerivedDocsConfig.Mode's YAML tag
// renders as. Kept as ONE shared Go constant so both hook bodies (and any
// future one) never drift from each other on this string.
const derivedDocsIntegrationPointGrep = `grep -Eq '^[[:space:]]*mode:[[:space:]]*"?integration-point"?[[:space:]]*$' .logmind/config.yml 2>/dev/null`

// BuildPostMergeBody returns the canonical post-merge hook body for
// the currently-running logmind binary. Byte-identical to the Python
// v0.6.16 _build_post_merge_hook_body output when run against a
// matching version constant.
//
// The body is a verbatim string concatenation — we deliberately do
// NOT template it with a Go text/template, because Python uses raw
// string literals and any templating layer risks introducing
// whitespace drift.
//
// v0.6.16 carry-forward: the v0.6.15 blanket default-branch skip is
// replaced with a HEAD-vs-origin check (skip only on a fast-forward
// pull-up). Local merges that introduce new commits not yet on origin
// (the multi-branch self-heal case) MUST still trigger regen. See
// the v0.6.16 inline comment block below for the contract.
//
// Main-canonical timeline roll-up — this hook is the LOCAL reconciler and
// needs NO change for it. Its `logmind timeline --write` call always rebuilds
// the §1.6.4 union (the sole timeline model as of v2.0.0) from the full merged
// working tree — no hook-body edit, no HookVersionPrefix bump, no fleet-wide
// "hook updated" churn. Branch detail pages are KEPT (never folded), so the
// union always has its sources. The
// server-side reconciler is the regen-timeline.yml workflow's
// check-derived-docs job (PR #159): in a repo that has adopted
// `derived_docs: {mode: integration-point}` it BLOCKS a PR that modifies
// either derived doc (and passes with an explanation otherwise), and it
// regenerates + pushes both files on every push to the default branch. We
// deliberately add NO push-to-default trigger here: it would reintroduce
// the GITHUB_TOKEN-stranding + self-trigger loop that server-side design
// exists to avoid. The TestPostMergeBody_RollupInvariants guard pins
// "regenerates the timeline, never pushes" even across a golden regen.
//
// v2.0.0 B6 adoption gate: the non-default-branch skip below fires ONLY when
// THIS repo has opted into `derived_docs: {mode: integration-point}` (see
// derivedDocsIntegrationPointGrep). "driver" (the default — including a repo
// with no `derived_docs:` section at all) regenerates on every branch, the
// exact pre-v2.0.0 behavior; that's the whole point of the adoption gate — a
// v2 binary must never silently change a driver-mode repo's behavior.
func BuildPostMergeBody() string {
	return "#!/bin/sh\n" +
		"# logmind post-merge hook\n" +
		HookVersionPrefix + hookVersion() + "\n" +
		"# Installed by `logmind init` (v0.3.0+). After every merge, regenerate\n" +
		"# derived docs from the FULL post-merge working tree. Belt + suspenders\n" +
		"# with the merge driver in .gitattributes — the driver fires per-file\n" +
		"# during conflict resolution, before sibling non-conflicted files (e.g.\n" +
		"# the merged-in branch's docs/decisions-branches/<branch>.md) are\n" +
		"# checked out. This hook runs once at the end and sweeps any incomplete\n" +
		"# regenerations.\n" +
		"#\n" +
		"# v0.6.7 bug fix: regenerate but do NOT git add. Previously the hook\n" +
		"# auto-staged docs/timeline.md + docs/file-structure.md, but the\n" +
		"# staged-but-uncommitted files then blocked `git checkout main` on\n" +
		"# every PR cycle (post-merge fires from `git pull --rebase` after a\n" +
		"# squash merge — there's no commit being constructed, so staging was\n" +
		"# wrong). Leaving them as unstaged modifications lets the next\n" +
		"# `logmind log` pick them up cleanly without blocking branch switches.\n" +
		"#\n" +
		"# v0.6.10: the line above embeds the binary version that wrote this hook,\n" +
		"# so doctor reports stale-hook drift loudly when the user's local CLI\n" +
		"# binary is stale relative to the workflow's installed version.\n" +
		"#\n" +
		"# v0.6.13 / issue #112: skip regen entirely when the current branch is\n" +
		"# orphaned — i.e., we're on a feature branch that was just squash-merged\n" +
		"# on origin and the remote branch has been deleted. The regen would\n" +
		"# produce content that conflicts with main's just-merged state, and the\n" +
		"# resulting unstaged modifications block `gh pr merge --delete-branch`'s\n" +
		"# follow-up `git checkout main`. Detection without network calls:\n" +
		"# `git rev-parse --abbrev-ref @{u}` returns nonzero / empty when the\n" +
		"# upstream is gone after `git fetch --prune`. Detection failure → fall\n" +
		"# through to today's behavior (regen + leave unstaged); never block on\n" +
		"# detection failure.\n" +
		"\n" +
		"if command -v logmind >/dev/null 2>&1 && [ -f .logmind/config.yml ]; then\n" +
		"  # Orphan-branch check: if our upstream tracking ref no longer exists,\n" +
		"  # we were just merged-and-deleted; skip regen.\n" +
		"  upstream=$(git rev-parse --abbrev-ref @{u} 2>/dev/null || true)\n" +
		"  if [ -n \"$upstream\" ]; then\n" +
		"    if ! git rev-parse --verify --quiet \"refs/remotes/$upstream\" >/dev/null 2>&1; then\n" +
		"      # Upstream config says <upstream> but no such remote-tracking ref\n" +
		"      # exists (typical after `git fetch --prune` removes the merged\n" +
		"      # branch). Skip regen — main will regen correctly after checkout.\n" +
		"      exit 0\n" +
		"    fi\n" +
		"  fi\n" +
		"  # v0.6.16 (replaces v0.6.15's blanket default-branch skip): on the\n" +
		"  # default branch, only skip when HEAD already matches origin/<default>\n" +
		"  # — i.e., a fast-forward pull-up from server where regen-timeline.yml\n" +
		"  # has already produced the authoritative timeline. For local merges\n" +
		"  # that introduced new commits not yet on origin (the multi-branch\n" +
		"  # self-heal case: `git merge feat-branch` on main), regen MUST fire\n" +
		"  # so the working tree reflects the merged-in decision files. v0.6.15's\n" +
		"  # blanket skip dropped Decision-B-style entries from local main; the\n" +
		"  # multi-branch self-heal regression test in tests/test_merge_driver.py\n" +
		"  # catches this. Surfaces the bug PRE-merge instead of in a downstream\n" +
		"  # check-derived-docs failure weeks later.\n" +
		"  current=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)\n" +
		"  default=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null || true)\n" +
		"  # --short on a remote symbolic-ref leaves the `origin/` prefix\n" +
		"  # in place; strip it so the bare-name comparison below works.\n" +
		"  default=${default#origin/}\n" +
		"  [ -z \"$default\" ] && default=main\n" +
		"  if [ -n \"$current\" ] && [ \"$current\" != \"$default\" ]; then\n" +
		"    # v2.0.0 B6: only skip regen here when THIS repo declared\n" +
		"    # derived_docs.mode: integration-point — the branch must then keep\n" +
		"    # the derived docs byte-identical to its main merge-base (the\n" +
		"    # zero-conflict invariant); main regenerates post-merge. A repo\n" +
		"    # that hasn't adopted (mode unset or \"driver\", the default) falls\n" +
		"    # through and regenerates on every branch — the pre-v2.0.0 behavior.\n" +
		"    if " + derivedDocsIntegrationPointGrep + "; then\n" +
		"      exit 0\n" +
		"    fi\n" +
		"  fi\n" +
		"  if [ -n \"$current\" ] && [ \"$current\" = \"$default\" ]; then\n" +
		"    head_sha=$(git rev-parse HEAD 2>/dev/null || true)\n" +
		"    origin_sha=$(git rev-parse \"origin/$default\" 2>/dev/null || true)\n" +
		"    if [ -n \"$origin_sha\" ] && [ \"$head_sha\" = \"$origin_sha\" ]; then\n" +
		"      exit 0\n" +
		"    fi\n" +
		"  fi\n" +
		"  if [ -d docs ]; then\n" +
		"    logmind timeline --write docs/timeline.md >/dev/null 2>&1 || true\n" +
		"    logmind file-structure --write docs/file-structure.md >/dev/null 2>&1 || true\n" +
		"  fi\n" +
		"fi\n"
}

// BuildPostRewriteBody returns the canonical post-rewrite hook body
// for the currently-running logmind binary. See BuildPostMergeBody
// for the byte-identical-vs-Python contract.
//
// v2.0.0 B6 adoption gate: same contract as BuildPostMergeBody — the
// non-default-branch skip fires ONLY when THIS repo declared
// `derived_docs: {mode: integration-point}` (see
// derivedDocsIntegrationPointGrep). "driver" (the default) regenerates
// after every rebase/amend regardless of branch, the pre-v2.0.0 behavior.
func BuildPostRewriteBody() string {
	return "#!/bin/sh\n" +
		"# logmind post-rewrite hook\n" +
		HookVersionPrefix + hookVersion() + "\n" +
		"# Installed by `logmind init` (v0.5.11+). Companion to the post-merge\n" +
		"# hook. Fires after `git rebase` or `git commit --amend` and\n" +
		"# regenerates derived docs from the FULL post-rewrite working tree.\n" +
		"#\n" +
		"# Why: the merge driver in .gitattributes only fires when a merge\n" +
		"# produces conflicts on the derived files. A clean rebase rewrites\n" +
		"# multiple commits without ever invoking the driver — leaving\n" +
		"# docs/timeline.md and docs/file-structure.md stale relative to the\n" +
		"# replayed `docs/decisions-branches/<branch>.md` entries. This hook\n" +
		"# sweeps the final state once and stages the regen for inclusion in\n" +
		"# the user's next commit / amend cycle.\n" +
		"#\n" +
		"# Git invokes post-rewrite with $1 = \"rebase\" or \"amend\"; the regen\n" +
		"# behaviour is identical in both, so we don't branch on $1.\n" +
		"#\n" +
		"# v0.6.10: hook-version marker embedded above for doctor drift detection.\n" +
		"\n" +
		"if command -v logmind >/dev/null 2>&1 && [ -f .logmind/config.yml ]; then\n" +
		"  current=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)\n" +
		"  default=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null || true)\n" +
		"  default=${default#origin/}\n" +
		"  [ -z \"$default\" ] && default=main\n" +
		"  # v2.0.0 B6: only skip regen on a non-default branch when THIS repo\n" +
		"  # declared derived_docs.mode: integration-point (invariant: branches\n" +
		"  # never edit the derived docs under that mode). A repo that hasn't\n" +
		"  # adopted (mode unset or \"driver\", the default) falls through to the\n" +
		"  # regen below regardless of branch — the pre-v2.0.0 behavior.\n" +
		"  if [ -n \"$current\" ] && [ \"$current\" != \"$default\" ]; then\n" +
		"    if " + derivedDocsIntegrationPointGrep + "; then\n" +
		"      exit 0\n" +
		"    fi\n" +
		"  fi\n" +
		"  if [ -n \"$current\" ] && [ -d docs ]; then\n" +
		"    logmind timeline --write docs/timeline.md >/dev/null 2>&1 || true\n" +
		"    logmind file-structure --write docs/file-structure.md >/dev/null 2>&1 || true\n" +
		"    git add docs/timeline.md docs/file-structure.md 2>/dev/null || true\n" +
		"  fi\n" +
		"fi\n"
}

// BuildCommitMsgBody returns the canonical commit-msg hook body for the
// v2.0.0+ enforcement contract: Layer 2 of logmind's two-layer
// commit-enforcement design (see internal/claudehook for Layer 1, the
// Claude Code harness's PreToolUse guard, and internal/guardcommit for
// the shared decision engine both layers call). The hook body itself
// carries no decision logic — it just locates the commit message file
// and delegates to `logmind guard-commit --layer git-hook`, exiting with
// whatever code that command returns.
//
// v0.6.16 → v2.0.0 change: this hook used to be warn-only (surfaced a
// `[skip-logmind]` notice to stderr but always exited 0, never blocking
// the commit). It now BLOCKS a substantive commit that bypasses
// `logmind log`, unless git.enforce_commits:false or a guardcommit
// carve-out applies ([skip-logmind], LOGMIND_ALLOW_GIT_COMMIT=1, a
// staged decision file, under-threshold, or a rebase/merge/cherry-pick
// in progress — see internal/guardcommit's package doc for the full
// list). Because the hook-version marker below is embedded via
// hookVersion(), every repo's existing v0.6.16-era warn-only hook
// auto-upgrades to this enforcing body the next time `logmind init`
// (refresh mode) or `logmind doctor --fix` runs — installHook's
// "ours + body differs → overwrite" path needs no new install wiring to
// carry out this upgrade.
//
// `command -v logmind` (unlike the harness layer's canonical command,
// which deliberately OMITS that guard for cross-platform reasons — see
// internal/claudehook.CanonicalCommand) IS correct here: git hooks run
// under POSIX sh on every platform git itself supports, so `command -v`
// is always available. A missing binary fails open (exit 0) — logmind
// not being installed should never block a commit.
//
// Stale-binary hardening (CTO design amendment, post-PR2): this body used
// to do `logmind guard-commit --layer git-hook --msg-file "$MSG_FILE"; exit $?`
// — i.e. relay ANY nonzero exit from `logmind` straight into the hook's own
// exit code. That's a footgun: a STALE-but-present logmind on PATH (an old
// 1.x Cobra build that doesn't know `guard-commit` yet, or the frozen
// Python v0.6.16 CLI) exits nonzero — 1 for an unrecognized Cobra
// subcommand, 2 for argparse's unknown-subcommand error — for a reason that
// has NOTHING to do with guard-commit's actual allow/block decision. Under
// the old `exit $?` relay, that stale exit code would abort EVERY commit on
// that machine, including `logmind log`'s own internal commit — bricking
// the very tool meant to unblock you. Now the hook checks for EXACTLY 65
// (guardcommit's distinctive EX_DATAERR block signal — see
// internal/cli/guard_commit.go's guardCommitGitHook) before aborting; any
// other nonzero rc — stale binary, crash, usage error, anything we didn't
// anticipate — is NOT our block signal and falls through to the fail-open
// `exit 0` at the bottom. The corresponding `logmind guard-commit` process
// itself never calls os.Exit with anything else meaningful here, so this is
// safe: a current binary's block is ALWAYS exactly 65, never some other
// nonzero value that would now be silently ignored.
//
// Hang-guard (issue #213): the `logmind guard-commit` call runs under a
// POSIX-portable deadline (background + `sleep N; kill` watchdog + `wait`)
// so a wedged binary can never stall `git commit`. A timeout kill yields a
// signal exit >128 — never 65 — so it fails open through the same rc!=65
// fall-through as a stale binary. The block path (rc==65 → exit 1) and the
// escape hatches are untouched; only the never-HANG guarantee is added.
func BuildCommitMsgBody() string {
	return "#!/bin/sh\n" +
		"# logmind commit-msg hook\n" +
		HookVersionPrefix + hookVersion() + "\n" +
		"# Installed by `logmind init`. Layer 2 of the v2.0.0+ commit-enforcement\n" +
		"# design (see internal/claudehook for Layer 1, the Claude Code harness's\n" +
		"# PreToolUse guard). Delegates the enforce/allow decision entirely to\n" +
		"# `logmind guard-commit --layer git-hook` — see internal/guardcommit for\n" +
		"# the carve-outs (git.enforce_commits:false, [skip-logmind],\n" +
		"# LOGMIND_ALLOW_GIT_COMMIT=1, a staged decision file, under-threshold,\n" +
		"# rebase/merge/cherry-pick in progress).\n" +
		"#\n" +
		"# Fails open when logmind isn't on PATH: a missing binary should never\n" +
		"# block a commit.\n" +
		"#\n" +
		"# Stale-binary hardening: 65 is guard-commit's OWN distinctive block\n" +
		"# signal (EX_DATAERR). Any other nonzero exit — including a STALE\n" +
		"# logmind on PATH that doesn't know `guard-commit` at all (an old\n" +
		"# Cobra build's unknown-command exit 1, or the frozen Python CLI's\n" +
		"# argparse exit 2) — must NOT abort the commit, or a stale binary\n" +
		"# would brick every commit on this machine, including `logmind log`'s\n" +
		"# own internal one.\n" +
		"#\n" +
		"# Hang-guard (issue #213): a wedged/hung logmind must never stall\n" +
		"# `git commit`. timeout(1) isn't on macOS by default, so we run\n" +
		"# guard-commit in the background under a watchdog: a `sleep N; kill`\n" +
		"# subshell terminates it after the deadline, we `wait` for its real\n" +
		"# exit code, then reap the watchdog. On a timeout the watchdog kill\n" +
		"# leaves a signal exit (>128, never 65), so the rc!=65 fall-through\n" +
		"# below FAILS OPEN (exit 0) — the goal is to never HANG, not to block.\n" +
		"# The watchdog's fds go to /dev/null so its (possibly orphaned) `sleep`\n" +
		"# can't hold this hook's stdout/stderr open — otherwise a tool that\n" +
		"# CAPTURES git's output (Claude Code's Bash tool, CI) would block\n" +
		"# reading the pipe until the sleep expired, even on a fast commit.\n" +
		"\n" +
		"MSG_FILE=\"$1\"\n" +
		"if [ -z \"$MSG_FILE\" ] || [ ! -f \"$MSG_FILE\" ]; then\n" +
		"    exit 0\n" +
		"fi\n" +
		"if command -v logmind >/dev/null 2>&1; then\n" +
		"    logmind guard-commit --layer git-hook --msg-file \"$MSG_FILE\" &\n" +
		"    __lm_pid=$!\n" +
		"    ( sleep 10; kill \"$__lm_pid\" 2>/dev/null ) >/dev/null 2>&1 &\n" +
		"    __lm_watcher=$!\n" +
		"    wait \"$__lm_pid\" 2>/dev/null\n" +
		"    rc=$?\n" +
		"    kill \"$__lm_watcher\" 2>/dev/null\n" +
		"    wait \"$__lm_watcher\" 2>/dev/null\n" +
		"    if [ \"$rc\" -eq 65 ]; then\n" +
		"        exit 1\n" +
		"    fi\n" +
		"fi\n" +
		"exit 0\n"
}

// BuildPreCommitBody returns the canonical pre-commit hook body — L2a of
// the v2.0.0 derived-docs pin-preservation design (see internal/cli/derived.go
// for the invariant, and internal/cli/log.go's commitDecision for L1, the
// same restore already run by `logmind log` itself).
//
// The gap L2a closes: L1 only fires inside `logmind log`. A raw `git commit
// -am ...` (or any commit that skips `logmind log` entirely) stages
// whatever docs/timeline.md / docs/file-structure.md currently look like in
// the working tree — including a stale/dirty copy left behind by something
// like `logmind warp` deliberately writing the default branch's newer copy
// into the working tree for review. `guard-commit`'s CarveOutUnderThreshold
// lets such a commit through (a derived-doc-only change is small), so
// nothing else in the enforcement stack catches it. This hook does: on a
// non-default branch, it restores both files to their committed (HEAD)
// content — in both the index and the working tree — before the commit is
// built, so the raw commit can never carry a derived-doc change off the
// zero-conflict invariant.
//
// Restore target is HEAD, NOT the merge-base with the default branch (v2.0.0
// 4b-ter reversal of the short-lived 4b-bis "repair-path fix" — mirrors
// commitDecision's L1 in internal/cli/log.go). 4b-bis pointed this restore
// at merge-base(origin/<default>, HEAD) so an already-diverged branch could
// self-heal, but that target depends on refs/remotes/origin/<default> being
// CURRENT, and nothing on the commit path ever runs `git fetch` to refresh
// it — this hook runs as a pure POSIX-sh script with no logmind binary
// required, so it has even less business trusting a remote-tracking ref
// that may be arbitrarily stale. A stale ref meant this restore could
// silently commit an OLDER snapshot than the branch's true merge-base,
// actively writing WRONG bytes and typically FAILING the very CI gate this
// hook exists to satisfy. HEAD has no such dependency: it's always
// correct offline, using only already-committed local state. The tradeoff,
// accepted deliberately: this hook can no longer repair a branch whose HEAD
// already carries a diverged copy (an old binary's local regen, or a hand
// edit) — it re-affirms whatever's there instead. That's fine: this hook's
// job is narrower than "repair" — it only keeps an ALREADY-clean branch
// clean, stopping divergence from ARRIVING via a stray hook regen or a raw
// `git commit -a`. Repairing an already-diverged branch needs a trustworthy,
// freshly-fetched origin/<default> to compute a correct merge-base against
// — `logmind warp` is the one surface that fetches first, so that's where
// the repair now lives (see runWarp in internal/cli/warp.go).
//
// v2.0.0 4b-quater — deliberately does NOT gain the "skip an already-staged
// path" filter that commitDecision's L1 (internal/cli/log.go) and
// guardCommitHarness's L2b (internal/cli/guard_commit.go) gained in that
// change. Those two surfaces run their restore BEFORE their OWN staging
// step (L1: before `git add -A`/`git add <target>`; L2b: before the
// pending Bash command — which is what would stage anything — even runs),
// so at THEIR check time "the index already differs from HEAD for this
// path" can only mean a prior, SEPARATE, deliberate action — chiefly
// `logmind warp`'s merge-base repair. This hook has no such guarantee: it
// fires from `git commit` itself, which runs AFTER whatever staged the
// index for THIS commit — very often `git add -A` or `git commit -a`
// sweeping up an ACCIDENTALLY dirtied derived doc right along with
// everything else a raw commit carries. At hook-fire time here, "staged"
// is simply the normal state of anything about to be committed, not a
// reliable signal that a human (or `warp`) deliberately meant to keep it.
// Skipping a staged path here would silently reopen exactly the hole this
// hook exists to close: see
// TestPreCommitHook_EndToEnd_RawGitCommitDoesNotCarryDirtyDerivedDoc
// (hooks_test.go), which dirties docs/timeline.md and commits via `git
// commit -am` — `-a` stages the dirt automatically, before this hook ever
// runs — and asserts the commit does NOT carry it; that test would start
// failing the instant this hook skipped staged paths.
//
// Residual gap, accepted, and BROADER than just a raw `git commit`: this
// hook is a REAL `.git/hooks/pre-commit` script, so it fires for EVERY
// `git commit` in this repo — including the one `logmind log` itself runs
// internally (gitcli.Commit shells out to a literal `git commit -m
// <message>`, no `--no-verify`). That means a warp-then-`logmind log`
// sequence is NOT fully protected by L1 alone whenever this hook happens
// to be installed (which `logmind init`/`doctor --fix` do automatically
// the moment a repo adopts `derived_docs: {mode: integration-point}` — see
// init.go): L1 runs first and correctly leaves the warp-staged repair
// alone, `git add -A` stages the rest, and then `logmind log`'s own `git
// commit` invocation triggers THIS hook, which restores unconditionally to
// HEAD and undoes what L1 just preserved — same bug, reached through the
// installed hook instead of a manual bypass. A plain, unqualified "raw
// `git commit` bypassing `logmind log`" is the NARROWER case (no
// installed hook needed to trigger it, since the user's own `git commit`
// IS the trigger) but not the only one. Both are currently unresolved:
// closing them needs a signal from `logmind log`'s own commit invocation
// telling this hook "L1 already decided, stand down" (e.g. an environment
// variable set only around that specific `git commit` call) — deliberately
// NOT implemented here, since it is a distinct, cross-cutting change
// (touching gitcli.Commit / commitDecision AND this hook body, with its
// own golden-file + test surface) beyond this fix's scope. L3 (the CI
// check-derived-docs gate) is the backstop for both variants of this gap,
// same as for any other already-diverged branch. See
// TestPreCommitHook_EndToEnd_DoesNotRepairAlreadyDivergedBranch, which
// pins the raw-`git commit` half of this residual behavior.
//
// MUST NEVER block a commit. Unlike the commit-msg hook (Layer 2 of commit
// enforcement, which can legitimately reject a commit), this hook exists
// solely to keep two files pinned — a purely mechanical, lossless operation
// (the docs regenerate deterministically from the committed decision files,
// which travel with the branch and never conflict). It always exits 0,
// whether or not `.logmind/config.yml` is present, whether or not the
// restore itself succeeds, and regardless of branch-detection failures.
//
// Deliberately pure git — NO `logmind` binary required on PATH, unlike the
// commit-msg hook's delegation to `logmind guard-commit`. The restore is
// simple enough (`git checkout HEAD -- <path>`) to inline directly, which
// keeps this guardrail working in the two places it matters most: a fresh
// clone or CI runner where logmind might not be installed yet, and a repo
// where the on-PATH logmind is stale or broken.
//
// Branch detection mirrors BuildPostMergeBody / BuildPostRewriteBody
// exactly: `git rev-parse --abbrev-ref HEAD` for the current branch,
// `git symbolic-ref --short refs/remotes/origin/HEAD` (stripped of its
// `origin/` prefix) for the default branch, falling back to "main" when the
// remote symbolic ref can't be resolved (no origin, detached HEAD, etc.).
func BuildPreCommitBody() string {
	return "#!/bin/sh\n" +
		"# logmind pre-commit hook\n" +
		HookVersionPrefix + hookVersion() + "\n" +
		"# Installed by `logmind init` (v2.0.0+). L2a of the derived-docs-on-main\n" +
		"# pin-preservation design: docs/timeline.md and docs/file-structure.md are\n" +
		"# purely-derived, main-only artifacts (see internal/cli/derived.go). A raw\n" +
		"# `git commit` — bypassing `logmind log`, whose own commitDecision restore\n" +
		"# is Layer 1 — could otherwise sweep a dirtied copy of either file into the\n" +
		"# commit on a non-default branch, e.g. after `logmind warp` pulls in the\n" +
		"# default branch's newer copy for review. This hook restores both to their\n" +
		"# committed (HEAD) content, in both the index and the working tree, right\n" +
		"# before the commit is built.\n" +
		"#\n" +
		"# This hook MUST NEVER block a commit — it always exits 0. The restore is\n" +
		"# lossless (the docs regenerate deterministically from the committed\n" +
		"# decision files) so there is nothing to protect by failing closed.\n" +
		"#\n" +
		"# Deliberately pure git — no `logmind` binary required on PATH. Unlike the\n" +
		"# commit-msg hook (which delegates the enforce/allow decision to `logmind\n" +
		"# guard-commit`), this restore is simple enough to inline directly, which\n" +
		"# keeps it working in a fresh clone or CI runner before logmind is\n" +
		"# installed, or when the on-PATH binary is stale or broken.\n" +
		"\n" +
		"if [ -f .logmind/config.yml ]; then\n" +
		"  current=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)\n" +
		"  default=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null || true)\n" +
		"  # --short on a remote symbolic-ref leaves the `origin/` prefix\n" +
		"  # in place; strip it so the bare-name comparison below works.\n" +
		"  default=${default#origin/}\n" +
		"  [ -z \"$default\" ] && default=main\n" +
		"  if [ -n \"$current\" ] && [ \"$current\" != \"$default\" ]; then\n" +
		"    git checkout HEAD -- docs/timeline.md docs/file-structure.md >/dev/null 2>&1 || true\n" +
		"  fi\n" +
		"fi\n" +
		"exit 0\n"
}

// InstallPostMerge writes `.git/hooks/post-merge` to the current
// binary's canonical body. Returns (true, nil) if the file was
// created or rewritten, (false, nil) if a logmind-marked hook was
// already present at the exact byte content (idempotent no-op).
//
// Refuses to overwrite a foreign hook — returns (false, nil) and
// lets logmind doctor flag the state instead. Mirrors the Python
// install_post_merge_hook semantics line-for-line.
func InstallPostMerge(repoRoot string) (bool, error) {
	return installHook(
		filepath.Join(repoRoot, ".git", "hooks", "post-merge"),
		BuildPostMergeBody(),
		PostMergeMarker,
	)
}

// InstallPostRewrite writes `.git/hooks/post-rewrite`. See
// InstallPostMerge for the contract.
func InstallPostRewrite(repoRoot string) (bool, error) {
	return installHook(
		filepath.Join(repoRoot, ".git", "hooks", "post-rewrite"),
		BuildPostRewriteBody(),
		PostRewriteMarker,
	)
}

// InstallCommitMsg writes `.git/hooks/commit-msg`. See InstallPostMerge
// for the contract. v0.6.16+.
func InstallCommitMsg(repoRoot string) (bool, error) {
	return installHook(
		filepath.Join(repoRoot, ".git", "hooks", "commit-msg"),
		BuildCommitMsgBody(),
		CommitMsgMarker,
	)
}

// InstallPreCommit writes `.git/hooks/pre-commit`. See InstallPostMerge for
// the shared contract — including the conservative-interop behavior that
// matters most here: a pre-commit hook that already exists WITHOUT
// PreCommitMarker (a hand-written hook, OR the separate, opt-in
// `logmind check-decisions` body `logmind install-hook` writes — see
// PreCommitMarker's doc comment) is left completely alone. v2.0.0+.
func InstallPreCommit(repoRoot string) (bool, error) {
	return installHook(
		filepath.Join(repoRoot, ".git", "hooks", "pre-commit"),
		BuildPreCommitBody(),
		PreCommitMarker,
	)
}

// installHook is the shared write path. Returns true iff the file
// on disk now differs from what it was before this call.
//
// Behaviour matrix:
//
//	hooks dir missing  → (false, nil) — caller is not in a git repo
//	                                    or the hooks dir was nuked
//	hook absent        → write body + chmod 0755, return (true, nil)
//	hook present, ours, body matches  → (false, nil) — no-op
//	hook present, ours, body differs  → overwrite + chmod, (true, nil)
//	hook present, foreign             → (false, nil) — leave alone
//
// Read errors on the existing hook are treated as "present but
// unrecognised" — we don't trample, matching the Python errors=
// "ignore" pattern.
func installHook(hookPath, body, marker string) (bool, error) {
	hooksDir := filepath.Dir(hookPath)
	if _, err := os.Stat(hooksDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	existing, err := os.ReadFile(hookPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Permission denied or some other read-side error: treat as
		// "present, unrecognised" — leave alone.
		return false, nil
	}
	if err == nil {
		// File exists — check whether it's ours.
		if !containsBytes(existing, marker) {
			return false, nil // foreign hook; don't trample
		}
		if string(existing) == body {
			return false, nil // already current
		}
	}
	// Write atomically (temp sibling + rename): a bare os.WriteFile here
	// would O_TRUNC an EXISTING hook before writing the new body, so a
	// crash mid-write could leave a truncated/corrupt git hook behind.
	// atomicio.WriteFile also chmods the temp file before the rename, so
	// the executable bit lands correctly whether hookPath is new or being
	// updated — no separate re-chmod needed.
	if err := atomicio.WriteFile(hookPath, []byte(body), 0o755); err != nil {
		return false, err
	}
	return true, nil
}

// ExtractVersion reads the `# logmind-hook-version: X.Y.Z` marker
// from hookPath. Returns ("", false) if the file is missing, the
// marker is absent (pre-v0.6.10 hook), or any read error occurs.
// Mirrors src/logmind/core/gitattributes.extract_hook_version.
//
// Note: this is the SINGLE source of truth for version-marker
// extraction across the Go codebase. internal/doctor calls it
// directly (see probeHook) rather than reimplementing the parsing.
func ExtractVersion(hookPath string) (string, bool) {
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return "", false
	}
	return extractMarker(data, HookVersionPrefix)
}

// containsBytes is bytes.Contains with a string needle. Kept inline
// to avoid pulling in the bytes import for a single call.
func containsBytes(haystack []byte, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == needle {
			return true
		}
	}
	return false
}

// extractMarker walks data line-by-line looking for one that starts
// with prefix. Returns the suffix (whitespace-trimmed) and true on
// match. Uses a manual scan rather than regexp to keep the hooks
// package import-graph small.
func extractMarker(data []byte, prefix string) (string, bool) {
	start := 0
	for i := 0; i <= len(data); i++ {
		if i == len(data) || data[i] == '\n' {
			line := string(data[start:i])
			if len(line) >= len(prefix) && line[:len(prefix)] == prefix {
				return trimSpace(line[len(prefix):]), true
			}
			start = i + 1
		}
	}
	return "", false
}

// trimSpace is a tiny strings.TrimSpace alternative that handles just
// the whitespace characters bash's `$(...)` expansion can leave
// behind. Avoids the strings dependency from the hot path.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && isASCIISpace(s[start]) {
		start++
	}
	for end > start && isASCIISpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}
