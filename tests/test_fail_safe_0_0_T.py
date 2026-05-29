"""Tests for 0.0.T (RTK-inspired fail-safe patterns).

Two surfaces audited:

1. ``iter_decisions`` in ``src/logmind/core/parser.py`` — previously
   silent-dropped malformed decision headers (regex matched but the
   date/time failed to parse). Now emits a stderr warning naming the
   file + lineno + parse error before skipping.

2. ``atomic_write_text`` in ``src/logmind/core/atomic_io.py`` —
   previously left an orphaned ``.tmp`` sibling behind if the write
   failed partway. Now cleans up the orphan on any exception, while
   still propagating the original exception (cleanup never masks it).
"""

from __future__ import annotations

import os
from datetime import datetime
from pathlib import Path

import pytest


# --- iter_decisions: warn on malformed header instead of silent-drop ---

def test_iter_decisions_warns_on_malformed_date(tmp_path, capsys):
    """Header structure matches but the date is impossible. The parser
    must skip the entry AND warn on stderr — silent drops were the
    Phase 0 hindsight bug 0.0.T fixes."""
    from logmind.core.parser import iter_decisions

    path = tmp_path / "decisions.md"
    path.write_text(
        "# Decision Log\n\n"
        "## 2026-01-01 10:00 - valid entry\n\n"
        "## 2026-13-45 25:99 - malformed date\n\n"   # impossible date
        "## 2026-02-02 11:00 - another valid entry\n\n",
        encoding="utf-8",
    )

    entries = list(iter_decisions(path))
    # Both valid entries yielded; malformed one skipped.
    assert len(entries) == 2
    assert entries[0][1] == "valid entry"
    assert entries[1][1] == "another valid entry"

    # And — critically — the malformed entry generated a stderr warning
    # naming the file, line number, and the parse error.
    captured = capsys.readouterr()
    assert "logmind: skipping malformed decision header" in captured.err
    assert str(path) in captured.err
    # The malformed entry is at line 5 (1-indexed, after 4 lines of
    # prelude/valid entry/blank).
    assert ":5:" in captured.err


def test_iter_decisions_no_warning_on_well_formed_file(tmp_path, capsys):
    """No spurious warnings on a clean file — only the malformed-header
    path emits to stderr."""
    from logmind.core.parser import iter_decisions

    path = tmp_path / "decisions.md"
    path.write_text(
        "# Decision Log\n\n## 2026-01-01 10:00 - clean entry\n\n",
        encoding="utf-8",
    )
    entries = list(iter_decisions(path))
    assert len(entries) == 1
    captured = capsys.readouterr()
    assert captured.err == ""


def test_iter_decisions_missing_file_returns_empty_no_warning(tmp_path, capsys):
    """Missing file is expected (optional decision-branch files); do
    not warn. Only malformed-header detection warns."""
    from logmind.core.parser import iter_decisions

    entries = list(iter_decisions(tmp_path / "does-not-exist.md"))
    assert entries == []
    captured = capsys.readouterr()
    assert captured.err == ""


# --- atomic_write_text: clean up orphan tmp file on failure ---

def test_atomic_write_cleans_up_tmp_on_write_failure(tmp_path, monkeypatch):
    """If Path.write_text raises mid-write, the orphaned .tmp sibling
    must be removed. Otherwise the next write would either trip on the
    stale file or have to ignore it."""
    from logmind.core import atomic_io

    target = tmp_path / "out.md"
    tmp_sibling = tmp_path / "out.md.tmp"

    # Monkey-patch Path.write_text to fail AFTER creating the tmp file
    # (simulating a disk-full or interrupt scenario where bytes started
    # to land but the write didn't complete).
    real_write_text = Path.write_text
    calls = {"n": 0}

    def failing_write_text(self, content, encoding="utf-8"):
        # Create a partial file first, then fail.
        real_write_text(self, "PARTIAL", encoding=encoding)
        calls["n"] += 1
        raise OSError("simulated disk full")

    monkeypatch.setattr(Path, "write_text", failing_write_text)

    with pytest.raises(OSError, match="simulated disk full"):
        atomic_io.atomic_write_text(target, "new content")

    # Original target untouched (it never existed in this test, so
    # absent is the right state).
    assert not target.exists()
    # CRITICAL: the orphaned tmp must have been cleaned up.
    assert not tmp_sibling.exists(), (
        "atomic_write_text must clean up the .tmp sibling on failure — "
        "leaving it behind would trip the next write"
    )
    # And the failure path ran exactly once (no retry / hidden swallow).
    assert calls["n"] == 1


def test_atomic_write_cleanup_never_masks_original_exception(tmp_path, monkeypatch):
    """If both the write AND the cleanup fail, the ORIGINAL exception
    must propagate — never replaced by the cleanup's exception. RTK
    pattern: cleanup is best-effort, never load-bearing."""
    from logmind.core import atomic_io

    target = tmp_path / "out.md"

    # Force the write to fail first.
    def failing_write_text(self, content, encoding="utf-8"):
        raise OSError("PRIMARY: write failed")

    # Force the cleanup (tmp.unlink) to also fail.
    real_unlink = Path.unlink

    def failing_unlink(self, missing_ok=False):
        raise OSError("SECONDARY: unlink failed (should not propagate)")

    monkeypatch.setattr(Path, "write_text", failing_write_text)
    monkeypatch.setattr(Path, "unlink", failing_unlink)

    # The PRIMARY exception is what surfaces; the SECONDARY (cleanup)
    # is swallowed.
    with pytest.raises(OSError, match="PRIMARY: write failed"):
        atomic_io.atomic_write_text(target, "x")


def test_atomic_write_success_path_unchanged(tmp_path):
    """Sanity: the happy path still works (no regression from adding
    the try/except)."""
    from logmind.core.atomic_io import atomic_write_text

    target = tmp_path / "out.md"
    atomic_write_text(target, "content\n")
    assert target.read_text(encoding="utf-8") == "content\n"
    # No orphan tmp sibling left behind.
    assert not (tmp_path / "out.md.tmp").exists()
