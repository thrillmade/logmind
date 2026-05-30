"""Tests for v0.6.0 `logmind skill new/test` CLI.

These cover the basic-scaffold path (no `skdd` on PATH) since that's
what's deterministically reproducible in CI. The skdd-delegate path
is exercised via subprocess mocking.
"""

from __future__ import annotations

import subprocess
from pathlib import Path
from unittest.mock import patch

import pytest
from click.testing import CliRunner

from logmind.cli import main as cli_main
from logmind.core.skill_cli import (
    check_frontmatter_required_fields,
    check_size_cap,
    scaffold_basic_skill,
    skill_md_path,
)


# ---------------------------------------------------------------------------
# scaffold_basic_skill — pure helper
# ---------------------------------------------------------------------------


def test_scaffold_basic_skill_creates_valid_skill_md(tmp_path):
    target = scaffold_basic_skill(tmp_path, "my-skill", description="trigger")
    assert target.exists()
    assert target == tmp_path / ".claude" / "skills" / "my-skill" / "SKILL.md"
    content = target.read_text(encoding="utf-8")
    assert "name: my-skill" in content
    assert "description: trigger" in content
    assert "metadata:" in content
    assert "spec: agentskills.io" in content


def test_scaffold_basic_skill_refuses_to_clobber(tmp_path):
    scaffold_basic_skill(tmp_path, "my-skill")
    with pytest.raises(FileExistsError):
        scaffold_basic_skill(tmp_path, "my-skill")


def test_scaffold_basic_skill_emits_TODO_when_no_description(tmp_path):
    """When the user doesn't pass --description, scaffold a TODO so they
    know it needs filling — the description IS the discovery surface."""
    target = scaffold_basic_skill(tmp_path, "my-skill")
    content = target.read_text(encoding="utf-8")
    assert "TODO" in content
    assert "trigger description" in content


# ---------------------------------------------------------------------------
# Validation helpers
# ---------------------------------------------------------------------------


def test_check_frontmatter_required_fields_passes_valid():
    content = "---\nname: x\ndescription: y\n---\n\nbody\n"
    ok, msg = check_frontmatter_required_fields(content)
    assert ok is True


def test_check_frontmatter_required_fields_fails_no_frontmatter():
    ok, msg = check_frontmatter_required_fields("just body text\n")
    assert ok is False
    assert "frontmatter" in msg.lower()


def test_check_frontmatter_required_fields_fails_missing_name():
    content = "---\ndescription: y\n---\nbody\n"
    ok, msg = check_frontmatter_required_fields(content)
    assert ok is False
    assert "name" in msg


def test_check_frontmatter_required_fields_fails_missing_description():
    content = "---\nname: x\n---\nbody\n"
    ok, msg = check_frontmatter_required_fields(content)
    assert ok is False
    assert "description" in msg


def test_check_frontmatter_required_fields_fails_unterminated():
    content = "---\nname: x\ndescription: y\n\nno closing dashes\n"
    ok, msg = check_frontmatter_required_fields(content)
    assert ok is False


def test_check_size_cap_passes_small_skill():
    ok, msg = check_size_cap("---\nname: x\ndescription: y\n---\n\nshort body\n")
    assert ok is True


def test_check_size_cap_fails_large_skill():
    bloat = "x" * 10000
    ok, msg = check_size_cap(bloat)
    assert ok is False
    assert "8000" in msg  # cap mentioned in error


# ---------------------------------------------------------------------------
# `logmind skill new` CLI (basic-scaffold path, skdd mocked off)
# ---------------------------------------------------------------------------


def test_skill_new_creates_skill_via_basic_scaffold(tmp_path, monkeypatch):
    """When skdd is NOT on PATH, fall back to basic scaffold."""
    monkeypatch.chdir(tmp_path)

    with patch("logmind.core.skill_cli.shutil.which", return_value=None):
        runner = CliRunner()
        result = runner.invoke(
            cli_main,
            ["skill", "new", "my-skill", "--description", "trigger here", "--no-log"],
        )

    assert result.exit_code == 0, result.output
    assert "Created skill 'my-skill'" in result.output
    target = skill_md_path(tmp_path, "my-skill")
    assert target.exists()
    content = target.read_text(encoding="utf-8")
    assert "name: my-skill" in content
    assert "description: trigger here" in content


def test_skill_new_refuses_to_clobber(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    scaffold_basic_skill(tmp_path, "existing")

    with patch("logmind.core.skill_cli.shutil.which", return_value=None):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["skill", "new", "existing", "--no-log"])

    assert result.exit_code == 1
    assert "already exists" in result.output


def test_skill_new_no_log_flag_skips_decision_log(tmp_path, monkeypatch):
    """--no-log skips the auto-decision-log even when docs/ exists."""
    monkeypatch.chdir(tmp_path)
    (tmp_path / "docs").mkdir()
    (tmp_path / "docs" / "decisions.md").write_text("# Decisions\n", encoding="utf-8")

    with patch("logmind.core.skill_cli.shutil.which", return_value=None):
        runner = CliRunner()
        result = runner.invoke(
            cli_main,
            ["skill", "new", "no-log-skill", "--no-log"],
        )

    assert result.exit_code == 0, result.output
    decisions = (tmp_path / "docs" / "decisions.md").read_text(encoding="utf-8")
    assert "no-log-skill" not in decisions, (
        "v0.6.0 --no-log: must not write to decisions.md"
    )


