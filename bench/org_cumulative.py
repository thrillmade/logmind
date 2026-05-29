"""Org-cumulative angle (real impl).

Same data source as :mod:`bench.per_session` (real agent session logs at
``~/.claude/projects/*/*.jsonl``), but rolls up DIFFERENTLY: instead of
averaging ``net_pct`` across sessions, we **sum bytes across all
sessions and all repos** to produce one global cumulative ``net_pct``.

The angle is informational (same exclusion from the exit-gate as
``per-session``): the ``git log --oneline -100`` baseline is too thin
to interpret pos/neg as a quality signal. The load-bearing data is
``per_repo_share`` — which consuming repos contribute most of the
sampled byte volume — used by Step 4 validation to spot per-repo
outliers (a single misconfigured consumer >2× the median share would
indicate a cache-key regression).

Closes the last Q7-logmind bench stub. See the Phase 0.5 §2 entry in
``CHANGELOG.md`` for the ship/defer rationale.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path

from .per_session import (
    _bucket,
    _git_equivalent_bytes,
    _GIT_BASELINE_CACHE,
    _is_logmind_repo,
    _session_cwd,
    _session_paths,
    _walk_reads,
)


@dataclass
class OrgCumulativeResult:
    label: str
    net_pct: float | None
    stub: bool
    # All sessions whose cwd is a logmind repo. Matches per_session's
    # ``sessions_sampled`` semantic so the cross-check invariant
    # (``org_cumulative.sessions_sampled == per_session.sessions_sampled``
    # when run against the same home dir) holds.
    sessions_sampled: int = 0
    # Subset of ``sessions_sampled`` that actually contributed bytes
    # (>0 decision-doc reads AND non-zero git baseline). Matches
    # per_session's ``sessions_with_decision_reads``. These are the
    # sessions whose bytes flow into ``total_logmind_bytes``.
    sessions_contributing: int = 0
    repos_sampled: int = 0
    total_logmind_bytes: int = 0
    total_git_bytes: int = 0
    per_repo_share: dict[str, float] = field(default_factory=dict)


def run_org_cumulative(home: Path | None = None) -> OrgCumulativeResult:
    """Walk session logs, sum bytes across all (session, repo) pairs,
    produce one global cumulative ``net_pct`` + per-repo share.

    ``home`` is injectable for tests (mirrors :func:`per_session.run_per_session`).
    """
    _GIT_BASELINE_CACHE.clear()

    paths = _session_paths(home=home)
    if not paths:
        return OrgCumulativeResult(
            label="no sessions found",
            net_pct=None,
            stub=True,
        )

    sessions_sampled = 0
    sessions_contributing = 0
    total_logmind_bytes = 0
    total_git_bytes = 0
    repo_bytes: dict[str, int] = {}
    repos_with_reads: set[str] = set()

    for path in paths:
        cwd = _session_cwd(path)
        if cwd is None or not _is_logmind_repo(cwd):
            continue
        sessions_sampled += 1
        session_logmind_bytes = 0
        for fp, nbytes in _walk_reads(path):
            if _bucket(fp, cwd) is None:
                continue
            session_logmind_bytes += nbytes
        if session_logmind_bytes == 0:
            continue
        # Each session contributes one git-baseline read (cached per repo
        # in ``_GIT_BASELINE_CACHE``). For org-cumulative, every session
        # in the no-logmind world would pay that cost — so we add it on
        # every contributing session, not just once per repo.
        git_bytes = _git_equivalent_bytes(cwd)
        if git_bytes == 0:
            # Same divide-by-zero defence as per_session: skip the
            # session entirely rather than fake a "free" git baseline.
            continue
        sessions_contributing += 1
        total_logmind_bytes += session_logmind_bytes
        total_git_bytes += git_bytes
        repo_key = str(cwd)
        repo_bytes[repo_key] = repo_bytes.get(repo_key, 0) + session_logmind_bytes
        repos_with_reads.add(repo_key)

    if sessions_sampled == 0:
        return OrgCumulativeResult(
            label="no logmind repos in sampled sessions",
            net_pct=None,
            stub=True,
        )
    if total_git_bytes == 0:
        # Every contributing session had a zero baseline — same condition
        # per_session guards. Without a usable baseline we have no
        # denominator and reporting any ``net_pct`` would be fake signal.
        return OrgCumulativeResult(
            label="no decision-doc reads in sampled sessions",
            net_pct=None,
            stub=True,
            sessions_sampled=sessions_sampled,
        )

    net_pct = ((total_logmind_bytes - total_git_bytes) / total_git_bytes) * 100.0
    per_repo_share = {
        repo: repo_bytes[repo] / total_logmind_bytes
        for repo in repo_bytes
    }
    repos_sampled = len(repos_with_reads)
    label = (
        f"bytes amortized (across {repos_sampled} repos, "
        f"{total_logmind_bytes // 1024} KB logmind / "
        f"{total_git_bytes // 1024} KB git)"
    )

    return OrgCumulativeResult(
        label=label,
        net_pct=net_pct,
        stub=False,
        sessions_sampled=sessions_sampled,
        sessions_contributing=sessions_contributing,
        repos_sampled=repos_sampled,
        total_logmind_bytes=total_logmind_bytes,
        total_git_bytes=total_git_bytes,
        per_repo_share=per_repo_share,
    )


def run_org_cumulative_stub() -> OrgCumulativeResult:
    """Back-compat shim — kept so callers that imported the stub
    directly don't break. The registry in ``bench/__main__.py`` has
    been switched to ``run_org_cumulative``."""
    return run_org_cumulative()
