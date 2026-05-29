"""Per-session amortization angle.

Walks real agent session logs at ``~/.claude/projects/*/*.jsonl``,
counts the bytes the agent actually read from logmind-managed docs
(``docs/decisions.md``, ``docs/timeline.md``, ``docs/file-structure.md``,
``AGENTS.md``), and compares to a ``git log --oneline -100`` baseline
each session would have read in the no-logmind world.

Negative ``net_pct`` = saver (logmind docs cheaper than the git
equivalent). Positive = spender (P0 fix).

The output also surfaces per-file shares + an AGENTS.md-logmind-block
sub-share — these are the load-bearing metrics for the conditional
0.B.5 (decisions.md per-entry compact) and 0.B.6 (logmind-block trim)
candidates. The ship/defer rubric that consumes them lives in
``CHANGELOG.md`` (the per-version entries describe how the per-file
shares + ``agents_md_block_share`` thresholds gated each decision).

Edge cases all return ``stub=True`` / ``net_pct=None`` so the angle
reports as "not yet implemented"-shaped (and the exit gate doesn't
fire on a measurement that didn't run): no sessions found, no
logmind-initialised repos in the sampled sessions, zero decision-doc
reads, malformed JSONL line, ``git log`` failure.
"""

from __future__ import annotations

import json
import subprocess
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Iterator


# Sessions live at ``~/.claude/projects/<encoded-cwd>/<sessionId>.jsonl``.
# Subagent jsonls live one level deeper (``.../subagents/<id>.jsonl``)
# and memory MDs aren't .jsonl at all, so depth-2 glob covers exactly
# top-level session files.
_SESSION_GLOB = ".claude/projects/*/*.jsonl"

# Window + cap. We bound work because some users may have hundreds of
# sessions; the metric stabilises well before we need them all.
_MAX_AGE_DAYS = 30
_SESSION_CAP = 50

# The four buckets we count toward the logmind-doc cost.
_BUCKETS = (
    "docs/decisions.md",
    "docs/timeline.md",
    "docs/file-structure.md",
    "AGENTS.md",
)

# Anchor text the AGENTS.md logmind-block contains. We use a substring
# scan rather than the literal ``<!-- logmind-start -->`` marker so the
# metric still works if logmind ever swaps the marker (it's been
# ``logmind-start`` since v0.2.x, but be robust). If the heading text
# changes the same scan finds it.
_LOGMIND_BLOCK_ANCHORS = (
    "logmind log",         # always in the block (the commit-primitive heading)
    "logmind-start",       # the canonical marker
)


@dataclass
class PerSessionResult:
    label: str
    net_pct: float | None
    stub: bool
    sessions_sampled: int = 0
    sessions_with_decision_reads: int = 0
    per_file_share: dict[str, float] = field(default_factory=dict)
    agents_md_block_share: float = 0.0
    detail: list[dict] = field(default_factory=list)


def _session_paths(home: Path | None = None) -> list[Path]:
    """Return up to ``_SESSION_CAP`` session JSONLs, newest-first, in
    the last ``_MAX_AGE_DAYS`` days. ``home`` is injectable for tests.
    """
    home = home or Path.home()
    cutoff = time.time() - _MAX_AGE_DAYS * 86400
    paths: list[tuple[float, Path]] = []
    for p in home.glob(_SESSION_GLOB):
        try:
            mtime = p.stat().st_mtime
        except OSError:
            continue
        if mtime < cutoff:
            continue
        paths.append((mtime, p))
    paths.sort(key=lambda t: t[0], reverse=True)
    return [p for _, p in paths[:_SESSION_CAP]]


def _session_cwd(path: Path, scan_lines: int = 50) -> Path | None:
    """Find the session's working directory from the first event that
    carries a ``cwd`` field.

    Early JSONL events (session boot, snapshot metadata) don't include
    ``cwd`` — it shows up a few events in once the first user message
    or tool event lands. Scan the first ``scan_lines`` events; return
    ``None`` if no event carries ``cwd`` or the path isn't a directory.
    """
    try:
        fh = path.open("r", encoding="utf-8")
    except OSError:
        return None
    with fh:
        for _ in range(scan_lines):
            raw = fh.readline()
            if not raw:
                break
            raw = raw.strip()
            if not raw:
                continue
            try:
                event = json.loads(raw)
            except json.JSONDecodeError:
                continue
            cwd_str = event.get("cwd")
            if isinstance(cwd_str, str) and cwd_str:
                cwd = Path(cwd_str)
                if cwd.is_dir():
                    return cwd
    return None


def _is_logmind_repo(cwd: Path) -> bool:
    """A repo counts as logmind-initialised when it has ``.logmind/config.yml``.
    Sessions outside logmind repos aren't relevant to the Q7-logmind
    question."""
    return (cwd / ".logmind" / "config.yml").is_file()


