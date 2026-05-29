"""Shared decision file parsing utilities."""

import re
import sys
from datetime import datetime
from pathlib import Path
from typing import Generator, Tuple

DECISION_HEADER = re.compile(r"^## (\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}) - (.+)$")


def iter_decisions(path: Path) -> Generator[Tuple[datetime, str], None, None]:
    """Yield (datetime, title) tuples from a decision markdown file.

    Headers that match the structure but carry a malformed date/time
    (e.g. "2026-13-45 25:99 - title") are SKIPPED with a stderr warning
    rather than silently dropped. Pattern: RTK-inspired (0.0.T) —
    fail-safe → loud rather than silent on parse failure.

    Missing files return empty (this is expected when iterating
    optional decision-branch files).
    """
    if not path.exists():
        return
    text = path.read_text(encoding="utf-8")
    for lineno, line in enumerate(text.splitlines(), start=1):
        m = DECISION_HEADER.match(line)
        if not m:
            continue
        try:
            dt = datetime.strptime(f"{m.group(1)} {m.group(2)}", "%Y-%m-%d %H:%M")
        except ValueError as e:
            # Header matched the structure but the date/time is
            # malformed. Warn loudly so the user can fix the source
            # file; do NOT yield anything for this entry.
            print(
                f"  ! logmind: skipping malformed decision header at {path}:{lineno}: {e}",
                file=sys.stderr,
            )
            continue
        yield dt, m.group(3)
