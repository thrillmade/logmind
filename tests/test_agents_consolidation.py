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
    sync_agent_files_from_config,
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
    assert has_logmind_section((tmp_path / "AGENTS.md").read_text(encoding="utf-8"))


def test_ensure_agents_md_inserts_block_when_present_without_marker(tmp_path):
    (tmp_path / "AGENTS.md").write_text("# AGENTS.md\n\nUser content here.\n", encoding="utf-8")
    msg = ensure_agents_md(tmp_path)
    assert msg is not None and "Added" in msg
    content = (tmp_path / "AGENTS.md").read_text(encoding="utf-8")
    assert "User content here" in content
    assert has_logmind_section(content)


def test_ensure_agents_md_noop_when_already_canonical(tmp_path):
    (tmp_path / "AGENTS.md").write_text(get_agents_md_template(), encoding="utf-8")
    assert ensure_agents_md(tmp_path) is None


def test_ensure_agents_md_auto_refreshes_stale_marker_block(tmp_path):
    """If the marker block exists but its body differs from the current
    template's, ensure_agents_md silently rewrites the body in-place,
    preserving content above and below the markers."""
    from logmind.core.inserter import LOGMIND_END_MARKER, LOGMIND_START_MARKER

    stale_body = "\n## Old version of the logmind block\nThis is outdated.\n"
    user_above = "# AGENTS.md\n\nMy project lead-in.\n\n"
    user_below = "\n## Project Overview\n\nMy custom content.\n"
    (tmp_path / "AGENTS.md").write_text(
        user_above + LOGMIND_START_MARKER + stale_body + LOGMIND_END_MARKER + user_below,
        encoding="utf-8",
    )

    msg = ensure_agents_md(tmp_path)
    assert msg is not None and "Refreshed" in msg

    new_content = (tmp_path / "AGENTS.md").read_text(encoding="utf-8")
    # User content above + below preserved
    assert "My project lead-in." in new_content
    assert "My custom content." in new_content
    # Old block content gone
    assert "Old version of the logmind block" not in new_content
    # New canonical block content present
    assert LOGMIND_START_MARKER in new_content
    assert LOGMIND_END_MARKER in new_content
    # And the second call is a no-op
    assert ensure_agents_md(tmp_path) is None


def test_get_agents_md_template_returns_slim_when_skills_available(monkeypatch):
    """When skills.sh is detected, template adapts to the slim variant.

    0.B.6 (v0.5.6): block trimmed from v5-slim (2526 bytes / 48 lines)
    to v6-pointer (~770 bytes / 12 lines, 69 % reduction) — drops the
    inline procedure (covered by the skill at the linked URL), keeps
    the load-bearing "commit primitive" rule + bash example + skill
    pointer.
    """
    from logmind.core import skill_install

    monkeypatch.setattr(skill_install, "is_skills_available", lambda: True)
    slim = get_agents_md_template()
    assert "logmind-block-version: v6-pointer" in slim
    # The load-bearing contract — "logmind log replaces git add+commit+push"
    # — must survive every future trim. If this assertion fails, the
    # block was over-trimmed and agents will fall back to direct git.
    assert "logmind log` is the commit primitive" in slim
    assert "replaces `git add` + `git commit` + `git push`" in slim
    # Skill pointer is the authority delegation — without it, agents
    # have no way to find the full procedure when this block trims it out.
    assert "agent-skills/tree/main/skills/logmind" in slim
    # Hard size cap on the slim variant — guards against future block
    # growth re-bloating it. v6 is ~770 bytes; cap at 1500 leaves 2×
    # headroom for additions while preventing return to v5's 2526.
    block_start = slim.find("<!-- logmind-start -->")
    block_end = slim.find("<!-- logmind-end -->") + len("<!-- logmind-end -->")
    assert block_start != -1 and block_end > block_start
    block = slim[block_start:block_end]
    assert len(block.encode("utf-8")) <= 1500, (
        f"slim logmind-block is {len(block.encode('utf-8'))} bytes "
        f"— v6-pointer should stay under 1500 (current ~770)"
    )


def test_get_agents_md_template_returns_full_when_skills_absent(monkeypatch):
    """Without skills.sh, the full template ships the procedure inline."""
    from logmind.core import skill_install

    monkeypatch.setattr(skill_install, "is_skills_available", lambda: False)
    full = get_agents_md_template()
    # Match exact marker (v4 — not v4-slim, which is a different file). Use
    # the line-exact match so a future v4-extended variant wouldn't also
    # satisfy this assertion accidentally.
    assert "logmind-block-version: v5 " in full or "logmind-block-version: v5\n" in full
    # Full template carries the inline procedure
    assert "When you MUST log" in full or "REQUIREMENT" in full or "skill is also embedded" in full


