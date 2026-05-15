"""Tests for core/inserter.py."""

from pathlib import Path

from logmind.core.inserter import (
    AGENT_REGISTRY,
    create_agent_file,
    create_claude_md,
    find_ai_instruction_files,
    get_agent_display_name,
    get_agent_file_path,
    get_agent_status,
    get_agent_template,
    get_all_agent_names,
    get_full_claude_template,
    get_logmind_section,
    has_logmind_section,
    insert_into_all_ai_files,
    insert_logmind_section,
    is_agent_json,
    remove_agent_file,
    sync_agent_files_from_config,
)


# ============================================================================
# Test Agent Registry
# ============================================================================


def test_agent_registry_contains_all_agents():
    """Test that the registry contains all expected agents."""
    expected_agents = [
        "claude",
        "cursor",
        "copilot",
        "windsurf",
        "aider",
        "continue",
        "cody",
        "zed",
        "amazonq",
        "cline",
        "codex",
    ]
    for agent in expected_agents:
        assert agent in AGENT_REGISTRY


def test_get_all_agent_names():
    """Test getting all agent names."""
    names = get_all_agent_names()
    assert len(names) == 11
    assert "claude" in names
    assert "cursor" in names
    assert "windsurf" in names
    assert "cline" in names
    assert "codex" in names


def test_get_agent_file_path(temp_dir):
    """Test getting agent file paths."""
    assert get_agent_file_path("claude", temp_dir) == temp_dir / "CLAUDE.md"
    assert get_agent_file_path("cursor", temp_dir) == temp_dir / ".cursorrules"
    assert get_agent_file_path("copilot", temp_dir) == temp_dir / ".github" / "copilot-instructions.md"
    assert get_agent_file_path("windsurf", temp_dir) == temp_dir / ".windsurfrules"
    assert get_agent_file_path("aider", temp_dir) == temp_dir / "CONVENTIONS.md"
    assert get_agent_file_path("continue", temp_dir) == temp_dir / ".continuerules"
    assert get_agent_file_path("cody", temp_dir) == temp_dir / ".sourcegraph" / "cody.json"
    assert get_agent_file_path("zed", temp_dir) == temp_dir / ".zed" / "settings.json"
    assert get_agent_file_path("amazonq", temp_dir) == temp_dir / ".amazonq" / "rules.md"
    assert get_agent_file_path("cline", temp_dir) == temp_dir / ".clinerules"
    assert get_agent_file_path("codex", temp_dir) == temp_dir / "AGENTS.md"
    assert get_agent_file_path("unknown", temp_dir) is None


def test_get_agent_display_name():
    """Test getting agent display names."""
    assert get_agent_display_name("claude") == "Claude Code"
    assert get_agent_display_name("cursor") == "Cursor"
    assert get_agent_display_name("copilot") == "GitHub Copilot"
    assert get_agent_display_name("windsurf") == "Windsurf"
    assert get_agent_display_name("unknown") == "unknown"


def test_is_agent_json():
    """Test JSON agent detection."""
    assert is_agent_json("cody") is True
    assert is_agent_json("zed") is True
    assert is_agent_json("claude") is False
    assert is_agent_json("cursor") is False
    assert is_agent_json("windsurf") is False
    assert is_agent_json("unknown") is False


# ============================================================================
# Test File Detection
# ============================================================================


def test_find_ai_instruction_files_empty(temp_dir):
    """Test finding AI files when none exist."""
    files = find_ai_instruction_files(temp_dir)
    assert len(files) == 0


def test_find_ai_instruction_files_claude(temp_dir):
    """Test finding CLAUDE.md."""
    (temp_dir / "CLAUDE.md").write_text("# CLAUDE.md\n", encoding="utf-8")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 1
    assert files[0][0] == temp_dir / "CLAUDE.md"
    assert files[0][1] == "claude"


def test_find_ai_instruction_files_cursor(temp_dir):
    """Test finding .cursorrules."""
    (temp_dir / ".cursorrules").write_text("rules\n", encoding="utf-8")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 1
    assert files[0][0] == temp_dir / ".cursorrules"
    assert files[0][1] == "cursor"


