"""Tests for tree generation hardening (Phase 9 — file-structure auto-update)."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from logmind.core.tree_gen import (
    DEFAULT_IGNORES,
    _generate_fallback_tree,
    _read_gitignore_patterns,
    _resolve_ignore_patterns,
    generate_tree,
    update_file_structure,
)


# ---------------------------------------------------------------------------
# .gitignore parsing
# ---------------------------------------------------------------------------


def test_read_gitignore_skips_comments_and_blanks(tmp_path):
    (tmp_path / ".gitignore").write_text(
        "# header\n\nfoo\n# inline\nbar/\n!keep_me\n/baz\n"
    )
    out = _read_gitignore_patterns(tmp_path)
    assert "foo" in out
    assert "bar" in out  # trailing slash stripped
    assert "baz" in out  # leading slash stripped
    assert "keep_me" not in out  # negation dropped (out of scope)


def test_read_gitignore_returns_empty_when_missing(tmp_path):
    assert _read_gitignore_patterns(tmp_path) == []


def test_resolve_ignore_includes_defaults_plus_gitignore(tmp_path):
    (tmp_path / ".gitignore").write_text("custom_dir\n*.log\n")
    out = _resolve_ignore_patterns(tmp_path)
    assert "custom_dir" in out
    assert "*.log" in out
    for default in DEFAULT_IGNORES:
        assert default in out


# ---------------------------------------------------------------------------
# Fallback tree
# ---------------------------------------------------------------------------


def test_fallback_tree_respects_gitignore(tmp_path):
    (tmp_path / "kept.txt").write_text("ok")
    (tmp_path / "secret_dir").mkdir()
    (tmp_path / "secret_dir" / "x").write_text("ok")
    (tmp_path / ".gitignore").write_text("secret_dir\n")

    tree = _generate_fallback_tree(tmp_path)
    assert "kept.txt" in tree
    assert "secret_dir" not in tree


def test_fallback_tree_sort_is_dirs_first_then_alphabetical(tmp_path):
    (tmp_path / "z.txt").write_text("")
    (tmp_path / "a.txt").write_text("")
    (tmp_path / "M_dir").mkdir()
    (tmp_path / "B_dir").mkdir()

    tree = _generate_fallback_tree(tmp_path)
    lines = tree.splitlines()
    # First line is the root name; rest are children
    children = lines[1:]
    indices = {name: next(i for i, line in enumerate(children) if name in line)
               for name in ("a.txt", "z.txt", "M_dir", "B_dir")}
    assert indices["B_dir"] < indices["M_dir"] < indices["a.txt"] < indices["z.txt"]


def test_fallback_tree_walks_unbounded_by_default(tmp_path):
    """No artificial depth cap — match the real `tree` binary."""
    cur = tmp_path
    for i in range(8):
        cur = cur / f"L{i}"
        cur.mkdir()
        (cur / f"f{i}.txt").write_text("")
    tree = _generate_fallback_tree(tmp_path)
    for i in range(8):
        assert f"L{i}" in tree
        assert f"f{i}.txt" in tree


def test_generate_tree_returns_text(tmp_path):
    (tmp_path / "a.txt").write_text("")
    out = generate_tree(tmp_path)
    assert "a.txt" in out


# ---------------------------------------------------------------------------
# update_file_structure side effects
# ---------------------------------------------------------------------------


def test_update_file_structure_writes_freshly(tmp_path):
    docs = tmp_path / "docs"
    docs.mkdir()
    (tmp_path / "marker.txt").write_text("ok")

    update_file_structure(docs)

    content = (docs / "file-structure.md").read_text()
    assert "marker.txt" in content
    assert "Last updated:" in content


def test_update_file_structure_updates_on_repeat(tmp_path):
    """Adding a new file then re-running should reflect the new file."""
    docs = tmp_path / "docs"
    docs.mkdir()
    update_file_structure(docs)

    (tmp_path / "newcomer.md").write_text("")
    update_file_structure(docs)

    content = (docs / "file-structure.md").read_text()
    assert "newcomer.md" in content


# ---------------------------------------------------------------------------
# log() integrates with update_file_structure
# ---------------------------------------------------------------------------


def test_log_updates_file_structure(tmp_path, monkeypatch):
    """Calling `log()` writes a decision AND regenerates file-structure.md."""
    from logmind.core.logger import log

    # Init a git repo so branch detection succeeds
    subprocess.run(["git", "init", "-b", "main"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.name", "T"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.email", "t@t.com"], cwd=tmp_path, check=True, capture_output=True)
    monkeypatch.chdir(tmp_path)

    docs = tmp_path / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n")
    (docs / "file-structure.md").write_text("# old\n")

    (tmp_path / "interesting_new_file.py").write_text("# matters\n")

    log("did a thing", reasoning="r", docs_path=docs, auto_commit=False)

    fs_content = (docs / "file-structure.md").read_text()
    assert "interesting_new_file.py" in fs_content
    assert "Last updated:" in fs_content
