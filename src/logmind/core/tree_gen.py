"""File structure generation using the `tree` command, with a stable
gitignore-aware Python fallback."""

from __future__ import annotations

import fnmatch
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, List, Optional, Sequence, Tuple

DEFAULT_IGNORES: Sequence[str] = (
    "__pycache__",
    ".git",
    "node_modules",
    "venv",
    ".venv",
    "env",
    ".env",
    "*.pyc",
    ".pytest_cache",
    ".mypy_cache",
    ".ruff_cache",
    "dist",
    "build",
    "*.egg-info",
)


@dataclass
class IgnoreRules:
    """Compiled ignore + negation patterns for path-aware matching.

    Patterns can be basename-only (``.next``) or path-shaped (``site/.next``).
    Negation (``!pattern``) re-includes a previously-matched path.
    """

    ignore: Tuple[str, ...] = ()
    negate: Tuple[str, ...] = ()

    def has_path_pattern(self) -> bool:
        """True if any pattern contains a slash — the `tree` binary's
        basename-only ``-I`` flag can't handle these, so callers should
        fall back to the Python tree walker for correctness."""
        return any("/" in p for p in self.ignore + self.negate)

    def matches(self, rel_path: str, basename: str) -> bool:
        """Return True if a path should be ignored.

        Matches a pattern against (a) the full relative path, (b) any
        path component, or (c) the basename. Honors negation patterns
        with gitignore-style precedence: if both ignore and negate match,
        negate wins (the path is re-included).
        """
        if _pattern_set_matches(self.ignore, rel_path, basename):
            if _pattern_set_matches(self.negate, rel_path, basename):
                return False
            return True
        return False


def _pattern_set_matches(
    patterns: Sequence[str], rel_path: str, basename: str
) -> bool:
    """Match patterns against rel_path, any component, or basename."""
    if not patterns:
        return False
    components = rel_path.split("/") if rel_path else ()
    for pat in patterns:
        if fnmatch.fnmatchcase(rel_path, pat):
            return True
        if fnmatch.fnmatchcase(basename, pat):
            return True
        for comp in components:
            if fnmatch.fnmatchcase(comp, pat):
                return True
    return False


def _read_gitignore_patterns(repo_root: Path) -> Tuple[List[str], List[str]]:
    """
    Return (ignore, negate) pattern lists from ``repo_root/.gitignore``.

    Strips comments, blanks, leading "/", trailing "/". ``!pattern`` lines
    are collected separately so callers can apply gitignore-style negation
    precedence. Full gitignore semantics (anchored patterns, recursive
    globs) are still out of scope — this is "good enough to not emit
    obvious build-cache noise" rather than a complete gitignore parser.
    """
    gi = repo_root / ".gitignore"
    if not gi.exists():
        return [], []
    ignore: List[str] = []
    negate: List[str] = []
    for raw in gi.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        is_negate = line.startswith("!")
        if is_negate:
            line = line[1:]
        if line.startswith("/"):
            line = line[1:]
        if line.endswith("/"):
            line = line[:-1]
        if not line:
            continue
        (negate if is_negate else ignore).append(line)
    return ignore, negate


def _resolve_ignore_rules(
    root_path: Path,
    extra: Optional[Iterable[str]] = None,
) -> IgnoreRules:
    ignore: List[str] = list(DEFAULT_IGNORES)
    gi_ignore, gi_negate = _read_gitignore_patterns(root_path)
    ignore.extend(gi_ignore)
    if extra:
        ignore.extend(extra)
    # Stable de-dup preserving order
    seen: set = set()
    deduped_ignore: List[str] = []
    for p in ignore:
        if p not in seen:
            seen.add(p)
            deduped_ignore.append(p)
    seen.clear()
    deduped_negate: List[str] = []
    for p in gi_negate:
        if p not in seen:
            seen.add(p)
            deduped_negate.append(p)
    return IgnoreRules(
        ignore=tuple(deduped_ignore),
        negate=tuple(deduped_negate),
    )


def _resolve_ignore_patterns(
    root_path: Path,
    extra: Optional[Iterable[str]] = None,
) -> List[str]:
    """Back-compat wrapper used by callers that only need the flat list
    (e.g. passing ``-I`` to the `tree` binary which is basename-only)."""
    return list(_resolve_ignore_rules(root_path, extra).ignore)


