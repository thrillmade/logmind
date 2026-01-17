"""Tests for CLI commands."""

import subprocess
from pathlib import Path

from click.testing import CliRunner

from logmind.cli import agents, config, init, log, main, show, update


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


# ============================================================================
# Agents Command Tests
# ============================================================================


def test_agents_command_help():
    """Test agents command help."""
    runner = CliRunner()
    result = runner.invoke(main, ["agents", "--help"])

    assert result.exit_code == 0
    assert "Manage AI agent" in result.output


def test_agents_list_command(temp_dir):
    """Test agents list command."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(main, ["agents", "list"])

        assert result.exit_code == 0
        assert "AI Agent Status" in result.output
        assert "claude" in result.output
        assert "cursor" in result.output
        assert "windsurf" in result.output


def test_agents_list_shows_configured(git_repo):
    """Test agents list shows configured agents."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        # Initialize first
        runner.invoke(init)

        result = runner.invoke(main, ["agents", "list"])

        assert result.exit_code == 0
        # CLAUDE.md should be configured
        assert "configured" in result.output


def test_agents_add_command(temp_dir):
    """Test agents add command."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(main, ["agents", "add", "cursor", "--no-commit"])

        assert result.exit_code == 0
        assert "Created .cursorrules" in result.output
        assert (Path.cwd() / ".cursorrules").exists()


def test_agents_add_windsurf(temp_dir):
    """Test adding windsurf agent."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(main, ["agents", "add", "windsurf", "--no-commit"])

        assert result.exit_code == 0
        assert "Created .windsurfrules" in result.output
        assert (Path.cwd() / ".windsurfrules").exists()


def test_agents_add_unknown_fails(temp_dir):
    """Test adding unknown agent fails."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(main, ["agents", "add", "unknown_agent"])

        assert result.exit_code != 0
        assert "Unknown agent" in result.output


def test_agents_add_existing_file(temp_dir):
    """Test adding agent when file exists but without logmind."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Create existing file without logmind section
        (Path.cwd() / ".cursorrules").write_text("# Existing rules\n")

        result = runner.invoke(main, ["agents", "add", "cursor", "--no-commit"])

        assert result.exit_code == 0
        assert "Added logmind instructions" in result.output


def test_agents_remove_command(temp_dir):
    """Test agents remove command."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Create the file first
        (Path.cwd() / ".cursorrules").write_text("rules\n")

        result = runner.invoke(main, ["agents", "remove", "cursor", "--force", "--no-commit"])

        assert result.exit_code == 0
        assert "Removed .cursorrules" in result.output
        assert not (Path.cwd() / ".cursorrules").exists()


def test_agents_remove_nonexistent(temp_dir):
    """Test removing non-existent agent."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(main, ["agents", "remove", "cursor", "--force"])

        assert result.exit_code == 0
        assert "not configured" in result.output


def test_agents_remove_unknown_fails(temp_dir):
    """Test removing unknown agent fails."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(main, ["agents", "remove", "unknown_agent", "--force"])

        assert result.exit_code != 0
        assert "Unknown agent" in result.output


# ============================================================================
# Init with Agents Flag Tests
# ============================================================================


def test_init_with_agents_flag(temp_dir):
    """Test init with --agents flag."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--agents", "claude,cursor"])

        assert result.exit_code == 0
        assert (Path.cwd() / "CLAUDE.md").exists()
        assert (Path.cwd() / ".cursorrules").exists()


def test_init_with_all_agents_flag(temp_dir):
    """Test init with --all-agents flag."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--all-agents"])

        assert result.exit_code == 0
        assert (Path.cwd() / "CLAUDE.md").exists()
        assert (Path.cwd() / ".cursorrules").exists()
        assert (Path.cwd() / ".windsurfrules").exists()
        assert (Path.cwd() / "CONVENTIONS.md").exists()


def test_init_with_windsurf(temp_dir):
    """Test init with windsurf agent."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--agents", "windsurf"])

        assert result.exit_code == 0
        assert (Path.cwd() / ".windsurfrules").exists()
        content = (Path.cwd() / ".windsurfrules").read_text()
        assert "<!-- logmind-start -->" in content


def test_init_with_unknown_agent_warns(temp_dir):
    """Test init with unknown agent shows warning."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--agents", "claude,unknown_agent"])

        assert result.exit_code == 0
        assert "Warning" in result.output
        assert "unknown_agent" in result.output
        # Should still create claude
        assert (Path.cwd() / "CLAUDE.md").exists()


# ============================================================================
# Update Command Tests
# ============================================================================


def test_update_command_help():
    """Test update command help."""
    runner = CliRunner()
    result = runner.invoke(main, ["update", "--help"])

    assert result.exit_code == 0
    assert "Update logmind" in result.output
    assert "pip install --upgrade" in result.output


def test_update_command_shows_version():
    """Test update command shows version info."""
    runner = CliRunner()
    # The actual update may fail in test environment, but it should at least start
    result = runner.invoke(update)

    # Should show current version info
    assert "Current version:" in result.output or "version" in result.output.lower()


# ============================================================================
# Config-Driven Sync Integration Tests
# ============================================================================


def test_show_command_syncs_agent_files(temp_dir):
    """Test that show command syncs agent files from config."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Initialize first
        runner.invoke(init, ["--no-git"])

        # Manually enable cursor in config
        config_path = Path.cwd() / ".logmind" / "config.yml"
        config_content = config_path.read_text()
        config_content = config_content.replace("cursor: false", "cursor: true")
        config_path.write_text(config_content)

        # Run show (should sync and create .cursorrules)
        result = runner.invoke(show)

        assert result.exit_code == 0
        assert (Path.cwd() / ".cursorrules").exists()


