"""Multi-project decision aggregation for logmind."""

import re
from pathlib import Path
from typing import Dict, List, NamedTuple, Optional
from datetime import datetime


_DECISION_HEADER = re.compile(r"^## (\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}) - (.+)$")


class AggregatedEntry(NamedTuple):
    date: datetime
    title: str
    project: str       # project name (directory basename)
    project_path: Path
    source: str        # "recent" or "archive"


def _project_name(project_path: Path) -> str:
    """Return a short display name for a project path."""
    return project_path.resolve().name


def load_project_decisions(
    project_path: Path,
    include_archive: bool = True,
) -> List[AggregatedEntry]:
    """Load all decisions from a single project's docs/ directory."""
    docs_path = project_path / "docs"
    if not docs_path.exists():
        return []

    name = _project_name(project_path)
    entries: List[AggregatedEntry] = []

    files = [("recent", docs_path / "decisions.md")]
    if include_archive:
        files.append(("archive", docs_path / "decisions-archive.md"))

    for source, path in files:
        if not path.exists():
            continue
        for line in path.read_text().splitlines():
            m = _DECISION_HEADER.match(line)
            if m:
                date_str, time_str, title = m.group(1), m.group(2), m.group(3)
                try:
                    dt = datetime.strptime(f"{date_str} {time_str}", "%Y-%m-%d %H:%M")
                    entries.append(
                        AggregatedEntry(
                            date=dt,
                            title=title,
                            project=name,
                            project_path=project_path,
                            source=source,
                        )
                    )
                except ValueError:
                    pass

    return entries


def aggregate_projects(
    project_paths: List[Path],
    include_archive: bool = True,
    limit: Optional[int] = None,
) -> List[AggregatedEntry]:
    """Aggregate decisions across multiple projects, sorted newest-first."""
    all_entries: List[AggregatedEntry] = []
    for path in project_paths:
        all_entries.extend(load_project_decisions(path, include_archive=include_archive))

    all_entries.sort(key=lambda e: e.date, reverse=True)

    if limit is not None:
        all_entries = all_entries[:limit]

    return all_entries


def project_summary(project_paths: List[Path]) -> Dict[str, int]:
    """Return a mapping of project name -> total decision count."""
    summary: Dict[str, int] = {}
    for path in project_paths:
        entries = load_project_decisions(path, include_archive=True)
        summary[_project_name(path)] = len(entries)
    return summary
