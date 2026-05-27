"""Tests for the v0.4.0 propose_skill_update.py CI helper.

The script itself lives at `.github/scripts/propose_skill_update.py`
(outside src/ — it's a CI artifact, not part of the installable
package). These tests import it via a path append so the same parsing
logic that runs in CI is exercised locally.

We don't make real Anthropic API calls; the `call_claude` function is
mocked at module level so we can assert prompt construction without
network or cost.
"""

from __future__ import annotations

import sys
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

# Insert the scripts dir so we can import propose_skill_update.
SCRIPTS_DIR = Path(__file__).parent.parent / ".github" / "scripts"
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import propose_skill_update as psu  # noqa: E402


# ---------------------------------------------------------------------------
# parse_response — the format-handling core
# ---------------------------------------------------------------------------


def test_parse_response_full_proposal():
    """Happy path: model returns <reasoning> + <skill_md>."""
    response = """<reasoning>v0.4.0 ships PR-shape notify; agents
need to know about the new auto-fix workflow.</reasoning>
<skill_md>---
name: logmind
review_mode: shared
---
# logmind
new body content
</skill_md>"""
    proposed, reasoning = psu.parse_response(response)
    assert proposed is not None
    assert "new body content" in proposed
    assert proposed.endswith("\n")  # parse_response normalizes trailing newline
    assert "PR-shape" in reasoning
    assert "agents" in reasoning


def test_parse_response_sentinel():
    """Model decides no skill update needed — returns sentinel + reasoning."""
    response = """NO_SKILL_UPDATE_NEEDED
<reasoning>v0.4.0 is a CI-internal change; nothing user-facing.</reasoning>"""
    proposed, reasoning = psu.parse_response(response)
    assert proposed is None
    assert "CI-internal" in reasoning


def test_parse_response_sentinel_precedence_over_stray_block():
    """If the model emits BOTH sentinel and a <skill_md> block, sentinel wins.
    Conservative: anyone reading the response sees the explicit no-update
    signal and we don't accidentally ship a half-formed proposal."""
    response = """NO_SKILL_UPDATE_NEEDED
<reasoning>actually no change</reasoning>
<skill_md>oops should have been silent</skill_md>"""
    proposed, reasoning = psu.parse_response(response)
    assert proposed is None  # sentinel takes precedence
    assert "no change" in reasoning


def test_parse_response_malformed():
    """No sentinel + no <skill_md> block — degrade to no-op with a flag in
    reasoning so the human reviewer can spot the malformed response."""
    response = "I think you should update the SKILL.md but I forgot how."
    proposed, reasoning = psu.parse_response(response)
    assert proposed is None
    assert "malformed" in reasoning


def test_parse_response_skill_only_no_reasoning():
    """If the model emits <skill_md> but no <reasoning>, surface the gap.
    The skill update still proposed; the human sees empty reasoning."""
    response = "<skill_md>---\nname: logmind\n---\n# body\n</skill_md>"
    proposed, reasoning = psu.parse_response(response)
    assert proposed is not None
    assert "body" in proposed
    assert reasoning == ""  # no reasoning extracted; workflow writes "(no reasoning provided)"


# ---------------------------------------------------------------------------
# call_claude — guard paths (no real API call)
# ---------------------------------------------------------------------------


def test_call_claude_raises_without_api_key(monkeypatch):
    # Patch Anthropic so the first guard (package-present) passes;
    # we're verifying the API-key guard specifically.
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
    monkeypatch.setattr(psu, "Anthropic", MagicMock())
    with pytest.raises(RuntimeError, match="ANTHROPIC_API_KEY"):
        psu.call_claude("changelog", "skill", "v0.4.0")


def test_call_claude_raises_without_anthropic_package(monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "fake-key")
    monkeypatch.setattr(psu, "Anthropic", None)
    with pytest.raises(RuntimeError, match="anthropic package"):
        psu.call_claude("changelog", "skill", "v0.4.0")


