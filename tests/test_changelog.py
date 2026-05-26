"""Tests for the CHANGELOG section extractor used by upgrade prompts."""

from __future__ import annotations

from logmind.core.changelog import (
    _parse_version,
    extract_sections_between,
    render_upgrade_prompt,
)


SAMPLE = """# Changelog

## [Unreleased]

## [0.2.10] - 2026-05-26

### Added
- Upgrade-prompt printing in `logmind init`.

## [0.2.9] - 2026-05-26

### Changed
- actions/checkout@v4 → @v6.

## [0.2.8] - 2026-05-26

### Fixed
- pinVersion detection uses grep.

## [0.2.7] - 2026-05-26

### Changed
- logmind log defaults to --stage all.
"""


def test_parse_version_orders_numerically():
    assert _parse_version("0.2.7") < _parse_version("0.2.10")
    assert _parse_version("0.2.10") < _parse_version("0.3.0")
    assert _parse_version("0.2.7") < _parse_version("0.2.8")


def test_parse_version_pre_release_sorts_before_stable():
    assert _parse_version("0.2.10-rc1") < _parse_version("0.2.10")


def test_extract_two_consecutive_versions():
    """Slice between 0.2.8 (exclusive) and 0.2.10 (inclusive) — should
    grab 0.2.10 and 0.2.9 sections only.
    """
    out = extract_sections_between(SAMPLE, after="0.2.8", up_to="0.2.10")
    assert "## [0.2.10]" in out
    assert "## [0.2.9]" in out
    assert "## [0.2.8]" not in out
    assert "## [0.2.7]" not in out
    assert "## [Unreleased]" not in out


def test_extract_single_version():
    out = extract_sections_between(SAMPLE, after="0.2.9", up_to="0.2.10")
    assert "## [0.2.10]" in out
    assert "## [0.2.9]" not in out


def test_extract_returns_empty_when_already_current():
    out = extract_sections_between(SAMPLE, after="0.2.10", up_to="0.2.10")
    assert out == ""


def test_extract_returns_empty_when_prior_ahead():
    out = extract_sections_between(SAMPLE, after="0.3.0", up_to="0.2.10")
    assert out == ""


def test_extract_excludes_unreleased():
    """[Unreleased] header doesn't parse as a semver and should never
    appear in the prompt body."""
    out = extract_sections_between(SAMPLE, after="0.2.7", up_to="0.2.10")
    assert "## [Unreleased]" not in out
    assert "## [0.2.10]" in out


def test_extract_handles_none_as_after():
    """after=None means 'include everything up to up_to'."""
    out = extract_sections_between(SAMPLE, after=None, up_to="0.2.8")
    assert "## [0.2.8]" in out
    assert "## [0.2.7]" in out
    assert "## [0.2.9]" not in out


def test_render_upgrade_prompt_returns_none_on_same_version():
    assert render_upgrade_prompt(prior_version="0.2.10", current_version="0.2.10") is None


def test_render_upgrade_prompt_returns_block_on_real_upgrade(tmp_path, monkeypatch):
    """End-to-end: with a CHANGELOG findable on disk, the prompt is
    non-empty and contains the version delta."""
    from logmind.core import changelog as cl

    # Point the loader at an in-memory CHANGELOG
    sample_path = tmp_path / "CHANGELOG.md"
    sample_path.write_text(SAMPLE, encoding="utf-8")
    monkeypatch.setattr(cl, "_changelog_path", lambda: sample_path)

    prompt = render_upgrade_prompt(prior_version="0.2.8", current_version="0.2.10")
    assert prompt is not None
    # Section headers for 0.2.10 and 0.2.9 must appear; 0.2.8's section
    # must NOT (it's the prior version, exclusive lower bound). The "since
    # v0.2.8" appears in the prompt's intro line — that's expected — so
    # we check for the section header form `## [X.Y.Z]` specifically.
    assert "## [0.2.10]" in prompt
    assert "## [0.2.9]" in prompt
    assert "## [0.2.8]" not in prompt
    assert "What's new" in prompt
    assert "since v0.2.8" in prompt  # header references the prior version


def test_render_upgrade_prompt_handles_missing_changelog(monkeypatch):
    from logmind.core import changelog as cl

    monkeypatch.setattr(cl, "_changelog_path", lambda: None)
    assert render_upgrade_prompt(prior_version="0.2.8", current_version="0.2.10") is None
