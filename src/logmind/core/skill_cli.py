"""v0.6.0 — `logmind skill new/test` CLI implementations.

Composes with Zak Elfassi's ``@zakelfassi/skdd`` (the canonical SkDD
methodology toolkit) when present on PATH:

- ``logmind skill new <name>``: prefers ``skdd forge <name>`` if available;
  otherwise scaffolds a basic SKILL.md against the agentskills.io/v1 spec.
- ``logmind skill test <name>``: prefers ``skdd validate <name>``; layers
  logmind-specific checks (size cap, frontmatter presence) on top.

Design (per explore-agent strategic analysis, 2026-05-30):
- Don't reimplement Zak's ``forge`` / ``validate`` if the upstream CLI is
  available. Wire to it.
- Inject logmind-specific behavior AFTER his CLI does its work:
  decision-log the skill creation; layer additional validation.
- Spec compliance: SKILL.md frontmatter follows agentskills.io/v1
  (``name``, ``description`` required; ``metadata`` optional).

Scope:
- v0.6.0: ``new`` + ``test`` (minimum viable; ships the loop's first
  two arrows).
- v0.6.1: ``bench`` (token-cost measurement per skill).
- v0.6.2: ``log`` (decision-log a skill iteration, automatable from a bot).
"""

from __future__ import annotations

import re
import shutil
import subprocess
from pathlib import Path
from typing import Optional, Tuple


# Per agentskills.io/v1 spec: required fields are `name` + `description`.
# `metadata` is optional; we add a logmind-specific marker inside it.
_BASIC_SKILL_TEMPLATE = """---
name: {name}
description: {description}
metadata:
  logmind-managed: true
  spec: agentskills.io
---

# {title}

<!-- One short paragraph describing what this skill does + when an agent
should load it. The description field above is the discovery surface;
this body is what the agent reads once they decide to load it. -->

## When to use

- Trigger condition 1
- Trigger condition 2

## Steps

1. Step one
2. Step two

## Examples

<!-- Concrete examples make skills easier to apply correctly. -->
"""


def has_skdd() -> bool:
    """Return True iff Zak's ``skdd`` CLI is on PATH."""
    return shutil.which("skdd") is not None


def default_skills_dir(repo_root: Path) -> Path:
    """The canonical location for SKILL.md files in a repo.

    Defaults to ``.claude/skills/`` — the Claude Code convention that
    clud-bug and the agent-skills catalog use. Other harnesses (Cursor,
    Codex) follow the same per-skill subdirectory layout, so this works
    for any consumer.
    """
    return repo_root / ".claude" / "skills"


def skill_dir(repo_root: Path, name: str) -> Path:
    """Return the canonical directory for a skill of the given name."""
    return default_skills_dir(repo_root) / name


def skill_md_path(repo_root: Path, name: str) -> Path:
    return skill_dir(repo_root, name) / "SKILL.md"


def scaffold_basic_skill(
    repo_root: Path, name: str, description: str = ""
) -> Path:
    """Scaffold a basic agentskills.io/v1-compliant SKILL.md.

    Used when ``skdd`` is not on PATH. Creates the skill directory + a
    minimal SKILL.md with the required frontmatter fields.

    Raises ``FileExistsError`` if the skill already exists (refuse to
    clobber).
    """
    target = skill_md_path(repo_root, name)
    if target.exists():
        raise FileExistsError(f"Skill '{name}' already exists at {target}")

    target.parent.mkdir(parents=True, exist_ok=True)
    if not description:
        description = (
            f"TODO: one-sentence trigger description for '{name}'. "
            f"Be specific — this field is the discovery surface."
        )
    target.write_text(
        _BASIC_SKILL_TEMPLATE.format(
            name=name,
            description=description,
            title=name.replace("-", " ").replace("_", " ").title(),
        ),
        encoding="utf-8",
    )
    return target


def delegate_skdd_forge(repo_root: Path, name: str) -> Tuple[bool, str]:
    """Run ``skdd forge <name>`` in ``repo_root``. Returns (success, output)."""
    try:
        result = subprocess.run(
            ["skdd", "forge", name],
            cwd=repo_root,
            capture_output=True,
            text=True,
            check=False,
        )
        return (result.returncode == 0, result.stdout + result.stderr)
    except (FileNotFoundError, OSError) as e:
        return (False, str(e))


