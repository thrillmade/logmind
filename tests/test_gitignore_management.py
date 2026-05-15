"""Tests for logmind's .gitignore block management (Phase 9b)."""

from __future__ import annotations

from pathlib import Path

import pytest

from logmind.core.gitignore import (
    DEFAULT_GITIGNORE_LINES,
    LOGMIND_GITIGNORE_END,
    LOGMIND_GITIGNORE_START,
    ensure_block,
    has_block,
)


def test_creates_file_when_missing(tmp_path):
    gi = tmp_path / ".gitignore"
    assert ensure_block(gi) is True
    text = gi.read_text(encoding="utf-8")
    assert LOGMIND_GITIGNORE_START in text
    assert LOGMIND_GITIGNORE_END in text
    for line in DEFAULT_GITIGNORE_LINES:
        assert line in text


def test_appends_to_existing_file_preserving_user_lines(tmp_path):
    gi = tmp_path / ".gitignore"
    user_content = "# user\nvenv/\n*.pyc\n"
    gi.write_text(user_content, encoding="utf-8")

    assert ensure_block(gi) is True

    text = gi.read_text(encoding="utf-8")
    # User content preserved verbatim at the top
    assert text.startswith(user_content)
    assert LOGMIND_GITIGNORE_START in text


def test_idempotent_when_block_already_present(tmp_path):
    gi = tmp_path / ".gitignore"
    ensure_block(gi)
    before = gi.read_text(encoding="utf-8")

    assert ensure_block(gi) is False  # no-op
    assert gi.read_text(encoding="utf-8") == before


def test_user_edits_inside_block_preserved(tmp_path):
    gi = tmp_path / ".gitignore"
    ensure_block(gi)
    text = gi.read_text(encoding="utf-8")
    # Mutate the managed block as if a user edited it
    text = text.replace(".logmind/cache/", ".logmind/cache/\nuser-added-pattern/")
    gi.write_text(text, encoding="utf-8")

    assert ensure_block(gi) is False
    new_text = gi.read_text(encoding="utf-8")
    assert "user-added-pattern/" in new_text  # preserved
    # Block markers still present and intact
    assert new_text.count(LOGMIND_GITIGNORE_START) == 1
    assert new_text.count(LOGMIND_GITIGNORE_END) == 1


def test_creates_parent_dirs_when_target_path_nested(tmp_path):
    gi = tmp_path / "nested" / "subdir" / ".gitignore"
    assert ensure_block(gi) is True
    assert gi.exists()
    assert LOGMIND_GITIGNORE_START in gi.read_text(encoding="utf-8")


def test_has_block_detects_marker(tmp_path):
    gi = tmp_path / ".gitignore"
    assert has_block(gi) is False
    ensure_block(gi)
    assert has_block(gi) is True


def test_custom_lines_override_default(tmp_path):
    gi = tmp_path / ".gitignore"
    ensure_block(gi, lines=["custom_a", "custom_b"])
    text = gi.read_text(encoding="utf-8")
    assert "custom_a" in text
    assert "custom_b" in text
    assert ".logmind/cache/" not in text  # defaults bypassed
