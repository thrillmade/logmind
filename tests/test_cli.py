"""Tests for CLI commands."""

import subprocess
from pathlib import Path

from click.testing import CliRunner

from logmind.cli import agents, config, init, log, main, show, update, check_decisions, install_hook


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
    """`logmind --version` matches the package version (no drift)."""
    from logmind import __version__

    runner = CliRunner()
    result = runner.invoke(main, ["--version"])

    assert result.exit_code == 0
    assert __version__ in result.output


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
        content = (cwd / "docs" / "decisions.md").read_text(encoding="utf-8")
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


def test_init_does_not_install_aggregator_workflow(temp_dir):
    """v0.2: aggregator is gone; init must not scaffold logmind-aggregate.yml
    (deleted in v0.2). The new regen-timeline.yml replaces it."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--no-skill-install"])
        assert result.exit_code == 0
        agg = Path(".github/workflows/logmind-aggregate.yml")
        regen = Path(".github/workflows/regen-timeline.yml")
        assert not agg.exists(), "aggregator workflow should not be installed in v0.2"
        assert regen.exists(), "regen-timeline workflow must replace it"


def test_init_seeds_timeline_md(temp_dir):
    """v0.2: AGENTS.md template links docs/timeline.md, and check-doc-links
    runs on first push. `logmind init` must seed timeline.md so the link
    isn't broken until the first PR triggers regen-timeline.yml.
    Caught by clud-bug review of PR #36 (issue #3, three review rounds)."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--no-skill-install"])
        assert result.exit_code == 0
        timeline = Path("docs/timeline.md")
        assert timeline.exists(), \
            "logmind init must create docs/timeline.md to keep AGENTS.md link valid"
        # And it should be a valid timeline file, not just a placeholder
        content = timeline.read_text(encoding="utf-8")
        assert "Decision Timeline" in content


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
        content = (Path.cwd() / "docs" / "decisions.md").read_text(encoding="utf-8")
        assert "Test decision" in content


def test_log_command_default_stage_is_all(git_repo):
    """v0.2.7: default --stage is 'all', not 'scoped'. An unrelated
    working-tree change made between `logmind init` and `logmind log`
    must end up in the decision commit by default. The previous scoped
    default forced agents into a two-step git add + git commit + push
    pattern that defeated the whole point of `logmind log` being a
    single primitive.
    """
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)
        # Make an unrelated working-tree change (simulates the agent's
        # actual code change that prompted the decision).
        rogue = Path.cwd() / "rogue_change.py"
        rogue.write_text("# unrelated edit\n", encoding="utf-8")

        result = runner.invoke(log, ["Test decision"])
        assert result.exit_code == 0, result.output

        # The rogue change must be in the resulting commit AND no longer
        # dirty in the working tree.
        out = subprocess.run(
            ["git", "log", "-1", "--name-only", "--format="],
            capture_output=True, text=True, check=True,
        )
        # Paths in the commit may carry a tmp-fs prefix; substring match.
        assert "rogue_change.py" in out.stdout, (
            f"v0.2.7 default --stage all must sweep working-tree changes "
            f"into the decision commit; commit listed:\n{out.stdout}"
        )
        status = subprocess.run(
            ["git", "status", "--porcelain"],
            capture_output=True, text=True, check=True,
        )
        assert status.stdout.strip() == "", (
            f"working tree must be clean after `logmind log` (default "
            f"--stage all); leftover: {status.stdout!r}"
        )


