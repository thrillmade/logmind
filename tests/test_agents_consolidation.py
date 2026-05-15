"""Tests for AGENTS.md consolidation (Phase 8 — agents migrate, stub model)."""

from __future__ import annotations

from pathlib import Path

import pytest

from logmind.core.inserter import (
    LOGMIND_END_MARKER,
    LOGMIND_START_MARKER,
    LOGMIND_STUB_MARKER,
    create_agent_file,
    ensure_agents_md,
    get_agents_md_template,
    get_stub_template,
    has_logmind_section,
    insert_into_all_ai_files,
    is_stub,
    migrate_to_agents_md,
)


# ---------------------------------------------------------------------------
# Templates / detection
# ---------------------------------------------------------------------------


def test_stub_template_contains_marker_and_pointer():
    stub = get_stub_template()
    assert LOGMIND_STUB_MARKER in stub
    assert "AGENTS.md" in stub


def test_agents_md_template_contains_logmind_block():
    tpl = get_agents_md_template()
    assert LOGMIND_START_MARKER in tpl
    assert LOGMIND_END_MARKER in tpl
    assert "AGENTS.md" in tpl


def test_is_stub_detects_marker():
    assert is_stub(get_stub_template()) is True
    assert is_stub("# CLAUDE.md\n\nNot a stub\n") is False


# ---------------------------------------------------------------------------
# ensure_agents_md
# ---------------------------------------------------------------------------


def test_ensure_agents_md_creates_when_missing(tmp_path):
    msg = ensure_agents_md(tmp_path)
    assert msg is not None and "Created" in msg
    assert (tmp_path / "AGENTS.md").exists()
    assert has_logmind_section((tmp_path / "AGENTS.md").read_text())


def test_ensure_agents_md_inserts_block_when_present_without_marker(tmp_path):
    (tmp_path / "AGENTS.md").write_text("# AGENTS.md\n\nUser content here.\n")
    msg = ensure_agents_md(tmp_path)
    assert msg is not None and "Added" in msg
    content = (tmp_path / "AGENTS.md").read_text()
    assert "User content here" in content
    assert has_logmind_section(content)


def test_ensure_agents_md_noop_when_already_canonical(tmp_path):
    (tmp_path / "AGENTS.md").write_text(get_agents_md_template())
    assert ensure_agents_md(tmp_path) is None


# ---------------------------------------------------------------------------
# create_agent_file (new behaviour)
# ---------------------------------------------------------------------------


def test_create_agent_file_for_codex_writes_canonical_template(tmp_path):
    p = create_agent_file("codex", tmp_path)
    assert p == tmp_path / "AGENTS.md"
    assert has_logmind_section(p.read_text())
    # Not a stub — it's the canonical doc
    assert not is_stub(p.read_text())


@pytest.mark.parametrize(
    "agent",
    ["claude", "cursor", "windsurf", "aider", "continue", "amazonq", "cline", "copilot"],
)
def test_create_agent_file_for_markdown_agents_writes_stub(tmp_path, agent):
    p = create_agent_file(agent, tmp_path)
    assert p is not None
    content = p.read_text()
    assert is_stub(content)
    assert "AGENTS.md" in content


@pytest.mark.parametrize("agent", ["cody", "zed"])
def test_create_agent_file_for_json_agents_keeps_json(tmp_path, agent):
    p = create_agent_file(agent, tmp_path)
    assert p is not None
    content = p.read_text()
    # JSON files don't contain the stub marker
    assert not is_stub(content)
    assert content.strip().startswith("{")


# ---------------------------------------------------------------------------
# insert_into_all_ai_files
# ---------------------------------------------------------------------------


