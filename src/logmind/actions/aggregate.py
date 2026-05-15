"""Aggregate per-branch decision logs into docs/decisions.md on PR merge.

Designed to be invoked by ``.github/workflows/logmind-aggregate.yml`` after a
pull request is merged. Reads the per-branch decisions file written by Phase 5
and appends a single summary entry to the canonical decisions log so the
default branch's history shows merged work alongside its detail link.

Importable for unit testing; ``main()`` adapts environment variables for use
with ``python -m logmind.actions.aggregate``.
"""

from __future__ import annotations

import os
import sys
from datetime import datetime
from pathlib import Path
from typing import Optional

from logmind.core.logger import _sanitize_branch
from logmind.core.parser import iter_decisions
from logmind.core.tree_gen import update_file_structure


def aggregate(
    branch: str,
    pr_number: int,
    pr_url: str,
    docs_path: Path,
    timestamp: Optional[datetime] = None,
) -> Optional[Path]:
    """
    Append a merge-summary entry to ``docs/decisions.md``.

    Returns the path to the updated file, or ``None`` if the per-branch file is
    missing / empty (in which case there is nothing to summarise and the main
    log is left untouched).
    """
    if timestamp is None:
        timestamp = datetime.now()

    sanitized = _sanitize_branch(branch)
    branch_file = docs_path / "decisions-branches" / f"{sanitized}.md"
    if not branch_file.exists():
        return None

    decision_count = sum(1 for _ in iter_decisions(branch_file))
    if decision_count == 0:
        return None

    rel_link = f"decisions-branches/{sanitized}.md"
    entry = (
        f"## {timestamp.strftime('%Y-%m-%d %H:%M')} - "
        f"Merged: {branch} (#{pr_number})\n"
        "\n"
        f"- **PR:** {pr_url}\n"
        f"- **Decisions:** {decision_count} from this branch\n"
        f"- **Detail:** [{rel_link}]({rel_link})\n"
        "\n"
        "---\n"
    )

    decisions_path = docs_path / "decisions.md"
    existing = decisions_path.read_text(encoding="utf-8") if decisions_path.exists() else ""
    decisions_path.write_text(existing + entry, encoding="utf-8")

    # Regenerate file-structure.md on main as part of the aggregation commit.
    # `logmind log` on feature branches no longer touches this file (v0.1.3),
    # so per-PR conflicts can't happen — main alone owns the tree snapshot.
    try:
        update_file_structure(docs_path)
    except Exception:
        # Don't fail aggregation if tree regen blows up — the decision entry
        # is the load-bearing part.
        pass

    return decisions_path


def main() -> int:
    """Entry point for ``python -m logmind.actions.aggregate``."""
    branch = os.environ.get("BRANCH_NAME") or os.environ.get("GITHUB_HEAD_REF")
    pr_number_str = os.environ.get("PR_NUMBER")
    pr_url = os.environ.get("PR_URL")

    if not branch or not pr_number_str or not pr_url:
        print(
            "logmind aggregate: BRANCH_NAME, PR_NUMBER, and PR_URL must be set",
            file=sys.stderr,
        )
        return 2

    try:
        pr_number = int(pr_number_str)
    except ValueError:
        print(f"logmind aggregate: PR_NUMBER must be an integer, got {pr_number_str!r}",
              file=sys.stderr)
        return 2

    docs_path = Path(os.environ.get("LOGMIND_DOCS", "docs"))
    if not docs_path.is_absolute():
        docs_path = Path.cwd() / docs_path

    result = aggregate(
        branch=branch,
        pr_number=pr_number,
        pr_url=pr_url,
        docs_path=docs_path,
    )

    if result is None:
        print(f"logmind aggregate: no per-branch decisions for {branch}; nothing to do.")
        return 0

    print(f"logmind aggregate: appended merge summary for {branch} to {result}")
    return 0


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