def test_find_ai_instruction_files_copilot(temp_dir):
    """Test finding copilot instructions."""
    github_dir = temp_dir / ".github"
    github_dir.mkdir()
    (github_dir / "copilot-instructions.md").write_text("# Instructions\n", encoding="utf-8")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 1
    assert files[0][0] == github_dir / "copilot-instructions.md"
    assert files[0][1] == "copilot"


def test_find_ai_instruction_files_windsurf(temp_dir):
    """Test finding .windsurfrules."""
    (temp_dir / ".windsurfrules").write_text("rules\n", encoding="utf-8")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 1
    assert files[0][0] == temp_dir / ".windsurfrules"
    assert files[0][1] == "windsurf"


def test_find_ai_instruction_files_aider(temp_dir):
    """Test finding CONVENTIONS.md."""
    (temp_dir / "CONVENTIONS.md").write_text("# Conventions\n", encoding="utf-8")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 1
    assert files[0][0] == temp_dir / "CONVENTIONS.md"
    assert files[0][1] == "aider"


def test_find_ai_instruction_files_multiple(temp_dir):
    """Test finding multiple AI instruction files."""
    (temp_dir / "CLAUDE.md").write_text("# CLAUDE.md\n", encoding="utf-8")
    (temp_dir / ".cursorrules").write_text("rules\n", encoding="utf-8")
    (temp_dir / ".windsurfrules").write_text("rules\n", encoding="utf-8")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 3


# ============================================================================
# Test Agent Status
# ============================================================================


def test_get_agent_status_empty(temp_dir):
    """Test agent status when no files exist."""
    status = get_agent_status(temp_dir)

    assert len(status) == 11  # All agents should be in status
    assert status["claude"]["exists"] is False
    assert status["claude"]["configured"] is False
    assert status["cursor"]["exists"] is False
    assert status["cline"]["exists"] is False
    assert status["codex"]["exists"] is False


def test_get_agent_status_with_files(temp_dir):
    """Test agent status with some files present."""
    # Create CLAUDE.md with logmind section
    (temp_dir / "CLAUDE.md").write_text("# CLAUDE.md\n\n<!-- logmind-start -->\nContent\n<!-- logmind-end -->\n", encoding="utf-8")
    # Create .cursorrules without logmind section
    (temp_dir / ".cursorrules").write_text("rules only\n", encoding="utf-8")

    status = get_agent_status(temp_dir)

    assert status["claude"]["exists"] is True
    assert status["claude"]["configured"] is True
    assert status["cursor"]["exists"] is True
    assert status["cursor"]["configured"] is False  # No logmind section


# ============================================================================
# Test Logmind Section
# ============================================================================


def test_has_logmind_section_false():
    """Test detecting absence of logmind section."""
    content = "# CLAUDE.md\n\nSome content\n"
    assert has_logmind_section(content) is False


def test_has_logmind_section_true():
    """Test detecting presence of logmind section."""
    content = "# CLAUDE.md\n\n<!-- logmind-start -->\nContent\n<!-- logmind-end -->\n"
    assert has_logmind_section(content) is True


def test_get_logmind_section():
    """Test getting logmind section content."""
    section = get_logmind_section()

    assert "<!-- logmind-start -->" in section
    assert "<!-- logmind-end -->" in section
    assert "Decision Logging" in section
    assert "from logmind import log" in section


def test_get_full_claude_template():
    """Test getting full CLAUDE.md template."""
    template = get_full_claude_template()

    assert "# CLAUDE.md" in template
    assert "<!-- logmind-start -->" in template
    assert "Project Overview" in template


# ============================================================================
# Test Agent Templates
# ============================================================================


def test_get_agent_template_claude():
    """Claude template is a stub pointing at AGENTS.md."""
    template = get_agent_template("claude")
    assert "<!-- logmind-stub:" in template
    assert "AGENTS.md" in template


