"""Tests for `logmind doctor`."""

from __future__ import annotations

import json
from pathlib import Path

import pytest
from click.testing import CliRunner

from logmind.cli import main
from logmind.core import doctor


@pytest.fixture
def project(tmp_path: Path) -> Path:
    """Empty project root with .github/workflows/ ready to populate."""
    (tmp_path / ".github" / "workflows").mkdir(parents=True)
    return tmp_path


@pytest.fixture(autouse=True)
def _stub_path_probe_in_clean_state(monkeypatch, request):
    """Tests in this file generally assert on workflow drift, NOT on the
    PATH-resolution probe. In the editable-install + pyenv-shim test
    environment, `shutil.which("logmind")` resolves to a shim that may
    invoke a SLIGHTLY OLDER version (the previous editable rebuild)
    than what the running Python module reports — a real signal in
    production but a noisy false-positive in tests that aren't about it.

    Tests opt OUT by including "path_probe" or "path_stale" in their
    name; those tests want the real probe to run.
    """
    if "path_probe" in request.node.name or "path_stale" in request.node.name:
        return
    from logmind.core import doctor as doctor_mod
    from logmind.core.doctor import WorkflowStatus

    def _stubbed_probe():
        return WorkflowStatus(
            name="logmind on PATH", installed=True,
            marker="(stubbed for test)", bundled_marker="(stubbed)",
            drift="current",
        )
    monkeypatch.setattr(doctor_mod, "_probe_path_resolution", _stubbed_probe)


def _write_workflow(project: Path, name: str, content: str) -> None:
    (project / ".github" / "workflows" / name).write_text(content, encoding="utf-8")


def _write_clud_bug_cfg(project: Path, payload: dict) -> None:
    skills = project / ".claude" / "skills"
    skills.mkdir(parents=True, exist_ok=True)
    (skills / ".clud-bug.json").write_text(json.dumps(payload), encoding="utf-8")


# ---------------------------------------------------------------------------
# collect_status — pure logic, no CLI
# ---------------------------------------------------------------------------


def test_offline_no_workflows_reports_clean_unknown_versions(project: Path):
    """No workflows, no clud-bug, offline mode → no crash; latest=None."""
    report = doctor.collect_status(project, offline=True)
    assert report.network_used is False
    assert len(report.tools) == 1
    logmind = report.tools[0]
    assert logmind.name == "logmind"
    assert logmind.installed_version is None
    assert logmind.latest_version is None
    # Every shipped workflow + AGENTS.md + the merge-driver probes
    # (v0.3.0: gitattributes block, git config, post-merge; v0.5.11:
    # post-rewrite) are reported as missing. None should be `stale` —
    # that would false-positive every fresh fixture.
    # v0.6.16: the `logmind on PATH` probe is also added unconditionally
    # (it surfaces tokenomics-style stale-binary cases). In a fresh test
    # fixture it typically reports `current` (running matches PATH) — it
    # isn't a "missing" row and shouldn't be counted as drift.
    names = {w.name for w in logmind.workflows}
    expected = (
        set(doctor.LOGMIND_WORKFLOWS)
        | {"AGENTS.md"}
        | {
            ".gitattributes (merge driver)",
            "git config (merge driver)",
            "post-merge hook",
            "post-rewrite hook",
            "commit-msg hook",
            "logmind on PATH",
        }
    )
    assert names == expected
    # All probes EXCEPT the PATH probe report `missing` in this fixture.
    non_path = [w for w in logmind.workflows if w.name != "logmind on PATH"]
    assert all(w.drift == "missing" for w in non_path)


def test_pinned_workflow_with_matching_marker_is_current(project: Path, monkeypatch):
    """A workflow pinned to logmind==X.Y.Z with marker matching bundled → current."""
    bundled = doctor._bundled_logmind_marker("regen-timeline.yml")
    assert bundled is not None  # sanity: shipped template has a marker
    _write_workflow(
        project,
        "regen-timeline.yml",
        f"# logmind-template-version: {bundled}\nname: regen\n"
        f"          pip install \"logmind==0.2.3\"\n",
    )
    # Monkeypatch the HTTP probe to return 0.2.3 too — no drift
    monkeypatch.setattr(
        doctor, "_http_get_json", lambda *_a, **_kw: {"info": {"version": "0.2.3"}}
    )
    report = doctor.collect_status(project, offline=False)
    logmind = report.tools[0]
    assert logmind.installed_version == "0.2.3"
    assert logmind.latest_version == "0.2.3"
    regen = next(w for w in logmind.workflows if w.name == "regen-timeline.yml")
    assert regen.drift == "current"
    assert logmind.drift == "ok"
    assert report.overall == "OK"


