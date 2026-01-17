"""Tests for core/config.py."""

from pathlib import Path

import pytest

from logmind.core.config import Config, DEFAULT_CONFIG, load_config


def test_config_loads_defaults_when_no_file(temp_dir):
    """Test that config loads defaults when file doesn't exist."""
    config_path = temp_dir / ".logmind" / "config.yml"
    config = Config(config_path)

    assert config.auto_commit is True
    assert config.auto_push is True
    assert config.max_recent_decisions == 20


def test_config_loads_from_file(temp_dir):
    """Test loading config from YAML file."""
    config_dir = temp_dir / ".logmind"
    config_dir.mkdir()

    config_file = config_dir / "config.yml"
    config_file.write_text("""
git:
  auto_commit: false
  auto_push: false
  commit_message_template: "decision: {decision}"

decisions:
  max_recent: 30
""")

    config = Config(config_file)

    assert config.auto_commit is False
    assert config.auto_push is False
    assert config.commit_message_template == "decision: {decision}"
    assert config.max_recent_decisions == 30


def test_config_get_with_dot_notation(temp_dir):
    """Test getting values with dot notation."""
    config_path = temp_dir / ".logmind" / "config.yml"
    config = Config(config_path)

    assert config.get("git.auto_commit") is True
    assert config.get("decisions.max_recent") == 20
    assert config.get("nonexistent.key", "default") == "default"


def test_config_set_value(temp_dir):
    """Test setting config values."""
    config_path = temp_dir / ".logmind" / "config.yml"
    config = Config(config_path)

    config.set("git.auto_commit", False)
    assert config.get("git.auto_commit") is False

    config.set("custom.new.key", "value")
    assert config.get("custom.new.key") == "value"


def test_config_save(temp_dir):
    """Test saving config to file."""
    config_dir = temp_dir / ".logmind"
    config_dir.mkdir()

    config_file = config_dir / "config.yml"
    config = Config(config_file)

    config.set("git.auto_push", False)
    config.set("decisions.max_recent", 15)
    config.save()

    # Verify file was created
    assert config_file.exists()

    # Load again and verify values persisted
    config2 = Config(config_file)
    assert config2.get("git.auto_push") is False
    assert config2.get("decisions.max_recent") == 15


def test_config_properties(temp_dir):
    """Test config convenience properties."""
    config_path = temp_dir / ".logmind" / "config.yml"
    config = Config(config_path)

    assert isinstance(config.auto_commit, bool)
    assert isinstance(config.auto_push, bool)
    assert isinstance(config.commit_message_template, str)
    assert isinstance(config.max_recent_decisions, int)
    assert isinstance(config.auto_update_file_structure, bool)
    assert isinstance(config.ignore_patterns, list)


def test_config_merges_with_defaults(temp_dir):
    """Test that partial config merges with defaults."""
    config_dir = temp_dir / ".logmind"
    config_dir.mkdir()

    config_file = config_dir / "config.yml"
    # Only override one value
    config_file.write_text("""
git:
  auto_push: false
""")

    config = Config(config_file)

    # Override should work
    assert config.auto_push is False

    # Defaults should still be present
    assert config.auto_commit is True
    assert config.max_recent_decisions == 20


def test_load_config_helper(temp_dir):
    """Test load_config helper function."""
    config_dir = temp_dir / ".logmind"
    config_dir.mkdir()

    config_file = config_dir / "config.yml"
    config_file.write_text("""
git:
  auto_commit: false
""")

    # Change to temp dir
    import os
    old_cwd = os.getcwd()
    try:
        os.chdir(temp_dir)
        config = load_config()
        assert config.auto_commit is False
    finally:
        os.chdir(old_cwd)


def test_config_handles_corrupt_yaml(temp_dir):
    """Test that corrupt YAML falls back to defaults."""
    config_dir = temp_dir / ".logmind"
    config_dir.mkdir()

    config_file = config_dir / "config.yml"
    config_file.write_text("invalid: yaml: content: ][")

    config = Config(config_file)

    # Should fall back to defaults
    assert config.auto_commit is True
    assert config.auto_push is True


def test_default_config_structure():
    """Test that DEFAULT_CONFIG has expected structure."""
    assert "git" in DEFAULT_CONFIG
    assert "decisions" in DEFAULT_CONFIG
    assert "file_structure" in DEFAULT_CONFIG
    assert "agents" in DEFAULT_CONFIG

    assert "auto_commit" in DEFAULT_CONFIG["git"]
    assert "auto_push" in DEFAULT_CONFIG["git"]
    assert "commit_message_template" in DEFAULT_CONFIG["git"]

    assert "max_recent" in DEFAULT_CONFIG["decisions"]

    assert "ignore_patterns" in DEFAULT_CONFIG["file_structure"]
    assert isinstance(DEFAULT_CONFIG["file_structure"]["ignore_patterns"], list)


def test_config_agents_property(temp_dir):
    """Test config agents property."""
    config_path = temp_dir / ".logmind" / "config.yml"
    config = Config(config_path)

    agents = config.agents
    assert isinstance(agents, dict)
    assert "claude" in agents
    assert "cursor" in agents
    assert "windsurf" in agents
    assert agents["claude"] is True  # Default enabled
    assert agents["cursor"] is True  # Default enabled


def test_config_get_enabled_agents(temp_dir):
    """Test getting enabled agents."""
    config_path = temp_dir / ".logmind" / "config.yml"
    config = Config(config_path)

    enabled = config.get_enabled_agents()
    assert isinstance(enabled, list)
    assert "claude" in enabled  # Claude is enabled by default
    assert "cursor" in enabled  # Cursor is enabled by default
    assert "windsurf" not in enabled  # Other agents disabled by default


def test_config_agents_from_file(temp_dir):
    """Test loading agents config from file."""
    config_dir = temp_dir / ".logmind"
    config_dir.mkdir()

    config_file = config_dir / "config.yml"
    config_file.write_text("""
agents:
  claude: true
  cursor: true
  copilot: false
  windsurf: true
""")

    config = Config(config_file)
    enabled = config.get_enabled_agents()

    assert "claude" in enabled
    assert "cursor" in enabled
    assert "windsurf" in enabled
    assert "copilot" not in enabled


def test_default_agents_structure():
    """Test that DEFAULT_CONFIG has all agents."""
    agents = DEFAULT_CONFIG["agents"]
    expected = ["claude", "cursor", "copilot", "windsurf", "aider", "continue", "cody", "zed", "amazonq", "cline", "codex"]

    for agent in expected:
        assert agent in agents
        assert isinstance(agents[agent], bool)
