"""Git operations for logmind."""

import subprocess
from pathlib import Path
from typing import List, Optional


class GitError(Exception):
    """Exception raised for git operation errors."""
    pass


def is_git_repo(path: Optional[Path] = None) -> bool:
    """
    Check if the current directory is a git repository.

    Args:
        path: Directory to check. Defaults to current directory.

    Returns:
        True if directory is a git repo, False otherwise
    """
    if path is None:
        path = Path.cwd()

    try:
        subprocess.run(
            ["git", "rev-parse", "--git-dir"],
            cwd=path,
            capture_output=True,
            check=True,
        )
        return True
    except (subprocess.CalledProcessError, FileNotFoundError):
        return False


def git_add(files: List[str], path: Optional[Path] = None) -> None:
    """
    Add files to git staging area.

    Args:
        files: List of file paths to add
        path: Repository root. Defaults to current directory.

    Raises:
        GitError: If git add fails
    """
    if path is None:
        path = Path.cwd()

    if not files:
        return

    try:
        subprocess.run(
            ["git", "add"] + files,
            cwd=path,
            capture_output=True,
            check=True,
            text=True,
        )
    except subprocess.CalledProcessError as e:
        raise GitError(f"Failed to add files to git: {e.stderr}")


def git_commit(message: str, path: Optional[Path] = None) -> None:
    """
    Commit staged changes with a message.

    Args:
        message: Commit message
        path: Repository root. Defaults to current directory.

    Raises:
        GitError: If git commit fails
    """
    if path is None:
        path = Path.cwd()

    try:
        subprocess.run(
            ["git", "commit", "-m", message],
            cwd=path,
            capture_output=True,
            check=True,
            text=True,
        )
    except subprocess.CalledProcessError as e:
        # It's okay if there's nothing to commit
        if "nothing to commit" in e.stdout or "nothing to commit" in e.stderr:
            return
        raise GitError(f"Failed to commit: {e.stderr}")


def git_push(path: Optional[Path] = None) -> None:
    """
    Push commits to remote.

    Args:
        path: Repository root. Defaults to current directory.

    Raises:
        GitError: If git push fails
    """
    if path is None:
        path = Path.cwd()

    try:
        subprocess.run(
            ["git", "push"],
            cwd=path,
            capture_output=True,
            check=True,
            text=True,
        )
    except subprocess.CalledProcessError as e:
        # Don't fail if there's no remote configured
        if "No configured push destination" in e.stderr or "no upstream branch" in e.stderr:
            # Silent fail - not all repos have remotes
            return
        raise GitError(f"Failed to push: {e.stderr}")


def commit_and_push(files: List[str], message: str, path: Optional[Path] = None, push: bool = True) -> None:
    """
    Add files, commit, and optionally push in one operation.

    Args:
        files: List of file paths to commit
        message: Commit message
        path: Repository root. Defaults to current directory.
        push: Whether to push after committing. Defaults to True.

    Raises:
        GitError: If any git operation fails
    """
    if not is_git_repo(path):
        raise GitError("Not a git repository. Initialize git first with 'git init'")

    git_add(files, path)
    git_commit(message, path)

    if push:
        git_push(path)
