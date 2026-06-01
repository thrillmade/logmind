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
                args=cmd, returncode=0,
                stdout="✓ skills/delegated-skill/SKILL.md: ok",
                stderr="",
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


# ---------------------------------------------------------------------------
# v0.6.0 PR #92 review fixes — regression tests
# ---------------------------------------------------------------------------


def test_skill_test_per_skill_gate_not_colony_gate(tmp_path, monkeypatch):
    """v0.6.0 PR #92 critical #1: `logmind skill test <name>` must gate on
    that skill's pass/fail line, NOT the colony-wide exit code. Setup:
    `skdd validate` exits non-zero (some OTHER skill broken in the colony)
    but the target skill's line is a pass — we should still pass.
    """
    monkeypatch.chdir(tmp_path)
    scaffold_basic_skill(tmp_path, "good-skill", description="trigger")
    scaffold_basic_skill(tmp_path, "broken-other", description="")

    def mock_run(cmd, *args, **kwargs):
        if cmd[:2] == ["skdd", "validate"]:
            return subprocess.CompletedProcess(
                args=cmd,
                returncode=1,  # colony-wide failure due to broken-other
                stdout=(
                    "✓ skills/good-skill/SKILL.md: ok\n"
                    "✗ skills/broken-other/SKILL.md: missing description\n"
                ),
                stderr="",
            )
        return subprocess.run(cmd, *args, **kwargs)

    with patch("logmind.core.skill_cli.shutil.which", return_value="/usr/bin/skdd"), \
         patch("logmind.core.skill_cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["skill", "test", "good-skill"])

    assert result.exit_code == 0, (
        f"v0.6.0 PR #92 critical #1 regression: per-skill test must pass "
        f"when target skill's line is ok even if colony-wide exit is "
        f"non-zero. Got:\n{result.output}"
    )
    assert "skdd validate passed" in result.output


def test_skill_test_per_skill_gate_fails_when_target_fails(tmp_path, monkeypatch):
    """Inverse: when the target skill's line shows fail, exit 1 even if
    other skills passed (no false-pass from grepping the right line)."""
    monkeypatch.chdir(tmp_path)
    scaffold_basic_skill(tmp_path, "broken-target", description="trigger")

    def mock_run(cmd, *args, **kwargs):
        if cmd[:2] == ["skdd", "validate"]:
            return subprocess.CompletedProcess(
                args=cmd,
                returncode=0,  # colony-wide OK
                stdout=(
                    "✓ skills/good-skill/SKILL.md: ok\n"
                    "✗ skills/broken-target/SKILL.md: invalid frontmatter\n"
                ),
                stderr="",
            )
        return subprocess.run(cmd, *args, **kwargs)

    with patch("logmind.core.skill_cli.shutil.which", return_value="/usr/bin/skdd"), \
         patch("logmind.core.skill_cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(cli_main, ["skill", "test", "broken-target"])

    assert result.exit_code == 1


def test_skill_new_skdd_partial_failure_no_traceback(tmp_path, monkeypatch):
    """v0.6.0 PR #92 critical #2: when skdd forge CREATES the SKILL.md
    AND THEN exits non-zero (post-creation check fails), the fallback
    scaffold_basic_skill must NOT raise an uncaught FileExistsError.
    User sees a clean error + clean exit, not a Python traceback.
    """
    monkeypatch.chdir(tmp_path)

    def mock_run(cmd, *args, **kwargs):
        if cmd[:2] == ["skdd", "forge"]:
            # Create the SKILL.md AND return non-zero — the bug scenario
            target = skill_md_path(tmp_path, cmd[2])
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(
                f"---\nname: {cmd[2]}\ndescription: \n---\nbody\n",
                encoding="utf-8",
            )
            return subprocess.CompletedProcess(
                args=cmd, returncode=1,
                stdout="created file but description is empty", stderr="",
            )
        return subprocess.run(cmd, *args, **kwargs)

    with patch("logmind.core.skill_cli.shutil.which", return_value="/usr/bin/skdd"), \
         patch("logmind.core.skill_cli.subprocess.run", side_effect=mock_run):
        runner = CliRunner()
        result = runner.invoke(
            cli_main, ["skill", "new", "partial-fail-skill", "--no-log"]
        )

    assert result.exit_code == 1
    # No raw Python traceback in the output
    assert "FileExistsError" not in result.output, (
        f"v0.6.0 PR #92 critical #2 regression: raw FileExistsError "
        f"traceback in user-facing output. Got:\n{result.output}"
    )
    assert "Traceback" not in result.output
    # User-friendly error mentions next steps
    assert (
        "partially succeeded" in result.output.lower()
        or "rm -r" in result.output
    )


def test_check_frontmatter_no_false_positive_on_domain_name(tmp_path):
    """v0.6.0 PR #92 minor: `domain_name:` must NOT satisfy the `name:`
    required-field check. Pre-fix used substring `in`; now uses anchored
    regex.
    """
    content = (
        "---\n"
        "domain_name: my-domain\n"
        "description: missing real name field\n"
        "---\n"
        "body\n"
    )
    ok, msg = check_frontmatter_required_fields(content)
    assert ok is False, (
        f"v0.6.0 PR #92 minor regression: 'domain_name:' falsely passed "
        f"as 'name:'. Got ok={ok}, msg={msg!r}"
    )
    assert "name" in msg


def test_check_frontmatter_no_false_positive_on_description_nested():
    """Mirror test for description field — `pkg_description:` shouldn't pass."""
    content = (
        "---\n"
        "name: x\n"
        "pkg_description: nested only\n"
        "---\n"
        "body\n"
    )
    ok, msg = check_frontmatter_required_fields(content)
    assert ok is False
    assert "description" in msg


# ---------------------------------------------------------------------------
# v0.6.3 — bench_skill
# ---------------------------------------------------------------------------


def _bench_skill():
    """Late import so the rest of the test file doesn't pay the cost."""
    from logmind.core.skill_cli import bench_skill
    return bench_skill


def test_bench_skill_tight():
    """A short skill lands in the 'tight' bucket."""
    bench_skill = _bench_skill()
    content = (
        "---\nname: x\ndescription: y\n---\n"
        "# Title\n\n## When to use\n- a\n- b\n\n## Steps\n1. one\n"
    )
    out = bench_skill(content)
    assert out["status"] == "tight"
    assert out["bytes"] < 2000
    assert out["est_tokens"] == out["bytes"] // 4
    assert out["suggestions"] == []


def test_bench_skill_typical():
    """Between tight and budget = 'typical'."""
    bench_skill = _bench_skill()
    body = "x " * 1500  # ~3000 bytes — between tight (2000) and budget (6000)
    content = f"---\nname: x\ndescription: y\n---\n# T\n{body}"
    out = bench_skill(content)
    assert out["status"] == "typical"
    assert out["suggestions"] == []


def test_bench_skill_verbose_emits_suggestions():
    """Past budget = 'verbose' AND gets at least one suggestion."""
    bench_skill = _bench_skill()
    body = "y " * 3500  # ~7000 bytes, past 6000 budget but under 8000 cap
    content = f"---\nname: x\ndescription: y\n---\n# T\n## Examples\n{body}"
    out = bench_skill(content)
    assert out["status"] == "verbose"
    assert len(out["suggestions"]) >= 1


def test_bench_skill_over_budget_recommends_split():
    """Past 8KB hard cap = 'over-budget' AND suggestion mentions splitting."""
    bench_skill = _bench_skill()
    body = "z " * 5000  # ~10000 bytes — past the 8000 cap
    content = f"---\nname: x\ndescription: y\n---\n# T\n## Examples\n{body}"
    out = bench_skill(content)
    assert out["status"] == "over-budget"
    assert any("split" in s.lower() for s in out["suggestions"])


def test_bench_skill_section_breakdown():
    """Each ## section appears in the breakdown with its byte count."""
    bench_skill = _bench_skill()
    content = (
        "---\nname: x\ndescription: y\n---\n"
        "# Title\n\n"
        "## When to use\n- short trigger\n\n"
        "## Steps\n1. first\n2. second\n\n"
        "## Examples\nlorem ipsum dolor sit amet " * 30 + "\n"
    )
    out = bench_skill(content)
    section_names = [s["name"] for s in out["sections"]]
    assert "frontmatter" in section_names
    assert "When to use" in section_names
    assert "Steps" in section_names
    assert "Examples" in section_names
    # Each pct is non-negative + sums roughly to 100% (rounding).
    assert all(0 <= s["pct"] <= 100 for s in out["sections"])


def test_bench_skill_dominant_section_flagged():
    """A section taking >30% of the total gets called out in suggestions."""
    bench_skill = _bench_skill()
    # Make the Examples section dominate (over budget + over 30%).
    big_examples = "example " * 1200  # ~9600 bytes — definitively dominant
    content = (
        "---\nname: x\ndescription: y\n---\n"
        "# T\n## When to use\n- a\n\n## Examples\n" + big_examples
    )
    out = bench_skill(content)
    assert any("Examples" in s for s in out["suggestions"])


def test_bench_skill_html_comments_flagged_when_large():
    """Many HTML comments are flagged as wasted bytes."""
    bench_skill = _bench_skill()
    big_comment = "<!-- " + ("authoring note " * 50) + "-->"  # ~750 bytes
    body = "real prose " * 1000  # ~11000 bytes, pushes past 8KB
    content = (
        f"---\nname: x\ndescription: y\n---\n"
        f"# T\n## When\n{big_comment}\n\n## Body\n{body}"
    )
    out = bench_skill(content)
    assert any("HTML comments" in s for s in out["suggestions"])


def test_bench_skill_no_headers_falls_into_body():
    """A skill with no ## headers gets a single 'body' section."""
    bench_skill = _bench_skill()
    content = "---\nname: x\ndescription: y\n---\n# Title\njust prose, no sections.\n"
    out = bench_skill(content)
    names = [s["name"] for s in out["sections"]]
    assert "body" in names


def test_bench_skill_honors_custom_budget():
    """PR #99 regression: target + budget kwargs must thread through to
    _bench_status + _trim_suggestions, not be silently ignored.

    A 3KB skill is 'typical' under the default 6KB budget, but should
    be 'verbose' when the caller passes budget=2000.
    """
    bench_skill = _bench_skill()
    body = "y " * 1500  # ~3000 bytes
    content = f"---\nname: x\ndescription: y\n---\n# T\n## Examples\n{body}"
    default = bench_skill(content)
    custom = bench_skill(content, target=500, budget=2000)
    assert default["status"] == "typical"
    assert custom["status"] == "verbose", (
        f"PR #99 review fix: custom budget=2000 should be honored. "
        f"Got status={custom['status']}"
    )
    # Suggestions should fire under the tighter budget.
    assert len(custom["suggestions"]) >= 1
    # And not under the looser default.
    assert default["suggestions"] == []


def test_skill_bench_cli_emits_status_line(tmp_path: Path):
    """End-to-end: `logmind skill bench <name>` exits 0 + prints status."""
    skill_dir = tmp_path / ".claude" / "skills" / "my-skill"
    skill_dir.mkdir(parents=True)
    (skill_dir / "SKILL.md").write_text(
        "---\nname: my-skill\ndescription: trigger\n---\n# My\n## When\n- a\n",
        encoding="utf-8",
    )
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=str(tmp_path)) as fs:
        # Re-create the skill under the isolated cwd.
        from pathlib import Path as _P
        target = _P(fs) / ".claude" / "skills" / "my-skill"
        target.mkdir(parents=True)
        (target / "SKILL.md").write_text(
            "---\nname: my-skill\ndescription: trigger\n---\n# My\n## When\n- a\n",
            encoding="utf-8",
        )
        result = runner.invoke(cli_main, ["skill", "bench", "my-skill"])
    assert result.exit_code == 0, result.output
    assert "my-skill" in result.output
    assert "tight" in result.output or "typical" in result.output


def test_skill_bench_cli_json(tmp_path: Path):
    """--json emits a parseable JSON object."""
    import json
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=str(tmp_path)) as fs:
        from pathlib import Path as _P
        target = _P(fs) / ".claude" / "skills" / "x"
        target.mkdir(parents=True)
        (target / "SKILL.md").write_text(
            "---\nname: x\ndescription: y\n---\n# X\n## Y\nz\n",
            encoding="utf-8",
        )
        result = runner.invoke(cli_main, ["skill", "bench", "x", "--json"])
    assert result.exit_code == 0, result.output
    data = json.loads(result.output.split("ok ")[0].strip())
    assert "bytes" in data and "status" in data and "sections" in data


