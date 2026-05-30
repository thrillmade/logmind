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
from typing import Dict, List, Optional, Tuple

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
# AGENTS.md ships a `<!-- logmind-block-version: vN -->` comment marking the
# version of the embedded logmind instructions. Doctor uses it to detect when
# an installed repo's AGENTS.md is older than what the current logmind would
# write (the common cause: agent runtime memory cached a previous AGENTS.md
# revision and the agent is still working from those stale instructions).
_LOGMIND_BLOCK_VERSION_RE = re.compile(
    r"^<!--\s*logmind-block-version:\s*(\S+)\s*-->", re.MULTILINE
)


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


def _bundled_agents_md_block_versions() -> Tuple[Optional[str], Optional[str]]:
    """Return (slim_marker, full_marker) — both bundled AGENTS.md template
    markers, since logmind init writes one OR the other depending on
    whether skills.sh is available at install time (see inserter.py).

    Reporting both lets doctor accept an installed marker that matches
    EITHER variant — otherwise we'd false-positive every full-template
    install as stale against the slim bundled marker.
    """
    def _read_marker(path: Path) -> Optional[str]:
        try:
            text = path.read_text(encoding="utf-8")
        except (FileNotFoundError, OSError):
            return None
        m = _LOGMIND_BLOCK_VERSION_RE.search(text)
        return m.group(1) if m else None

    slim = _read_marker(_TEMPLATES_DIR.parent / "AGENTS.md.slim.template")
    full = _read_marker(_TEMPLATES_DIR.parent / "AGENTS.md.template")
    return slim, full


def _probe_agents_md(project_root: Path) -> WorkflowStatus:
    """AGENTS.md block-version vs bundled template. Reported alongside
    workflow probes because the failure mode is identical — agents work
    from a stale instruction set when the installed repo's AGENTS.md
    block is older than what logmind init would currently write.

    A repo's installed marker may match EITHER the slim or the full
    bundled marker (logmind init writes one or the other based on
    skills.sh availability). We treat the install as current if it
    matches either; stale only if it matches neither.
    """
    agents_path = project_root / "AGENTS.md"
    slim_bundled, full_bundled = _bundled_agents_md_block_versions()
    # Choose one bundled marker for display (the user installed one of
    # them, but we don't know which without reading more context). Prefer
    # slim because it's the more common modern install path.
    display_bundled = slim_bundled or full_bundled
    if not agents_path.exists():
        return WorkflowStatus(
            name="AGENTS.md", installed=False, marker=None,
            bundled_marker=display_bundled, drift="missing",
        )
    try:
        text = agents_path.read_text(encoding="utf-8")
    except OSError:
        return WorkflowStatus(
            name="AGENTS.md", installed=True, marker=None,
            bundled_marker=display_bundled, drift="markerless",
        )
    m = _LOGMIND_BLOCK_VERSION_RE.search(text)
    marker = m.group(1) if m else None
    if marker is None:
        drift = "markerless"
    elif slim_bundled is None and full_bundled is None:
        drift = "unknown"
    elif marker == slim_bundled or marker == full_bundled:
        drift = "current"
    else:
        drift = "stale"
    return WorkflowStatus(
        name="AGENTS.md", installed=True, marker=marker,
        bundled_marker=display_bundled, drift=drift,
    )


def _probe_merge_driver_attrs(project_root: Path) -> WorkflowStatus:
    """v0.3.0: .gitattributes contains the logmind merge-driver block.

    Reports `current` when present, `missing` when absent. We never
    return `stale` here — the block is binary present/absent, and
    treating absence as drift would false-positive every repo that
    hasn't yet run `logmind init` post-v0.3.0 (or every test fixture).
    The next `logmind init` writes the block; that's the signal flow.
    """
    from logmind.core.gitattributes import has_block

    gitattrs = project_root / ".gitattributes"
    if has_block(gitattrs):
        return WorkflowStatus(
            name=".gitattributes (merge driver)", installed=True,
            marker="present", bundled_marker="present", drift="current",
        )
    return WorkflowStatus(
        name=".gitattributes (merge driver)", installed=False,
        marker=None, bundled_marker="present", drift="missing",
    )


