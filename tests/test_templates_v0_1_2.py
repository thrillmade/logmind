"""Template regression tests for the upstream bug fixes shipped in v0.1.2.

Each bug was caught by bot reviewers on a downstream `logmind init` run
(thrillmot/clud-bug PR #21). The fixes live in the .template files shipped
with the package; these tests assert each fix is present so a future refactor
can't silently re-introduce the bug.
"""

from __future__ import annotations

from pathlib import Path

import pytest

TEMPLATE_ROOT = Path(__file__).parent.parent / "src" / "logmind" / "templates"


def _read(rel: str) -> str:
    return (TEMPLATE_ROOT / rel).read_text(encoding="utf-8")


# ---------------------------------------------------------------------------
# config.yml: richer default ignore_patterns
# ---------------------------------------------------------------------------


def test_config_template_includes_node_next_patterns():
    """Node/Next.js projects should have build cache excluded by default,
    not just Python __pycache__/dist/build."""
    content = _read("config.yml.template")
    for pat in (".next", ".vercel", ".turbo", "out", ".cache", "coverage",
                "*.tsbuildinfo", ".DS_Store"):
        assert pat in content, f"missing ignore pattern: {pat}"


# ---------------------------------------------------------------------------
# check-decisions.yml: 3 fixes
# ---------------------------------------------------------------------------


def test_check_decisions_skip_hatch_is_wired():
    """The [skip-logmind] PR-title override advertised in the error message
    must actually be checked in the gate `if:` (bug #2 in v0.1.1)."""
    content = _read("github/check-decisions.yml.template")
    assert "!contains(github.event.pull_request.title, '[skip-logmind]')" in content


def test_check_decisions_threshold_threaded():
    """The THRESHOLD env var must flow into the gate condition; hardcoded
    `>= 20` made the env dead code (bug #3 in v0.1.1)."""
    content = _read("github/check-decisions.yml.template")
    assert "fromJSON(env.THRESHOLD)" in content


def test_check_decisions_no_renames():
    """`git diff --numstat` without --no-renames mishandles src→docs
    renames (bug #5 in v0.1.1)."""
    content = _read("github/check-decisions.yml.template")
    assert "git diff --numstat --no-renames" in content


# ---------------------------------------------------------------------------
# logmind-aggregate.yml: PR fallback under branch protection
# ---------------------------------------------------------------------------


def test_aggregate_pr_fallback_present():
    """Direct push to a protected base ref fails with GITHUB_TOKEN; the
    template must fall back to opening a PR (bug #4 in v0.1.1)."""
    content = _read("github/logmind-aggregate.yml.template")
    assert "gh pr create" in content
    assert "pull-requests: write" in content
    assert "GH013" in content
    assert "protected branch hook declined" in content


def test_aggregate_preserves_recursion_guard():
    """The early-exit `git diff --quiet docs/decisions.md` must remain so
    the fallback-PR-merge re-trigger is a no-op."""
    content = _read("github/logmind-aggregate.yml.template")
    assert "git diff --quiet docs/decisions.md" in content


def test_aggregate_handles_disabled_actions_pr_creation():
    """When 'Allow GitHub Actions to create PRs' is off, gh pr create fails.
    Template should surface a ::warning:: and exit 0, not fail the run."""
    content = _read("github/logmind-aggregate.yml.template")
    assert "::warning::" in content


# ---------------------------------------------------------------------------
# v0.1.4 — LOGMIND_BOT_PAT fallback for aggregator PRs
# ---------------------------------------------------------------------------


def test_aggregate_template_uses_bot_pat_fallback():
    """v0.1.4: aggregator must prefer LOGMIND_BOT_PAT and fall back to
    GITHUB_TOKEN. Without the fallback, users with required status checks
    on their base ref get permanently-unmergeable aggregator PRs because
    GITHUB_TOKEN-opened PRs can't trigger downstream workflows."""
    content = _read("github/logmind-aggregate.yml.template")
    assert "secrets.LOGMIND_BOT_PAT || secrets.GITHUB_TOKEN" in content


def test_aggregate_template_warns_when_no_bot_pat():
    """If the fallback PR opens without LOGMIND_BOT_PAT set, the workflow
    must emit a ::warning:: explaining why required checks won't fire and
    how to fix it (set the secret)."""
    content = _read("github/logmind-aggregate.yml.template")
    assert "HAS_BOT_PAT" in content
    # The warning must name the secret so users know what to set
    assert "LOGMIND_BOT_PAT" in content
    # And mention the consequence so the warning is actionable
    assert "Required status checks" in content or "required checks" in content


# ---------------------------------------------------------------------------
# AGENTS.md templates: new agent-skills collection URL
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "tmpl", ["AGENTS.md.template", "AGENTS.md.slim.template"]
)
def test_agents_md_install_url_points_at_collection(tmpl):
    """v0.1.1 templates referenced thrillmot/logmind-skill (two-level URL);
    v0.1.2 must reference the agent-skills collection layout."""
    content = _read(tmpl)
    assert "thrillmot/agent-skills" in content
    assert "--skill logmind" in content
    # Old single-skill repo URL must not linger
    assert "thrillmot/logmind-skill" not in content
