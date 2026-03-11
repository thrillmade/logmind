"""Analytics for logmind decision logs."""

import re
from collections import Counter, defaultdict
from datetime import datetime
from pathlib import Path
from typing import Dict, List, NamedTuple, Optional, Tuple


_DECISION_HEADER = re.compile(r"^## (\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}) - (.+)$")


class DecisionEntry(NamedTuple):
    date: datetime
    title: str
    source: str  # "recent" or "archive"


def parse_decisions(docs_path: Path, include_archive: bool = True) -> List[DecisionEntry]:
    """Parse all decisions from decisions.md and optionally decisions-archive.md."""
    entries: List[DecisionEntry] = []

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
                    entries.append(DecisionEntry(date=dt, title=title, source=source))
                except ValueError:
                    pass

    return sorted(entries, key=lambda e: e.date)


def decisions_by_month(entries: List[DecisionEntry]) -> Dict[str, int]:
    """Return a dict of 'YYYY-MM' -> count, ordered chronologically."""
    counts: Dict[str, int] = {}
    for entry in entries:
        key = entry.date.strftime("%Y-%m")
        counts[key] = counts.get(key, 0) + 1
    return counts


def top_keywords(entries: List[DecisionEntry], n: int = 10) -> List[Tuple[str, int]]:
    """Return the n most common meaningful words across all decision titles."""
    _STOP = {
        "a", "an", "the", "to", "for", "of", "in", "on", "at", "by", "and",
        "or", "is", "as", "be", "use", "add", "with", "from", "into", "via",
        "when", "that", "this", "it", "its", "not", "no", "so", "than", "then",
        "was", "are", "were", "has", "have", "had", "will", "would", "can",
        "make", "using", "based", "support", "all", "new", "only", "also",
    }
    counter: Counter = Counter()
    for entry in entries:
        words = re.findall(r"[a-zA-Z]{3,}", entry.title.lower())
        counter.update(w for w in words if w not in _STOP)
    return counter.most_common(n)


def ascii_bar_chart(counts: Dict[str, int], width: int = 30) -> str:
    """Render a horizontal ASCII bar chart from a dict of label -> count."""
    if not counts:
        return "  (no data)"
    max_val = max(counts.values())
    lines = []
    for label, val in counts.items():
        bar_len = int((val / max_val) * width) if max_val else 0
        bar = "█" * bar_len
        lines.append(f"  {label}  {bar} {val}")
    return "\n".join(lines)


def compute_stats(docs_path: Path) -> dict:
    """Compute all analytics statistics."""
    entries = parse_decisions(docs_path, include_archive=True)
    recent_entries = [e for e in entries if e.source == "recent"]

    by_month = decisions_by_month(entries)
    keywords = top_keywords(entries)

    # Velocity: decisions in last 30 days vs prior 30 days
    now = datetime.now()
    recent_30 = sum(1 for e in entries if (now - e.date).days <= 30)
    prior_30 = sum(1 for e in entries if 30 < (now - e.date).days <= 60)

    # Most active month
    most_active = max(by_month, key=by_month.get) if by_month else None

    return {
        "total": len(entries),
        "recent_count": len(recent_entries),
        "archive_count": len(entries) - len(recent_entries),
        "by_month": by_month,
        "keywords": keywords,
        "velocity_30": recent_30,
        "velocity_prior_30": prior_30,
        "most_active_month": most_active,
        "most_active_count": by_month.get(most_active, 0) if most_active else 0,
    }