def _probe_merge_driver_config(project_root: Path) -> WorkflowStatus:
    """v0.3.0: per-clone git config has the merge driver definitions.

    Per-clone state (lives in .git/config, not committed) — a fresh
    checkout starts without it; `logmind init` sets it. If unset, the
    `.gitattributes` directive does nothing because git refuses to
    invoke an undefined driver. Reports `current` when configured,
    `missing` otherwise (never `stale` — binary).
    """
    from logmind.core.gitattributes import driver_configured

    if not (project_root / ".git").exists():
        return WorkflowStatus(
            name="git config (merge driver)", installed=False,
            marker=None, bundled_marker=None, drift="missing",
        )
    if driver_configured(project_root):
        return WorkflowStatus(
            name="git config (merge driver)", installed=True,
            marker="configured", bundled_marker="configured", drift="current",
        )
    return WorkflowStatus(
        name="git config (merge driver)", installed=False,
        marker=None, bundled_marker="configured", drift="missing",
    )


def _probe_post_merge_hook(project_root: Path) -> WorkflowStatus:
    """v0.3.0: .git/hooks/post-merge installed by logmind. Companion to
    the merge driver — re-regenerates derived files with the full
    post-merge tree (the driver alone can fire before all merged-in
    files are checked out, producing an incomplete regen).
    """
    from logmind.core.gitattributes import post_merge_hook_installed

    if not (project_root / ".git").exists():
        return WorkflowStatus(
            name="post-merge hook", installed=False,
            marker=None, bundled_marker=None, drift="missing",
        )
    if post_merge_hook_installed(project_root):
        return WorkflowStatus(
            name="post-merge hook", installed=True,
            marker="installed", bundled_marker="installed", drift="current",
        )
    return WorkflowStatus(
        name="post-merge hook", installed=False,
        marker=None, bundled_marker="installed", drift="missing",
    )


def check_stale_derived_docs_warning(project_root: Path) -> Optional[str]:
    """v0.5.13 / tokenomics agent #2 — warn when the current branch is
    behind ``origin/<default-branch>`` AND the gap touches
    ``docs/timeline.md`` or ``docs/file-structure.md``.

    Symptom this surfaces: the tokenomics agent's Phase D pain — a PR
    batch where the trailing PRs go ``mergeStateStatus: DIRTY``
    immediately after an earlier PR merges (the merge regenerates a
    derived doc on main; trailing branches now have a textual
    conflict). Running `logmind doctor` early surfaces "your branch
    will go DIRTY on the next main push; consider `logmind rebase`
    now" before the failure manifests.

    Returns a one-line warning string when the condition fires, else
    ``None``. Silent no-op outside a git repo, on the default branch
    itself, when no remote is configured, or when the upstream tracking
    ref is missing.
    """
    import subprocess

    if not (project_root / ".git").exists():
        return None

    # Get current branch via existing helpers (avoid re-implementing).
    try:
        from logmind.core.git_handler import current_branch, default_branch
    except ImportError:  # pragma: no cover — defensive only
        return None

    branch = current_branch(project_root)
    if branch is None:
        return None  # detached HEAD
    default = default_branch(project_root)
    if branch == default:
        return None  # we ARE the default branch

    # Files we care about. Hard-coded to the canonical set; if a user
    # adds custom derived docs they can extend this list via a future
    # config knob, not for v0.5.13.
    derived_docs = ("docs/timeline.md", "docs/file-structure.md")

    # Resolve `origin/<default>` — graceful no-op if origin isn't set.
    upstream = f"origin/{default}"
    try:
        subprocess.run(
            ["git", "-C", str(project_root), "rev-parse", "--verify", "--quiet", upstream],
            capture_output=True,
            check=True,
        )
    except (subprocess.CalledProcessError, FileNotFoundError, OSError):
        return None  # no remote / not yet fetched

    # List files touched by commits on origin/<default> that aren't on
    # this branch. `--format=` suppresses commit messages so the output
    # is just file paths (deduplicated by set membership downstream).
    try:
        result = subprocess.run(
            ["git", "-C", str(project_root), "log",
             f"{branch}..{upstream}", "--name-only", "--format="],
            capture_output=True,
            text=True,
            check=True,
        )
    except (subprocess.CalledProcessError, FileNotFoundError, OSError):
        return None

    touched_files = {line for line in result.stdout.splitlines() if line}
    overlap = sorted(touched_files & set(derived_docs))
    if not overlap:
        return None

    # Count commits in the gap so the warning quantifies "how stale."
    try:
        count_result = subprocess.run(
            ["git", "-C", str(project_root), "rev-list", "--count",
             f"{branch}..{upstream}"],
            capture_output=True,
            text=True,
            check=True,
        )
        n_commits = count_result.stdout.strip() or "?"
    except (subprocess.CalledProcessError, FileNotFoundError, OSError):
        n_commits = "?"

    return (
        f"⚠ '{branch}' is {n_commits} commits behind {upstream} AND the gap "
        f"touches {', '.join(overlap)}. Next push will likely DIRTY this "
        f"PR. Consider `logmind rebase` now."
    )


