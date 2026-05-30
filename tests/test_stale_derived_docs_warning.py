"""Tests for v0.5.13 stale-derived-docs heads-up warning in `logmind doctor`.

When the current branch is behind ``origin/<default-branch>`` AND the gap
touches ``docs/timeline.md`` or ``docs/file-structure.md``, doctor surfaces
a heads-up warning predicting the trailing-PR DIRTY scenario reported by
the tokenomics agent (2026-05-30).
"""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from logmind.core.doctor import (
    check_stale_derived_docs_warning,
    collect_status,
)


def _git_init_with_remote(local: Path, remote: Path) -> None:
    """Set up a bare remote + local clone with an initial commit on main."""
    subprocess.run(["git", "init", "--bare", "-b", "main"], cwd=remote, check=True, capture_output=True)
    subprocess.run(["git", "init", "-b", "main"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.email", "t@t.com"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=local, check=True, capture_output=True)
    (local / "README.md").write_text("seed\n", encoding="utf-8")
    subprocess.run(["git", "add", "README.md"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "init"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "remote", "add", "origin", str(remote)], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "-u", "origin", "main"], cwd=local, check=True, capture_output=True)


def test_warning_silent_outside_git_repo(tmp_path):
    """No `.git/` dir → silent no-op."""
    assert check_stale_derived_docs_warning(tmp_path) is None


def test_warning_silent_on_default_branch(tmp_path):
    """On main itself, there's no upstream-vs-feature divergence to warn about."""
    subprocess.run(["git", "init", "-b", "main"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.email", "t@t.com"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=tmp_path, check=True, capture_output=True)
    (tmp_path / "x.txt").write_text("x", encoding="utf-8")
    subprocess.run(["git", "add", "x.txt"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "init"], cwd=tmp_path, check=True, capture_output=True)
    assert check_stale_derived_docs_warning(tmp_path) is None


def test_warning_silent_when_no_upstream(tmp_path):
    """Repo with no `origin` remote → silent no-op."""
    subprocess.run(["git", "init", "-b", "main"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.email", "t@t.com"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=tmp_path, check=True, capture_output=True)
    (tmp_path / "x.txt").write_text("x", encoding="utf-8")
    subprocess.run(["git", "add", "x.txt"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "init"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "checkout", "-b", "feature"], cwd=tmp_path, check=True, capture_output=True)
    assert check_stale_derived_docs_warning(tmp_path) is None


def test_warning_silent_when_branch_is_up_to_date(tmp_path):
    """Feature branch with origin/main reachable + no derived-doc changes
    on main → silent."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    subprocess.run(["git", "checkout", "-b", "feature"], cwd=local, check=True, capture_output=True)
    # Fetch origin so origin/main ref exists
    subprocess.run(["git", "fetch", "origin"], cwd=local, check=True, capture_output=True)
    assert check_stale_derived_docs_warning(local) is None


def test_warning_silent_when_gap_does_not_touch_derived_docs(tmp_path):
    """Main moves ahead but only touches code → silent (gap doesn't
    intersect with the derived-doc set)."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    subprocess.run(["git", "checkout", "-b", "feature"], cwd=local, check=True, capture_output=True)
    # Switch back to main, add a code-only commit, push
    subprocess.run(["git", "checkout", "main"], cwd=local, check=True, capture_output=True)
    (local / "main.py").write_text("print('x')\n", encoding="utf-8")
    subprocess.run(["git", "add", "main.py"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "add code"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "origin", "main"], cwd=local, check=True, capture_output=True)
    # Back to feature; fetch
    subprocess.run(["git", "checkout", "feature"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "fetch", "origin"], cwd=local, check=True, capture_output=True)
    assert check_stale_derived_docs_warning(local) is None


def test_warning_fires_when_gap_touches_timeline_md(tmp_path):
    """Main moves ahead with a docs/timeline.md change → warning fires."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    subprocess.run(["git", "checkout", "-b", "feature"], cwd=local, check=True, capture_output=True)
    # Switch back to main, add a docs/timeline.md commit, push
    subprocess.run(["git", "checkout", "main"], cwd=local, check=True, capture_output=True)
    docs = local / "docs"
    docs.mkdir()
    (docs / "timeline.md").write_text("# timeline\n- entry\n", encoding="utf-8")
    subprocess.run(["git", "add", "docs/timeline.md"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "regen timeline"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "origin", "main"], cwd=local, check=True, capture_output=True)
    # Back to feature; fetch + check warning
    subprocess.run(["git", "checkout", "feature"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "fetch", "origin"], cwd=local, check=True, capture_output=True)

    warning = check_stale_derived_docs_warning(local)
    assert warning is not None, "warning must fire when gap touches docs/timeline.md"
    assert "feature" in warning
    assert "docs/timeline.md" in warning
    assert "logmind rebase" in warning
    assert "DIRTY" in warning or "behind" in warning


def test_warning_fires_for_file_structure_md(tmp_path):
    """Mirror test for docs/file-structure.md — both derived docs trip the warning."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    subprocess.run(["git", "checkout", "-b", "feature"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "checkout", "main"], cwd=local, check=True, capture_output=True)
    docs = local / "docs"
    docs.mkdir()
    (docs / "file-structure.md").write_text("# tree\n", encoding="utf-8")
    subprocess.run(["git", "add", "docs/file-structure.md"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "regen tree"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "origin", "main"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "checkout", "feature"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "fetch", "origin"], cwd=local, check=True, capture_output=True)

    warning = check_stale_derived_docs_warning(local)
    assert warning is not None
    assert "docs/file-structure.md" in warning


def test_doctor_collect_status_surfaces_warning_as_suggestion(tmp_path, monkeypatch):
    """End-to-end: the warning lands in StatusReport.suggestions when fired,
    so `logmind doctor` (CLI) renders it in the suggestions block."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    subprocess.run(["git", "checkout", "-b", "feature"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "checkout", "main"], cwd=local, check=True, capture_output=True)
    docs = local / "docs"
    docs.mkdir()
    (docs / "timeline.md").write_text("# timeline\n", encoding="utf-8")
    subprocess.run(["git", "add", "docs/timeline.md"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "regen"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "origin", "main"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "checkout", "feature"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "fetch", "origin"], cwd=local, check=True, capture_output=True)

    report = collect_status(local, offline=True)
    # The warning shows up in suggestions, NOT as overall DRIFT
    # (predictive heads-up, not a current failure).
    assert any("docs/timeline.md" in s for s in report.suggestions), (
        f"warning must be in suggestions list. Got: {report.suggestions}"
    )