def test_get_agent_template_cursor():
    """Cursor template is a stub pointing at AGENTS.md."""
    template = get_agent_template("cursor")
    assert "<!-- logmind-stub:" in template
    assert "AGENTS.md" in template


def test_get_agent_template_windsurf():
    """Windsurf template is a stub pointing at AGENTS.md."""
    template = get_agent_template("windsurf")
    assert "<!-- logmind-stub:" in template
    assert "AGENTS.md" in template


def test_get_agent_template_aider():
    """Aider template is a stub pointing at AGENTS.md."""
    template = get_agent_template("aider")
    assert "<!-- logmind-stub:" in template
    assert "AGENTS.md" in template


def test_get_agent_template_cody_json():
    """Test getting Cody template (JSON format)."""
    template = get_agent_template("cody")
    assert '"logmind"' in template
    assert '"enabled": true' in template


def test_get_agent_template_zed_json():
    """Test getting Zed template (JSON format)."""
    template = get_agent_template("zed")
    assert '"logmind"' in template
    assert '"enabled": true' in template


# ============================================================================
# Test Insertion
# ============================================================================


def test_insert_logmind_section_new_file(temp_dir):
    """Test inserting logmind section into new file."""
    claude_file = temp_dir / "CLAUDE.md"
    claude_file.write_text("# CLAUDE.md\n\nExisting content\n", encoding="utf-8")

    result = insert_logmind_section(claude_file)

    assert result is True
    content = claude_file.read_text(encoding="utf-8")
    assert "<!-- logmind-start -->" in content
    assert "Existing content" in content  # Should preserve existing


def test_insert_logmind_section_already_exists(temp_dir):
    """Test that inserting again returns False."""
    claude_file = temp_dir / "CLAUDE.md"
    claude_file.write_text("# CLAUDE.md\n\n<!-- logmind-start -->\nAlready here\n<!-- logmind-end -->\n", encoding="utf-8")

    result = insert_logmind_section(claude_file)

    assert result is False  # Should not insert again


def test_insert_logmind_section_after_heading(temp_dir):
    """Test that logmind section is inserted after first heading."""
    claude_file = temp_dir / "CLAUDE.md"
    claude_file.write_text("# CLAUDE.md\n\nThis is guidance.\n\n## Section 2\n\nMore content\n", encoding="utf-8")

    insert_logmind_section(claude_file)

    content = claude_file.read_text(encoding="utf-8")
    lines = content.split("\n")

    # Find where logmind section starts
    logmind_idx = next(i for i, line in enumerate(lines) if "<!-- logmind-start -->" in line)

    # Should be after "# CLAUDE.md" but before "## Section 2"
    heading_idx = next(i for i, line in enumerate(lines) if line.startswith("# CLAUDE.md"))
    section2_idx = next(i for i, line in enumerate(lines) if "## Section 2" in line)

    assert heading_idx < logmind_idx < section2_idx


# ============================================================================
# Test File Creation
# ============================================================================


def test_create_claude_md(temp_dir):
    """Test creating new CLAUDE.md file."""
    claude_file = temp_dir / "CLAUDE.md"

    result = create_claude_md(claude_file)

    assert result == claude_file
    assert claude_file.exists()

    content = claude_file.read_text(encoding="utf-8")
    assert "# CLAUDE.md" in content
    assert "<!-- logmind-start -->" in content


def test_create_agent_file_cursor(temp_dir):
    """Creating .cursorrules now writes the AGENTS.md-pointer stub."""
    result = create_agent_file("cursor", temp_dir)

    assert result == temp_dir / ".cursorrules"
    assert result.exists()

    content = result.read_text(encoding="utf-8")
    assert "<!-- logmind-stub:" in content
    assert "AGENTS.md" in content


def test_create_agent_file_windsurf(temp_dir):
    """Creating .windsurfrules now writes the AGENTS.md-pointer stub."""
    result = create_agent_file("windsurf", temp_dir)

    assert result == temp_dir / ".windsurfrules"
    assert result.exists()

    content = result.read_text(encoding="utf-8")
    assert "<!-- logmind-stub:" in content
    assert "AGENTS.md" in content


