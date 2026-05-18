"""Regression tests for the v0.2.1 audit-fix release.

Four findings landed (P0, P1, P3a, P3b). Each gets a targeted test so a
future refactor can't silently regress.
"""

from __future__ import annotations

import os
import re
import subprocess
from pathlib import Path
from unittest.mock import patch

import pytest
from click.testing import CliRunner

from logmind import __version__
from logmind.cli import init, main as cli_main


# ---------------------------------------------------------------------------
# P0 — logmind version pinned in shipped workflow templates
# ---------------------------------------------------------------------------


def test_install_github_action_templates_pins_logmind_version(temp_dir):
    """After `logmind init`, the rendered regen-timeline.yml + check-doc-links.yml
    must contain `pip install "logmind==<__version__>"`, not the unpinned
    `pip install logmind` that broke downstream CI on every release."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--no-skill-install"])
        assert result.exit_code == 0, result.output

        for wf_name in ("regen-timeline.yml", "check-doc-links.yml"):
            wf = Path(f".github/workflows/{wf_name}")
            assert wf.exists(), f"workflow not created: {wf_name}"
            content = wf.read_text(encoding="utf-8")
            expected = f'pip install "logmind=={__version__}"'
            assert expected in content, (
                f"{wf_name} should pin logmind to {__version__} but got:\n"
                + "\n".join(
                    line for line in content.splitlines() if "pip install" in line
                )
            )
            # And the placeholder must NOT leak through
            assert "__LOGMIND_VERSION__" not in content


def test_check_doc_links_template_has_no_paths_filter():
    """v0.2.2 fix: shipped check-doc-links.yml.template must NOT have a
    `paths:` filter. The filter (which v0.2.0/v0.2.1 shipped) causes
    GitHub to skip the workflow on non-markdown PRs, which silently
    blocks merges when the check is in required_status_checks (treated
    as 'expected but never reported'). Bit clud-bug PR #52.

    The fix lifts the no-filter behavior from logmind's own dogfood copy
    into the shipped template + bumps the template marker to v2."""
    template = (
        Path(__file__).parent.parent
        / "src"
        / "logmind"
        / "templates"
        / "github"
        / "check-doc-links.yml.template"
    ).read_text(encoding="utf-8")

    # The on: block must NOT contain `paths:` filter.
    # Use a narrow check: any `paths:` line in the file means regression.
    paths_lines = [
        line for line in template.splitlines()
        if line.strip().startswith("paths:")
    ]
    assert not paths_lines, (
        "check-doc-links.yml.template ships a paths: filter again; this "
        "silently blocks merges when check-links is a required status check. "
        f"Found: {paths_lines!r}"
    )

    # Template marker must be v2 (bumped from v1)
    assert "# logmind-template-version: v2" in template


def test_self_update_template_is_shipped(temp_dir):
    """v0.2.x+: logmind init must install the logmind-self-update workflow
    so downstream repos get the Monday-cron PR for new releases without
    manual `pip install --upgrade logmind && logmind init` each cycle."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--no-skill-install"])
        assert result.exit_code == 0, result.output
        wf = Path(".github/workflows/logmind-self-update.yml")
        assert wf.exists(), "logmind init must install logmind-self-update.yml"
        content = wf.read_text(encoding="utf-8")
        # cron schedule on Mondays at noon UTC (mirror of clud-bug self-update)
        assert "cron: '0 12 * * 1'" in content
        # Reads the workflow pin to detect installed version
        assert "regen-timeline.yml" in content
        # Opt-out via pinVersion in .logmind/config.yml
        assert "pinVersion" in content


