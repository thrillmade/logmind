"""Per-call angle: prove `logmind <cmd>` emits ≤ bytes of its manual git
equivalent.

For each logmind command, set up a fresh tempdir + git repo, run the
logmind command, capture stdout+stderr bytes. Then run the equivalent
manual git command sequence in another tempdir, capture its bytes. Net
percentage = (logmind_bytes - git_bytes) / git_bytes.

Negative net = saver (logmind cheaper). Positive net = spender (P0 fix).

This is the foundational angle. If per-call is negative, every use of
logmind in any session is a token win. The other three angles compound
on top.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Callable


@dataclass
class CommandPair:
    """One logmind command + its manual git equivalent for comparison."""

    name: str
    logmind_setup: Callable[[Path], None]
    logmind_cmd: list[str]
    git_setup: Callable[[Path], None]
    git_cmds: list[list[str]]


@dataclass
class PerCallResult:
    label: str
    net_pct: float
    pairs: list[dict]


def _quiet_env() -> dict[str, str]:
    """LOGMIND_QUIET=1 is the agent-invocation default — matches how
    real agent sessions invoke logmind."""
    env = os.environ.copy()
    env["LOGMIND_QUIET"] = "1"
    env["GIT_COMMITTER_NAME"] = "bench"
    env["GIT_COMMITTER_EMAIL"] = "bench@example.com"
    env["GIT_AUTHOR_NAME"] = "bench"
    env["GIT_AUTHOR_EMAIL"] = "bench@example.com"
    return env


def _setup_logmind_repo(path: Path) -> None:
    """Fresh git repo + `logmind init` so the docs/ scaffold + .logmind/
    config exist."""
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=path, check=True)
    subprocess.run(["git", "config", "user.email", "bench@example.com"], cwd=path, check=True)
    subprocess.run(["git", "config", "user.name", "bench"], cwd=path, check=True)
    # Make initial commit so HEAD exists.
    (path / "README.md").write_text("# bench\n", encoding="utf-8")
    subprocess.run(["git", "add", "README.md"], cwd=path, check=True)
    subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=path, check=True)
    subprocess.run(
        ["logmind", "init"],
        cwd=path, env=_quiet_env(), check=True, capture_output=True,
    )


def _setup_plain_git(path: Path) -> None:
    """Same fresh git repo, but no logmind init. For the manual-git
    equivalent harness."""
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=path, check=True)
    subprocess.run(["git", "config", "user.email", "bench@example.com"], cwd=path, check=True)
    subprocess.run(["git", "config", "user.name", "bench"], cwd=path, check=True)
    (path / "README.md").write_text("# bench\n", encoding="utf-8")
    subprocess.run(["git", "add", "README.md"], cwd=path, check=True)
    subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=path, check=True)


def _make_change(path: Path) -> None:
    """Realistic single-file change to simulate "agent did some work" before
    invoking logmind log / git commit."""
    (path / "feature.py").write_text("def feature(): return 42\n", encoding="utf-8")


def _run_pair(pair: CommandPair) -> dict:
    """Run the logmind command and its git equivalent in fresh tempdirs;
    return byte counts + per-pair net %."""
    with tempfile.TemporaryDirectory(prefix="bench-lm-") as lm_dir_str:
        lm_dir = Path(lm_dir_str)
        pair.logmind_setup(lm_dir)
        _make_change(lm_dir)
        result = subprocess.run(
            pair.logmind_cmd,
            cwd=lm_dir, env=_quiet_env(),
            capture_output=True, check=False,
        )
        lm_bytes = len(result.stdout) + len(result.stderr)
        lm_exit = result.returncode

    with tempfile.TemporaryDirectory(prefix="bench-git-") as g_dir_str:
        g_dir = Path(g_dir_str)
        pair.git_setup(g_dir)
        _make_change(g_dir)
        g_bytes = 0
        g_exit = 0
        for cmd in pair.git_cmds:
            r = subprocess.run(
                cmd, cwd=g_dir, env=_quiet_env(),
                capture_output=True, check=False,
            )
            g_bytes += len(r.stdout) + len(r.stderr)
            if r.returncode != 0:
                g_exit = r.returncode

    if g_bytes == 0:
        net = 0.0  # Avoid div-by-zero on empty git output (shouldn't happen).
    else:
        net = ((lm_bytes - g_bytes) / g_bytes) * 100.0
    return {
        "name": pair.name,
        "logmind_bytes": lm_bytes,
        "git_bytes": g_bytes,
        "net_pct": net,
        "logmind_exit": lm_exit,
        "git_exit": g_exit,
    }


def _pairs() -> list[CommandPair]:
    """The command pairs we benchmark.

    Honest framing — measure bytes THE AGENT ACTUALLY SEES per call.
    NOT every logmind command has a direct git equivalent; we only
    benchmark commands an agent actually invokes per session.

    `log` is the most common — `logmind log` replaces the
    add+commit+push trio. Manual git equivalent runs WITHOUT `-q` so
    git's normal commit-summary output (~200 bytes for a 1-file
    change) counts toward the agent's input.

    `show` is the second most common — agents read prior decisions
    before non-trivial work. logmind show --brief gives one-line
    summaries; manual git equivalent is `git log --pretty=format` which
    shows full commit messages.

    Artifact-regeneration commands (timeline, tree, file-structure)
    are NOT benchmarked here — they're invoked rarely by agents
    (mostly by CI) and their output is the file content, which an
    agent reads ONCE then amortizes across many later sessions. They
    belong to the per-session amortization angle (per_session.py),
    not per-call.
    """
    return [
        CommandPair(
            name="log",
            logmind_setup=_setup_logmind_repo,
            logmind_cmd=["logmind", "log", "smoke test decision", "--no-push", "--stage", "scoped"],
            git_setup=_setup_plain_git,
            # NOT -q: a real dev/agent invoking git directly sees this
            # output. The commit summary is part of the cost of the
            # manual approach.
            git_cmds=[
                ["git", "add", "-A"],
                ["git", "commit", "-m", "smoke test decision"],
                # `git push` to nonexistent remote would fail noisily —
                # in a real session it'd succeed but emit ~5 lines.
                # Skip to keep the comparison deterministic; logmind
                # is conservatively under-credited here vs reality.
            ],
        ),
        CommandPair(
            name="show",
            logmind_setup=_seed_decisions_then_logmind,
            logmind_cmd=["logmind", "show", "--brief"],
            git_setup=_seed_decisions_then_git,
            # FUNCTIONAL parity: logmind --brief emits date + time +
            # title + source. Git equivalent must include the same
            # information (date + title + commit SHA as the "source"
            # proxy), otherwise we're comparing a richer artifact to a
            # sparser one — biased in logmind's favor on naive measures.
            # `--date=iso-strict-local` matches logmind's YYYY-MM-DD
            # HH:MM format; `--all` matches logmind's cross-branch scan.
            git_cmds=[
                ["git", "log", "--all", "--date=iso-strict-local",
                 "--pretty=format:%ad %s %h"],
            ],
        ),
    ]


def _seed_decisions_then_logmind(path: Path) -> None:
    """Setup that ensures logmind show has something to show. Initializes
    a repo, runs logmind init, then makes 3 decisions via `logmind log`."""
    _setup_logmind_repo(path)
    for i in range(3):
        # Each `logmind log` creates a commit recording a decision.
        # We use --stage scoped so the synthetic write doesn't pull in
        # other files, and --no-push because no remote exists.
        (path / f"file_{i}.py").write_text(f"# decision {i}\n", encoding="utf-8")
        subprocess.run(
            ["logmind", "log", f"decision {i}", "--no-push"],
            cwd=path, env=_quiet_env(), check=True, capture_output=True,
        )


def _seed_decisions_then_git(path: Path) -> None:
    """Parallel seed for the plain-git harness — same 3 commits, same
    decision messages, no logmind."""
    _setup_plain_git(path)
    for i in range(3):
        (path / f"file_{i}.py").write_text(f"# decision {i}\n", encoding="utf-8")
        subprocess.run(["git", "add", "-A"], cwd=path, env=_quiet_env(), check=True)
        subprocess.run(
            ["git", "commit", "-m", f"decision {i}"],
            cwd=path, env=_quiet_env(), check=True, capture_output=True,
        )


def run_per_call() -> PerCallResult:
    """Top-level entry point: run all pairs, aggregate."""
    pairs_results = [_run_pair(p) for p in _pairs()]
    if not pairs_results:
        return PerCallResult(label="bytes vs git equivalent", net_pct=0.0, pairs=[])
    # Aggregate: average net % across pairs (each pair weighted equally).
    nets = [r["net_pct"] for r in pairs_results]
    avg_net = sum(nets) / len(nets)
    return PerCallResult(
        label="bytes vs git equivalent",
        net_pct=avg_net,
        pairs=pairs_results,
    )