def test_skill_bench_cli_missing_skill_exits_1(tmp_path: Path):
    """Missing skill = non-zero exit."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=str(tmp_path)):
        result = runner.invoke(cli_main, ["skill", "bench", "does-not-exist"])
    assert result.exit_code == 1
    assert "not found" in result.output


# ---------------------------------------------------------------------------
# v0.6.4 — audit_skills / classify_audit_row / `logmind skill audit` CLI
# ---------------------------------------------------------------------------


def _audit_helpers():
    from logmind.core.skill_cli import audit_skills, classify_audit_row
    return audit_skills, classify_audit_row


def _git_init_with_skill(tmp_path: Path, name: str, content: str) -> Path:
    """Helper: init a git repo with one SKILL.md committed."""
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=tmp_path, check=True)
    subprocess.run(["git", "config", "user.email", "t@t"], cwd=tmp_path, check=True)
    subprocess.run(["git", "config", "user.name", "t"], cwd=tmp_path, check=True)
    subprocess.run(["git", "config", "commit.gpgsign", "false"], cwd=tmp_path, check=True)
    skill_dir = tmp_path / ".claude" / "skills" / name
    skill_dir.mkdir(parents=True)
    (skill_dir / "SKILL.md").write_text(content, encoding="utf-8")
    subprocess.run(["git", "add", "."], cwd=tmp_path, check=True)
    subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=tmp_path, check=True)
    return skill_dir / "SKILL.md"


def test_audit_skills_no_directory_returns_empty(tmp_path: Path):
    audit_skills, _ = _audit_helpers()
    out = audit_skills(tmp_path)
    assert out == []


def test_audit_skills_empty_directory_returns_empty(tmp_path: Path):
    (tmp_path / ".claude" / "skills").mkdir(parents=True)
    audit_skills, _ = _audit_helpers()
    assert audit_skills(tmp_path) == []


def test_audit_skills_reports_each_skill(tmp_path: Path):
    audit_skills, _ = _audit_helpers()
    _git_init_with_skill(
        tmp_path, "alpha", "---\nname: alpha\ndescription: a\n---\nbody\n"
    )
    second = tmp_path / ".claude" / "skills" / "beta"
    second.mkdir(parents=True)
    (second / "SKILL.md").write_text(
        "---\nname: beta\ndescription: b\n---\nbody\n", encoding="utf-8"
    )
    out = audit_skills(tmp_path)
    names = sorted(r["name"] for r in out)
    assert names == ["alpha", "beta"]
    for row in out:
        assert "bytes" in row
        assert "last_modified" in row
        assert "decision_count" in row
        assert row["bytes"] > 0


def test_audit_skills_counts_decision_mentions(tmp_path: Path):
    """Skill name appearing in docs/decisions.md → decision_count > 0."""
    audit_skills, _ = _audit_helpers()
    _git_init_with_skill(
        tmp_path, "gamma", "---\nname: gamma\ndescription: g\n---\nbody\n"
    )
    (tmp_path / "docs").mkdir()
    (tmp_path / "docs" / "decisions.md").write_text(
        "## Decision 1\nUsed gamma for X.\n\n## Decision 2\ngamma iterated.\n",
        encoding="utf-8",
    )
    out = audit_skills(tmp_path)
    row = next(r for r in out if r["name"] == "gamma")
    assert row["decision_count"] == 2


def test_audit_skills_includes_decision_branches(tmp_path: Path):
    """Branch decision files under docs/decisions-branches/ also counted."""
    audit_skills, _ = _audit_helpers()
    _git_init_with_skill(
        tmp_path, "delta", "---\nname: delta\ndescription: d\n---\nbody\n"
    )
    branches = tmp_path / "docs" / "decisions-branches"
    branches.mkdir(parents=True)
    (branches / "feat__x.md").write_text("delta was extended.\n", encoding="utf-8")
    (branches / "feat__y.md").write_text("delta refactored.\n", encoding="utf-8")
    out = audit_skills(tmp_path)
    row = next(r for r in out if r["name"] == "delta")
    assert row["decision_count"] == 2


def test_classify_audit_row_active():
    _, classify_audit_row = _audit_helpers()
    import datetime as _dt
    today = _dt.date(2026, 6, 1)
    row = {"bytes": 1500, "decision_count": 2, "last_modified": "2026-05-15"}
    assert classify_audit_row(row, now=today) == "active"


def test_classify_audit_row_ghost():
    """Big skill with no decision-log mentions = ghost."""
    _, classify_audit_row = _audit_helpers()
    import datetime as _dt
    today = _dt.date(2026, 6, 1)
    row = {"bytes": 5000, "decision_count": 0, "last_modified": "2026-05-15"}
    assert classify_audit_row(row, now=today) == "ghost"


def test_classify_audit_row_aging():
    """Last touched > 90 days = aging (even with decisions + small)."""
    _, classify_audit_row = _audit_helpers()
    import datetime as _dt
    today = _dt.date(2026, 6, 1)
    row = {"bytes": 500, "decision_count": 5, "last_modified": "2026-01-01"}
    assert classify_audit_row(row, now=today) == "aging"


def test_classify_audit_row_small_no_decisions_is_active():
    """Tight skill with no decisions = active (could be new or just simple)."""
    _, classify_audit_row = _audit_helpers()
    import datetime as _dt
    today = _dt.date(2026, 6, 1)
    row = {"bytes": 500, "decision_count": 0, "last_modified": "2026-05-30"}
    assert classify_audit_row(row, now=today) == "active"


def test_classify_audit_row_handles_invalid_date():
    _, classify_audit_row = _audit_helpers()
    row = {"bytes": 500, "decision_count": 1, "last_modified": "not-a-date"}
    assert classify_audit_row(row) == "active"


def test_skill_audit_cli_no_skills(tmp_path: Path):
    """No .claude/skills/ → friendly message + exit 0."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=str(tmp_path)):
        result = runner.invoke(cli_main, ["skill", "audit"])
    assert result.exit_code == 0
    assert "No skills found" in result.output


def test_skill_audit_cli_renders_table(tmp_path: Path):
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=str(tmp_path)) as fs:
        from pathlib import Path as _P
        repo = _P(fs)
        _git_init_with_skill(
            repo, "epsilon", "---\nname: epsilon\ndescription: e\n---\nbody\n"
        )
        result = runner.invoke(cli_main, ["skill", "audit"])
    assert result.exit_code == 0, result.output
    assert "epsilon" in result.output
    assert "skill: audit 1 skill" in result.output


def test_skill_audit_cli_json(tmp_path: Path):
    """--json emits a parseable JSON array."""
    import json
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=str(tmp_path)) as fs:
        from pathlib import Path as _P
        repo = _P(fs)
        _git_init_with_skill(
            repo, "zeta", "---\nname: zeta\ndescription: z\n---\nbody\n"
        )
        result = runner.invoke(cli_main, ["skill", "audit", "--json"])
    assert result.exit_code == 0, result.output
    payload = result.output.split("ok ")[0].strip()
    data = json.loads(payload)
    assert isinstance(data, list)
    assert len(data) == 1
    assert data[0]["name"] == "zeta"
    assert "status" in data[0]
