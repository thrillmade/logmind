"""AI instruction file insertion and management logic."""

from pathlib import Path
from typing import Dict, List, Optional, Tuple


LOGMIND_START_MARKER = "<!-- logmind-start -->"
LOGMIND_END_MARKER = "<!-- logmind-end -->"


# Registry of all supported AI agents
# Format: agent_name -> (file_path_pattern, display_name, is_json)
AGENT_REGISTRY: Dict[str, Tuple[str, str, bool]] = {
    "claude": ("CLAUDE.md", "Claude Code", False),
    "cursor": (".cursorrules", "Cursor", False),
    "copilot": (".github/copilot-instructions.md", "GitHub Copilot", False),
    "windsurf": (".windsurfrules", "Windsurf", False),
    "aider": ("CONVENTIONS.md", "Aider", False),
    "continue": (".continuerules", "Continue", False),
    "cody": (".sourcegraph/cody.json", "Sourcegraph Cody", True),
    "zed": (".zed/settings.json", "Zed AI", True),
    "amazonq": (".amazonq/rules.md", "Amazon Q", False),
    "cline": (".clinerules", "Cline", False),
    "codex": ("AGENTS.md", "OpenAI Codex", False),
}


def get_all_agent_names() -> List[str]:
    """
    Get list of all supported agent names.

    Returns:
        List of agent names
    """
    return list(AGENT_REGISTRY.keys())


def get_agent_file_path(agent_name: str, root_path: Optional[Path] = None) -> Optional[Path]:
    """
    Get the file path for an agent.

    Args:
        agent_name: Name of the agent (e.g., 'claude', 'cursor')
        root_path: Project root. Defaults to current directory.

    Returns:
        Path to agent file or None if agent not found
    """
    if root_path is None:
        root_path = Path.cwd()

    if agent_name not in AGENT_REGISTRY:
        return None

    file_pattern, _, _ = AGENT_REGISTRY[agent_name]
    return root_path / file_pattern


def get_agent_display_name(agent_name: str) -> str:
    """
    Get the display name for an agent.

    Args:
        agent_name: Name of the agent

    Returns:
        Display name or the agent name if not found
    """
    if agent_name in AGENT_REGISTRY:
        _, display_name, _ = AGENT_REGISTRY[agent_name]
        return display_name
    return agent_name


def is_agent_json(agent_name: str) -> bool:
    """
    Check if an agent uses JSON format.

    Args:
        agent_name: Name of the agent

    Returns:
        True if agent uses JSON, False for markdown
    """
    if agent_name in AGENT_REGISTRY:
        _, _, is_json = AGENT_REGISTRY[agent_name]
        return is_json
    return False


def find_ai_instruction_files(root_path: Optional[Path] = None) -> List[Tuple[Path, str]]:
    """
    Find AI instruction files in the project.

    Args:
        root_path: Project root. Defaults to current directory.

    Returns:
        List of (file_path, agent_name) tuples
    """
    if root_path is None:
        root_path = Path.cwd()

    files = []

    for agent_name, (file_pattern, _, _) in AGENT_REGISTRY.items():
        file_path = root_path / file_pattern
        if file_path.exists():
            files.append((file_path, agent_name))

    return files


def get_agent_status(root_path: Optional[Path] = None) -> Dict[str, Dict]:
    """
    Get status of all agents.

    Args:
        root_path: Project root. Defaults to current directory.

    Returns:
        Dict mapping agent name to status info
    """
    if root_path is None:
        root_path = Path.cwd()

    status = {}

    for agent_name, (file_pattern, display_name, is_json) in AGENT_REGISTRY.items():
        file_path = root_path / file_pattern
        exists = file_path.exists()
        has_logmind = False

        if exists and not is_json:
            try:
                content = file_path.read_text()
                has_logmind = has_logmind_section(content)
            except Exception:
                pass

        status[agent_name] = {
            "file": file_pattern,
            "display_name": display_name,
            "exists": exists,
            "configured": exists and (has_logmind or is_json),
            "is_json": is_json,
        }

    return status


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


