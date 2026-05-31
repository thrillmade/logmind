"""Tests for bench/scripts/calibration_layer1.py.

Coefficient parity with clud-bug's jq formula is the load-bearing
invariant. The CROSS-CHECK tests (against real Layer 1.5 markers
from v0.6.25+ PRs) are the most important — they verify our
retro-prediction matches what the live workflow actually computed.
"""

from __future__ import annotations

import pytest

from bench.scripts.calibration_layer1 import (
    classify_path,
    per_file_cost,
    predict,
    predict_estimated_turns,
    predict_max_turns,
    weight_for_class,
)


# ---------------------------------------------------------------------------
# classify_path — pattern dispatch
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("path,expected", [
    ("tests/test_foo.py", "test"),
    ("test/foo.py", "test"),
    ("src/foo.test.ts", "test"),
    ("src/foo.spec.js", "test"),
    ("src/__tests__/foo.tsx", "test"),
    ("README.md", "docs"),
    ("docs/plan.md", "docs"),
    ("doc.txt", "docs"),
    ("guide.rst", "docs"),
    ("page.mdx", "docs"),
    (".github/workflows/x.yml", "config"),
    ("pyproject.toml", "config"),
    ("package.json", "config"),
    ("template.yml.tmpl", "config"),
    ("src/foo.py", "code"),
    ("lib/bar.ts", "code"),
    ("api.go", "code"),
    ("Main.java", "code"),
    ("Dockerfile", "unknown"),
    ("Makefile", "unknown"),
    ("LICENSE", "unknown"),
])
def test_classify_path(path, expected):
    assert classify_path(path) == expected


def test_weight_ordering_matches_doc_assumptions():
    assert weight_for_class("docs") < weight_for_class("test")
    assert weight_for_class("test") == weight_for_class("config")
    assert weight_for_class("test") < weight_for_class("code")
    assert weight_for_class("unknown") < weight_for_class("code")


# ---------------------------------------------------------------------------
# per_file_cost — formula correctness
# ---------------------------------------------------------------------------


def test_per_file_cost_excluded_paths_return_zero():
    for path in ("docs/timeline.md", "docs/file-structure.md", "docs/decisions.md"):
        assert per_file_cost(path, 100, 100) == 0.0, f"{path} must be excluded"


def test_per_file_cost_pure_addition_code():
    """50 added: 0.3 + 50 × 0.02 = 1.3 turns."""
    assert per_file_cost("src/foo.py", 50, 0) == pytest.approx(1.3, abs=0.001)


def test_per_file_cost_pure_deletion_code():
    """50 deleted: 0.3 + 50 × 0.02 × 0.1 = 0.4."""
    assert per_file_cost("src/foo.py", 0, 50) == pytest.approx(0.4, abs=0.001)


def test_per_file_cost_pure_modify_code():
    """50/50: mod=50, pa=0, pd=0. 0.3 + 50 × 1.5 × 0.02 = 1.8."""
    assert per_file_cost("src/foo.py", 50, 50) == pytest.approx(1.8, abs=0.001)


def test_per_file_cost_docs_cheaper_than_code():
    code = per_file_cost("src/foo.py", 100, 0)
    docs = per_file_cost("README.md", 100, 0)
    assert docs < code
    assert docs == pytest.approx(0.3 + 100 * 0.00666667, abs=0.001)


# ---------------------------------------------------------------------------
# predict_estimated_turns
# ---------------------------------------------------------------------------


def test_predict_trivial_is_flat_10():
    assert predict_estimated_turns([], is_trivial=True) == 10
    assert predict_estimated_turns(
        [{"path": "huge.py", "additions": 10000, "deletions": 0}],
        is_trivial=True,
    ) == 10


def test_predict_empty_diff_returns_emit_overhead():
    """Zero files = 10 emit overhead."""
    assert predict_estimated_turns([], prior_threads=0) == 10


def test_predict_one_thread_adds_1_5_turns():
    """0 files + 1 thread = ceil(10 + 1.5) = 12."""
    assert predict_estimated_turns([], prior_threads=1) == 12


