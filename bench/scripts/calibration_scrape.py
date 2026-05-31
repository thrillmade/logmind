#!/usr/bin/env python3
"""Stream 1 Path A — historical clud-bug-review scrape.

Scrapes every completed clud-bug-review run since v0.6.13 across the 6
thrillmade repos, extracts actual turn usage (proxy: `tool_use` count
in action log), fetches per-PR diff stats, retroactively applies the
v0.6.25 Layer 1 estimator (via :mod:`bench.scripts.calibration_layer1`),
and reports the distribution of ``actual / predicted``.

**Caveats** (call out in any report):

- Historical runs used pre-Smart-Budget prompts (no Layer 2 in-prompt
  budget, no Layer 3 mid-review check-in). Actual usage likely runs
  higher than post-v0.6.25 reality. Treat as **upper bound on
  estimator error**, not exact validation.
- ``tool_use`` count is a LOWER BOUND on actual turns (parallel tool
  calls in one assistant message = 1 turn). Net: ratio is ±~20%
  noisy. Good for magnitude check, not p90-precise.

Usage:
    python -m bench.scripts.calibration_scrape [--repo REPO] [--limit N]

Cache: `bench/scripts/.cache/scrape_<repo>_<run_id>.json` (gitignored).
Re-runs are fast after first scrape.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path
from statistics import median, quantiles
from typing import Any

from bench.scripts.calibration_layer1 import predict_estimated_turns

GH_ORG = "thrillmade"
REPOS = (
    "logmind",
    "clud-bug",
    "tokenomics",
    "agent-skills",
    "reporulez",
    "rezgen",
)

_CACHE_DIR = Path(__file__).parent / ".cache"

# v0.6.13 shipped 2026-05-28. Before that there was no Smart Budget
# instrumentation worth scraping. Filter by run created_at.
SINCE_DATE = "2026-05-28"

# Regex helpers for parsing the action log.
_TOOL_USE_RE = re.compile(r'"type"\s*:\s*"tool_use"')
_STOP_REASON_RE = re.compile(r'"stop_reason"\s*:\s*"([^"]+)"')
_MAX_TURNS_INPUT_RE = re.compile(r"--max-turns\s+(\d+)")


def _gh_json(args: list[str], default: Any = None) -> Any:
    """Run gh + return JSON output, or `default` on failure."""
    try:
        result = subprocess.run(
            ["gh"] + args,
            capture_output=True,
            text=True,
            check=True,
            timeout=60,
        )
        return json.loads(result.stdout) if result.stdout.strip() else default
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, json.JSONDecodeError) as e:
        print(f"[gh-json] failed: {' '.join(args[:4])} ... — {e}", file=sys.stderr)
        return default


def list_clud_bug_review_runs(repo: str, limit: int = 200) -> list[dict]:
    """List recent clud-bug-review.yml runs for a repo.

    Filters to PR events only (drops schedule/workflow_dispatch).
    Includes failed runs (per Q7 measurement integrity — Anthropic
    bills regardless of conclusion).
    """
    runs = _gh_json([
        "api",
        f"repos/{GH_ORG}/{repo}/actions/workflows/clud-bug-review.yml/runs"
        f"?per_page={limit}&event=pull_request&created=>={SINCE_DATE}",
        "--jq",
        ".workflow_runs[] | {id, run_number, status, conclusion, created_at, "
        "event, head_branch, pull_requests: [.pull_requests[].number]}",
    ], default=None)
    if runs is None:
        return []
    # `gh api --jq` over an array emits one JSON-line per element.
    out = []
    for line in (runs if isinstance(runs, str) else "").splitlines():
        if not line.strip():
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    # If --jq returned a single object (not the per-line stream), wrap:
    if not out and isinstance(runs, dict):
        out = [runs]
    return out


def list_runs_alt(repo: str, limit: int) -> list[dict]:
    """Alt path using `gh run list --json` — more reliable for parsing."""
    result = subprocess.run(
        [
            "gh", "run", "list",
            "--repo", f"{GH_ORG}/{repo}",
            "--workflow", "clud-bug-review.yml",
            "--limit", str(limit),
            "--json", "databaseId,number,status,conclusion,createdAt,event,headBranch,headSha",
        ],
        capture_output=True,
        text=True,
        check=False,
        timeout=60,
    )
    if result.returncode != 0:
        print(f"[{repo}] gh run list failed: {result.stderr.strip()}", file=sys.stderr)
        return []
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        return []
    # Filter to pull_request events; drop schedule/manual.
    return [r for r in data if r.get("event") == "pull_request"]


def fetch_run_log(repo: str, run_id: int) -> str | None:
    """Fetch a run's full log; cache locally to avoid re-fetching.

    Uses `gh api .../logs` (returns ZIP) + extracts in-memory rather than
    `gh run view --log` (returns empty for some runs — silent reliability bug).
    """
    import io
    import zipfile

    _CACHE_DIR.mkdir(parents=True, exist_ok=True)
    cache_file = _CACHE_DIR / f"scrape_{repo}_{run_id}.txt"
    if cache_file.exists():
        return cache_file.read_text(encoding="utf-8", errors="ignore")
    try:
        # `gh api -H Accept: raw .../logs` returns the ZIP content
        # directly. We capture as bytes (not text) so the binary stays
        # intact for zipfile parsing.
        result = subprocess.run(
            [
                "gh", "api",
                "-H", "Accept: application/vnd.github.v3.raw",
                f"repos/{GH_ORG}/{repo}/actions/runs/{run_id}/logs",
            ],
            capture_output=True,
            check=False,
            timeout=120,
        )
    except subprocess.TimeoutExpired:
        print(f"[{repo}/{run_id}] log fetch timed out", file=sys.stderr)
        return None
    if result.returncode != 0 or len(result.stdout) < 100:
        # Skipped / unavailable logs. Cache empty to avoid retry.
        cache_file.write_text("", encoding="utf-8")
        return ""

    # Extract every file in the ZIP, concatenate into one text blob.
    try:
        zf = zipfile.ZipFile(io.BytesIO(result.stdout))
        parts = []
        for name in zf.namelist():
            if name.endswith(".txt"):
                parts.append(zf.read(name).decode("utf-8", errors="ignore"))
        log_text = "\n".join(parts)
    except zipfile.BadZipFile:
        # Some endpoints return raw text instead of ZIP. Fall back.
        log_text = result.stdout.decode("utf-8", errors="ignore")

    cache_file.write_text(log_text, encoding="utf-8")
    return log_text


def parse_log(log: str) -> dict:
    """Extract from log: tool_use count, stop_reason, max_turns input."""
    if not log:
        return {"tool_use_count": 0, "stop_reason": None, "max_turns_input": None}

    tool_uses = len(_TOOL_USE_RE.findall(log))

    # Last non-null stop_reason (multiple stop_reason: null lines appear
    # during streaming; the final completion has a real value).
    stop_reasons = [m for m in _STOP_REASON_RE.findall(log) if m and m != "null"]
    stop_reason = stop_reasons[-1] if stop_reasons else None

    mt_match = _MAX_TURNS_INPUT_RE.search(log)
    max_turns_input = int(mt_match.group(1)) if mt_match else None

    return {
        "tool_use_count": tool_uses,
        "stop_reason": stop_reason,
        "max_turns_input": max_turns_input,
    }


def fetch_pr_files(repo: str, pr_number: int) -> list[dict] | None:
    """Get per-file diff stats for a PR."""
    try:
        result = subprocess.run(
            [
                "gh", "pr", "view", str(pr_number),
                "--repo", f"{GH_ORG}/{repo}",
                "--json", "files",
            ],
            capture_output=True,
            text=True,
            check=False,
            timeout=30,
        )
        if result.returncode != 0:
            return None
        data = json.loads(result.stdout)
        return data.get("files", [])
    except (subprocess.TimeoutExpired, json.JSONDecodeError):
        return None


def get_pr_for_run(repo: str, run: dict) -> int | None:
    """Resolve the PR number for a workflow run. Tries multiple sources."""
    # Branch name often matches the head_branch on the PR.
    branch = run.get("headBranch")
    if not branch or branch == "main":
        return None
    # Look up open + closed PRs by branch.
    result = subprocess.run(
        [
            "gh", "pr", "list",
            "--repo", f"{GH_ORG}/{repo}",
            "--head", branch,
            "--state", "all",
            "--limit", "5",
            "--json", "number",
        ],
        capture_output=True,
        text=True,
        check=False,
        timeout=30,
    )
    if result.returncode != 0:
        return None
    try:
        data = json.loads(result.stdout)
        return data[0]["number"] if data else None
    except (json.JSONDecodeError, KeyError, IndexError):
        return None


def scrape_repo(repo: str, limit: int) -> list[dict]:
    """Scrape one repo's clud-bug-review runs. Returns per-run records."""
    print(f"[{repo}] listing runs…", file=sys.stderr)
    runs = list_runs_alt(repo, limit)
    print(f"[{repo}]   {len(runs)} runs", file=sys.stderr)

    records = []
    for i, run in enumerate(runs):
        run_id = run["databaseId"]
        print(f"[{repo}] ({i+1}/{len(runs)}) run {run_id}…", file=sys.stderr)

        log = fetch_run_log(repo, run_id)
        log_parsed = parse_log(log or "")

        # Skip auto-skipped (0.0.W²) runs — no review happened.
        if log_parsed["tool_use_count"] == 0 and run.get("conclusion") == "success":
            continue

        pr_number = get_pr_for_run(repo, run)
        if pr_number is None:
            continue

        files = fetch_pr_files(repo, pr_number)
        if files is None:
            continue

        predicted = predict_estimated_turns(files, prior_threads=0)

        records.append({
            "repo": repo,
            "run_id": run_id,
            "pr_number": pr_number,
            "created_at": run["createdAt"],
            "status": run.get("status"),
            "conclusion": run.get("conclusion"),
            "stop_reason": log_parsed["stop_reason"],
            "max_turns_input": log_parsed["max_turns_input"],
            "actual_tool_uses": log_parsed["tool_use_count"],
            "predicted_estimated_turns": predicted,
            "file_count": len(files),
            "lines_added": sum(f.get("additions", 0) for f in files),
            "lines_deleted": sum(f.get("deletions", 0) for f in files),
        })

    return records


