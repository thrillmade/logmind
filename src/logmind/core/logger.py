"""Decision logging functionality."""

from datetime import datetime
from pathlib import Path
from typing import List, Optional, Union

from logmind.core.git_handler import commit_and_push, is_git_repo
from logmind.core.tree_gen import update_file_structure


MAX_RECENT_DECISIONS = 20


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


def _archive_oldest_decision(docs_path: Path) -> None:
    """
    Move the oldest decision from decisions.md to decisions-archive.md.

    Args:
        docs_path: Path to docs directory
    """
    decisions_path = docs_path / "decisions.md"
    archive_path = docs_path / "decisions-archive.md"

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
    auto_commit: bool = True,
    auto_push: bool = True,
) -> None:
    """
    Log a decision to the decision log.

    This function:
    1. Appends the decision to docs/decisions.md
    2. Archives oldest decision if > 20 entries
    3. Updates docs/file-structure.md with current tree
    4. Commits and pushes changes (if auto_commit=True)

    Args:
        decision: Decision summary (will be used in commit message)
        reasoning: Why this decision was made
        alternatives: Other options considered (string or list)
        implications: What this decision means (string or list)
        docs_path: Path to docs directory. Defaults to ./docs
        auto_commit: Whether to auto-commit. Defaults to True.
        auto_push: Whether to auto-push. Defaults to True.

    Raises:
        FileNotFoundError: If docs/ doesn't exist (run logmind init first)
    """
    if docs_path is None:
        docs_path = Path.cwd() / "docs"

    if not docs_path.exists():
        raise FileNotFoundError(
            "docs/ directory not found. Run 'logmind init' first to initialize."
        )

    # Normalize inputs
    if isinstance(alternatives, str):
        alternatives = [alternatives]
    if isinstance(implications, str):
        implications = [implications]

    # Format the decision
    decision_entry = _format_decision(decision, reasoning, alternatives, implications)

    # Append to decisions.md
    decisions_path = docs_path / "decisions.md"
    current_content = decisions_path.read_text() if decisions_path.exists() else ""

    # Append new decision
    updated_content = current_content + decision_entry
    decisions_path.write_text(updated_content)

    # Check if we need to archive
    decision_count = _count_decisions(updated_content)
    if decision_count > MAX_RECENT_DECISIONS:
        _archive_oldest_decision(docs_path)

    # Update file structure
    update_file_structure(docs_path)

    # Commit and push if requested
    if auto_commit and is_git_repo():
        files_to_commit = [
            "docs/decisions.md",
            "docs/file-structure.md",
        ]

        # Add archive if it was created/updated
        archive_path = docs_path / "decisions-archive.md"
        if archive_path.exists():
            files_to_commit.append("docs/decisions-archive.md")

        commit_message = f"logmind: {decision}"
        commit_and_push(files_to_commit, commit_message, push=auto_push)


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
