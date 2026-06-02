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
    install_post_rewrite_hook,
    post_merge_hook_installed,
    post_rewrite_hook_installed,
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
    """Hook is written under .git/hooks/ and (on POSIX) executable."""
    import os
    changed = install_post_merge_hook(git_repo)
    assert changed is True
    hook = git_repo / ".git" / "hooks" / "post-merge"
    assert hook.exists()
    # On Windows, `chmod(0o755)` doesn't translate to Unix execute bits;
    # git for Windows runs hooks via the shell directly without checking
    # file mode. So we only assert the bit on POSIX systems.
    if os.name != "nt":
        assert hook.stat().st_mode & 0o111  # any execute bit set
    body = hook.read_text(encoding="utf-8")
    assert "logmind timeline --write" in body
    assert "logmind file-structure --write" in body


def test_post_merge_hook_does_not_stage_derived_docs(git_repo: Path):
    """v0.6.7 regression guard: the hook must regenerate but NOT stage.

    Background: pre-v0.6.7 hook ran `git add docs/timeline.md
    docs/file-structure.md` after the regen. The staged-but-uncommitted
    files then blocked `git checkout main` on every PR cycle (a
    downstream agent reported the friction). Removing the auto-stage
    fixes the bug; this test prevents accidental re-addition.

    Anchored regex: match `git add docs/timeline.md` only on a line
    where it's the actual command (start-of-line + optional whitespace),
    not inside a comment. Mirrors clud-bug's v0.6.32 release-discipline
    pattern.
    """
    import re
    install_post_merge_hook(git_repo)
    hook = git_repo / ".git" / "hooks" / "post-merge"
    body = hook.read_text(encoding="utf-8")
    forbidden = re.search(
        r"^\s*git add docs/timeline\.md", body, re.MULTILINE,
    )
    assert forbidden is None, (
        "post-merge hook must NOT auto-stage docs/timeline.md.\n"
        "Background: the staged-but-uncommitted files block `git "
        "checkout main` on every PR cycle. See v0.6.7 CHANGELOG + the "
        "downstream bug report. Regen should leave files unstaged so "
        "the next `logmind log` (or explicit `git add`) picks them up "
        "cleanly without blocking branch switches.\n"
        f"Found forbidden line: {forbidden.group(0) if forbidden else '(none)'}"
    )
    # Sibling assertion for docs/file-structure.md — same contract.
    forbidden_fs = re.search(
        r"^\s*git add docs/file-structure\.md", body, re.MULTILINE,
    )
    assert forbidden_fs is None, (
        "post-merge hook must NOT auto-stage docs/file-structure.md either."
    )


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


# ---------------------------------------------------------------------------
# v0.5.11 / issue #58 — post-rewrite hook
#
# Companion to post-merge. Covers `git rebase` and `git commit --amend`
# (which the merge driver + post-merge hook don't cover). Hit live on
# agent-skills PRs #21 + #22 during the 2026-05-27 merge cascade —
# multi-commit rebases left docs/timeline.md stale, failing
# check-derived-docs.
# ---------------------------------------------------------------------------


def test_install_post_rewrite_hook_creates_executable_hook(git_repo: Path):
    """v0.5.11 / #58: hook is written under .git/hooks/ and (on POSIX)
    executable, mirroring the post-merge hook pattern."""
    import os
    changed = install_post_rewrite_hook(git_repo)
    assert changed is True
    hook = git_repo / ".git" / "hooks" / "post-rewrite"
    assert hook.exists()
    if os.name != "nt":
        assert hook.stat().st_mode & 0o111
    body = hook.read_text(encoding="utf-8")
    assert "logmind timeline --write" in body
    assert "logmind file-structure --write" in body


def test_install_post_rewrite_hook_is_idempotent(git_repo: Path):
    """v0.5.11 / #58: second invocation is a no-op when the canonical
    body is already in place."""
    install_post_rewrite_hook(git_repo)
    second = install_post_rewrite_hook(git_repo)
    assert second is False


def test_install_post_rewrite_hook_does_not_overwrite_foreign_hook(git_repo: Path):
    """v0.5.11 / #58: a user-authored post-rewrite hook is preserved
    untouched — same contract as post-merge."""
    hook = git_repo / ".git" / "hooks" / "post-rewrite"
    hook.parent.mkdir(parents=True, exist_ok=True)
    hook.write_text("#!/bin/sh\necho 'custom rewrite hook'\n", encoding="utf-8")
    hook.chmod(0o755)
    changed = install_post_rewrite_hook(git_repo)
    assert changed is False
    assert "custom rewrite hook" in hook.read_text(encoding="utf-8")


