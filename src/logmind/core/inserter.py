"""CLAUDE.md and AI instruction file insertion logic."""

from pathlib import Path
from typing import List, Tuple, Optional


LOGMIND_START_MARKER = "<!-- logmind-start -->"
LOGMIND_END_MARKER = "<!-- logmind-end -->"


def find_ai_instruction_files(root_path: Optional[Path] = None) -> List[Tuple[Path, str]]:
    """
    Find AI instruction files in the project.

    Args:
        root_path: Project root. Defaults to current directory.

    Returns:
        List of (file_path, file_type) tuples
    """
    if root_path is None:
        root_path = Path.cwd()

    files = []

    # Check for CLAUDE.md
    claude_path = root_path / "CLAUDE.md"
    if claude_path.exists():
        files.append((claude_path, "claude"))

    # Check for .cursorrules
    cursor_path = root_path / ".cursorrules"
    if cursor_path.exists():
        files.append((cursor_path, "cursor"))

    # Check for GitHub Copilot instructions
    copilot_path = root_path / ".github" / "copilot-instructions.md"
    if copilot_path.exists():
        files.append((copilot_path, "copilot"))

    return files


def has_logmind_section(content: str) -> bool:
    """
    Check if content already has logmind section.

    Args:
        content: File content to check

    Returns:
        True if logmind section exists, False otherwise
    """
    return LOGMIND_START_MARKER in content


def get_logmind_section() -> str:
    """
    Get the logmind section to insert.

    Returns:
        Logmind section content
    """
    template_path = Path(__file__).parent.parent / "templates" / "logmind-section.md"
    return template_path.read_text()


def get_full_claude_template() -> str:
    """
    Get the full CLAUDE.md template for new files.

    Returns:
        Full CLAUDE.md template content
    """
    template_path = Path(__file__).parent.parent / "templates" / "CLAUDE.md.template"
    return template_path.read_text()


def insert_logmind_section(file_path: Path) -> bool:
    """
    Insert logmind section into an existing AI instruction file.

    Args:
        file_path: Path to AI instruction file

    Returns:
        True if insertion was performed, False if already present
    """
    content = file_path.read_text()

    # Check if already initialized
    if has_logmind_section(content):
        return False

    # Find insertion point (after first heading)
    lines = content.split("\n")
    insert_index = 0

    for i, line in enumerate(lines):
        if line.startswith("# "):
            insert_index = i + 1
            # Skip any blank lines after title
            while insert_index < len(lines) and not lines[insert_index].strip():
                insert_index += 1
            break

    # If no heading found, insert at top
    if insert_index == 0 and lines:
        insert_index = 0

    # Get logmind section
    logmind_section = get_logmind_section()

    # Insert the section
    lines.insert(insert_index, logmind_section)

    # Write back
    file_path.write_text("\n".join(lines))
    return True


def create_claude_md(file_path: Optional[Path] = None) -> Path:
    """
    Create a new CLAUDE.md file with logmind section.

    Args:
        file_path: Path for new CLAUDE.md. Defaults to ./CLAUDE.md

    Returns:
        Path to created file
    """
    if file_path is None:
        file_path = Path.cwd() / "CLAUDE.md"

    template = get_full_claude_template()
    file_path.write_text(template)

    return file_path


def insert_into_all_ai_files(root_path: Optional[Path] = None, create_if_missing: bool = True) -> List[str]:
    """
    Insert logmind section into all AI instruction files.

    Args:
        root_path: Project root. Defaults to current directory.
        create_if_missing: Create CLAUDE.md if no AI files exist. Defaults to True.

    Returns:
        List of status messages
    """
    if root_path is None:
        root_path = Path.cwd()

    messages = []
    ai_files = find_ai_instruction_files(root_path)

    if not ai_files and create_if_missing:
        # Create new CLAUDE.md
        claude_path = create_claude_md(root_path / "CLAUDE.md")
        messages.append(f"✓ Created {claude_path.name} with logmind instructions")
    else:
        # Insert into existing files
        for file_path, file_type in ai_files:
            inserted = insert_logmind_section(file_path)
            if inserted:
                messages.append(f"✓ Added logmind instructions to {file_path.name}")
            else:
                messages.append(f"✓ {file_path.name} already has logmind instructions")

    return messages
