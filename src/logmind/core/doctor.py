"""Stack-status command — read installed versions and workflow template markers
across logmind + clud-bug, probe PyPI/npm for the latest, report drift.

Read-only by design: doctor never modifies state. The suggested action when
drift is found (`pip install --upgrade logmind && logmind init`) is printed,
not executed.
"""

from __future__ import annotations

import json
import re
import urllib.error
import urllib.request
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Dict, List, Optional

import logmind  # for resolving the bundled templates/ directory


# Bundled templates live next to the package — resolve via __file__ so this
# works on Python 3.8 (where importlib.resources.files() is not yet available)
# and avoids pulling in the importlib_resources backport.
_TEMPLATES_DIR = Path(logmind.__file__).resolve().parent / "templates" / "github"


LOGMIND_WORKFLOWS = (
    "regen-timeline.yml",
    "check-doc-links.yml",
    "logmind-self-update.yml",
    "check-decisions.yml",
)
CLUD_BUG_WORKFLOWS = (
    "clud-bug-review.yml",
    "clud-bug-audit.yml",
    "clud-bug-self-update.yml",
)

_LOGMIND_PIN_RE = re.compile(r'pip install\s+"?logmind==([\d.]+)"?')
_LOGMIND_MARKER_RE = re.compile(r"^# logmind-template-version:\s*(\S+)")
_CLUD_BUG_MARKER_RE = re.compile(r"^# clud-bug-template-version:\s*(\S+)")


@dataclass
class WorkflowStatus:
    """One workflow file's marker state vs. what we ship today."""

    name: str
    installed: bool  # the file exists in .github/workflows/
    marker: Optional[str]  # e.g. "v1", or None for markerless / missing
    bundled_marker: Optional[str]  # what the shipped template carries today
    drift: str  # "current" | "stale" | "markerless" | "missing" | "unknown"


@dataclass
class ToolStatus:
    name: str
    installed_version: Optional[str]  # parsed from workflow pin or config
    latest_version: Optional[str]  # from PyPI / npm, None when offline or unreachable
    workflows: List[WorkflowStatus] = field(default_factory=list)
    drift: str = "ok"  # "ok" | "stale" | "unknown"
    extras: Dict[str, str] = field(default_factory=dict)


@dataclass
class StatusReport:
    project_root: Path
    tools: List[ToolStatus]
    overall: str  # "OK" | "DRIFT" | "UNKNOWN"
    network_used: bool
    suggestions: List[str] = field(default_factory=list)

    def to_json(self) -> str:
        return json.dumps(
            {
                "project_root": str(self.project_root),
                "tools": [asdict(t) for t in self.tools],
                "overall": self.overall,
                "network_used": self.network_used,
                "suggestions": self.suggestions,
            },
            indent=2,
        )


# ---------------------------------------------------------------------------
# HTTP — best-effort, never raises
# ---------------------------------------------------------------------------


def _http_get_json(url: str, timeout: float = 2.0) -> Optional[dict]:
    """Best-effort JSON GET. Returns None on any network / parse failure."""
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "logmind-doctor"})
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except (
        urllib.error.URLError,
        urllib.error.HTTPError,
        OSError,
        ValueError,  # JSONDecodeError subclass
        TimeoutError,
    ):
        return None


# ---------------------------------------------------------------------------
# Bundled marker discovery
# ---------------------------------------------------------------------------


def _bundled_logmind_marker(workflow_name: str) -> Optional[str]:
    """Read the marker from the shipped template under templates/github/."""
    template_path = _TEMPLATES_DIR / f"{workflow_name}.template"
    try:
        text = template_path.read_text(encoding="utf-8")
    except (FileNotFoundError, OSError):
        return None
    first_line = text.splitlines()[0] if text else ""
    m = _LOGMIND_MARKER_RE.match(first_line)
    return m.group(1) if m else None


# ---------------------------------------------------------------------------
# Workflow probing
# ---------------------------------------------------------------------------


