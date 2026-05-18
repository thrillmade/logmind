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
    except (subprocess.CalledProcessError, FileNotFoundError, OSError):
        # OSError covers permission errors on .git/, unreachable network
        # mounts, etc. Treat as "not a git repo" and let callers decide.
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


def git_add_all(path: Optional[Path] = None) -> None:
    """
    Add all changed files to git staging area (git add .).

    Args:
        path: Repository root. Defaults to current directory.

    Raises:
        GitError: If git add fails
    """
    if path is None:
        path = Path.cwd()

    try:
        subprocess.run(
            ["git", "add", "."],
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


def current_branch(path: Optional[Path] = None) -> Optional[str]:
    """
    Return the current git branch name.

    Returns None if the path is not a git repo or HEAD is detached.
    Works on freshly-init'd repos with no commits yet (returns the
    unborn-branch name from `git symbolic-ref HEAD`).

    Args:
        path: Repository root. Defaults to current directory.
    """
    if path is None:
        path = Path.cwd()

    try:
        result = subprocess.run(
            ["git", "symbolic-ref", "--short", "HEAD"],
            cwd=path,
            capture_output=True,
            check=True,
            text=True,
        )
        branch = result.stdout.strip()
        return branch or None
    except (subprocess.CalledProcessError, FileNotFoundError, OSError):
        # OSError covers permission errors on .git/HEAD, etc. Caller code
        # already handles None-as-detached-HEAD; treat permission errors
        # the same way.
        return None


def default_branch(path: Optional[Path] = None) -> str:
    """
    Return the repo's default branch name.

    Resolution order:
      1. `refs/remotes/origin/HEAD` (set by `git clone` or `git remote set-head`)
      2. Local `main` if it exists, else local `master`
      3. If exactly one local branch exists, use it
      4. `git config init.defaultBranch`
      5. Hard fallback: "main"

    Args:
        path: Repository root. Defaults to current directory.
    """
    if path is None:
        path = Path.cwd()

    # 1. origin/HEAD
    try:
        result = subprocess.run(
            ["git", "symbolic-ref", "refs/remotes/origin/HEAD"],
            cwd=path,
            capture_output=True,
            check=True,
            text=True,
        )
        ref = result.stdout.strip()
        prefix = "refs/remotes/origin/"
        if ref.startswith(prefix):
            return ref[len(prefix):]
    except (subprocess.CalledProcessError, FileNotFoundError):
        pass

    # 2. Local main / master
    for candidate in ("main", "master"):
        try:
            subprocess.run(
                ["git", "show-ref", "--verify", "--quiet", f"refs/heads/{candidate}"],
                cwd=path,
                check=True,
                capture_output=True,
            )
            return candidate
        except (subprocess.CalledProcessError, FileNotFoundError):
            continue

    # 3. Single-branch repo: that branch IS the default by definition
    try:
        result = subprocess.run(
            ["git", "for-each-ref", "--format=%(refname:short)", "refs/heads/"],
            cwd=path,
            capture_output=True,
            check=True,
            text=True,
        )
        local_branches = [b for b in result.stdout.split() if b]
        if len(local_branches) == 1:
            return local_branches[0]
    except (subprocess.CalledProcessError, FileNotFoundError):
        pass

    # 4. init.defaultBranch
    try:
        result = subprocess.run(
            ["git", "config", "--get", "init.defaultBranch"],
            cwd=path,
            capture_output=True,
            check=True,
            text=True,
        )
        configured = result.stdout.strip()
        if configured:
            return configured
    except (subprocess.CalledProcessError, FileNotFoundError):
        pass

    return "main"


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
