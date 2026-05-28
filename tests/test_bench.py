"""Tests for `bench/` — the Q7-logmind internal measurement tool.

We test the aggregation + exit-code logic (deterministic) and smoke-run
the per-call harness (slow — tempdirs + subprocesses). The per-call
smoke test asserts the NET-SAVER invariant: every realistic command
pair we benchmark must be ≤ break-even vs git equivalent.

If this test fails, Q7-logmind is broken — a logmind change shipped
that flips us into being a token spender. P0.
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

import pytest


REPO_ROOT = Path(__file__).resolve().parent.parent


def test_bench_module_importable():
    """`python -m bench` is the entry point. Module must import cleanly."""
    from bench import per_call, worst_case, per_session, org_cumulative  # noqa: F401


def test_per_call_returns_saver_or_break_even_for_log_command():
    """The `log` command pair must be a net saver. This is the
    foundational guarantee — if `logmind log` costs more bytes than
    `git add + git commit`, every agent invocation makes us a spender.
    """
    from bench.per_call import _pairs, _run_pair

    log_pair = next((p for p in _pairs() if p.name == "log"), None)
    assert log_pair is not None, "the `log` pair must exist"

    result = _run_pair(log_pair)
    assert result["logmind_exit"] == 0, f"logmind log failed: {result}"
    assert result["git_exit"] == 0, f"git equivalent failed: {result}"
    # Must be a saver (negative net %) — the user-stated Q7 guarantee.
    assert result["net_pct"] <= 0, (
        f"`logmind log` is a token SPENDER (net {result['net_pct']:.1f}%) "
        f"vs git equivalent — Q7-logmind violated"
    )


def test_worst_case_is_saver():
    """Worst-case (fresh session, single `logmind log`, never reads
    back) must still be ≤ break-even. The HARDEST guarantee — if this
    breaks, logmind is fundamentally a net spender no matter what
    amortization happens."""
    from bench.worst_case import run_worst_case

    result = run_worst_case()
    assert result.net_pct <= 0, (
        f"worst-case is a token SPENDER ({result.net_pct:.1f}%) — "
        f"Q7-logmind fundamentally broken"
    )


def test_per_session_returns_stub_with_null_net_pct():
    """Stub angles return net_pct=None so they don't gate the exit
    code (they report as 'not yet implemented' in human output, and
    they're omitted from the spender check)."""
    from bench.per_session import run_per_session_stub
    result = run_per_session_stub()
    assert result.stub is True
    assert result.net_pct is None


def test_org_cumulative_returns_stub_with_null_net_pct():
    from bench.org_cumulative import run_org_cumulative_stub
    result = run_org_cumulative_stub()
    assert result.stub is True
    assert result.net_pct is None


def test_main_exits_zero_when_all_angles_saver_or_stub():
    """Top-level: when every non-stub angle is a saver, `python -m bench`
    exits 0. CI uses this as the gate."""
    proc = subprocess.run(
        [sys.executable, "-m", "bench"],
        cwd=REPO_ROOT, capture_output=True, text=True, check=False,
    )
    assert proc.returncode == 0, (
        f"bench exited non-zero (Q7 fail) — stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}"
    )
    assert "ok: 4-angle Q7-logmind compliance" in proc.stdout


def test_main_json_output_is_parseable():
    """`--json` emits machine-readable output for downstream consumers."""
    proc = subprocess.run(
        [sys.executable, "-m", "bench", "--json"],
        cwd=REPO_ROOT, capture_output=True, text=True, check=False,
    )
    assert proc.returncode == 0
    parsed = json.loads(proc.stdout)
    assert "per-call" in parsed
    assert "worst-case" in parsed
    assert "per-session" in parsed
    assert "org-cumulative" in parsed
    # Per-call should have its pair-level detail.
    assert "pairs" in parsed["per-call"]
    pair_names = {p["name"] for p in parsed["per-call"]["pairs"]}
    assert "log" in pair_names


def test_main_runs_single_angle():
    """`python -m bench per-call` runs only that angle. Useful for fast
    iteration on a specific surface."""
    proc = subprocess.run(
        [sys.executable, "-m", "bench", "per-call"],
        cwd=REPO_ROOT, capture_output=True, text=True, check=False,
    )
    assert proc.returncode == 0
    # Only the per-call angle appears; the others don't.
    assert "per-call" in proc.stdout
    assert "worst-case" not in proc.stdout


def test_baseline_diff_detects_regression(tmp_path):
    """Pass a previous --json run via --baseline; bench reports the
    per-angle diff. Used to detect regressions in CI."""
    # Create a synthetic baseline showing a -40% saver.
    baseline = {
        "per-call": {"label": "x", "net_pct": -40.0, "pairs": []},
        "worst-case": {"label": "x", "net_pct": -50.0},
        "per-session": {"label": "x", "net_pct": None, "stub": True},
        "org-cumulative": {"label": "x", "net_pct": None, "stub": True},
    }
    baseline_path = tmp_path / "baseline.json"
    baseline_path.write_text(json.dumps(baseline), encoding="utf-8")
    proc = subprocess.run(
        [sys.executable, "-m", "bench", "--baseline", str(baseline_path)],
        cwd=REPO_ROOT, capture_output=True, text=True, check=False,
    )
    assert proc.returncode == 0
    assert "vs baseline:" in proc.stdout
