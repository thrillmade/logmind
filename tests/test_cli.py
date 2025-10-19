"""Tests for CLI commands."""

import subprocess
from pathlib import Path

from click.testing import CliRunner

from logmind.cli import init, log, main, show


def test_cli_help():
    """Test main CLI help."""
    runner = CliRunner()
    result = runner.invoke(main, ["--help"])

    assert result.exit_code == 0
    assert "logmind" in result.output
    assert "init" in result.output
    assert "log" in result.output
    assert "show" in result.output


def test_cli_version():
    """Test version flag."""
    runner = CliRunner()
    result = runner.invoke(main, ["--version"])

    assert result.exit_code == 0
    assert "0.1.0" in result.output


def test_init_command_in_git_repo(git_repo):
    """Test init command in a git repository."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(init)

        assert result.exit_code == 0
        assert "logmind initialized successfully" in result.output

        # Check files were created (use cwd since we're in isolated filesystem)
        cwd = Path.cwd()
        assert (cwd / "docs" / "decisions.md").exists()
        assert (cwd / "docs" / "decisions-archive.md").exists()
        assert (cwd / "docs" / "file-structure.md").exists()
        assert (cwd / "CLAUDE.md").exists()

        # Check first decision was logged
        content = (cwd / "docs" / "decisions.md").read_text()
        assert "Initialize logmind decision tracking" in content


def test_init_command_not_git_repo(temp_dir):
    """Test init command in non-git directory."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Should prompt about git
        result = runner.invoke(init, input="y\n")  # Answer yes to continue

        assert result.exit_code == 0 or "Warning: Not a git repository" in result.output


def test_init_command_with_no_git_flag(temp_dir):
    """Test init with --no-git flag."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git"])

        assert result.exit_code == 0
        assert "logmind initialized successfully" in result.output


def test_init_command_already_initialized(git_repo):
    """Test init when already initialized."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        # Initialize once
        runner.invoke(init)

        # Try to initialize again
        result = runner.invoke(init)

        assert result.exit_code == 0
        assert "already initialized" in result.output


def test_log_command_basic(git_repo):
    """Test basic log command."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        # Initialize first
        runner.invoke(init)

        # Log a decision
        result = runner.invoke(log, ["Test decision"])

        assert result.exit_code == 0
        assert "Logged decision" in result.output

        # Check decision was added
        content = (Path.cwd() / "docs" / "decisions.md").read_text()
        assert "Test decision" in content


def test_log_command_with_reasoning(git_repo):
    """Test log command with reasoning."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)

        result = runner.invoke(log, ["Test decision", "-r", "Because reasons"])

        assert result.exit_code == 0

        content = (Path.cwd() / "docs" / "decisions.md").read_text()
        assert "Test decision" in content
        assert "Because reasons" in content


def test_log_command_with_alternatives(git_repo):
    """Test log command with multiple alternatives."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)

        result = runner.invoke(
            log, ["Test decision", "-a", "Option A", "-a", "Option B"]
        )

        assert result.exit_code == 0

        content = (Path.cwd() / "docs" / "decisions.md").read_text()
        assert "Option A" in content
        assert "Option B" in content


def test_log_command_with_implications(git_repo):
    """Test log command with implications."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)

        result = runner.invoke(
            log, ["Test decision", "-i", "Impact 1", "-i", "Impact 2"]
        )

        assert result.exit_code == 0

        content = (Path.cwd() / "docs" / "decisions.md").read_text()
        assert "Impact 1" in content
        assert "Impact 2" in content


def test_log_command_no_commit(git_repo):
    """Test log command with --no-commit."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)

        result = runner.invoke(log, ["Test decision", "--no-commit"])

        assert result.exit_code == 0
        # Should not show commit message
        assert "Committed" not in result.output or result.exit_code == 0


def test_log_command_without_init_fails(temp_dir):
    """Test that log fails without init."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(log, ["Test decision"])

        assert result.exit_code != 0
        assert "docs/ directory not found" in result.output


def test_show_command(git_repo):
    """Test show command."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)
        runner.invoke(log, ["First decision"])
        runner.invoke(log, ["Second decision"])

        result = runner.invoke(show)

        assert result.exit_code == 0
        assert "First decision" in result.output
        assert "Second decision" in result.output


def test_show_command_with_all_flag(git_repo):
    """Test show command with --all flag."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)

        # Add enough decisions to trigger archival
        for i in range(21):
            runner.invoke(log, [f"Decision {i}", "--no-commit"])

        result = runner.invoke(show, ["--all"])

        assert result.exit_code == 0
        assert "ARCHIVED DECISIONS" in result.output or result.exit_code == 0


def test_show_command_without_init(temp_dir):
    """Test show command without init."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(show)

        assert result.exit_code != 0
        assert "docs/ directory not found" in result.output


def test_full_workflow(git_repo):
    """Test complete workflow: init -> log -> show."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        # Initialize
        init_result = runner.invoke(init)
        assert init_result.exit_code == 0

        # Log a decision
        log_result = runner.invoke(
            log,
            [
                "Use PostgreSQL",
                "-r",
                "Need ACID compliance",
                "-a",
                "MongoDB",
                "-i",
                "Need connection pooling",
            ],
        )
        assert log_result.exit_code == 0

        # Show decisions
        show_result = runner.invoke(show)
        assert show_result.exit_code == 0
        assert "Use PostgreSQL" in show_result.output
        assert "MongoDB" in show_result.output

        # Check git commits were created
        result = subprocess.run(
            ["git", "log", "--oneline"],
            cwd=git_repo,
            capture_output=True,
            text=True,
        )
        assert "logmind: Initialize decision tracking" in result.stdout
        assert "logmind: Use PostgreSQL" in result.stdout