def generate_tree(
    root_path: Optional[Path] = None,
    extra_ignore: Optional[Iterable[str]] = None,
    max_depth: Optional[int] = None,
) -> str:
    """
    Render a tree of ``root_path`` using the system ``tree`` binary if
    present, else a stable, sorted Python fallback.

    Both paths honour DEFAULT_IGNORES + .gitignore patterns + any extra
    patterns the caller supplies. Path-shaped patterns (``site/.next``)
    and gitignore negation (``!keep.log``) force the Python fallback,
    since the `tree` binary's ``-I`` flag is basename-only.

    ``max_depth=N`` truncates the tree at depth N (root is depth 0).
    ``max_depth=None`` (default) is unbounded. v0.5.0+: ``generate_file_structure``
    passes ``max_depth=2`` by default so the on-disk file-structure.md ships
    at a token-frugal depth across every consuming repo. Callers wanting
    the full tree (drill-in via CLI ``--max-depth 0``) pass ``None``.
    """
    if root_path is None:
        root_path = Path.cwd()
    root_path = Path(root_path)

    rules = _resolve_ignore_rules(root_path, extra_ignore)

    # The `tree` binary only understands basename excludes via -I. If any
    # pattern is path-shaped (e.g. "site/.next") or there are negation
    # rules, fall back to the Python walker which handles both correctly.
    use_python_fallback = rules.has_path_pattern() or bool(rules.negate)

    if not use_python_fallback:
        try:
            subprocess.run(
                ["tree", "--version"], capture_output=True, check=True
            )
            binary_available = True
        except (subprocess.CalledProcessError, FileNotFoundError):
            binary_available = False

        if binary_available:
            ignore_arg = "|".join(rules.ignore)
            cmd = [
                "tree",
                "-I",
                ignore_arg,
                "-a",
                "--noreport",
                "--dirsfirst",
            ]
            if max_depth is not None:
                # tree(1)'s -L N caps display depth at N levels BELOW the
                # root. Matches our Python fallback's convention exactly:
                # `_current_depth >= max_depth` early-returns, so
                # max_depth=2 shows root + its children + grandchild
                # names (but no further), which is precisely what
                # `tree -L 2` produces. PR #68's first revision had a
                # spurious +1 here that made the tree(1) path display
                # one level deeper than the Python fallback; clud-bug
                # caught it.
                cmd.extend(["-L", str(max_depth)])
            try:
                result = subprocess.run(
                    cmd,
                    cwd=root_path,
                    capture_output=True,
                    text=True,
                    check=True,
                )
                return result.stdout.strip()
            except subprocess.CalledProcessError:
                pass  # fall through to Python fallback

    return _generate_fallback_tree(root_path, rules=rules, max_depth=max_depth)


def _generate_fallback_tree(
    root_path: Path,
    rules: Optional[IgnoreRules] = None,
    prefix: str = "",
    max_depth: Optional[int] = None,
    _current_depth: int = 0,
    _repo_root: Optional[Path] = None,
) -> str:
    """
    Stable, sorted, gitignore-aware Python tree.

    Pattern matching is path-aware: a pattern like ``site/.next`` matches
    a path that traverses through ``site/.next`` even when basename-only
    fnmatch wouldn't. ``!pat`` negations re-include matched paths.

    No artificial depth cap by default (matches ``tree``'s behaviour).
    Pass ``max_depth=N`` to truncate.
    """
    if rules is None:
        rules = _resolve_ignore_rules(root_path)
    if _repo_root is None:
        _repo_root = root_path

    if max_depth is not None and _current_depth >= max_depth:
        return ""

    lines: List[str] = []
    if _current_depth == 0:
        lines.append(root_path.name or ".")

    try:
        items = sorted(
            root_path.iterdir(),
            key=lambda p: (not p.is_dir(), p.name.lower()),
        )
    except (PermissionError, FileNotFoundError):
        return "\n".join(lines)

    def _keep(item: Path) -> bool:
        try:
            rel = item.relative_to(_repo_root).as_posix()
        except ValueError:
            rel = item.name
        return not rules.matches(rel, item.name)

    items = [it for it in items if _keep(it)]

    for i, item in enumerate(items):
        is_last = i == len(items) - 1
        connector = "└── " if is_last else "├── "
        next_prefix = "    " if is_last else "│   "

        lines.append(f"{prefix}{connector}{item.name}")

        if item.is_dir() and not item.is_symlink():
            subtree = _generate_fallback_tree(
                item,
                rules=rules,
                prefix=prefix + next_prefix,
                max_depth=max_depth,
                _current_depth=_current_depth + 1,
                _repo_root=_repo_root,
            )
            if subtree:
                lines.append(subtree)

    return "\n".join(line for line in lines if line)