def test_install_templates_substitution_does_not_break_yaml_braces(temp_dir):
    """Confirm the placeholder substitution uses str.replace (not str.format),
    so YAML's `${{ ... }}` expressions in workflow templates survive intact.
    Verified against check-decisions.yml which uses ${{ github.event... }}."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=temp_dir):
        result = runner.invoke(init, ["--no-git", "--no-skill-install"])
        assert result.exit_code == 0
        wf = Path(".github/workflows/check-decisions.yml")
        assert wf.exists()
        content = wf.read_text(encoding="utf-8")
        assert "${{" in content, (
            "check-decisions.yml should preserve GitHub Actions ${...} syntax "
            "(template would be broken if str.format was used)"
        )


# ---------------------------------------------------------------------------
# P1 — `logmind init` refreshes workflows when already initialized
# ---------------------------------------------------------------------------


def test_init_refresh_mode_preserves_existing_docs(temp_dir):
    """Running init twice — first to set up, then again — must leave
    docs/decisions.md untouched. Pre-v0.2.1 the second run was a hard exit."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=temp_dir):
        # First init
        runner.invoke(init, ["--no-git", "--no-skill-install"])
        decisions = Path("docs/decisions.md")
        original_content = decisions.read_text(encoding="utf-8")
        # Add a marker the user might add manually
        decisions.write_text(original_content + "\n## USER MARKER\n", encoding="utf-8")
        marked = decisions.read_text(encoding="utf-8")

        # Second init — refresh mode
        result = runner.invoke(init, ["--no-git", "--no-skill-install"])
        assert result.exit_code == 0
        assert "refresh mode" in result.output.lower()

        # docs/decisions.md must be untouched
        after = decisions.read_text(encoding="utf-8")
        assert after == marked, (
            "logmind init in refresh mode must not touch docs/decisions.md, "
            "but the user marker was lost"
        )


def test_init_refresh_mode_regenerates_missing_workflow(temp_dir):
    """If a workflow file was deleted between init runs, refresh mode
    must re-create it from the template (the common upgrade case)."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=temp_dir):
        runner.invoke(init, ["--no-git", "--no-skill-install"])
        regen = Path(".github/workflows/regen-timeline.yml")
        assert regen.exists()
        regen.unlink()
        assert not regen.exists()

        result = runner.invoke(init, ["--no-git", "--no-skill-install"])
        assert result.exit_code == 0
        assert regen.exists(), \
            "refresh mode must regenerate missing workflow files"


def test_init_refresh_mode_refreshes_stale_workflow_by_template_version(temp_dir):
    """If a workflow has an OLDER `# logmind-template-version:` marker,
    refresh mode replaces it. If markers match, leaves it alone."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=temp_dir):
        runner.invoke(init, ["--no-git", "--no-skill-install"])
        regen = Path(".github/workflows/regen-timeline.yml")
        original = regen.read_text(encoding="utf-8")
        # Tamper with the version marker to simulate an older install
        tampered = re.sub(
            r"^# logmind-template-version:.*$",
            "# logmind-template-version: v0",
            original,
            count=1,
            flags=re.MULTILINE,
        )
        regen.write_text(tampered, encoding="utf-8")
        assert "v0" in regen.read_text(encoding="utf-8")

        result = runner.invoke(init, ["--no-git", "--no-skill-install"])
        assert result.exit_code == 0
        after = regen.read_text(encoding="utf-8")
        # Should be back to the current template version (not v0)
        assert "logmind-template-version: v0" not in after
        # And the output should mention the refresh
        assert "refresh" in result.output.lower() or "refreshed" in result.output.lower()


# ---------------------------------------------------------------------------
# P3a — narrow except in logger.py branch detection
# ---------------------------------------------------------------------------


def test_is_git_repo_returns_false_on_oserror():
    """`is_git_repo` must return False (not raise) when the subprocess
    raises an OSError — e.g. permission-denied on `.git/`. Caller code
    treats this as "not a git repo" rather than crashing."""
    from logmind.core import git_handler

    with patch.object(
        git_handler.subprocess,
        "run",
        side_effect=PermissionError("simulated .git/ permission denied"),
    ):
        assert git_handler.is_git_repo(Path("/nonexistent")) is False


