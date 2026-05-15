"""AI instruction file insertion and management logic."""

from pathlib import Path
from typing import Dict, List, Optional, Tuple


LOGMIND_START_MARKER = "<!-- logmind-start -->"
LOGMIND_END_MARKER = "<!-- logmind-end -->"
LOGMIND_STUB_MARKER = "<!-- logmind-stub:"


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
                # A stub is fully "configured" because it points at AGENTS.md.
                has_logmind = has_logmind_section(content) or is_stub(content)
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


def is_stub(content: str) -> bool:
    """Return True if content is a logmind AGENTS.md-pointer stub."""
    return LOGMIND_STUB_MARKER in content


def get_agents_md_template() -> str:
    """Return the canonical AGENTS.md template content."""
    template_path = Path(__file__).parent.parent / "templates" / "AGENTS.md.template"
    return template_path.read_text()


def get_stub_template() -> str:
    """Return the per-agent AGENTS.md-pointer stub content."""
    template_path = Path(__file__).parent.parent / "templates" / "agent-stub.md"
    return template_path.read_text()


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

    Behaviour (Phase 8 consolidation):
      - codex (AGENTS.md): the canonical full template — AGENTS.md is the
        single source of truth.
      - All other markdown-based agents: a 2-line stub pointing at AGENTS.md.
      - JSON agents (cody, zed): unchanged JSON content.

    Args:
        agent_name: Name of the agent

    Returns:
        Template content for the agent
    """
    if agent_name == "codex":
        return get_agents_md_template()

    if agent_name in ("claude", "cursor", "copilot", "windsurf", "aider",
                      "continue", "amazonq", "cline"):
        return get_stub_template()

    if agent_name == "cody":
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


def ensure_agents_md(root_path: Optional[Path] = None) -> Optional[str]:
    """
    Make sure ``AGENTS.md`` exists at the project root with the canonical
    logmind content.

    - Missing → write the canonical template.
    - Exists without logmind markers → insert the logmind block in-place.
    - Exists with logmind markers → no-op.

    Returns a status string when a write happened, or None for no-op.
    """
    if root_path is None:
        root_path = Path.cwd()

    agents_path = root_path / "AGENTS.md"

    if not agents_path.exists():
        agents_path.write_text(get_agents_md_template())
        return "Created AGENTS.md (canonical agent instructions)"

    content = agents_path.read_text()
    if has_logmind_section(content):
        return None
    insert_logmind_section(agents_path)
    return "Added logmind section to existing AGENTS.md"


def insert_into_all_ai_files(
    root_path: Optional[Path] = None,
    create_if_missing: bool = True,
    agents: Optional[List[str]] = None,
) -> List[str]:
    """
    Set up AGENTS.md as canonical and per-agent files as stubs pointing to it.

    For each requested agent:
      - JSON agents (cody, zed) get their JSON template (unchanged behavior).
      - Codex IS AGENTS.md and is handled by ``ensure_agents_md``.
      - Other markdown agents get a 2-line stub if their file is missing.
      - If the file exists with user content but no logmind block, the legacy
        in-place insertion runs so user content is preserved (run
        ``logmind agents migrate`` to consolidate that content into AGENTS.md
        and replace the file with a stub).
    """
    if root_path is None:
        root_path = Path.cwd()

    messages: List[str] = []

    # Ensure AGENTS.md is canonical first; everything else points at it.
    # Skip when the caller explicitly opted out of creating files AND no
    # agents were specified (legacy "look only" mode).
    if agents is not None or create_if_missing:
        canonical_msg = ensure_agents_md(root_path)
        if canonical_msg:
            messages.append(f"✓ {canonical_msg}")

    if agents is not None:
        for agent_name in agents:
            if agent_name not in AGENT_REGISTRY:
                messages.append(f"✗ Unknown agent: {agent_name}")
                continue

            if agent_name == "codex":
                # AGENTS.md handled above
                continue

            file_path = get_agent_file_path(agent_name, root_path)
            if file_path is None:
                continue

            if file_path.exists():
                if is_agent_json(agent_name):
                    messages.append(f"✓ {file_path.name} exists (JSON format)")
                    continue
                content = file_path.read_text()
                if is_stub(content) or has_logmind_section(content):
                    messages.append(
                        f"✓ {file_path.name} already configured"
                    )
                    continue
                # Existing user content — preserve via legacy in-place insertion.
                inserted = insert_logmind_section(file_path)
                if inserted:
                    messages.append(
                        f"✓ Added logmind instructions to {file_path.name} "
                        f"(consider `logmind agents migrate` to convert to a stub)"
                    )
            else:
                created = create_agent_file(agent_name, root_path)
                if created:
                    if is_agent_json(agent_name):
                        messages.append(f"✓ Created {created.name} (JSON format)")
                    else:
                        messages.append(f"✓ Created {created.name} (stub → AGENTS.md)")
    else:
        # Auto-detect existing files (legacy path used by some tests/integrations)
        ai_files = find_ai_instruction_files(root_path)

        for file_path, agent_name in ai_files:
            if is_agent_json(agent_name):
                messages.append(f"✓ {file_path.name} exists (JSON format)")
                continue
            if file_path.name == "AGENTS.md":
                continue  # handled by ensure_agents_md
            content = file_path.read_text()
            if is_stub(content) or has_logmind_section(content):
                messages.append(f"✓ {file_path.name} already configured")
                continue
            inserted = insert_logmind_section(file_path)
            if inserted:
                messages.append(
                    f"✓ Added logmind instructions to {file_path.name} "
                    f"(consider `logmind agents migrate`)"
                )

    return messages


def _strip_logmind_block(content: str) -> str:
    """Remove the marker-bracketed logmind block from a file's content."""
    start = content.find(LOGMIND_START_MARKER)
    end = content.find(LOGMIND_END_MARKER)
    if start == -1 or end == -1 or end < start:
        return content
    end_full = end + len(LOGMIND_END_MARKER)
    # Trim a trailing newline that often follows the end marker
    if end_full < len(content) and content[end_full] == "\n":
        end_full += 1
    return content[:start] + content[end_full:]