def delegate_skdd_validate(repo_root: Path, name: str) -> Tuple[bool, str]:
    """Run ``skdd validate`` in ``repo_root`` + filter result to ``name``.

    Zak's ``skdd validate`` walks every SKILL.md in the colony and
    reports per-file pass/fail. The upstream CLI doesn't take a name
    filter, so we run it once + parse the output for the line
    referencing ``name``. Returns:

      - (True, lines mentioning ``name`` — should be a pass marker)
      - (False, lines mentioning ``name`` — should be a fail marker)
      - (False, full output) when ``name`` isn't found in the output
        (defensive: report the whole thing rather than silently passing)

    **v0.6.0 PR #92 review fix**: previously returned the colony-wide
    exit code. Any unrelated broken skill in the repo caused
    ``logmind skill test <good-skill>`` to fail. Now we extract just
    the lines about ``name`` so the gate is per-skill as intended.
    """
    try:
        result = subprocess.run(
            ["skdd", "validate"],
            cwd=repo_root,
            capture_output=True,
            text=True,
            check=False,
        )
    except (FileNotFoundError, OSError) as e:
        return (False, str(e))

    combined = result.stdout + result.stderr
    # Pull out lines referencing the target skill. `skdd validate`'s
    # output format mentions the skill by path or name; we match both
    # to stay format-tolerant.
    relevant = [
        line for line in combined.splitlines()
        if name in line or f"skills/{name}/" in line
    ]

    if not relevant:
        # Skill not mentioned in output — likely means it wasn't found
        # by skdd OR the output format changed. Report the whole output
        # + fail, rather than silently passing.
        return (
            False,
            f"skdd validate did not mention '{name}' in its output. "
            f"Full output:\n{combined}",
        )

    # Determine pass/fail per-skill from the relevant lines. skdd's
    # convention: pass lines look like "✓" / "PASS" / "ok"; fail lines
    # look like "✗" / "FAIL" / "error". Be permissive on the markers.
    relevant_text = "\n".join(relevant)
    fail_markers = ("✗", "FAIL", "fail", "ERROR", "error")
    has_fail = any(m in relevant_text for m in fail_markers)
    return (not has_fail, relevant_text)


# Logmind-specific validation layered on top of skdd validate. Each
# returns (ok, message). When `ok is False`, message explains why.

# Soft size cap — most well-written skills land 50-200 lines. The cap
# guards against runaway skills that bloat every agent invocation.
_LOGMIND_SKILL_BYTE_CAP = 8000


# Anchored regexes to detect required-field presence in YAML
# frontmatter. Substring matches (e.g. `"name:" in fm`) false-positive on
# nested fields like `domain_name:` or `package_name:`. Per v0.6.0 PR #92
# review (evidence-based-review). Multiline + start-anchor matches both
# top-level fields AND fields nested under a single indent level (which
# is fine — they're still real `name:` declarations).
_FRONTMATTER_NAME_RE = re.compile(r"^\s*name\s*:", re.MULTILINE)
_FRONTMATTER_DESCRIPTION_RE = re.compile(r"^\s*description\s*:", re.MULTILINE)


def check_frontmatter_required_fields(content: str) -> Tuple[bool, str]:
    """Required fields per agentskills.io/v1: ``name`` + ``description``.

    Lightweight check — proper validation is `skdd validate`'s job.
    This catches the most common authoring mistakes when skdd isn't
    available.

    v0.6.0 PR #92 review fix: use anchored regexes instead of substring
    `in` checks so `domain_name:` doesn't false-positive as `name:`.
    """
    if not content.startswith("---"):
        return (False, "SKILL.md must start with YAML frontmatter (--- block)")
    end = content.find("\n---", 4)
    if end == -1:
        return (False, "SKILL.md frontmatter is unterminated (missing closing ---)")
    fm = content[4:end]
    if not _FRONTMATTER_NAME_RE.search(fm):
        return (False, "SKILL.md frontmatter missing required field: name")
    if not _FRONTMATTER_DESCRIPTION_RE.search(fm):
        return (False, "SKILL.md frontmatter missing required field: description")
    return (True, "")


