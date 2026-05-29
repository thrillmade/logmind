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

# The bench harness spawns real `logmind` subprocesses + tempdirs +
# git invocations + initializes logmind in those tempdirs. The bench
# itself is designed to run on Linux CI (.github/workflows/bench.yml
# uses ubuntu-latest). Windows behaves differently around tempdir
# paths, git default-branch handling, and `logmind init`'s symlink/
# workflow-template install — skip the integration tests there. The
# pure-logic tests below still run on Windows.
_skip_on_windows = pytest.mark.skipif(
    sys.platform == "win32",
    reason="bench harness targets Linux CI (bench.yml ubuntu-latest); Windows tempdir/git behavior differs",
)


def test_bench_module_importable():
    """`python -m bench` is the entry point. Module must import cleanly."""
    from bench import per_call, worst_case, per_session, org_cumulative  # noqa: F401


@_skip_on_windows
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


@_skip_on_windows
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


def test_per_session_stub_invariant_when_no_sessions(tmp_path):
    """The stub-invariant: when no sessions exist on disk, the angle
    returns ``stub=True`` / ``net_pct=None`` so the exit gate skips it
    (matches the org-cumulative stub contract). Was
    ``test_per_session_returns_stub_with_null_net_pct`` pre-impl —
    renamed + routed through the real entry point with an empty fake
    ``~/.claude/projects/``."""
    from bench.per_session import run_per_session
    fake_home = tmp_path / "home"
    fake_home.mkdir()
    result = run_per_session(home=fake_home)
    assert result.stub is True
    assert result.net_pct is None
    assert "no sessions found" in result.label


def test_per_session_no_logmind_repos_returns_stub(tmp_path):
    """A session whose cwd isn't a logmind repo (no
    ``.logmind/config.yml``) is dropped from the sample. If ALL sampled
    sessions are non-logmind, the angle reports as stub — the metric
    has nothing to assert."""
    from bench.per_session import run_per_session
    fake_home = tmp_path / "home"
    projects_dir = fake_home / ".claude" / "projects" / "encoded-cwd"
    projects_dir.mkdir(parents=True)
    # A plain (non-logmind) repo as the session cwd.
    plain_repo = tmp_path / "plain-repo"
    plain_repo.mkdir()
    session = projects_dir / "session-1.jsonl"
    session.write_text(
        json.dumps({"cwd": str(plain_repo), "type": "user"}) + "\n",
        encoding="utf-8",
    )
    result = run_per_session(home=fake_home)
    assert result.stub is True
    assert result.net_pct is None
    assert result.sessions_sampled == 0


def test_per_session_aggregates_from_fixture(tmp_path):
    """End-to-end aggregation: fake home + fake logmind repo + inline
    JSONL with one Read of decisions.md. Validates the tool_use →
    tool_result join, bucket assignment, and per-file-share output."""
    from bench.per_session import run_per_session
    # Set up the fake logmind repo so _is_logmind_repo passes.
    repo = tmp_path / "logmind-repo"
    repo.mkdir()
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
    subprocess.run(["git", "config", "user.email", "t@t"], cwd=repo, check=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=repo, check=True)
    (repo / "README.md").write_text("# t\n", encoding="utf-8")
    subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
    subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=repo, check=True)
    (repo / ".logmind").mkdir()
    (repo / ".logmind" / "config.yml").write_text("project: t\n", encoding="utf-8")
    docs = repo / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text("# Decision Log\n", encoding="utf-8")

    # Inline JSONL fixture: one Read of decisions.md returning 2000 bytes.
    fake_home = tmp_path / "home"
    projects_dir = fake_home / ".claude" / "projects" / "enc"
    projects_dir.mkdir(parents=True)
    session = projects_dir / "session.jsonl"
    payload = "x" * 2000
    lines = [
        json.dumps({"cwd": str(repo), "type": "user"}),
        json.dumps(
            {
                "message": {
                    "content": [
                        {
                            "type": "tool_use",
                            "name": "Read",
                            "id": "tu-1",
                            "input": {"file_path": str(repo / "docs" / "decisions.md")},
                        }
                    ]
                }
            }
        ),
        json.dumps(
            {
                "message": {
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": "tu-1",
                            "content": payload,
                        }
                    ]
                }
            }
        ),
    ]
    session.write_text("\n".join(lines) + "\n", encoding="utf-8")

    result = run_per_session(home=fake_home)
    # 1 session sampled, 1 with decision reads, net_pct computed.
    assert result.stub is False
    assert result.sessions_sampled == 1
    assert result.sessions_with_decision_reads == 1
    assert result.net_pct is not None
    # decisions.md share should be 1.0 (only file read).
    assert result.per_file_share["docs/decisions.md"] == pytest.approx(1.0)
    assert result.per_file_share["AGENTS.md"] == pytest.approx(0.0)


