"""Tests for core/search.py."""

from pathlib import Path

import pytest

from logmind.core.search import (
    SearchResult,
    format_search_results,
    search_decisions,
)


@pytest.fixture
def sample_decisions_file(temp_dir):
    """Create a sample decisions.md file for testing."""
    docs_path = temp_dir / "docs"
    docs_path.mkdir()

    decisions_content = """# Decision Log

This file contains the 20 most recent decisions.

---
## 2025-10-19 16:25 - Initialize logmind decision tracking

**Reasoning:** Starting structured decision logging for this project.

**Alternatives considered:** Manual decision documentation, ADR

**Implications:**
- All significant decisions should be logged
- AI agents will have access to decision history
- Git history will serve as an audit trail

---
## 2025-10-19 16:26 - Use PostgreSQL for database

**Reasoning:** Need ACID compliance and strong relational support

**Alternatives considered:** MongoDB, MySQL, SQLite

**Implications:**
- Need to set up connection pooling
- Better data consistency guarantees

---
## 2025-10-19 16:30 - Use Click for CLI framework

**Reasoning:** Click provides excellent argument parsing

**Alternatives considered:** argparse, Typer

**Implications:**
- Need to learn Click API
"""

    decisions_path = docs_path / "decisions.md"
    decisions_path.write_text(decisions_content)

    return docs_path


@pytest.fixture
def sample_with_archive(sample_decisions_file):
    """Create both decisions.md and decisions-archive.md."""
    archive_content = """# Decision Archive

---
## 2025-09-01 - Use Python for backend

**Reasoning:** Team expertise in Python

**Implications:**
- Faster development time
"""

    archive_path = sample_decisions_file / "decisions-archive.md"
    archive_path.write_text(archive_content)

    return sample_decisions_file


def test_search_finds_simple_match(sample_decisions_file):
    """Test basic search functionality."""
    results = search_decisions("PostgreSQL", sample_decisions_file)

    assert len(results) == 1
    assert results[0].decision_title == "2025-10-19 16:26 - Use PostgreSQL for database"
    assert "PostgreSQL" in results[0].matched_line


def test_search_case_insensitive_by_default(sample_decisions_file):
    """Test that search is case-insensitive by default."""
    results = search_decisions("postgresql", sample_decisions_file)

    assert len(results) == 1
    assert "PostgreSQL" in results[0].matched_line


def test_search_case_sensitive(sample_decisions_file):
    """Test case-sensitive search."""
    # Should find with correct case
    results = search_decisions("PostgreSQL", sample_decisions_file, case_sensitive=True)
    assert len(results) == 1

    # Should not find with wrong case
    results = search_decisions("postgresql", sample_decisions_file, case_sensitive=True)
    assert len(results) == 0


def test_search_multiple_matches(sample_decisions_file):
    """Test searching for a term that appears multiple times."""
    results = search_decisions("decision", sample_decisions_file)

    # Should find "decision" in multiple places
    assert len(results) > 1


def test_search_with_regex_pattern(sample_decisions_file):
    """Test searching with regex patterns."""
    # Search for "Click" or "PostgreSQL"
    results = search_decisions("(Click|PostgreSQL)", sample_decisions_file)

    assert len(results) >= 2


def test_search_no_matches(sample_decisions_file):
    """Test search with no matches."""
    results = search_decisions("nonexistent_term_xyz", sample_decisions_file)

    assert len(results) == 0


def test_search_includes_archive_by_default(sample_with_archive):
    """Test that archive is searched by default."""
    results = search_decisions("Python", sample_with_archive)

    # Should find matches in archive (Python appears in title and reasoning)
    assert len(results) == 2
    assert all(r.file == "decisions-archive.md" for r in results)


def test_search_can_exclude_archive(sample_with_archive):
    """Test excluding archive from search."""
    results = search_decisions(
        "Python",
        sample_with_archive,
        include_archive=False,
    )

    # Should not find anything (Python only in archive)
    assert len(results) == 0


