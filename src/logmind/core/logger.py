"""Decision logging functionality."""

from __future__ import annotations

import sys
from datetime import datetime
from pathlib import Path
from typing import List, Optional, Union

from logmind.core.atomic_io import atomic_write_text
from logmind.core.config import load_config
from logmind.core.git_handler import (
    commit_and_push,
    current_branch,
    default_branch,
    git_add,
    git_add_all,
    git_commit,
    git_push,
    is_git_repo,
    unstaged_tracked_modifications,
)
from logmind.core.gitattributes import (
    configure_merge_drivers,
    install_post_merge_hook,
    install_post_rewrite_hook,
)
from logmind.core.timeline import write_timeline
from logmind.core.tree_gen import update_file_structure


def _sanitize_branch(name: str) -> str:
    """Make a branch name safe to use as a filename component."""
    return name.replace("/", "__").replace("\\", "__").replace(":", "_")


def _resolve_decisions_path(docs_path: Path, config) -> Path:
    """
    Resolve which decisions log file the next entry should be appended to.

    On the default branch, in non-git directories, on detached HEAD, or when
    branch_aware is disabled, returns docs/decisions.md. Otherwise returns
    docs/decisions-branches/<sanitized-branch>.md (creating the directory).

    The repo root is inferred from ``docs_path.parent`` so the resolver works
    regardless of the caller's cwd (important for tests and library usage).
    """
    decisions_path = docs_path / "decisions.md"
    repo_path = docs_path.parent

    if not config.branch_aware or not is_git_repo(repo_path):
        return decisions_path

    branch = current_branch(repo_path)
    if branch is None or branch == default_branch(repo_path):
        return decisions_path

    branch_dir = docs_path / "decisions-branches"
    branch_dir.mkdir(parents=True, exist_ok=True)
    return branch_dir / f"{_sanitize_branch(branch)}.md"


def _archive_path_for(decisions_path: Path) -> Path:
    """Return the archive file paired with a given decisions log file."""
    if decisions_path.name == "decisions.md":
        return decisions_path.parent / "decisions-archive.md"
    return decisions_path.parent / f"{decisions_path.stem}-archive.md"


def _format_decision(
    decision: str,
    reasoning: Optional[str] = None,
    alternatives: Optional[List[str]] = None,
    implications: Optional[List[str]] = None,
) -> str:
    """
    Format a decision entry in markdown.

    Args:
        decision: Decision summary
        reasoning: Why this decision was made
        alternatives: Other options considered
        implications: What this decision means

    Returns:
        Formatted decision entry
    """
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M")
    lines = [f"## {timestamp} - {decision}", ""]

    if reasoning:
        lines.append(f"**Reasoning:** {reasoning}")
        lines.append("")

    if alternatives:
        alt_str = ", ".join(alternatives)
        lines.append(f"**Alternatives considered:** {alt_str}")
        lines.append("")

    if implications:
        lines.append("**Implications:**")
        for impl in implications:
            lines.append(f"- {impl}")
        lines.append("")

    lines.append("---")
    lines.append("")

    return "\n".join(lines)


def _count_decisions(content: str) -> int:
    """
    Count number of decisions in a decision log file.

    Args:
        content: File content

    Returns:
        Number of decisions (count of "## YYYY-MM-DD" headers)
    """
    lines = content.split("\n")
    count = 0
    for line in lines:
        # Match decision headers: "## 2025-10-19 14:32 - ..."
        if line.startswith("## ") and any(char.isdigit() for char in line[:20]):
            count += 1
    return count


def _extract_oldest_decision(content: str) -> tuple[str, str]:
    """
    Extract and remove the oldest decision from content.

    Args:
        content: Decision log content

    Returns:
        Tuple of (oldest_decision, remaining_content)
    """
    lines = content.split("\n")

    # Find first decision header
    first_decision_idx = None
    second_decision_idx = None

    for i, line in enumerate(lines):
        if line.startswith("## ") and any(char.isdigit() for char in line[:20]):
            if first_decision_idx is None:
                first_decision_idx = i
            elif second_decision_idx is None:
                second_decision_idx = i
                break

    if first_decision_idx is None:
        return "", content

    # Extract oldest decision
    if second_decision_idx is not None:
        # There are multiple decisions
        oldest_decision_lines = lines[first_decision_idx:second_decision_idx]
        remaining_lines = lines[:first_decision_idx] + lines[second_decision_idx:]
    else:
        # Only one decision
        oldest_decision_lines = lines[first_decision_idx:]
        remaining_lines = lines[:first_decision_idx]

    oldest_decision = "\n".join(oldest_decision_lines).strip()
    remaining_content = "\n".join(remaining_lines)

    return oldest_decision, remaining_content


