"""Worst-case angle: prove logmind is still ≤ break-even even when no
amortization happens.

The worst-case for logmind is: fresh session, single `logmind log`,
agent never reads back any of the generated artifacts. If logmind STILL
emits ≤ the bytes of the manual git equivalent in that scenario, we
have a HARD GUARANTEE that using logmind never costs more than not
using it.

This is a synthetic stress test. It uses the same per-call pair
mechanism but only the `log` command (the most common use). Output is
the same shape as per_call so it can plug into the same dashboard.
"""

from __future__ import annotations

from dataclasses import dataclass

from .per_call import _pairs, _run_pair


@dataclass
class WorstCaseResult:
    label: str
    net_pct: float
    detail: dict


def run_worst_case() -> WorstCaseResult:
    """Run only the `log` pair (the most common worst-case).

    PR #74 fix: if the logmind invocation failed (lm_exit != 0), return
    net_pct=None so the angle reports as unable-to-measure rather than
    claiming the failure was a saver. Aligns with run_per_call.
    """
    log_pair = next((p for p in _pairs() if p.name == "log"), None)
    if log_pair is None:
        return WorstCaseResult(label="even on never-read", net_pct=None, detail={})
    pair_result = _run_pair(log_pair)
    if pair_result.get("failed", False):
        return WorstCaseResult(
            label="even on never-read (logmind log FAILED)",
            net_pct=None,
            detail=pair_result,
        )
    return WorstCaseResult(
        label="even on never-read",
        net_pct=pair_result["net_pct"],
        detail=pair_result,
    )
