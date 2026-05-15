"""Regression tests for the v0.1.3 structural fixes.

Two bugs resolved here:

1. `logmind log` on a feature branch regenerated docs/file-structure.md
   against the PR's working tree, guaranteeing a merge conflict against
   main once another PR landed. Fix: skip regeneration on non-default
   branches; let the aggregator workflow regenerate on main after PR
   merge as part of the aggregation commit.

2. `sync_agent_files_from_config()` ran AFTER `logmind log`'s commit,
   leaving refreshed AGENTS.md / CLAUDE.md as dirty working-tree
   modifications. Fix: run sync first, snapshot changed agent files,
   include them in scoped staging.
"""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from logmind.core.logger import log


def _git_init(path: Path, default_branch: str = "main") -> None:
    subprocess.run(
        ["git", "init", "-b", default_branch],
        cwd=path, check=True, capture_output=True,
    )
    subprocess.run(
        ["git", "config", "user.name", "T"],
        cwd=path, check=True, capture_output=True,
    )
    subprocess.run(
        ["git", "config", "user.email", "t@t.com"],
        cwd=path, check=True, capture_output=True,
    )
    (path / ".keep").write_text("", encoding="utf-8")
    subprocess.run(
        ["git", "add", ".keep"], cwd=path, check=True, capture_output=True,
    )
    subprocess.run(
        ["git", "commit", "-m", "init"],
        cwd=path, check=True, capture_output=True,
    )


def _checkout(path: Path, branch: str) -> None:
    subprocess.run(
        ["git", "checkout", "-b", branch],
        cwd=path, check=True, capture_output=True,
    )


# ---------------------------------------------------------------------------
# Bug #1: file-structure.md must NOT regenerate on a feature branch
# ---------------------------------------------------------------------------


def test_log_on_feature_branch_does_not_touch_file_structure(tmp_path, monkeypatch):
    """On a feature branch, `logmind log` should leave file-structure.md
    alone — only main owns the tree snapshot now."""
    _git_init(tmp_path, default_branch="main")
    _checkout(tmp_path, "feat/something")
    monkeypatch.chdir(tmp_path)

    docs = tmp_path / "docs"
    (docs / "decisions-branches").mkdir(parents=True)
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n", encoding="utf-8")
    fs_path = docs / "file-structure.md"
    sentinel = "BASELINE-SENTINEL-DO-NOT-REGENERATE\n"
    fs_path.write_text(sentinel, encoding="utf-8")

    log("feature work", reasoning="r", docs_path=docs, auto_commit=False)

    # file-structure.md must still contain the sentinel — proves no regen.
    assert fs_path.read_text(encoding="utf-8") == sentinel


def test_log_on_default_branch_does_regenerate_file_structure(tmp_path, monkeypatch):
    """On the default branch, `logmind log` regenerates file-structure.md
    so direct commits to main keep it fresh."""
    _git_init(tmp_path, default_branch="main")
    monkeypatch.chdir(tmp_path)

    docs = tmp_path / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n", encoding="utf-8")
    fs_path = docs / "file-structure.md"
    fs_path.write_text("# placeholder\n", encoding="utf-8")

    (tmp_path / "interesting.py").write_text("x = 1\n", encoding="utf-8")

    log("main-branch decision", reasoning="r", docs_path=docs, auto_commit=False)

    assert "interesting.py" in fs_path.read_text(encoding="utf-8")


# ---------------------------------------------------------------------------
# v0.1.3's "aggregator regenerates file-structure.md on main" test was
# deleted in v0.2: the aggregator is gone. The replacement behavior
# (regen-timeline.yml workflow regenerates BOTH timeline.md AND
# file-structure.md on PR commits) is covered in tests/test_timeline.py.
# ---------------------------------------------------------------------------


# ---------------------------------------------------------------------------
# Bug #2: agent-file refresh must be staged in the same commit
# ---------------------------------------------------------------------------


def test_log_stages_modified_agent_files_via_extra_scoped_paths(tmp_path, monkeypatch):
    """When AGENTS.md is dirty (e.g. just refreshed by
    sync_agent_files_from_config), `log()` with extra_scoped_paths should
    include it in the same commit — not leave it as a working-tree drift."""
    _git_init(tmp_path, default_branch="main")
    monkeypatch.chdir(tmp_path)

    docs = tmp_path / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n", encoding="utf-8")

    # Create an AGENTS.md that's tracked, then modify it to simulate a sync.
    agents = tmp_path / "AGENTS.md"
    agents.write_text("v1 content\n", encoding="utf-8")
    subprocess.run(
        ["git", "add", "AGENTS.md"], cwd=tmp_path,
        check=True, capture_output=True,
    )
    subprocess.run(
        ["git", "commit", "-m", "add agents"], cwd=tmp_path,
        check=True, capture_output=True,
    )
    agents.write_text("v2 content (refreshed)\n", encoding="utf-8")

    log(
        "test agent staging",
        reasoning="r",
        docs_path=docs,
        auto_commit=True,
        auto_push=False,
        stage="scoped",
        extra_scoped_paths=["AGENTS.md"],
    )

    # Commit should include AGENTS.md, not leave it dirty
    out = subprocess.run(
        ["git", "show", "--name-only", "--format=", "HEAD"],
        cwd=tmp_path, check=True, capture_output=True, text=True,
    )
    committed = {line for line in out.stdout.split() if line}
    assert "AGENTS.md" in committed

    status_out = subprocess.run(
        ["git", "status", "--porcelain", "AGENTS.md"],
        cwd=tmp_path, check=True, capture_output=True, text=True,
    )
    assert status_out.stdout.strip() == "", \
        f"AGENTS.md should be clean after commit, got: {status_out.stdout!r}"