def get_agent_template(agent_name: str) -> str:
    """
    Get template content for creating a new agent file.

    Args:
        agent_name: Name of the agent

    Returns:
        Template content for the agent
    """
    if agent_name == "claude":
        return get_full_claude_template()

    # For other markdown-based agents, use a simpler template
    logmind_section = get_logmind_section()

    if agent_name == "cursor":
        return f"# Cursor Rules\n\n{logmind_section}\n\n## Project Rules\n\n[Add your Cursor rules here]\n"
    elif agent_name == "copilot":
        return f"# GitHub Copilot Instructions\n\n{logmind_section}\n\n## Project Instructions\n\n[Add your Copilot instructions here]\n"
    elif agent_name == "windsurf":
        return f"# Windsurf Rules\n\n{logmind_section}\n\n## Project Rules\n\n[Add your Windsurf rules here]\n"
    elif agent_name == "aider":
        return f"# Project Conventions\n\n{logmind_section}\n\n## Coding Conventions\n\n[Add your coding conventions here]\n"
    elif agent_name == "continue":
        return f"# Continue Rules\n\n{logmind_section}\n\n## Project Rules\n\n[Add your Continue rules here]\n"
    elif agent_name == "amazonq":
        return f"# Amazon Q Rules\n\n{logmind_section}\n\n## Project Rules\n\n[Add your Amazon Q rules here]\n"
    elif agent_name == "cline":
        return f"# Cline Rules\n\n{logmind_section}\n\n## Project Rules\n\n[Add your Cline rules here]\n"
    elif agent_name == "codex":
        return f"# AGENTS.md\n\nThis file provides instructions for AI coding agents.\n\n{logmind_section}\n\n## Project Guidelines\n\n[Add your project guidelines here]\n"
    elif agent_name == "cody":
        # JSON format for Cody
        return """{
  "logmind": {
    "enabled": true,
    "description": "This project uses logmind for decision tracking. See docs/decisions.md for recent decisions.",
    "context_files": [
      "docs/decisions.md",
      "docs/file-structure.md"
    ]
  }
}
"""
    elif agent_name == "zed":
        # JSON format for Zed
        return """{
  "logmind": {
    "enabled": true,
    "description": "This project uses logmind for decision tracking. See docs/decisions.md for recent decisions.",
    "context_files": [
      "docs/decisions.md",
      "docs/file-structure.md"
    ]
  }
}
"""
    else:
        # Default template
        return f"# AI Instructions\n\n{logmind_section}\n"


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


def create_agent_file(agent_name: str, root_path: Optional[Path] = None) -> Optional[Path]:
    """
    Create a new agent instruction file with logmind section.

    Args:
        agent_name: Name of the agent to create
        root_path: Project root. Defaults to current directory.

    Returns:
        Path to created file or None if agent not supported
    """
    if root_path is None:
        root_path = Path.cwd()

    if agent_name not in AGENT_REGISTRY:
        return None

    file_path = get_agent_file_path(agent_name, root_path)
    if file_path is None:
        return None

    # Create parent directories if needed
    file_path.parent.mkdir(parents=True, exist_ok=True)

    # Get template and write
    template = get_agent_template(agent_name)
    file_path.write_text(template)

    return file_path


def remove_agent_file(agent_name: str, root_path: Optional[Path] = None) -> bool:
    """
    Remove an agent instruction file.

    Args:
        agent_name: Name of the agent to remove
        root_path: Project root. Defaults to current directory.

    Returns:
        True if file was removed, False otherwise
    """
    if root_path is None:
        root_path = Path.cwd()

    file_path = get_agent_file_path(agent_name, root_path)
    if file_path is None or not file_path.exists():
        return False

    file_path.unlink()
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


def insert_into_all_ai_files(
    root_path: Optional[Path] = None,
    create_if_missing: bool = True,
    agents: Optional[List[str]] = None,
) -> List[str]:
    """
    Insert logmind section into all AI instruction files.

    Args:
        root_path: Project root. Defaults to current directory.
        create_if_missing: Create CLAUDE.md if no AI files exist. Defaults to True.
        agents: List of specific agents to process. None means auto-detect existing.

    Returns:
        List of status messages
    """
    if root_path is None:
        root_path = Path.cwd()

    messages = []

    if agents is not None:
        # Create/update specific agents
        for agent_name in agents:
            if agent_name not in AGENT_REGISTRY:
                messages.append(f"✗ Unknown agent: {agent_name}")
                continue

            file_path = get_agent_file_path(agent_name, root_path)
            if file_path is None:
                continue

            if file_path.exists():
                # Insert into existing file (skip JSON files)
                if not is_agent_json(agent_name):
                    inserted = insert_logmind_section(file_path)
                    if inserted:
                        messages.append(f"✓ Added logmind instructions to {file_path.name}")
                    else:
                        messages.append(f"✓ {file_path.name} already has logmind instructions")
                else:
                    messages.append(f"✓ {file_path.name} exists (JSON format)")
            else:
                # Create new file
                created = create_agent_file(agent_name, root_path)
                if created:
                    messages.append(f"✓ Created {created.name} with logmind instructions")
    else:
        # Auto-detect existing files
        ai_files = find_ai_instruction_files(root_path)

        if not ai_files and create_if_missing:
            # Create new CLAUDE.md
            claude_path = create_claude_md(root_path / "CLAUDE.md")
            messages.append(f"✓ Created {claude_path.name} with logmind instructions")
        else:
            # Insert into existing files
            for file_path, agent_name in ai_files:
                if is_agent_json(agent_name):
                    messages.append(f"✓ {file_path.name} exists (JSON format)")
                    continue

                inserted = insert_logmind_section(file_path)
                if inserted:
                    messages.append(f"✓ Added logmind instructions to {file_path.name}")
                else:
                    messages.append(f"✓ {file_path.name} already has logmind instructions")

    return messages
