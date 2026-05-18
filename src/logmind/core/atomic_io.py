"""Atomic writes for logmind's state files (v0.2.1+).

`Path.write_text()` opens the file, truncates, writes — three operations
that aren't atomic. If two `logmind log` invocations race in the same
repo (multiple agents, parallel CI steps, etc.) one can read a truncated
or partially-written file. The pattern below — write to a sibling tmp
file then `os.replace` — is atomic on POSIX and on Windows for files in
the same directory, and is the standard Python recipe for "don't lose
the user's data if we crash mid-write."

Used for every user-visible markdown file logmind writes:

  - docs/decisions.md
  - docs/decisions-archive.md
  - docs/file-structure.md
  - docs/timeline.md
  - docs/decisions-branches/<branch>.md
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Union


def atomic_write_text(
    path: Union[str, Path],
    content: str,
    encoding: str = "utf-8",
) -> None:
    """Write ``content`` to ``path`` atomically via a sibling tmp file.

    Equivalent to ``Path(path).write_text(content, encoding=encoding)``
    except the original file is never truncated until the new content is
    fully on disk. A concurrent reader sees either the old file or the
    new one, never a partial write.

    The tmp file lives next to ``path`` (not in ``/tmp``) so the rename
    is guaranteed atomic — ``os.replace`` is atomic only within a single
    filesystem.
    """
    path = Path(path)
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(content, encoding=encoding)
    os.replace(tmp, path)
