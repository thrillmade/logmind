"""Tests for v0.6.6's `check_clud_bug_skill_usage_integration` doctor probe.

Mirrors the shape of `test_stale_derived_docs_warning.py`. The probe
surfaces drift between consumers' clud-bug-review.yml and the v0.6.29+
skill-usage upload-step contract (incl. v0.6.31 `include-hidden-files`).
"""

from __future__ import annotations

from pathlib import Path

from logmind.core.doctor import check_clud_bug_skill_usage_integration


def _write_workflow(repo: Path, content: str) -> Path:
    wf = repo / ".github" / "workflows" / "clud-bug-review.yml"
    wf.parent.mkdir(parents=True, exist_ok=True)
    wf.write_text(content, encoding="utf-8")
    return wf


def test_no_workflow_returns_none(tmp_path: Path):
    """Silent no-op when clud-bug-review.yml doesn't exist."""
    assert check_clud_bug_skill_usage_integration(tmp_path) is None


def test_workflow_without_upload_step_warns_v0_6_29_drift(tmp_path: Path):
    """Pre-v0.6.29 workflow has no Upload skill-usage step → warn."""
    _write_workflow(
        tmp_path,
        "name: clud-bug-review\n"
        "on:\n  pull_request:\n"
        "jobs:\n  clud-bug-review:\n    runs-on: ubuntu-latest\n"
        "    steps:\n"
        "      - uses: actions/checkout@v6\n"
        "      - name: Run clud-bug review\n"
        "        run: npx --yes clud-bug@0.6.28 review\n",
    )
    msg = check_clud_bug_skill_usage_integration(tmp_path)
    assert msg is not None
    assert "Upload skill-usage artifact" in msg
    assert "npx clud-bug update" in msg


def test_workflow_with_upload_but_missing_include_hidden_warns_v0_6_31_drift(tmp_path: Path):
    """v0.6.29 or v0.6.30 workflow has the step but no include-hidden-files → warn."""
    _write_workflow(
        tmp_path,
        "name: clud-bug-review\n"
        "jobs:\n  clud-bug-review:\n"
        "    steps:\n"
        "      - name: Upload skill-usage artifact\n"
        "        if: success()\n"
        "        continue-on-error: true\n"
        "        uses: actions/upload-artifact@v4\n"
        "        with:\n"
        "          name: clud-bug-skill-usage-pr-${{ github.event.pull_request.number }}\n"
        "          path: .claude/skills/.clud-bug.json\n"
        "          retention-days: 90\n",
    )
    msg = check_clud_bug_skill_usage_integration(tmp_path)
    assert msg is not None
    assert "include-hidden-files" in msg
    assert "v0.6.31" in msg
    assert "npx clud-bug update" in msg


def test_workflow_with_v0_6_31_fix_returns_none(tmp_path: Path):
    """Workflow with both the step + the flag → no warning."""
    _write_workflow(
        tmp_path,
        "name: clud-bug-review\n"
        "jobs:\n  clud-bug-review:\n"
        "    steps:\n"
        "      - name: Upload skill-usage artifact\n"
        "        if: success()\n"
        "        continue-on-error: true\n"
        "        uses: actions/upload-artifact@v4\n"
        "        with:\n"
        "          name: clud-bug-skill-usage-pr-${{ github.event.pull_request.number }}\n"
        "          path: .claude/skills/.clud-bug.json\n"
        "          include-hidden-files: true\n"
        "          retention-days: 90\n",
    )
    assert check_clud_bug_skill_usage_integration(tmp_path) is None


def test_commented_out_flag_does_not_satisfy_check(tmp_path: Path):
    """Commented include-hidden-files line must NOT be treated as present.

    Mirror of clud-bug's v0.6.32 release-discipline test — same anchored-regex
    behavior. If the consumer's workflow has a commented-out version of the
    flag (perhaps from a botched merge resolution), the check should still warn.
    """
    _write_workflow(
        tmp_path,
        "name: clud-bug-review\n"
        "jobs:\n  clud-bug-review:\n"
        "    steps:\n"
        "      - name: Upload skill-usage artifact\n"
        "        uses: actions/upload-artifact@v4\n"
        "        with:\n"
        "          name: clud-bug-skill-usage-pr-1\n"
        "          path: .claude/skills/.clud-bug.json\n"
        "          # include-hidden-files: true  (commented out)\n"
        "          retention-days: 90\n",
    )
    msg = check_clud_bug_skill_usage_integration(tmp_path)
    assert msg is not None
    assert "include-hidden-files" in msg


def test_workflow_is_directory_returns_none(tmp_path: Path):
    """If `.github/workflows/clud-bug-review.yml` is a directory (unusual but
    possible from a botched checkout), the `is_file()` guard returns None
    rather than letting `read_text` raise. Defensive."""
    weird = tmp_path / ".github" / "workflows" / "clud-bug-review.yml"
    weird.mkdir(parents=True)
    assert check_clud_bug_skill_usage_integration(tmp_path) is None