# ---------------------------------------------------------------------------
# create_agent_file (new behaviour)
# ---------------------------------------------------------------------------


def test_create_agent_file_for_codex_writes_canonical_template(tmp_path):
    p = create_agent_file("codex", tmp_path)
    assert p == tmp_path / "AGENTS.md"
    assert has_logmind_section(p.read_text(encoding="utf-8"))
    # Not a stub — it's the canonical doc
    assert not is_stub(p.read_text(encoding="utf-8"))


@pytest.mark.parametrize(
    "agent",
    ["claude", "cursor", "windsurf", "aider", "continue", "amazonq", "cline", "copilot"],
)
def test_create_agent_file_for_markdown_agents_writes_stub(tmp_path, agent):
    p = create_agent_file(agent, tmp_path)
    assert p is not None
    content = p.read_text(encoding="utf-8")
    assert is_stub(content)
    assert "AGENTS.md" in content


@pytest.mark.parametrize("agent", ["cody", "zed"])
def test_create_agent_file_for_json_agents_keeps_json(tmp_path, agent):
    p = create_agent_file(agent, tmp_path)
    assert p is not None
    content = p.read_text(encoding="utf-8")
    # JSON files don't contain the stub marker
    assert not is_stub(content)
    assert content.strip().startswith("{")


# ---------------------------------------------------------------------------
# insert_into_all_ai_files
# ---------------------------------------------------------------------------


def test_insert_into_all_ai_files_creates_agents_md_and_stubs(tmp_path):
    msgs = insert_into_all_ai_files(tmp_path, agents=["claude", "cursor"])

    assert (tmp_path / "AGENTS.md").exists()
    assert has_logmind_section((tmp_path / "AGENTS.md").read_text(encoding="utf-8"))

    assert (tmp_path / "CLAUDE.md").exists()
    assert is_stub((tmp_path / "CLAUDE.md").read_text(encoding="utf-8"))

    assert (tmp_path / ".cursorrules").exists()
    assert is_stub((tmp_path / ".cursorrules").read_text(encoding="utf-8"))

    # Status messages mention canonical + stubs
    joined = "\n".join(msgs)
    assert "AGENTS.md" in joined
    assert "stub" in joined.lower()


def test_insert_into_all_ai_files_preserves_existing_user_content(tmp_path):
    """An existing CLAUDE.md with user content gets a logmind block inserted,
    not blindly overwritten with a stub."""
    (tmp_path / "CLAUDE.md").write_text(
        "# CLAUDE.md\n\nProject-specific guidance.\n"
    , encoding="utf-8")
    msgs = insert_into_all_ai_files(tmp_path, agents=["claude"])

    content = (tmp_path / "CLAUDE.md").read_text(encoding="utf-8")
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
    content = (tmp_path / "CLAUDE.md").read_text(encoding="utf-8")
    assert is_stub(content)


def test_insert_into_all_ai_files_codex_is_canonical_not_stub(tmp_path):
    insert_into_all_ai_files(tmp_path, agents=["codex"])
    content = (tmp_path / "AGENTS.md").read_text(encoding="utf-8")
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
    , encoding="utf-8")
    (tmp_path / ".cursorrules").write_text(
        "# Cursor Rules\n\nMy Cursor-specific rules.\n\n"
        f"{LOGMIND_START_MARKER}\nold logmind block\n{LOGMIND_END_MARKER}\n"
    , encoding="utf-8")

    msgs = migrate_to_agents_md(tmp_path)

    # AGENTS.md created and contains migrated content
    agents_content = (tmp_path / "AGENTS.md").read_text(encoding="utf-8")
    assert "My Claude-specific guidance" in agents_content
    assert "My Cursor-specific rules" in agents_content
    assert has_logmind_section(agents_content)

    # Per-agent files now stubs
    assert is_stub((tmp_path / "CLAUDE.md").read_text(encoding="utf-8"))
    assert is_stub((tmp_path / ".cursorrules").read_text(encoding="utf-8"))

    # Status mentions migration
    joined = "\n".join(msgs)
    assert "Migrated" in joined
    assert "stub" in joined.lower()


def test_migrate_idempotent_on_already_stubbed_tree(tmp_path):
    insert_into_all_ai_files(tmp_path, agents=["claude", "cursor"])
    # First migration is a no-op (everything already a stub from init flow)
    msgs = migrate_to_agents_md(tmp_path)
    assert msgs == []