def _read_workflow(project_root: Path, name: str) -> Optional[str]:
    p = project_root / ".github" / "workflows" / name
    try:
        return p.read_text(encoding="utf-8")
    except (FileNotFoundError, OSError):
        return None


def _classify(marker: Optional[str], bundled: Optional[str]) -> str:
    if marker is None:
        return "markerless"
    if bundled is None:
        return "unknown"
    if marker == bundled:
        return "current"
    return "stale"


def _probe_workflow(
    project_root: Path,
    name: str,
    marker_re: re.Pattern,
    bundled_marker: Optional[str],
) -> WorkflowStatus:
    content = _read_workflow(project_root, name)
    if content is None:
        return WorkflowStatus(
            name=name,
            installed=False,
            marker=None,
            bundled_marker=bundled_marker,
            drift="missing",
        )
    first_line = content.splitlines()[0] if content else ""
    m = marker_re.match(first_line)
    marker = m.group(1) if m else None
    return WorkflowStatus(
        name=name,
        installed=True,
        marker=marker,
        bundled_marker=bundled_marker,
        drift=_classify(marker, bundled_marker),
    )


# ---------------------------------------------------------------------------
# logmind probe
# ---------------------------------------------------------------------------


def _logmind_installed_version(project_root: Path) -> Optional[str]:
    """Parse `pip install "logmind==X.Y.Z"` line from regen-timeline.yml.

    Returns None on dogfood installs (no pin — uses `pip install -e .`).
    """
    regen = _read_workflow(project_root, "regen-timeline.yml")
    if not regen:
        return None
    m = _LOGMIND_PIN_RE.search(regen)
    return m.group(1) if m else None


def collect_logmind_status(project_root: Path, *, offline: bool) -> ToolStatus:
    installed = _logmind_installed_version(project_root)
    latest: Optional[str] = None
    if not offline:
        data = _http_get_json("https://pypi.org/pypi/logmind/json")
        if data is not None:
            latest = data.get("info", {}).get("version")

    workflows = [
        _probe_workflow(project_root, name, _LOGMIND_MARKER_RE, _bundled_logmind_marker(name))
        for name in LOGMIND_WORKFLOWS
    ]

    # Drift = any installed workflow with a marker that's stale,
    # OR installed version != latest version (when both known).
    drift = "ok"
    if any(w.drift == "stale" for w in workflows):
        drift = "stale"
    if installed and latest and installed != latest:
        drift = "stale"

    return ToolStatus(
        name="logmind",
        installed_version=installed,
        latest_version=latest,
        workflows=workflows,
        drift=drift,
    )


# ---------------------------------------------------------------------------
# clud-bug probe (optional — only reports if .clud-bug.json exists)
# ---------------------------------------------------------------------------


def _read_clud_bug_config(project_root: Path) -> Optional[dict]:
    p = project_root / ".claude" / "skills" / ".clud-bug.json"
    if not p.exists():
        return None
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError):
        return None


def collect_clud_bug_status(project_root: Path, *, offline: bool) -> Optional[ToolStatus]:
    cfg = _read_clud_bug_config(project_root)
    if cfg is None:
        return None  # clud-bug not installed in this repo

    # `lastUpdateVersion` is the clud-bug release version (e.g. "0.5.6");
    # `version` in this file is the schema version (e.g. `1`) — don't confuse
    # them. Prefer the release field; fall back to schema only if it looks
    # like a semver string (some older installs wrote it there).
    installed = cfg.get("lastUpdateVersion")
    if installed is None:
        v = cfg.get("version")
        if isinstance(v, str) and "." in v:
            installed = v
    latest: Optional[str] = None
    if not offline:
        data = _http_get_json("https://registry.npmjs.org/clud-bug/latest")
        if data is not None:
            latest = data.get("version")

    # Workflow markers — bundled marker for clud-bug isn't accessible from
    # the logmind package, so we can't compare against a known-good baseline.
    # Report what's installed and let the user judge.
    workflows: List[WorkflowStatus] = []
    for name in CLUD_BUG_WORKFLOWS:
        content = _read_workflow(project_root, name)
        if content is None:
            workflows.append(
                WorkflowStatus(
                    name=name, installed=False, marker=None, bundled_marker=None, drift="missing"
                )
            )
            continue
        first_line = content.splitlines()[0] if content else ""
        m = _CLUD_BUG_MARKER_RE.match(first_line)
        marker = m.group(1) if m else None
        workflows.append(
            WorkflowStatus(
                name=name,
                installed=True,
                marker=marker,
                bundled_marker=None,
                drift="current" if marker else "markerless",
            )
        )

    drift = "ok"
    if installed and latest and installed != latest:
        drift = "stale"

    extras: Dict[str, str] = {}
    if "strictMode" in cfg:
        extras["strict_mode"] = "on" if cfg["strictMode"] else "off"

    return ToolStatus(
        name="clud-bug",
        installed_version=installed,
        latest_version=latest,
        workflows=workflows,
        drift=drift,
        extras=extras,
    )


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------