def _probe_post_rewrite_hook(project_root: Path) -> WorkflowStatus:
    """v0.5.11 / issue #58: .git/hooks/post-rewrite installed by logmind.
    Companion to the post-merge hook. Fires after `git rebase` or
    `git commit --amend` (which the merge driver and post-merge hook
    don't cover), regenerating derived docs against the post-rewrite
    tree so multi-commit rebases don't leave docs/timeline.md stale.
    """
    from logmind.core.gitattributes import post_rewrite_hook_installed

    if not (project_root / ".git").exists():
        return WorkflowStatus(
            name="post-rewrite hook", installed=False,
            marker=None, bundled_marker=None, drift="missing",
        )
    if post_rewrite_hook_installed(project_root):
        return WorkflowStatus(
            name="post-rewrite hook", installed=True,
            marker="installed", bundled_marker="installed", drift="current",
        )
    return WorkflowStatus(
        name="post-rewrite hook", installed=False,
        marker=None, bundled_marker="installed", drift="missing",
    )


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
    # AGENTS.md block-version is reported in the same workflows list so
    # downstream rendering / drift aggregation treats it uniformly.
    workflows.append(_probe_agents_md(project_root))
    # v0.3.0: merge-driver config drift. Two signals:
    #   .gitattributes block missing → committed config absent (would
    #     reappear on next logmind init, but worth surfacing)
    #   per-clone git config unset → driver won't fire on local rebase
    workflows.append(_probe_merge_driver_attrs(project_root))
    workflows.append(_probe_merge_driver_config(project_root))
    workflows.append(_probe_post_merge_hook(project_root))
    workflows.append(_probe_post_rewrite_hook(project_root))

    # Drift = any installed workflow with a marker that's stale,
    # OR installed version != latest version (when both known),
    # OR (v0.5.13) merge-driver config / post-merge hook /
    # post-rewrite hook missing inside a git repo. The "merge driver
    # missing" cases default to "missing" (not "stale") per their
    # probe contract, but in a git repo they indicate the local
    # clone hasn't run `logmind init` yet and rebases/merges on
    # derived docs will silently fall back to git's textual merge.
    # Treating that as drift means CI gates surface the missing
    # config before it produces a check-derived-docs failure.
    drift = "ok"
    if any(w.drift == "stale" for w in workflows):
        drift = "stale"
    if installed and latest and installed != latest:
        drift = "stale"
    # v0.5.13: a git repo without merge-driver config / hooks is
    # one merge away from a check-derived-docs failure. Surface as
    # drift so `logmind doctor` exits non-zero on CI.
    if (project_root / ".git").exists():
        critical_missing_names = {
            "git config (merge driver)",
            "post-merge hook",
            "post-rewrite hook",
        }
        for w in workflows:
            if w.name in critical_missing_names and w.drift == "missing":
                drift = "stale"
                break

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

    # v0.5.13 / tokenomics agent #2 — surface the stale-derived-docs
    # warning as a suggestion line. Non-fatal (doesn't flip overall to
    # DRIFT) since it's a predictive heads-up, not a current failure.
    stale_warning = check_stale_derived_docs_warning(project_root)
    if stale_warning:
        suggestions.append(stale_warning)

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