def test_skill_new_without_docs_dir_silently_skips_decision_log(tmp_path, monkeypatch):
    """When docs/ doesn't exist (logmind init not run yet), skill new still
    succeeds — the decision-log step is skipped with a helpful note."""
    monkeypatch.chdir(tmp_path)
    # No docs/ directory

    with patch("logmind.core.skill_cli.shutil.which", return_value=None):
        runner = CliRunner()
        result = runner.invoke(
            cli_main, ["skill", "new", "first-skill", "--description", "trigger"]
        )

    assert result.exit_code == 0, result.output
    assert "skipped decision-log" in result.output.lower() or "docs/" in result.output


# ---------------------------------------------------------------------------
# `logmind skill test` CLI (basic checks only, skdd mocked off)
# ---------------------------------------------------------------------------


def test_skill_test_fails_on_missing_skill(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    runner = CliRunner()
    result = runner.invoke(cli_main, ["skill", "test", "does-not-exist"])
    assert result.exit_code == 1
    assert "not found" in result.output.lower()


def test_skill_test_passes_on_valid_skill(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    scaffold_basic_skill(tmp_path, "good-skill", description="valid trigger")

    with patch("logmind.core.skill_cli.shutil.which", return_value=None):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["skill", "test", "good-skill"])

    assert result.exit_code == 0, result.output
    assert "frontmatter required fields" in result.output
    assert "size cap" in result.output


def test_skill_test_fails_on_broken_frontmatter(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    # Create a SKILL.md without frontmatter
    target = tmp_path / ".claude" / "skills" / "broken" / "SKILL.md"
    target.parent.mkdir(parents=True)
    target.write_text("just body text — no frontmatter\n", encoding="utf-8")

    with patch("logmind.core.skill_cli.shutil.which", return_value=None):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["skill", "test", "broken"])

    assert result.exit_code == 1
    assert "frontmatter" in result.output.lower()


def test_skill_test_fails_on_oversized_skill(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    target = tmp_path / ".claude" / "skills" / "bloat" / "SKILL.md"
    target.parent.mkdir(parents=True)
    target.write_text(
        "---\nname: bloat\ndescription: huge\n---\n" + "x" * 10000,
        encoding="utf-8",
    )

    with patch("logmind.core.skill_cli.shutil.which", return_value=None):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["skill", "test", "bloat"])

    assert result.exit_code == 1
    assert "8000" in result.output  # cap mentioned


# ---------------------------------------------------------------------------
# Skdd delegate paths (mocked subprocess)
# ---------------------------------------------------------------------------


def test_skill_new_delegates_to_skdd_forge_when_available(tmp_path, monkeypatch):
    """When `skdd` is on PATH, prefer `skdd forge <name>` over basic scaffold."""
    monkeypatch.chdir(tmp_path)

    forge_calls: list = []
    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        if cmd[:2] == ["skdd", "forge"]:
            forge_calls.append(cmd)
            # Pretend skdd succeeded + created the file
            target = skill_md_path(tmp_path, cmd[2])
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(
                f"---\nname: {cmd[2]}\ndescription: forged\n---\nbody\n",
                encoding="utf-8",
            )
            return subprocess.CompletedProcess(args=cmd, returncode=0, stdout="forged", stderr="")
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.core.skill_cli.shutil.which", return_value="/usr/bin/skdd"), \
         patch("logmind.core.skill_cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(
            cli_main, ["skill", "new", "forged-skill", "--no-log"]
        )

    assert result.exit_code == 0, result.output
    assert len(forge_calls) == 1
    assert forge_calls[0] == ["skdd", "forge", "forged-skill"]
    target = skill_md_path(tmp_path, "forged-skill")
    assert target.exists()


def test_skill_test_delegates_to_skdd_validate_when_available(tmp_path, monkeypatch):
    """When `skdd` is on PATH, run `skdd validate` AND the logmind layered checks."""
    monkeypatch.chdir(tmp_path)
    scaffold_basic_skill(tmp_path, "delegated-skill", description="trigger")

    validate_calls: list = []
    real_run = subprocess.run

    def mock_run(cmd, *args, **kwargs):
        if cmd[:2] == ["skdd", "validate"]:
            validate_calls.append(cmd)
            return subprocess.CompletedProcess(
                args=cmd, returncode=0, stdout="all skills passed validation", stderr=""
            )
        return real_run(cmd, *args, **kwargs)

    with patch("logmind.core.skill_cli.shutil.which", return_value="/usr/bin/skdd"), \
         patch("logmind.core.skill_cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["skill", "test", "delegated-skill"])

    assert result.exit_code == 0, result.output
    assert len(validate_calls) == 1
    assert "skdd validate passed" in result.output
    # Logmind-layered checks also ran
    assert "frontmatter required fields" in result.output
