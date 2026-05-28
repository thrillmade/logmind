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
    # Use brief=False so every entry is exercised (v0.5.4+ default is
    # brief, which elides the middle entry of a 3-entry month).
    rendered = render_markdown(collect_entries(docs), brief=False)
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
# v0.5.4 — brief format on disk (Phase 0.B.4)
# ---------------------------------------------------------------------------


def _seed_docs_with_many_decisions(tmp_path: Path, may_count: int = 5) -> Path:
    """Build a docs/ layout with `may_count` decisions in 2026-05 plus
    one entry in 2025-01 so brief mode has something to compress and
    a separate month with a single entry."""
    docs = tmp_path / "docs"
    (docs).mkdir(parents=True, exist_ok=True)

    lines = ["# Decision Log\n", ""]
    # Newest day first; entries inside same month so brief can elide.
    for day in range(may_count, 0, -1):
        lines.append(f"## 2026-05-{day:02d} 10:00 - may entry {day}\n")
        lines.append("**Reasoning:** test\n\n---\n")
    (docs / "decisions.md").write_text("\n".join(lines), encoding="utf-8")

    (docs / "decisions-archive.md").write_text(
        "# Decision Archive\n\n---\n"
        "## 2025-01-15 09:00 - lone ancient decision\n\n---\n",
        encoding="utf-8",
    )
    return docs


def test_render_markdown_brief_is_default_v054_plus(tmp_path):
    """v0.5.4: render_markdown() defaults to brief=True."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    default = render_markdown(collect_entries(docs))
    explicit_brief = render_markdown(collect_entries(docs), brief=True)
    assert default == explicit_brief


def test_render_markdown_brief_shorter_than_full(tmp_path):
    """Brief mode strictly fewer bytes than full mode when months have
    ≥3 decisions to elide."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    entries = collect_entries(docs)
    brief = render_markdown(entries, brief=True)
    full = render_markdown(entries, brief=False)
    assert len(brief) < len(full), (
        f"brief ({len(brief)}) must be strictly shorter than full ({len(full)})"
    )


def test_render_markdown_brief_header_carries_month_count(tmp_path):
    """Brief mode's month heading includes the decision count so
    readers know how much was elided."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    rendered = render_markdown(collect_entries(docs), brief=True)
    # 5 May entries → "## 2026-05 (5 decisions)"
    assert "## 2026-05 (5 decisions)" in rendered


def test_render_markdown_brief_shows_first_and_last_when_3plus(tmp_path):
    """≥3-entry month renders newest + oldest with elision line between."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    rendered = render_markdown(collect_entries(docs), brief=True)
    # Newest May entry (day 05) appears.
    assert "may entry 5" in rendered
    # Oldest May entry (day 01) appears.
    assert "may entry 1" in rendered
    # Elision line for the 3 middle entries.
    assert "*... 3 more decisions ...*" in rendered
    # Middle entries (days 02-04) are NOT in brief output.
    assert "may entry 2" not in rendered
    assert "may entry 3" not in rendered
    assert "may entry 4" not in rendered


def test_render_markdown_brief_shows_all_when_2_or_fewer(tmp_path):
    """≤2-entry months show every entry verbatim (nothing to elide)."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    rendered = render_markdown(collect_entries(docs), brief=True)
    # 2025-01 has exactly 1 entry → no count suffix, no elision line.
    assert "## 2025-01\n" in rendered or "## 2025-01 " not in rendered.replace(
        "## 2025-01 (", ""
    )
    assert "lone ancient decision" in rendered


def test_render_markdown_full_matches_legacy_format(tmp_path):
    """Full mode reproduces the pre-v0.5.4 per-decision listing exactly
    — every entry shown, no count in header, no elision line."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    full = render_markdown(collect_entries(docs), brief=False)
    # All 5 May entries present.
    for n in range(1, 6):
        assert f"may entry {n}" in full
    # No count suffix in header.
    assert "## 2026-05\n" in full
    assert "(5 decisions)" not in full
    # No elision line.
    assert "more decisions ...*" not in full


def test_render_markdown_brief_still_deterministic(tmp_path):
    """Brief mode preserves the deterministic-rendering invariant."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    a = render_markdown(collect_entries(docs), brief=True)
    b = render_markdown(collect_entries(docs), brief=True)
    assert a == b


def test_write_timeline_default_is_brief(tmp_path):
    """write_timeline() writes brief by default — every consuming repo
    sees a smaller docs/timeline.md after upgrading to v0.5.4."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    target = tmp_path / "docs" / "timeline.md"
    write_timeline(target, docs)
    written = target.read_text(encoding="utf-8")
    assert "## 2026-05 (5 decisions)" in written
    assert "*... 3 more decisions ...*" in written


def test_write_timeline_full_opt_in(tmp_path):
    """write_timeline(brief=False) emits the legacy per-decision listing."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    target = tmp_path / "docs" / "timeline.md"
    write_timeline(target, docs, brief=False)
    written = target.read_text(encoding="utf-8")
    assert "(5 decisions)" not in written
    assert "more decisions ...*" not in written
    # Every entry present.
    for n in range(1, 6):
        assert f"may entry {n}" in written


def test_timeline_cli_full_flag_uses_legacy_format(tmp_path, monkeypatch):
    """`logmind timeline --full` renders legacy per-decision listing on stdout."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    # Need a git repo for cwd context; CLI checks Path.cwd()/docs.
    monkeypatch.chdir(tmp_path)
    runner = CliRunner()
    result = runner.invoke(cli_main, ["timeline", "--full"])
    assert result.exit_code == 0, result.output
    # Every entry present, no count or elision.
    for n in range(1, 6):
        assert f"may entry {n}" in result.output
    assert "(5 decisions)" not in result.output
    assert "more decisions ...*" not in result.output


def test_timeline_cli_default_is_brief(tmp_path, monkeypatch):
    """`logmind timeline` (no flag) renders brief by default."""
    docs = _seed_docs_with_many_decisions(tmp_path, may_count=5)
    monkeypatch.chdir(tmp_path)
    runner = CliRunner()
    result = runner.invoke(cli_main, ["timeline"])
    assert result.exit_code == 0, result.output
    assert "## 2026-05 (5 decisions)" in result.output
    assert "*... 3 more decisions ...*" in result.output


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