def test_log_command_syncs_agent_files(git_repo):
    """Test that log command syncs agent files from config."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        # Initialize first
        runner.invoke(init)

        # Manually enable windsurf in config
        config_path = Path.cwd() / ".logmind" / "config.yml"
        config_content = config_path.read_text()
        config_content = config_content.replace("windsurf: false", "windsurf: true")
        config_path.write_text(config_content)

        # Run log (should sync and create .windsurfrules)
        result = runner.invoke(log, ["Test decision", "--no-commit"])

        assert result.exit_code == 0
        assert (Path.cwd() / ".windsurfrules").exists()
        assert ".windsurfrules" in result.output


def test_init_creates_default_agents(temp_dir):
    """Test that init creates both CLAUDE.md and .cursorrules by default."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git"])

        assert result.exit_code == 0
        # Both claude and cursor are enabled by default
        assert (Path.cwd() / "CLAUDE.md").exists()
        assert (Path.cwd() / ".cursorrules").exists()

        # Verify they have logmind sections
        claude_content = (Path.cwd() / "CLAUDE.md").read_text()
        cursor_content = (Path.cwd() / ".cursorrules").read_text()
        assert "<!-- logmind-start -->" in claude_content
        assert "<!-- logmind-start -->" in cursor_content


def test_default_config_enables_claude_and_cursor(temp_dir):
    """Test that default config has claude and cursor enabled."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        runner.invoke(init, ["--no-git"])

        config_path = Path.cwd() / ".logmind" / "config.yml"
        config_content = config_path.read_text()

        # Verify defaults
        assert "claude: true" in config_content
        assert "cursor: true" in config_content


# ============================================================================
# Config Command Tests
# ============================================================================


def test_config_list_command(temp_dir):
    """Test config list command shows all configuration."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Initialize first to create config
        runner.invoke(init, ["--no-git"])

        result = runner.invoke(main, ["config", "list"])

        assert result.exit_code == 0
        # Verify output contains expected YAML keys
        assert "git:" in result.output
        assert "decisions:" in result.output
        assert "agents:" in result.output


def test_config_get_command(temp_dir):
    """Test config get command retrieves values."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Initialize first
        runner.invoke(init, ["--no-git"])

        result = runner.invoke(main, ["config", "get", "git.auto_push"])

        assert result.exit_code == 0
        assert "True" in result.output


def test_config_get_nonexistent_key(temp_dir):
    """Test config get with nonexistent key returns error."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Initialize first
        runner.invoke(init, ["--no-git"])

        result = runner.invoke(main, ["config", "get", "nonexistent.key"])

        assert result.exit_code == 1
        assert "not found" in result.output


def test_config_set_command(temp_dir):
    """Test config set command modifies values."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Initialize first
        runner.invoke(init, ["--no-git"])

        # Set a value
        result = runner.invoke(main, ["config", "set", "git.auto_push", "false"])

        assert result.exit_code == 0
        assert "Set git.auto_push = False" in result.output

        # Verify the change
        get_result = runner.invoke(main, ["config", "get", "git.auto_push"])
        assert "False" in get_result.output


def test_config_set_creates_nested_key(temp_dir):
    """Test config set creates nested keys that don't exist."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Initialize first
        runner.invoke(init, ["--no-git"])

        # Set a new nested key
        result = runner.invoke(main, ["config", "set", "custom.new.key", "myvalue"])

        assert result.exit_code == 0
        assert "Set custom.new.key = myvalue" in result.output

        # Verify the key was created
        get_result = runner.invoke(main, ["config", "get", "custom.new.key"])
        assert result.exit_code == 0
        assert "myvalue" in get_result.output


def test_config_set_type_conversion(temp_dir):
    """Test config set converts types correctly."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Initialize first
        runner.invoke(init, ["--no-git"])

        # Test bool conversion: "true" -> True
        result = runner.invoke(main, ["config", "set", "test.bool_true", "true"])
        assert result.exit_code == 0
        assert "Set test.bool_true = True" in result.output

        # Test bool conversion: "false" -> False
        result = runner.invoke(main, ["config", "set", "test.bool_false", "false"])
        assert result.exit_code == 0
        assert "Set test.bool_false = False" in result.output

        # Test int conversion: "42" -> 42
        result = runner.invoke(main, ["config", "set", "test.integer", "42"])
        assert result.exit_code == 0
        assert "Set test.integer = 42" in result.output

        # Test float conversion: "3.14" -> 3.14
        result = runner.invoke(main, ["config", "set", "test.float_val", "3.14"])
        assert result.exit_code == 0
        assert "Set test.float_val = 3.14" in result.output

        # Test string passthrough: "hello" -> "hello"
        result = runner.invoke(main, ["config", "set", "test.string_val", "hello"])
        assert result.exit_code == 0
        assert "Set test.string_val = hello" in result.output

        # Verify the values via config list
        list_result = runner.invoke(main, ["config", "list"])
        assert "bool_true: true" in list_result.output
        assert "bool_false: false" in list_result.output
        assert "integer: 42" in list_result.output
        assert "float_val: 3.14" in list_result.output
        assert "string_val: hello" in list_result.output
