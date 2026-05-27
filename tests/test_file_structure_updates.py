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
        "# header\n\nfoo\n# inline\nbar/\n!keep_me\n/baz\n",
        encoding="utf-8",
    )
    ignore, negate = _read_gitignore_patterns(tmp_path)
    assert "foo" in ignore
    assert "bar" in ignore  # trailing slash stripped
    assert "baz" in ignore  # leading slash stripped
    assert "keep_me" in negate  # negation collected for precedence-aware match


def test_read_gitignore_returns_empty_when_missing(tmp_path):
    assert _read_gitignore_patterns(tmp_path) == ([], [])


def test_resolve_ignore_includes_defaults_plus_gitignore(tmp_path):
    (tmp_path / ".gitignore").write_text("custom_dir\n*.log\n", encoding="utf-8")
    out = _resolve_ignore_patterns(tmp_path)
    assert "custom_dir" in out
    assert "*.log" in out
    for default in DEFAULT_IGNORES:
        assert default in out


# ---------------------------------------------------------------------------
# Path-aware ignore matching (regression for v0.1.1 bug: site/.next/ in tree)
# ---------------------------------------------------------------------------


def test_fallback_tree_honors_path_pattern_from_gitignore(tmp_path):
    """A .gitignore entry like ``site/.next/`` should skip the nested
    ``site/.next/cache/...`` tree, not just basename-match ``.next``.
    Regression for v0.1.1 where 280 lines of Next.js build cache landed
    in docs/file-structure.md."""
    site = tmp_path / "site"
    cache = site / ".next" / "cache"
    cache.mkdir(parents=True)
    (cache / "chunk_abc123.js").write_text("", encoding="utf-8")
    (site / "page.tsx").write_text("", encoding="utf-8")
    (tmp_path / ".gitignore").write_text("site/.next/\n", encoding="utf-8")

    tree = _generate_fallback_tree(tmp_path)
    assert "page.tsx" in tree
    assert ".next" not in tree
    assert "chunk_abc123.js" not in tree


def test_fallback_tree_honors_gitignore_negation(tmp_path):
    """``!keep.log`` after ``*.log`` should re-include keep.log."""
    (tmp_path / "noisy.log").write_text("", encoding="utf-8")
    (tmp_path / "keep.log").write_text("", encoding="utf-8")
    (tmp_path / ".gitignore").write_text("*.log\n!keep.log\n", encoding="utf-8")

    tree = _generate_fallback_tree(tmp_path)
    assert "noisy.log" not in tree
    assert "keep.log" in tree


# ---------------------------------------------------------------------------
# Fallback tree
# ---------------------------------------------------------------------------


def test_fallback_tree_respects_gitignore(tmp_path):
    (tmp_path / "kept.txt").write_text("ok", encoding="utf-8")
    (tmp_path / "secret_dir").mkdir()
    (tmp_path / "secret_dir" / "x").write_text("ok", encoding="utf-8")
    (tmp_path / ".gitignore").write_text("secret_dir\n", encoding="utf-8")

    tree = _generate_fallback_tree(tmp_path)
    assert "kept.txt" in tree
    assert "secret_dir" not in tree


def test_fallback_tree_sort_is_dirs_first_then_alphabetical(tmp_path):
    (tmp_path / "z.txt").write_text("", encoding="utf-8")
    (tmp_path / "a.txt").write_text("", encoding="utf-8")
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
        (cur / f"f{i}.txt").write_text("", encoding="utf-8")
    tree = _generate_fallback_tree(tmp_path)
    for i in range(8):
        assert f"L{i}" in tree
        assert f"f{i}.txt" in tree


def test_generate_tree_returns_text(tmp_path):
    (tmp_path / "a.txt").write_text("", encoding="utf-8")
    out = generate_tree(tmp_path)
    assert "a.txt" in out


# ---------------------------------------------------------------------------
# update_file_structure side effects
# ---------------------------------------------------------------------------


def test_update_file_structure_writes_freshly(tmp_path):
    docs = tmp_path / "docs"
    docs.mkdir()
    (tmp_path / "marker.txt").write_text("ok", encoding="utf-8")

    update_file_structure(docs)

    content = (docs / "file-structure.md").read_text(encoding="utf-8")
    assert "marker.txt" in content
    assert "# File Structure" in content


def test_update_file_structure_updates_on_repeat(tmp_path):
    """Adding a new file then re-running should reflect the new file."""
    docs = tmp_path / "docs"
    docs.mkdir()
    update_file_structure(docs)

    (tmp_path / "newcomer.md").write_text("", encoding="utf-8")
    update_file_structure(docs)

    content = (docs / "file-structure.md").read_text(encoding="utf-8")
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
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n", encoding="utf-8")
    (docs / "file-structure.md").write_text("# old\n", encoding="utf-8")

    (tmp_path / "interesting_new_file.py").write_text("# matters\n", encoding="utf-8")

    log("did a thing", reasoning="r", docs_path=docs, auto_commit=False)

    fs_content = (docs / "file-structure.md").read_text(encoding="utf-8")
    assert "interesting_new_file.py" in fs_content
    assert "# File Structure" in fs_content
