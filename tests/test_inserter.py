"""Tests for core/inserter.py."""

from pathlib import Path

from logmind.core.inserter import (
    create_claude_md,
    find_ai_instruction_files,
    get_full_claude_template,
    get_logmind_section,
    has_logmind_section,
    insert_into_all_ai_files,
    insert_logmind_section,
)


def test_find_ai_instruction_files_empty(temp_dir):
    """Test finding AI files when none exist."""
    files = find_ai_instruction_files(temp_dir)
    assert len(files) == 0


def test_find_ai_instruction_files_claude(temp_dir):
    """Test finding CLAUDE.md."""
    (temp_dir / "CLAUDE.md").write_text("# CLAUDE.md\n")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 1
    assert files[0][0] == temp_dir / "CLAUDE.md"
    assert files[0][1] == "claude"


def test_find_ai_instruction_files_cursor(temp_dir):
    """Test finding .cursorrules."""
    (temp_dir / ".cursorrules").write_text("rules\n")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 1
    assert files[0][0] == temp_dir / ".cursorrules"
    assert files[0][1] == "cursor"


def test_find_ai_instruction_files_copilot(temp_dir):
    """Test finding copilot instructions."""
    github_dir = temp_dir / ".github"
    github_dir.mkdir()
    (github_dir / "copilot-instructions.md").write_text("# Instructions\n")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 1
    assert files[0][0] == github_dir / "copilot-instructions.md"
    assert files[0][1] == "copilot"


def test_find_ai_instruction_files_multiple(temp_dir):
    """Test finding multiple AI instruction files."""
    (temp_dir / "CLAUDE.md").write_text("# CLAUDE.md\n")
    (temp_dir / ".cursorrules").write_text("rules\n")

    files = find_ai_instruction_files(temp_dir)

    assert len(files) == 2


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


def test_insert_logmind_section_new_file(temp_dir):
    """Test inserting logmind section into new file."""
    claude_file = temp_dir / "CLAUDE.md"
    claude_file.write_text("# CLAUDE.md\n\nExisting content\n")

    result = insert_logmind_section(claude_file)

    assert result is True
    content = claude_file.read_text()
    assert "<!-- logmind-start -->" in content
    assert "Existing content" in content  # Should preserve existing


def test_insert_logmind_section_already_exists(temp_dir):
    """Test that inserting again returns False."""
    claude_file = temp_dir / "CLAUDE.md"
    claude_file.write_text("# CLAUDE.md\n\n<!-- logmind-start -->\nAlready here\n<!-- logmind-end -->\n")

    result = insert_logmind_section(claude_file)

    assert result is False  # Should not insert again


def test_insert_logmind_section_after_heading(temp_dir):
    """Test that logmind section is inserted after first heading."""
    claude_file = temp_dir / "CLAUDE.md"
    claude_file.write_text("# CLAUDE.md\n\nThis is guidance.\n\n## Section 2\n\nMore content\n")

    insert_logmind_section(claude_file)

    content = claude_file.read_text()
    lines = content.split("\n")

    # Find where logmind section starts
    logmind_idx = next(i for i, line in enumerate(lines) if "<!-- logmind-start -->" in line)

    # Should be after "# CLAUDE.md" but before "## Section 2"
    heading_idx = next(i for i, line in enumerate(lines) if line.startswith("# CLAUDE.md"))
    section2_idx = next(i for i, line in enumerate(lines) if "## Section 2" in line)

    assert heading_idx < logmind_idx < section2_idx


def test_create_claude_md(temp_dir):
    """Test creating new CLAUDE.md file."""
    claude_file = temp_dir / "CLAUDE.md"

    result = create_claude_md(claude_file)

    assert result == claude_file
    assert claude_file.exists()

    content = claude_file.read_text()
    assert "# CLAUDE.md" in content
    assert "<!-- logmind-start -->" in content


def test_insert_into_all_ai_files_creates_claude(temp_dir):
    """Test that insert_into_all creates CLAUDE.md if no files exist."""
    messages = insert_into_all_ai_files(temp_dir)

    assert len(messages) == 1
    assert "Created CLAUDE.md" in messages[0]
    assert (temp_dir / "CLAUDE.md").exists()


def test_insert_into_all_ai_files_existing_file(temp_dir):
    """Test inserting into existing file."""
    (temp_dir / "CLAUDE.md").write_text("# CLAUDE.md\n\nContent\n")

    messages = insert_into_all_ai_files(temp_dir)

    assert len(messages) == 1
    assert "Added logmind instructions" in messages[0]

    content = (temp_dir / "CLAUDE.md").read_text()
    assert "<!-- logmind-start -->" in content
    assert "Content" in content  # Preserved


def test_insert_into_all_ai_files_already_initialized(temp_dir):
    """Test behavior when already initialized."""
    (temp_dir / "CLAUDE.md").write_text(
        "# CLAUDE.md\n\n<!-- logmind-start -->\nAlready done\n<!-- logmind-end -->\n"
    )

    messages = insert_into_all_ai_files(temp_dir)

    assert len(messages) == 1
    assert "already has logmind instructions" in messages[0]


def test_insert_into_all_ai_files_no_create(temp_dir):
    """Test with create_if_missing=False."""
    messages = insert_into_all_ai_files(temp_dir, create_if_missing=False)

    assert len(messages) == 0
    assert not (temp_dir / "CLAUDE.md").exists()
