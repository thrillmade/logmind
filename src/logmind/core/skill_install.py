"""Optional install of the logmind skills.sh skill at logmind-init time.

The logmind agent skill is published as a separate repository so any AI agent
that supports skills.sh (Claude Code, Cursor, Codex, Cline, ...) can pick up
the "log decisions while you work" instructions without needing to read
AGENTS.md every time.

This module is intentionally minimal — it shells out to the ``skills`` CLI
already on the user's machine. We never pull npm packages without consent;
the install is gated behind a CLI flag or interactive prompt.
"""

from __future__ import annotations

import shutil
import subprocess
from typing import Optional

# The default upstream skill source — full URL so `skills add` recognizes
# this as a collection install (skills/<name>/SKILL.md layout). Overridable
# via env var or CLI flag.
DEFAULT_SKILL_SOURCE = "https://github.com/thrillmot/agent-skills"
DEFAULT_SKILL_NAME = "logmind"


def is_skills_available() -> bool:
    """Return True iff the skills.sh CLI (`npx skills` or `skills`) is usable.

    Detection order:
      1. ``skills`` binary on PATH
      2. ``npx`` on PATH (we'll invoke ``npx -y skills``)
    """
    if shutil.which("skills") is not None:
        return True
    if shutil.which("npx") is not None:
        return True
    return False


def _build_install_argv(
    source: str,
    skill_name: str,
    *,
    use_npx: bool,
) -> list:
    if use_npx:
        return ["npx", "-y", "skills", "add", "-g", source, "--skill", skill_name]
    return ["skills", "add", "-g", source, "--skill", skill_name]


def install_globally(
    source: str = DEFAULT_SKILL_SOURCE,
    skill_name: str = DEFAULT_SKILL_NAME,
    *,
    runner: Optional[object] = None,
) -> tuple:
    """
    Run the global install of the logmind skill.

    Returns ``(returncode, output)`` from the underlying CLI invocation.
    Pass a custom ``runner`` (a callable equivalent to ``subprocess.run``)
    to mock in tests.
    """
    use_npx = shutil.which("skills") is None
    argv = _build_install_argv(source, skill_name, use_npx=use_npx)

    run = runner if runner is not None else subprocess.run
    result = run(argv, capture_output=True, text=True)
    output = (getattr(result, "stdout", "") or "") + (getattr(result, "stderr", "") or "")
    return result.returncode, output