def summarize(records: list[dict]) -> None:
    """Emit distribution + outlier summary."""
    if not records:
        print("\nNo records — nothing to summarize.")
        return

    # Compute ratio: actual / predicted.
    rated = [
        {**r, "ratio": r["actual_tool_uses"] / r["predicted_estimated_turns"]}
        for r in records
        if r["predicted_estimated_turns"] > 0
    ]

    ratios = [r["ratio"] for r in rated]
    print(f"\n=== Path A historical scrape — {len(rated)} runs ===\n")

    print("Distribution: actual_tool_uses / predicted_estimated_turns")
    print(f"  p50: {median(ratios):.2f}")
    if len(ratios) >= 4:
        q = quantiles(ratios, n=10)
        print(f"  p75: {q[6]:.2f}")
        print(f"  p90: {q[8]:.2f}")
    if len(ratios) >= 100:
        q99 = quantiles(ratios, n=100)
        print(f"  p99: {q99[98]:.2f}")
    print(f"  min: {min(ratios):.2f}   max: {max(ratios):.2f}")

    # Per-repo breakdown
    by_repo: dict[str, list[float]] = defaultdict(list)
    for r in rated:
        by_repo[r["repo"]].append(r["ratio"])
    print("\nPer-repo p50 ratio:")
    for repo in sorted(by_repo):
        rs = by_repo[repo]
        print(f"  {repo:>14}: n={len(rs):>3}  p50={median(rs):.2f}")

    # Stop-reason distribution
    stop_reasons: dict[str | None, int] = defaultdict(int)
    for r in records:
        stop_reasons[r["stop_reason"]] += 1
    print("\nStop reasons:")
    for sr, n in sorted(stop_reasons.items(), key=lambda x: -x[1]):
        print(f"  {str(sr):>14}: {n}")

    # Outliers
    outliers = [r for r in rated if r["ratio"] > 2.0]
    cap_hits = [r for r in records if r["stop_reason"] == "max_turns"]
    print(f"\nOutliers (actual/predicted > 2.0): {len(outliers)}")
    for r in sorted(outliers, key=lambda x: -x["ratio"])[:10]:
        print(
            f"  {r['repo']}/PR#{r['pr_number']}: predicted={r['predicted_estimated_turns']} "
            f"actual={r['actual_tool_uses']} ratio={r['ratio']:.2f} "
            f"files={r['file_count']} +{r['lines_added']}/-{r['lines_deleted']}"
        )

    print(f"\nCap-hits (stop_reason=max_turns): {len(cap_hits)}")
    for r in cap_hits[:5]:
        print(
            f"  {r['repo']}/PR#{r['pr_number']}: max_turns_input={r['max_turns_input']} "
            f"actual={r['actual_tool_uses']}"
        )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", default=None, help="Scrape a single repo (default: all)")
    parser.add_argument("--limit", type=int, default=50, help="Max runs per repo")
    parser.add_argument("--out", default=None, help="Write JSON records to this path")
    args = parser.parse_args()

    repos = [args.repo] if args.repo else list(REPOS)

    all_records = []
    for repo in repos:
        all_records.extend(scrape_repo(repo, args.limit))

    summarize(all_records)

    if args.out:
        Path(args.out).write_text(json.dumps(all_records, indent=2), encoding="utf-8")
        print(f"\nWrote {len(all_records)} records to {args.out}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