def test_stale_marker_in_workflow_triggers_drift(project: Path, monkeypatch):
    """Marker on workflow ≠ bundled marker → stale → DRIFT.

    Read the bundled marker dynamically so this test doesn't have to be
    updated every release that bumps a template version.
    """
    bundled = doctor._bundled_logmind_marker("check-doc-links.yml")
    assert bundled is not None
    # Pick a marker that's definitely different from bundled (v0 is older
    # than any version we've ever shipped).
    _write_workflow(
        project,
        "check-doc-links.yml",
        "# logmind-template-version: v0\nname: links\n",
    )
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    report = doctor.collect_status(project, offline=False)
    cdl = next(w for w in report.tools[0].workflows if w.name == "check-doc-links.yml")
    assert cdl.marker == "v0"
    assert cdl.bundled_marker == bundled
    assert cdl.drift == "stale"
    assert report.tools[0].drift == "stale"
    assert report.overall == "DRIFT"
    assert any("pip install --upgrade logmind" in s for s in report.suggestions)


def test_installed_version_older_than_latest_is_drift(project: Path, monkeypatch):
    """Workflow pin says 0.2.1, PyPI says 0.2.3 → DRIFT."""
    _write_workflow(
        project,
        "regen-timeline.yml",
        '          pip install "logmind==0.2.1"\n',
    )
    monkeypatch.setattr(
        doctor, "_http_get_json", lambda *_a, **_kw: {"info": {"version": "0.2.3"}}
    )
    report = doctor.collect_status(project, offline=False)
    logmind = report.tools[0]
    assert logmind.installed_version == "0.2.1"
    assert logmind.latest_version == "0.2.3"
    assert logmind.drift == "stale"
    assert report.overall == "DRIFT"


def test_markerless_workflow_is_not_drift(project: Path, monkeypatch):
    """Dogfood-style (markerless) installs must not false-positive."""
    _write_workflow(
        project,
        "regen-timeline.yml",
        "name: regen\n# no marker line\n",
    )
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    report = doctor.collect_status(project, offline=False)
    regen = next(w for w in report.tools[0].workflows if w.name == "regen-timeline.yml")
    assert regen.drift == "markerless"
    # No workflow marker → no drift contribution (and no PyPI pin → no version drift)
    assert report.tools[0].drift == "ok"
    assert report.overall == "OK"


def test_network_failure_offline_safe(project: Path, monkeypatch):
    """A network failure on PyPI must not crash; latest just becomes None."""
    _write_workflow(
        project,
        "regen-timeline.yml",
        '          pip install "logmind==0.2.3"\n',
    )
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    report = doctor.collect_status(project, offline=False)
    logmind = report.tools[0]
    assert logmind.installed_version == "0.2.3"
    assert logmind.latest_version is None
    assert logmind.drift == "ok"  # can't claim drift without a latest signal


def test_offline_flag_skips_network(project: Path, monkeypatch):
    """--offline must short-circuit _http_get_json entirely."""
    calls = []

    def fake_http(*args, **kwargs):
        calls.append(args)
        return {"info": {"version": "0.2.3"}}

    monkeypatch.setattr(doctor, "_http_get_json", fake_http)
    report = doctor.collect_status(project, offline=True)
    assert calls == [], "no HTTP calls should happen in --offline mode"
    assert report.network_used is False
    assert report.tools[0].latest_version is None


def test_agents_md_stale_block_version_triggers_drift(project: Path, monkeypatch):
    """v0.2.9: doctor must flag an out-of-date `<!-- logmind-block-version:
    vN -->` marker in AGENTS.md. Closes the propagation gap where an agent
    keeps working from cached pre-v0.2.7 instructions even though logmind
    init would have refreshed the block on disk.
    """
    (project / "AGENTS.md").write_text(
        "# AGENTS.md\n\n"
        "<!-- logmind-start -->\n"
        "<!-- logmind-block-version: v0 -->\n"
        "(stale embedded block)\n"
        "<!-- logmind-end -->\n",
        encoding="utf-8",
    )
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    report = doctor.collect_status(project, offline=False)
    am = next(w for w in report.tools[0].workflows if w.name == "AGENTS.md")
    assert am.marker == "v0"
    assert am.bundled_marker is not None  # bundled template exists in this repo
    assert am.drift == "stale"
    assert report.overall == "DRIFT"


