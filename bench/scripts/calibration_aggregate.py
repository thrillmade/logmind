#!/usr/bin/env python3
"""Stream 1 Path C: aggregate Layer 1.5 calibration markers across consumer repos.

Fetches every PR comment from the GH org's clud-bug-using repos, filters to
the ``<!-- clud-bug-calibration: ... -->`` HTML markers that clud-bug
v0.6.25+ emits on every review summary, and computes the distribution of
estimator outputs.

Background:
  clud-bug v0.6.25 (Smart Budget Phase 1) introduced a jq-driven line-based
  budget estimator + a calibration marker that records the estimator's
  output on every PR review:

      <!-- clud-bug-calibration: turns_estimated=N, max_turns=M,
           files=F, lines_added=A, lines_deleted=D, threads=T -->

  Path C of the Stream 1 (calibration acceleration) work watches these
  markers accumulate via the natural PR flow and computes the distribution
  of estimator outputs across the org. Combined with Path A (action-log
  scrape for actual turn usage) and Path B (synthetic stress PRs), gates
  the clud-bug v0.6.28 L5 auto-retry ship decision.

  This script is the Path C piece — runnable today against whatever data
  has accumulated, no waiting required.

Usage:
  python -m bench.scripts.calibration_aggregate

Configuration:
  GH org + repo list lives in REPOS below. To add a repo, append its name.
  The marker regex is locked to the v0.6.25 format; future format changes
  bump the version-pinned regex.

Output:
  - Per-repo counts to stderr
  - Distribution stats + per-row table to stdout
  - One JSON object per marker (newline-delimited) for downstream tooling
"""
from __future__ import annotations

import json
import re
import subprocess
import sys
from collections import defaultdict
from statistics import median, quantiles
from typing import Any

GH_ORG = "thrillmade"
REPOS = (
    "logmind",
    "clud-bug",
    "tokenomics",
    "agent-skills",
    "reporulez",
    "rezgen",
)

# v0.6.25+ Layer 1.5 marker. Comment is emitted by the post-step renderer
# on every clud-bug-review summary. Format is fixed; any format change is a
# clud-bug release bump + a corresponding bench/ change.
MARKER_RE = re.compile(
    r"<!--\s*clud-bug-calibration:\s*"
    r"turns_estimated=(\d+),\s*"
    r"max_turns=(\d+),\s*"
    r"files=(\d+),\s*"
    r"lines_added=(\d+),\s*"
    r"lines_deleted=(\d+),\s*"
    r"threads=(\d+)\s*-->"
)


def fetch_markers(repo: str) -> list[dict[str, Any]]:
    """Return every calibration marker found in ``repo``'s PR comments.

    Uses ``gh api --paginate`` to walk all pages; emits one record per
    matching marker. Comments without markers are silently skipped.
    Returns an empty list on any gh / API failure (best-effort by design).
    """
    out = subprocess.run(
        [
            "gh", "api",
            f"repos/{GH_ORG}/{repo}/issues/comments?per_page=100",
            "--paginate",
            "--jq", ".[] | {body, html_url, created_at}",
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    if out.returncode != 0:
        print(f"[{repo}] gh api failed: {out.stderr.strip()}", file=sys.stderr)
        return []

    markers: list[dict[str, Any]] = []
    # `gh api --paginate --jq` emits one JSON-line per matched object.
    for line in out.stdout.splitlines():
        if not line.strip():
            continue
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            continue
        body = obj.get("body") or ""
        m = MARKER_RE.search(body)
        if not m:
            continue
        markers.append({
            "repo": repo,
            "comment_url": obj.get("html_url"),
            "created_at": obj.get("created_at"),
            "turns_estimated": int(m.group(1)),
            "max_turns": int(m.group(2)),
            "files": int(m.group(3)),
            "lines_added": int(m.group(4)),
            "lines_deleted": int(m.group(5)),
            "threads": int(m.group(6)),
        })
    return markers


def _format_distribution(markers: list[dict[str, Any]]) -> str:
    """Compute + format the cap/est ratio distribution. Returns a multi-line string."""
    ratios = [
        m["max_turns"] / m["turns_estimated"]
        for m in markers
        if m["turns_estimated"] > 0
    ]
    if not ratios:
        return ""

    lines = [
        "cap / est ratio (Layer 1 safety margin = 1.2x rounded up to integer turn):",
        f"  p50: {median(ratios):.2f}",
    ]
    if len(ratios) >= 4:
        q = quantiles(ratios, n=10)
        lines.append(f"  p90: {q[-1]:.2f}")
    else:
        lines.append(f"  p90: N/A (need >= 4 samples)")
    lines.append(f"  min: {min(ratios):.2f}   max: {max(ratios):.2f}")
    return "\n".join(lines)


def _format_per_pr_table(markers: list[dict[str, Any]]) -> str:
    """Render a sorted-by-estimated-turns table of (repo, shape, est, cap)."""
    rows = ["Estimated turns by PR size (sorted ascending):",
            f"  {'repo':>14}  {'files':>5}  {'+lines':>6}  {'-lines':>6}  "
            f"{'threads':>7}  {'est':>3}  {'cap':>3}"]
    for m in sorted(markers, key=lambda x: x["turns_estimated"]):
        rows.append(
            f"  {m['repo']:>14}  {m['files']:>5}  {m['lines_added']:>6}  "
            f"{m['lines_deleted']:>6}  {m['threads']:>7}  "
            f"{m['turns_estimated']:>3}  {m['max_turns']:>3}"
        )
    return "\n".join(rows)


def main() -> int:
    all_markers: list[dict[str, Any]] = []
    per_repo: dict[str, int] = defaultdict(int)

    print("Fetching calibration markers from", file=sys.stderr)
    for repo in REPOS:
        ms = fetch_markers(repo)
        all_markers.extend(ms)
        per_repo[repo] = len(ms)
        print(f"  {repo:>14}  N = {len(ms)}", file=sys.stderr)

    print(f"\n=== Stream 1 Path C — calibration markers (N = {len(all_markers)}) ===\n")

    print("Per-repo counts:")
    for repo in REPOS:
        print(f"  {repo:>14}: {per_repo[repo]}")

    if not all_markers:
        print("\nNo markers yet. Need at least one v0.6.25+ clud-bug-review to land.")
        return 0

    dist = _format_distribution(all_markers)
    if dist:
        print(f"\n{dist}")

    print(f"\n{_format_per_pr_table(all_markers)}")

    # JSON output for downstream tooling — one object per line.
    print("\n--- JSON rows (one per marker) ---")
    for m in all_markers:
        print(json.dumps(m, separators=(",", ":")))

    return 0


if __name__ == "__main__":
    sys.exit(main())
