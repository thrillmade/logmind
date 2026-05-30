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

from pathlib import Path
from typing import Iterable

LOGMIND_GITATTRIBUTES_START = "# >>> logmind >>>"
LOGMIND_GITATTRIBUTES_END = "# <<< logmind <<<"

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
_POST_MERGE_HOOK_BODY = """#!/bin/sh
# logmind post-merge hook
# Installed by `logmind init` (v0.3.0+). After every merge, regenerate
# derived docs from the FULL post-merge working tree. Belt + suspenders
# with the merge driver in .gitattributes — the driver fires per-file
# during conflict resolution, before sibling non-conflicted files (e.g.
# the merged-in branch's docs/decisions-branches/<branch>.md) are
# checked out. This hook runs once at the end and sweeps any incomplete
# regenerations.

if command -v logmind >/dev/null 2>&1 && [ -f .logmind/config.yml ]; then
  if [ -d docs ]; then
    logmind timeline --write docs/timeline.md >/dev/null 2>&1 || true
    logmind file-structure --write docs/file-structure.md >/dev/null 2>&1 || true
    # Stage the regens if they changed anything; user can `git commit --amend`
    # or include in their next commit.
    git add docs/timeline.md docs/file-structure.md 2>/dev/null || true
  fi
fi
"""


def install_post_merge_hook(repo_root: Path) -> bool:
    """Write `.git/hooks/post-merge` to re-regenerate derived files after
    every merge. Returns True if the hook was created or updated, False
    if logmind's version was already present.

    Refuses to overwrite a non-logmind hook (someone may have custom
    hook content; we leave it alone and let `logmind doctor` flag the
    state instead).
    """
    hooks_dir = repo_root / ".git" / "hooks"
    if not hooks_dir.exists():
        return False
    hook = hooks_dir / "post-merge"
    if hook.exists():
        existing = hook.read_text(encoding="utf-8", errors="ignore")
        if _POST_MERGE_HOOK_MARKER in existing:
            # Our hook already present (or some legacy version of it).
            # Rewrite to current canonical contents; same idempotency
            # contract as the merge-driver git config.
            if existing == _POST_MERGE_HOOK_BODY:
                return False
            hook.write_text(_POST_MERGE_HOOK_BODY, encoding="utf-8")
            hook.chmod(0o755)
            return True
        # Foreign hook — don't trample.
        return False
    hook.write_text(_POST_MERGE_HOOK_BODY, encoding="utf-8")
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
_POST_REWRITE_HOOK_BODY = """#!/bin/sh
# logmind post-rewrite hook
# Installed by `logmind init` (v0.5.11+). Companion to the post-merge
# hook. Fires after `git rebase` or `git commit --amend` and
# regenerates derived docs from the FULL post-rewrite working tree.
#
# Why: the merge driver in .gitattributes only fires when a merge
# produces conflicts on the derived files. A clean rebase rewrites
# multiple commits without ever invoking the driver — leaving
# docs/timeline.md and docs/file-structure.md stale relative to the
# replayed `docs/decisions-branches/<branch>.md` entries. This hook
# sweeps the final state once and stages the regen for inclusion in
# the user's next commit / amend cycle.
#
# Git invokes post-rewrite with $1 = "rebase" or "amend"; the regen
# behaviour is identical in both, so we don't branch on $1.

if command -v logmind >/dev/null 2>&1 && [ -f .logmind/config.yml ]; then
  if [ -d docs ]; then
    logmind timeline --write docs/timeline.md >/dev/null 2>&1 || true
    logmind file-structure --write docs/file-structure.md >/dev/null 2>&1 || true
    # Stage the regens if they changed anything; user can `git commit --amend`
    # or include in their next commit.
    git add docs/timeline.md docs/file-structure.md 2>/dev/null || true
  fi
fi
"""


def install_post_rewrite_hook(repo_root: Path) -> bool:
    """Write `.git/hooks/post-rewrite` to re-regenerate derived files
    after every rebase or `git commit --amend`. Returns True if the
    hook was created or updated, False if logmind's version was
    already present.

    Refuses to overwrite a non-logmind hook (someone may have custom
    hook content; we leave it alone and let `logmind doctor` flag the
    state instead).
    """
    hooks_dir = repo_root / ".git" / "hooks"
    if not hooks_dir.exists():
        return False
    hook = hooks_dir / "post-rewrite"
    if hook.exists():
        existing = hook.read_text(encoding="utf-8", errors="ignore")
        if _POST_REWRITE_HOOK_MARKER in existing:
            if existing == _POST_REWRITE_HOOK_BODY:
                return False
            hook.write_text(_POST_REWRITE_HOOK_BODY, encoding="utf-8")
            hook.chmod(0o755)
            return True
        # Foreign hook — don't trample.
        return False
    hook.write_text(_POST_REWRITE_HOOK_BODY, encoding="utf-8")
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