def test_per_session_zero_decision_reads_returns_stub(tmp_path):
    """Session reads ONLY non-bucket files (e.g. random source). The
    aggregate has no signal — angle reports stub with
    sessions_sampled > 0 to distinguish from "no sessions found"."""
    from bench.per_session import run_per_session
    repo = tmp_path / "logmind-repo"
    repo.mkdir()
    (repo / ".logmind").mkdir()
    (repo / ".logmind" / "config.yml").write_text("project: t\n", encoding="utf-8")
    fake_home = tmp_path / "home"
    projects_dir = fake_home / ".claude" / "projects" / "enc"
    projects_dir.mkdir(parents=True)
    session = projects_dir / "session.jsonl"
    lines = [
        json.dumps({"cwd": str(repo), "type": "user"}),
        json.dumps(
            {
                "message": {
                    "content": [
                        {
                            "type": "tool_use",
                            "name": "Read",
                            "id": "tu-1",
                            "input": {"file_path": str(repo / "src" / "foo.py")},
                        }
                    ]
                }
            }
        ),
        json.dumps(
            {
                "message": {
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": "tu-1",
                            "content": "x" * 500,
                        }
                    ]
                }
            }
        ),
    ]
    session.write_text("\n".join(lines) + "\n", encoding="utf-8")
    result = run_per_session(home=fake_home)
    assert result.stub is True
    assert result.net_pct is None
    assert result.sessions_sampled == 1
    assert result.sessions_with_decision_reads == 0


def test_per_session_empty_git_baseline_does_not_count_session(tmp_path):
    """REGRESSION GUARD (PR #78 review): if a session has decision-doc
    reads but the repo's ``git log --oneline -100`` returns 0 bytes
    (fresh ``git init`` with no commits, broken HEAD, etc.), the
    session must NOT be counted in ``sessions_with_decision_reads``.
    Otherwise the aggregate falls through to ``stub=False, net_pct=0.0``
    — a fake "break-even" verdict.

    The test sets up a fresh git init repo WITHOUT a commit, so
    ``git log --oneline -100`` exits 0 with empty stdout. The session
    reads AGENTS.md (a logmind-doc bucket); the aggregate should report
    stub=True / net_pct=None, not stub=False / net_pct=0.0.
    """
    from bench.per_session import run_per_session
    repo = tmp_path / "logmind-repo"
    repo.mkdir()
    # Fresh git init — no commit, so ``git log --oneline -100`` returns
    # empty stdout (exit 0).
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
    (repo / ".logmind").mkdir()
    (repo / ".logmind" / "config.yml").write_text("project: t\n", encoding="utf-8")
    fake_home = tmp_path / "home"
    projects_dir = fake_home / ".claude" / "projects" / "enc"
    projects_dir.mkdir(parents=True)
    session = projects_dir / "session.jsonl"
    lines = [
        json.dumps({"cwd": str(repo), "type": "user"}),
        json.dumps(
            {
                "message": {
                    "content": [
                        {
                            "type": "tool_use",
                            "name": "Read",
                            "id": "tu-1",
                            "input": {"file_path": str(repo / "AGENTS.md")},
                        }
                    ]
                }
            }
        ),
        json.dumps(
            {
                "message": {
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": "tu-1",
                            "content": "x" * 1000,
                        }
                    ]
                }
            }
        ),
    ]
    session.write_text("\n".join(lines) + "\n", encoding="utf-8")
    result = run_per_session(home=fake_home)
    # NOT stub=False with net_pct=0.0. The empty git baseline means
    # this session has no usable measurement → angle reports stub.
    assert result.stub is True
    assert result.net_pct is None
    assert result.sessions_sampled == 1
    assert result.sessions_with_decision_reads == 0
    # Verify the detail entry has net_pct=None so callers can introspect.
    assert any(
        d.get("net_pct") is None and d.get("git_bytes") == 0
        for d in result.detail
    )