def _archive_oldest_decision(decisions_path: Path) -> None:
    """
    Move the oldest decision from a decisions log file into its paired archive.

    Args:
        decisions_path: Path to the decisions log file. The archive is derived
            via :func:`_archive_path_for` (e.g. ``decisions.md`` →
            ``decisions-archive.md``; ``decisions-branches/foo.md`` →
            ``decisions-branches/foo-archive.md``).
    """
    # Backwards-compat: callers used to pass docs_dir. If we got a directory,
    # assume the canonical decisions.md inside it.
    if decisions_path.is_dir():
        decisions_path = decisions_path / "decisions.md"

    archive_path = _archive_path_for(decisions_path)

    # Read current decisions
    decisions_content = decisions_path.read_text(encoding="utf-8")

    # Extract oldest decision
    oldest_decision, remaining_content = _extract_oldest_decision(decisions_content)

    if not oldest_decision:
        return

    # Write remaining decisions back
    atomic_write_text(decisions_path, remaining_content)

    # Read archive (or create if doesn't exist)
    if archive_path.exists():
        archive_content = archive_path.read_text(encoding="utf-8")
    else:
        template_path = Path(__file__).parent.parent / "templates" / "decisions-archive.md.template"
        archive_content = template_path.read_text(encoding="utf-8")

    # Insert oldest decision at the end of archive (after header)
    archive_lines = archive_content.split("\n")

    # Find where to insert (after the "---" separator)
    insert_idx = len(archive_lines)
    for i, line in enumerate(archive_lines):
        if line.strip() == "---":
            insert_idx = i + 1
            break

    # Insert the decision
    archive_lines.insert(insert_idx, oldest_decision)
    archive_lines.insert(insert_idx + 1, "")

    # Write back to archive
    atomic_write_text(archive_path, "\n".join(archive_lines))


