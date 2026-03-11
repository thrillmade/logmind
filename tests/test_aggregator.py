"""Tests for multi-project aggregation."""

import pytest
from datetime import datetime
from pathlib import Path
from click.testing import CliRunner

from logmind.cli import main
from logmind.core.aggregator import (
    AggregatedEntry,
    aggregate_projects,
    load_project_decisions,
    project_summary,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

DECISIONS_A = """\
# Decision Log

---

## 2026-01-10 09:00 - Use PostgreSQL for project-a
**Reasoning:** ACID compliance
---

## 2026-02-01 10:00 - Add Redis cache to project-a
**Reasoning:** Performance
---
"""

DECISIONS_B = """\
# Decision Log

---

## 2026-01-15 11:00 - Use React for project-b
**Reasoning:** Team familiarity
---
"""

ARCHIVE_A = """\
# Decision Archive

---

## 2025-12-01 08:00 - Initialize project-a
**Reasoning:** Starting fresh
---
"""


@pytest.fixture
def project_a(tmp_path):
    """A project with 2 recent and 1 archived decision."""
    proj = tmp_path / "project-a"
    proj.mkdir()
    docs = proj / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text(DECISIONS_A)
    (docs / "decisions-archive.md").write_text(ARCHIVE_A)
    return proj


@pytest.fixture
def project_b(tmp_path):
    """A project with 1 recent decision, no archive."""
    proj = tmp_path / "project-b"
    proj.mkdir()
    docs = proj / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text(DECISIONS_B)
    return proj


@pytest.fixture
def project_no_docs(tmp_path):
    """A project directory with no docs/."""
    proj = tmp_path / "project-no-docs"
    proj.mkdir()
    return proj


# ---------------------------------------------------------------------------
# load_project_decisions tests
# ---------------------------------------------------------------------------


class TestLoadProjectDecisions:
    def test_loads_recent_decisions(self, project_a):
        entries = load_project_decisions(project_a, include_archive=False)
        assert len(entries) == 2

    def test_loads_with_archive(self, project_a):
        entries = load_project_decisions(project_a, include_archive=True)
        assert len(entries) == 3

    def test_project_name_set(self, project_a):
        entries = load_project_decisions(project_a)
        assert all(e.project == "project-a" for e in entries)

    def test_project_path_set(self, project_a):
        entries = load_project_decisions(project_a)
        assert all(e.project_path == project_a for e in entries)

    def test_returns_empty_when_no_docs(self, project_no_docs):
        entries = load_project_decisions(project_no_docs)
        assert entries == []

    def test_handles_missing_archive_file(self, project_b):
        # project_b has no archive file
        entries = load_project_decisions(project_b, include_archive=True)
        assert len(entries) == 1  # only recent

    def test_entry_titles_correct(self, project_a):
        entries = load_project_decisions(project_a, include_archive=False)
        titles = [e.title for e in entries]
        assert "Use PostgreSQL for project-a" in titles
        assert "Add Redis cache to project-a" in titles


# ---------------------------------------------------------------------------
# aggregate_projects tests
# ---------------------------------------------------------------------------


class TestAggregateProjects:
    def test_combines_multiple_projects(self, project_a, project_b):
        entries = aggregate_projects([project_a, project_b], include_archive=False)
        assert len(entries) == 3

    def test_sorted_newest_first(self, project_a, project_b):
        entries = aggregate_projects([project_a, project_b], include_archive=False)
        dates = [e.date for e in entries]
        assert dates == sorted(dates, reverse=True)

    def test_limit_respected(self, project_a, project_b):
        entries = aggregate_projects([project_a, project_b], limit=2)
        assert len(entries) == 2

    def test_limit_gets_newest(self, project_a, project_b):
        entries = aggregate_projects([project_a, project_b], include_archive=False, limit=1)
        assert entries[0].date == datetime(2026, 2, 1, 10, 0)

    def test_include_archive_adds_entries(self, project_a, project_b):
        without = aggregate_projects([project_a, project_b], include_archive=False)
        with_arch = aggregate_projects([project_a, project_b], include_archive=True)
        assert len(with_arch) > len(without)

    def test_skips_projects_without_docs(self, project_a, project_no_docs):
        entries = aggregate_projects([project_a, project_no_docs])
        assert all(e.project == "project-a" for e in entries)

    def test_empty_list_returns_empty(self):
        assert aggregate_projects([]) == []

    def test_project_field_identifies_source(self, project_a, project_b):
        entries = aggregate_projects([project_a, project_b], include_archive=False)
        projects = {e.project for e in entries}
        assert "project-a" in projects
        assert "project-b" in projects


# ---------------------------------------------------------------------------
# project_summary tests
# ---------------------------------------------------------------------------


class TestProjectSummary:
    def test_returns_dict_of_counts(self, project_a, project_b):
        summary = project_summary([project_a, project_b])
        assert summary["project-a"] == 3  # 2 recent + 1 archive
        assert summary["project-b"] == 1

    def test_zero_for_no_docs(self, project_no_docs):
        summary = project_summary([project_no_docs])
        assert summary["project-no-docs"] == 0

    def test_empty_list(self):
        assert project_summary([]) == {}


# ---------------------------------------------------------------------------
# CLI: logmind aggregate
# ---------------------------------------------------------------------------


def test_aggregate_no_args(git_repo):
    """aggregate exits with error when no project paths given."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(main, ["aggregate"])
    assert result.exit_code == 1
    assert "provide at least one" in result.output


def test_aggregate_shows_decisions(tmp_path):
    """aggregate displays decisions from given project."""
    runner = CliRunner()
    proj = tmp_path / "myproject"
    proj.mkdir()
    docs = proj / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text(DECISIONS_A)
    (docs / "decisions-archive.md").write_text("")

    result = runner.invoke(main, ["aggregate", str(proj)])
    assert result.exit_code == 0
    assert "myproject" in result.output
    assert "PostgreSQL" in result.output


def test_aggregate_shows_project_name(tmp_path):
    """aggregate prefixes each decision with project name."""
    runner = CliRunner()
    proj = tmp_path / "cool-service"
    proj.mkdir()
    docs = proj / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text(DECISIONS_A)

    result = runner.invoke(main, ["aggregate", str(proj)])
    assert result.exit_code == 0
    assert "cool-service" in result.output


def test_aggregate_limit_option(tmp_path):
    """aggregate --limit restricts output to N entries."""
    runner = CliRunner()
    proj = tmp_path / "proj"
    proj.mkdir()
    docs = proj / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text(DECISIONS_A)
    (docs / "decisions-archive.md").write_text(ARCHIVE_A)

    result = runner.invoke(main, ["aggregate", "--limit", "1", str(proj)])
    assert result.exit_code == 0
    # Only 1 decision line shown
    project_lines = [l for l in result.output.splitlines() if "proj" in l and "2026" in l]
    assert len(project_lines) == 1


def test_aggregate_summary_flag(tmp_path):
    """aggregate --summary shows per-project counts."""
    runner = CliRunner()
    proj = tmp_path / "myapp"
    proj.mkdir()
    docs = proj / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text(DECISIONS_A)
    (docs / "decisions-archive.md").write_text(ARCHIVE_A)

    result = runner.invoke(main, ["aggregate", "--summary", str(proj)])
    assert result.exit_code == 0
    assert "myapp" in result.output
    assert "3" in result.output  # 2 recent + 1 archive


def test_aggregate_no_archive_flag(tmp_path):
    """aggregate --no-archive excludes archived decisions."""
    runner = CliRunner()
    proj = tmp_path / "proj"
    proj.mkdir()
    docs = proj / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text(DECISIONS_A)
    (docs / "decisions-archive.md").write_text(ARCHIVE_A)

    result_with = runner.invoke(main, ["aggregate", str(proj)])
    result_without = runner.invoke(main, ["aggregate", "--no-archive", str(proj)])

    # Without archive should have fewer lines
    lines_with = [l for l in result_with.output.splitlines() if "proj" in l]
    lines_without = [l for l in result_without.output.splitlines() if "proj" in l]
    assert len(lines_without) < len(lines_with)


def test_aggregate_skips_missing_docs(tmp_path):
    """aggregate warns and skips projects without docs/."""
    runner = CliRunner()
    proj_valid = tmp_path / "valid"
    proj_valid.mkdir()
    (proj_valid / "docs").mkdir()
    (proj_valid / "docs" / "decisions.md").write_text(DECISIONS_A)

    proj_invalid = tmp_path / "invalid"
    proj_invalid.mkdir()

    result = runner.invoke(main, ["aggregate", str(proj_valid), str(proj_invalid)])
    assert result.exit_code == 0
    assert "Warning" in result.output
    assert "valid" in result.output


def test_aggregate_multiple_projects(tmp_path):
    """aggregate combines decisions from multiple projects."""
    runner = CliRunner()
    proj_a = tmp_path / "project-a"
    proj_a.mkdir()
    (proj_a / "docs").mkdir()
    (proj_a / "docs" / "decisions.md").write_text(DECISIONS_A)

    proj_b = tmp_path / "project-b"
    proj_b.mkdir()
    (proj_b / "docs").mkdir()
    (proj_b / "docs" / "decisions.md").write_text(DECISIONS_B)

    result = runner.invoke(main, ["aggregate", str(proj_a), str(proj_b)])
    assert result.exit_code == 0
    assert "project-a" in result.output
    assert "project-b" in result.output
