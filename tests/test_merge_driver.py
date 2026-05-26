"""Tests for v0.3.0's git merge driver for derived files."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest
from click.testing import CliRunner

from logmind.cli import init, main as cli_main
from logmind.core import doctor
from logmind.core.gitattributes import (
    LOGMIND_GITATTRIBUTES_START,
    MERGE_DRIVER_CONFIG,
    configure_merge_drivers,
    driver_configured,
    ensure_block,
    has_block,
    install_post_merge_hook,
    post_merge_hook_installed,
)


# ---------------------------------------------------------------------------
# .gitattributes block (committed)
# ---------------------------------------------------------------------------


def test_ensure_block_creates_file_with_marker(tmp_path: Path):
    gitattrs = tmp_path / ".gitattributes"
    changed = ensure_block(gitattrs)
    assert changed is True
    assert gitattrs.exists()
    text = gitattrs.read_text(encoding="utf-8")
    assert LOGMIND_GITATTRIBUTES_START in text
    assert "docs/timeline.md" in text
    assert "merge=logmind-timeline" in text


def test_ensure_block_is_idempotent(tmp_path: Path):
    gitattrs = tmp_path / ".gitattributes"
    ensure_block(gitattrs)
    second = ensure_block(gitattrs)
    assert second is False  # no-op on second run


def test_ensure_block_appends_to_existing_file(tmp_path: Path):
    gitattrs = tmp_path / ".gitattributes"
    gitattrs.write_text("*.lock binary\n", encoding="utf-8")
    changed = ensure_block(gitattrs)
    assert changed is True
    text = gitattrs.read_text(encoding="utf-8")
    assert "*.lock binary" in text  # preserved
    assert LOGMIND_GITATTRIBUTES_START in text


# ---------------------------------------------------------------------------
# git config per-clone driver setup
# ---------------------------------------------------------------------------


@pytest.fixture
def git_repo(tmp_path: Path) -> Path:
    """Bare git repo for testing per-clone config interactions."""
    subprocess.run(["git", "init", "-q"], cwd=tmp_path, check=True)
    subprocess.run(
        ["git", "config", "user.email", "t@t.com"], cwd=tmp_path, check=True
    )
    subprocess.run(["git", "config", "user.name", "t"], cwd=tmp_path, check=True)
    return tmp_path


def test_configure_merge_drivers_sets_all_keys(git_repo: Path):
    changed = configure_merge_drivers(git_repo)
    assert changed is True
    # Verify every expected key was set
    for key, expected_value in MERGE_DRIVER_CONFIG:
        result = subprocess.run(
            ["git", "-C", str(git_repo), "config", "--get", key],
            capture_output=True, text=True, check=True,
        )
        assert result.stdout.strip() == expected_value


def test_configure_merge_drivers_is_idempotent(git_repo: Path):
    configure_merge_drivers(git_repo)
    second = configure_merge_drivers(git_repo)
    assert second is False  # no-op when values already match


def test_configure_merge_drivers_noop_outside_git_repo(tmp_path: Path):
    """Not a git checkout (no .git dir) — silent no-op, no crash."""
    assert configure_merge_drivers(tmp_path) is False


def test_driver_configured_returns_false_when_unset(git_repo: Path):
    assert driver_configured(git_repo) is False


def test_driver_configured_returns_true_after_configure(git_repo: Path):
    configure_merge_drivers(git_repo)
    assert driver_configured(git_repo) is True


# ---------------------------------------------------------------------------
# logmind init wires up both
# ---------------------------------------------------------------------------


def test_init_writes_gitattributes_and_configures_drivers(git_repo: Path):
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        cwd = Path.cwd()
        subprocess.run(["git", "init", "-q"], cwd=cwd, check=True)
        subprocess.run(["git", "config", "user.email", "t@t.com"], cwd=cwd, check=True)
        subprocess.run(["git", "config", "user.name", "t"], cwd=cwd, check=True)
        # Initial commit so git considers it a real repo
        (cwd / "README.md").write_text("x", encoding="utf-8")
        subprocess.run(["git", "add", "README.md"], cwd=cwd, check=True)
        subprocess.run(
            ["git", "commit", "-qm", "init"], cwd=cwd, check=True
        )

        result = runner.invoke(init, ["--no-skill-install"])
        assert result.exit_code == 0, result.output

        # .gitattributes carries the block
        assert has_block(cwd / ".gitattributes")
        # Per-clone config has the driver definitions
        assert driver_configured(cwd)


# ---------------------------------------------------------------------------
# logmind file-structure --write CLI (mirror of timeline --write)
# ---------------------------------------------------------------------------


def test_file_structure_write_creates_output_at_arbitrary_path(git_repo: Path):
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        cwd = Path.cwd()
        (cwd / "some_file.txt").write_text("x", encoding="utf-8")
        target = cwd / "scratch" / "tree.md"
        result = runner.invoke(cli_main, ["file-structure", "--write", str(target)])
        assert result.exit_code == 0, result.output
        assert target.exists()
        content = target.read_text(encoding="utf-8")
        assert "some_file.txt" in content


def test_file_structure_write_creates_output_when_missing(git_repo: Path):
    """The merge driver calls file-structure --write %A on a path git
    chose. The command must create the file even when the parent dir
    doesn't exist yet — git's merge target path is in the worktree,
    parent dirs are guaranteed, but we're defensive."""
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=git_repo):
        cwd = Path.cwd()
        (cwd / "some_file.txt").write_text("x", encoding="utf-8")
        target = cwd / "nested" / "deeper" / "tree.md"
        result = runner.invoke(cli_main, ["file-structure", "--write", str(target)])
        assert result.exit_code == 0, result.output
        assert target.exists()
        assert "Regenerated" in result.output

    # NB: file-structure.md content includes a timestamp, so we don't
    # assert byte-stable idempotency — `write_file_structure` returns
    # True every call because the timestamp moves. That's fine for the
    # merge-driver path: git already detected a conflict and accepts
    # whatever the driver writes.