def _walk_reads(path: Path) -> Iterator[tuple[str, int]]:
    """Yield ``(file_path, bytes_read)`` for every ``Read`` tool call in
    the session.

    Matches ``tool_use`` events (name == ``Read``, captures ``id`` +
    ``input.file_path``) to the subsequent ``tool_result`` event with
    the same ``tool_use_id``. Bytes = UTF-8 length of the result
    ``content`` field.

    Malformed lines are skipped (``json.JSONDecodeError``); reads
    without a matching result are skipped. ``Edit`` and ``Write``
    don't count — only ``Read`` represents bytes flowing INTO the
    agent's context.
    """
    pending: dict[str, str] = {}
    try:
        fh = path.open("r", encoding="utf-8")
    except OSError:
        return
    with fh:
        for raw in fh:
            raw = raw.strip()
            if not raw:
                continue
            try:
                event = json.loads(raw)
            except json.JSONDecodeError:
                continue
            message = event.get("message")
            if not isinstance(message, dict):
                continue
            content = message.get("content")
            if not isinstance(content, list):
                continue
            for block in content:
                if not isinstance(block, dict):
                    continue
                btype = block.get("type")
                if btype == "tool_use" and block.get("name") == "Read":
                    tu_id = block.get("id")
                    inputs = block.get("input") or {}
                    fp = inputs.get("file_path")
                    if isinstance(tu_id, str) and isinstance(fp, str):
                        pending[tu_id] = fp
                elif btype == "tool_result":
                    tu_id = block.get("tool_use_id")
                    if not isinstance(tu_id, str) or tu_id not in pending:
                        continue
                    fp = pending.pop(tu_id)
                    body = block.get("content")
                    # ``content`` may be a string OR a list of text blocks
                    # depending on platform version. Coerce both into
                    # UTF-8 byte length.
                    if isinstance(body, str):
                        nbytes = len(body.encode("utf-8"))
                    elif isinstance(body, list):
                        nbytes = sum(
                            len(b.get("text", "").encode("utf-8"))
                            for b in body
                            if isinstance(b, dict) and isinstance(b.get("text"), str)
                        )
                    else:
                        nbytes = 0
                    yield fp, nbytes


def _bucket(file_path: str, cwd: Path) -> str | None:
    """Return which logmind-doc bucket ``file_path`` belongs to, relative
    to ``cwd``. Returns ``None`` for anything outside the four buckets.

    Matches both absolute and cwd-relative paths so behaviour is
    consistent across platforms / session-log variants.
    """
    try:
        candidate = Path(file_path)
        if candidate.is_absolute():
            rel = candidate.relative_to(cwd).as_posix()
        else:
            rel = candidate.as_posix()
    except ValueError:
        return None
    return rel if rel in _BUCKETS else None


# Memoise per-repo git baselines — sessions in the same repo share one.
_GIT_BASELINE_CACHE: dict[Path, int] = {}


def _git_equivalent_bytes(cwd: Path) -> int:
    """Bytes the agent would have read from ``git log --oneline -100``
    if logmind didn't exist. This is the no-logmind world baseline.
    """
    if cwd in _GIT_BASELINE_CACHE:
        return _GIT_BASELINE_CACHE[cwd]
    try:
        result = subprocess.run(
            ["git", "log", "--oneline", "-100"],
            cwd=cwd,
            capture_output=True,
            check=False,
            text=False,
        )
    except (OSError, subprocess.SubprocessError):
        _GIT_BASELINE_CACHE[cwd] = 0
        return 0
    if result.returncode != 0:
        _GIT_BASELINE_CACHE[cwd] = 0
        return 0
    nbytes = len(result.stdout)
    _GIT_BASELINE_CACHE[cwd] = nbytes
    return nbytes


def _agents_md_block_bytes(cwd: Path) -> tuple[int, int]:
    """Return ``(block_bytes, total_bytes)`` for ``cwd/AGENTS.md``.

    The logmind block is detected by the canonical ``<!-- logmind-start -->``
    / ``<!-- logmind-end -->`` markers; falls back to anchor-text scan
    if the markers are absent (some older installs).

    Returns ``(0, 0)`` if AGENTS.md doesn't exist or can't be read.
    Block-share readers should guard division-by-zero on ``total_bytes``.
    """
    agents = cwd / "AGENTS.md"
    if not agents.is_file():
        return 0, 0
    try:
        text = agents.read_text(encoding="utf-8")
    except OSError:
        return 0, 0
    total = len(text.encode("utf-8"))
    start = text.find("<!-- logmind-start -->")
    end = text.find("<!-- logmind-end -->")
    if start != -1 and end != -1 and end > start:
        block = text[start : end + len("<!-- logmind-end -->")]
        return len(block.encode("utf-8")), total
    # Fallback for older installs that lack the markers: anchor-text
    # region from the first matching anchor to the next H2 heading
    # (or EOF). The result is a best-effort estimate, not a precise
    # block size — older installs are a rare case and a heuristic is
    # better than ``0`` (which would mute the metric entirely).
    for anchor in _LOGMIND_BLOCK_ANCHORS:
        if anchor in text:
            idx = text.find(anchor)
            tail = text[idx:]
            # Stop at the next "## " heading or end of file.
            stop = tail.find("\n## ")
            if stop == -1:
                stop = len(tail)
            return len(tail[:stop].encode("utf-8")), total
    return 0, total


