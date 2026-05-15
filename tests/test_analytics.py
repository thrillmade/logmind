"""Tests for analytics module and logmind stats command."""

import pytest
from datetime import datetime
from pathlib import Path
from click.testing import CliRunner

from logmind.cli import main
from logmind.core.analytics import (
    DecisionEntry,
    ascii_bar_chart,
    compute_stats,
    decisions_by_month,
    parse_decisions,
    top_keywords,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

SAMPLE_DECISIONS_MD = """\
# Decision Log

---

## 2026-01-10 09:00 - Use PostgreSQL for database
**Reasoning:** ACID compliance needed
---

## 2026-01-20 14:30 - Adopt React for frontend
**Reasoning:** Team familiarity
---

## 2026-02-05 11:00 - Use Redis for caching
**Reasoning:** Fast session storage
---
"""

SAMPLE_ARCHIVE_MD = """\
# Decision Archive

---

## 2025-11-15 10:00 - Initialize logmind decision tracking
**Reasoning:** Structured logging
---

## 2025-12-01 08:00 - Choose Python for implementation
**Reasoning:** Rich ecosystem
---
"""


@pytest.fixture
def docs_with_decisions(tmp_path):
    """Create a docs dir with sample decisions and archive."""
    docs = tmp_path / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text(SAMPLE_DECISIONS_MD, encoding="utf-8")
    (docs / "decisions-archive.md").write_text(SAMPLE_ARCHIVE_MD, encoding="utf-8")
    return docs


@pytest.fixture
def empty_docs(tmp_path):
    """Create a docs dir with empty decision files."""
    docs = tmp_path / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n", encoding="utf-8")
    (docs / "decisions-archive.md").write_text("# Decision Archive\n\n---\n", encoding="utf-8")
    return docs


# ---------------------------------------------------------------------------
# parse_decisions tests
# ---------------------------------------------------------------------------


class TestParseDecisions:
    def test_parses_recent_decisions(self, docs_with_decisions):
        entries = parse_decisions(docs_with_decisions, include_archive=False)
        assert len(entries) == 3

    def test_parses_archive_decisions(self, docs_with_decisions):
        entries = parse_decisions(docs_with_decisions, include_archive=True)
        assert len(entries) == 5

    def test_excludes_archive_when_flag_false(self, docs_with_decisions):
        entries = parse_decisions(docs_with_decisions, include_archive=False)
        assert all(e.source == "recent" for e in entries)

    def test_entries_sorted_by_date(self, docs_with_decisions):
        entries = parse_decisions(docs_with_decisions)
        dates = [e.date for e in entries]
        assert dates == sorted(dates)

    def test_entry_title_parsed(self, docs_with_decisions):
        entries = parse_decisions(docs_with_decisions, include_archive=False)
        titles = [e.title for e in entries]
        assert "Use PostgreSQL for database" in titles
        assert "Adopt React for frontend" in titles

    def test_entry_date_parsed(self, docs_with_decisions):
        entries = parse_decisions(docs_with_decisions, include_archive=False)
        jan_entries = [e for e in entries if e.date.month == 1]
        assert len(jan_entries) == 2

    def test_returns_empty_for_no_decisions(self, empty_docs):
        entries = parse_decisions(empty_docs)
        assert entries == []

    def test_handles_missing_decisions_file(self, tmp_path):
        docs = tmp_path / "docs"
        docs.mkdir()
        entries = parse_decisions(docs)
        assert entries == []

    def test_source_field_set_correctly(self, docs_with_decisions):
        entries = parse_decisions(docs_with_decisions)
        recent = [e for e in entries if e.source == "recent"]
        archive = [e for e in entries if e.source == "archive"]
        assert len(recent) == 3
        assert len(archive) == 2


# ---------------------------------------------------------------------------
# decisions_by_month tests
# ---------------------------------------------------------------------------


class TestDecisionsByMonth:
    def _make_entry(self, year, month, day, title="Test"):
        return DecisionEntry(
            date=datetime(year, month, day, 10, 0),
            title=title,
            source="recent",
        )

    def test_groups_by_month(self):
        entries = [
            self._make_entry(2026, 1, 10),
            self._make_entry(2026, 1, 20),
            self._make_entry(2026, 2, 5),
        ]
        by_month = decisions_by_month(entries)
        assert by_month["2026-01"] == 2
        assert by_month["2026-02"] == 1

    def test_empty_returns_empty(self):
        assert decisions_by_month([]) == {}

    def test_keys_are_yyyy_mm_format(self):
        entries = [self._make_entry(2026, 3, 1)]
        by_month = decisions_by_month(entries)
        assert "2026-03" in by_month


# ---------------------------------------------------------------------------
# top_keywords tests
# ---------------------------------------------------------------------------


class TestTopKeywords:
    def _make_entry(self, title):
        return DecisionEntry(
            date=datetime(2026, 1, 1, 10, 0), title=title, source="recent"
        )

    def test_returns_top_n(self):
        entries = [
            self._make_entry("database postgres migration"),
            self._make_entry("database schema postgres"),
            self._make_entry("postgres configuration"),
        ]
        keywords = top_keywords(entries, n=3)
        words = [kw for kw, _ in keywords]
        assert "postgres" in words
        assert "database" in words

    def test_filters_stop_words(self):
        entries = [self._make_entry("use the api for the endpoint")]
        keywords = top_keywords(entries)
        words = [kw for kw, _ in keywords]
        assert "the" not in words
        assert "for" not in words
        assert "use" not in words

    def test_returns_empty_for_no_entries(self):
        assert top_keywords([]) == []

    def test_counts_are_descending(self):
        entries = [
            self._make_entry("postgres postgres postgres"),
            self._make_entry("redis redis"),
            self._make_entry("mongo"),
        ]
        keywords = top_keywords(entries)
        counts = [c for _, c in keywords]
        assert counts == sorted(counts, reverse=True)

    def test_case_insensitive(self):
        entries = [
            self._make_entry("PostgreSQL database"),
            self._make_entry("postgresql schema"),
        ]
        keywords = top_keywords(entries)
        words = [kw for kw, _ in keywords]
        assert "postgresql" in words


# ---------------------------------------------------------------------------
# ascii_bar_chart tests
# ---------------------------------------------------------------------------


class TestAsciiBarChart:
    def test_returns_string(self):
        result = ascii_bar_chart({"2026-01": 5})
        assert isinstance(result, str)

    def test_contains_labels(self):
        result = ascii_bar_chart({"2026-01": 5, "2026-02": 3})
        assert "2026-01" in result
        assert "2026-02" in result

    def test_contains_counts(self):
        result = ascii_bar_chart({"2026-01": 5})
        assert "5" in result

    def test_empty_returns_no_data(self):
        result = ascii_bar_chart({})
        assert "no data" in result.lower()

    def test_max_value_gets_full_bar(self):
        result = ascii_bar_chart({"month": 10}, width=10)
        assert "██████████" in result

    def test_bar_uses_block_chars(self):
        result = ascii_bar_chart({"month": 5})
        assert "█" in result


# ---------------------------------------------------------------------------
# compute_stats tests
# ---------------------------------------------------------------------------


class TestComputeStats:
    def test_total_count(self, docs_with_decisions):
        stats = compute_stats(docs_with_decisions)
        assert stats["total"] == 5

    def test_recent_count(self, docs_with_decisions):
        stats = compute_stats(docs_with_decisions)
        assert stats["recent_count"] == 3

    def test_archive_count(self, docs_with_decisions):
        stats = compute_stats(docs_with_decisions)
        assert stats["archive_count"] == 2

    def test_by_month_populated(self, docs_with_decisions):
        stats = compute_stats(docs_with_decisions)
        assert "2026-01" in stats["by_month"]
        assert stats["by_month"]["2026-01"] == 2

    def test_keywords_populated(self, docs_with_decisions):
        stats = compute_stats(docs_with_decisions)
        assert len(stats["keywords"]) > 0

    def test_most_active_month(self, docs_with_decisions):
        stats = compute_stats(docs_with_decisions)
        assert stats["most_active_month"] == "2026-01"
        assert stats["most_active_count"] == 2

    def test_empty_docs(self, empty_docs):
        stats = compute_stats(empty_docs)
        assert stats["total"] == 0

    def test_velocity_keys_present(self, docs_with_decisions):
        stats = compute_stats(docs_with_decisions)
        assert "velocity_30" in stats
        assert "velocity_prior_30" in stats


# ---------------------------------------------------------------------------
# CLI: logmind stats
# ---------------------------------------------------------------------------


def test_stats_command_no_docs(git_repo):
    """stats exits with error when docs/ doesn't exist."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(main, ["stats"])
    assert result.exit_code == 1
    assert "docs/" in result.output


def test_stats_command_no_decisions(git_repo):
    """stats shows message when no decisions have been logged."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        docs = Path(".") / "docs"
        docs.mkdir()
        (docs / "decisions.md").write_text("# Decision Log\n\n---\n", encoding="utf-8")
        (docs / "decisions-archive.md").write_text("# Decision Archive\n\n---\n", encoding="utf-8")
        result = runner.invoke(main, ["stats"])
    assert result.exit_code == 0
    assert "No decisions" in result.output


def test_stats_command_shows_total(git_repo):
    """stats displays total decision count."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        docs = Path(".") / "docs"
        docs.mkdir()
        (docs / "decisions.md").write_text(SAMPLE_DECISIONS_MD, encoding="utf-8")
        (docs / "decisions-archive.md").write_text(SAMPLE_ARCHIVE_MD, encoding="utf-8")
        result = runner.invoke(main, ["stats"])
    assert result.exit_code == 0
    assert "5" in result.output
    assert "Total" in result.output


def test_stats_command_shows_chart(git_repo):
    """stats displays the monthly activity chart."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        docs = Path(".") / "docs"
        docs.mkdir()
        (docs / "decisions.md").write_text(SAMPLE_DECISIONS_MD, encoding="utf-8")
        (docs / "decisions-archive.md").write_text(SAMPLE_ARCHIVE_MD, encoding="utf-8")
        result = runner.invoke(main, ["stats"])
    assert result.exit_code == 0
    assert "█" in result.output


def test_stats_command_shows_keywords(git_repo):
    """stats displays top keywords section."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        docs = Path(".") / "docs"
        docs.mkdir()
        (docs / "decisions.md").write_text(SAMPLE_DECISIONS_MD, encoding="utf-8")
        (docs / "decisions-archive.md").write_text(SAMPLE_ARCHIVE_MD, encoding="utf-8")
        result = runner.invoke(main, ["stats"])
    assert result.exit_code == 0
    assert "keyword" in result.output.lower()


def test_stats_command_months_option(git_repo):
    """stats --months limits the chart to N months."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        docs = Path(".") / "docs"
        docs.mkdir()
        (docs / "decisions.md").write_text(SAMPLE_DECISIONS_MD, encoding="utf-8")
        (docs / "decisions-archive.md").write_text(SAMPLE_ARCHIVE_MD, encoding="utf-8")
        result = runner.invoke(main, ["stats", "--months", "1"])
    assert result.exit_code == 0
