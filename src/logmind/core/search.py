"""Search functionality for logmind decisions."""

import re
from pathlib import Path
from typing import List, Optional, Tuple


class SearchResult:
    """Represents a search result from decision files."""

    def __init__(
        self,
        file: str,
        decision_title: str,
        line_number: int,
        matched_line: str,
        context_before: List[str],
        context_after: List[str],
    ):
        self.file = file
        self.decision_title = decision_title
        self.line_number = line_number
        self.matched_line = matched_line
        self.context_before = context_before
        self.context_after = context_after

    def __repr__(self):
        return (
            f"SearchResult(file={self.file}, decision={self.decision_title}, "
            f"line={self.line_number})"
        )


def search_decisions(
    query: str,
    docs_path: Path,
    case_sensitive: bool = False,
    include_archive: bool = True,
    context_lines: int = 2,
) -> List[SearchResult]:
    """
    Search through decision files for a query string.

    Args:
        query: Search term or regex pattern
        docs_path: Path to docs directory
        case_sensitive: Whether to perform case-sensitive search
        include_archive: Whether to search archived decisions
        context_lines: Number of context lines to show before/after match

    Returns:
        List of SearchResult objects
    """
    results = []

    # Files to search
    files_to_search = [docs_path / "decisions.md"]
    if include_archive:
        archive_path = docs_path / "decisions-archive.md"
        if archive_path.exists():
            files_to_search.append(archive_path)

    # Compile regex pattern
    flags = 0 if case_sensitive else re.IGNORECASE
    try:
        pattern = re.compile(query, flags)
    except re.error:
        # If query isn't valid regex, escape it and search literally
        pattern = re.compile(re.escape(query), flags)

    for file_path in files_to_search:
        if not file_path.exists():
            continue

        results.extend(
            _search_file(
                file_path=file_path,
                pattern=pattern,
                context_lines=context_lines,
            )
        )

    return results


def _search_file(
    file_path: Path,
    pattern: re.Pattern,
    context_lines: int,
) -> List[SearchResult]:
    """
    Search a single file for pattern matches.

    Args:
        file_path: Path to file to search
        pattern: Compiled regex pattern
        context_lines: Number of context lines to show

    Returns:
        List of SearchResult objects
    """
    results = []

    with open(file_path, "r", encoding="utf-8") as f:
        lines = f.readlines()

    current_decision = None

    for i, line in enumerate(lines):
        # Track current decision section (lines starting with ##)
        if line.startswith("## "):
            current_decision = line.strip("# \n")

        # Check if line matches pattern
        if pattern.search(line):
            # Get context lines
            start_idx = max(0, i - context_lines)
            end_idx = min(len(lines), i + context_lines + 1)

            context_before = [
                ln.rstrip("\n") for ln in lines[start_idx:i]
            ]
            context_after = [
                ln.rstrip("\n") for ln in lines[i + 1 : end_idx]
            ]

            result = SearchResult(
                file=file_path.name,
                decision_title=current_decision or "Unknown decision",
                line_number=i + 1,  # 1-indexed line numbers
                matched_line=line.rstrip("\n"),
                context_before=context_before,
                context_after=context_after,
            )
            results.append(result)

    return results


def format_search_results(
    results: List[SearchResult],
    show_context: bool = True,
    highlight_term: Optional[str] = None,
) -> str:
    """
    Format search results for display.

    Args:
        results: List of SearchResult objects
        show_context: Whether to show context lines
        highlight_term: Term to highlight in output (basic terminal highlighting)

    Returns:
        Formatted string for display
    """
    if not results:
        return "No matches found."

    output_lines = []

    for i, result in enumerate(results):
        if i > 0:
            output_lines.append("")  # Blank line between results

        # Header: file and decision
        header = f"{result.file} - {result.decision_title} (line {result.line_number})"
        output_lines.append(header)
        output_lines.append("-" * len(header))

        if show_context:
            # Show context before
            for ctx_line in result.context_before:
                output_lines.append(f"  {ctx_line}")

        # Show matched line with marker
        matched_line = result.matched_line
        if highlight_term:
            # Simple highlighting with surrounding markers
            matched_line = re.sub(
                f"({re.escape(highlight_term)})",
                r">>> \1 <<<",
                matched_line,
                flags=re.IGNORECASE,
            )
        output_lines.append(f"> {matched_line}")

        if show_context:
            # Show context after
            for ctx_line in result.context_after:
                output_lines.append(f"  {ctx_line}")

    return "\n".join(output_lines)
