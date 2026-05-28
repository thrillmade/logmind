"""Per-session amortization angle (STUB).

The plan: sample 10 real agent sessions from `~/.claude/projects/*/sessions/`
and count bytes saved by reading `docs/decisions.md` / `timeline.md` /
`file-structure.md` vs reading the raw `git log --grep` equivalent.

Initial stub: return null `net_pct` so the angle reports as "not yet
implemented" in the human-readable output. Real implementation lands as
a follow-up PR when the session-log path + grep-proxy logic is
designed.

Why ship a stub: the 4-angle dashboard frame is more valuable than
waiting for all 4 to be perfect. Per-call + worst-case (both real
today) cover the hard guarantee (logmind never costs more than git).
Per-session amortization is the "compounds across reads" angle — nice
to have, not load-bearing for Q7.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass
class PerSessionResult:
    label: str
    net_pct: float | None
    stub: bool


def run_per_session_stub() -> PerSessionResult:
    return PerSessionResult(
        label="bytes amortized (stub)",
        net_pct=None,
        stub=True,
    )
