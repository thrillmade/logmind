"""File structure generation using tree command."""

import subprocess
from datetime import datetime
from pathlib import Path
from typing import Optional


def generate_tree(root_path: Optional[Path] = None) -> str:
    """
    Generate file structure using tree command.

    Args:
        root_path: Root directory to generate tree for. Defaults to current directory.

    Returns:
        Tree output as string

    Raises:
        FileNotFoundError: If tree command is not available
        subprocess.CalledProcessError: If tree command fails
    """
    if root_path is None:
        root_path = Path.cwd()

    # Check if tree command exists
    try:
        subprocess.run(["tree", "--version"], capture_output=True, check=True)
    except (subprocess.CalledProcessError, FileNotFoundError):
        # Fallback: use simple directory listing if tree not available
        return _generate_fallback_tree(root_path)

    # Run tree command with exclusions
    ignore_patterns = [
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
        "dist",
        "build",
        "*.egg-info",
    ]

    ignore_arg = "|".join(ignore_patterns)

    try:
        result = subprocess.run(
            ["tree", "-I", ignore_arg, "-a", "--noreport"],
            cwd=root_path,
            capture_output=True,
            text=True,
            check=True,
        )
        return result.stdout.strip()
    except subprocess.CalledProcessError as e:
        # If tree fails, use fallback
        return _generate_fallback_tree(root_path)


def _generate_fallback_tree(root_path: Path, prefix: str = "", max_depth: int = 3, current_depth: int = 0) -> str:
    """
    Generate a simple tree structure without the tree command.

    Args:
        root_path: Root directory
        prefix: Prefix for current line
        max_depth: Maximum depth to traverse
        current_depth: Current depth level

    Returns:
        Simple tree structure as string
    """
    if current_depth >= max_depth:
        return ""

    ignore_names = {
        "__pycache__", ".git", "node_modules", "venv", ".venv",
        "env", ".env", ".pytest_cache", ".mypy_cache", "dist",
        "build", ".egg-info"
    }

    lines = [str(root_path.name) if current_depth == 0 else ""]

    try:
        items = sorted(root_path.iterdir(), key=lambda x: (not x.is_dir(), x.name))
        items = [item for item in items if item.name not in ignore_names and not item.name.endswith('.pyc')]

        for i, item in enumerate(items):
            is_last = i == len(items) - 1
            current_prefix = "└── " if is_last else "├── "
            next_prefix = "    " if is_last else "│   "

            lines.append(f"{prefix}{current_prefix}{item.name}")

            if item.is_dir() and not item.is_symlink():
                subtree = _generate_fallback_tree(
                    item,
                    prefix + next_prefix,
                    max_depth,
                    current_depth + 1
                )
                if subtree:
                    lines.append(subtree)

    except PermissionError:
        pass

    return "\n".join(line for line in lines if line)


def update_file_structure(docs_path: Optional[Path] = None) -> None:
    """
    Update the file-structure.md file with current project tree.

    Args:
        docs_path: Path to docs directory. Defaults to ./docs
    """
    if docs_path is None:
        docs_path = Path.cwd() / "docs"

    docs_path.mkdir(parents=True, exist_ok=True)

    # Generate tree
    tree_output = generate_tree()

    # Read template
    template_path = Path(__file__).parent.parent / "templates" / "file-structure.md.template"
    template = template_path.read_text()

    # Fill template
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    content = template.format(timestamp=timestamp, tree_output=tree_output)

    # Write file
    file_structure_path = docs_path / "file-structure.md"
    file_structure_path.write_text(content)