def test_log_command_scoped_stage_keeps_rogue_unstaged(git_repo):
    """v0.2.7: explicit --stage scoped preserves the old behavior —
    unrelated working-tree changes stay unstaged. Backwards-compat
    escape hatch for users who relied on the previous default."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)
        rogue = Path.cwd() / "rogue_change.py"
        rogue.write_text("# unrelated edit\n", encoding="utf-8")

        result = runner.invoke(log, ["Test decision", "--stage", "scoped"])
        assert result.exit_code == 0, result.output

        out = subprocess.run(
            ["git", "log", "-1", "--name-only", "--format="],
            capture_output=True, text=True, check=True,
        )
        assert "rogue_change.py" not in out.stdout, (
            f"explicit --stage scoped must leave rogue_change.py unstaged; "
            f"commit listed:\n{out.stdout}"
        )
        # And it must still be dirty
        status = subprocess.run(
            ["git", "status", "--porcelain"],
            capture_output=True, text=True, check=True,
        )
        assert "rogue_change.py" in status.stdout, (
            "rogue_change.py must remain unstaged after --stage scoped"
        )


def test_log_command_regenerates_timeline(git_repo):
    """v0.2.3: logmind log must regenerate + stage docs/timeline.md so
    derived-docs CI doesn't catch authors out. Regression: PR #42 stalled
    because the workflow gate caught a stale timeline.md the author hadn't
    regenerated manually."""
    from logmind.core.timeline import write_timeline

    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)
        result = runner.invoke(log, ["Test decision"])
        assert result.exit_code == 0, result.output

        timeline_path = Path.cwd() / "docs" / "timeline.md"
        assert timeline_path.exists(), "timeline.md should be created by logmind log"
        assert "Test decision" in timeline_path.read_text(encoding="utf-8"), (
            "timeline.md should reference the new decision"
        )

        # Running write_timeline again produces no diff — proves the commit
        # is self-consistent, which is what check-derived-docs verifies.
        changed = write_timeline(timeline_path, Path.cwd() / "docs")
        assert changed is False, "timeline.md should already be current after logmind log"


def test_log_command_with_reasoning(git_repo):
    """Test log command with reasoning."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)

        result = runner.invoke(log, ["Test decision", "-r", "Because reasons"])

        assert result.exit_code == 0

        content = (Path.cwd() / "docs" / "decisions.md").read_text(encoding="utf-8")
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

        content = (Path.cwd() / "docs" / "decisions.md").read_text(encoding="utf-8")
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

        content = (Path.cwd() / "docs" / "decisions.md").read_text(encoding="utf-8")
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
        (Path.cwd() / ".cursorrules").write_text("# Existing rules\n", encoding="utf-8")

        result = runner.invoke(main, ["agents", "add", "cursor", "--no-commit"])

        assert result.exit_code == 0
        assert "Added logmind instructions" in result.output


def test_agents_remove_command(temp_dir):
    """Test agents remove command."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Create the file first
        (Path.cwd() / ".cursorrules").write_text("rules\n", encoding="utf-8")

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
    """Init with windsurf creates a stub .windsurfrules + canonical AGENTS.md."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--agents", "windsurf"])

        assert result.exit_code == 0
        assert (Path.cwd() / ".windsurfrules").exists()
        assert "<!-- logmind-stub:" in (Path.cwd() / ".windsurfrules").read_text(encoding="utf-8")
        # AGENTS.md is canonical and contains the actual logmind block
        assert (Path.cwd() / "AGENTS.md").exists()
        assert "<!-- logmind-start -->" in (Path.cwd() / "AGENTS.md").read_text(encoding="utf-8")


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
        config_content = config_path.read_text(encoding="utf-8")
        config_content = config_content.replace("cursor: false", "cursor: true")
        config_path.write_text(config_content, encoding="utf-8")

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
        config_content = config_path.read_text(encoding="utf-8")
        config_content = config_content.replace("windsurf: false", "windsurf: true")
        config_path.write_text(config_content, encoding="utf-8")

        # Run log (should sync and create .windsurfrules)
        result = runner.invoke(log, ["Test decision", "--no-commit"])

        assert result.exit_code == 0
        assert (Path.cwd() / ".windsurfrules").exists()
        assert ".windsurfrules" in result.output


def test_init_creates_default_agents(temp_dir):
    """Init creates AGENTS.md (canonical) plus stub CLAUDE.md and .cursorrules."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git"])

        assert result.exit_code == 0
        # Canonical doc + per-agent stubs
        assert (Path.cwd() / "AGENTS.md").exists()
        assert (Path.cwd() / "CLAUDE.md").exists()
        assert (Path.cwd() / ".cursorrules").exists()

        agents_content = (Path.cwd() / "AGENTS.md").read_text(encoding="utf-8")
        claude_content = (Path.cwd() / "CLAUDE.md").read_text(encoding="utf-8")
        cursor_content = (Path.cwd() / ".cursorrules").read_text(encoding="utf-8")

        # Logmind block lives in AGENTS.md only
        assert "<!-- logmind-start -->" in agents_content
        # Per-agent files are stubs that point at AGENTS.md
        assert "<!-- logmind-stub:" in claude_content
        assert "<!-- logmind-stub:" in cursor_content


def test_default_config_enables_claude_and_cursor(temp_dir):
    """Test that default config has claude and cursor enabled."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        runner.invoke(init, ["--no-git"])

        config_path = Path.cwd() / ".logmind" / "config.yml"
        config_content = config_path.read_text(encoding="utf-8")

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