def test_per_session_malformed_line_skipped(tmp_path):
    """Garbage JSONL lines are skipped, not crashing. Surrounding valid
    events still aggregate normally."""
    from bench.per_session import run_per_session
    repo = tmp_path / "logmind-repo"
    repo.mkdir()
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
    subprocess.run(["git", "config", "user.email", "t@t"], cwd=repo, check=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=repo, check=True)
    (repo / "README.md").write_text("# t\n", encoding="utf-8")
    subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
    subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=repo, check=True)
    (repo / ".logmind").mkdir()
    (repo / ".logmind" / "config.yml").write_text("project: t\n", encoding="utf-8")
    fake_home = tmp_path / "home"
    projects_dir = fake_home / ".claude" / "projects" / "enc"
    projects_dir.mkdir(parents=True)
    session = projects_dir / "session.jsonl"
    lines = [
        json.dumps({"cwd": str(repo), "type": "user"}),
        "this is not json at all — should be skipped",
        json.dumps(
            {
                "message": {
                    "content": [
                        {
                            "type": "tool_use",
                            "name": "Read",
                            "id": "tu-1",
                            "input": {"file_path": str(repo / "AGENTS.md")},
                        }
                    ]
                }
            }
        ),
        json.dumps(
            {
                "message": {
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": "tu-1",
                            "content": "x" * 1000,
                        }
                    ]
                }
            }
        ),
    ]
    session.write_text("\n".join(lines) + "\n", encoding="utf-8")
    result = run_per_session(home=fake_home)
    # The malformed line was skipped; the Read+result joined and was
    # bucketed to AGENTS.md.
    assert result.stub is False
    assert result.sessions_with_decision_reads == 1
    assert result.per_file_share["AGENTS.md"] == pytest.approx(1.0)


def test_org_cumulative_no_sessions_returns_stub(tmp_path):
    """Stub-invariant: when no sessions exist on disk, the angle
    reports as not-yet-implemented-shaped so the exit gate skips it."""
    from bench.org_cumulative import run_org_cumulative
    fake_home = tmp_path / "home"
    fake_home.mkdir()
    result = run_org_cumulative(home=fake_home)
    assert result.stub is True
    assert result.net_pct is None
    assert "no sessions found" in result.label


