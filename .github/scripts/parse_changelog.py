#!/usr/bin/env python3
"""Standalone CHANGELOG section extractor — vendored for
`notify-agent-skills.yml` after PR #137 deleted the Python source tree
(including `logmind.core.changelog`).

This script reproduces ONLY what the workflow needed from the deleted
`extract_sections_between` function: slice the CHANGELOG markdown to
the `## [X.Y.Z]` sections between a `--after` tag and a `--up-to` tag.

Long-term migration path: add `logmind changelog --since v<prev>` as a
Go subcommand registered in `internal/cli/root.go`, then replace this
script + the `python3` invocation in `notify-agent-skills.yml` with a
single binary call. Tracked as a follow-up — this script is the
stopgap for the v1.0.x release wave.

Input file resolution:
  1. Prefer `docs/changelog-python.md` (PR #137's new home).
  2. Fall back to top-level `CHANGELOG.md` (pre-#137 location) so the
     script remains correct on older refs the workflow might still
     check out via `actions/checkout` (e.g. re-running on an older
     tag).

Usage:
  python3 .github/scripts/parse_changelog.py \\
    --since v0.6.16 \\
    --up-to v1.0.0 \\
    --out /tmp/notify/changelog-section.md

  # --since may be omitted on the very first release; the script will
  # then emit every section up to and including --up-to.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

# Mirrors logmind.core.changelog._VERSION_HEADER_RE. Matches the
# `## [X.Y.Z]` header at the start of a line; trailing date suffix
# (` - 2026-06-03`) is ignored. Pre-release suffixes like `1.0.0-rc1`
# are accepted via the optional `[\w.+\-]*` tail.
_VERSION_HEADER_RE = re.compile(r"^##\s*\[(\d+\.\d+\.\d+(?:[\w.+\-]*)?)\]")


def _parse_version(v: str) -> tuple[int, ...]:
    """Parse a semver-ish string into a comparable tuple. Non-numeric
    components (pre-release suffixes) collapse to a -1 sentinel so e.g.
    0.2.10-rc1 sorts before 0.2.10. Mirrors the deleted Python helper.
    """
    parts: list[int] = []
    for chunk in v.split("."):
        try:
            parts.append(int(chunk))
        except ValueError:
            # First non-int chunk → stop comparing numerically; treat
            # as a pre-release suffix that sorts before the stable.
            return tuple(parts) + (-1,)
    return tuple(parts)


def extract_sections_between(
    changelog_text: str, *, after: str | None, up_to: str
) -> str:
    """Return the substring of CHANGELOG containing every `## [X.Y.Z]`
    section S such that ``after < S.version <= up_to``.

    `after=None` means "everything up to and including up_to".

    Sections are emitted in source order. The CHANGELOG is laid out
    most-recent-first, so once we step OUT of the included range we
    can break — earlier sections are necessarily older.
    """
    if after is not None and _parse_version(up_to) <= _parse_version(after):
        return ""  # caller is on or ahead of the target

    lines = changelog_text.splitlines(keepends=True)
    out: list[str] = []
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
            # Stepped OUT of the included range after collecting at
            # least one section → done (sections are descending).
            if out:
                break
            in_section = False
            continue
        if in_section:
            out.append(line)
    return "".join(out).rstrip() + "\n" if out else ""


def _resolve_changelog_path(repo_root: Path) -> Path:
    """Prefer the new docs/changelog-python.md location (PR #137);
    fall back to top-level CHANGELOG.md for older checkouts. Fail
    loudly if neither exists — the workflow should not silently
    produce an empty section.
    """
    candidates = (
        repo_root / "docs" / "changelog-python.md",
        repo_root / "CHANGELOG.md",
    )
    for path in candidates:
        if path.exists():
            return path
    paths_str = ", ".join(str(p) for p in candidates)
    raise FileNotFoundError(
        f"CHANGELOG file not found at any of: {paths_str}. "
        "Did the file move again? Update _resolve_changelog_path."
    )


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Extract CHANGELOG sections between two version tags."
    )
    parser.add_argument(
        "--since",
        default="",
        help=(
            "Previous version tag (e.g. v0.6.16). Sections strictly newer "
            "than this are included. Omit / empty string = include "
            "everything up to --up-to (first-release case)."
        ),
    )
    parser.add_argument(
        "--up-to",
        required=True,
        help="Current version tag (e.g. v1.0.0). Inclusive upper bound.",
    )
    parser.add_argument(
        "--out",
        type=Path,
        required=True,
        help="File path to write the extracted section to.",
    )
    parser.add_argument(
        "--repo-root",
        type=Path,
        default=Path.cwd(),
        help=(
            "Repository root to search for the CHANGELOG file. "
            "Defaults to the current working directory (matches "
            "actions/checkout's default workspace)."
        ),
    )
    args = parser.parse_args()

    # Strip leading `v` from both tags before passing into the version
    # comparator, which expects bare X.Y.Z (matches the Python
    # workflow's prior behavior).
    after = args.since.lstrip("v") if args.since else None
    up_to = args.up_to.lstrip("v")

    changelog_path = _resolve_changelog_path(args.repo_root)
    text = changelog_path.read_text(encoding="utf-8")
    section = extract_sections_between(text, after=after, up_to=up_to)

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(section, encoding="utf-8")
    print(
        f"  Read CHANGELOG from: {changelog_path}\n"
        f"  Extracted {len(section)} chars of CHANGELOG section "
        f"(after={after or '<none>'}, up_to={up_to})"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