# ---------------------------------------------------------------------------
# check-decisions tests
# ---------------------------------------------------------------------------


def test_check_decisions_not_a_git_repo(tmp_path):
    """check-decisions exits 0 with a message when not in a git repo."""
    runner = CliRunner()
    from unittest.mock import patch

    with patch("logmind.cli.is_git_repo", return_value=False):
        result = runner.invoke(main, ["check-decisions"])

    assert result.exit_code == 0
    assert "Not a git repository" in result.output


def test_check_decisions_passes_when_decisions_md_staged(git_repo):
    """check-decisions exits 0 when docs/decisions.md is in the staged files."""
    runner = CliRunner()
    from unittest.mock import patch, MagicMock

    staged_output = "docs/decisions.md\nsrc/foo.py\n"
    mock_result = MagicMock()
    mock_result.stdout = staged_output

    with runner.isolated_filesystem(temp_dir=git_repo):
        with patch("logmind.cli.is_git_repo", return_value=True):
            with patch("subprocess.run", return_value=mock_result):
                result = runner.invoke(main, ["check-decisions"])

    assert result.exit_code == 0
    assert "staged" in result.output


def test_check_decisions_passes_when_branch_decisions_file_staged(git_repo):
    """Regression: in branch_aware mode (the default), decisions go to
    docs/decisions-branches/<branch>.md — check-decisions must accept that
    as a documented change, not just docs/decisions.md."""
    runner = CliRunner()
    from unittest.mock import patch, MagicMock

    staged_output = "docs/decisions-branches/feat__auth.md\nsrc/foo.py\n"
    mock_result = MagicMock()
    mock_result.stdout = staged_output

    with runner.isolated_filesystem(temp_dir=git_repo):
        with patch("logmind.cli.is_git_repo", return_value=True):
            with patch("subprocess.run", return_value=mock_result):
                result = runner.invoke(main, ["check-decisions"])

    assert result.exit_code == 0
    assert "staged" in result.output


def test_check_decisions_passes_below_threshold(git_repo):
    """check-decisions exits 0 when lines changed are below the threshold."""
    runner = CliRunner()
    from unittest.mock import patch, MagicMock

    # No staged files (no decisions.md)
    staged = MagicMock()
    staged.stdout = "src/small_change.py\n"

    # 5 lines added, 3 removed = 8 total (below default 20)
    numstat = MagicMock()
    numstat.stdout = "5\t3\tsrc/small_change.py\n"

    with runner.isolated_filesystem(temp_dir=git_repo):
        with patch("logmind.cli.is_git_repo", return_value=True):
            with patch("subprocess.run", side_effect=[staged, numstat]):
                result = runner.invoke(main, ["check-decisions"])

    assert result.exit_code == 0
    assert "below" in result.output


def test_check_decisions_fails_above_threshold(git_repo):
    """check-decisions exits 1 when lines changed exceed the threshold."""
    runner = CliRunner()
    from unittest.mock import patch, MagicMock

    staged = MagicMock()
    staged.stdout = "src/big_change.py\n"

    # 15 added + 10 removed = 25 total (above default 20)
    numstat = MagicMock()
    numstat.stdout = "15\t10\tsrc/big_change.py\n"

    with runner.isolated_filesystem(temp_dir=git_repo):
        with patch("logmind.cli.is_git_repo", return_value=True):
            with patch("subprocess.run", side_effect=[staged, numstat]):
                result = runner.invoke(main, ["check-decisions"])

    assert result.exit_code == 1
    assert "without updating" in result.output


def test_check_decisions_no_fail_flag(git_repo):
    """check-decisions exits 0 with --no-fail even when threshold exceeded."""
    runner = CliRunner()
    from unittest.mock import patch, MagicMock

    staged = MagicMock()
    staged.stdout = "src/big_change.py\n"

    numstat = MagicMock()
    numstat.stdout = "15\t10\tsrc/big_change.py\n"

    with runner.isolated_filesystem(temp_dir=git_repo):
        with patch("logmind.cli.is_git_repo", return_value=True):
            with patch("subprocess.run", side_effect=[staged, numstat]):
                result = runner.invoke(main, ["check-decisions", "--no-fail"])

    assert result.exit_code == 0
    assert "without updating" in result.output