def check_size_cap(content: str, cap: int = _LOGMIND_SKILL_BYTE_CAP) -> Tuple[bool, str]:
    """Soft size cap — warns + fails if skill is unreasonably large."""
    size = len(content.encode("utf-8"))
    if size > cap:
        return (
            False,
            f"SKILL.md is {size} bytes — over the {cap}-byte logmind cap. "
            f"Large skills bloat every agent load. Consider splitting "
            f"into multiple focused skills."
        )
    return (True, f"{size} bytes (cap: {cap})")


# v0.6.3 — `logmind skill bench <name>`. Per-call token-cost measurement.
#
# Every time a skill triggers, its SKILL.md gets loaded into the agent's
# context window. The per-call cost is the byte size translated to an
# approximate token count (English text ≈ 4 bytes/token). This is the
# "measure" arrow of the SkDD loop — pairs with clud-bug's
# `usage --health` (the enforcement read) for a complete picture of
# whether each skill earns its token budget.

_LOGMIND_SKILL_TIGHT_BYTES = 2000   # ~500 tokens — a focused, well-trimmed skill
_LOGMIND_SKILL_BUDGET_BYTES = 6000  # ~1500 tokens — past this, splitting helps


_HEADER_RE = re.compile(r"^(#{1,6})\s+(.+?)\s*$", re.MULTILINE)
_HTML_COMMENT_RE = re.compile(r"<!--.*?-->", re.DOTALL)


def _split_into_sections(content: str) -> "list[Tuple[str, int]]":
    """Return [(header_text, body_bytes)] for each top-level (##) section.

    Frontmatter (the leading --- block) is counted as its own section
    named "frontmatter". The body before the first ## is named "intro".
    Bytes per section = UTF-8 byte count of the SECTION BODY (not the
    header line itself), so trimming suggestions point at the prose,
    not the structural markers.
    """
    sections: "list[Tuple[str, int]]" = []

    # Frontmatter — leading ---...--- block.
    rest = content
    if content.startswith("---"):
        end = content.find("\n---", 4)
        if end != -1:
            frontmatter = content[: end + 4]
            sections.append(("frontmatter", len(frontmatter.encode("utf-8"))))
            rest = content[end + 4 :].lstrip("\n")

    # Find every ## section header (level 2) in the body.
    headers = [
        (m.start(), len(m.group(1)), m.group(2))
        for m in _HEADER_RE.finditer(rest)
        if len(m.group(1)) == 2
    ]

    if not headers:
        # Whole body is one chunk.
        body = rest.strip()
        if body:
            sections.append(("body", len(body.encode("utf-8"))))
        return sections

    # Intro = anything before the first ## header.
    intro = rest[: headers[0][0]].strip()
    if intro:
        sections.append(("intro", len(intro.encode("utf-8"))))

    for i, (pos, _level, title) in enumerate(headers):
        end = headers[i + 1][0] if i + 1 < len(headers) else len(rest)
        section_body = rest[pos:end].strip()
        sections.append((title, len(section_body.encode("utf-8"))))

    return sections


def _bench_status(size: int, target: int, budget: int) -> str:
    """Bucket the size into a status label using the caller's thresholds."""
    if size <= target:
        return "tight"
    if size <= budget:
        return "typical"
    if size <= _LOGMIND_SKILL_BYTE_CAP:
        return "verbose"
    return "over-budget"


def _trim_suggestions(
    content: str,
    sections: "list[Tuple[str, int]]",
    total: int,
    *,
    target: int,
    budget: int,
) -> "list[str]":
    """Heuristic suggestions for trimming an oversized skill.

    Triggered only when total > budget. Returns empty list when the
    skill is at or under budget.
    """
    suggestions: list[str] = []
    if total <= budget:
        return suggestions

    # Heuristic 1: any single section taking >30% of the total is a
    # prime candidate for trimming or linking out.
    for name, size in sections:
        if name in ("frontmatter", "intro", "body"):
            continue
        if total > 0 and size / total > 0.30:
            suggestions.append(
                f"Section '{name}' is {size} bytes ({size * 100 // total}% of total) — "
                f"consider linking out to docs OR moving to its own skill."
            )

    # Heuristic 2: many HTML comments inflate size without paying back
    # — they're invisible to the rendered prompt but charged in tokens.
    comment_bytes = sum(
        len(m.encode("utf-8")) for m in _HTML_COMMENT_RE.findall(content)
    )
    if comment_bytes >= 200:
        suggestions.append(
            f"{comment_bytes} bytes of HTML comments — agents load them too. "
            f"Move authoring notes to a sibling NOTES.md if they're not for the agent."
        )

    # Heuristic 3: if over the hard cap, recommend split.
    if total > _LOGMIND_SKILL_BYTE_CAP:
        suggestions.append(
            f"Total exceeds {_LOGMIND_SKILL_BYTE_CAP}-byte cap — split into "
            f"multiple focused skills."
        )
    elif not suggestions:
        # Generic fallback when the skill is verbose but no clear culprit section.
        suggestions.append(
            f"Total is {total} bytes (target: {target}, budget: {budget}). "
            f"Tighten the largest sections or move detailed examples behind links."
        )

    return suggestions