def migrate_to_agents_md(root_path: Optional[Path] = None) -> List[str]:
    """
    Consolidate per-agent instruction files into AGENTS.md and replace each
    one with a 2-line stub.

    For each existing markdown agent file (CLAUDE.md, .cursorrules, etc.):
      - Strip the logmind marker block.
      - If any non-marker content remains, append it under a "## From <name>"
        heading at the bottom of AGENTS.md.
      - Replace the file content with the canonical stub.

    JSON agents (cody, zed) and AGENTS.md itself are skipped. The function is
    idempotent: re-running on an already-stubbed tree is a no-op.
    """
    if root_path is None:
        root_path = Path.cwd()

    messages: List[str] = []
    ensure_agents_md(root_path)

    agents_path = root_path / "AGENTS.md"
    appended_blocks: List[str] = []

    for agent_name, (file_pattern, display_name, json_) in AGENT_REGISTRY.items():
        if agent_name == "codex" or json_:
            continue
        file_path = root_path / file_pattern
        if not file_path.exists():
            continue
        content = file_path.read_text()
        if is_stub(content):
            continue  # already migrated

        remaining = _strip_logmind_block(content).strip()
        if remaining:
            appended_blocks.append(f"## From {display_name}\n\n{remaining}\n")
            messages.append(
                f"✓ Migrated {display_name} ({file_path.name}) content into AGENTS.md"
            )

        file_path.write_text(get_stub_template())
        messages.append(f"✓ {file_path.name} replaced with stub")

    if appended_blocks:
        existing = agents_path.read_text().rstrip()
        agents_path.write_text(existing + "\n\n" + "\n".join(appended_blocks))

    return messages


def sync_agent_files_from_config(root_path: Optional[Path] = None) -> List[str]:
    """
    Sync agent files based on configuration.

    For each enabled agent in config:
    - If file doesn't exist → CREATE it with logmind section
    - If file exists but lacks logmind section → INSERT section
    - If file exists with logmind section → skip (already done)

    This runs silently by default - only returns messages for actions taken.

    Args:
        root_path: Project root. Defaults to current directory.

    Returns:
        List of action messages (only for files created or updated)
    """
    if root_path is None:
        root_path = Path.cwd()

    # Import here to avoid circular dependency
    from logmind.core.config import load_config

    # Check if logmind is initialized (config exists)
    config_path = root_path / ".logmind" / "config.yml"
    if not config_path.exists():
        return []

    config = load_config(config_path)
    enabled_agents = config.get_enabled_agents()

    if not enabled_agents:
        return []

    messages = []

    for agent_name in enabled_agents:
        if agent_name not in AGENT_REGISTRY:
            continue

        file_path = get_agent_file_path(agent_name, root_path)
        if file_path is None:
            continue

        if file_path.exists():
            # File exists - check if it needs logmind section
            if not is_agent_json(agent_name):
                try:
                    content = file_path.read_text()
                    if not has_logmind_section(content):
                        # Insert logmind section
                        inserted = insert_logmind_section(file_path)
                        if inserted:
                            messages.append(f"✓ Added logmind section to {file_path.name}")
                except Exception:
                    pass
            # JSON files and already-configured files: skip silently
        else:
            # File doesn't exist - create it
            created = create_agent_file(agent_name, root_path)
            if created:
                messages.append(f"✓ Created {created.name} with logmind section")

    return messages
