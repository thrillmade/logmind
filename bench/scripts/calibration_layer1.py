"""Python re-implementation of clud-bug v0.6.25 Layer 1 budget estimator.

Translates the jq formula in
``/Users/ludlow/clud-bug/templates/workflow.yml.tmpl`` (lines 271-289)
verbatim, so we can apply it retroactively to historical PRs (Stream 1
Path A) without invoking jq.

**Coefficient parity is the load-bearing invariant.** If the jq formula
in clud-bug changes (e.g., v0.6.30 re-tunes per-line weights from
calibration data), this module's constants MUST be updated to match.
Tests cross-check against known PR data points (Layer 1.5 markers from
v0.6.25+ PRs) to surface drift loudly.
"""

from __future__ import annotations

import re
from math import ceil
from typing import Iterable

# Per the jq formula at clud-bug/templates/workflow.yml.tmpl:271-289.
# Patterns are checked in order — first match wins. Mirrors the jq
# if/elif chain exactly.

_TEST_RE = re.compile(r"\.(test|spec)\.|__tests__|^tests?/")
_DOCS_RE = re.compile(r"\.(md|txt|rst|adoc|mdx)$")
_CONFIG_RE = re.compile(r"\.(yml|yaml|toml|json|cfg|ini|conf|tmpl)$")
_CODE_RE = re.compile(
    r"\.(ts|tsx|py|js|jsx|mjs|cjs|go|rs|java|kt|rb|php|cs|c|cpp|h|hpp|swift|scala)$"
)

# turns-per-line. Reciprocal of "lines per turn" — code at 1/50 = 50
# lines per turn; docs at 1/150 = 150 lines per turn (skim-faster).
_WEIGHT_TEST = 0.01        # 1/100
_WEIGHT_DOCS = 0.00666667  # 1/150 (jq uses this exact decimal)
_WEIGHT_CONFIG = 0.01      # 1/100
_WEIGHT_CODE = 0.02        # 1/50
_WEIGHT_UNKNOWN = 0.0125   # 1/80 (defensive default)

# Per-file fixed cost — accounts for opening a file, scanning frontmatter,
# orienting in the diff. Independent of size.
_FILE_OPEN_COST = 0.3

# Modified-line multiplier — modified lines (`mod = min(add, del)`) cost
# more than pure adds because they require reading the original AND the
# replacement. Pure deletions cost almost nothing (0.1×).
_MOD_LINE_MULTIPLIER = 1.5
_DEL_LINE_MULTIPLIER = 0.1

# Cumulative emit overhead — structured-output rendering, JSON-schema
# validation/retries, initial context loading. Raised from 5 to 10 in
# v0.6.25 after tokenomics #21 (26 docs files used ~25 actual turns
# vs predicted 16; the 5-10 gap was attributable to emit costs).
_EMIT_OVERHEAD = 10.0

# Per-prior-thread cost — each unresolved claude-bot review thread the
# fix-push flow walks + decides on costs ~1.5 turns.
_PRIOR_THREAD_COST = 1.5

# Files EXCLUDED from per-file cost — derived/auto-generated docs that
# logmind regenerates from sources; clud-bug doesn't need to review
# them. Skipping these matches the jq `.path != ...` filters exactly.
_EXCLUDED_PATHS = frozenset({
    "docs/timeline.md",
    "docs/file-structure.md",
    "docs/decisions.md",
})

# max_turns derivation: ceil(est × 1.2), clamped to [15, 60]. Mirrors
# the bash arithmetic in workflow.yml.tmpl exactly:
#   MAX_TURNS=$(( (TURNS_ESTIMATED * 12 + 9) / 10 ))
#   [ "$MAX_TURNS" -lt 15 ] && MAX_TURNS=15
#   [ "$MAX_TURNS" -gt 60 ] && MAX_TURNS=60
_SAFETY_MARGIN = 1.2
_MAX_TURNS_FLOOR = 15
_MAX_TURNS_CEILING = 60

# Trivial (Haiku-routed) PRs short-circuit the formula entirely with a
# flat budget. The paths-check job sets IS_TRIVIAL=true based on diff
# size / dependabot signals; when true, both estimated + cap are 10.
_TRIVIAL_FLAT_BUDGET = 10


