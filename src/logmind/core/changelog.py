"""CHANGELOG parsing — extract sections between two versions for upgrade
prompts.

When logmind init refreshes a repo's workflows from an older pinned
version to the current installed logmind, agents need to see what
changed. The repo's AGENTS.md gets refreshed in place; agent session
memory does not. Printing the CHANGELOG entries between old and new
version puts the actual behavior changes into command output, where
agents observing the init invocation will see them inline.

The CHANGELOG.md file is at repo root in the source tree. For installed
wheels it's bundled at package root via pyproject.toml package-data.
The lookup falls back to the repo-root copy for editable installs.
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import List, Optional, Tuple

import logmind


_VERSION_HEADER_RE = re.compile(r"^##\s*\[(\d+\.\d+\.\d+(?:[\w.+\-]*)?)\]")


def _changelog_path() -> Optional[Path]:
    """Find the CHANGELOG.md file. Prefer the bundled package copy
    (wheel installs); fall back to the repo-root copy for editable
    installs from source.
    """
    pkg_dir = Path(logmind.__file__).resolve().parent
    bundled = pkg_dir / "CHANGELOG.md"
    if bundled.exists():
        return bundled
    # Editable install: src/logmind/ → repo root is two levels up.
    repo_root = pkg_dir.parent.parent
    dev = repo_root / "CHANGELOG.md"
    if dev.exists():
        return dev
    return None


def _parse_version(v: str) -> Tuple[int, ...]:
    """Parse a semver-ish string into a comparable tuple. Non-numeric
    components sort lowest (so e.g. 0.2.10-rc1 < 0.2.10)."""
    parts: List[int] = []
    for chunk in v.split("."):
        try:
            parts.append(int(chunk))
        except ValueError:
            # First non-int chunk → stop comparing numerically; treat
            # this as a pre-release suffix that sorts before the stable.
            return tuple(parts) + (-1,)
    return tuple(parts)


def extract_sections_between(
    changelog_text: str, *, after: Optional[str], up_to: str
) -> str:
    """Return the substring of CHANGELOG.md containing every `## [X.Y.Z]`
    section S such that ``after < S.version <= up_to``.

    ``after=None`` means "everything up to and including up_to".

    Sections are emitted in source order (most recent first, as the
    canonical CHANGELOG is laid out). The trailing newline / blank line
    of the last included section is preserved.
    """
    if after is not None and _parse_version(up_to) <= _parse_version(after):
        return ""  # caller is on or ahead of the target

    lines = changelog_text.splitlines(keepends=True)
    out: List[str] = []
    in_section = False
    for line in lines:
        m = _VERSION_HEADER_RE.match(line)
        if m:
            version = m.group(1)
            v_tuple = _parse_version(version)
            include = v_tuple <= _parse_version(up_to) and (
                after is None or v_tuple > _parse_version(after)
            )
            if include:
                in_section = True
                out.append(line)
                continue
            # If we encounter a section we don't want AFTER having
            # already collected some, we're done — sections are in
            # descending version order.
            if out:
                break
            in_section = False
            continue
        if in_section:
            out.append(line)
    return "".join(out).rstrip() + "\n" if out else ""


def render_upgrade_prompt(
    *, prior_version: Optional[str], current_version: str
) -> Optional[str]:
    """Compose the printed block for an upgrade prompt. Returns None if
    no upgrade applies (e.g. same version, or CHANGELOG not findable)."""
    if prior_version == current_version:
        return None
    path = _changelog_path()
    if path is None:
        return None
    try:
        text = path.read_text(encoding="utf-8")
    except OSError:
        return None
    body = extract_sections_between(text, after=prior_version, up_to=current_version)
    if not body.strip():
        return None
    header_prior = prior_version if prior_version else "first install"
    return (
        f"\n📋 What's new in logmind since v{header_prior} "
        f"(currently installed: v{current_version}):\n\n"
        f"{body}"
    )