def test_predict_classic_small_pr():
    """3 code files × 50 added = 3 × 1.3 = 3.9 + 10 = 13.9 → ceil 14."""
    files = [
        {"path": f"src/file{i}.py", "additions": 50, "deletions": 0}
        for i in range(3)
    ]
    assert predict_estimated_turns(files) == 14


# ---------------------------------------------------------------------------
# predict_max_turns
# ---------------------------------------------------------------------------


def test_max_turns_trivial_flat_10():
    assert predict_max_turns(10, is_trivial=True) == 10


def test_max_turns_below_floor_clamps_to_15():
    """est=10 → 10×1.2=12 → clamps up to 15."""
    assert predict_max_turns(10) == 15


def test_max_turns_above_ceiling_clamps_to_60():
    assert predict_max_turns(100) == 60


def test_max_turns_in_range_is_ceil_120pct():
    assert predict_max_turns(20) == 24
    assert predict_max_turns(30) == 36


def test_max_turns_matches_bash_arithmetic():
    """Bash: (est * 12 + 9) / 10 integer division."""
    assert predict_max_turns(17) == 21  # (204+9)//10 = 21
    assert predict_max_turns(15) == 18  # (180+9)//10 = 18
    assert predict_max_turns(21) == 26  # (252+9)//10 = 26


# ---------------------------------------------------------------------------
# CROSS-CHECK against known Layer 1.5 markers (most important tests)
# ---------------------------------------------------------------------------


def test_cross_check_logmind_pr_86_v0511():
    """PR #86: marker had turns_estimated=17, max_turns=21, files=10,
    +299/-10, threads=0. Approximate shape: 5 tests + 5 code, ~30 added each."""
    pred = predict_estimated_turns(
        [{"path": f"tests/test_{i}.py", "additions": 30, "deletions": 1} for i in range(5)] +
        [{"path": f"src/file{i}.py", "additions": 30, "deletions": 1} for i in range(5)],
        prior_threads=0,
    )
    assert 14 <= pred <= 20, f"v0.5.11 cross-check: predicted={pred}, marker=17"


def test_cross_check_logmind_pr_87_v0512():
    """PR #87: turns_estimated=15, max_turns=18, files=8, +225/-6, threads=0."""
    pred = predict_estimated_turns(
        [{"path": f"src/x{i}.py", "additions": 28, "deletions": 1} for i in range(4)] +
        [{"path": f"tests/test_{i}.py", "additions": 28, "deletions": 1} for i in range(4)],
        prior_threads=0,
    )
    assert 12 <= pred <= 18, f"v0.5.12 cross-check: predicted={pred}, marker=15"


def test_cross_check_logmind_pr_90_v0513():
    """PR #90: turns_estimated=30, max_turns=36, files=16, +1099/-30, threads=0."""
    pred = predict_estimated_turns(
        [{"path": f"src/x{i}.py", "additions": 70, "deletions": 2} for i in range(6)] +
        [{"path": f"tests/test_{i}.py", "additions": 70, "deletions": 2} for i in range(6)] +
        [{"path": f"docs/x{i}.md", "additions": 70, "deletions": 2} for i in range(2)] +
        [{"path": ".github/workflows/x.yml", "additions": 70, "deletions": 2}] +
        [{"path": "pyproject.toml", "additions": 70, "deletions": 2}],
        prior_threads=0,
    )
    assert 25 <= pred <= 35, f"v0.5.13 cross-check: predicted={pred}, marker=30"


# ---------------------------------------------------------------------------
# Wrapper
# ---------------------------------------------------------------------------


def test_predict_returns_dict():
    out = predict(
        [{"path": "src/foo.py", "additions": 50, "deletions": 0}] * 3,
    )
    assert out["estimated"] == 14
    assert out["max_turns"] == 17
    assert out["is_trivial"] is False


def test_predict_max_turns_for_est_14_is_17():
    """(14*12+9)//10 = (168+9)//10 = 17. Above floor 15, below ceiling 60."""
    assert predict_max_turns(14) == 17