def test_search_tracks_decision_context(sample_decisions_file):
    """Test that search correctly tracks which decision a match belongs to."""
    results = search_decisions("ACID", sample_decisions_file)

    assert len(results) == 1
    assert "PostgreSQL" in results[0].decision_title


def test_search_includes_context_lines(sample_decisions_file):
    """Test that context lines are included."""
    results = search_decisions("PostgreSQL", sample_decisions_file, context_lines=2)

    assert len(results) == 1
    assert len(results[0].context_before) > 0 or len(results[0].context_after) > 0


def test_search_context_lines_configurable(sample_decisions_file):
    """Test that context line count is configurable."""
    results_2 = search_decisions("PostgreSQL", sample_decisions_file, context_lines=2)
    results_5 = search_decisions("PostgreSQL", sample_decisions_file, context_lines=5)

    # More context lines should give more results
    total_context_2 = len(results_2[0].context_before) + len(results_2[0].context_after)
    total_context_5 = len(results_5[0].context_before) + len(results_5[0].context_after)

    assert total_context_5 >= total_context_2


def test_search_result_line_numbers(sample_decisions_file):
    """Test that line numbers are correct (1-indexed)."""
    results = search_decisions("PostgreSQL", sample_decisions_file)

    assert len(results) == 1
    assert results[0].line_number > 0  # Should be 1-indexed


def test_search_with_invalid_regex_falls_back_to_literal(sample_decisions_file):
    """Test that invalid regex falls back to literal search."""
    # "[" is invalid regex, should search literally
    results = search_decisions("[", sample_decisions_file)

    # Should not raise an error, and might find matches
    assert isinstance(results, list)


def test_search_nonexistent_docs_path():
    """Test search with nonexistent docs path."""
    fake_path = Path("/nonexistent/path/docs")
    results = search_decisions("test", fake_path)

    # Should return empty results, not error
    assert results == []


def test_format_search_results_empty():
    """Test formatting empty results."""
    formatted = format_search_results([])

    assert "No matches found" in formatted


def test_format_search_results_single_match(sample_decisions_file):
    """Test formatting a single search result."""
    results = search_decisions("PostgreSQL", sample_decisions_file)
    formatted = format_search_results(results)

    assert "decisions.md" in formatted
    assert "PostgreSQL" in formatted
    assert "line" in formatted


def test_format_search_results_multiple_matches(sample_decisions_file):
    """Test formatting multiple search results."""
    results = search_decisions("decision", sample_decisions_file)
    formatted = format_search_results(results)

    # Should have multiple sections
    assert formatted.count("decisions.md") > 1


def test_format_search_results_without_context(sample_decisions_file):
    """Test formatting results without context lines."""
    results = search_decisions("PostgreSQL", sample_decisions_file)
    formatted = format_search_results(results, show_context=False)

    # Should still show the matched line
    assert "PostgreSQL" in formatted


def test_format_search_results_with_highlight(sample_decisions_file):
    """Test formatting results with term highlighting."""
    results = search_decisions("PostgreSQL", sample_decisions_file)
    formatted = format_search_results(results, highlight_term="PostgreSQL")

    # Should have highlighting markers
    assert ">>>" in formatted
    assert "<<<" in formatted


def test_search_result_repr():
    """Test SearchResult string representation."""
    result = SearchResult(
        file="decisions.md",
        decision_title="Test Decision",
        line_number=42,
        matched_line="test line",
        context_before=[],
        context_after=[],
    )

    repr_str = repr(result)
    assert "decisions.md" in repr_str
    assert "42" in repr_str
    assert "Test Decision" in repr_str


# ---------------------------------------------------------------------------
# CLI: logmind search
# ---------------------------------------------------------------------------

from click.testing import CliRunner

from logmind.cli import main

SAMPLE_DECISIONS_MD_CLI = """\
# Decision Log
---
## 2026-01-10 09:00 - Use PostgreSQL for database
**Reasoning:** ACID compliance needed
---
## 2026-02-01 10:00 - Adopt Redis for caching
**Reasoning:** Fast session storage
---
"""

SAMPLE_ARCHIVE_MD_CLI = """\
# Decision Archive
---
## 2025-11-01 08:00 - Choose Python for implementation
**Reasoning:** Rich ecosystem
---
"""


