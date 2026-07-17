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

// hookVersion returns the version string embedded in each hook body.
// Reads from internal/version.Version — kept as a function (not a
// package-level var) so tests can override it via a build-tagged
// alternative file without touching the production code.
func hookVersion() string {
	return version.Version
}

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
// server-side reconciler is the advisory regen-timeline.yml workflow (PR
// #159). We deliberately add NO push-to-default trigger here: it would
// reintroduce the GITHUB_TOKEN-stranding + self-trigger loop the advisory
// model exists to avoid. The TestPostMergeBody_RollupInvariants guard pins
// "regenerates the timeline, never pushes" even across a golden regen.
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
		"  if [ -d docs ]; then\n" +
		"    logmind timeline --write docs/timeline.md >/dev/null 2>&1 || true\n" +
		"    logmind file-structure --write docs/file-structure.md >/dev/null 2>&1 || true\n" +
		"    # Stage the regens if they changed anything; user can `git commit --amend`\n" +
		"    # or include in their next commit.\n" +
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
		"\n" +
		"MSG_FILE=\"$1\"\n" +
		"if [ -z \"$MSG_FILE\" ] || [ ! -f \"$MSG_FILE\" ]; then\n" +
		"    exit 0\n" +
		"fi\n" +
		"if command -v logmind >/dev/null 2>&1; then\n" +
		"    logmind guard-commit --layer git-hook --msg-file \"$MSG_FILE\"\n" +
		"    rc=$?\n" +
		"    if [ \"$rc\" -eq 65 ]; then\n" +
		"        exit 1\n" +
		"    fi\n" +
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
// extraction across the Go codebase. Doctor (later wave) will call
// it directly rather than reimplementing the regex.
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