def run_per_session(home: Path | None = None) -> PerSessionResult:
    """Top-level entry point: walk session logs, aggregate, return a
    ``PerSessionResult``. ``home`` is injectable for tests."""
    _GIT_BASELINE_CACHE.clear()  # Don't bleed cache across runs in tests.

    paths = _session_paths(home=home)
    if not paths:
        return PerSessionResult(
            label="no sessions found",
            net_pct=None,
            stub=True,
        )

    detail: list[dict] = []
    sessions_sampled = 0
    sessions_with_reads = 0
    bucket_totals: dict[str, int] = {b: 0 for b in _BUCKETS}
    agents_block_total = 0
    agents_total = 0
    logmind_total_bytes = 0
    git_total_bytes = 0

    for path in paths:
        cwd = _session_cwd(path)
        if cwd is None or not _is_logmind_repo(cwd):
            continue
        sessions_sampled += 1
        per_session_buckets: dict[str, int] = {b: 0 for b in _BUCKETS}
        for fp, nbytes in _walk_reads(path):
            bucket = _bucket(fp, cwd)
            if bucket is None:
                continue
            per_session_buckets[bucket] += nbytes
        session_logmind_bytes = sum(per_session_buckets.values())
        if session_logmind_bytes == 0:
            detail.append(
                {
                    "path": str(path),
                    "cwd": str(cwd),
                    "logmind_bytes": 0,
                    "git_bytes": 0,
                    "net_pct": None,
                    "buckets": per_session_buckets,
                }
            )
            continue
        git_bytes = _git_equivalent_bytes(cwd)
        if git_bytes == 0:
            # Empty git baseline — divide-by-zero. Don't count this
            # session in the aggregate (it's noise, not signal). Also
            # don't increment ``sessions_with_reads``: a session with
            # decision reads BUT no usable baseline has nothing for
            # ``avg_net`` to average over, and would otherwise produce
            # ``stub=False, net_pct=0.0`` (a fake "break-even" verdict)
            # via the empty-list path at the aggregate. Caught by PR #78
            # review thread; pinned by a fixture test below.
            detail.append(
                {
                    "path": str(path),
                    "cwd": str(cwd),
                    "logmind_bytes": session_logmind_bytes,
                    "git_bytes": 0,
                    "net_pct": None,
                    "buckets": per_session_buckets,
                }
            )
            continue
        sessions_with_reads += 1
        net_pct = ((session_logmind_bytes - git_bytes) / git_bytes) * 100.0
        logmind_total_bytes += session_logmind_bytes
        git_total_bytes += git_bytes
        for b, n in per_session_buckets.items():
            bucket_totals[b] += n
        block_bytes, agents_md_total = _agents_md_block_bytes(cwd)
        agents_block_total += block_bytes
        agents_total += agents_md_total
        detail.append(
            {
                "path": str(path),
                "cwd": str(cwd),
                "logmind_bytes": session_logmind_bytes,
                "git_bytes": git_bytes,
                "net_pct": net_pct,
                "buckets": per_session_buckets,
            }
        )

    if sessions_sampled == 0:
        return PerSessionResult(
            label="no logmind repos in sampled sessions",
            net_pct=None,
            stub=True,
            sessions_sampled=0,
            detail=detail,
        )
    if sessions_with_reads == 0:
        return PerSessionResult(
            label="no decision-doc reads in sampled sessions",
            net_pct=None,
            stub=True,
            sessions_sampled=sessions_sampled,
            sessions_with_decision_reads=0,
            detail=detail,
        )

    # Per-session ``net_pct`` averaged across sessions with reads
    # (matches per_call.py's aggregation pattern from PR #74).
    session_nets = [d["net_pct"] for d in detail if d.get("net_pct") is not None]
    avg_net = sum(session_nets) / len(session_nets) if session_nets else 0.0

    per_file_share: dict[str, float] = {}
    if logmind_total_bytes > 0:
        per_file_share = {
            b: bucket_totals[b] / logmind_total_bytes for b in _BUCKETS
        }
    agents_md_block_share = (
        agents_block_total / agents_total if agents_total > 0 else 0.0
    )

    # Compact human-readable label surfaces the load-bearing 0.B.5 +
    # 0.B.6 metrics directly.
    label = (
        f"bytes amortized ({sessions_with_reads}/{sessions_sampled} sessions, "
        f"AGENTS.md={int(per_file_share.get('AGENTS.md', 0.0) * 100)}%, "
        f"decisions={int(per_file_share.get('docs/decisions.md', 0.0) * 100)}%)"
    )

    return PerSessionResult(
        label=label,
        net_pct=avg_net,
        stub=False,
        sessions_sampled=sessions_sampled,
        sessions_with_decision_reads=sessions_with_reads,
        per_file_share=per_file_share,
        agents_md_block_share=agents_md_block_share,
        detail=detail,
    )


# Back-compat shim — kept so callers that imported the stub directly
# don't break. The registry in ``bench/__main__.py`` has been switched
# to ``run_per_session``.
def run_per_session_stub() -> PerSessionResult:
    return run_per_session()
