"""Org cumulative angle (STUB).

The plan: aggregate per-call + per-session measurements across 5
consuming repos over 30 days. Total bytes-saved minus total bytes-spent
must be net negative.

Initial stub returns null `net_pct`. Real implementation depends on
per_session.py shipping first (it consumes session-log data) AND on a
mechanism to walk 5 consuming repos' Git history for logmind-log usage
frequency.

Ship the stub now so the 4-angle frame is complete in the dashboard;
real implementation lands as a follow-up PR.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass
class OrgCumulativeResult:
    label: str
    net_pct: float | None
    stub: bool


def run_org_cumulative_stub() -> OrgCumulativeResult:
    return OrgCumulativeResult(
        label="net (cumulative, stub)",
        net_pct=None,
        stub=True,
    )
