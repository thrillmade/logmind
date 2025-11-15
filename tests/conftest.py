"""Pytest configuration and fixtures."""

import tempfile
from pathlib import Path

import pytest


@pytest.fixture
def temp_dir():
    """Create a temporary directory for tests."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def git_repo(temp_dir):
    """Create a temporary git repository."""
    import subprocess

    subprocess.run(["git", "init"], cwd=temp_dir, check=True, capture_output=True)
    subprocess.run(
        ["git", "config", "user.name", "Test User"],
        cwd=temp_dir,
        check=True,
        capture_output=True,
    )
    subprocess.run(
        ["git", "config", "user.email", "test@example.com"],
        cwd=temp_dir,
        check=True,
        capture_output=True,
    )

    return temp_dir


@pytest.fixture
def docs_dir(temp_dir):
    """Create a docs directory with decision log files."""
    docs = temp_dir / "docs"
    docs.mkdir()

    # Create template files
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n")
    (docs / "decisions-archive.md").write_text("# Decision Archive\n\n---\n")
    (docs / "file-structure.md").write_text("# File Structure\n\n```\n.\n```\n")

    return docs
