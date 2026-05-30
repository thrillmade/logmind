"""AI instruction file insertion and management logic."""

import re
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
                content = file_path.read_text(encoding="utf-8")
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


def get_agents_md_template(prefer_slim: Optional[bool] = None) -> str:
    """Return the canonical AGENTS.md template content.

    Adaptive: if the user has skills.sh available (skills CLI or npx on PATH),
    we ship the slim variant that defers to the `logmind` skill for the
    canonical procedure. Otherwise we ship the full variant with the
    procedure inline as a fallback.

    Pass prefer_slim=True/False to force one variant; the default detects.
    """
    if prefer_slim is None:
        try:
            from logmind.core.skill_install import is_skills_available
            prefer_slim = is_skills_available()
        except Exception:
            prefer_slim = False

    name = "AGENTS.md.slim.template" if prefer_slim else "AGENTS.md.template"
    template_path = Path(__file__).parent.parent / "templates" / name
    return template_path.read_text(encoding="utf-8")


def get_stub_template() -> str:
    """Return the per-agent AGENTS.md-pointer stub content."""
    template_path = Path(__file__).parent.parent / "templates" / "agent-stub.md"
    return template_path.read_text(encoding="utf-8")


# v0.5.13 / recurring gotcha #1: pin auto-update for CI workflows.
# When `clud-bug update` re-renders workflows in consumer repos, the
# logmind==X.Y.Z pin inside our CI workflows goes stale (clud-bug
# doesn't know about logmind releases). Pre-v0.5.13, this required
# manual `sed -i` after every clud-bug update cycle. Now `logmind
# agents update --apply` sweeps the pin alongside the AGENTS.md
# block — one command, two refreshes.

# Workflows whose pin should track __version__. Names match the
# canonical templates shipped under templates/github/.
LOGMIND_PIN_WORKFLOWS = (
    "regen-timeline.yml",
    "check-doc-links.yml",
    "logmind-self-update.yml",
    "check-decisions.yml",
)

# Captures the pin line: prefix up through `==` (group 1), version (group 2),
# trailing quote (group 3). Matches both `"logmind==X.Y.Z"` and bare
# `logmind==X.Y.Z` forms. Anchored to `pip install` so we don't false-positive
# on a comment that happens to mention the package name + version.
_PIN_LINE_RE = re.compile(
    r'(pip install\s+"?logmind==)([\d.]+)("?)'
)


def find_outdated_workflow_pins(
    root_path: Optional[Path] = None,
) -> List[Tuple[Path, str, str]]:
    """
    Return [(workflow_file, current_pin_version, target_version), ...] for
    every `.github/workflows/<name>.yml` whose `pip install "logmind==X.Y.Z"`
    pin doesn't match the running logmind's `__version__`.

    Only the workflows in `LOGMIND_PIN_WORKFLOWS` are checked — the canonical
    set logmind ships templates for. Custom user workflows aren't swept
    (would risk surprising users with rewrites of files they own).

    Returns empty list when:
      - `.github/workflows/` doesn't exist
      - none of the canonical workflows are present
      - all present workflows already pin the current `__version__`
    """
    if root_path is None:
        root_path = Path.cwd()

    from logmind import __version__ as current_version

    outdated: List[Tuple[Path, str, str]] = []
    workflows_dir = root_path / ".github" / "workflows"
    if not workflows_dir.exists():
        return outdated

    for name in LOGMIND_PIN_WORKFLOWS:
        wf_path = workflows_dir / name
        if not wf_path.exists():
            continue
        try:
            content = wf_path.read_text(encoding="utf-8")
        except OSError:
            continue
        m = _PIN_LINE_RE.search(content)
        if m is None:
            continue  # no pin → nothing to update (dogfood-style installs)
        found_version = m.group(2)
        if found_version != current_version:
            outdated.append((wf_path, found_version, current_version))

    return outdated


def update_workflow_pin(content: str, new_version: str) -> Tuple[str, Optional[str]]:
    """
    Rewrite every `pip install "logmind==X.Y.Z"` line in ``content`` to pin
    ``new_version`` instead.

    Returns ``(new_content, previous_version_or_None)``. ``previous_version``
    is taken from the FIRST pin found, so it's the version string in both
    the rewrite case AND the idempotent (already-current) case.
    ``previous`` is ``None`` only when no pin is present at all (dogfood-
    style workflows). The 3 return shapes:

      - ``(content, None)`` — no pin in content; nothing to do
      - ``(content, "X.Y.Z")`` — pin already matches ``new_version``; idempotent no-op
      - ``(rewritten_content, "OLD")`` — pin bumped; old version surfaced for logging

    Idempotent: re-applying the same version is a no-op (content unchanged).
    """
    m = _PIN_LINE_RE.search(content)
    if m is None:
        return content, None
    previous = m.group(2)
    if previous == new_version:
        return content, previous

    new_content = _PIN_LINE_RE.sub(
        lambda mo: f"{mo.group(1)}{new_version}{mo.group(3)}",
        content,
    )
    return new_content, previous


