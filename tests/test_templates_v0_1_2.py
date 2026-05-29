"""Template regression tests for the upstream bug fixes shipped in v0.1.2.

Each bug was caught by bot reviewers on a downstream `logmind init` run
(thrillmade/clud-bug PR #21). The fixes live in the .template files shipped
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
# v0.1.2 aggregator-template tests removed in v0.2 — the entire aggregator
# template is gone, replaced by docs/timeline.md as a derived artifact.
# See tests/test_timeline.py for the new regression suite + the
# test_aggregator_template_no_longer_shipped guard that this stays gone.
# ---------------------------------------------------------------------------


# ---------------------------------------------------------------------------
# AGENTS.md templates: new agent-skills collection URL
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "tmpl", ["AGENTS.md.template", "AGENTS.md.slim.template"]
)
def test_agents_md_install_url_points_at_collection(tmpl):
    """v0.1.1 templates referenced thrillmade/logmind-skill (two-level URL);
    v0.1.2 must reference the agent-skills collection layout.

    0.B.6 (v0.5.6): the slim variant dropped the explicit
    `npx skills add ... --skill logmind` bash example as part of the
    block trim (~770 bytes vs v5-slim's 2526). The skill-URL pointer
    is still present — that's the load-bearing assertion. The
    `--skill logmind` install command is now covered by the skill
    itself or the README; checking it only on the FULL template
    where the inline procedure still lives.
    """
    content = _read(tmpl)
    assert "thrillmade/agent-skills" in content
    if tmpl == "AGENTS.md.template":
        # Full template still ships the inline install command.
        assert "--skill logmind" in content
    # Old single-skill repo URL must not linger (both variants).
    assert "thrillmade/logmind-skill" not in content