def test_agents_md_full_template_marker_is_current(project: Path, monkeypatch):
    """Regression: a repo whose AGENTS.md was written by logmind init when
    skills.sh was NOT available carries the FULL template's marker (e.g.
    `v4`), not the slim one (`v4-slim`). Doctor must treat that as
    current — matching either bundled variant.

    Before this fix, _bundled_agents_md_block_version() returned only the
    slim marker; the full-template install showed up as STALE on every
    doctor run, breaking exit-code-based CI gating for those repos.
    """
    slim_bundled, full_bundled = doctor._bundled_agents_md_block_versions()
    assert slim_bundled is not None and full_bundled is not None, (
        "this test relies on both bundled templates carrying a marker"
    )
    # Use the FULL marker — the variant slim agents wouldn't get
    (project / "AGENTS.md").write_text(
        f"# AGENTS.md\n\n"
        f"<!-- logmind-start -->\n"
        f"<!-- logmind-block-version: {full_bundled} -->\n"
        f"(full-template body)\n"
        f"<!-- logmind-end -->\n",
        encoding="utf-8",
    )
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    report = doctor.collect_status(project, offline=False)
    am = next(w for w in report.tools[0].workflows if w.name == "AGENTS.md")
    assert am.marker == full_bundled
    assert am.drift == "current", (
        f"full-template install (marker={am.marker}) must not be flagged "
        f"stale just because bundled slim marker is {slim_bundled}"
    )


def test_agents_md_markerless_is_not_drift(project: Path, monkeypatch):
    """Markerless AGENTS.md (user heavily customized OR predates the
    marker convention) must NOT count as drift — same heuristic as
    workflow probes."""
    (project / "AGENTS.md").write_text(
        "# AGENTS.md\n\nCustomized — no logmind block.\n",
        encoding="utf-8",
    )
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    report = doctor.collect_status(project, offline=False)
    am = next(w for w in report.tools[0].workflows if w.name == "AGENTS.md")
    assert am.marker is None
    assert am.drift == "markerless"
    # Other workflows are missing; drift comes from them, not from AGENTS.md.


def test_clud_bug_absent_omitted(project: Path, monkeypatch):
    """Repos without clud-bug installed should not get a clud-bug section."""
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    report = doctor.collect_status(project, offline=False)
    assert [t.name for t in report.tools] == ["logmind"]


def test_clud_bug_release_version_from_lastUpdateVersion(project: Path, monkeypatch):
    """clud-bug's release version lives in `lastUpdateVersion`; `version` is a
    schema-version int that must not be misreported as a release."""
    _write_clud_bug_cfg(
        project,
        {"version": 1, "lastUpdateVersion": "0.5.6", "strictMode": False, "installed": []},
    )
    monkeypatch.setattr(
        doctor, "_http_get_json", lambda *_a, **_kw: {"version": "0.5.10"}
    )
    report = doctor.collect_status(project, offline=False)
    clud_bug = next(t for t in report.tools if t.name == "clud-bug")
    assert clud_bug.installed_version == "0.5.6"
    assert clud_bug.latest_version == "0.5.10"
    assert clud_bug.drift == "stale"
    assert clud_bug.extras["strict_mode"] == "off"


# ---------------------------------------------------------------------------
# CLI integration — exit codes + flags
# ---------------------------------------------------------------------------


def test_cli_doctor_ok_exits_zero(project: Path, monkeypatch):
    """Stack is OK → exit 0."""
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    runner = CliRunner()
    with runner.isolated_filesystem(temp_dir=project):
        # Empty project, offline-friendly: no version pin → no drift signal
        result = runner.invoke(main, ["doctor", "--offline"])
        assert result.exit_code == 0, result.output
        assert "Stack status: OK" in result.output


