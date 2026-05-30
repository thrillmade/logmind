"""v0.6.0 — `logmind skill new/test` CLI implementations.

Composes with Zak Elfassi's ``@zakelfassi/skdd`` (the canonical SkDD
methodology toolkit) when present on PATH:

- ``logmind skill new <name>``: prefers ``skdd forge <name>`` if available;
  otherwise scaffolds a basic SKILL.md against the agentskills.io/v1 spec.
- ``logmind skill test <name>``: prefers ``skdd validate <name>``; layers
  logmind-specific checks (size cap, frontmatter presence) on top.

Design (per explore-agent strategic analysis, 2026-05-30):
- Don't reimplement Zak's ``forge`` / ``validate`` if the upstream CLI is
  available. Wire to it.
- Inject logmind-specific behavior AFTER his CLI does its work:
  decision-log the skill creation; layer additional validation.
- Spec compliance: SKILL.md frontmatter follows agentskills.io/v1
  (``name``, ``description`` required; ``metadata`` optional).

Scope:
- v0.6.0: ``new`` + ``test`` (minimum viable; ships the loop's first
  two arrows).
- v0.6.1: ``bench`` (token-cost measurement per skill).
- v0.6.2: ``log`` (decision-log a skill iteration, automatable from a bot).
"""

from __future__ import annotations

import re
import shutil
import subprocess
from pathlib import Path
from typing import Optional, Tuple


# Per agentskills.io/v1 spec: required fields are `name` + `description`.
# `metadata` is optional; we add a logmind-specific marker inside it.
_BASIC_SKILL_TEMPLATE = """---
name: {name}
description: {description}
metadata:
  logmind-managed: true
  spec: agentskills.io
---

# {title}

<!-- One short paragraph describing what this skill does + when an agent
should load it. The description field above is the discovery surface;
this body is what the agent reads once they decide to load it. -->

## When to use

- Trigger condition 1
- Trigger condition 2

## Steps

1. Step one
2. Step two

## Examples

<!-- Concrete examples make skills easier to apply correctly. -->
"""


def has_skdd() -> bool:
    """Return True iff Zak's ``skdd`` CLI is on PATH."""
    return shutil.which("skdd") is not None


def default_skills_dir(repo_root: Path) -> Path:
    """The canonical location for SKILL.md files in a repo.

    Defaults to ``.claude/skills/`` — the Claude Code convention that
    clud-bug and the agent-skills catalog use. Other harnesses (Cursor,
    Codex) follow the same per-skill subdirectory layout, so this works
    for any consumer.
    """
    return repo_root / ".claude" / "skills"


def skill_dir(repo_root: Path, name: str) -> Path:
    """Return the canonical directory for a skill of the given name."""
    return default_skills_dir(repo_root) / name


def skill_md_path(repo_root: Path, name: str) -> Path:
    return skill_dir(repo_root, name) / "SKILL.md"


def scaffold_basic_skill(
    repo_root: Path, name: str, description: str = ""
) -> Path:
    """Scaffold a basic agentskills.io/v1-compliant SKILL.md.

    Used when ``skdd`` is not on PATH. Creates the skill directory + a
    minimal SKILL.md with the required frontmatter fields.

    Raises ``FileExistsError`` if the skill already exists (refuse to
    clobber).
    """
    target = skill_md_path(repo_root, name)
    if target.exists():
        raise FileExistsError(f"Skill '{name}' already exists at {target}")

    target.parent.mkdir(parents=True, exist_ok=True)
    if not description:
        description = (
            f"TODO: one-sentence trigger description for '{name}'. "
            f"Be specific — this field is the discovery surface."
        )
    target.write_text(
        _BASIC_SKILL_TEMPLATE.format(
            name=name,
            description=description,
            title=name.replace("-", " ").replace("_", " ").title(),
        ),
        encoding="utf-8",
    )
    return target


def delegate_skdd_forge(repo_root: Path, name: str) -> Tuple[bool, str]:
    """Run ``skdd forge <name>`` in ``repo_root``. Returns (success, output)."""
    try:
        result = subprocess.run(
            ["skdd", "forge", name],
            cwd=repo_root,
            capture_output=True,
            text=True,
            check=False,
        )
        return (result.returncode == 0, result.stdout + result.stderr)
    except (FileNotFoundError, OSError) as e:
        return (False, str(e))