DEFAULT_FILE_STRUCTURE_DEPTH = 2


def generate_file_structure(
    repo_root: Path,
    max_depth: Optional[int] = DEFAULT_FILE_STRUCTURE_DEPTH,
) -> str:
    """Render the file-structure.md content for ``repo_root`` and return it
    as a string (no file is written). Useful when the caller wants to
    direct the output somewhere other than the canonical
    ``<repo_root>/docs/file-structure.md`` — e.g. v0.3.0's custom merge
    driver, which receives the target path from git as ``%A``.

    Output is deterministic for a given tree: identical trees render to
    byte-identical files. v0.3.3 dropped the prior wall-clock
    ``Last updated:`` line, which caused the post-merge hook to re-stage
    the file on every ``git pull`` even when the tree was unchanged.

    v0.5.0+: ``max_depth`` defaults to 2 so the on-disk file ships
    token-frugal across all consuming repos. The 103 KB unbounded tree
    in logmind itself drops to ~10 KB at depth 2. Pass ``max_depth=None``
    (or CLI ``--max-depth 0``) to get the full tree.
    """
    tree_output = generate_tree(repo_root, max_depth=max_depth)
    template_path = Path(__file__).parent.parent / "templates" / "file-structure.md.template"
    template = template_path.read_text(encoding="utf-8")
    return template.format(tree_output=tree_output)


def write_file_structure(
    target_path: Path,
    repo_root: Optional[Path] = None,
    max_depth: Optional[int] = DEFAULT_FILE_STRUCTURE_DEPTH,
) -> bool:
    """Write rendered file-structure to ``target_path`` atomically. Returns
    True if the file's content changed, False if it was already up to date.
    Mirrors the shape of ``logmind.core.timeline.write_timeline()``.

    Args:
        target_path: where to write (e.g. docs/file-structure.md, or %A
            when invoked by the v0.3.0 git merge driver).
        repo_root: project root. Defaults to ``target_path.parent.parent``
            (assumes docs/file-structure.md layout), then falls back to cwd.
        max_depth: depth cap for the rendered tree. Defaults to
            ``DEFAULT_FILE_STRUCTURE_DEPTH`` (2). Pass ``None`` for the
            full tree (drill-in via CLI ``--max-depth 0``).
    """
    from logmind.core.atomic_io import atomic_write_text

    if repo_root is None:
        # Heuristic: assume target_path is <repo>/docs/file-structure.md
        # so repo_root is two levels up. Git's merge driver passes an
        # absolute path under the worktree, which satisfies this.
        repo_root = target_path.resolve().parent.parent
        if not (repo_root / ".git").exists():
            repo_root = Path.cwd()

    rendered = generate_file_structure(repo_root, max_depth=max_depth)
    existing = target_path.read_text(encoding="utf-8") if target_path.exists() else ""
    if existing == rendered:
        return False
    target_path.parent.mkdir(parents=True, exist_ok=True)
    atomic_write_text(target_path, rendered)
    return True


def update_file_structure(docs_path: Optional[Path] = None) -> None:
    """
    Update the file-structure.md file with current project tree.

    The project root is inferred from ``docs_path.parent`` so the function
    works regardless of the caller's cwd. Uses the gitignore-aware tree
    generator (DEFAULT_IGNORES + .gitignore patterns).

    Args:
        docs_path: Path to docs directory. Defaults to ./docs
    """
    if docs_path is None:
        docs_path = Path.cwd() / "docs"
    docs_path.mkdir(parents=True, exist_ok=True)
    write_file_structure(docs_path / "file-structure.md", docs_path.parent)