def test_post_rewrite_hook_installed_detects_logmind_hook(git_repo: Path):
    install_post_rewrite_hook(git_repo)
    assert post_rewrite_hook_installed(git_repo) is True


def test_post_rewrite_hook_installed_returns_false_for_foreign_hook(git_repo: Path):
    hook = git_repo / ".git" / "hooks" / "post-rewrite"
    hook.parent.mkdir(parents=True, exist_ok=True)
    hook.write_text("#!/bin/sh\necho 'custom'\n", encoding="utf-8")
    assert post_rewrite_hook_installed(git_repo) is False


def test_doctor_surfaces_post_rewrite_hook_status(git_repo: Path):
    """v0.5.11 / #58: `logmind doctor` reports post-rewrite hook
    drift the same way it reports post-merge drift."""
    # Before install: missing
    report = doctor.collect_status(git_repo, offline=True)
    hook_status = next(
        w for w in report.tools[0].workflows if "post-rewrite" in w.name
    )
    assert hook_status.drift == "missing"

    # After install: current
    install_post_rewrite_hook(git_repo)
    report = doctor.collect_status(git_repo, offline=True)
    hook_status = next(
        w for w in report.tools[0].workflows if "post-rewrite" in w.name
    )
    assert hook_status.drift == "current"


def test_logmind_init_installs_post_rewrite_hook(git_repo: Path, monkeypatch):
    """v0.5.11 / #58: `logmind init` invokes install_post_rewrite_hook
    so a fresh project picks up the hook automatically — closes the
    loop with the post-merge hook that's also init-installed."""
    monkeypatch.chdir(git_repo)
    runner = CliRunner()
    result = runner.invoke(cli_main, ["init", "--no-skill-install"])
    assert result.exit_code == 0, result.output
    hook = git_repo / ".git" / "hooks" / "post-rewrite"
    assert hook.exists(), (
        f"v0.5.11 / #58: logmind init must install post-rewrite hook. "
        f"init output:\n{result.output}"
    )
    assert "logmind timeline --write" in hook.read_text(encoding="utf-8")


# ---------------------------------------------------------------------------
# v0.6.10 — hook-version drift detection (tokenomics 2026-06-01 bug recurrence)
#
# Root cause we want surfaced loudly: the local CLI binary writes the hook
# body, so if the binary is stale relative to the workflow's pinned version,
# the hook on disk is stale too. doctor must report this as drift so the
# user knows to upgrade + refresh.
# ---------------------------------------------------------------------------


def test_post_merge_hook_embeds_logmind_version_marker(git_repo: Path):
    """v0.6.10: the installed hook body MUST embed a `# logmind-hook-version: …`
    line so doctor can detect drift between the binary that wrote it and
    the binary running now."""
    from logmind import __version__ as current_version
    from logmind.core.gitattributes import (
        HOOK_VERSION_PREFIX,
        install_post_merge_hook,
    )

    install_post_merge_hook(git_repo)
    body = (git_repo / ".git" / "hooks" / "post-merge").read_text(
        encoding="utf-8"
    )
    assert HOOK_VERSION_PREFIX in body, (
        "v0.6.10: post-merge hook must embed the version marker prefix."
    )
    assert f"{HOOK_VERSION_PREFIX}{current_version}" in body, (
        f"v0.6.10: marker must include the CURRENT logmind version "
        f"({current_version}), not a stale one."
    )


def test_post_rewrite_hook_embeds_logmind_version_marker(git_repo: Path):
    """Same as the post-merge test — companion hook."""
    from logmind import __version__ as current_version
    from logmind.core.gitattributes import (
        HOOK_VERSION_PREFIX,
        install_post_rewrite_hook,
    )

    install_post_rewrite_hook(git_repo)
    body = (git_repo / ".git" / "hooks" / "post-rewrite").read_text(
        encoding="utf-8"
    )
    assert HOOK_VERSION_PREFIX in body
    assert f"{HOOK_VERSION_PREFIX}{current_version}" in body