def delegate_skdd_validate(repo_root: Path, name: str) -> Tuple[bool, str]:
    """Run ``skdd validate`` in ``repo_root`` + filter result to ``name``.

    Zak's ``skdd validate`` walks every SKILL.md in the colony and
    reports per-file pass/fail. The upstream CLI doesn't take a name
    filter, so we run it once + parse the output for the line
    referencing ``name``. Returns:

      - (True, lines mentioning ``name`` — should be a pass marker)
      - (False, lines mentioning ``name`` — should be a fail marker)
      - (False, full output) when ``name`` isn't found in the output
        (defensive: report the whole thing rather than silently passing)

    **v0.6.0 PR #92 review fix**: previously returned the colony-wide
    exit code. Any unrelated broken skill in the repo caused
    ``logmind skill test <good-skill>`` to fail. Now we extract just
    the lines about ``name`` so the gate is per-skill as intended.
    """
    try:
        result = subprocess.run(
            ["skdd", "validate"],
            cwd=repo_root,
            capture_output=True,
            text=True,
            check=False,
        )
    except (FileNotFoundError, OSError) as e:
        return (False, str(e))

    combined = result.stdout + result.stderr
    # Pull out lines referencing the target skill. `skdd validate`'s
    # output format mentions the skill by path or name; we match both
    # to stay format-tolerant.
    relevant = [
        line for line in combined.splitlines()
        if name in line or f"skills/{name}/" in line
    ]

    if not relevant:
        # Skill not mentioned in output — likely means it wasn't found
        # by skdd OR the output format changed. Report the whole output
        # + fail, rather than silently passing.
        return (
            False,
            f"skdd validate did not mention '{name}' in its output. "
            f"Full output:\n{combined}",
        )

    # Determine pass/fail per-skill from the relevant lines. skdd's
    # convention: pass lines look like "✓" / "PASS" / "ok"; fail lines
    # look like "✗" / "FAIL" / "error". Be permissive on the markers.
    relevant_text = "\n".join(relevant)
    fail_markers = ("✗", "FAIL", "fail", "ERROR", "error")
    has_fail = any(m in relevant_text for m in fail_markers)
    return (not has_fail, relevant_text)


# Logmind-specific validation layered on top of skdd validate. Each
# returns (ok, message). When `ok is False`, message explains why.

# Soft size cap — most well-written skills land 50-200 lines. The cap
# guards against runaway skills that bloat every agent invocation.
_LOGMIND_SKILL_BYTE_CAP = 8000


# Anchored regexes to detect required-field presence in YAML
# frontmatter. Substring matches (e.g. `"name:" in fm`) false-positive on
# nested fields like `domain_name:` or `package_name:`. Per v0.6.0 PR #92
# review (evidence-based-review). Multiline + start-anchor matches both
# top-level fields AND fields nested under a single indent level (which
# is fine — they're still real `name:` declarations).
_FRONTMATTER_NAME_RE = re.compile(r"^\s*name\s*:", re.MULTILINE)
_FRONTMATTER_DESCRIPTION_RE = re.compile(r"^\s*description\s*:", re.MULTILINE)


def check_frontmatter_required_fields(content: str) -> Tuple[bool, str]:
    """Required fields per agentskills.io/v1: ``name`` + ``description``.

    Lightweight check — proper validation is `skdd validate`'s job.
    This catches the most common authoring mistakes when skdd isn't
    available.

    v0.6.0 PR #92 review fix: use anchored regexes instead of substring
    `in` checks so `domain_name:` doesn't false-positive as `name:`.
    """
    if not content.startswith("---"):
        return (False, "SKILL.md must start with YAML frontmatter (--- block)")
    end = content.find("\n---", 4)
    if end == -1:
        return (False, "SKILL.md frontmatter is unterminated (missing closing ---)")
    fm = content[4:end]
    if not _FRONTMATTER_NAME_RE.search(fm):
        return (False, "SKILL.md frontmatter missing required field: name")
    if not _FRONTMATTER_DESCRIPTION_RE.search(fm):
        return (False, "SKILL.md frontmatter missing required field: description")
    return (True, "")


def check_size_cap(content: str, cap: int = _LOGMIND_SKILL_BYTE_CAP) -> Tuple[bool, str]:
    """Soft size cap — warns + fails if skill is unreasonably large."""
    size = len(content.encode("utf-8"))
    if size > cap:
        return (
            False,
            f"SKILL.md is {size} bytes — over the {cap}-byte logmind cap. "
            f"Large skills bloat every agent load. Consider splitting "
            f"into multiple focused skills."
        )
    return (True, f"{size} bytes (cap: {cap})")
