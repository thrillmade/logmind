"""Idempotent management of a marker-bracketed block in ``.gitattributes``.

The block registers custom merge drivers for logmind's derived files
(docs/timeline.md, docs/file-structure.md). Without them, parallel PRs
that both ran ``logmind log`` produce textual merge conflicts when one
rebases onto the other — git can't tell that the right resolution is
"regenerate from the per-branch decision files" rather than "interleave
the diffs textually."

The driver definitions themselves live in ``.git/config`` (per-clone,
not committed — git refuses to invoke a merge driver that wasn't
explicitly configured locally as a security guard). ``logmind init``
runs the matching ``git config`` calls every time it runs; see
``configure_merge_drivers`` in this module.
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import Iterable, Optional

LOGMIND_GITATTRIBUTES_START = "# >>> logmind >>>"
LOGMIND_GITATTRIBUTES_END = "# <<< logmind <<<"

# v0.6.10 — embed a version marker in every installed hook so doctor can
# detect drift between the CLI binary that wrote the hook and the binary
# currently running. The tokenomics agent's 2026-06-01 bug report
# (post-merge hook still stages after v0.6.9 propagation) had the root
# cause: their local CLI binary was v0.3.4, so every `logmind log`
# overwrote the hook with v0.3.4's body even though the workflow pin
# said v0.6.9. Doctor needs to read THIS marker (not just check
# presence) to surface the drift loudly.
HOOK_VERSION_PREFIX = "# logmind-hook-version: "
_HOOK_VERSION_RE = re.compile(r"^# logmind-hook-version:\s*(\S+)\s*$", re.MULTILINE)


def _current_logmind_version() -> str:
    """Return the version string of the currently-running logmind package.

    Imported lazily to avoid circular imports — `gitattributes` is loaded
    during `logmind` package init, before `__version__` is necessarily
    bound.
    """
    try:
        from logmind import __version__
        return __version__
    except (ImportError, AttributeError):
        return "unknown"

# Lines that go inside the managed block. Each registers a merge driver
# for one of logmind's derived files. The driver names match what
# configure_merge_drivers() sets in .git/config.
DEFAULT_GITATTRIBUTES_LINES = (
    "docs/timeline.md          merge=logmind-timeline",
    "docs/file-structure.md    merge=logmind-file-structure",
)


def ensure_block(
    path: Path,
    lines: Iterable[str] = DEFAULT_GITATTRIBUTES_LINES,
) -> bool:
    """Ensure ``path`` (a .gitattributes) contains the logmind-managed block.

    Returns True if the file was created or modified, False if the block
    was already present (manual edits inside it preserved).
    """
    existing = path.read_text(encoding="utf-8") if path.exists() else ""
    if LOGMIND_GITATTRIBUTES_START in existing:
        return False

    block_parts = [LOGMIND_GITATTRIBUTES_START]
    for line in lines:
        block_parts.append(line)
    block_parts.append(LOGMIND_GITATTRIBUTES_END)
    block = "\n".join(block_parts) + "\n"

    if existing and not existing.endswith("\n"):
        existing += "\n"
    if existing and not existing.endswith("\n\n"):
        existing += "\n"

    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(existing + block, encoding="utf-8")
    return True


def has_block(path: Path) -> bool:
    if not path.exists():
        return False
    return LOGMIND_GITATTRIBUTES_START in path.read_text(encoding="utf-8")


# ---------------------------------------------------------------------------
# Per-clone git config — the driver definitions themselves live in .git/config
# ---------------------------------------------------------------------------


# Each entry is (config_key, value). Set every time `logmind init` runs;
# git config is idempotent (no-op when the value already matches).
MERGE_DRIVER_CONFIG = (
    ("merge.logmind-timeline.driver", "logmind timeline --write %A"),
    ("merge.logmind-timeline.name", "Regenerate logmind timeline"),
    ("merge.logmind-file-structure.driver", "logmind file-structure --write %A"),
    ("merge.logmind-file-structure.name", "Regenerate logmind file structure"),
)


def configure_merge_drivers(repo_root: Path) -> bool:
    """Set the per-clone git config keys that define the logmind merge drivers.

    Returns True if any config key was changed, False if all keys already
    held the expected value. Silently no-ops outside a git repo.
    """
    import subprocess

    if not (repo_root / ".git").exists():
        # Not a git checkout; caller decides whether that's an error.
        return False

    changed = False
    for key, value in MERGE_DRIVER_CONFIG:
        try:
            current = subprocess.run(
                ["git", "-C", str(repo_root), "config", "--get", key],
                capture_output=True,
                text=True,
                check=False,
            )
            if current.returncode == 0 and current.stdout.strip() == value:
                continue
            subprocess.run(
                ["git", "-C", str(repo_root), "config", key, value],
                capture_output=True,
                text=True,
                check=True,
            )
            changed = True
        except (subprocess.CalledProcessError, FileNotFoundError, OSError):
            # Don't break logmind init for a git config hiccup — the driver
            # is a "merge time" optimization, not a "commit time" requirement.
            # logmind doctor surfaces the drift on the next invocation.
            continue
    return changed


# ---------------------------------------------------------------------------
# post-merge hook — re-regenerates derived files with the FULL post-merge
# tree (the merge driver fires before all non-conflicted files are in the
# working tree, so its output may miss decisions from the merged-in branch).
# ---------------------------------------------------------------------------

_POST_MERGE_HOOK_MARKER = "# logmind post-merge hook"


def _build_post_merge_hook_body() -> str:
    """Build the canonical post-merge hook body for the current binary
    version. The embedded ``# logmind-hook-version:`` line lets ``logmind
    doctor`` detect drift between the CLI binary that wrote this hook
    and the binary currently running.
    """
    return (
        "#!/bin/sh\n"
        "# logmind post-merge hook\n"
        f"{HOOK_VERSION_PREFIX}{_current_logmind_version()}\n"
        "# Installed by `logmind init` (v0.3.0+). After every merge, regenerate\n"
        "# derived docs from the FULL post-merge working tree. Belt + suspenders\n"
        "# with the merge driver in .gitattributes — the driver fires per-file\n"
        "# during conflict resolution, before sibling non-conflicted files (e.g.\n"
        "# the merged-in branch's docs/decisions-branches/<branch>.md) are\n"
        "# checked out. This hook runs once at the end and sweeps any incomplete\n"
        "# regenerations.\n"
        "#\n"
        "# v0.6.7 bug fix: regenerate but do NOT git add. Previously the hook\n"
        "# auto-staged docs/timeline.md + docs/file-structure.md, but the\n"
        "# staged-but-uncommitted files then blocked `git checkout main` on\n"
        "# every PR cycle (post-merge fires from `git pull --rebase` after a\n"
        "# squash merge — there's no commit being constructed, so staging was\n"
        "# wrong). Leaving them as unstaged modifications lets the next\n"
        "# `logmind log` pick them up cleanly without blocking branch switches.\n"
        "#\n"
        "# v0.6.10: the line above embeds the binary version that wrote this hook,\n"
        "# so doctor reports stale-hook drift loudly when the user's local CLI\n"
        "# binary is stale relative to the workflow's installed version.\n"
        "#\n"
        "# v0.6.13 / issue #112: skip regen entirely when the current branch is\n"
        "# orphaned — i.e., we're on a feature branch that was just squash-merged\n"
        "# on origin and the remote branch has been deleted. The regen would\n"
        "# produce content that conflicts with main's just-merged state, and the\n"
        "# resulting unstaged modifications block `gh pr merge --delete-branch`'s\n"
        "# follow-up `git checkout main`. Detection without network calls:\n"
        "# `git rev-parse --abbrev-ref @{u}` returns nonzero / empty when the\n"
        "# upstream is gone after `git fetch --prune`. Detection failure → fall\n"
        "# through to today's behavior (regen + leave unstaged); never block on\n"
        "# detection failure.\n"
        "\n"
        "if command -v logmind >/dev/null 2>&1 && [ -f .logmind/config.yml ]; then\n"
        "  # Orphan-branch check: if our upstream tracking ref no longer exists,\n"
        "  # we were just merged-and-deleted; skip regen.\n"
        "  upstream=$(git rev-parse --abbrev-ref @{u} 2>/dev/null || true)\n"
        "  if [ -n \"$upstream\" ]; then\n"
        "    if ! git rev-parse --verify --quiet \"refs/remotes/$upstream\" >/dev/null 2>&1; then\n"
        "      # Upstream config says <upstream> but no such remote-tracking ref\n"
        "      # exists (typical after `git fetch --prune` removes the merged\n"
        "      # branch). Skip regen — main will regen correctly after checkout.\n"
        "      exit 0\n"
        "    fi\n"
        "  fi\n"
        "  # v0.6.16 (replaces v0.6.15's blanket default-branch skip): on the\n"
        "  # default branch, only skip when HEAD already matches origin/<default>\n"
        "  # — i.e., a fast-forward pull-up from server where regen-timeline.yml\n"
        "  # has already produced the authoritative timeline. For local merges\n"
        "  # that introduced new commits not yet on origin (the multi-branch\n"
        "  # self-heal case: `git merge feat-branch` on main), regen MUST fire\n"
        "  # so the working tree reflects the merged-in decision files. v0.6.15's\n"
        "  # blanket skip dropped Decision-B-style entries from local main; the\n"
        "  # multi-branch self-heal regression test in tests/test_merge_driver.py\n"
        "  # catches this. Surfaces the bug PRE-merge instead of in a downstream\n"
        "  # check-derived-docs failure weeks later.\n"
        "  current=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)\n"
        "  default=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null || true)\n"
        "  # --short on a remote symbolic-ref leaves the `origin/` prefix\n"
        "  # in place; strip it so the bare-name comparison below works.\n"
        "  default=${default#origin/}\n"
        "  [ -z \"$default\" ] && default=main\n"
        "  if [ -n \"$current\" ] && [ \"$current\" = \"$default\" ]; then\n"
        "    head_sha=$(git rev-parse HEAD 2>/dev/null || true)\n"
        "    origin_sha=$(git rev-parse \"origin/$default\" 2>/dev/null || true)\n"
        "    if [ -n \"$origin_sha\" ] && [ \"$head_sha\" = \"$origin_sha\" ]; then\n"
        "      exit 0\n"
        "    fi\n"
        "  fi\n"
        "  if [ -d docs ]; then\n"
        "    logmind timeline --write docs/timeline.md >/dev/null 2>&1 || true\n"
        "    logmind file-structure --write docs/file-structure.md >/dev/null 2>&1 || true\n"
        "  fi\n"
        "fi\n"
    )


def install_post_merge_hook(repo_root: Path) -> bool:
    """Write `.git/hooks/post-merge` to re-regenerate derived files after
    every merge. Returns True if the hook was created or updated, False
    if logmind's version was already present at the CURRENT binary's
    version.

    Refuses to overwrite a non-logmind hook (someone may have custom
    hook content; we leave it alone and let `logmind doctor` flag the
    state instead).
    """
    hooks_dir = repo_root / ".git" / "hooks"
    if not hooks_dir.exists():
        return False
    hook = hooks_dir / "post-merge"
    body = _build_post_merge_hook_body()
    if hook.exists():
        existing = hook.read_text(encoding="utf-8", errors="ignore")
        if _POST_MERGE_HOOK_MARKER in existing:
            # Our hook (or some prior-version of it) is present. Rewrite
            # to the current binary's canonical body. The idempotency
            # contract: if the body byte-matches, no-op; otherwise
            # overwrite — this handles upgrades cleanly (v0.3.x -> v0.6.10
            # automatically refreshes the hook on the next `logmind log`).
            if existing == body:
                return False
            hook.write_text(body, encoding="utf-8")
            hook.chmod(0o755)
            return True
        # Foreign hook — don't trample.
        return False
    hook.write_text(body, encoding="utf-8")
    hook.chmod(0o755)
    return True


def post_merge_hook_installed(repo_root: Path) -> bool:
    hook = repo_root / ".git" / "hooks" / "post-merge"
    if not hook.exists():
        return False
    try:
        return _POST_MERGE_HOOK_MARKER in hook.read_text(encoding="utf-8", errors="ignore")
    except OSError:
        return False


def extract_hook_version(hook_path: Path) -> Optional[str]:
    """Read ``hook_path`` and extract the embedded ``# logmind-hook-version: …``
    marker. Returns the version string, ``None`` if no marker is present
    (pre-v0.6.10 hooks), or ``None`` if the file is missing / unreadable.

    Used by ``logmind doctor`` to surface drift between the binary that
    wrote the hook and the binary currently running — the root cause of
    the tokenomics agent's 2026-06-01 post-merge bug recurrence.
    """
    if not hook_path.exists():
        return None
    try:
        content = hook_path.read_text(encoding="utf-8", errors="ignore")
    except OSError:
        return None
    match = _HOOK_VERSION_RE.search(content)
    return match.group(1) if match else None


def installed_post_merge_hook_version(repo_root: Path) -> Optional[str]:
    """Return the version marker embedded in ``.git/hooks/post-merge``
    when it exists + is a logmind hook + has a v0.6.10+ marker. Returns
    ``None`` for pre-v0.6.10 hooks (markerless), missing hooks, or
    foreign hooks.
    """
    hook = repo_root / ".git" / "hooks" / "post-merge"
    if not hook.exists():
        return None
    content = hook.read_text(encoding="utf-8", errors="ignore")
    if _POST_MERGE_HOOK_MARKER not in content:
        return None  # Foreign hook; not ours.
    match = _HOOK_VERSION_RE.search(content)
    return match.group(1) if match else None


def installed_post_rewrite_hook_version(repo_root: Path) -> Optional[str]:
    """Same as :func:`installed_post_merge_hook_version` but for the
    post-rewrite hook.
    """
    hook = repo_root / ".git" / "hooks" / "post-rewrite"
    if not hook.exists():
        return None
    content = hook.read_text(encoding="utf-8", errors="ignore")
    if _POST_REWRITE_HOOK_MARKER not in content:
        return None
    match = _HOOK_VERSION_RE.search(content)
    return match.group(1) if match else None


# ---------------------------------------------------------------------------
# v0.6.16 — commit-msg hook (dogfood enforcement)
#
# The tokenomics-agent session's "why git add instead of logmind" feedback
# revealed that even with AGENTS.md guidance + skill docs, agents reflexively
# fall back to raw `git commit` on substantive code changes. The commit-msg
# hook adds a client-side check: when a commit's first line doesn't carry
# the `logmind:` prefix AND the staged diff exceeds a threshold of
# code-bearing lines, the hook prints a warning (default) or rejects the
# commit (STRICT mode via `.logmind/config.yml`: commit_msg_hook.strict).
#
# Exempt patterns (always allowed without warning):
#   - `logmind:*` — produced by `logmind log`
#   - `Revert `, `Merge ` — git-generated
#   - `Bump version`, `chore(release):` — release-bot patterns
#   - `fixup!`, `squash!` — interactive rebase intermediates
#
# Threshold: 20 staged lines across `.py .go .ts .tsx .js .jsx .sh .yaml
# .yml .toml`. Below threshold = trivial change (typo, formatting,
# dep-bump) → exit 0 silently.
# ---------------------------------------------------------------------------

_COMMIT_MSG_HOOK_MARKER = "# logmind commit-msg hook"


def _build_commit_msg_hook_body() -> str:
    """Build the canonical commit-msg hook body for the current binary
    version. v0.6.16+: embed binary version marker for doctor drift
    detection. WARN-vs-STRICT behavior is selected at runtime from
    `.logmind/config.yml` so users can flip the mode without re-installing
    the hook.
    """
    return (
        "#!/bin/sh\n"
        "# logmind commit-msg hook\n"
        f"{HOOK_VERSION_PREFIX}{_current_logmind_version()}\n"
        "# Installed by `logmind init` (v0.6.16+). Enforces dogfood discipline:\n"
        "# substantive code commits should be produced via `logmind log` so the\n"
        "# decision tree captures the reasoning, alternatives, and implications.\n"
        "# Raw `git commit` for substantive changes loses that signal.\n"
        "#\n"
        "# WARN by default. STRICT mode (reject commit) when .logmind/config.yml\n"
        "# contains `commit_msg_hook: strict: true` in a top-level key. Users\n"
        "# can bypass either mode with `git commit --no-verify` when the\n"
        "# bypass is intentional.\n"
        "#\n"
        "# Exempt patterns (no check applied):\n"
        "#   logmind:*    — written by `logmind log`\n"
        "#   Revert       — git-generated revert message\n"
        "#   Merge        — git-generated merge message\n"
        "#   Bump version — release bot\n"
        "#   chore(release): — release bot (conventional commits)\n"
        "#   fixup!, squash! — interactive rebase intermediates\n"
        "\n"
        'msg=$(head -n1 "$1" 2>/dev/null || echo "")\n'
        'case "$msg" in\n'
        "  logmind:*|\"Revert \"*|\"Merge \"*|\"Bump version\"*|\"chore(release):\"*|fixup!*|squash!*)\n"
        "    exit 0\n"
        "    ;;\n"
        "esac\n"
        "\n"
        "# Count staged additions+deletions across code-bearing extensions.\n"
        "staged=$(git diff --cached --numstat -- \\\n"
        "  '*.py' '*.go' '*.ts' '*.tsx' '*.js' '*.jsx' '*.sh' \\\n"
        "  '*.yaml' '*.yml' '*.toml' '*.rs' '*.rb' 2>/dev/null \\\n"
        "  | awk '$1 != \"-\" { s+=$1+$2 } END { print s+0 }')\n"
        "\n"
        "threshold=20\n"
        'if [ "${staged:-0}" -lt "$threshold" ] 2>/dev/null; then\n'
        "  exit 0\n"
        "fi\n"
        "\n"
        "# Best-effort grep for strict mode in .logmind/config.yml.\n"
        "# Matches a `commit_msg_hook:` block with a nested `strict: true`.\n"
        'strict=""\n'
        'if [ -f .logmind/config.yml ]; then\n'
        '  strict=$(awk \'/^commit_msg_hook:/{in_block=1; next} /^[a-zA-Z_]/ && !/^  /{in_block=0} in_block && /^  +strict:[[:space:]]*true/{print "yes"; exit}\' .logmind/config.yml 2>/dev/null || true)\n'
        "fi\n"
        "\n"
        'echo "⚠ logmind: substantive commit ($staged lines staged in code) not produced via \\`logmind log\\`." >&2\n'
        'echo "  Use:   logmind log \\"summary\\" -r \\"why\\" -a \\"alt\\" -i \\"impl\\"" >&2\n'
        'echo "  Skip:  git commit --no-verify  (intentional override)" >&2\n'
        "\n"
        'if [ "$strict" = "yes" ]; then\n'
        '  echo "  STRICT mode (commit_msg_hook.strict in .logmind/config.yml): rejecting commit." >&2\n'
        "  exit 1\n"
        "fi\n"
        "exit 0\n"
    )


def install_commit_msg_hook(repo_root: Path) -> bool:
    """Write `.git/hooks/commit-msg`. Returns True if the hook was created
    or updated, False if logmind's hook with the CURRENT body was already
    in place. Refuses to overwrite a non-logmind hook (some users have
    custom commit-msg hooks for conventional commits / linting; we don't
    trample them — doctor reports the foreign-hook state instead).
    """
    hooks_dir = repo_root / ".git" / "hooks"
    if not hooks_dir.exists():
        return False
    hook = hooks_dir / "commit-msg"
    body = _build_commit_msg_hook_body()
    if hook.exists():
        existing = hook.read_text(encoding="utf-8", errors="ignore")
        if _COMMIT_MSG_HOOK_MARKER in existing:
            if existing == body:
                return False
            hook.write_text(body, encoding="utf-8")
            hook.chmod(0o755)
            return True
        return False  # foreign hook — preserve
    hook.write_text(body, encoding="utf-8")
    hook.chmod(0o755)
    return True


def commit_msg_hook_installed(repo_root: Path) -> bool:
    hook = repo_root / ".git" / "hooks" / "commit-msg"
    if not hook.exists():
        return False
    try:
        return _COMMIT_MSG_HOOK_MARKER in hook.read_text(encoding="utf-8", errors="ignore")
    except OSError:
        return False


def installed_commit_msg_hook_version(repo_root: Path) -> Optional[str]:
    """Read the `# logmind-hook-version: …` marker from the installed
    commit-msg hook. Returns the version string when present + ours;
    None for missing / foreign / unmarked hooks.
    """
    hook = repo_root / ".git" / "hooks" / "commit-msg"
    if not hook.exists():
        return None
    content = hook.read_text(encoding="utf-8", errors="ignore")
    if _COMMIT_MSG_HOOK_MARKER not in content:
        return None
    match = _HOOK_VERSION_RE.search(content)
    return match.group(1) if match else None


# ---------------------------------------------------------------------------
# v0.5.11 / issue #58 — post-rewrite hook
#
# Companion to post-merge. The merge driver in .gitattributes fires per
# conflict during `git merge`, and post-merge sweeps the full post-merge
# tree at end-of-merge. But neither path covers `git rebase` or
# `git commit --amend` — both rewrite history without producing merge
# conflicts on logmind's derived files, so the driver never fires AND
# post-merge never fires either.
#
# Concrete failure mode (issue #58, hit on agent-skills PRs #21 + #22):
# rebasing a feature branch with 2+ `logmind log` commits against a moved
# main produces a stale `docs/timeline.md` (only the first commit's regen
# survives; subsequent commits' regens are dropped during the rebase
# replay). check-derived-docs fails the resulting PR.
#
# Fix: install a `post-rewrite` hook that regenerates timeline.md +
# file-structure.md after every rebase/amend. Git invokes it once at
# end-of-rewrite with $1 = "rebase" or "amend" — we don't branch on
# the kind because the regen behaviour is identical.
# ---------------------------------------------------------------------------


_POST_REWRITE_HOOK_MARKER = "# logmind post-rewrite hook"


def _build_post_rewrite_hook_body() -> str:
    """Build the canonical post-rewrite hook body for the current binary
    version. v0.6.10 embeds the binary version so doctor can detect drift
    between the binary that wrote this hook vs the current binary.
    """
    return (
        "#!/bin/sh\n"
        "# logmind post-rewrite hook\n"
        f"{HOOK_VERSION_PREFIX}{_current_logmind_version()}\n"
        "# Installed by `logmind init` (v0.5.11+). Companion to the post-merge\n"
        "# hook. Fires after `git rebase` or `git commit --amend` and\n"
        "# regenerates derived docs from the FULL post-rewrite working tree.\n"
        "#\n"
        "# Why: the merge driver in .gitattributes only fires when a merge\n"
        "# produces conflicts on the derived files. A clean rebase rewrites\n"
        "# multiple commits without ever invoking the driver — leaving\n"
        "# docs/timeline.md and docs/file-structure.md stale relative to the\n"
        "# replayed `docs/decisions-branches/<branch>.md` entries. This hook\n"
        "# sweeps the final state once and stages the regen for inclusion in\n"
        "# the user's next commit / amend cycle.\n"
        "#\n"
        "# Git invokes post-rewrite with $1 = \"rebase\" or \"amend\"; the regen\n"
        "# behaviour is identical in both, so we don't branch on $1.\n"
        "#\n"
        "# v0.6.10: hook-version marker embedded above for doctor drift detection.\n"
        "\n"
        "if command -v logmind >/dev/null 2>&1 && [ -f .logmind/config.yml ]; then\n"
        "  if [ -d docs ]; then\n"
        "    logmind timeline --write docs/timeline.md >/dev/null 2>&1 || true\n"
        "    logmind file-structure --write docs/file-structure.md >/dev/null 2>&1 || true\n"
        "    # Stage the regens if they changed anything; user can `git commit --amend`\n"
        "    # or include in their next commit.\n"
        "    git add docs/timeline.md docs/file-structure.md 2>/dev/null || true\n"
        "  fi\n"
        "fi\n"
    )


def install_post_rewrite_hook(repo_root: Path) -> bool:
    """Write `.git/hooks/post-rewrite` to re-regenerate derived files
    after every rebase or `git commit --amend`. Returns True if the
    hook was created or updated, False if logmind's version was
    already present at the CURRENT binary's version.

    Refuses to overwrite a non-logmind hook (someone may have custom
    hook content; we leave it alone and let `logmind doctor` flag the
    state instead).
    """
    hooks_dir = repo_root / ".git" / "hooks"
    if not hooks_dir.exists():
        return False
    hook = hooks_dir / "post-rewrite"
    body = _build_post_rewrite_hook_body()
    if hook.exists():
        existing = hook.read_text(encoding="utf-8", errors="ignore")
        if _POST_REWRITE_HOOK_MARKER in existing:
            if existing == body:
                return False
            hook.write_text(body, encoding="utf-8")
            hook.chmod(0o755)
            return True
        return False
    hook.write_text(body, encoding="utf-8")
    hook.chmod(0o755)
    return True


def post_rewrite_hook_installed(repo_root: Path) -> bool:
    hook = repo_root / ".git" / "hooks" / "post-rewrite"
    if not hook.exists():
        return False
    try:
        return _POST_REWRITE_HOOK_MARKER in hook.read_text(encoding="utf-8", errors="ignore")
    except OSError:
        return False


def driver_configured(repo_root: Path) -> bool:
    """Return True iff every key in MERGE_DRIVER_CONFIG is set to its
    expected value in the repo's local git config."""
    import subprocess

    if not (repo_root / ".git").exists():
        return False
    for key, value in MERGE_DRIVER_CONFIG:
        try:
            result = subprocess.run(
                ["git", "-C", str(repo_root), "config", "--get", key],
                capture_output=True,
                text=True,
                check=False,
            )
        except (FileNotFoundError, OSError):
            return False
        if result.returncode != 0 or result.stdout.strip() != value:
            return False
    return True
