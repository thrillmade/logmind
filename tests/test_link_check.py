"""Tests for logmind.actions.link_check (Phase 7 markdown link integrity)."""

from __future__ import annotations

from pathlib import Path

import pytest

from logmind.actions.link_check import (
    DEFAULT_ALLOW_ORPHANS,
    check,
    format_report,
    main,
)


def _make_clean_tree(root: Path) -> None:
    """Build a small docs tree where every markdown is reachable."""
    (root / "docs").mkdir()
    (root / "README.md").write_text(
        "# Project\n\n- [plan](docs/plan.md)\n- [extra](docs/extra.md)\n"
    , encoding="utf-8")
    (root / "docs" / "plan.md").write_text(
        "# Plan\n\nSee also [extra](extra.md) and [README](../README.md).\n"
    , encoding="utf-8")
    (root / "docs" / "extra.md").write_text("# Extra\n\nLinked from plan.\n", encoding="utf-8")


def test_clean_tree_reports_nothing(tmp_path):
    _make_clean_tree(tmp_path)
    broken, orphans = check(tmp_path)
    assert broken == []
    assert orphans == []


def test_broken_relative_link(tmp_path):
    _make_clean_tree(tmp_path)
    (tmp_path / "README.md").write_text("[bad](docs/missing.md)\n", encoding="utf-8")
    broken, orphans = check(tmp_path)
    assert any("missing -> docs/missing.md" in b for b in broken)


def test_orphan_md_in_docs(tmp_path):
    _make_clean_tree(tmp_path)
    (tmp_path / "docs" / "alone.md").write_text("# Alone\n", encoding="utf-8")
    broken, orphans = check(tmp_path)
    assert "docs/alone.md" in orphans
    assert broken == []


def test_orphan_allowlist(tmp_path):
    """Files on the allowlist are never flagged as orphan even if no incoming link."""
    _make_clean_tree(tmp_path)
    (tmp_path / "docs" / "decisions.md").write_text("# Log\n", encoding="utf-8")  # in default allowlist
    broken, orphans = check(tmp_path)
    assert "docs/decisions.md" not in orphans


def test_custom_allowlist(tmp_path):
    _make_clean_tree(tmp_path)
    (tmp_path / "docs" / "alone.md").write_text("# Alone\n", encoding="utf-8")
    broken, orphans = check(
        tmp_path,
        allow_orphans=list(DEFAULT_ALLOW_ORPHANS) + ["docs/alone.md"],
    )
    assert orphans == []


def test_external_links_ignored(tmp_path):
    _make_clean_tree(tmp_path)
    (tmp_path / "README.md").write_text(
        "[ext](https://example.com)\n"
        "[mail](mailto:x@y.com)\n"
        "[anchor](#section)\n"
        "[plan](docs/plan.md)\n"
        "[extra](docs/extra.md)\n"
    , encoding="utf-8")
    broken, orphans = check(tmp_path)
    assert broken == []
    assert orphans == []


def test_anchor_in_target_validated_by_file_only(tmp_path):
    _make_clean_tree(tmp_path)
    (tmp_path / "README.md").write_text("[deep](docs/plan.md#section)\n[extra](docs/extra.md)\n", encoding="utf-8")
    broken, orphans = check(tmp_path)
    assert broken == []  # plan.md exists; anchor not validated


def test_broken_anchor_to_missing_file(tmp_path):
    _make_clean_tree(tmp_path)
    (tmp_path / "README.md").write_text(
        "[bad](docs/missing.md#x)\n[extra](docs/extra.md)\n[plan](docs/plan.md)\n"
    , encoding="utf-8")
    broken, orphans = check(tmp_path)
    assert any("docs/missing.md" in b for b in broken)


# v0.5.9 / issue #60 — strip code regions before scanning for links.
# Pre-v0.5.9, mentioning markdown link syntax inside backticks (a
# documentation-of-the-syntax pattern) tripped the link checker even
# though the bracket-paren wasn't a live link. These tests pin the new
# code-aware behaviour.

def test_links_inside_inline_code_span_are_ignored(tmp_path):
    """Mentioning `[text](path)` inside backticks is NOT a live link."""
    _make_clean_tree(tmp_path)
    (tmp_path / "README.md").write_text(
        "Discussion: use `[text](docs/this-does-not-exist.md)` syntax for links.\n",
        encoding="utf-8",
    )
    broken, orphans = check(tmp_path)
    assert broken == [], (
        f"v0.5.9 / #60: link inside inline-code span must be ignored. "
        f"Got broken={broken!r}"
    )