def test_create_agent_file_copilot_creates_directory(temp_dir):
    """Test that creating copilot file creates .github directory."""
    result = create_agent_file("copilot", temp_dir)

    assert result == temp_dir / ".github" / "copilot-instructions.md"
    assert result.exists()
    assert (temp_dir / ".github").is_dir()


def test_create_agent_file_cody_json(temp_dir):
    """Test creating Cody JSON file."""
    result = create_agent_file("cody", temp_dir)

    assert result == temp_dir / ".sourcegraph" / "cody.json"
    assert result.exists()

    content = result.read_text(encoding="utf-8")
    assert '"logmind"' in content


def test_create_agent_file_unknown(temp_dir):
    """Test creating unknown agent returns None."""
    result = create_agent_file("unknown_agent", temp_dir)
    assert result is None


# ============================================================================
# Test File Removal
# ============================================================================


def test_remove_agent_file(temp_dir):
    """Test removing agent file."""
    # Create file first
    (temp_dir / ".cursorrules").write_text("rules\n", encoding="utf-8")

    result = remove_agent_file("cursor", temp_dir)

    assert result is True
    assert not (temp_dir / ".cursorrules").exists()


def test_remove_agent_file_not_exists(temp_dir):
    """Test removing non-existent file returns False."""
    result = remove_agent_file("cursor", temp_dir)
    assert result is False


def test_remove_agent_file_unknown(temp_dir):
    """Test removing unknown agent returns False."""
    result = remove_agent_file("unknown", temp_dir)
    assert result is False


# ============================================================================
# Test Insert Into All
# ============================================================================


def test_insert_into_all_ai_files_creates_agents_md(temp_dir):
    """With no existing agent files, AGENTS.md is created as canonical."""
    messages = insert_into_all_ai_files(temp_dir)

    assert (temp_dir / "AGENTS.md").exists()
    joined = "\n".join(messages)
    assert "AGENTS.md" in joined


def test_insert_into_all_ai_files_existing_file(temp_dir):
    """Existing per-agent file with user content gets a logmind block inserted in place."""
    (temp_dir / "CLAUDE.md").write_text("# CLAUDE.md\n\nContent\n", encoding="utf-8")

    messages = insert_into_all_ai_files(temp_dir)

    joined = "\n".join(messages)
    assert "Added logmind instructions" in joined

    content = (temp_dir / "CLAUDE.md").read_text(encoding="utf-8")
    assert "<!-- logmind-start -->" in content
    assert "Content" in content  # Preserved


def test_insert_into_all_ai_files_already_initialized(temp_dir):
    """An already-initialised CLAUDE.md is treated as configured."""
    (temp_dir / "CLAUDE.md").write_text(
        "# CLAUDE.md\n\n<!-- logmind-start -->\nAlready done\n<!-- logmind-end -->\n"
    , encoding="utf-8")

    messages = insert_into_all_ai_files(temp_dir)

    joined = "\n".join(messages)
    assert "already configured" in joined


def test_insert_into_all_ai_files_no_create(temp_dir):
    """create_if_missing=False suppresses creating AGENTS.md too."""
    messages = insert_into_all_ai_files(temp_dir, create_if_missing=False)

    assert len(messages) == 0
    assert not (temp_dir / "CLAUDE.md").exists()
    assert not (temp_dir / "AGENTS.md").exists()


def test_insert_into_all_ai_files_specific_agents(temp_dir):
    """Specific agents create stub files + canonical AGENTS.md."""
    messages = insert_into_all_ai_files(temp_dir, agents=["claude", "cursor", "windsurf"])

    # AGENTS.md (canonical) + 3 stubs
    assert (temp_dir / "AGENTS.md").exists()
    assert (temp_dir / "CLAUDE.md").exists()
    assert (temp_dir / ".cursorrules").exists()
    assert (temp_dir / ".windsurfrules").exists()
    # Each per-agent file is a stub
    for f in (temp_dir / "CLAUDE.md", temp_dir / ".cursorrules", temp_dir / ".windsurfrules"):
        assert "<!-- logmind-stub:" in f.read_text(encoding="utf-8")