def find_outdated_marker_blocks(
    root_path: Optional[Path] = None,
) -> List[Tuple[Path, str, str]]:
    """
    Return [(file_path, current_block_body, fresh_block_body), ...] for every
    tracked agent file whose installed logmind marker block differs from the
    current template's block.

    Only AGENTS.md is checked — the canonical instruction file. Per-tool
    stubs don't carry a marker block and JSON agents (cody, zed) don't use
    the marker system.
    """
    if root_path is None:
        root_path = Path.cwd()

    outdated: List[Tuple[Path, str, str]] = []

    agents_path = root_path / "AGENTS.md"
    if agents_path.exists():
        content = agents_path.read_text(encoding="utf-8")
        installed = _extract_marker_block(content)
        if installed is not None:
            template = get_agents_md_template()
            fresh = _extract_marker_block(template)
            if fresh is not None and installed.strip() != fresh.strip():
                outdated.append((agents_path, installed, fresh))

    return outdated


def get_logmind_section() -> str:
    """
    Get the logmind section to insert.

    Returns:
        Logmind section content
    """
    template_path = Path(__file__).parent.parent / "templates" / "logmind-section.md"
    return template_path.read_text(encoding="utf-8")


def get_full_claude_template() -> str:
    """
    Get the full CLAUDE.md template for new files.

    Returns:
        Full CLAUDE.md template content
    """
    template_path = Path(__file__).parent.parent / "templates" / "CLAUDE.md.template"
    return template_path.read_text(encoding="utf-8")


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
    content = file_path.read_text(encoding="utf-8")

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
    file_path.write_text("\n".join(lines), encoding="utf-8")
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
    file_path.write_text(template, encoding="utf-8")

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
    file_path.write_text(template, encoding="utf-8")

    return file_path


def _extract_marker_block(content: str) -> Optional[str]:
    """Return the text between the logmind markers, or None if absent."""
    start = content.find(LOGMIND_START_MARKER)
    end = content.find(LOGMIND_END_MARKER)
    if start == -1 or end == -1 or end < start:
        return None
    block_start = start + len(LOGMIND_START_MARKER)
    return content[block_start:end]


def _replace_marker_block(content: str, new_block_body: str) -> str:
    """Swap the body between the existing logmind markers, preserving everything else."""
    start = content.find(LOGMIND_START_MARKER)
    end = content.find(LOGMIND_END_MARKER)
    if start == -1 or end == -1:
        return content
    return (
        content[: start + len(LOGMIND_START_MARKER)]
        + new_block_body
        + content[end:]
    )


def ensure_agents_md(root_path: Optional[Path] = None) -> Optional[str]:
    """
    Make sure ``AGENTS.md`` exists at the project root with the **current**
    canonical logmind block.

    - Missing → write the canonical template (adaptive: slim if skills.sh
      is available, full otherwise).
    - Exists without logmind markers → insert the logmind block in-place.
    - Exists with logmind markers but the body is out of date (different
      from the current template's marker body) → silently refresh the body
      in place. Content above and below the markers is preserved.
    - Exists with logmind markers and the body matches the current
      template → no-op.

    Returns a status string when a write happened, or None for no-op.
    """
    if root_path is None:
        root_path = Path.cwd()

    agents_path = root_path / "AGENTS.md"
    template = get_agents_md_template()

    if not agents_path.exists():
        agents_path.write_text(template, encoding="utf-8")
        return "Created AGENTS.md (canonical agent instructions)"

    content = agents_path.read_text(encoding="utf-8")

    if not has_logmind_section(content):
        insert_logmind_section(agents_path)
        return "Added logmind section to existing AGENTS.md"

    template_block = _extract_marker_block(template)
    installed_block = _extract_marker_block(content)
    if (
        template_block is not None
        and installed_block is not None
        and installed_block.strip() != template_block.strip()
    ):
        refreshed = _replace_marker_block(content, template_block)
        agents_path.write_text(refreshed, encoding="utf-8")
        return "Refreshed AGENTS.md logmind block to current template"

    return None


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
                content = file_path.read_text(encoding="utf-8")
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
            content = file_path.read_text(encoding="utf-8")
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
        content = file_path.read_text(encoding="utf-8")
        if is_stub(content):
            continue  # already migrated

        remaining = _strip_logmind_block(content).strip()
        if remaining:
            appended_blocks.append(f"## From {display_name}\n\n{remaining}\n")
            messages.append(
                f"✓ Migrated {display_name} ({file_path.name}) content into AGENTS.md"
            )

        file_path.write_text(get_stub_template(), encoding="utf-8")
        messages.append(f"✓ {file_path.name} replaced with stub")

    if appended_blocks:
        existing = agents_path.read_text(encoding="utf-8").rstrip()
        agents_path.write_text(existing + "\n\n" + "\n".join(appended_blocks), encoding="utf-8")

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

    # Auto-fix AGENTS.md if its logmind block is missing or out-of-date
    # against the current template. ensure_agents_md is the source of truth
    # for "what should this look like right now" and silently rewrites the
    # marker block when needed (preserving content above + below the markers).
    canonical_msg = ensure_agents_md(root_path)
    if canonical_msg:
        messages.append(f"✓ {canonical_msg}")

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
                    content = file_path.read_text(encoding="utf-8")
                    # Stubs and files with the logmind block are already
                    # configured. Don't trample stub files by inserting a
                    # logmind block into them — that would defeat the
                    # AGENTS.md-as-canonical model.
                    if not has_logmind_section(content) and not is_stub(content):
                        inserted = insert_logmind_section(file_path)
                        if inserted:
                            messages.append(f"✓ Added logmind section to {file_path.name}")
                except Exception:
                    pass
            # JSON files, stubs, and already-configured files: skip silently
        else:
            # File doesn't exist - create it (stub or canonical depending on agent)
            created = create_agent_file(agent_name, root_path)
            if created:
                messages.append(f"✓ Created {created.name}")

    return messages
