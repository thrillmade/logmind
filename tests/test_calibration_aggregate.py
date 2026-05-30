"""Test the Layer 1.5 calibration marker regex in bench/scripts/calibration_aggregate.py.

Avoids gh-API mocking (the fetcher is intentionally I/O bound). Locks down
the regex against the v0.6.25 marker format so any clud-bug-side format
change surfaces as a test failure rather than silent zero markers.
"""

from __future__ import annotations

import pytest

from bench.scripts.calibration_aggregate import MARKER_RE


def test_marker_regex_matches_canonical_v0625_format():
    """The marker shape that clud-bug v0.6.25+ emits on every review."""
    body = (
        "## 🐛 Clud Bug review\n"
        "(...summary content...)\n"
        "<!-- last-reviewed-sha: abc123 -->\n"
        "<!-- clud-bug-calibration: turns_estimated=17, max_turns=21, "
        "files=10, lines_added=299, lines_deleted=10, threads=0 -->\n"
    )
    m = MARKER_RE.search(body)
    assert m is not None, "regex must match the canonical v0.6.25 marker format"
    assert int(m.group(1)) == 17  # turns_estimated
    assert int(m.group(2)) == 21  # max_turns
    assert int(m.group(3)) == 10  # files
    assert int(m.group(4)) == 299  # lines_added
    assert int(m.group(5)) == 10  # lines_deleted
    assert int(m.group(6)) == 0  # threads


def test_marker_regex_handles_extra_whitespace():
    """The renderer may inject extra whitespace; tolerate it without breaking."""
    body = (
        "<!--   clud-bug-calibration:    "
        "turns_estimated=5,   max_turns=10,  files=1, lines_added=5, "
        "lines_deleted=0,  threads=0   -->"
    )
    m = MARKER_RE.search(body)
    assert m is not None
    assert int(m.group(1)) == 5
    assert int(m.group(2)) == 10


def test_marker_regex_does_not_match_partial_or_malformed_markers():
    """Don't false-positive on near-misses (missing fields, typos).

    Path C's downstream stats break if we miscount; better to silently
    skip a malformed marker than count it wrongly.
    """
    cases = [
        # Missing threads field
        "<!-- clud-bug-calibration: turns_estimated=5, max_turns=10, "
        "files=1, lines_added=5, lines_deleted=0 -->",
        # Typo in marker prefix
        "<!-- clud-buug-calibration: turns_estimated=5, max_turns=10, "
        "files=1, lines_added=5, lines_deleted=0, threads=0 -->",
        # Non-numeric value
        "<!-- clud-bug-calibration: turns_estimated=many, max_turns=10, "
        "files=1, lines_added=5, lines_deleted=0, threads=0 -->",
        # Empty body
        "",
        # No marker at all
        "## 🐛 Clud Bug review\n0 critical · 0 minor",
    ]
    for body in cases:
        assert MARKER_RE.search(body) is None, (
            f"regex must not match malformed/missing marker: {body!r}"
        )


def test_marker_regex_extracts_only_one_per_comment():
    """A comment with multiple marker-shaped strings (e.g. quoted in a fix-push)
    should still extract the FIRST one — downstream code uses .search() not
    .findall() since the renderer emits exactly one per review."""
    body = (
        "<!-- clud-bug-calibration: turns_estimated=1, max_turns=1, files=1, "
        "lines_added=1, lines_deleted=0, threads=0 -->\n"
        "<!-- clud-bug-calibration: turns_estimated=99, max_turns=99, "
        "files=99, lines_added=99, lines_deleted=99, threads=9 -->\n"
    )
    m = MARKER_RE.search(body)
    assert m is not None
    assert int(m.group(1)) == 1  # first marker wins
