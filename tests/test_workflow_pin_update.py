"""Tests for v0.5.13 workflow pin auto-update.

`logmind agents update --apply` now sweeps `pip install "logmind==X.Y.Z"`
lines in `.github/workflows/<canonical>.yml` files alongside the AGENTS.md
block refresh. Closes recurring gotcha #1 (logmind workflow install pin
goes stale when consumer repos re-render workflows via `clud-bug update`).
"""

from __future__ import annotations

from pathlib import Path

import pytest
from click.testing import CliRunner

from logmind import __version__ as current_version
from logmind.cli import main as cli_main
from logmind.core.inserter import (
    LOGMIND_PIN_WORKFLOWS,
    find_outdated_workflow_pins,
    update_workflow_pin,
)


def _write_workflow(root: Path, name: str, pin_version: str) -> Path:
    wf_dir = root / ".github" / "workflows"
    wf_dir.mkdir(parents=True, exist_ok=True)
    wf_path = wf_dir / name
    wf_path.write_text(
        f"name: Test\n"
        f"on: push\n"
        f"jobs:\n"
        f"  go:\n"
        f"    runs-on: ubuntu-latest\n"
        f"    steps:\n"
        f"      - run: pip install \"logmind=={pin_version}\"\n",
        encoding="utf-8",
    )
    return wf_path


def test_update_workflow_pin_bumps_stale_version():
    content = '      - run: pip install "logmind==0.3.4"\n'
    new_content, prev = update_workflow_pin(content, "0.5.13")
    assert prev == "0.3.4"
    assert 'pip install "logmind==0.5.13"' in new_content
    assert 'pip install "logmind==0.3.4"' not in new_content


def test_update_workflow_pin_idempotent_when_already_current():
    content = '      - run: pip install "logmind==0.5.13"\n'
    new_content, prev = update_workflow_pin(content, "0.5.13")
    assert prev == "0.5.13"
    assert new_content == content


def test_update_workflow_pin_noop_when_no_pin_present():
    """Dogfood-style workflow files (no pin, uses pip install -e .) are
    untouched. Returns the original content + None."""
    content = "name: dogfood\non: push\n# no pin line\n"
    new_content, prev = update_workflow_pin(content, "0.5.13")
    assert prev is None
    assert new_content == content


def test_find_outdated_workflow_pins_finds_stale(tmp_path):
    _write_workflow(tmp_path, "regen-timeline.yml", "0.3.4")
    _write_workflow(tmp_path, "check-doc-links.yml", "0.3.4")

    outdated = find_outdated_workflow_pins(tmp_path)
    assert len(outdated) == 2
    for wf_path, old, new in outdated:
        assert old == "0.3.4"
        assert new == current_version


def test_find_outdated_workflow_pins_skips_current(tmp_path):
    """Workflows already pinning the current version are NOT returned."""
    _write_workflow(tmp_path, "regen-timeline.yml", current_version)

    outdated = find_outdated_workflow_pins(tmp_path)
    assert outdated == []


def test_find_outdated_workflow_pins_only_canonical_workflows(tmp_path):
    """Only LOGMIND_PIN_WORKFLOWS files are checked — user-authored
    workflow files with similar names are NOT touched."""
    _write_workflow(tmp_path, "regen-timeline.yml", "0.3.4")  # canonical
    _write_workflow(tmp_path, "my-custom-workflow.yml", "0.3.4")  # NOT canonical

    outdated = find_outdated_workflow_pins(tmp_path)
    assert len(outdated) == 1
    assert outdated[0][0].name == "regen-timeline.yml"


def test_find_outdated_workflow_pins_handles_missing_workflows_dir(tmp_path):
    """No `.github/workflows/` at all → empty list (no crash)."""
    assert find_outdated_workflow_pins(tmp_path) == []


def test_agents_update_dry_run_reports_stale_pins(tmp_path, monkeypatch):
    """Dry-run output mentions the stale pins so user sees what --apply would change."""
    _write_workflow(tmp_path, "regen-timeline.yml", "0.3.4")
    _write_workflow(tmp_path, "check-doc-links.yml", "0.3.4")
    monkeypatch.chdir(tmp_path)

    runner = CliRunner()
    result = runner.invoke(cli_main, ["agents", "update"])
    assert result.exit_code == 0, result.output
    assert "CI workflow pin" in result.output
    assert "logmind==0.3.4" in result.output
    assert f"logmind=={current_version}" in result.output
    # Dry-run does NOT modify files
    assert 'pip install "logmind==0.3.4"' in (
        tmp_path / ".github" / "workflows" / "regen-timeline.yml"
    ).read_text()


