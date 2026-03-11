"""Multi-project decision aggregation for logmind."""

from pathlib import Path
from typing import Dict, List, NamedTuple, Optional
from datetime import datetime

from logmind.core.parser import iter_decisions


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
        for dt, title in iter_decisions(path):
            entries.append(
                AggregatedEntry(
                    date=dt,
                    title=title,
                    project=name,
                    project_path=project_path,
                    source=source,
                )
            )

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
