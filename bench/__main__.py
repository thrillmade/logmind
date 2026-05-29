"""`python -m bench` entry point.

Runs the four Q7-logmind angles and aggregates. Exits non-zero if any
angle flips negative (i.e., logmind is spending more bytes than it
saves on that surface) — CI gates on this exit code.
"""

from __future__ import annotations

import argparse
import json
import sys
from dataclasses import asdict
from typing import Any

from .per_call import run_per_call
from .worst_case import run_worst_case
from .per_session import run_per_session
from .org_cumulative import run_org_cumulative


ANGLES = {
    "per-call": run_per_call,
    "worst-case": run_worst_case,
    "per-session": run_per_session,
    "org-cumulative": run_org_cumulative,
}


def main() -> int:
    parser = argparse.ArgumentParser(
        prog="python -m bench",
        description="logmind-bench — Q7-logmind enforcement (4-angle net-saver measurement).",
    )
    parser.add_argument(
        "angle",
        nargs="?",
        default=None,
        choices=list(ANGLES.keys()),
        help="Run only one angle (default: all four).",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Emit JSON instead of human-readable output.",
    )
    parser.add_argument(
        "--baseline",
        type=str,
        default=None,
        help="Path to a previous --json output to diff against (regression detection).",
    )
    args = parser.parse_args()

    angles = [args.angle] if args.angle else list(ANGLES.keys())
    results: dict[str, Any] = {}
    for name in angles:
        result = ANGLES[name]()
        results[name] = asdict(result) if hasattr(result, "__dataclass_fields__") else result

    if args.baseline:
        try:
            with open(args.baseline, "r", encoding="utf-8") as f:
                baseline = json.load(f)
            results["_baseline_diff"] = _diff_baseline(baseline, results)
        except OSError as e:
            print(f"!  baseline not loadable: {e}", file=sys.stderr)

    if args.json:
        print(json.dumps(results, indent=2))
    else:
        print(_format_human(results))

    # Exit non-zero if any GATING non-stub angle is a NET SPENDER.
    # Stubs (net_pct=None) don't gate. Per-session AND org-cumulative
    # are informational only — they share the ``git log --oneline -100``
    # baseline, which is conceptually too thin (agents would not get
    # equivalent context from raw git log alone in the no-logmind world),
    # so the absolute ``net_pct`` isn't a quality signal. Per-session's
    # value is per-file shares (``per_file_share``, ``agents_md_block_share``)
    # which gate 0.B.5 / 0.B.6; org-cumulative's value is ``per_repo_share``
    # which surfaces per-consumer outliers for Step 4 validation.
    informational = {"per-session", "org-cumulative"}
    any_spender = any(
        (r.get("net_pct") or 0) > 0
        for name, r in results.items()
        if isinstance(r, dict)
        and r.get("net_pct") is not None
        and name not in informational
    )
    return 1 if any_spender else 0


def _format_human(results: dict[str, Any]) -> str:
    lines = ["ok: 4-angle Q7-logmind compliance" if not _has_spender(results) else "FAIL: Q7-logmind net-spender detected"]
    informational = {"per-session", "org-cumulative"}
    for name, r in results.items():
        if name.startswith("_"):
            continue
        if not isinstance(r, dict):
            continue
        pct = r.get("net_pct")
        label = r.get("label", name)
        if pct is None:
            lines.append(f"  {name:<14} (stub — not yet implemented)")
            continue
        if name in informational:
            # Same baseline-thinness concern for both per-session and
            # org-cumulative — pos/neg net_pct isn't a quality signal.
            # The load-bearing data is in ``label`` + per-repo / per-file
            # shares (Step 4 validation + 0.B.5/0.B.6 gating).
            verdict = "ℹ info (Step 4 / 0.B.5 / 0.B.6 inputs)"
        else:
            verdict = "✅ saver" if pct < 0 else "❌ spender"
        sign = "" if pct < 0 else "+"
        lines.append(f"  {name:<14} {sign}{pct:.0f}% {label:<28} {verdict}")
    if results.get("_baseline_diff"):
        lines.append("")
        lines.append("  vs baseline:")
        for k, v in results["_baseline_diff"].items():
            sign = "+" if v > 0 else ""
            lines.append(f"    {k:<14} {sign}{v:.1f}pp")
    return "\n".join(lines) + "\n"


def _has_spender(results: dict[str, Any]) -> bool:
    """Header verdict — informational angles (per-session, org-cumulative)
    are excluded so the human-readable banner matches the exit-gate
    logic."""
    informational = {"per-session", "org-cumulative"}
    return any(
        isinstance(r, dict) and (r.get("net_pct") or 0) > 0
        for name, r in results.items()
        if name not in informational
    )


def _diff_baseline(baseline: dict[str, Any], current: dict[str, Any]) -> dict[str, float]:
    """Per-angle delta vs baseline. Skip angles where either side has
    a null net_pct (stubs) — they're nonsensical to subtract."""
    diff: dict[str, float] = {}
    for name, r in current.items():
        if name.startswith("_") or not isinstance(r, dict):
            continue
        b = baseline.get(name, {})
        if not isinstance(b, dict):
            continue
        cur = r.get("net_pct")
        prev = b.get("net_pct")
        if cur is None or prev is None:
            continue
        diff[name] = cur - prev
    return diff


if __name__ == "__main__":
    raise SystemExit(main())