def test_installed_post_merge_hook_version_extracts_marker(git_repo: Path):
    """`installed_post_merge_hook_version` returns the embedded version."""
    from logmind import __version__ as current_version
    from logmind.core.gitattributes import (
        install_post_merge_hook,
        installed_post_merge_hook_version,
    )

    assert installed_post_merge_hook_version(git_repo) is None  # No hook yet.
    install_post_merge_hook(git_repo)
    assert installed_post_merge_hook_version(git_repo) == current_version


def test_doctor_reports_post_merge_hook_drift_when_binary_newer_than_hook(
    git_repo: Path,
):
    """The tokenomics 2026-06-01 case in miniature: a v0.3.4 binary wrote
    a stale hook (no marker line at all), then the user upgraded to v0.6.10.
    Doctor must surface this as drift, not report 'current'."""
    from logmind.core import doctor
    from logmind.core.gitattributes import _POST_MERGE_HOOK_MARKER

    # Plant a pre-v0.6.10 hook (markerless, but with our hook marker so
    # we know it's a logmind hook, not a foreign one).
    hooks_dir = git_repo / ".git" / "hooks"
    hooks_dir.mkdir(parents=True, exist_ok=True)
    legacy_body = (
        "#!/bin/sh\n"
        f"{_POST_MERGE_HOOK_MARKER}\n"
        "# Installed by `logmind init` (v0.3.0+).\n"
        "logmind timeline --write docs/timeline.md\n"
    )
    legacy_hook = hooks_dir / "post-merge"
    legacy_hook.write_text(legacy_body, encoding="utf-8")
    legacy_hook.chmod(0o755)

    report = doctor.collect_status(git_repo, offline=True)
    hook_status = next(
        w for w in report.tools[0].workflows if "post-merge" in w.name
    )
    assert hook_status.installed is True
    assert hook_status.drift == "markerless", (
        f"Doctor must surface markerless (pre-v0.6.10) hook as drift so "
        f"the user knows to refresh. Got drift={hook_status.drift!r}."
    )


def test_doctor_reports_post_merge_hook_drift_when_version_marker_is_stale(
    git_repo: Path,
):
    """Simulate a hook written by an older v0.6.10+ binary — same prefix
    but an older version embedded. Doctor must flag drift."""
    from logmind.core import doctor
    from logmind.core.gitattributes import (
        HOOK_VERSION_PREFIX,
        _POST_MERGE_HOOK_MARKER,
    )

    hooks_dir = git_repo / ".git" / "hooks"
    hooks_dir.mkdir(parents=True, exist_ok=True)
    stale_body = (
        "#!/bin/sh\n"
        f"{_POST_MERGE_HOOK_MARKER}\n"
        f"{HOOK_VERSION_PREFIX}0.6.7\n"   # Pretend a v0.6.7 binary wrote this.
        "logmind timeline --write docs/timeline.md\n"
    )
    (hooks_dir / "post-merge").write_text(stale_body, encoding="utf-8")
    (hooks_dir / "post-merge").chmod(0o755)

    report = doctor.collect_status(git_repo, offline=True)
    hook_status = next(
        w for w in report.tools[0].workflows if "post-merge" in w.name
    )
    assert hook_status.drift == "stale"
    assert hook_status.marker == "0.6.7"


def test_install_post_merge_hook_refreshes_stale_marker(
    git_repo: Path,
):
    """When `logmind log` (or `init`) runs and the hook on disk has a
    stale version marker, the hook must be rewritten with the current
    binary's body. This is the auto-self-heal path."""
    from logmind import __version__ as current_version
    from logmind.core.gitattributes import (
        HOOK_VERSION_PREFIX,
        _POST_MERGE_HOOK_MARKER,
        install_post_merge_hook,
        installed_post_merge_hook_version,
    )

    hooks_dir = git_repo / ".git" / "hooks"
    hooks_dir.mkdir(parents=True, exist_ok=True)
    stale_body = (
        "#!/bin/sh\n"
        f"{_POST_MERGE_HOOK_MARKER}\n"
        f"{HOOK_VERSION_PREFIX}0.6.7\n"
        "logmind timeline --write docs/timeline.md\n"
    )
    hook = hooks_dir / "post-merge"
    hook.write_text(stale_body, encoding="utf-8")
    hook.chmod(0o755)

    changed = install_post_merge_hook(git_repo)
    assert changed is True
    assert installed_post_merge_hook_version(git_repo) == current_version


