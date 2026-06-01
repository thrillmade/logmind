"""Tests for v0.6.8's `logmind init --with-skdd` opt-in flag.

The flag subprocesses to `npx --yes clud-bug@latest init` after the
standard logmind scaffolding completes. Failure modes:
- npx missing on PATH → graceful warning, logmind side still succeeds
- subprocess returns non-zero → warning surfaced, logmind side unaffected
- subprocess raises OSError → defensive catch, warning surfaced
"""

from __future__ import annotations

import subprocess
from pathlib import Path
from unittest.mock import MagicMock, patch

from click.testing import CliRunner

from logmind.cli import main as cli_main


def test_init_with_skdd_flag_is_accepted(tmp_path: Path, monkeypatch):
    """Flag exists + doesn't error during parse."""
    monkeypatch.chdir(tmp_path)
    # Run with --no-git to skip git ops + --skill-install false to avoid prompt
    # + use a mock for npx absence (which triggers the warning-not-error path).
    with patch("logmind.cli.shutil.which", return_value=None):
        runner = CliRunner()
        result = runner.invoke(
            cli_main,
            ["init", "--no-git", "--no-skill-install", "--with-skdd"],
        )
    assert result.exit_code == 0, result.output


def _selective_subprocess_mock(npx_returncode: int = 0):
    """Build a selective subprocess.run mock that intercepts ONLY npx
    invocations, leaving other subprocess.run calls (git status checks,
    etc.) to the real implementation.

    Returns (side_effect_fn, npx_calls_list). After invocation, inspect
    npx_calls_list to assert on call args.
    """
    real_run = subprocess.run
    npx_calls = []

    def selective_run(cmd, *a, **kw):
        if isinstance(cmd, list) and cmd and cmd[0] == "npx":
            npx_calls.append(cmd)
            mock_result = MagicMock()
            mock_result.returncode = npx_returncode
            return mock_result
        return real_run(cmd, *a, **kw)

    return selective_run, npx_calls


def test_init_without_skdd_flag_does_not_invoke_npx(tmp_path: Path, monkeypatch):
    """Default behavior (no --with-skdd) skips the subprocess entirely."""
    monkeypatch.chdir(tmp_path)
    selective_run, npx_calls = _selective_subprocess_mock()
    with patch("logmind.cli.subprocess.run", side_effect=selective_run), \
         patch("logmind.cli.shutil.which", return_value="/usr/bin/npx"):
        runner = CliRunner()
        result = runner.invoke(
            cli_main, ["init", "--no-git", "--no-skill-install"]
        )
    assert result.exit_code == 0, result.output
    assert len(npx_calls) == 0, "npx should not run without --with-skdd"


def test_init_with_skdd_invokes_npx_when_available(tmp_path: Path, monkeypatch):
    """When --with-skdd is passed AND npx is on PATH, subprocess fires."""
    monkeypatch.chdir(tmp_path)
    selective_run, npx_calls = _selective_subprocess_mock(npx_returncode=0)
    with patch("logmind.cli.subprocess.run", side_effect=selective_run), \
         patch("logmind.cli.shutil.which", return_value="/usr/bin/npx"):
        runner = CliRunner()
        result = runner.invoke(
            cli_main,
            ["init", "--no-git", "--no-skill-install", "--with-skdd"],
        )
    assert result.exit_code == 0, result.output
    assert len(npx_calls) == 1, "exactly one npx call expected"
    assert npx_calls[0] == ["npx", "--yes", "clud-bug@latest", "init"]


def test_init_with_skdd_warns_when_npx_missing(tmp_path: Path, monkeypatch):
    """No npx on PATH → friendly warning, exit 0 (logmind side succeeded)."""
    monkeypatch.chdir(tmp_path)
    with patch("logmind.cli.shutil.which", return_value=None):
        runner = CliRunner()
        result = runner.invoke(
            cli_main,
            ["init", "--no-git", "--no-skill-install", "--with-skdd"],
        )
    assert result.exit_code == 0, result.output
    assert "npx" in result.output.lower()
    assert "node" in result.output.lower()
    # Recovery hint surfaced
    assert "clud-bug@latest init" in result.output


def test_init_with_skdd_warns_when_npx_exits_nonzero(tmp_path: Path, monkeypatch):
    """If npx subprocess returns non-zero, warn but do not fail init."""
    monkeypatch.chdir(tmp_path)
    selective_run, _ = _selective_subprocess_mock(npx_returncode=7)
    with patch("logmind.cli.subprocess.run", side_effect=selective_run), \
         patch("logmind.cli.shutil.which", return_value="/usr/bin/npx"):
        runner = CliRunner()
        result = runner.invoke(
            cli_main,
            ["init", "--no-git", "--no-skill-install", "--with-skdd"],
        )
    # logmind init still succeeds despite npx failure
    assert result.exit_code == 0, result.output
    assert "exited 7" in result.output or "warning" in result.output.lower()


def test_init_with_skdd_warns_when_npx_raises_oserror(tmp_path: Path, monkeypatch):
    """Defensive: if subprocess raises OSError (e.g., permission error)
    when invoking npx, catch + warn + still succeed.

    The mock targets only the npx invocation specifically — other
    subprocess.run calls in init (git config, etc.) need to work
    normally, otherwise the test fails for the wrong reason.
    """
    monkeypatch.chdir(tmp_path)
    real_run = subprocess.run

    def selective_run(cmd, *a, **kw):
        if isinstance(cmd, list) and cmd and cmd[0] == "npx":
            raise OSError("simulated npx failure")
        return real_run(cmd, *a, **kw)

    with patch("logmind.cli.subprocess.run", side_effect=selective_run), \
         patch("logmind.cli.shutil.which", return_value="/usr/bin/npx"):
        runner = CliRunner()
        result = runner.invoke(
            cli_main,
            ["init", "--no-git", "--no-skill-install", "--with-skdd"],
        )
    assert result.exit_code == 0, result.output
    assert "warning" in result.output.lower()