def test_insert_into_all_ai_files_creates_agents_md_and_stubs(tmp_path):
    msgs = insert_into_all_ai_files(tmp_path, agents=["claude", "cursor"])

    assert (tmp_path / "AGENTS.md").exists()
    assert has_logmind_section((tmp_path / "AGENTS.md").read_text())

    assert (tmp_path / "CLAUDE.md").exists()
    assert is_stub((tmp_path / "CLAUDE.md").read_text())

    assert (tmp_path / ".cursorrules").exists()
    assert is_stub((tmp_path / ".cursorrules").read_text())

    # Status messages mention canonical + stubs
    joined = "\n".join(msgs)
    assert "AGENTS.md" in joined
    assert "stub" in joined.lower()


def test_insert_into_all_ai_files_preserves_existing_user_content(tmp_path):
    """An existing CLAUDE.md with user content gets a logmind block inserted,
    not blindly overwritten with a stub."""
    (tmp_path / "CLAUDE.md").write_text(
        "# CLAUDE.md\n\nProject-specific guidance.\n"
    )
    msgs = insert_into_all_ai_files(tmp_path, agents=["claude"])

    content = (tmp_path / "CLAUDE.md").read_text()
    assert "Project-specific guidance" in content
    assert has_logmind_section(content)
    assert not is_stub(content)

    joined = "\n".join(msgs)
    assert "agents migrate" in joined  # user is nudged to consolidate


def test_insert_into_all_ai_files_idempotent_for_existing_stubs(tmp_path):
    insert_into_all_ai_files(tmp_path, agents=["claude"])
    msgs = insert_into_all_ai_files(tmp_path, agents=["claude"])
    joined = "\n".join(msgs)
    assert "already configured" in joined
    # Still a stub — not double-written
    content = (tmp_path / "CLAUDE.md").read_text()
    assert is_stub(content)


def test_insert_into_all_ai_files_codex_is_canonical_not_stub(tmp_path):
    insert_into_all_ai_files(tmp_path, agents=["codex"])
    content = (tmp_path / "AGENTS.md").read_text()
    assert has_logmind_section(content)
    assert not is_stub(content)


# ---------------------------------------------------------------------------
# migrate_to_agents_md
# ---------------------------------------------------------------------------


def test_migrate_consolidates_user_content_and_stubs_files(tmp_path):
    # Pretend we have legacy CLAUDE.md and .cursorrules with user content
    (tmp_path / "CLAUDE.md").write_text(
        "# CLAUDE.md\n\nMy Claude-specific guidance.\n\n"
        f"{LOGMIND_START_MARKER}\n## logmind\nold content\n{LOGMIND_END_MARKER}\n"
    )
    (tmp_path / ".cursorrules").write_text(
        "# Cursor Rules\n\nMy Cursor-specific rules.\n\n"
        f"{LOGMIND_START_MARKER}\nold logmind block\n{LOGMIND_END_MARKER}\n"
    )

    msgs = migrate_to_agents_md(tmp_path)

    # AGENTS.md created and contains migrated content
    agents_content = (tmp_path / "AGENTS.md").read_text()
    assert "My Claude-specific guidance" in agents_content
    assert "My Cursor-specific rules" in agents_content
    assert has_logmind_section(agents_content)

    # Per-agent files now stubs
    assert is_stub((tmp_path / "CLAUDE.md").read_text())
    assert is_stub((tmp_path / ".cursorrules").read_text())

    # Status mentions migration
    joined = "\n".join(msgs)
    assert "Migrated" in joined
    assert "stub" in joined.lower()


def test_migrate_idempotent_on_already_stubbed_tree(tmp_path):
    insert_into_all_ai_files(tmp_path, agents=["claude", "cursor"])
    # First migration is a no-op (everything already a stub from init flow)
    msgs = migrate_to_agents_md(tmp_path)
    assert msgs == []


def test_migrate_skips_json_agents(tmp_path):
    # Set up a JSON agent file with content
    (tmp_path / ".sourcegraph").mkdir()
    (tmp_path / ".sourcegraph" / "cody.json").write_text('{"existing": true}')

    migrate_to_agents_md(tmp_path)

    # JSON file untouched
    assert (tmp_path / ".sourcegraph" / "cody.json").read_text() == '{"existing": true}'