def test_links_inside_fenced_code_block_are_ignored(tmp_path):
    """Mentioning [text](path) inside ``` blocks is NOT a live link."""
    _make_clean_tree(tmp_path)
    (tmp_path / "README.md").write_text(
        "Example:\n\n```markdown\n[broken](docs/nonexistent.md)\n```\n",
        encoding="utf-8",
    )
    broken, orphans = check(tmp_path)
    assert broken == [], (
        f"v0.5.9 / #60: link inside fenced code block must be ignored. "
        f"Got broken={broken!r}"
    )


def test_links_outside_code_still_detected_alongside_code_examples(tmp_path):
    """Real broken links outside code regions are STILL caught even when
    the same file mentions the link syntax in prose. Regression guard:
    the strip-code-regions fix mustn't silence legitimate broken-link
    detection."""
    _make_clean_tree(tmp_path)
    (tmp_path / "README.md").write_text(
        # Real broken link in prose:
        "[real-broken](docs/actually-missing.md)\n\n"
        # Same file mentions the link syntax in code (should be ignored):
        "We use the `[text](path)` syntax. Avoid `[broken](nope.md)` examples.\n",
        encoding="utf-8",
    )
    broken, orphans = check(tmp_path)
    # Must detect the real broken link...
    assert any("docs/actually-missing.md" in b for b in broken), (
        f"v0.5.9 / #60 regression: legitimate broken link in prose missed. "
        f"Got broken={broken!r}"
    )
    # ...but NOT the in-code examples.
    assert not any("nope.md" in b for b in broken), (
        f"v0.5.9 / #60: in-code example `[broken](nope.md)` got flagged. "
        f"Got broken={broken!r}"
    )


def test_strip_code_regions_preserves_line_numbers(tmp_path):
    """The strip-code-regions transform replaces code regions with
    whitespace of equivalent length so byte offsets + line numbers stay
    correct. Tested indirectly: a broken link AFTER a multi-line fenced
    block reports correctly."""
    from logmind.actions.link_check import _strip_code_regions
    src = (
        "Line 1\n"
        "```\n"
        "code line A\n"
        "code line B\n"
        "```\n"
        "Line 6: [real](missing.md)\n"
    )
    stripped = _strip_code_regions(src)
    # Same number of lines.
    assert stripped.count("\n") == src.count("\n")
    # Real link still present in the stripped output.
    assert "[real](missing.md)" in stripped
    # Fenced content gone (replaced by whitespace).
    assert "code line A" not in stripped
    assert "code line B" not in stripped


def test_format_report_clean():
    assert "no orphans" in format_report([], [])


def test_format_report_with_failures():
    out = format_report(["a.md: missing -> x.md"], ["docs/y.md"])
    assert "Broken links (1)" in out
    assert "Orphan markdown files (1)" in out


def test_main_exits_zero_on_clean(tmp_path, monkeypatch, capsys):
    _make_clean_tree(tmp_path)
    monkeypatch.chdir(tmp_path)
    rc = main()
    assert rc == 0
    assert "no orphans" in capsys.readouterr().out


def test_main_exits_one_on_broken(tmp_path, monkeypatch, capsys):
    _make_clean_tree(tmp_path)
    (tmp_path / "README.md").write_text("[bad](docs/missing.md)\n[extra](docs/extra.md)\n", encoding="utf-8")
    monkeypatch.chdir(tmp_path)
    rc = main()
    assert rc == 1
    assert "Broken" in capsys.readouterr().out


def test_main_honours_config_allow_orphans(tmp_path, monkeypatch, capsys):
    _make_clean_tree(tmp_path)
    (tmp_path / "docs" / "alone.md").write_text("# Alone\n", encoding="utf-8")
    (tmp_path / ".logmind").mkdir()
    (tmp_path / ".logmind" / "config.yml").write_text(
        "linkcheck:\n  allow_orphans:\n    - docs/alone.md\n"
    , encoding="utf-8")
    monkeypatch.chdir(tmp_path)
    rc = main()
    assert rc == 0


def test_main_honours_config_roots(tmp_path, monkeypatch, capsys):
    """When roots is configured, files outside the roots aren't even scanned."""
    _make_clean_tree(tmp_path)
    # Add an ignored area with broken link — must not affect exit code
    (tmp_path / "ignored").mkdir()
    (tmp_path / "ignored" / "x.md").write_text("[bad](nope.md)\n", encoding="utf-8")
    (tmp_path / ".logmind").mkdir()
    (tmp_path / ".logmind" / "config.yml").write_text(
        "linkcheck:\n  roots:\n    - README.md\n    - docs\n"
    , encoding="utf-8")
    monkeypatch.chdir(tmp_path)
    rc = main()
    assert rc == 0
