"""Tests for core/git_handler.py."""

import subprocess
from pathlib import Path

import pytest

from logmind.core.git_handler import (
    GitError,
    commit_and_push,
    git_add,
    git_add_all,
    git_commit,
    git_push,
    is_git_repo,
)


def test_is_git_repo_true(git_repo):
    """Test detecting a git repository."""
    assert is_git_repo(git_repo) is True


def test_is_git_repo_false(temp_dir):
    """Test detecting non-git directory."""
    assert is_git_repo(temp_dir) is False


def test_git_add_single_file(git_repo):
    """Test adding a single file."""
    test_file = git_repo / "test.txt"
    test_file.write_text("test content", encoding="utf-8")

    git_add(["test.txt"], git_repo)

    # Check file is staged
    result = subprocess.run(
        ["git", "diff", "--cached", "--name-only"],
        cwd=git_repo,
        capture_output=True,
        text=True,
    )
    assert "test.txt" in result.stdout


def test_git_add_multiple_files(git_repo):
    """Test adding multiple files."""
    (git_repo / "file1.txt").write_text("test", encoding="utf-8")
    (git_repo / "file2.txt").write_text("test", encoding="utf-8")

    git_add(["file1.txt", "file2.txt"], git_repo)

    result = subprocess.run(
        ["git", "diff", "--cached", "--name-only"],
        cwd=git_repo,
        capture_output=True,
        text=True,
    )
    assert "file1.txt" in result.stdout
    assert "file2.txt" in result.stdout


def test_git_add_empty_list(git_repo):
    """Test that adding empty list doesn't error."""
    git_add([], git_repo)  # Should not raise


def test_git_commit_with_message(git_repo):
    """Test committing with a message."""
    test_file = git_repo / "test.txt"
    test_file.write_text("test", encoding="utf-8")

    git_add(["test.txt"], git_repo)
    git_commit("Test commit message", git_repo)

    # Check commit exists
    result = subprocess.run(
        ["git", "log", "--oneline", "-1"],
        cwd=git_repo,
        capture_output=True,
        text=True,
    )
    assert "Test commit message" in result.stdout


def test_git_commit_nothing_to_commit(git_repo):
    """Test that committing with nothing staged doesn't error."""
    git_commit("Empty commit", git_repo)  # Should not raise


def test_git_push_no_remote(git_repo):
    """Test that push without remote doesn't error."""
    # Should fail silently when no remote configured
    git_push(git_repo)  # Should not raise


def test_commit_and_push(git_repo):
    """Test combined commit and push operation."""
    test_file = git_repo / "test.txt"
    test_file.write_text("test", encoding="utf-8")

    commit_and_push(["test.txt"], "Test commit", git_repo, push=False)

    # Check commit exists
    result = subprocess.run(
        ["git", "log", "--oneline", "-1"],
        cwd=git_repo,
        capture_output=True,
        text=True,
    )
    assert "Test commit" in result.stdout


def test_commit_and_push_not_git_repo_raises_error(temp_dir):
    """Test that commit_and_push raises error in non-git directory."""
    with pytest.raises(GitError, match="Not a git repository"):
        commit_and_push(["test.txt"], "Test", temp_dir)


def test_git_add_nonexistent_file_raises_error(git_repo):
    """Test that adding nonexistent file raises error."""
    with pytest.raises(GitError, match="Failed to add files"):
        git_add(["nonexistent.txt"], git_repo)


def test_git_add_all_stages_all_files(git_repo):
    """Test that git_add_all stages all untracked files."""
    (git_repo / "alpha.txt").write_text("alpha content", encoding="utf-8")
    (git_repo / "beta.txt").write_text("beta content", encoding="utf-8")

    git_add_all(git_repo)

    result = subprocess.run(
        ["git", "status", "--porcelain"],
        cwd=git_repo,
        capture_output=True,
        text=True,
    )
    assert "A  alpha.txt" in result.stdout or "A alpha.txt" in result.stdout
    assert "A  beta.txt" in result.stdout or "A beta.txt" in result.stdout