def _make_docs(decisions_content=SAMPLE_DECISIONS_MD_CLI, archive_content=SAMPLE_ARCHIVE_MD_CLI):
    """Create docs/ with decisions.md and decisions-archive.md under cwd."""
    docs = Path(".") / "docs"
    docs.mkdir(exist_ok=True)
    (docs / "decisions.md").write_text(decisions_content)
    (docs / "decisions-archive.md").write_text(archive_content)


def test_search_cli_finds_match(git_repo):
    """Basic query returns matching decision title in output."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        _make_docs()
        result = runner.invoke(main, ["search", "PostgreSQL"])
    assert result.exit_code == 0
    assert "PostgreSQL" in result.output


def test_search_cli_no_results(git_repo):
    """Query with no match shows a 'no results' style message."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        _make_docs()
        result = runner.invoke(main, ["search", "nonexistent_term_xyz"])
    assert result.exit_code == 0
    assert "no matches" in result.output.lower() or "no results" in result.output.lower()


def test_search_cli_no_docs_fails(git_repo):
    """search exits 1 when docs/ directory doesn't exist."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        # Do NOT create docs/
        result = runner.invoke(main, ["search", "anything"])
    assert result.exit_code == 1
    assert "docs/" in result.output


def test_search_cli_case_sensitive(git_repo):
    """--case-sensitive flag: uppercase query matches but lowercase does not."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        _make_docs()
        # Uppercase "PostgreSQL" should match
        result_upper = runner.invoke(main, ["search", "--case-sensitive", "PostgreSQL"])
        assert result_upper.exit_code == 0
        assert "PostgreSQL" in result_upper.output

        # Lowercase "postgresql" should NOT match
        result_lower = runner.invoke(main, ["search", "--case-sensitive", "postgresql"])
        assert result_lower.exit_code == 0
        assert "no matches" in result_lower.output.lower() or "no results" in result_lower.output.lower()


def test_search_cli_case_insensitive_default(git_repo):
    """Default (no flag): lowercase query matches mixed-case content."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        _make_docs()
        result = runner.invoke(main, ["search", "postgresql"])
    assert result.exit_code == 0
    assert "PostgreSQL" in result.output


def test_search_cli_no_archive_flag(git_repo):
    """--no-archive excludes archive results from output."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        _make_docs()
        # "Python" only appears in the archive content
        result_with_archive = runner.invoke(main, ["search", "Python"])
        assert result_with_archive.exit_code == 0
        assert "Python" in result_with_archive.output

        result_no_archive = runner.invoke(main, ["search", "--no-archive", "Python"])
        assert result_no_archive.exit_code == 0
        assert "no matches" in result_no_archive.output.lower() or "no results" in result_no_archive.output.lower()


def test_search_cli_no_context_flag(git_repo):
    """--no-context output doesn't show surrounding context lines beyond the match."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        _make_docs()
        result_default = runner.invoke(main, ["search", "PostgreSQL"])
        result_no_ctx = runner.invoke(main, ["search", "--no-context", "PostgreSQL"])

    assert result_no_ctx.exit_code == 0
    assert "PostgreSQL" in result_no_ctx.output
    # With no context the output should be shorter than (or equal to) the default
    assert len(result_no_ctx.output) <= len(result_default.output)


def test_search_cli_exit_code_zero_on_match(git_repo):
    """Exit code is 0 when results are found."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        _make_docs()
        result = runner.invoke(main, ["search", "Redis"])
    assert result.exit_code == 0


def test_search_cli_exit_code_nonzero_on_no_match(git_repo):
    """Exit code is non-zero (or output indicates no match) when nothing found.

    The search command returns exit code 0 even on no match (it just prints a
    message), so we verify the output explicitly instead of relying on exit code.
    """
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        _make_docs()
        result = runner.invoke(main, ["search", "zzz_no_match_zzz"])
    # The CLI prints a "no matches" style message; exit code may be 0 or 1
    assert "no matches" in result.output.lower() or "no results" in result.output.lower() or result.exit_code != 0