# ---------------------------------------------------------------------------
# v0.6.12 — LOGMIND_AUTO_REGEN_PAT secret probe (tokenomics 2026-06-01 PM
# follow-up: PAT was already configured but had insufficient scopes; doctor
# should now surface the dependency proactively.)
# ---------------------------------------------------------------------------


def test_pat_probe_returns_inapplicable_when_no_workflow_needs_it(tmp_path):
    """If a project has no workflow that references the PAT, the probe
    returns an `installed=False` + `marker=None` row that the aggregator
    skips. We don't surface PAT drift on projects that don't need it."""
    from logmind.core.doctor import _probe_auto_regen_pat

    status = _probe_auto_regen_pat(tmp_path)
    assert status.installed is False
    assert status.marker is None


def test_pat_probe_flags_markerless_when_workflow_present_but_no_git_remote(
    tmp_path,
):
    """When a logmind workflow references the secret but the project has
    no git remote (so we can't query secrets), the probe returns
    `drift="markerless"` — informational, not a hard fail."""
    from logmind.core.doctor import _probe_auto_regen_pat

    workflows = tmp_path / ".github" / "workflows"
    workflows.mkdir(parents=True)
    (workflows / "regen-timeline.yml").write_text(
        "name: x\n# uses ${{ secrets.LOGMIND_AUTO_REGEN_PAT }}\n",
        encoding="utf-8",
    )
    status = _probe_auto_regen_pat(tmp_path)
    assert status.installed is False
    assert status.drift == "markerless"
    assert "cannot-verify" in (status.marker or "")


def test_pat_required_scopes_constant_is_complete():
    """The required-scopes string must enumerate Contents / Workflows /
    Pull-requests writes — the exact set the regen-timeline.yml v3 and
    logmind-self-update.yml v5 templates need."""
    from logmind.core.doctor import _PAT_REQUIRED_SCOPES

    s = " ".join(_PAT_REQUIRED_SCOPES).lower()
    assert "contents" in s
    assert "workflows" in s
    assert "pull-requests" in s


def _make_git_repo(tmp_path):
    """Make tmp_path look like a git repo with a github.com origin so the
    PAT probe can run its `gh secret list` step."""
    import subprocess

    subprocess.run(["git", "init", "-b", "main"], cwd=tmp_path, check=True, capture_output=True)
    subprocess.run(
        ["git", "remote", "add", "origin", "git@github.com:thrillmade/example-repo.git"],
        cwd=tmp_path, check=True, capture_output=True,
    )


def _write_pat_dependent_workflow(tmp_path):
    """Drop a workflow that references the PAT, so the probe knows to look
    up the secret."""
    workflows = tmp_path / ".github" / "workflows"
    workflows.mkdir(parents=True)
    (workflows / "regen-timeline.yml").write_text(
        "name: x\n# uses ${{ secrets.LOGMIND_AUTO_REGEN_PAT }}\n",
        encoding="utf-8",
    )


def test_pat_probe_reports_stale_when_gh_confirms_secret_missing(
    tmp_path, monkeypatch,
):
    """Primary feature path: workflow needs the PAT AND `gh secret list`
    confirms it's not configured. doctor must surface drift="stale" with
    actionable remediation in the marker text."""
    import subprocess
    from logmind.core import doctor as doctor_mod

    _make_git_repo(tmp_path)
    _write_pat_dependent_workflow(tmp_path)

    import subprocess as real_subprocess
    _original_run = real_subprocess.run

    def fake_run(cmd, *args, **kwargs):
        # gh secret list — return an empty list (no secrets configured)
        if isinstance(cmd, list) and len(cmd) >= 2 and cmd[0] == "gh" and cmd[1] == "secret":
            class R:
                returncode = 0
                stdout = "[]"
                stderr = ""
            return R()
        # All other subprocess.run calls (git operations, etc.) pass through.
        return _original_run(cmd, *args, **kwargs)

    monkeypatch.setattr(real_subprocess, "run", fake_run)

    status = doctor_mod._probe_auto_regen_pat(tmp_path)
    assert status.drift == "stale"
    assert "MISSING" in (status.marker or "")
    # Remediation must embed both the URL pattern AND the required scopes
    # — otherwise a user hitting this row doesn't know what to configure.
    marker = status.marker or ""
    assert "thrillmade/example-repo" in marker, (
        f"marker must include repo URL for one-click remediation. Got: {marker!r}"
    )
    assert "Contents: write" in marker
    assert "Workflows: write" in marker
    assert "Pull-requests: write" in marker