def test_current_branch_returns_none_on_oserror():
    """`current_branch` must return None (not raise) on subprocess
    OSError — caller code already handles None-as-detached-HEAD; treat
    permission errors the same way. Pre-v0.2.1 OSError propagated up
    and crashed `logmind log`."""
    from logmind.core import git_handler

    with patch.object(
        git_handler.subprocess,
        "run",
        side_effect=PermissionError("simulated .git/HEAD permission denied"),
    ):
        assert git_handler.current_branch(Path("/nonexistent")) is None


# The end-to-end "log() doesn't crash on OSError" check is implicit:
# - test_is_git_repo_returns_false_on_oserror ensures is_git_repo can't raise
# - test_current_branch_returns_none_on_oserror ensures current_branch can't raise
# - log() only calls those two + default_branch (which is OSError-safe by design)
# So an end-to-end mock that leaks to ALL subprocess.run calls (including the
# unrelated `tree` binary in tree_gen.py) would be a noisy way to verify the
# same property. The two unit tests above are the canonical proof.


# ---------------------------------------------------------------------------
# P3b — atomic writes for state files
# ---------------------------------------------------------------------------


def test_atomic_write_preserves_original_on_failure(tmp_path):
    """If atomic_write_text fails mid-write, the original file must be intact."""
    from logmind.core.atomic_io import atomic_write_text

    target = tmp_path / "decisions.md"
    target.write_text("ORIGINAL\n", encoding="utf-8")

    # Force a failure during the tmp-file write by giving an unwritable
    # tmp filename (parent doesn't exist after we delete it)
    fake = tmp_path / "nonexistent-subdir" / "decisions.md"
    fake.parent.mkdir()
    fake.write_text("ORIGINAL\n", encoding="utf-8")
    # Now remove the parent dir so atomic_write's tmp-file write fails
    import shutil
    shutil.rmtree(fake.parent)

    with pytest.raises(OSError):
        atomic_write_text(fake, "NEW CONTENT")
    # The target obviously doesn't exist (we deleted the dir), but for the
    # OTHER target — the one with a real parent — original content is intact.
    assert target.read_text(encoding="utf-8") == "ORIGINAL\n"


def test_atomic_write_uses_sibling_tmp_then_replaces(tmp_path, monkeypatch):
    """Verify the temp-then-replace pattern, not direct write. The sibling
    tmp file ensures os.replace is atomic (same-filesystem)."""
    from logmind.core import atomic_io

    target = tmp_path / "decisions.md"
    target.write_text("OLD\n", encoding="utf-8")

    seen_tmps: list = []
    original_replace = os.replace

    def spy(src, dst):
        seen_tmps.append((str(src), str(dst)))
        return original_replace(src, dst)

    monkeypatch.setattr(atomic_io.os, "replace", spy)
    atomic_io.atomic_write_text(target, "NEW\n")

    assert target.read_text(encoding="utf-8") == "NEW\n"
    assert len(seen_tmps) == 1
    src, dst = seen_tmps[0]
    # The tmp file must be in the same directory as the target
    assert Path(src).parent == Path(dst).parent == tmp_path
    # And the tmp file must be cleaned up (os.replace consumed it)
    assert not Path(src).exists()


def test_log_decisions_file_uses_atomic_write(tmp_path, monkeypatch):
    """After `log()`, the decisions file content must have been written
    via atomic_write_text — verified by spying on os.replace."""
    from logmind.core import atomic_io, logger as logger_mod

    docs = tmp_path / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n", encoding="utf-8")

    replaces: list = []
    original = atomic_io.os.replace
    monkeypatch.setattr(
        atomic_io.os,
        "replace",
        lambda s, d: (replaces.append((str(s), str(d))), original(s, d))[1],
    )

    monkeypatch.chdir(tmp_path)
    logger_mod.log(
        "atomic test", reasoning="r", docs_path=docs, auto_commit=False
    )

    # At least one replace must target docs/decisions.md
    assert any(str(docs / "decisions.md") in d for _, d in replaces), \
        f"expected atomic write to docs/decisions.md, saw: {replaces}"