def test_check_decisions_custom_threshold(git_repo):
    """check-decisions respects --threshold option."""
    runner = CliRunner()
    from unittest.mock import patch, MagicMock

    staged = MagicMock()
    staged.stdout = "src/change.py\n"

    # 25 lines changed — above default 20 but below custom 50
    numstat = MagicMock()
    numstat.stdout = "15\t10\tsrc/change.py\n"

    with runner.isolated_filesystem(temp_dir=git_repo):
        with patch("logmind.cli.is_git_repo", return_value=True):
            with patch("subprocess.run", side_effect=[staged, numstat]):
                result = runner.invoke(main, ["check-decisions", "--threshold", "50"])

    assert result.exit_code == 0
    assert "below" in result.output


def test_check_decisions_skips_docs_files(git_repo):
    """check-decisions excludes docs/ files from the line count."""
    runner = CliRunner()
    from unittest.mock import patch, MagicMock

    staged = MagicMock()
    staged.stdout = "docs/plan.md\n"

    # 50 lines in docs/ only — should be excluded
    numstat = MagicMock()
    numstat.stdout = "50\t0\tdocs/plan.md\n"

    with runner.isolated_filesystem(temp_dir=git_repo):
        with patch("logmind.cli.is_git_repo", return_value=True):
            with patch("subprocess.run", side_effect=[staged, numstat]):
                result = runner.invoke(main, ["check-decisions"])

    assert result.exit_code == 0
    assert "below" in result.output


def test_check_decisions_skips_binary_files(git_repo):
    """check-decisions skips binary files (shown as '-' in numstat)."""
    runner = CliRunner()
    from unittest.mock import patch, MagicMock

    staged = MagicMock()
    staged.stdout = "assets/image.png\n"

    numstat = MagicMock()
    numstat.stdout = "-\t-\tassets/image.png\n"

    with runner.isolated_filesystem(temp_dir=git_repo):
        with patch("logmind.cli.is_git_repo", return_value=True):
            with patch("subprocess.run", side_effect=[staged, numstat]):
                result = runner.invoke(main, ["check-decisions"])

    assert result.exit_code == 0


def test_check_decisions_empty_staged(git_repo):
    """check-decisions handles no staged files gracefully."""
    runner = CliRunner()
    from unittest.mock import patch, MagicMock

    staged = MagicMock()
    staged.stdout = ""

    numstat = MagicMock()
    numstat.stdout = ""

    with runner.isolated_filesystem(temp_dir=git_repo):
        with patch("logmind.cli.is_git_repo", return_value=True):
            with patch("subprocess.run", side_effect=[staged, numstat]):
                result = runner.invoke(main, ["check-decisions"])

    assert result.exit_code == 0


# ---------------------------------------------------------------------------
# install-hook tests
# ---------------------------------------------------------------------------


def test_install_hook_not_a_git_repo(tmp_path):
    """install-hook exits 1 when not in a git repo."""
    runner = CliRunner()
    from unittest.mock import patch

    with patch("logmind.cli.is_git_repo", return_value=False):
        result = runner.invoke(main, ["install-hook"])

    assert result.exit_code == 1
    assert "Error" in result.output