def test_cli_doctor_missing_merge_driver_in_git_repo_exits_one(project: Path, monkeypatch):
    """v0.5.13 / tokenomics issue: a git repo with missing merge-driver
    config OR missing post-merge / post-rewrite hooks is one merge away
    from a check-derived-docs failure. doctor must exit non-zero so CI
    gates catch this before the failure manifests.

    Pre-v0.5.13, `_probe_merge_driver_config` returned drift="missing"
    but `collect_logmind_status` only escalated to "stale" on workflow
    template drift — so a fresh clone with no merge-driver config
    silently reported OK. Now: any critical missing in a git repo →
    overall DRIFT.
    """
    import subprocess
    # Make the project a real git repo (without any logmind init).
    subprocess.run(["git", "init", "-q"], cwd=project, check=True)
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    monkeypatch.chdir(project)
    runner = CliRunner()
    result = runner.invoke(main, ["doctor", "--offline"])
    assert result.exit_code == 1, result.output
    assert "Stack status: DRIFT" in result.output
    # The signal should mention either the merge driver or a hook (so
    # users see WHICH critical-missing tripped the gate).
    assert (
        "merge driver" in result.output.lower()
        or "post-merge" in result.output.lower()
        or "post-rewrite" in result.output.lower()
    )


def test_cli_doctor_missing_merge_driver_outside_git_repo_is_ok(project: Path, monkeypatch):
    """v0.5.13: outside a git repo, missing merge-driver config is NOT
    drift — the driver only matters for git operations, so there's
    nothing for it to fail at. Test fixture: bare project dir (no .git/)
    must stay OK so test fixtures don't false-positive."""
    # NOTE: `project` fixture creates `.github/workflows` but no `.git/`
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    monkeypatch.chdir(project)
    runner = CliRunner()
    result = runner.invoke(main, ["doctor", "--offline"])
    assert result.exit_code == 0, result.output
    assert "Stack status: OK" in result.output


def test_cli_doctor_drift_exits_one(project: Path, monkeypatch):
    """Stale marker → exit 1."""
    _write_workflow(
        project,
        "check-doc-links.yml",
        "# logmind-template-version: v1\nname: links\n",  # bundled is v2
    )
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    runner = CliRunner()
    # Note: isolated_filesystem can't be used because we need cwd == project,
    # so test the CLI by chdir into project_root explicitly.
    monkeypatch.chdir(project)
    runner = CliRunner()
    result = runner.invoke(main, ["doctor", "--offline"])
    assert result.exit_code == 1, result.output
    assert "Stack status: DRIFT" in result.output


def test_cli_exit_zero_flag_overrides_drift(project: Path, monkeypatch):
    """--exit-zero forces exit 0 even on drift (informational CI runs)."""
    _write_workflow(
        project,
        "check-doc-links.yml",
        "# logmind-template-version: v1\nname: links\n",
    )
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    monkeypatch.chdir(project)
    runner = CliRunner()
    result = runner.invoke(main, ["doctor", "--offline", "--exit-zero"])
    assert result.exit_code == 0, result.output
    assert "Stack status: DRIFT" in result.output  # still REPORTS drift


def test_cli_json_output_parses(project: Path, monkeypatch):
    """--json emits valid JSON matching the StatusReport shape."""
    monkeypatch.setattr(doctor, "_http_get_json", lambda *_a, **_kw: None)
    monkeypatch.chdir(project)
    runner = CliRunner()
    result = runner.invoke(main, ["doctor", "--offline", "--json", "--exit-zero"])
    payload = json.loads(result.output)
    assert "tools" in payload
    assert "overall" in payload
    assert "network_used" in payload
    assert payload["network_used"] is False


# ---------------------------------------------------------------------------
# v0.6.16 — PATH-resolution conflict probe (tokenomics-agent 2026-06-01
# recurrence root cause: stale `logmind` ahead on shell PATH while doctor
# inspects a different binary via `python -m logmind`).
# ---------------------------------------------------------------------------


def test_path_probe_returns_current_when_path_matches_running(monkeypatch, tmp_path: Path):
    """Common case: `shutil.which("logmind")` resolves to a binary whose
    --version matches the currently-running version. Drift = "current"."""
    from logmind import __version__ as running_version
    from logmind.core.doctor import _probe_path_resolution
    import subprocess as real_subprocess

    fake_path = "/usr/local/bin/logmind"
    monkeypatch.setattr("shutil.which", lambda _name: fake_path)

    def fake_run(cmd, *args, **kwargs):
        class R:
            returncode = 0
            stdout = f"logmind, version {running_version}\n"
            stderr = ""
        return R()
    monkeypatch.setattr(real_subprocess, "run", fake_run)

    status = _probe_path_resolution()
    assert status.installed is True
    assert status.drift == "current"
    assert running_version in (status.marker or "")


