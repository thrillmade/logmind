"""Idempotent management of a marker-bracketed block in ``.gitignore``.

logmind doesn't take ownership of a project's ``.gitignore`` — it appends
exactly one marker-bracketed block of paths that the package itself produces
(local caches, lock files, ...) and leaves everything else untouched.

Once the block is present, manual edits inside it are preserved on every
subsequent run. To intentionally regenerate the canonical content, delete
the marker block from the file and re-run ``logmind init``.
"""

from __future__ import annotations

from pathlib import Path
from typing import Iterable

LOGMIND_GITIGNORE_START = "# >>> logmind >>>"
LOGMIND_GITIGNORE_END = "# <<< logmind <<<"

# The default lines logmind init writes into the managed block.
DEFAULT_GITIGNORE_LINES = (
    ".logmind/cache/",
    ".logmind/.lock",
)


def ensure_block(
    path: Path,
    lines: Iterable[str] = DEFAULT_GITIGNORE_LINES,
) -> bool:
    """
    Ensure ``path`` (a .gitignore) contains the logmind-managed block.

    Returns True if the file was created or modified, False if the block
    was already present (manual edits inside it preserved).
    """
    existing = path.read_text(encoding="utf-8") if path.exists() else ""
    if LOGMIND_GITIGNORE_START in existing:
        return False

    block_parts = [LOGMIND_GITIGNORE_START]
    for line in lines:
        block_parts.append(line)
    block_parts.append(LOGMIND_GITIGNORE_END)
    block = "\n".join(block_parts) + "\n"

    if existing and not existing.endswith("\n"):
        existing += "\n"
    if existing and not existing.endswith("\n\n"):
        existing += "\n"

    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(existing + block, encoding="utf-8")
    return True


def has_block(path: Path) -> bool:
    if not path.exists():
        return False
    return LOGMIND_GITIGNORE_START in path.read_text(encoding="utf-8")
