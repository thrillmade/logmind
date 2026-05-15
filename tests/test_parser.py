"""Tests for the parser module (core/parser.py)."""

import inspect
from datetime import datetime
from pathlib import Path

import pytest

from logmind.core.parser import DECISION_HEADER, iter_decisions


# ---------------------------------------------------------------------------
# DECISION_HEADER regex tests
# ---------------------------------------------------------------------------


class TestDecisionHeaderRegex:
    def test_matches_valid_header(self):
        assert DECISION_HEADER.match("## 2026-01-10 09:00 - Use PostgreSQL for database")

    def test_matches_minimal_title(self):
        assert DECISION_HEADER.match("## 2026-01-01 00:00 - X")

    def test_matches_title_with_hyphens(self):
        assert DECISION_HEADER.match("## 2026-03-11 12:00 - Use FastAPI -- async support")

    def test_matches_title_with_special_chars(self):
        assert DECISION_HEADER.match("## 2025-12-31 23:59 - Adopt React (v18) & Next.js!")

    def test_no_match_missing_hash_prefix(self):
        assert not DECISION_HEADER.match("# 2026-01-10 09:00 - Only one hash")

    def test_no_match_missing_space_after_hashes(self):
        assert not DECISION_HEADER.match("##2026-01-10 09:00 - No space after hashes")

    def test_no_match_missing_dash_separator(self):
        assert not DECISION_HEADER.match("## 2026-01-10 09:00 Use PostgreSQL")

    def test_no_match_wrong_date_format(self):
        assert not DECISION_HEADER.match("## 26-01-10 09:00 - Short year")

    def test_no_match_wrong_time_format(self):
        assert not DECISION_HEADER.match("## 2026-01-10 9:00 - Single digit hour")

    def test_no_match_extra_leading_whitespace(self):
        assert not DECISION_HEADER.match(" ## 2026-01-10 09:00 - Leading space")

    def test_no_match_empty_string(self):
        assert not DECISION_HEADER.match("")

    def test_captures_date_group(self):
        m = DECISION_HEADER.match("## 2026-01-10 09:00 - Title")
        assert m.group(1) == "2026-01-10"

    def test_captures_time_group(self):
        m = DECISION_HEADER.match("## 2026-01-10 09:00 - Title")
        assert m.group(2) == "09:00"

    def test_captures_title_group(self):
        m = DECISION_HEADER.match("## 2026-01-10 09:00 - Title")
        assert m.group(3) == "Title"


# ---------------------------------------------------------------------------
# iter_decisions tests
# ---------------------------------------------------------------------------