# ---------------------------------------------------------------------------
# doctor surfaces the new merge-driver drift signals
# ---------------------------------------------------------------------------


def test_doctor_reports_missing_gitattributes_block(tmp_path: Path):
    """Fresh dir with no .gitattributes: doctor reports MISSING (not
    stale — binary present/absent, absence isn't drift, just 'not yet
    installed for this version of logmind')."""
    (tmp_path / ".github" / "workflows").mkdir(parents=True)
    report = doctor.collect_status(tmp_path, offline=True)
    ga_row = next(
        w for w in report.tools[0].workflows if "merge driver" in w.name and ".gitattributes" in w.name
    )
    assert ga_row.drift == "missing"


def test_doctor_reports_missing_git_config(tmp_path: Path):
    """In a git repo without merge-driver config: doctor reports MISSING."""
    subprocess.run(["git", "init", "-q"], cwd=tmp_path, check=True)
    (tmp_path / ".github" / "workflows").mkdir(parents=True)
    report = doctor.collect_status(tmp_path, offline=True)
    cfg_row = next(
        w for w in report.tools[0].workflows if "git config" in w.name
    )
    assert cfg_row.drift == "missing"


def test_doctor_reports_current_after_full_install(tmp_path: Path):
    """Both .gitattributes and git config set → both rows show current."""
    subprocess.run(["git", "init", "-q"], cwd=tmp_path, check=True)
    ensure_block(tmp_path / ".gitattributes")
    configure_merge_drivers(tmp_path)
    install_post_merge_hook(tmp_path)
    (tmp_path / ".github" / "workflows").mkdir(parents=True)
    report = doctor.collect_status(tmp_path, offline=True)
    ga = next(w for w in report.tools[0].workflows if ".gitattributes" in w.name)
    cfg = next(w for w in report.tools[0].workflows if "git config" in w.name)
    hook = next(w for w in report.tools[0].workflows if "post-merge" in w.name)
    assert ga.drift == "current"
    assert cfg.drift == "current"
    assert hook.drift == "current"


# ---------------------------------------------------------------------------
# post-merge hook
# ---------------------------------------------------------------------------


def test_install_post_merge_hook_creates_executable_hook(git_repo: Path):
    """Hook is written under .git/hooks/ and is executable."""
    changed = install_post_merge_hook(git_repo)
    assert changed is True
    hook = git_repo / ".git" / "hooks" / "post-merge"
    assert hook.exists()
    assert hook.stat().st_mode & 0o111  # any execute bit set
    body = hook.read_text(encoding="utf-8")
    assert "logmind timeline --write" in body
    assert "logmind file-structure --write" in body


def test_install_post_merge_hook_does_not_overwrite_foreign_hook(git_repo: Path):
    """If a non-logmind hook is already in place, leave it. The user's
    custom hook isn't our problem to inherit."""
    hook = git_repo / ".git" / "hooks" / "post-merge"
    hook.parent.mkdir(parents=True, exist_ok=True)
    hook.write_text("#!/bin/sh\necho 'custom user hook'\n", encoding="utf-8")
    hook.chmod(0o755)
    changed = install_post_merge_hook(git_repo)
    assert changed is False
    assert "custom user hook" in hook.read_text(encoding="utf-8")


def test_post_merge_hook_installed_detects_logmind_hook(git_repo: Path):
    install_post_merge_hook(git_repo)
    assert post_merge_hook_installed(git_repo) is True


def test_post_merge_hook_installed_returns_false_for_foreign_hook(git_repo: Path):
    hook = git_repo / ".git" / "hooks" / "post-merge"
    hook.parent.mkdir(parents=True, exist_ok=True)
    hook.write_text("#!/bin/sh\necho 'custom'\n", encoding="utf-8")
    assert post_merge_hook_installed(git_repo) is False