def test_org_cumulative_aggregates_across_sessions_and_repos(tmp_path):
    """End-to-end: two sessions across two distinct logmind repos.
    Verifies global sum aggregation (not per-session averaging) AND
    that ``per_repo_share`` keys both repos. Distinguishes from
    per_session.py's average-of-percentages rollup."""
    from bench.org_cumulative import run_org_cumulative

    def _setup_repo(name: str, doc_content: str) -> Path:
        repo = tmp_path / name
        repo.mkdir()
        subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
        subprocess.run(["git", "config", "user.email", "t@t"], cwd=repo, check=True)
        subprocess.run(["git", "config", "user.name", "t"], cwd=repo, check=True)
        (repo / "README.md").write_text("# t\n", encoding="utf-8")
        subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
        subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=repo, check=True)
        (repo / ".logmind").mkdir()
        (repo / ".logmind" / "config.yml").write_text("project: t\n", encoding="utf-8")
        (repo / "docs").mkdir()
        (repo / "docs" / "decisions.md").write_text(doc_content, encoding="utf-8")
        return repo

    repo_a = _setup_repo("repo-a", "# A\n")
    repo_b = _setup_repo("repo-b", "# B\n")

    fake_home = tmp_path / "home"

    def _emit_session(session_name: str, repo: Path, payload_size: int) -> None:
        projects_dir = fake_home / ".claude" / "projects" / session_name
        projects_dir.mkdir(parents=True, exist_ok=True)
        session = projects_dir / "session.jsonl"
        payload = "x" * payload_size
        lines = [
            json.dumps({"cwd": str(repo), "type": "user"}),
            json.dumps({
                "message": {
                    "content": [{
                        "type": "tool_use",
                        "name": "Read",
                        "id": f"tu-{session_name}",
                        "input": {"file_path": str(repo / "docs" / "decisions.md")},
                    }]
                }
            }),
            json.dumps({
                "message": {
                    "content": [{
                        "type": "tool_result",
                        "tool_use_id": f"tu-{session_name}",
                        "content": payload,
                    }]
                }
            }),
        ]
        session.write_text("\n".join(lines) + "\n", encoding="utf-8")

    _emit_session("enc-a", repo_a, 3000)
    _emit_session("enc-b", repo_b, 1000)

    result = run_org_cumulative(home=fake_home)
    assert result.stub is False
    assert result.sessions_sampled == 2
    assert result.repos_sampled == 2
    # Total logmind bytes = 3000 + 1000 = 4000.
    assert result.total_logmind_bytes == 4000
    assert result.total_git_bytes > 0
    # Global net_pct = (4000 - total_git) / total_git * 100. Sign +/-
    # depends on baseline size, but the value must be defined.
    assert result.net_pct is not None
    # Per-repo share: repo-a contributes 75 %, repo-b 25 %.
    shares = result.per_repo_share
    assert sum(shares.values()) == pytest.approx(1.0)
    assert shares[str(repo_a)] == pytest.approx(0.75)
    assert shares[str(repo_b)] == pytest.approx(0.25)


def test_org_cumulative_per_repo_share_isolates_outlier(tmp_path):
    """Three sessions in the same repo vs one in another → first repo
    should carry the dominant share. This is the metric Step 4 uses
    to spot per-consumer cache-key regressions (>2× median share)."""
    from bench.org_cumulative import run_org_cumulative

    repos: list[Path] = []
    for name in ("dominant", "small"):
        r = tmp_path / name
        r.mkdir()
        subprocess.run(["git", "init", "-q", "-b", "main"], cwd=r, check=True)
        subprocess.run(["git", "config", "user.email", "t@t"], cwd=r, check=True)
        subprocess.run(["git", "config", "user.name", "t"], cwd=r, check=True)
        (r / "README.md").write_text("# t\n", encoding="utf-8")
        subprocess.run(["git", "add", "-A"], cwd=r, check=True)
        subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=r, check=True)
        (r / ".logmind").mkdir()
        (r / ".logmind" / "config.yml").write_text("project: t\n", encoding="utf-8")
        (r / "docs").mkdir()
        (r / "docs" / "decisions.md").write_text(f"# {name}\n", encoding="utf-8")
        repos.append(r)

    fake_home = tmp_path / "home"

    def _emit(slug: str, repo: Path, payload: int) -> None:
        d = fake_home / ".claude" / "projects" / slug
        d.mkdir(parents=True, exist_ok=True)
        s = d / "session.jsonl"
        lines = [
            json.dumps({"cwd": str(repo), "type": "user"}),
            json.dumps({"message": {"content": [{
                "type": "tool_use", "name": "Read", "id": f"tu-{slug}",
                "input": {"file_path": str(repo / "docs" / "decisions.md")}
            }]}}),
            json.dumps({"message": {"content": [{
                "type": "tool_result", "tool_use_id": f"tu-{slug}", "content": "x" * payload,
            }]}}),
        ]
        s.write_text("\n".join(lines) + "\n", encoding="utf-8")

    _emit("d1", repos[0], 1000)
    _emit("d2", repos[0], 1000)
    _emit("d3", repos[0], 1000)
    _emit("s1", repos[1], 500)

    result = run_org_cumulative(home=fake_home)
    assert result.stub is False
    assert result.repos_sampled == 2
    # Dominant repo: 3000 / 3500 ≈ 0.857; small repo: 500 / 3500 ≈ 0.143.
    assert result.per_repo_share[str(repos[0])] == pytest.approx(3000 / 3500)
    assert result.per_repo_share[str(repos[1])] == pytest.approx(500 / 3500)


