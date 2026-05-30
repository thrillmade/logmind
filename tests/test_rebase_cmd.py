"""Tests for `logmind rebase` (v0.5.13).

Convenience wrapper around the fetch + rebase + force-with-lease pattern
captured from the tokenomics agent's Phase D friction report (2026-05-30).
"""

from __future__ import annotations

import subprocess
from pathlib import Path
from unittest.mock import patch

import pytest
from click.testing import CliRunner

from logmind.cli import main as cli_main


def _git_init_with_branch(repo: Path, branch: str = "feature") -> None:
    """Init repo + commit to main + check out a feature branch."""
    subprocess.run(["git", "init", "-b", "main"], cwd=repo, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.email", "t@t.com"], cwd=repo, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=repo, check=True, capture_output=True)
    (repo / "README.md").write_text("seed\n", encoding="utf-8")
    subprocess.run(["git", "add", "README.md"], cwd=repo, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "init"], cwd=repo, check=True, capture_output=True)
    if branch != "main":
        subprocess.run(["git", "checkout", "-b", branch], cwd=repo, check=True, capture_output=True)


def test_rebase_refuses_outside_git_repo(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    runner = CliRunner()
    result = runner.invoke(cli_main, ["rebase"])
    assert result.exit_code == 1
    assert "not in a git repository" in result.output.lower()


def test_rebase_refuses_on_detached_head(tmp_path, monkeypatch):
    _git_init_with_branch(tmp_path, branch="main")
    # Make a second commit so we can detach onto the first one.
    (tmp_path / "x.txt").write_text("x\n", encoding="utf-8")
    subprocess.run(["git", "add", "x.txt"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-qm", "second"], cwd=tmp_path, check=True, capture_output=True)
    # Detach.
    first_sha = subprocess.run(
        ["git", "rev-parse", "HEAD~1"], cwd=tmp_path, check=True, capture_output=True, text=True
    ).stdout.strip()
    subprocess.run(
        ["git", "checkout", "--detach", first_sha], cwd=tmp_path, check=True, capture_output=True
    )
    monkeypatch.chdir(tmp_path)

    runner = CliRunner()
    result = runner.invoke(cli_main, ["rebase"])
    assert result.exit_code == 1
    assert "detached head" in result.output.lower()


def test_rebase_refuses_on_default_branch(tmp_path, monkeypatch):
    _git_init_with_branch(tmp_path, branch="main")
    monkeypatch.chdir(tmp_path)

    runner = CliRunner()
    result = runner.invoke(cli_main, ["rebase"])
    assert result.exit_code == 1
    # Either "main" mentioned OR "refusing to rebase ... onto itself"
    assert "main" in result.output.lower() or "itself" in result.output.lower()


def test_rebase_happy_path_runs_fetch_rebase_push(tmp_path, monkeypatch):
    """Happy path: fetch + rebase + push --force-with-lease in order."""
    _git_init_with_branch(tmp_path, branch="feature")
    monkeypatch.chdir(tmp_path)

    calls: list[list[str]] = []
    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        # Capture git fetch / rebase / push; let everything else through.
        if cmd[:1] == ["git"] and cmd[1:2] in (["fetch"], ["rebase"], ["push"]):
            calls.append(cmd)
            # Pretend all 3 succeed cleanly.
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["rebase"])

    assert result.exit_code == 0, result.output
    # Three commands in order: fetch, rebase, push --force-with-lease.
    assert calls[0][:3] == ["git", "fetch", "origin"]
    assert calls[1][:2] == ["git", "rebase"]
    assert calls[1][2].startswith("origin/")
    assert calls[2][:4] == ["git", "push", "--force-with-lease", "origin"]


def test_rebase_no_push_flag_skips_push(tmp_path, monkeypatch):
    _git_init_with_branch(tmp_path, branch="feature")
    monkeypatch.chdir(tmp_path)

    calls: list[list[str]] = []
    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        if cmd[:1] == ["git"] and cmd[1:2] in (["fetch"], ["rebase"], ["push"]):
            calls.append(cmd)
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["rebase", "--no-push"])

    assert result.exit_code == 0, result.output
    # Only fetch + rebase ran; push skipped.
    git_verbs = [c[1] for c in calls]
    assert "push" not in git_verbs
    assert "fetch" in git_verbs and "rebase" in git_verbs


def test_rebase_no_fetch_flag_skips_fetch(tmp_path, monkeypatch):
    _git_init_with_branch(tmp_path, branch="feature")
    monkeypatch.chdir(tmp_path)

    calls: list[list[str]] = []
    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        if cmd[:1] == ["git"] and cmd[1:2] in (["fetch"], ["rebase"], ["push"]):
            calls.append(cmd)
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["rebase", "--no-fetch"])

    assert result.exit_code == 0, result.output
    git_verbs = [c[1] for c in calls]
    assert "fetch" not in git_verbs
    assert "rebase" in git_verbs and "push" in git_verbs


def test_rebase_rebase_failure_surfaces_clear_message(tmp_path, monkeypatch):
    """When git rebase fails (conflicts), surface the error + recovery hint."""
    _git_init_with_branch(tmp_path, branch="feature")
    monkeypatch.chdir(tmp_path)

    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        if cmd[:2] == ["git", "fetch"]:
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")
        if cmd[:2] == ["git", "rebase"]:
            raise subprocess.CalledProcessError(
                returncode=1, cmd=cmd, stderr="CONFLICT (content): merge conflict in docs/timeline.md"
            )
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["rebase"])

    assert result.exit_code == 1
    assert "rebase failed" in result.output.lower()
    # User-facing recovery hint must be present.
    assert "rebase --continue" in result.output or "rebase --abort" in result.output


def test_rebase_push_failure_surfaces_clear_message(tmp_path, monkeypatch):
    """When push fails (e.g., protected branch), surface the error and
    note that rebase succeeded locally."""
    _git_init_with_branch(tmp_path, branch="feature")
    monkeypatch.chdir(tmp_path)

    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        if cmd[:2] in (["git", "fetch"], ["git", "rebase"]):
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")
        if cmd[:2] == ["git", "push"]:
            raise subprocess.CalledProcessError(
                returncode=1, cmd=cmd, stderr="remote: branch protection rejected"
            )
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["rebase"])

    assert result.exit_code == 1
    assert "push" in result.output.lower() and "failed" in result.output.lower()
    # Reassures user that local rebase succeeded (only push failed).
    assert "rebase succeeded locally" in result.output.lower()


def test_rebase_custom_base_branch_via_flag(tmp_path, monkeypatch):
    _git_init_with_branch(tmp_path, branch="feature")
    monkeypatch.chdir(tmp_path)

    calls: list[list[str]] = []
    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        if cmd[:1] == ["git"] and cmd[1:2] in (["fetch"], ["rebase"], ["push"]):
            calls.append(cmd)
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="", stderr="")
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["rebase", "--base", "develop"])

    assert result.exit_code == 0, result.output
    # Both fetch + rebase target the custom base.
    fetch_cmd = next(c for c in calls if c[1] == "fetch")
    rebase_cmd = next(c for c in calls if c[1] == "rebase")
    assert fetch_cmd[-1] == "develop"
    assert rebase_cmd[-1] == "origin/develop"
