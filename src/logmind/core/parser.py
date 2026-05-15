"""Shared decision file parsing utilities."""

import re
from datetime import datetime
from pathlib import Path
from typing import Generator, Tuple

DECISION_HEADER = re.compile(r"^## (\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}) - (.+)$")


def iter_decisions(path: Path) -> Generator[Tuple[datetime, str], None, None]:
    """Yield (datetime, title) tuples from a decision markdown file."""
    if not path.exists():
        return
    for line in path.read_text(encoding="utf-8").splitlines():
        m = DECISION_HEADER.match(line)
        if m:
            try:
                dt = datetime.strptime(f"{m.group(1)} {m.group(2)}", "%Y-%m-%d %H:%M")
                yield dt, m.group(3)
            except ValueError:
                pass