def test_pat_probe_reports_current_when_gh_confirms_secret_present(
    tmp_path, monkeypatch,
):
    """Mirror test: workflow needs PAT and gh secret list confirms presence."""
    import subprocess
    from logmind.core import doctor as doctor_mod

    _make_git_repo(tmp_path)
    _write_pat_dependent_workflow(tmp_path)

    import subprocess as real_subprocess
    _original_run = real_subprocess.run

    def fake_run(cmd, *args, **kwargs):
        if isinstance(cmd, list) and len(cmd) >= 2 and cmd[0] == "gh" and cmd[1] == "secret":
            class R:
                returncode = 0
                stdout = '[{"name":"LOGMIND_AUTO_REGEN_PAT"},{"name":"OTHER"}]'
                stderr = ""
            return R()
        return _original_run(cmd, *args, **kwargs)

    monkeypatch.setattr(real_subprocess, "run", fake_run)

    status = doctor_mod._probe_auto_regen_pat(tmp_path)
    assert status.drift == "current"
    assert status.installed is True
    # Even on "current", the marker MUST surface the required scopes so
    # users know to verify the existing token is properly scoped — today's
    # tokenomics case had the secret present but under-scoped.
    marker = status.marker or ""
    assert "Contents: write" in marker
    assert "Workflows: write" in marker
    assert "Pull-requests: write" in marker


def test_pat_probe_tolerates_malformed_gh_json(tmp_path, monkeypatch):
    """Defensive: if `gh secret list --json name` returns a bare integer
    (or any non-list), the iteration must not crash. Should fall through
    to "missing"."""
    import subprocess
    from logmind.core import doctor as doctor_mod

    _make_git_repo(tmp_path)
    _write_pat_dependent_workflow(tmp_path)

    import subprocess as real_subprocess
    _original_run = real_subprocess.run

    def fake_run(cmd, *args, **kwargs):
        if isinstance(cmd, list) and len(cmd) >= 2 and cmd[0] == "gh" and cmd[1] == "secret":
            class R:
                returncode = 0
                stdout = "42"  # Malformed: a bare JSON integer.
                stderr = ""
            return R()
        return _original_run(cmd, *args, **kwargs)

    monkeypatch.setattr(real_subprocess, "run", fake_run)

    # Must not raise TypeError; should fall through to "missing" since the
    # set of recognized secret names is empty.
    status = doctor_mod._probe_auto_regen_pat(tmp_path)
    assert status.drift == "stale"
    assert "MISSING" in (status.marker or "")


# ---------------------------------------------------------------------------
# v0.6.13 / issue #112 — post-merge hook embeds orphan-branch skip logic.
# Even on stale-binary installs (the tokenomics agent's case), the hook
# itself must short-circuit when the current branch's upstream remote-
# tracking ref no longer exists (typical after `gh pr merge --delete-
# branch` + `git fetch --prune`). Skip > regen-and-leave-unstaged.
# ---------------------------------------------------------------------------


def test_post_merge_hook_body_embeds_orphan_branch_skip_logic():
    """The hook body produced by the builder MUST include the orphan-
    branch detection block so issue #112 can't recur even when the user
    runs `gh pr merge --delete-branch` on a feature branch."""
    from logmind.core.gitattributes import _build_post_merge_hook_body

    body = _build_post_merge_hook_body()
    # The literal sentinel for the detection — references the @{u}
    # upstream tracking ref and the `refs/remotes/$upstream` test that
    # confirms the upstream-tracking ref still exists.
    assert "@{u}" in body, (
        "v0.6.13 issue #112: post-merge hook must check git rev-parse @{u} "
        "to detect orphaned-branch state."
    )
    assert "refs/remotes" in body, (
        "v0.6.13 issue #112: post-merge hook must check that the upstream's "
        "remote-tracking ref still exists (not pruned away by `git fetch "
        "--prune` after the merged-away branch was deleted)."
    )
    assert "exit 0" in body, (
        "v0.6.13 issue #112: post-merge hook must SKIP regen (exit 0) on "
        "orphan-branch detection, not fall through."
    )