class TestIterDecisionsMissingAndEmpty:
    def test_nonexistent_file_returns_empty(self, tmp_path):
        result = list(iter_decisions(tmp_path / "nonexistent.md"))
        assert result == []

    def test_nonexistent_file_raises_no_exception(self, tmp_path):
        # Consuming the generator must not raise
        list(iter_decisions(tmp_path / "ghost.md"))

    def test_empty_file_returns_empty(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("", encoding="utf-8")
        result = list(iter_decisions(f))
        assert result == []

    def test_prose_only_file_returns_empty(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text(
            "# Decision Log\n\n"
            "Some introductory text.\n"
            "**Reasoning:** We chose this because it is simple.\n"
            "---\n"
        , encoding="utf-8")
        result = list(iter_decisions(f))
        assert result == []


class TestIterDecisionsParsing:
    def test_yields_correct_number_of_decisions(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text(
            "## 2026-01-10 09:00 - First\n"
            "## 2026-01-20 14:30 - Second\n"
            "## 2026-02-05 11:00 - Third\n"
        , encoding="utf-8")
        result = list(iter_decisions(f))
        assert len(result) == 3

    def test_yields_datetime_title_tuples(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2026-01-10 09:00 - Use PostgreSQL\n", encoding="utf-8")
        dt, title = list(iter_decisions(f))[0]
        assert isinstance(dt, datetime)
        assert isinstance(title, str)

    def test_parses_date_correctly(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2026-01-10 09:00 - Title\n", encoding="utf-8")
        dt, _ = list(iter_decisions(f))[0]
        assert dt.year == 2026
        assert dt.month == 1
        assert dt.day == 10

    def test_parses_time_correctly(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2026-01-10 14:30 - Title\n", encoding="utf-8")
        dt, _ = list(iter_decisions(f))[0]
        assert dt.hour == 14
        assert dt.minute == 30

    def test_parses_title_correctly(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2026-01-10 09:00 - Use PostgreSQL for database\n", encoding="utf-8")
        _, title = list(iter_decisions(f))[0]
        assert title == "Use PostgreSQL for database"

    def test_preserves_title_with_hyphens_and_special_chars(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2026-03-11 12:00 - Use FastAPI -- async support\n", encoding="utf-8")
        _, title = list(iter_decisions(f))[0]
        assert title == "Use FastAPI -- async support"

    def test_boundary_date_end_of_year(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2025-12-31 23:59 - End of year\n", encoding="utf-8")
        dt, title = list(iter_decisions(f))[0]
        assert dt == datetime(2025, 12, 31, 23, 59)
        assert title == "End of year"

    def test_boundary_date_start_of_year(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2026-01-01 00:00 - Start of year\n", encoding="utf-8")
        dt, title = list(iter_decisions(f))[0]
        assert dt == datetime(2026, 1, 1, 0, 0)
        assert title == "Start of year"


class TestIterDecisionsFiltering:
    def test_skips_invalid_month_13(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2025-13-01 09:00 - Bad month\n", encoding="utf-8")
        result = list(iter_decisions(f))
        assert result == []

    def test_skips_invalid_day_32(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2025-01-32 09:00 - Bad day\n", encoding="utf-8")
        result = list(iter_decisions(f))
        assert result == []

    def test_skips_line_missing_double_hash(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("# 2026-01-10 09:00 - Single hash\n", encoding="utf-8")
        result = list(iter_decisions(f))
        assert result == []

    def test_skips_line_with_wrong_spacing(self, tmp_path):
        f = tmp_path / "decisions.md"
        # Two spaces between ## and date
        f.write_text("##  2026-01-10 09:00 - Extra space\n", encoding="utf-8")
        result = list(iter_decisions(f))
        assert result == []

    def test_skips_line_missing_dash_separator(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2026-01-10 09:00 Missing separator\n", encoding="utf-8")
        result = list(iter_decisions(f))
        assert result == []

    def test_valid_and_invalid_headers_mixed(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text(
            "## 2025-13-01 09:00 - Invalid month\n"
            "## 2026-01-10 09:00 - Valid entry\n"
            "# 2026-02-01 10:00 - Wrong hash count\n"
        , encoding="utf-8")
        result = list(iter_decisions(f))
        assert len(result) == 1
        assert result[0][1] == "Valid entry"


class TestIterDecisionsMixed:
    def test_mixed_file_yields_only_headers(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text(
            "# Decision Log\n"
            "\n"
            "---\n"
            "\n"
            "## 2026-01-10 09:00 - Use PostgreSQL for database\n"
            "**Reasoning:** ACID compliance needed\n"
            "**Alternatives:** MongoDB, SQLite\n"
            "---\n"
            "\n"
            "## 2026-02-05 11:00 - Use Redis for caching\n"
            "**Reasoning:** Fast session storage\n"
            "---\n"
        , encoding="utf-8")
        result = list(iter_decisions(f))
        assert len(result) == 2

    def test_mixed_file_titles_correct(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text(
            "## 2026-01-10 09:00 - Use PostgreSQL for database\n"
            "**Reasoning:** ACID compliance needed\n"
            "## 2026-02-05 11:00 - Use Redis for caching\n"
            "**Reasoning:** Fast session storage\n"
        , encoding="utf-8")
        _, titles = zip(*iter_decisions(f))
        assert "Use PostgreSQL for database" in titles
        assert "Use Redis for caching" in titles

    def test_mixed_file_prose_not_in_results(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text(
            "## 2026-01-10 09:00 - Real decision\n"
            "This line is just prose and should be ignored.\n"
        , encoding="utf-8")
        result = list(iter_decisions(f))
        assert len(result) == 1


class TestIterDecisionsGenerator:
    def test_returns_generator(self, tmp_path):
        f = tmp_path / "decisions.md"
        f.write_text("## 2026-01-10 09:00 - Title\n", encoding="utf-8")
        result = iter_decisions(f)
        assert inspect.isgenerator(result)

    def test_is_lazy_does_not_read_before_iteration(self, tmp_path):
        # Passing a non-existent path and not iterating must not raise
        gen = iter_decisions(tmp_path / "never.md")
        assert inspect.isgenerator(gen)