def test_agents_update_apply_rewrites_stale_pins(tmp_path, monkeypatch):
    """--apply actually writes the new pins."""
    _write_workflow(tmp_path, "regen-timeline.yml", "0.3.4")
    _write_workflow(tmp_path, "check-doc-links.yml", "0.3.4")
    monkeypatch.chdir(tmp_path)

    runner = CliRunner()
    result = runner.invoke(cli_main, ["agents", "update", "--apply"])
    assert result.exit_code == 0, result.output

    regen = (tmp_path / ".github" / "workflows" / "regen-timeline.yml").read_text()
    cdl = (tmp_path / ".github" / "workflows" / "check-doc-links.yml").read_text()
    assert f'logmind=={current_version}' in regen
    assert f'logmind=={current_version}' in cdl
    assert 'logmind==0.3.4' not in regen
    assert 'logmind==0.3.4' not in cdl


def test_agents_update_apply_is_idempotent(tmp_path, monkeypatch):
    """Second --apply run is a no-op (nothing to update)."""
    _write_workflow(tmp_path, "regen-timeline.yml", "0.3.4")
    monkeypatch.chdir(tmp_path)

    runner = CliRunner()
    first = runner.invoke(cli_main, ["agents", "update", "--apply"])
    assert first.exit_code == 0
    second = runner.invoke(cli_main, ["agents", "update", "--apply"])
    assert second.exit_code == 0
    # After the first update everything is current → some flavor of
    # "nothing to update" message (exact phrasing depends on whether
    # AGENTS.md exists; both "No AGENTS.md / nothing to update" and
    # "block is current (no update needed)" satisfy this).
    out = second.output.lower()
    assert "nothing to update" in out or "current" in out or "no update" in out


def test_canonical_workflow_list_matches_doctor_LOGMIND_WORKFLOWS():
    """Sanity: the pin-update workflow list should match the canonical set
    surfaced by `logmind doctor`. Drift between the two would mean doctor
    reports a workflow as "current" while the pin sweeper silently skips it
    (or vice-versa)."""
    from logmind.core.doctor import LOGMIND_WORKFLOWS

    assert set(LOGMIND_PIN_WORKFLOWS) == set(LOGMIND_WORKFLOWS), (
        "LOGMIND_PIN_WORKFLOWS (inserter.py) must match LOGMIND_WORKFLOWS "
        "(doctor.py) — drift would surface as inconsistent reports."
    )


# ---------------------------------------------------------------------------
# v0.6.11 — quote-style coverage for the pin regex
#
# reporulez ships single-quoted pins (`pip install 'logmind==X.Y.Z'`); the
# pre-v0.6.11 regex only matched bare or double-quoted forms, so
# `logmind agents update --apply` silently returned "nothing to bump" on
# reporulez during the v0.6.9 propagation cycle (manual sed required).
# These tests pin all three styles so future regex changes don't regress.
# ---------------------------------------------------------------------------


def test_update_workflow_pin_handles_single_quoted_form():
    """Single-quoted pins (`'logmind==X.Y.Z'`) are reporulez convention."""
    content = "      run: pip install 'logmind==0.5.6'\n"
    new_content, prev = update_workflow_pin(content, "0.6.11")
    assert prev == "0.5.6"
    assert new_content == "      run: pip install 'logmind==0.6.11'\n", (
        "v0.6.11 must preserve the EXACT quote style — no churn to "
        "double-quote on a single-quoted source."
    )


def test_update_workflow_pin_handles_double_quoted_form():
    """Double-quoted pins (`\"logmind==X.Y.Z\"`) are the historic default."""
    content = '      run: pip install "logmind==0.5.6"\n'
    new_content, prev = update_workflow_pin(content, "0.6.11")
    assert prev == "0.5.6"
    assert new_content == '      run: pip install "logmind==0.6.11"\n'


def test_update_workflow_pin_handles_bare_form():
    """Bare pins (no quotes) also work."""
    content = "      run: pip install logmind==0.5.6\n"
    new_content, prev = update_workflow_pin(content, "0.6.11")
    assert prev == "0.5.6"
    assert new_content == "      run: pip install logmind==0.6.11\n"


def test_find_outdated_workflow_pins_finds_single_quoted_pins(tmp_path):
    """reporulez regression guard: single-quoted pins must be detected as
    outdated when their version doesn't match `__version__`."""
    from logmind import __version__ as current_version
    workflows = tmp_path / ".github" / "workflows"
    workflows.mkdir(parents=True)
    (workflows / "regen-timeline.yml").write_text(
        "name: x\njobs:\n  j:\n    steps:\n      - run: pip install 'logmind==0.5.6'\n",
        encoding="utf-8",
    )
    outdated = find_outdated_workflow_pins(tmp_path)
    assert len(outdated) == 1
    path, found, target = outdated[0]
    assert path.name == "regen-timeline.yml"
    assert found == "0.5.6"
    assert target == current_version