def test_insert_into_all_ai_files_unknown_agent(temp_dir):
    """Unknown agent name is reported but doesn't abort the run."""
    messages = insert_into_all_ai_files(temp_dir, agents=["claude", "unknown_agent"])

    joined = "\n".join(messages)
    assert "Unknown agent" in joined
    assert (temp_dir / "CLAUDE.md").exists()


# ============================================================================
# Edge Case Tests
# ============================================================================


def test_create_agent_file_cline(temp_dir):
    """Creating .clinerules now writes the AGENTS.md-pointer stub."""
    result = create_agent_file("cline", temp_dir)

    assert result == temp_dir / ".clinerules"
    assert result.exists()

    content = result.read_text(encoding="utf-8")
    assert "<!-- logmind-stub:" in content
    assert "AGENTS.md" in content


def test_create_agent_file_codex(temp_dir):
    """Test creating AGENTS.md file."""
    result = create_agent_file("codex", temp_dir)

    assert result == temp_dir / "AGENTS.md"
    assert result.exists()

    content = result.read_text(encoding="utf-8")
    assert "# AGENTS.md" in content
    assert "<!-- logmind-start -->" in content


def test_json_templates_are_valid():
    """Test that JSON agent templates are valid JSON."""
    import json

    cody_template = get_agent_template("cody")
    zed_template = get_agent_template("zed")

    # Should not raise
    cody_parsed = json.loads(cody_template)
    zed_parsed = json.loads(zed_template)

    assert "logmind" in cody_parsed
    assert "logmind" in zed_parsed
    assert cody_parsed["logmind"]["enabled"] is True
    assert zed_parsed["logmind"]["enabled"] is True


def test_find_cline_and_codex(temp_dir):
    """Test finding Cline and Codex files."""
    (temp_dir / ".clinerules").write_text("# Cline rules\n", encoding="utf-8")
    (temp_dir / "AGENTS.md").write_text("# AGENTS.md\n", encoding="utf-8")

    files = find_ai_instruction_files(temp_dir)

    agent_names = [name for _, name in files]
    assert "cline" in agent_names
    assert "codex" in agent_names


def test_insert_preserves_unicode(temp_dir):
    """Test that insertion preserves unicode characters."""
    claude_file = temp_dir / "CLAUDE.md"
    claude_file.write_text("# CLAUDE.md\n\n## Japanese: \u65e5\u672c\u8a9e\n\n## Emoji: \U0001F680\n", encoding="utf-8")

    insert_logmind_section(claude_file)

    content = claude_file.read_text(encoding="utf-8")
    assert "\u65e5\u672c\u8a9e" in content  # Japanese text preserved
    assert "\U0001F680" in content  # Emoji preserved
    assert "<!-- logmind-start -->" in content


def test_insert_into_empty_file(temp_dir):
    """Test inserting into an empty file."""
    claude_file = temp_dir / "CLAUDE.md"
    claude_file.write_text("", encoding="utf-8")

    result = insert_logmind_section(claude_file)

    assert result is True
    content = claude_file.read_text(encoding="utf-8")
    assert "<!-- logmind-start -->" in content


def test_insert_into_file_no_heading(temp_dir):
    """Test inserting into file with no heading."""
    claude_file = temp_dir / "CLAUDE.md"
    claude_file.write_text("Some content without heading\n\nMore content\n", encoding="utf-8")

    result = insert_logmind_section(claude_file)

    assert result is True
    content = claude_file.read_text(encoding="utf-8")
    assert "<!-- logmind-start -->" in content
    assert "Some content without heading" in content


# ============================================================================
# Test Config-Driven Agent Sync
# ============================================================================


def test_sync_agent_files_no_config(temp_dir):
    """Test sync returns empty when no config exists."""
    messages = sync_agent_files_from_config(temp_dir)
    assert messages == []