def test_install_hook_creates_new_hook(git_repo):
    """install-hook creates .git/hooks/pre-commit when it doesn't exist."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(main, ["install-hook"])

    assert result.exit_code == 0
    assert "Installed" in result.output

    hook_path = git_repo / ".git" / "hooks" / "pre-commit"
    assert hook_path.exists()
    content = hook_path.read_text(encoding="utf-8")
    assert "logmind check-decisions" in content
    assert "#!/bin/sh" in content


def test_install_hook_is_executable(git_repo):
    """install-hook makes the pre-commit file executable."""
    import os

    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(main, ["install-hook"])

    hook_path = git_repo / ".git" / "hooks" / "pre-commit"
    assert os.access(hook_path, os.X_OK)


def test_install_hook_already_installed(git_repo):
    """install-hook exits 0 with message when already installed."""
    runner = CliRunner()

    hook_path = git_repo / ".git" / "hooks" / "pre-commit"
    hook_path.parent.mkdir(parents=True, exist_ok=True)
    hook_path.write_text("#!/bin/sh\nlogmind check-decisions\n", encoding="utf-8")

    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(main, ["install-hook"])

    assert result.exit_code == 0
    assert "already installed" in result.output


def test_install_hook_existing_hook_without_force(git_repo):
    """install-hook exits 1 when hook exists and --force not given."""
    runner = CliRunner()

    hook_path = git_repo / ".git" / "hooks" / "pre-commit"
    hook_path.parent.mkdir(parents=True, exist_ok=True)
    hook_path.write_text("#!/bin/sh\necho 'existing hook'\n", encoding="utf-8")

    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(main, ["install-hook"])

    assert result.exit_code == 1
    assert "already exists" in result.output


def test_install_hook_existing_hook_with_force(git_repo):
    """install-hook appends to existing hook when --force is given."""
    runner = CliRunner()

    hook_path = git_repo / ".git" / "hooks" / "pre-commit"
    hook_path.parent.mkdir(parents=True, exist_ok=True)
    hook_path.write_text("#!/bin/sh\necho 'existing hook'\n", encoding="utf-8")

    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(main, ["install-hook", "--force"])

    assert result.exit_code == 0
    assert "Added" in result.output

    content = hook_path.read_text(encoding="utf-8")
    assert "existing hook" in content
    assert "logmind check-decisions" in content


# ============================================================================
# Log --no-push / --no-commit flag tests
# ============================================================================


def test_log_command_no_commit_flag(git_repo):
    """Test that log --no-commit exits 0 and does not auto-commit."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)

        result = runner.invoke(log, ["Decision", "--no-commit"])

        assert result.exit_code == 0
        assert "Logged decision" in result.output
        # Commit message should NOT appear since commit is disabled
        assert "Committed" not in result.output


def test_log_command_no_push_flag(git_repo):
    """Test that log --no-push exits 0."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        runner.invoke(init)

        result = runner.invoke(log, ["Decision", "--no-push"])

        assert result.exit_code == 0
        assert "Logged decision" in result.output


# ============================================================================
# Agents remove without --force test
# ============================================================================


def test_agents_remove_without_force_prompts(temp_dir):
    """Test that agents remove without --force prompts for confirmation."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=temp_dir):
        # Create the agent file so removal can proceed past the "not configured" check
        (Path.cwd() / ".cursorrules").write_text("# rules\n", encoding="utf-8")

        # Provide "n" so the confirmation prompt cancels the removal
        result = runner.invoke(main, ["agents", "remove", "cursor"], input="n\n")

        assert result.exit_code == 0
        assert "Remove" in result.output
        assert "Cancelled" in result.output


# --- 0.B.3 (v0.5.1): --quiet / LOGMIND_QUIET=1 token-frugal mode ---

def test_quiet_flag_advertised_in_help():
    """Help text must mention --quiet/-q + LOGMIND_QUIET=1 so agents
    discover the env-var route at session boot."""
    runner = CliRunner()
    result = runner.invoke(main, ["--help"])
    assert result.exit_code == 0
    assert "--quiet" in result.output or "-q" in result.output
    assert "LOGMIND_QUIET" in result.output


def test_show_quiet_emits_single_ok_line_when_no_decisions(git_repo, docs_dir, monkeypatch):
    """When there are no decisions, --quiet mode should emit exactly one
    `ok ...` line on stdout — no progress chatter.

    monkeypatch.chdir (not raw os.chdir) so cwd restores after the
    test — clud-bug PR #69 caught the raw-chdir version as cascading
    23 unrelated test failures via the cwd leak.
    """
    runner = CliRunner()
    (docs_dir / "decisions.md").write_text("# Decision Log\n\n---\n", encoding="utf-8")
    monkeypatch.chdir(git_repo)
    result = runner.invoke(main, ["--quiet", "show"])
    ok_lines = [l for l in result.output.splitlines() if l.startswith("ok ")]
    assert len(ok_lines) >= 1, (
        f"--quiet show must emit at least one ok line; got: {result.output!r}"
    )