def test_org_cumulative_stub_shim_calls_real_impl():
    """``run_org_cumulative_stub`` is a back-compat alias that just
    delegates to the real implementation. It MUST NOT return the
    pre-impl stub shape (would silently keep callers on the old
    placeholder). Caught at registry-swap time."""
    from bench.org_cumulative import run_org_cumulative_stub
    result = run_org_cumulative_stub()
    # On a real machine this MAY be a stub (no sessions in test env)
    # but the SHAPE must include the new fields, not the old 3-field
    # dataclass. ``repos_sampled`` is the canary.
    assert hasattr(result, "repos_sampled")
    assert hasattr(result, "per_repo_share")


@_skip_on_windows
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


@_skip_on_windows
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


@_skip_on_windows
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


@_skip_on_windows
def test_failed_logmind_command_does_not_become_a_fake_saver(monkeypatch):
    """PR #74 regression: if a logmind invocation exits non-zero, the
    pair MUST NOT be folded into the aggregate as a saver. A broken
    command might emit just an error message (e.g. 80 bytes) against a
    much larger git equivalent — looking like a huge saver and hiding
    the regression."""
    from bench.per_call import _run_pair, CommandPair, _setup_logmind_repo, _setup_plain_git

    # Construct a pair whose "logmind command" is guaranteed to fail
    # (bogus subcommand). The git side stays normal.
    broken_pair = CommandPair(
        name="bogus",
        logmind_setup=_setup_logmind_repo,
        logmind_cmd=["logmind", "this-subcommand-does-not-exist"],
        git_setup=_setup_plain_git,
        git_cmds=[["git", "log", "--oneline"]],
    )
    result = _run_pair(broken_pair)
    assert result["failed"] is True, "failed logmind exit must be flagged"
    assert result["net_pct"] is None, (
        "failed pair must have net_pct=None — NEVER count a broken "
        "command as a saver in the aggregate"
    )
    assert result["logmind_exit"] != 0


def test_per_call_aggregate_skips_failed_pairs(monkeypatch):
    """If one pair fails and the other succeeds, run_per_call returns
    the average of the successful pair(s) only — failed pairs are
    surfaced in `pairs` but excluded from the average."""
    from bench import per_call

    real_run = per_call._run_pair

    call_count = {"n": 0}
    def fake_run_pair(pair):
        call_count["n"] += 1
        if call_count["n"] == 1:
            # First pair: pretend it failed.
            return {"name": "failed_pair", "net_pct": None, "failed": True,
                    "logmind_bytes": 80, "git_bytes": 500,
                    "logmind_exit": 1, "git_exit": 0}
        # Second pair: real saver.
        return {"name": "good_pair", "net_pct": -30.0, "failed": False,
                "logmind_bytes": 70, "git_bytes": 100,
                "logmind_exit": 0, "git_exit": 0}

    monkeypatch.setattr(per_call, "_run_pair", fake_run_pair)
    monkeypatch.setattr(per_call, "_pairs", lambda: ["dummy1", "dummy2"])

    result = per_call.run_per_call()
    # Average of successful pairs only = -30%, NOT (-30 + None)/2 nor
    # (-30 + 0)/2 nor an averaging that includes the broken pair.
    assert result.net_pct == -30.0, (
        "aggregate must average only successful pairs — never silently "
        "include a broken pair as 0% or as a saver"
    )


@_skip_on_windows
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