def collect_status(project_root: Optional[Path] = None, *, offline: bool = False) -> StatusReport:
    if project_root is None:
        project_root = Path.cwd()

    tools: List[ToolStatus] = [collect_logmind_status(project_root, offline=offline)]
    clud_bug = collect_clud_bug_status(project_root, offline=offline)
    if clud_bug is not None:
        tools.append(clud_bug)

    overall = "OK"
    if any(t.drift == "stale" for t in tools):
        overall = "DRIFT"
    elif any(t.drift == "unknown" for t in tools):
        overall = "UNKNOWN"

    suggestions: List[str] = []
    for t in tools:
        if t.drift == "stale":
            if t.name == "logmind":
                suggestions.append("pip install --upgrade logmind && logmind init")
            elif t.name == "clud-bug":
                suggestions.append("npx clud-bug update")

    return StatusReport(
        project_root=project_root,
        tools=tools,
        overall=overall,
        network_used=not offline,
        suggestions=suggestions,
    )


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------


def _fmt_version(v: Optional[str], offline: bool) -> str:
    if v is not None:
        return v
    return "(offline)" if offline else "?"


def _fmt_drift(drift: str) -> str:
    return {
        "ok": "current ✓",
        "stale": "STALE",
        "unknown": "unknown",
        "current": "current",
        "markerless": "markerless",
        "missing": "—",
    }.get(drift, drift)


def render_status(report: StatusReport) -> str:
    offline = not report.network_used
    lines: List[str] = []

    for tool in report.tools:
        installed = _fmt_version(tool.installed_version, offline=False)
        latest = _fmt_version(tool.latest_version, offline=offline)
        if tool.installed_version is None and tool.name == "logmind":
            installed = "(dev install)"
        status_word = "current ✓" if tool.drift == "ok" else _fmt_drift(tool.drift)
        lines.append(
            f"{tool.name} {installed} installed · {latest} latest · {status_word}"
        )

        # Workflow table — pad name column to 28 chars
        for wf in tool.workflows:
            if not wf.installed and wf.drift == "missing":
                # Don't spam "—" for not-installed-and-no-bundled-baseline.
                # Only show "not installed" if we ship the template (bundled_marker present).
                if wf.bundled_marker is None:
                    continue
                lines.append(f"  {wf.name:<28} —    not installed (latest: {wf.bundled_marker})")
                continue
            marker = wf.marker or "—"
            drift_word = _fmt_drift(wf.drift)
            if wf.drift == "stale" and wf.bundled_marker is not None:
                drift_word = f"STALE (latest: {wf.bundled_marker})"
            lines.append(f"  {wf.name:<28} {marker:<4} {drift_word}")

        for key, value in tool.extras.items():
            label = key.replace("_", " ")
            lines.append(f"  {label:<28} {value}")

        lines.append("")  # blank line between tools

    lines.append(f"Stack status: {report.overall}")
    if report.suggestions:
        lines.append("Suggested:")
        for s in report.suggestions:
            lines.append(f"  {s}")

    return "\n".join(lines)
