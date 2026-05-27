"""Tests for the v0.2 derived-file timeline (replaces per-merge aggregator)."""

from __future__ import annotations

import subprocess
from datetime import datetime
from pathlib import Path

import pytest
from click.testing import CliRunner

from logmind.cli import main as cli_main
from logmind.core.timeline import (
    TimelineEntry,
    _branch_label_from_filename,
    collect_entries,
    generate_timeline,
    render_markdown,
    write_timeline,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _seed_docs(tmp_path: Path) -> Path:
    """Build a representative docs/ layout the timeline should walk."""
    docs = tmp_path / "docs"
    branches = docs / "decisions-branches"
    branches.mkdir(parents=True)

    (docs / "decisions.md").write_text(
        "# Decision Log\n\n---\n"
        "## 2026-05-15 10:00 - direct-on-main decision\n\n"
        "**Reasoning:** explicit\n\n---\n",
        encoding="utf-8",
    )
    (docs / "decisions-archive.md").write_text(
        "# Decision Archive\n\n---\n"
        "## 2025-01-01 09:00 - ancient decision\n\n---\n",
        encoding="utf-8",
    )
    (branches / "feat__auth.md").write_text(
        "# feat/auth\n\n---\n"
        "## 2026-05-10 14:00 - JWT for stateless API auth\n\n"
        "**Reasoning:** horizontal scale\n\n---\n",
        encoding="utf-8",
    )
    (branches / "fix__bug.md").write_text(
        "# fix/bug\n\n---\n"
        "## 2026-05-12 16:30 - clear cache on logout\n\n"
        "**Reasoning:** stale tokens\n\n---\n",
        encoding="utf-8",
    )
    return docs


# ---------------------------------------------------------------------------
# Branch label inversion
# ---------------------------------------------------------------------------


def test_branch_label_from_filename_recovers_slashes():
    assert _branch_label_from_filename("feat__auth.md") == "feat/auth"
    assert _branch_label_from_filename("fix__bug-123.md") == "fix/bug-123"
    assert _branch_label_from_filename("main.md") == "main"


# ---------------------------------------------------------------------------
# Collection
# ---------------------------------------------------------------------------


def test_collect_entries_walks_all_sources(tmp_path):
    docs = _seed_docs(tmp_path)
    entries = collect_entries(docs)

    titles = [e.title for e in entries]
    assert "direct-on-main decision" in titles
    assert "ancient decision" in titles
    assert "JWT for stateless API auth" in titles
    assert "clear cache on logout" in titles


def test_collect_entries_sorted_newest_first(tmp_path):
    docs = _seed_docs(tmp_path)
    entries = collect_entries(docs)
    dates = [e.date for e in entries]
    assert dates == sorted(dates, reverse=True)


def test_collect_entries_labels_sources_correctly(tmp_path):
    docs = _seed_docs(tmp_path)
    entries = collect_entries(docs)
    labels = {e.title: e.source_label for e in entries}
    assert labels["direct-on-main decision"] == "main"
    assert labels["ancient decision"] == "archive"
    assert labels["JWT for stateless API auth"] == "feat/auth"
    assert labels["clear cache on logout"] == "fix/bug"


def test_collect_entries_tolerates_empty_docs(tmp_path):
    (tmp_path / "docs").mkdir()
    assert collect_entries(tmp_path / "docs") == []


def test_collect_entries_tolerates_missing_branches_dir(tmp_path):
    docs = tmp_path / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text(
        "# Decision Log\n\n---\n## 2026-05-15 10:00 - only main\n\n---\n",
        encoding="utf-8",
    )
    entries = collect_entries(docs)
    assert len(entries) == 1
    assert entries[0].source_label == "main"


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------


def test_render_markdown_groups_by_month(tmp_path):
    docs = _seed_docs(tmp_path)
    rendered = render_markdown(collect_entries(docs))
    # 2026-05 should appear once for the three May entries
    assert rendered.count("## 2026-05") == 1
    # 2025-01 for the archive entry
    assert rendered.count("## 2025-01") == 1
    # All four titles appear
    for title in (
        "direct-on-main decision",
        "ancient decision",
        "JWT for stateless API auth",
        "clear cache on logout",
    ):
        assert title in rendered


def test_render_markdown_includes_source_label(tmp_path):
    docs = _seed_docs(tmp_path)
    rendered = render_markdown(collect_entries(docs))
    assert "*(main)*" in rendered
    assert "*(feat/auth)*" in rendered
    assert "*(archive)*" in rendered


def test_render_markdown_empty_returns_header_only():
    rendered = render_markdown([])
    assert "Decision Timeline" in rendered
    assert "no decisions logged yet" in rendered


def test_render_markdown_is_deterministic(tmp_path):
    """Same inputs must produce byte-identical output (load-bearing for the
    derived-file architecture — two PRs with the same merged state must
    regenerate to the same docs/timeline.md so no merge conflict is possible)."""
    docs = _seed_docs(tmp_path)
    a = render_markdown(collect_entries(docs))
    b = render_markdown(collect_entries(docs))
    assert a == b


# ---------------------------------------------------------------------------
# write_timeline
# ---------------------------------------------------------------------------


def test_write_timeline_creates_file(tmp_path):
    docs = _seed_docs(tmp_path)
    target = tmp_path / "docs" / "timeline.md"
    assert not target.exists()
    changed = write_timeline(target, docs)
    assert changed is True
    assert target.exists()
    assert "Decision Timeline" in target.read_text(encoding="utf-8")


def test_write_timeline_returns_false_when_unchanged(tmp_path):
    docs = _seed_docs(tmp_path)
    target = tmp_path / "docs" / "timeline.md"
    write_timeline(target, docs)
    # Second call: file already matches, no change
    assert write_timeline(target, docs) is False


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def test_timeline_cli_stdout(tmp_path, monkeypatch):
    docs = _seed_docs(tmp_path)
    monkeypatch.chdir(tmp_path)
    runner = CliRunner()
    result = runner.invoke(cli_main, ["timeline"])
    assert result.exit_code == 0
    assert "Decision Timeline" in result.output
    assert "JWT for stateless API auth" in result.output


def test_timeline_cli_write(tmp_path, monkeypatch):
    docs = _seed_docs(tmp_path)
    monkeypatch.chdir(tmp_path)
    target = docs / "timeline.md"
    runner = CliRunner()
    result = runner.invoke(cli_main, ["timeline", "--write", str(target)])
    assert result.exit_code == 0
    assert target.exists()
    assert "Decision Timeline" in target.read_text(encoding="utf-8")


def test_timeline_cli_check_fails_when_stale(tmp_path, monkeypatch):
    docs = _seed_docs(tmp_path)
    monkeypatch.chdir(tmp_path)
    target = docs / "timeline.md"
    target.write_text("stale\n", encoding="utf-8")
    runner = CliRunner()
    result = runner.invoke(
        cli_main, ["timeline", "--write", str(target), "--check"]
    )
    assert result.exit_code == 1
    assert "stale" in result.output.lower() or "re-run" in result.output.lower()
    # Regression: the remediation hint must interpolate the actual path,
    # not print literal `{write_path}` (caught the f-string concat bug
    # flagged in clud-bug review of PR #36).
    assert str(target) in result.output
    assert "{write_path}" not in result.output


def test_timeline_cli_check_passes_when_fresh(tmp_path, monkeypatch):
    docs = _seed_docs(tmp_path)
    monkeypatch.chdir(tmp_path)
    target = docs / "timeline.md"
    # Pre-generate; --check should be green
    runner = CliRunner()
    runner.invoke(cli_main, ["timeline", "--write", str(target)])
    result = runner.invoke(
        cli_main, ["timeline", "--write", str(target), "--check"]
    )
    assert result.exit_code == 0
    assert "up to date" in result.output


def test_timeline_cli_check_requires_write_path(tmp_path, monkeypatch):
    docs = _seed_docs(tmp_path)
    monkeypatch.chdir(tmp_path)
    runner = CliRunner()
    result = runner.invoke(cli_main, ["timeline", "--check"])
    assert result.exit_code == 2
    assert "requires --write" in result.output.lower()


# ---------------------------------------------------------------------------
# Regression: derived-file architecture (no aggregator workflow)
# ---------------------------------------------------------------------------


def test_aggregator_template_no_longer_shipped():
    """v0.2 deletes the per-merge aggregator workflow template."""
    template_root = (
        Path(__file__).parent.parent / "src" / "logmind" / "templates" / "github"
    )
    assert not (template_root / "logmind-aggregate.yml.template").exists()
    # And the new regen workflow IS shipped
    assert (template_root / "regen-timeline.yml.template").exists()


def test_regen_timeline_template_supports_auto_fix_with_failfast_fallback():
    """v0.3.4 design: the workflow auto-fixes when LOGMIND_AUTO_REGEN_PAT is
    configured (pushes a regenerated derived-doc commit back to the PR branch
    so downstream CI re-runs naturally), and falls back to v0.2's fail-fast
    behavior when no PAT is configured. The fail-fast fallback exists because
    GITHUB_TOKEN-pushed commits don't re-trigger required status checks,
    which would leave the merge gate stuck on "Expected" forever — so the
    PAT path is the documented opt-in for the happy-path UX. Forked PRs
    always run in fail-fast mode (can't push to a fork's head ref)."""
    template = (
        Path(__file__).parent.parent
        / "src"
        / "logmind"
        / "templates"
        / "github"
        / "regen-timeline.yml.template"
    ).read_text(encoding="utf-8")
    # Auto-fix path: opt-in via LOGMIND_AUTO_REGEN_PAT. The legacy
    # LOGMIND_BOT_PAT name from the v0.1.x aggregator is NOT reused.
    assert "secrets.LOGMIND_BOT_PAT" not in template
    assert "secrets.LOGMIND_AUTO_REGEN_PAT" in template
    # The push back to the PR branch IS shipped now (auto-fix mode).
    assert "git push origin" in template
    # But the push must be gated — fork PRs and no-PAT cases fail fast,
    # never auto-commit via GITHUB_TOKEN. The gate is the explicit empty
    # PAT check.
    assert '[ -z "${PAT:-}" ]' in template
    # Fail-fast fallback must still surface a clear ::error:: and guide
    # the user to the regenerate-locally command.
    assert "::error" in template
    assert "logmind timeline --write docs/timeline.md" in template
    # Fork PRs always fall back to fail-fast (the head repo != base repo
    # branch in the env block proves the workflow is checking).
    assert "HEAD_REPO" in template and "BASE_REPO" in template