def bench_skill(
    content: str,
    *,
    target: int = _LOGMIND_SKILL_TIGHT_BYTES,
    budget: int = _LOGMIND_SKILL_BUDGET_BYTES,
) -> dict:
    """Measure per-call token cost of a skill body.

    ``target`` and ``budget`` are honored by the status bucketer + the
    suggestion heuristics (PR #99 review fix — prior versions accepted
    these kwargs but threaded only the module-level constants through
    to the helpers, silently ignoring the caller's overrides).

    Returns a dict with:

      - bytes: UTF-8 byte size of the whole SKILL.md
      - est_tokens: bytes / 4 (rough English-text approximation)
      - status: 'tight' | 'typical' | 'verbose' | 'over-budget'
      - target: the tight-skill target (info)
      - budget: the soft-budget threshold (info)
      - sections: [{name, bytes, pct}] per ## section
      - suggestions: trim hints when status is 'verbose' or worse
    """
    total = len(content.encode("utf-8"))
    sections_raw = _split_into_sections(content)
    sections = [
        {
            "name": name,
            "bytes": size,
            "pct": (size * 100 // total) if total else 0,
        }
        for name, size in sections_raw
    ]
    return {
        "bytes": total,
        "est_tokens": total // 4,
        "status": _bench_status(total, target, budget),
        "target": target,
        "budget": budget,
        "sections": sections,
        "suggestions": _trim_suggestions(
            content, sections_raw, total, target=target, budget=budget
        ),
    }


# v0.6.4 — `logmind skill audit`. Author's-side staleness read for every
# SKILL.md in `.claude/skills/`. Pairs with clud-bug `usage --health`:
#
#   audit:    What's HERE and how stale is it (file-system + git side).
#   usage:    Which skills earn their context budget (load + cite side).
#
# Together: a complete read on whether each skill earns its place.

_LOGMIND_SKILL_AUDIT_TIGHT_BYTES = 2000  # mirror of _LOGMIND_SKILL_TIGHT_BYTES;
                                          # declared standalone so audit doesn't
                                          # break if bench is ever extracted.


def audit_skills(repo_root: Path) -> "list[dict]":
    """List every `.claude/skills/*/SKILL.md` with staleness signals.

    For each skill returns:

      - ``name``: directory name under ``.claude/skills/``
      - ``path``: relative path to SKILL.md
      - ``bytes``: UTF-8 byte size
      - ``last_modified``: ISO date of last git commit touching SKILL.md
        (falls back to file mtime when not in git history)
      - ``decision_count``: # times skill name appears in
        ``docs/decisions.md`` + ``docs/decisions-branches/*.md`` (rough
        signal of author-side iteration cadence)

    Returns empty list if `.claude/skills/` doesn't exist OR no SKILL.md
    files are present.
    """
    skills_dir = default_skills_dir(repo_root)
    if not skills_dir.is_dir():
        return []

    decision_files: list[Path] = []
    docs_dir = repo_root / "docs"
    if (docs_dir / "decisions.md").exists():
        decision_files.append(docs_dir / "decisions.md")
    branches_dir = docs_dir / "decisions-branches"
    if branches_dir.is_dir():
        decision_files.extend(sorted(branches_dir.glob("*.md")))

    decision_text = "\n".join(
        f.read_text(encoding="utf-8", errors="ignore")
        for f in decision_files
    )

    results: "list[dict]" = []
    for skill_subdir in sorted(skills_dir.iterdir()):
        if not skill_subdir.is_dir():
            continue
        skill_md = skill_subdir / "SKILL.md"
        if not skill_md.is_file():
            continue

        rel_path = skill_md.relative_to(repo_root).as_posix()
        size = skill_md.stat().st_size
        name = skill_subdir.name

        last_modified = _git_last_touched(repo_root, rel_path)
        if last_modified is None:
            import datetime as _dt
            last_modified = _dt.date.fromtimestamp(skill_md.stat().st_mtime).isoformat()

        decision_count = decision_text.count(name) if name and decision_text else 0

        results.append({
            "name": name,
            "path": rel_path,
            "bytes": size,
            "last_modified": last_modified,
            "decision_count": decision_count,
        })

    return results


def _git_last_touched(repo_root: Path, rel_path: str) -> Optional[str]:
    """Return ISO date of last commit touching ``rel_path``, or None."""
    try:
        result = subprocess.run(
            ["git", "log", "-1", "--format=%cs", "--", rel_path],
            cwd=repo_root,
            capture_output=True,
            text=True,
            check=False,
            timeout=10,
        )
    except (FileNotFoundError, OSError, subprocess.TimeoutExpired):
        return None
    if result.returncode != 0:
        return None
    out = result.stdout.strip()
    return out if out else None


def classify_audit_row(row: dict, now=None) -> str:
    """Apply deterministic thresholds to an audit row.

    - ``ghost``: ``decision_count == 0`` AND ``bytes > _LOGMIND_SKILL_AUDIT_TIGHT_BYTES``
      (loaded into every context but author never iterates — candidate
      for clud-bug usage --health to confirm + archive).
    - ``aging``: ``last_modified > 90 days ago``.
    - ``active``: otherwise.

    ``now`` is injectable for testability; defaults to today.
    """
    import datetime as _dt
    if now is None:
        now = _dt.date.today()
    if row.get("decision_count", 0) == 0 and row.get("bytes", 0) > _LOGMIND_SKILL_AUDIT_TIGHT_BYTES:
        return "ghost"
    last = row.get("last_modified")
    if last:
        try:
            last_date = _dt.date.fromisoformat(last)
            if (now - last_date).days > 90:
                return "aging"
        except (ValueError, TypeError):
            pass
    return "active"


# v0.6.5 — `logmind skill suggest`. Human-initiated pattern detection.
#
# Replaces the killed-Stream-9 autonomous bot direction. The CLI scans
# recent decision-log entries for terms appearing across many distinct
# decisions — a heuristic signal that "we keep talking about X, maybe
# X should have its own skill." Output is a PRE-FILLED issue draft
# matching agent-skills/.github/ISSUE_TEMPLATE/new-skill.yml. The
# HUMAN reads, decides, and either opens the issue or discards.
#
# Never auto-PR. Never auto-create the skill. The whole point of the
# pragmatic SkDD pivot is that humans gate skill lifecycle.

_SUGGEST_STOPWORDS = frozenset({
    "the", "a", "an", "and", "or", "but", "if", "then", "else", "of", "to",
    "for", "in", "on", "at", "by", "with", "as", "is", "are", "was", "were",
    "be", "been", "being", "have", "has", "had", "do", "does", "did", "this",
    "that", "these", "those", "it", "its", "we", "our", "us", "they", "them",
    "their", "i", "my", "me", "you", "your", "he", "she", "his", "her",
    "use", "uses", "used", "using", "make", "made", "makes", "ship", "shipped",
    "ships", "add", "added", "adds", "remove", "removes", "removed", "fix",
    "fixed", "fixes", "build", "built", "test", "tests", "tested", "run",
    "ran", "runs", "see", "saw", "seen", "get", "got", "want", "wants",
    "need", "needs", "needed", "should", "could", "would", "will", "can",
    "may", "might", "must", "let", "lets",
    "code", "file", "files", "function", "functions", "method", "methods",
    "class", "classes", "module", "modules", "library", "libraries",
    "feature", "features", "change", "changes", "release", "releases",
    "version", "versions", "branch", "branches", "commit", "commits",
    "main", "now", "today", "all", "any", "some", "one", "two", "three",
    "first", "second", "next", "last", "new", "old", "before", "after",
    "decision", "decisions", "reasoning", "reason", "alternatives",
    "alternative", "implications", "implication", "summary", "context",
    "date", "pr",
})

_INTERESTING_TOKEN_RE = re.compile(
    r"\b("
    r"[a-z]+(?:-[a-z]+)+"            # kebab-case multi-word
    r"|[A-Z][a-z]+(?:[A-Z][a-z]+)+"  # PascalCase / camelCase
    r"|[A-Z]{2,}"                    # acronyms (API, JWT, CI)
    r"|[a-z]+_[a-z]+(?:_[a-z]+)*"    # snake_case
    r")\b"
)


def suggest_skills_from_decisions(
    repo_root: Path,
    *,
    since_days: int = 30,
    min_decisions: int = 3,
    top_n: int = 5,
) -> "list[dict]":
    """Scan recent decision entries for repeated patterns.

    Returns up to ``top_n`` suggestions, each a dict:

      - ``phrase``: raw token (e.g. "api-versioning", "PostgreSQL", "JWT")
      - ``slug``: kebab-cased normalization for issue title + skill slug
      - ``decision_count``: # distinct decisions citing the phrase
      - ``evidence``: list of {file, snippet} — first occurrence per decision
      - ``draft_description``: one-sentence placeholder description

    Filters:
      - Excludes phrases already used as existing skill names.
      - Excludes stop words + generic structural terms.
      - Requires ≥``min_decisions`` distinct decisions.

    HUMAN-INITIATED only. Caller decides whether to open the issue.
    """
    import datetime as _dt
    threshold_date = _dt.date.today() - _dt.timedelta(days=since_days)

    docs_dir = repo_root / "docs"
    if not docs_dir.is_dir():
        return []

    decision_entries = _gather_recent_decisions(docs_dir, threshold_date)
    if not decision_entries:
        return []

    existing_skill_names: set[str] = set()
    skills_dir = default_skills_dir(repo_root)
    if skills_dir.is_dir():
        for child in skills_dir.iterdir():
            if child.is_dir() and (child / "SKILL.md").is_file():
                existing_skill_names.add(child.name.lower())

    token_evidence: "dict[str, list[Tuple[int, dict]]]" = {}
    for idx, entry in enumerate(decision_entries):
        seen_in_entry: set[str] = set()
        body = entry["body"]
        for match in _INTERESTING_TOKEN_RE.finditer(body):
            tok = match.group(1)
            tok_lower = tok.lower()
            if tok_lower in _SUGGEST_STOPWORDS:
                continue
            if tok_lower in seen_in_entry:
                continue
            seen_in_entry.add(tok_lower)
            snippet = _excerpt_around(body, match.start(), width=80)
            token_evidence.setdefault(tok, []).append(
                (idx, {"file": entry["file"], "snippet": snippet})
            )

    ranked: "list[Tuple[str, list]]" = []
    for tok, hits in token_evidence.items():
        if len(hits) < min_decisions:
            continue
        slug = _kebab_slug(tok)
        if slug in existing_skill_names:
            continue
        ranked.append((tok, hits))

    ranked.sort(key=lambda kv: (-len(kv[1]), kv[0].lower()))
    top = ranked[:top_n]

    suggestions = []
    for tok, hits in top:
        slug = _kebab_slug(tok)
        evidence = [h[1] for h in hits[:5]]
        suggestions.append({
            "phrase": tok,
            "slug": slug,
            "decision_count": len(hits),
            "evidence": evidence,
            "draft_description": (
                f"When working on {tok}, follow consistent conventions across "
                f"the codebase. (TODO: replace with concrete trigger + steps.)"
            ),
        })
    return suggestions


def _gather_recent_decisions(docs_dir: Path, threshold_date) -> "list[dict]":
    """Collect decision entries from docs/decisions.md + branch files
    that fall within the threshold window.

    Each entry is parsed by '## ' header boundaries. Returns list of
    ``{file, header, body}``. The decision-file mtime is used as a
    coarse window filter when entries don't carry their own dates.
    """
    import datetime as _dt
    entries: "list[dict]" = []
    candidates: "list[Path]" = []
    main = docs_dir / "decisions.md"
    if main.exists():
        candidates.append(main)
    branches = docs_dir / "decisions-branches"
    if branches.is_dir():
        candidates.extend(sorted(branches.glob("*.md")))

    for path in candidates:
        text = path.read_text(encoding="utf-8", errors="ignore")
        rel = path.name
        try:
            file_mtime = _dt.date.fromtimestamp(path.stat().st_mtime)
        except OSError:
            file_mtime = None

        parts = re.split(r"\n## ", text)
        for i, part in enumerate(parts):
            if not part.strip():
                continue
            if i == 0 and not part.startswith("## "):
                continue
            header_end = part.find("\n")
            header = part[:header_end].strip() if header_end != -1 else part.strip()
            body = part[header_end:].strip() if header_end != -1 else ""

            # v0.6.5 PR #101 review fix: filter at the ENTRY level, not
            # the file level. decisions.md is appended on every log call,
            # so its mtime is always today — file-mtime filtering would
            # leak every decision ever logged in any active repo.
            entry_date = _extract_entry_date(body)
            if entry_date is None:
                # No date on this entry. Fall back to file mtime as a
                # coarse window — but only for branch files. decisions.md
                # without dates is too ambiguous (could be entries from
                # months ago); skip them rather than include everything.
                if rel == "decisions.md":
                    continue
                if file_mtime is None or file_mtime < threshold_date:
                    continue
            elif entry_date < threshold_date:
                continue

            entries.append({
                "file": rel,
                "header": header,
                "body": body,
            })
    return entries


# Match "**Date**: YYYY-MM-DD" or "Date: YYYY-MM-DD" in decision-entry
# bodies. logmind's `log` writes the **Date** line; older entries may
# use the bare form. Both anchored to start-of-line.
_ENTRY_DATE_RE = re.compile(
    r"^\s*\*{0,2}Date\*{0,2}\s*:\s*(\d{4}-\d{2}-\d{2})",
    re.MULTILINE,
)


def _extract_entry_date(body: str):
    """Pull an ISO date out of a decision-entry body, or None if absent."""
    import datetime as _dt
    m = _ENTRY_DATE_RE.search(body)
    if not m:
        return None
    try:
        return _dt.date.fromisoformat(m.group(1))
    except (ValueError, TypeError):
        return None


def _excerpt_around(text: str, idx: int, *, width: int = 80) -> str:
    """Return a short readable snippet around position ``idx``."""
    start = max(0, idx - width // 2)
    end = min(len(text), idx + width // 2)
    snippet = text[start:end].replace("\n", " ").strip()
    if start > 0:
        snippet = "…" + snippet
    if end < len(text):
        snippet = snippet + "…"
    return snippet


def _kebab_slug(token: str) -> str:
    """Normalize a token to a kebab-case slug suitable for skill names.

    Examples:
      - "api-versioning" → "api-versioning"
      - "PostgreSQL" → "postgresql"
      - "JWT" → "jwt"
      - "snake_case_name" → "snake-case-name"
      - "PascalCaseClass" → "pascal-case-class"
    """
    spaced = re.sub(r"(?<=[a-z])(?=[A-Z])", "-", token)
    spaced = spaced.replace("_", "-")
    slug = re.sub(r"-+", "-", spaced.lower()).strip("-")
    return slug


def format_suggest_issue_draft(suggestion: dict) -> str:
    """Render one suggestion as a pre-filled GH-issue body matching
    agent-skills/.github/ISSUE_TEMPLATE/new-skill.yml.

    Output is markdown the user can paste into a GH issue body. The
    fields match the agent-skills template form so the human can
    paste directly.
    """
    evidence_lines = "\n".join(
        f"- `{e['file']}`: {e['snippet']}" for e in suggestion["evidence"]
    )
    return (
        f"## New skill proposal: {suggestion['slug']}\n"
        f"\n"
        f"### Slug\n"
        f"`{suggestion['slug']}`\n"
        f"\n"
        f"### Trigger\n"
        f"When working on `{suggestion['phrase']}` — pattern emerged in "
        f"{suggestion['decision_count']} recent decisions.\n"
        f"\n"
        f"### Evidence (auto-extracted from decision log)\n"
        f"{evidence_lines}\n"
        f"\n"
        f"### Draft frontmatter description\n"
        f"{suggestion['draft_description']}\n"
        f"\n"
        f"### Review mode\n"
        f"`critical-only` (default — adjust if needed)\n"
        f"\n"
        f"### Scope\n"
        f"_(choose: cross-repo catalog vs single-repo custom)_\n"
        f"\n"
        f"### Applies to\n"
        f"_(globs — leave blank for repo-wide)_\n"
        f"\n"
        f"---\n"
        f"\n"
        f"_Generated by `logmind skill suggest`. Auto-extracted patterns "
        f"are heuristic — please review evidence and refine before opening._"
    )
