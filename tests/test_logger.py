"""Tests for core/logger.py."""

from pathlib import Path

import pytest

from logmind.core.logger import (
    _archive_oldest_decision,
    _count_decisions,
    _extract_oldest_decision,
    _format_decision,
    log,
    log_first_decision,
)


def test_format_decision_basic():
    """Test basic decision formatting."""
    result = _format_decision("Test decision")
    assert "Test decision" in result
    assert "##" in result  # Should have timestamp header


def test_format_decision_with_reasoning():
    """Test decision formatting with reasoning."""
    result = _format_decision("Test decision", reasoning="Because reasons")
    assert "Test decision" in result
    assert "Because reasons" in result
    assert "**Reasoning:**" in result


def test_format_decision_with_alternatives():
    """Test decision formatting with alternatives."""
    result = _format_decision("Test decision", alternatives=["Option A", "Option B"])
    assert "Option A" in result
    assert "Option B" in result
    assert "**Alternatives considered:**" in result


def test_format_decision_with_implications():
    """Test decision formatting with implications."""
    result = _format_decision("Test decision", implications=["Impact 1", "Impact 2"])
    assert "Impact 1" in result
    assert "Impact 2" in result
    assert "**Implications:**" in result


def test_count_decisions_empty():
    """Test counting decisions in empty content."""
    assert _count_decisions("# Decision Log\n\n---\n") == 0


def test_count_decisions_with_entries():
    """Test counting decisions with entries."""
    content = """# Decision Log

---
## 2025-10-19 16:25 - First decision

Some content

---
## 2025-10-19 16:26 - Second decision

More content

---
"""
    assert _count_decisions(content) == 2


def test_extract_oldest_decision():
    """Test extracting oldest decision."""
    content = """# Decision Log

---
## 2025-10-19 16:25 - First decision

Content 1

---
## 2025-10-19 16:26 - Second decision

Content 2

---
"""
    oldest, remaining = _extract_oldest_decision(content)

    assert "First decision" in oldest
    assert "Content 1" in oldest
    assert "First decision" not in remaining
    assert "Second decision" in remaining


def test_extract_oldest_decision_single():
    """Test extracting when only one decision exists."""
    content = """# Decision Log

---
## 2025-10-19 16:25 - Only decision

Content

---
"""
    oldest, remaining = _extract_oldest_decision(content)

    assert "Only decision" in oldest
    assert "## 2025" not in remaining  # Header should be removed


def test_log_creates_decision_file(docs_dir):
    """Test that log creates decision entry."""
    log("Test decision", reasoning="Test reasoning", docs_path=docs_dir, auto_commit=False)

    decisions_file = docs_dir / "decisions.md"
    content = decisions_file.read_text()

    assert "Test decision" in content
    assert "Test reasoning" in content


def test_log_with_alternatives_and_implications(docs_dir):
    """Test logging with alternatives and implications."""
    log(
        "Test decision",
        alternatives=["A", "B"],
        implications=["Impact 1"],
        docs_path=docs_dir,
        auto_commit=False,
    )

    content = (docs_dir / "decisions.md").read_text()

    assert "A" in content
    assert "B" in content
    assert "Impact 1" in content


def test_log_archives_after_20_decisions(docs_dir):
    """Test that decisions are archived after 20 entries."""
    # Add 21 decisions
    for i in range(21):
        log(f"Decision {i}", docs_path=docs_dir, auto_commit=False)

    decisions_content = (docs_dir / "decisions.md").read_text()
    archive_content = (docs_dir / "decisions-archive.md").read_text()

    # Should have exactly 20 in decisions.md
    assert _count_decisions(decisions_content) == 20

    # First decision should be in archive
    assert "Decision 0" in archive_content
    assert "Decision 0" not in decisions_content

    # Most recent should still be in decisions.md
    assert "Decision 20" in decisions_content


def test_log_updates_file_structure(docs_dir):
    """Test that logging updates file structure."""
    log("Test decision", docs_path=docs_dir, auto_commit=False)

    file_structure = docs_dir / "file-structure.md"
    assert file_structure.exists()

    content = file_structure.read_text()
    assert "Last updated:" in content


def test_log_without_docs_dir_raises_error(temp_dir):
    """Test that logging without docs dir raises error."""
    with pytest.raises(FileNotFoundError, match="docs/ directory not found"):
        log("Test decision", docs_path=temp_dir / "docs", auto_commit=False)


def test_log_first_decision(docs_dir):
    """Test logging the first initialization decision."""
    log_first_decision(docs_path=docs_dir)

    content = (docs_dir / "decisions.md").read_text()

    assert "Initialize logmind decision tracking" in content
    assert "AI agents" in content
    assert "ADR" in content  # Alternative mentioned


def test_log_with_string_alternatives_and_implications(docs_dir):
    """Test that string alternatives/implications are converted to lists."""
    log(
        "Test decision",
        alternatives="Single alternative",
        implications="Single implication",
        docs_path=docs_dir,
        auto_commit=False,
    )

    content = (docs_dir / "decisions.md").read_text()

    assert "Single alternative" in content
    assert "Single implication" in content
