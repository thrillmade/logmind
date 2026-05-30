"""Markdown link integrity checker.

Walks a configurable set of root files/directories, parses every relative
``[text](target)`` markdown link, and reports two failure modes:

- **Broken**: the link target does not exist on disk.
- **Orphan**: a tracked ``.md`` file under ``docs/`` that no other tracked
  file links to AND is not on an allowlist.

Importable for unit testing; ``main()`` is the entry point for the
``logmind check-links`` CLI subcommand and the matching GitHub Action.
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path
from typing import Iterable, List, Optional, Set, Tuple

LINK_PATTERN = re.compile(r"\[([^\]]+)\]\(([^)\s]+)\)")

DEFAULT_ALLOW_ORPHANS = (
    "docs/decisions.md",
    "docs/decisions-archive.md",
    "docs/file-structure.md",
    # The whole branch-decisions tree is aggregator-managed: files appear
    # during a feature branch's lifetime and are linked into decisions.md
    # only on PR merge. Trailing slash → directory prefix (any .md under it).
    "docs/decisions-branches/",
)

DEFAULT_ROOTS = ("README.md", "AGENTS.md", "CLAUDE.md", "docs")

_EXTERNAL_PREFIXES = ("http://", "https://", "mailto:", "ftp://", "//", "#")


def _is_external(target: str) -> bool:
    return target.startswith(_EXTERNAL_PREFIXES)


def _strip_anchor(target: str) -> str:
    return target.split("#", 1)[0]


def _collect_md_files(roots: Iterable[Path]) -> Set[Path]:
    found: Set[Path] = set()
    for root in roots:
        if not root.exists():
            continue
        if root.is_file() and root.suffix == ".md":
            found.add(root.resolve())
        elif root.is_dir():
            for p in root.rglob("*.md"):
                found.add(p.resolve())
    return found


# v0.5.9 / issue #60 — strip code regions before scanning for links.
# Pre-v0.5.9, fenced code blocks (``` ... ```) and inline-code spans
# (`...`) were scanned as live content, so any `[text](path)` example
# inside backticks tripped the link checker. This made it impossible
# to *talk about* markdown link syntax in prose or decision-log entries
# without breaking CI. Every other markdown linter (markdown-link-check,
# lychee, etc.) skips code regions because that's specifically where
# you discuss the syntax. Now logmind matches that convention.
#
# Order matters: strip FENCED blocks first (they can contain backticks
# that would confuse the inline-code regex), then inline-code spans.
_FENCED_CODE_BLOCK = re.compile(
    r"^```.*?^```",
    re.DOTALL | re.MULTILINE,
)
# Match 1+ backticks for the delimiter, then non-backtick content, then
# the same delimiter. Handles ``code with ` inside`` correctly (per
# CommonMark §6.1 the delimiter length must match).
#
# v0.5.9 PR #83 review fix: NO re.DOTALL — an unmatched stray backtick
# (common in informal prose, e.g. mid-sentence `foo) would otherwise
# match all the way to the next backtick paragraphs later, consuming
# real broken links along the way. Restricting to non-newline keeps
# code spans to a single line, matching CommonMark's common case and
# leaving cross-line code blocks to the (greedy, intentional) fenced-
# block regex. The cost: a CommonMark-compliant multi-line code span
# isn't recognized — but in the docs corpus that's vanishingly rare
# and the safer failure mode (false positive on a broken link rather
# than silent suppression of one).
_INLINE_CODE_SPAN = re.compile(r"(`+)(?:(?!\1)[^\n])+?\1")


def _strip_code_regions(text: str) -> str:
    """v0.5.9 / issue #60 — return ``text`` with fenced code blocks and
    inline-code spans replaced by whitespace of equivalent length.

    Whitespace-replacement (vs. deletion) preserves line numbers + byte
    offsets so error messages still point at the correct location in the
    original file when the link scanner reports problems.
    """
    def _to_whitespace(match: "re.Match[str]") -> str:
        # Preserve newlines so line numbers stay correct; replace other
        # chars with spaces.
        return "".join("\n" if c == "\n" else " " for c in match.group(0))

    # Fenced blocks first (multi-line, can contain inline-code patterns).
    text = _FENCED_CODE_BLOCK.sub(_to_whitespace, text)
    # Then inline-code spans.
    text = _INLINE_CODE_SPAN.sub(_to_whitespace, text)
    return text


def _extract_links(md_path: Path) -> List[Tuple[str, str]]:
    try:
        text = md_path.read_text(errors="ignore")
    except OSError:
        return []
    # v0.5.9 / issue #60: strip code regions before link-pattern scan.
    return LINK_PATTERN.findall(_strip_code_regions(text))


def check(
    repo_root: Path,
    roots: Optional[Iterable[Path]] = None,
    allow_orphans: Iterable[str] = DEFAULT_ALLOW_ORPHANS,
) -> Tuple[List[str], List[str]]:
    """
    Run the link check against ``repo_root``.

    Returns ``(broken, orphans)``. Each list is empty on a clean run; both
    lists contain repo-relative path strings (broken includes the link text).
    """
    repo_root = repo_root.resolve()
    if roots is None:
        roots = [repo_root / r for r in DEFAULT_ROOTS]
    else:
        roots = list(roots)

    md_files = _collect_md_files(roots)
    incoming = {p: set() for p in md_files}

    broken: List[str] = []
    for source in md_files:
        for _text, target in _extract_links(source):
            if _is_external(target):
                continue
            stripped = _strip_anchor(target)
            if not stripped:
                # Pure-anchor link to current file; skip.
                continue
            resolved = (source.parent / stripped).resolve()
            if not resolved.exists():
                try:
                    rel_source = source.relative_to(repo_root).as_posix()
                except ValueError:
                    rel_source = source.as_posix()
                broken.append(f"{rel_source}: missing -> {target}")
                continue
            if resolved.suffix == ".md" and resolved in incoming:
                incoming[resolved].add(source)

    orphans: List[str] = []
    for md, sources in incoming.items():
        try:
            rel = md.relative_to(repo_root)
        except ValueError:
            continue
        if not rel.parts or rel.parts[0] != "docs":
            continue
        if _is_allowed_orphan(rel, allow_orphans):
            continue
        if not sources:
            orphans.append(rel.as_posix())

    return sorted(broken), sorted(orphans)


def _is_allowed_orphan(rel_path: Path, allow_orphans: Iterable[str]) -> bool:
    """An entry matches if it equals rel_path OR is a parent directory of it.

    Directory prefixes are signalled by a trailing ``/`` in the entry, but
    we also tolerate plain dir paths (no slash) by checking the exact match
    against parents. Comparison is done in POSIX form so this works on
    Windows where Path renders with backslashes.
    """
    s = rel_path.as_posix()
    for entry in allow_orphans:
        e = entry.rstrip("/")
        if s == e:
            return True
        if s.startswith(e + "/"):
            return True
    return False


def format_report(broken: List[str], orphans: List[str]) -> str:
    lines: List[str] = []
    if broken:
        lines.append(f"Broken links ({len(broken)}):")
        for b in broken:
            lines.append(f"  - {b}")
    if orphans:
        if lines:
            lines.append("")
        lines.append(f"Orphan markdown files ({len(orphans)}):")
        for o in orphans:
            lines.append(f"  - {o}")
    if not lines:
        lines.append("All markdown links resolve and no orphans found.")
    return "\n".join(lines)


def main(argv: Optional[List[str]] = None) -> int:
    """Entry point for ``logmind check-links`` and the GH Action.

    Always operates on the current working directory. GH Actions runs
    ``run:`` steps with cwd set to ``$GITHUB_WORKSPACE`` automatically, so
    we don't need to special-case it (and reading the env var would
    actually defeat tests that monkeypatch.chdir into a temp dir while
    running on a CI runner that has GITHUB_WORKSPACE pointed elsewhere).
    """
    repo_root = Path(os.getcwd())

    # Optional config overrides via .logmind/config.yml
    allow_orphans = list(DEFAULT_ALLOW_ORPHANS)
    roots: Optional[List[Path]] = None
    config_path = repo_root / ".logmind" / "config.yml"
    if config_path.exists():
        try:
            from logmind.core.config import load_config

            cfg = load_config(config_path)
            extra = cfg.get("linkcheck.allow_orphans") or []
            allow_orphans = list(DEFAULT_ALLOW_ORPHANS) + list(extra)
            configured_roots = cfg.get("linkcheck.roots") or []
            if configured_roots:
                roots = [repo_root / r for r in configured_roots]
        except Exception:
            pass  # ignore config errors; defaults still work

    broken, orphans = check(repo_root, roots=roots, allow_orphans=allow_orphans)
    print(format_report(broken, orphans))
    return 0 if not broken and not orphans else 1


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