def test_sync_does_not_trample_existing_stubs(tmp_path):
    """Regression: sync_agent_files_from_config must not insert a logmind block
    into a stub file (it would defeat the AGENTS.md-canonical model)."""
    # Init the project so the config file exists
    insert_into_all_ai_files(tmp_path, agents=["claude", "cursor"])
    (tmp_path / ".logmind").mkdir(exist_ok=True)
    (tmp_path / ".logmind" / "config.yml").write_text(
        "agents:\n  claude: true\n  cursor: true\n"
    , encoding="utf-8")
    stub_before = (tmp_path / "CLAUDE.md").read_text(encoding="utf-8")

    msgs = sync_agent_files_from_config(tmp_path)

    # Sync is a silent no-op for stubs (no insertion message)
    assert all("Added logmind section to CLAUDE.md" not in m for m in msgs)
    # File still exactly the stub
    assert (tmp_path / "CLAUDE.md").read_text(encoding="utf-8") == stub_before
    assert is_stub((tmp_path / "CLAUDE.md").read_text(encoding="utf-8"))


def test_find_outdated_returns_empty_when_no_agents_md(tmp_path):
    from logmind.core.inserter import find_outdated_marker_blocks

    assert find_outdated_marker_blocks(tmp_path) == []


def test_find_outdated_returns_empty_when_current(tmp_path):
    from logmind.core.inserter import find_outdated_marker_blocks

    (tmp_path / "AGENTS.md").write_text(get_agents_md_template(), encoding="utf-8")
    assert find_outdated_marker_blocks(tmp_path) == []


def test_find_outdated_detects_stale_block(tmp_path):
    from logmind.core.inserter import (
        LOGMIND_END_MARKER,
        LOGMIND_START_MARKER,
        find_outdated_marker_blocks,
    )

    (tmp_path / "AGENTS.md").write_text(
        "# AGENTS.md\n\n"
        f"{LOGMIND_START_MARKER}\nOLD CONTENT\n{LOGMIND_END_MARKER}\n",
        encoding="utf-8",
    )
    out = find_outdated_marker_blocks(tmp_path)
    assert len(out) == 1
    file_path, current, fresh = out[0]
    assert file_path == tmp_path / "AGENTS.md"
    assert "OLD CONTENT" in current
    assert "OLD CONTENT" not in fresh


def test_agents_update_cli_dry_run_reports_without_writing(tmp_path, monkeypatch):
    from click.testing import CliRunner
    from logmind.cli import main as cli_main
    from logmind.core.inserter import LOGMIND_END_MARKER, LOGMIND_START_MARKER

    user_above = "# AGENTS.md\n\nMy header.\n\n"
    (tmp_path / "AGENTS.md").write_text(
        user_above + f"{LOGMIND_START_MARKER}\nSTALE\n{LOGMIND_END_MARKER}\n",
        encoding="utf-8",
    )

    monkeypatch.chdir(tmp_path)
    result = CliRunner().invoke(cli_main, ["agents", "update"])
    assert result.exit_code == 0
    assert "stale logmind block" in result.output
    assert "Dry-run" in result.output
    # File NOT rewritten
    assert "STALE" in (tmp_path / "AGENTS.md").read_text(encoding="utf-8")


def test_agents_update_cli_apply_rewrites_and_preserves_user_content(tmp_path, monkeypatch):
    from click.testing import CliRunner
    from logmind.cli import main as cli_main
    from logmind.core.inserter import LOGMIND_END_MARKER, LOGMIND_START_MARKER

    user_above = "# AGENTS.md\n\nMy header.\n\n"
    user_below = "\n## My section\nMy body.\n"
    (tmp_path / "AGENTS.md").write_text(
        user_above + f"{LOGMIND_START_MARKER}\nSTALE\n{LOGMIND_END_MARKER}" + user_below,
        encoding="utf-8",
    )

    monkeypatch.chdir(tmp_path)
    result = CliRunner().invoke(cli_main, ["agents", "update", "--apply"])
    assert result.exit_code == 0
    assert "Refreshed" in result.output

    content = (tmp_path / "AGENTS.md").read_text(encoding="utf-8")
    assert "My header." in content  # preserved above
    assert "My section" in content  # preserved below
    assert "STALE" not in content  # rewritten

    # Second invocation is a no-op
    result2 = CliRunner().invoke(cli_main, ["agents", "update"])
    assert "All agent files are current" in result2.output


def test_migrate_skips_json_agents(tmp_path):
    # Set up a JSON agent file with content
    (tmp_path / ".sourcegraph").mkdir()
    (tmp_path / ".sourcegraph" / "cody.json").write_text('{"existing": true}', encoding="utf-8")

    migrate_to_agents_md(tmp_path)

    # JSON file untouched
    assert (tmp_path / ".sourcegraph" / "cody.json").read_text(encoding="utf-8") == '{"existing": true}'