def test_path_probe_flags_stale_when_path_version_differs(monkeypatch):
    """Tokenomics-recurrence scenario: PATH binary reports an OLDER version
    than the currently-running binary. drift="stale" so doctor exits non-
    zero. Marker must include both versions + path for one-glance fix."""
    from logmind import __version__ as running_version
    from logmind.core.doctor import _probe_path_resolution
    import subprocess as real_subprocess

    fake_path = "/Users/x/local/bin/logmind"
    monkeypatch.setattr("shutil.which", lambda _name: fake_path)

    def fake_run(cmd, *args, **kwargs):
        class R:
            returncode = 0
            stdout = "logmind, version 0.3.4\n"
            stderr = ""
        return R()
    monkeypatch.setattr(real_subprocess, "run", fake_run)

    status = _probe_path_resolution()
    assert status.installed is True
    assert status.drift == "stale", (
        f"PATH-conflict must surface as stale so doctor exits non-zero; "
        f"got drift={status.drift!r}"
    )
    marker = status.marker or ""
    assert "0.3.4" in marker, "marker must include PATH binary's version"
    assert running_version in marker, "marker must include currently-running version"
    assert fake_path in marker, "marker must include the conflicting path"


def test_path_probe_reports_missing_when_logmind_not_on_path(monkeypatch):
    """No `logmind` on PATH at all → drift="missing". The merge driver
    shell-out would fail in this state. Not as severe as stale, but worth
    surfacing."""
    from logmind.core.doctor import _probe_path_resolution

    monkeypatch.setattr("shutil.which", lambda _name: None)
    status = _probe_path_resolution()
    assert status.installed is False
    assert status.drift == "missing"


def test_path_probe_tolerates_unparseable_version_output(monkeypatch):
    """Defensive: PATH binary exists but --version emits something we
    can't parse (truncated, mangled, foreign CLI named `logmind`).
    Drift = "markerless" so we report the binary's existence without
    falsely claiming drift OR claiming current."""
    from logmind.core.doctor import _probe_path_resolution
    import subprocess as real_subprocess

    monkeypatch.setattr("shutil.which", lambda _name: "/usr/bin/logmind")

    def fake_run(cmd, *args, **kwargs):
        class R:
            returncode = 0
            stdout = "(garbled output: no semver here)\n"
            stderr = ""
        return R()
    monkeypatch.setattr(real_subprocess, "run", fake_run)

    status = _probe_path_resolution()
    assert status.installed is True
    assert status.drift == "markerless"


def test_doctor_overall_flips_to_drift_when_path_stale(monkeypatch, project: Path):
    """End-to-end: a PATH-stale probe flows through collect_logmind_status
    → ToolStatus.drift = "stale" → overall = "DRIFT" → cli exits 1."""
    from logmind import __version__ as running_version
    from logmind.core import doctor as doctor_mod
    import subprocess as real_subprocess

    monkeypatch.setattr(doctor_mod, "_http_get_json", lambda *_a, **_kw: None)
    monkeypatch.setattr("shutil.which", lambda _name: "/usr/local/bin/logmind")

    # Capture the real subprocess.run BEFORE patching so the fallback
    # pass-through actually executes real subprocesses (git remote, gh
    # secret list, etc. — they all degrade gracefully on failure but
    # need a real run, not None). Prior implementation used
    # `real_subprocess.run.__wrapped__` which never exists on a plain
    # function and silently returned None, masking any new probe that
    # accesses `.returncode` on the result.
    real_run = real_subprocess.run

    def fake_run(cmd, *args, **kwargs):
        if isinstance(cmd, list) and len(cmd) >= 2 and cmd[1] == "--version":
            class R:
                returncode = 0
                stdout = "logmind, version 0.3.4\n"
                stderr = ""
            return R()
        return real_run(cmd, *args, **kwargs)

    monkeypatch.setattr(real_subprocess, "run", fake_run)

    monkeypatch.chdir(project)
    runner = CliRunner()
    result = runner.invoke(main, ["doctor", "--offline"])
    assert "Stack status: DRIFT" in result.output, (
        f"PATH-stale must drive overall=DRIFT.\nOutput:\n{result.output}"
    )
    assert result.exit_code != 0, (
        f"Doctor must exit non-zero on PATH-stale drift; got exit_code={result.exit_code}"
    )