def test_call_claude_constructs_user_message_with_inputs(monkeypatch):
    """Mock the Anthropic client and assert the user message includes the
    changelog section and current SKILL.md verbatim."""
    monkeypatch.setenv("ANTHROPIC_API_KEY", "fake-key")

    fake_response = MagicMock()
    fake_text_block = MagicMock()
    fake_text_block.text = "<reasoning>x</reasoning><skill_md>y</skill_md>"
    fake_response.content = [fake_text_block]

    fake_client = MagicMock()
    fake_client.messages.create.return_value = fake_response

    with patch.object(psu, "Anthropic", return_value=fake_client):
        result = psu.call_claude(
            changelog_section="### [0.4.0] - X\n- New CLI flag --foo",
            current_skill="---\nname: logmind\n---\n# old body",
            version="v0.4.0",
        )

    # Inspect the call: kwargs passed to messages.create
    kwargs = fake_client.messages.create.call_args.kwargs
    assert kwargs["model"] == psu.MODEL
    user_message = kwargs["messages"][0]["content"]
    assert "--foo" in user_message  # changelog text passed verbatim
    assert "old body" in user_message  # current SKILL.md passed verbatim
    assert "v0.4.0" in user_message
    # Verify the system prompt establishes the role
    assert "documentation engineer" in kwargs["system"].lower()
    # Verify the result is the model's text output
    assert "<skill_md>y</skill_md>" in result


def test_call_claude_raises_on_empty_response(monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "fake-key")

    fake_response = MagicMock()
    fake_response.content = []  # no blocks
    fake_client = MagicMock()
    fake_client.messages.create.return_value = fake_response

    with patch.object(psu, "Anthropic", return_value=fake_client):
        with pytest.raises(RuntimeError, match="empty response"):
            psu.call_claude("c", "s", "v0.4.0")


# ---------------------------------------------------------------------------
# main() — end-to-end wiring with mocked Claude
# ---------------------------------------------------------------------------


def test_main_writes_both_files_on_proposal(tmp_path, monkeypatch):
    """Full end-to-end with mocked API: changelog + skill in, proposal out
    on disk."""
    monkeypatch.setenv("ANTHROPIC_API_KEY", "fake-key")
    changelog = tmp_path / "changelog.md"
    changelog.write_text("### [0.4.0]\n- New thing\n", encoding="utf-8")
    skill = tmp_path / "skill.md"
    skill.write_text("---\nname: logmind\n---\n# old\n", encoding="utf-8")
    out_reason = tmp_path / "out" / "reason.md"
    out_skill = tmp_path / "out" / "skill.md"

    fake_response = MagicMock()
    fake_response.content = [MagicMock(text=(
        "<reasoning>release adds --foo</reasoning>\n"
        "<skill_md>---\nname: logmind\n---\n# updated body\n</skill_md>"
    ))]

    with patch.object(psu, "Anthropic", return_value=MagicMock(
        messages=MagicMock(create=MagicMock(return_value=fake_response))
    )):
        rc = psu.main([
            "--changelog", str(changelog),
            "--current-skill", str(skill),
            "--version", "v0.4.0",
            "--out-reasoning", str(out_reason),
            "--out-skill", str(out_skill),
        ])

    assert rc == 0
    assert "release adds --foo" in out_reason.read_text(encoding="utf-8")
    assert "updated body" in out_skill.read_text(encoding="utf-8")


def test_main_skips_skill_file_on_sentinel(tmp_path, monkeypatch):
    """When Claude returns the sentinel, only the reasoning file is
    written — the workflow inspects the out_skill path's existence to
    decide whether to overwrite the on-disk SKILL.md."""
    monkeypatch.setenv("ANTHROPIC_API_KEY", "fake-key")
    changelog = tmp_path / "changelog.md"
    changelog.write_text("### [0.4.0]\n- Internal CI tweak\n", encoding="utf-8")
    skill = tmp_path / "skill.md"
    skill.write_text("---\nname: logmind\n---\n# unchanged\n", encoding="utf-8")
    out_reason = tmp_path / "reason.md"
    out_skill = tmp_path / "skill-proposed.md"

    fake_response = MagicMock()
    fake_response.content = [MagicMock(text=(
        "NO_SKILL_UPDATE_NEEDED\n"
        "<reasoning>release is internal CI plumbing; SKILL.md untouched.</reasoning>"
    ))]

    with patch.object(psu, "Anthropic", return_value=MagicMock(
        messages=MagicMock(create=MagicMock(return_value=fake_response))
    )):
        rc = psu.main([
            "--changelog", str(changelog),
            "--current-skill", str(skill),
            "--version", "v0.4.0",
            "--out-reasoning", str(out_reason),
            "--out-skill", str(out_skill),
        ])

    assert rc == 0
    assert "internal CI plumbing" in out_reason.read_text(encoding="utf-8")
    assert not out_skill.exists()  # not created on sentinel