def classify_path(path: str) -> str:
    """Classify a file path into a Layer 1 weight class.

    Returns: 'test' | 'docs' | 'config' | 'code' | 'unknown'

    Order matches the jq if/elif chain — first match wins. A path that
    matches multiple patterns (e.g., a .py test file matches both test
    and code) returns the FIRST match, which is 'test'.
    """
    if _TEST_RE.search(path):
        return "test"
    if _DOCS_RE.search(path):
        return "docs"
    if _CONFIG_RE.search(path):
        return "config"
    if _CODE_RE.search(path):
        return "code"
    return "unknown"


def weight_for_class(cls: str) -> float:
    return {
        "test": _WEIGHT_TEST,
        "docs": _WEIGHT_DOCS,
        "config": _WEIGHT_CONFIG,
        "code": _WEIGHT_CODE,
        "unknown": _WEIGHT_UNKNOWN,
    }[cls]


def per_file_cost(path: str, additions: int, deletions: int) -> float:
    """Cost in turns for reviewing one file.

    Mirrors the jq formula:
        mod = min(add, del)        # modified-line approximation
        pa  = add - mod            # pure additions
        pd  = del - mod            # pure deletions
        cost = 0.3 + pa×tw + mod×1.5×tw + pd×0.1×tw
    """
    if path in _EXCLUDED_PATHS:
        return 0.0

    mod = min(additions, deletions)
    pa = additions - mod   # pure adds
    pd = deletions - mod   # pure deletes
    tw = weight_for_class(classify_path(path))

    return (
        _FILE_OPEN_COST
        + pa * tw
        + mod * _MOD_LINE_MULTIPLIER * tw
        + pd * _DEL_LINE_MULTIPLIER * tw
    )


def predict_estimated_turns(
    files: Iterable[dict],
    prior_threads: int = 0,
    is_trivial: bool = False,
) -> int:
    """Predict `turns_estimated` for a PR given its file shape.

    Args:
        files: iterable of dicts with ``path``, ``additions``, ``deletions``
        prior_threads: count of unresolved claude-bot review threads
        is_trivial: True for Haiku-routed dependabot-style PRs

    Returns:
        Integer turns estimate. Matches the v0.6.25 jq formula output
        within rounding (jq uses ``floor(x + 0.9999999)``, we use
        ``math.ceil``; equivalent for non-integer x).
    """
    if is_trivial:
        return _TRIVIAL_FLAT_BUDGET

    total = sum(
        per_file_cost(f["path"], f["additions"], f["deletions"])
        for f in files
    )
    total += _EMIT_OVERHEAD
    total += prior_threads * _PRIOR_THREAD_COST
    return ceil(total)


def predict_max_turns(estimated: int, is_trivial: bool = False) -> int:
    """Derive max_turns from the estimate.

    Trivial PRs: flat 10. Otherwise: ceil(estimated × 1.2), clamped
    to [15, 60]. Matches the bash arithmetic in workflow.yml.tmpl.
    """
    if is_trivial:
        return _TRIVIAL_FLAT_BUDGET

    # Bash: $(( (TURNS_ESTIMATED * 12 + 9) / 10 )) is integer division
    # that's equivalent to ceil(est × 1.2) for est >= 0.
    cap = (estimated * 12 + 9) // 10
    if cap < _MAX_TURNS_FLOOR:
        cap = _MAX_TURNS_FLOOR
    if cap > _MAX_TURNS_CEILING:
        cap = _MAX_TURNS_CEILING
    return cap


def predict(
    files: Iterable[dict],
    prior_threads: int = 0,
    is_trivial: bool = False,
) -> dict:
    """One-call wrapper returning both `estimated` and `max_turns`.

    Returns: {'estimated': N, 'max_turns': M, 'is_trivial': bool}
    """
    files_list = list(files)
    est = predict_estimated_turns(files_list, prior_threads, is_trivial)
    cap = predict_max_turns(est, is_trivial)
    return {"estimated": est, "max_turns": cap, "is_trivial": is_trivial}
