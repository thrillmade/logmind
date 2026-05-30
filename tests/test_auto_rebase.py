"""Tests for v0.6.1 deterministic auto-rebase.

Hard safety guards: opt-in only; timeline.md-only gap; never --force
(always --force-with-lease); aborts cleanly on any unexpected conflict.

These tests cover the GUARDS — the most important property is that
auto-rebase REFUSES TO ACT in any case beyond the very narrow target
scenario. False-positives here = silent destruction of user work.
"""

from __future__ import annotations

import subprocess
from pathlib import Path
from unittest.mock import patch

import pytest

from logmind.core.auto_rebase import (
    AutoRebaseResult,
    maybe_auto_rebase,
)


def _git_init_with_remote(local: Path, remote: Path) -> None:
    subprocess.run(["git", "init", "--bare", "-b", "main"], cwd=remote, check=True, capture_output=True)
    subprocess.run(["git", "init", "-b", "main"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.email", "t@t.com"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=local, check=True, capture_output=True)
    (local / "README.md").write_text("seed\n", encoding="utf-8")
    subprocess.run(["git", "add", "README.md"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "init"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "remote", "add", "origin", str(remote)], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "-u", "origin", "main"], cwd=local, check=True, capture_output=True)


def _make_feature_branch_with_decision(local: Path, branch: str = "feature") -> None:
    subprocess.run(["git", "checkout", "-b", branch], cwd=local, check=True, capture_output=True)
    docs = local / "docs"
    docs.mkdir(exist_ok=True)
    (docs / "timeline.md").write_text(
        "# Timeline\n\n- branch entry 1\n", encoding="utf-8"
    )
    subprocess.run(["git", "add", "docs/timeline.md"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "branch decision"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "-u", "origin", branch], cwd=local, check=True, capture_output=True)


def _push_to_main_timeline_only(local: Path) -> None:
    """Push a commit to main that ONLY touches docs/timeline.md."""
    subprocess.run(["git", "checkout", "main"], cwd=local, check=True, capture_output=True)
    docs = local / "docs"
    docs.mkdir(exist_ok=True)
    (docs / "timeline.md").write_text(
        "# Timeline\n\n- main entry 1\n", encoding="utf-8"
    )
    subprocess.run(["git", "add", "docs/timeline.md"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "main timeline update"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "origin", "main"], cwd=local, check=True, capture_output=True)


# ---------------------------------------------------------------------------
# Hard safety gates — auto-rebase REFUSES when conditions don't hold
# ---------------------------------------------------------------------------


def test_refuses_when_disabled(tmp_path):
    result = maybe_auto_rebase(tmp_path, "feature", "main", enabled=False)
    assert result.fired is False
    assert "disabled" in result.reason.lower()


def test_refuses_on_default_branch(tmp_path):
    """On main itself, no upstream-vs-feature gap to rebase across."""
    result = maybe_auto_rebase(tmp_path, "main", "main", enabled=True)
    assert result.fired is False
    assert "default branch" in result.reason.lower()


def test_refuses_when_not_a_git_repo(tmp_path):
    """No .git dir → fetch fails → graceful no-op."""
    result = maybe_auto_rebase(tmp_path, "feature", "main", enabled=True)
    assert result.fired is False
    # Either "fetch failed" or "no upstream ref" — both acceptable
    assert result.fired is False


def test_noop_when_branch_up_to_date(tmp_path):
    """Branch level with upstream → fired=False, reason="up-to-date"."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    subprocess.run(["git", "checkout", "-b", "feature"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "-u", "origin", "feature"], cwd=local, check=True, capture_output=True)

    result = maybe_auto_rebase(local, "feature", "main", enabled=True)
    assert result.fired is False
    assert "up-to-date" in result.reason.lower() or "up to date" in result.reason.lower()
    assert result.commits_behind == 0


# ---------------------------------------------------------------------------
# The deterministic-safety gate — gap must be EXACTLY {docs/timeline.md}
# ---------------------------------------------------------------------------


def test_refuses_when_gap_includes_code_file(tmp_path):
    """Code change on main → REFUSE. Even if timeline.md is also touched,
    the presence of any code file means we don't know how to resolve."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    _make_feature_branch_with_decision(local, "feature")

    # Main commit: touch both code + timeline
    subprocess.run(["git", "checkout", "main"], cwd=local, check=True, capture_output=True)
    (local / "feature.py").write_text("def f(): pass\n", encoding="utf-8")
    (local / "docs").mkdir(exist_ok=True)
    (local / "docs" / "timeline.md").write_text(
        "# Timeline\n\n- main timeline\n", encoding="utf-8"
    )
    subprocess.run(["git", "add", "-A"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "main code + timeline"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "origin", "main"], cwd=local, check=True, capture_output=True)

    subprocess.run(["git", "checkout", "feature"], cwd=local, check=True, capture_output=True)

    result = maybe_auto_rebase(local, "feature", "main", enabled=True)
    assert result.fired is False, (
        f"v0.6.1: auto-rebase MUST refuse when gap touches non-timeline files. "
        f"Got: {result}"
    )
    assert "feature.py" in result.reason
    assert result.commits_behind == 1


def test_refuses_when_gap_includes_file_structure_md(tmp_path):
    """SAFETY: even file-structure.md (also a derived doc) is NOT allowed
    in the v0.6.1 gap. Narrow scope = trusted scope."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    _make_feature_branch_with_decision(local, "feature")

    subprocess.run(["git", "checkout", "main"], cwd=local, check=True, capture_output=True)
    docs = local / "docs"
    docs.mkdir(exist_ok=True)
    (docs / "timeline.md").write_text("# Timeline\n\n- main\n", encoding="utf-8")
    (docs / "file-structure.md").write_text("# Tree\n", encoding="utf-8")
    subprocess.run(["git", "add", "-A"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "main timeline + file-structure"], cwd=local, check=True, capture_output=True)
    subprocess.run(["git", "push", "origin", "main"], cwd=local, check=True, capture_output=True)

    subprocess.run(["git", "checkout", "feature"], cwd=local, check=True, capture_output=True)

    result = maybe_auto_rebase(local, "feature", "main", enabled=True)
    assert result.fired is False, (
        f"v0.6.1: file-structure.md in the gap MUST refuse — only timeline.md "
        f"is in the allowed set. Got: {result}"
    )
    assert "file-structure.md" in result.reason


# ---------------------------------------------------------------------------
# The narrow happy path: gap is EXACTLY {docs/timeline.md}
# ---------------------------------------------------------------------------


def test_fires_when_gap_is_timeline_md_only(tmp_path, monkeypatch):
    """The whole point: when conditions hold (timeline.md-only gap),
    auto-rebase + push. We mock `logmind timeline --write` since the
    test repo doesn't have the full logmind install context, but the
    rebase + push flow is exercised end-to-end."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    _make_feature_branch_with_decision(local, "feature")
    _push_to_main_timeline_only(local)
    subprocess.run(["git", "checkout", "feature"], cwd=local, check=True, capture_output=True)

    # Mock the `logmind timeline --write docs/timeline.md` subprocess
    # call to actually write a unified file (since the real CLI needs
    # logmind installed in the test repo's venv).
    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        # Convert PosixPath cmd elements to strings
        cmd_list = [str(c) for c in cmd] if isinstance(cmd, list) else cmd
        if cmd_list[:2] == ["logmind", "timeline"]:
            # Pretend the regen produces the union of branch + main entries
            target = local / "docs" / "timeline.md"
            target.parent.mkdir(exist_ok=True)
            target.write_text(
                "# Timeline\n\n- main entry 1\n- branch entry 1\n",
                encoding="utf-8",
            )
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.core.auto_rebase.subprocess.run", side_effect=mock_run):
        result = maybe_auto_rebase(local, "feature", "main", enabled=True)

    assert result.fired is True, (
        f"v0.6.1 happy path: timeline-md-only gap MUST fire auto-rebase. "
        f"Got: {result}"
    )
    assert result.pushed is True
    assert result.commits_behind == 1
    assert "timeline.md" in result.reason


# ---------------------------------------------------------------------------
# Force-with-lease is ALWAYS used (never --force)
# ---------------------------------------------------------------------------


def test_uses_force_with_lease_not_force(tmp_path, monkeypatch):
    """Critical safety property: the push must use --force-with-lease,
    never --force. This guards against concurrent-push races."""
    remote = tmp_path / "remote.git"
    remote.mkdir()
    local = tmp_path / "local"
    local.mkdir()
    _git_init_with_remote(local, remote)
    _make_feature_branch_with_decision(local, "feature")
    _push_to_main_timeline_only(local)
    subprocess.run(["git", "checkout", "feature"], cwd=local, check=True, capture_output=True)

    push_commands: list = []
    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        cmd_list = [str(c) for c in cmd] if isinstance(cmd, list) else cmd
        if "push" in cmd_list:
            push_commands.append(cmd_list)
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")
        if cmd_list[:2] == ["logmind", "timeline"]:
            target = local / "docs" / "timeline.md"
            target.write_text("# Timeline\n- m\n- b\n", encoding="utf-8")
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.core.auto_rebase.subprocess.run", side_effect=mock_run):
        maybe_auto_rebase(local, "feature", "main", enabled=True)

    assert push_commands, "v0.6.1: must invoke at least one git push"
    for cmd in push_commands:
        assert "--force-with-lease" in cmd, (
            f"v0.6.1 SAFETY: must use --force-with-lease, not --force. Got: {cmd}"
        )
        assert "--force" not in [c for c in cmd if c != "--force-with-lease"], (
            f"v0.6.1 SAFETY: must NEVER use --force. Got: {cmd}"
        )