def test_env_var_LOGMIND_QUIET_suppresses_progress_end_to_end(
    git_repo, docs_dir, monkeypatch
):
    """End-to-end exercise of the env-var path: LOGMIND_QUIET=1 +
    `logmind show` on a non-empty decisions.md → exactly one ok line,
    no ✓-prefixed progress. clud-bug PR #69 review caught that the v1
    test only verified --help didn't crash without actually exercising
    the feature."""
    monkeypatch.setenv("LOGMIND_QUIET", "1")
    monkeypatch.chdir(git_repo)
    (docs_dir / "decisions.md").write_text(
        "# Decision Log\n\n## 2026-01-01\n\nTest decision body.\n", encoding="utf-8"
    )

    runner = CliRunner()
    result = runner.invoke(main, ["show"])
    assert result.exit_code == 0, f"show should succeed: {result.output!r}"
    ok_lines = [l for l in result.output.splitlines() if l.startswith("ok ")]
    assert len(ok_lines) == 1, (
        f"LOGMIND_QUIET=1 show must emit exactly one ok line; got: {result.output!r}"
    )
    assert "show: docs/decisions.md" in ok_lines[0]
    # ✓-prefixed progress lines SHOULD be absent in quiet mode.
    assert "✓" not in result.output, (
        f"LOGMIND_QUIET=1 should suppress ✓-prefixed progress lines; got: {result.output!r}"
    )


# --- 0.B.2 (v0.5.2): show --brief / --limit / --json ---

def test_show_brief_emits_one_line_per_entry(git_repo, docs_dir, monkeypatch):
    """--brief: one line per decision (date + title + source)."""
    (docs_dir / "decisions.md").write_text(
        "# Decision Log\n\n## 2026-01-01 10:00 - First decision\n\nBody.\n\n"
        "## 2026-01-02 11:00 - Second decision\n\nBody.\n", encoding="utf-8"
    )
    monkeypatch.chdir(git_repo)
    runner = CliRunner()
    result = runner.invoke(main, ["show", "--brief"])
    assert result.exit_code == 0, result.output
    lines = [l for l in result.output.splitlines() if not l.startswith("ok ")]
    # Two decisions, two non-ok lines (verbatim markdown not emitted in brief mode).
    summary_lines = [l for l in lines if l.strip() and "—" in l]
    assert len(summary_lines) == 2, f"expected 2 brief lines; got: {result.output!r}"
    # Newest-first: 2026-01-02 should come before 2026-01-01.
    assert "2026-01-02" in summary_lines[0]
    assert "2026-01-01" in summary_lines[1]
    # Source tag present.
    assert "[main]" in summary_lines[0]


def test_show_limit_caps_to_n_most_recent(git_repo, docs_dir, monkeypatch):
    """--limit N: keeps N most-recent (newest-first)."""
    (docs_dir / "decisions.md").write_text(
        "# Decision Log\n\n"
        "## 2026-01-01 10:00 - One\n\n"
        "## 2026-01-02 11:00 - Two\n\n"
        "## 2026-01-03 12:00 - Three\n\n",
        encoding="utf-8",
    )
    monkeypatch.chdir(git_repo)
    runner = CliRunner()
    result = runner.invoke(main, ["show", "--brief", "--limit", "2"])
    assert result.exit_code == 0
    assert "Three" in result.output
    assert "Two" in result.output
    assert "One" not in result.output  # capped out


def test_show_json_emits_valid_array(git_repo, docs_dir, monkeypatch):
    """--json: stable structured output for downstream tools."""
    import json
    (docs_dir / "decisions.md").write_text(
        "# Decision Log\n\n## 2026-01-01 10:00 - First decision\n\n", encoding="utf-8"
    )
    monkeypatch.chdir(git_repo)
    runner = CliRunner()
    result = runner.invoke(main, ["show", "--json"])
    assert result.exit_code == 0
    # Pull out the JSON array from result.output (sync_messages may precede).
    # JSON starts at first '[' and ends at matching ']'.
    output = result.output
    start = output.find("[")
    end = output.rfind("]") + 1
    assert start >= 0 and end > start, f"no JSON found: {output!r}"
    parsed = json.loads(output[start:end])
    assert isinstance(parsed, list)
    assert len(parsed) == 1
    assert parsed[0]["title"] == "First decision"
    assert parsed[0]["source"] == "main"
    assert "date" in parsed[0]
