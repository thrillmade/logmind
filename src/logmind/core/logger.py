"""Decision logging functionality."""

from __future__ import annotations

from datetime import datetime
from pathlib import Path
from typing import List, Optional, Union

from logmind.core.config import load_config
from logmind.core.git_handler import (
    commit_and_push,
    current_branch,
    default_branch,
    git_add_all,
    git_commit,
    git_push,
    is_git_repo,
)
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
    decisions_content = decisions_path.read_text()

    # Extract oldest decision
    oldest_decision, remaining_content = _extract_oldest_decision(decisions_content)

    if not oldest_decision:
        return

    # Write remaining decisions back
    decisions_path.write_text(remaining_content)

    # Read archive (or create if doesn't exist)
    if archive_path.exists():
        archive_content = archive_path.read_text()
    else:
        template_path = Path(__file__).parent.parent / "templates" / "decisions-archive.md.template"
        archive_content = template_path.read_text()

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
    archive_path.write_text("\n".join(archive_lines))


def log(
    decision: str,
    reasoning: Optional[str] = None,
    alternatives: Optional[Union[List[str], str]] = None,
    implications: Optional[Union[List[str], str]] = None,
    docs_path: Optional[Path] = None,
    auto_commit: Optional[bool] = None,
    auto_push: Optional[bool] = None,
) -> None:
    """
    Log a decision to the decision log.

    This function:
    1. Appends the decision to docs/decisions.md
    2. Archives oldest decision if > max_recent entries (from config)
    3. Updates docs/file-structure.md with current tree
    4. Commits and pushes changes (based on config or parameters)

    Args:
        decision: Decision summary (will be used in commit message)
        reasoning: Why this decision was made
        alternatives: Other options considered (string or list)
        implications: What this decision means (string or list)
        docs_path: Path to docs directory. Defaults to ./docs
        auto_commit: Whether to auto-commit. If None, uses config value.
        auto_push: Whether to auto-push. If None, uses config value.

    Raises:
        FileNotFoundError: If docs/ doesn't exist (run logmind init first)
    """
    if docs_path is None:
        docs_path = Path.cwd() / "docs"

    if not docs_path.exists():
        raise FileNotFoundError(
            "docs/ directory not found. Run 'logmind init' first to initialize."
        )

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
    current_content = decisions_path.read_text() if decisions_path.exists() else ""

    # Append new decision
    updated_content = current_content + decision_entry
    decisions_path.write_text(updated_content)

    # Check if we need to archive (use config value)
    decision_count = _count_decisions(updated_content)
    max_recent = config.max_recent_decisions
    if decision_count > max_recent:
        _archive_oldest_decision(decisions_path)

    # Update file structure (if configured)
    if config.auto_update_file_structure:
        update_file_structure(docs_path)

    # Commit and push if requested
    if auto_commit and is_git_repo():
        # Add ALL changed files (not just docs)
        git_add_all()

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
