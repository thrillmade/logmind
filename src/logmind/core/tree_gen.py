"""File structure generation using the `tree` command, with a stable
gitignore-aware Python fallback."""

from __future__ import annotations

import fnmatch
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Iterable, List, Optional, Sequence

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


def _read_gitignore_patterns(repo_root: Path) -> List[str]:
    """
    Return simple ignore patterns from ``repo_root/.gitignore``.

    Strips comments, blanks, leading "/", trailing "/". Negation lines
    (``!pattern``) are dropped — full gitignore semantics are out of scope;
    this helper just augments DEFAULT_IGNORES so ``tree`` and the fallback
    don't emit obviously-ignored noise.
    """
    gi = repo_root / ".gitignore"
    if not gi.exists():
        return []
    patterns: List[str] = []
    for raw in gi.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or line.startswith("!"):
            continue
        if line.startswith("/"):
            line = line[1:]
        if line.endswith("/"):
            line = line[:-1]
        if line:
            patterns.append(line)
    return patterns


def _resolve_ignore_patterns(
    root_path: Path,
    extra: Optional[Iterable[str]] = None,
) -> List[str]:
    out: List[str] = list(DEFAULT_IGNORES)
    out.extend(_read_gitignore_patterns(root_path))
    if extra:
        out.extend(extra)
    # Stable de-dup preserving order
    seen = set()
    deduped = []
    for p in out:
        if p not in seen:
            seen.add(p)
            deduped.append(p)
    return deduped


def _matches_any(name: str, patterns: Iterable[str]) -> bool:
    """Return True if ``name`` matches any glob pattern (basename match)."""
    for pat in patterns:
        if fnmatch.fnmatchcase(name, pat):
            return True
    return False


def generate_tree(
    root_path: Optional[Path] = None,
    extra_ignore: Optional[Iterable[str]] = None,
) -> str:
    """
    Render a tree of ``root_path`` using the system ``tree`` binary if
    present, else a stable, sorted Python fallback.

    Both paths honour DEFAULT_IGNORES + .gitignore basenames + any extra
    patterns the caller supplies.
    """
    if root_path is None:
        root_path = Path.cwd()
    root_path = Path(root_path)

    ignore_patterns = _resolve_ignore_patterns(root_path, extra_ignore)

    try:
        subprocess.run(["tree", "--version"], capture_output=True, check=True)
        binary_available = True
    except (subprocess.CalledProcessError, FileNotFoundError):
        binary_available = False

    if binary_available:
        ignore_arg = "|".join(ignore_patterns)
        try:
            result = subprocess.run(
                ["tree", "-I", ignore_arg, "-a", "--noreport", "--dirsfirst"],
                cwd=root_path,
                capture_output=True,
                text=True,
                check=True,
            )
            return result.stdout.strip()
        except subprocess.CalledProcessError:
            pass  # fall through to Python fallback

    return _generate_fallback_tree(root_path, ignore_patterns=ignore_patterns)


def _generate_fallback_tree(
    root_path: Path,
    ignore_patterns: Optional[Sequence[str]] = None,
    prefix: str = "",
    max_depth: Optional[int] = None,
    _current_depth: int = 0,
) -> str:
    """
    Stable, sorted, gitignore-aware Python tree.

    No artificial depth cap by default (matches ``tree``'s behaviour).
    Pass ``max_depth=N`` to truncate.
    """
    if ignore_patterns is None:
        ignore_patterns = _resolve_ignore_patterns(root_path)

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

    items = [it for it in items if not _matches_any(it.name, ignore_patterns)]

    for i, item in enumerate(items):
        is_last = i == len(items) - 1
        connector = "└── " if is_last else "├── "
        next_prefix = "    " if is_last else "│   "

        lines.append(f"{prefix}{connector}{item.name}")

        if item.is_dir() and not item.is_symlink():
            subtree = _generate_fallback_tree(
                item,
                ignore_patterns=ignore_patterns,
                prefix=prefix + next_prefix,
                max_depth=max_depth,
                _current_depth=_current_depth + 1,
            )
            if subtree:
                lines.append(subtree)

    return "\n".join(line for line in lines if line)


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

    # Project root is the parent of docs/
    repo_root = docs_path.parent

    # Generate tree rooted at repo, not the caller's cwd
    tree_output = generate_tree(repo_root)

    # Read template
    template_path = Path(__file__).parent.parent / "templates" / "file-structure.md.template"
    template = template_path.read_text()

    # Fill template
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    content = template.format(timestamp=timestamp, tree_output=tree_output)

    # Write file
    file_structure_path = docs_path / "file-structure.md"
    file_structure_path.write_text(content)