def test_sync_agent_files_creates_missing_files(temp_dir):
    """Test sync creates files for enabled agents."""
    # Create .logmind/config.yml with cursor enabled
    logmind_dir = temp_dir / ".logmind"
    logmind_dir.mkdir()
    config_content = """
agents:
  claude: false
  cursor: true
  windsurf: true
"""
    (logmind_dir / "config.yml").write_text(config_content, encoding="utf-8")

    # Run sync
    messages = sync_agent_files_from_config(temp_dir)

    # Should create .cursorrules and .windsurfrules (plus AGENTS.md auto-ensured)
    assert any(".cursorrules" in msg for msg in messages)
    assert any(".windsurfrules" in msg for msg in messages)
    assert (temp_dir / ".cursorrules").exists()
    assert (temp_dir / ".windsurfrules").exists()
    assert (temp_dir / "AGENTS.md").exists()  # auto-ensured by sync


def test_sync_agent_files_inserts_into_existing(temp_dir):
    """Test sync inserts logmind section into existing files without it."""
    # Create .logmind/config.yml
    logmind_dir = temp_dir / ".logmind"
    logmind_dir.mkdir()
    config_content = """
agents:
  claude: false
  cursor: true
"""
    (logmind_dir / "config.yml").write_text(config_content, encoding="utf-8")

    # Create existing .cursorrules without logmind section
    (temp_dir / ".cursorrules").write_text("# Existing rules\n\nSome rules here\n", encoding="utf-8")

    # Run sync
    messages = sync_agent_files_from_config(temp_dir)

    # Should insert into existing file (AGENTS.md is also auto-ensured)
    assert any("Added logmind section to .cursorrules" in m for m in messages)

    content = (temp_dir / ".cursorrules").read_text(encoding="utf-8")
    assert "<!-- logmind-start -->" in content
    assert "Existing rules" in content


def test_sync_agent_files_skips_configured(temp_dir):
    """Test sync skips files that already have logmind section."""
    # Create .logmind/config.yml
    logmind_dir = temp_dir / ".logmind"
    logmind_dir.mkdir()
    config_content = """
agents:
  claude: false
  cursor: true
"""
    (logmind_dir / "config.yml").write_text(config_content, encoding="utf-8")

    # Create .cursorrules WITH logmind section
    (temp_dir / ".cursorrules").write_text(
        "# Cursor Rules\n\n<!-- logmind-start -->\nContent\n<!-- logmind-end -->\n"
    , encoding="utf-8")

    # Run sync
    messages = sync_agent_files_from_config(temp_dir)

    # The .cursorrules file is already configured — no message about it.
    # AGENTS.md may be auto-created with the canonical block, that's fine.
    assert not any(".cursorrules" in m for m in messages)


def test_sync_agent_files_no_enabled_agents(temp_dir):
    """Test sync returns empty when no agents are enabled."""
    # Create .logmind/config.yml with all agents disabled
    logmind_dir = temp_dir / ".logmind"
    logmind_dir.mkdir()
    config_content = """
agents:
  claude: false
  cursor: false
"""
    (logmind_dir / "config.yml").write_text(config_content, encoding="utf-8")

    messages = sync_agent_files_from_config(temp_dir)
    assert messages == []


def test_sync_agent_files_creates_nested_directory(temp_dir):
    """Test sync creates parent directories for nested agent files."""
    # Create .logmind/config.yml with copilot enabled
    logmind_dir = temp_dir / ".logmind"
    logmind_dir.mkdir()
    config_content = """
agents:
  claude: false
  cursor: false
  copilot: true
"""
    (logmind_dir / "config.yml").write_text(config_content, encoding="utf-8")

    # Run sync
    messages = sync_agent_files_from_config(temp_dir)

    # Should create .github/copilot-instructions.md (AGENTS.md auto-ensured too)
    assert (temp_dir / ".github" / "copilot-instructions.md").exists()
    assert any("copilot-instructions" in m for m in messages)