def log(
    decision: str,
    reasoning: Optional[str] = None,
    alternatives: Optional[Union[List[str], str]] = None,
    implications: Optional[Union[List[str], str]] = None,
    docs_path: Optional[Path] = None,
    auto_commit: Optional[bool] = None,
    auto_push: Optional[bool] = None,
    stage: str = "all",
    extra_scoped_paths: Optional[List[str]] = None,
) -> None:
    """
    Log a decision to the decision log.

    This function:
    1. Appends the decision to docs/decisions.md
    2. Archives oldest decision if > max_recent entries (from config)
    3. Updates docs/file-structure.md with current tree
    4. Regenerates docs/timeline.md so the derived index stays in sync
       with the decision file it just wrote (v0.2.3+)
    5. Commits and pushes changes (based on config or parameters)

    Args:
        decision: Decision summary (will be used in commit message)
        reasoning: Why this decision was made
        alternatives: Other options considered (string or list)
        implications: What this decision means (string or list)
        docs_path: Path to docs directory. Defaults to ./docs
        auto_commit: Whether to auto-commit. If None, uses config value.
        auto_push: Whether to auto-push. If None, uses config value.
        stage: ``"all"`` (default since v0.2.7) stages every change in the
            working tree alongside the decision — `logmind log` is the
            single add+commit+push primitive for automated workflows.
            ``"scoped"`` stages only the decision log + companion files
            (file-structure.md, decisions-archive.md if rotated,
            timeline.md if changed) — opt in when you have unrelated WIP
            you want to keep unstaged.

    Raises:
        FileNotFoundError: If docs/ doesn't exist (run logmind init first)
    """
    if docs_path is None:
        docs_path = Path.cwd() / "docs"

    if not docs_path.exists():
        raise FileNotFoundError(
            "docs/ directory not found. Run 'logmind init' first to initialize."
        )

    # v0.5.12: auto-install merge-driver config + post-merge + post-rewrite
    # hooks on every invocation. All three helpers are idempotent (no-op
    # when already configured) AND silent no-ops outside a git repo.
    # Cost: ~3 `git config --get` calls + 2 file stats on each `logmind log`.
    #
    # Why: pre-v0.5.12, `logmind init` was the only path that installed
    # the per-clone git config + hooks. Fresh clones / CI runners /
    # agents working in throwaway worktrees had the committed
    # `.gitattributes` reference but no driver registered locally → git
    # refused to invoke the driver (security guard against untrusted
    # repos) → fell back to ort 3-way merge → text-valid but
    # semantically-wrong timeline.md → check-derived-docs failed
    # downstream. Hit live on tokenomics #21. Captured in memory
    # `project_timeline_conflict_should_auto_resolve`. v0.5.12 makes
    # `logmind log` self-healing: the first invocation in any fresh
    # checkout leaves the clone fully configured for future merges /
    # rebases / amends.
    repo_root = docs_path.parent
    configure_merge_drivers(repo_root)
    install_post_merge_hook(repo_root)
    install_post_rewrite_hook(repo_root)

    # Load configuration
    config = load_config()

    # Use config values if not explicitly provided
    if auto_commit is None:
        auto_commit = config.auto_commit
    if auto_push is None:
        auto_push = config.auto_push

    # Normalize inputs
    if isinstance(alternatives, str):
        alternatives = [alternatives]
    if isinstance(implications, str):
        implications = [implications]

    # Format the decision
    decision_entry = _format_decision(decision, reasoning, alternatives, implications)

    # Resolve target file based on current branch
    decisions_path = _resolve_decisions_path(docs_path, config)
    current_content = decisions_path.read_text(encoding="utf-8") if decisions_path.exists() else ""

    # Append new decision (atomic so concurrent loggers can't truncate)
    updated_content = current_content + decision_entry
    atomic_write_text(decisions_path, updated_content)

    # Check if we need to archive (use config value)
    decision_count = _count_decisions(updated_content)
    max_recent = config.max_recent_decisions
    archive_rotated = False
    if decision_count > max_recent:
        _archive_oldest_decision(decisions_path)
        archive_rotated = True

    # v0.5.8 / issue #66: Regenerate docs/file-structure.md on every
    # branch — same logic as timeline.md below. The pre-v0.5.8 behaviour
    # (default-branch-only regen) self-perpetuated a 1-entry-stale cycle
    # on main: PR adds docs/decisions-branches/<branch>.md → squash-merges
    # without an updated file-structure → main's file-structure.md
    # one entry behind reality → next PR catches it via check-derived-docs,
    # adds a regen + a new decision file → cycle repeats. The original
    # rationale (per-branch regen would conflict against main) was made
    # obsolete by v0.3.0's merge driver for file-structure.md, which
    # resolves conflicts by regenerating from the merged tree.
    #
    # update_file_structure handles all OSError / not-git-repo edge cases
    # internally; safe to call unconditionally per config.
    file_structure_updated = False
    if config.auto_update_file_structure:
        update_file_structure(docs_path)
        file_structure_updated = True

    # Regenerate docs/timeline.md on every branch — unlike file-structure.md,
    # timeline conflicts are trivially three-way-mergeable (each branch
    # appends its own dated row), and the check-derived-docs CI gate runs
    # on PR branches, so skipping on feature branches would defeat the
    # auto-heal. write_timeline() returns True only when content actually
    # changed; we use that to decide whether to stage it.
    timeline_path = docs_path / "timeline.md"
    timeline_updated = write_timeline(timeline_path, docs_path)

    # Commit and push if requested
    if auto_commit and is_git_repo():
        if stage == "all":
            # Pre-v0.1.2 behavior: stage everything in the working tree.
            # Use sparingly — it sweeps unrelated changes into the commit.
            git_add_all()
        else:
            # Default: stage only files logmind itself touched. Unrelated
            # working-tree changes don't piggyback on the decision commit.
            scoped: List[str] = [str(decisions_path)]
            if file_structure_updated:
                scoped.append(str(docs_path / "file-structure.md"))
            if archive_rotated:
                scoped.append(str(docs_path / "decisions-archive.md"))
            if timeline_updated:
                scoped.append(str(timeline_path))
            if extra_scoped_paths:
                scoped.extend(extra_scoped_paths)
            git_add(scoped)

            # v0.5.10 / issue #59: warn loudly when --stage scoped runs
            # with tracked-but-unstaged modifications still present after
            # staging logmind's own files. Without this warning, users
            # who forget `git add` before `logmind log --stage scoped`
            # silently ship only the decision-log entry — the actual
            # code change stays unstaged and the PR diff doesn't match
            # its description. Hit live in clud-bug PR #87 and reporulez
            # PR #20 in the 2026-05-27 wrap-up session. Q6 invariant:
            # warnings never silently dropped (warn-not-block, the user
            # may legitimately have unrelated WIP they want unstaged).
            leftover = unstaged_tracked_modifications()
            if leftover:
                count = len(leftover)
                sys.stderr.write(
                    f"\nWarning: --stage scoped committed without "
                    f"{count} tracked modification"
                    f"{'s' if count != 1 else ''} "
                    f"(still unstaged):\n"
                )
                for f in leftover:
                    sys.stderr.write(f"  - {f}\n")
                sys.stderr.write(
                    "  Did you mean --stage all (the default since "
                    "v0.2.7)? If you intended to include the file(s), "
                    "run `git add <files> && git commit --amend "
                    "--no-edit` before pushing.\n\n"
                )

        # Use configured commit message template
        commit_message = config.commit_message_template.format(decision=decision)
        git_commit(commit_message)

        if auto_push:
            git_push()


def log_first_decision(docs_path: Optional[Path] = None) -> None:
    """
    Log the first decision when initializing logmind.

    Args:
        docs_path: Path to docs directory. Defaults to ./docs
    """
    log(
        decision="Initialize logmind decision tracking",
        reasoning="Starting structured decision logging for this project to maintain clear documentation of architectural choices and provide context for AI agents.",
        implications=[
            "All significant decisions should now be logged using `logmind.log()`",
            "AI agents will have access to decision history via docs/decisions.md",
            "Git history will serve as an audit trail for all decisions",
        ],
        alternatives=["Manual decision documentation", "ADR (Architecture Decision Records)"],
        docs_path=docs_path,
        auto_commit=False,  # Will be committed with other init files
    )
