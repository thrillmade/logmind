"""Tests for branch-aware decision storage (Phase 5)."""

import subprocess
from pathlib import Path

import pytest

from logmind.core.git_handler import current_branch, default_branch
from logmind.core.logger import (
    _archive_path_for,
    _resolve_decisions_path,
    _sanitize_branch,
    log,
)


def _git(args, cwd):
    subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True)


def _init_repo_on_branch(path: Path, branch: str = "main") -> None:
    """Initialise a git repo with a single initial commit on the named branch."""
    _git(["init", "-b", branch], path)
    _git(["config", "user.name", "Test"], path)
    _git(["config", "user.email", "test@example.com"], path)
    (path / "README").write_text("seed\n")
    _git(["add", "."], path)
    _git(["commit", "-m", "init"], path)


def _checkout_new_branch(path: Path, branch: str) -> None:
    _git(["checkout", "-b", branch], path)


# ---------------------------------------------------------------------------
# Pure helpers
# ---------------------------------------------------------------------------


def test_sanitize_branch_replaces_slashes():
    assert _sanitize_branch("feat/x") == "feat__x"
    assert _sanitize_branch("user/jane/feature") == "user__jane__feature"


def test_sanitize_branch_replaces_backslashes_and_colons():
    assert _sanitize_branch("a\\b") == "a__b"
    assert _sanitize_branch("a:b") == "a_b"


def test_sanitize_branch_passthrough_for_simple_names():
    assert _sanitize_branch("main") == "main"
    assert _sanitize_branch("feature-x") == "feature-x"


def test_archive_path_for_main_decisions_file(tmp_path):
    p = tmp_path / "decisions.md"
    assert _archive_path_for(p) == tmp_path / "decisions-archive.md"


def test_archive_path_for_branch_decisions_file(tmp_path):
    branch_dir = tmp_path / "decisions-branches"
    branch_dir.mkdir()
    p = branch_dir / "feat__x.md"
    assert _archive_path_for(p) == branch_dir / "feat__x-archive.md"


# ---------------------------------------------------------------------------
# git_handler helpers
# ---------------------------------------------------------------------------


def test_current_branch_returns_none_for_non_git(temp_dir):
    assert current_branch(temp_dir) is None


def test_current_branch_returns_initial_branch(temp_dir):
    _init_repo_on_branch(temp_dir, "main")
    assert current_branch(temp_dir) == "main"


def test_current_branch_follows_checkout(temp_dir):
    _init_repo_on_branch(temp_dir, "main")
    _checkout_new_branch(temp_dir, "feat/x")
    assert current_branch(temp_dir) == "feat/x"


def test_default_branch_uses_init_default(temp_dir):
    _init_repo_on_branch(temp_dir, "main")
    # No remote, but local 'main' exists → default resolves to "main".
    assert default_branch(temp_dir) == "main"


def test_default_branch_falls_back_to_master(temp_dir):
    _init_repo_on_branch(temp_dir, "master")
    assert default_branch(temp_dir) == "master"


def test_default_branch_with_no_repo_returns_main(temp_dir):
    assert default_branch(temp_dir) == "main"


# ---------------------------------------------------------------------------
# _resolve_decisions_path
# ---------------------------------------------------------------------------


def test_resolve_path_default_branch(tmp_path, monkeypatch):
    """On the default branch, the canonical decisions.md is used."""
    docs = tmp_path / "docs"
    docs.mkdir()
    _init_repo_on_branch(tmp_path, "main")
    monkeypatch.chdir(tmp_path)

    from logmind.core.config import load_config

    config = load_config()
    resolved = _resolve_decisions_path(docs, config)
    assert resolved == docs / "decisions.md"


def test_resolve_path_feature_branch(tmp_path, monkeypatch):
    docs = tmp_path / "docs"
    docs.mkdir()
    _init_repo_on_branch(tmp_path, "main")
    _checkout_new_branch(tmp_path, "feat/x")
    monkeypatch.chdir(tmp_path)

    from logmind.core.config import load_config

    config = load_config()
    resolved = _resolve_decisions_path(docs, config)
    assert resolved == docs / "decisions-branches" / "feat__x.md"
    assert resolved.parent.exists()  # mkdir side-effect


def test_resolve_path_non_git_dir(tmp_path, monkeypatch):
    docs = tmp_path / "docs"
    docs.mkdir()
    monkeypatch.chdir(tmp_path)

    from logmind.core.config import load_config

    config = load_config()
    resolved = _resolve_decisions_path(docs, config)
    assert resolved == docs / "decisions.md"


def test_resolve_path_opt_out(tmp_path, monkeypatch):
    """branch_aware: false routes feature branches back to decisions.md."""
    docs = tmp_path / "docs"
    docs.mkdir()
    _init_repo_on_branch(tmp_path, "main")
    _checkout_new_branch(tmp_path, "feat/x")
    monkeypatch.chdir(tmp_path)

    (tmp_path / ".logmind").mkdir()
    (tmp_path / ".logmind" / "config.yml").write_text(
        "decisions:\n  branch_aware: false\n"
    )

    from logmind.core.config import load_config

    config = load_config()
    resolved = _resolve_decisions_path(docs, config)
    assert resolved == docs / "decisions.md"


# ---------------------------------------------------------------------------
# log() end-to-end
# ---------------------------------------------------------------------------


def _make_docs_dir(root: Path) -> Path:
    docs = root / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n")
    (docs / "decisions-archive.md").write_text("# Decision Archive\n\n---\n")
    (docs / "file-structure.md").write_text("# File Structure\n\n```\n.\n```\n")
    return docs


def test_log_on_default_branch_writes_to_decisions_md(tmp_path, monkeypatch):
    _init_repo_on_branch(tmp_path, "main")
    monkeypatch.chdir(tmp_path)
    docs = _make_docs_dir(tmp_path)

    log("Default-branch entry", reasoning="r", docs_path=docs, auto_commit=False)

    assert "Default-branch entry" in (docs / "decisions.md").read_text()
    assert not (docs / "decisions-branches").exists()


def test_log_on_feature_branch_writes_to_per_branch_file(tmp_path, monkeypatch):
    _init_repo_on_branch(tmp_path, "main")
    _checkout_new_branch(tmp_path, "feat/x")
    monkeypatch.chdir(tmp_path)
    docs = _make_docs_dir(tmp_path)

    log("Feature-branch entry", reasoning="r", docs_path=docs, auto_commit=False)

    branch_file = docs / "decisions-branches" / "feat__x.md"
    assert branch_file.exists()
    assert "Feature-branch entry" in branch_file.read_text()
    # Untouched
    assert "Feature-branch entry" not in (docs / "decisions.md").read_text()


def test_log_branch_aware_disabled_still_uses_decisions_md(tmp_path, monkeypatch):
    _init_repo_on_branch(tmp_path, "main")
    _checkout_new_branch(tmp_path, "feat/x")
    monkeypatch.chdir(tmp_path)
    docs = _make_docs_dir(tmp_path)

    (tmp_path / ".logmind").mkdir()
    (tmp_path / ".logmind" / "config.yml").write_text(
        "decisions:\n  branch_aware: false\n"
    )

    log("Legacy single-file entry", reasoning="r", docs_path=docs, auto_commit=False)

    assert "Legacy single-file entry" in (docs / "decisions.md").read_text()
    assert not (docs / "decisions-branches").exists()


def test_archival_on_branch_file_uses_paired_archive(tmp_path, monkeypatch):
    """Branch files archive into <stem>-archive.md, not the main archive."""
    _init_repo_on_branch(tmp_path, "main")
    _checkout_new_branch(tmp_path, "feat/y")
    monkeypatch.chdir(tmp_path)
    docs = _make_docs_dir(tmp_path)

    # Cap recent at 2 to force archival quickly.
    (tmp_path / ".logmind").mkdir()
    (tmp_path / ".logmind" / "config.yml").write_text(
        "decisions:\n  max_recent: 2\n  branch_aware: true\n"
    )

    log("first", docs_path=docs, auto_commit=False)
    log("second", docs_path=docs, auto_commit=False)
    log("third", docs_path=docs, auto_commit=False)  # triggers archival

    branch_file = docs / "decisions-branches" / "feat__y.md"
    branch_archive = docs / "decisions-branches" / "feat__y-archive.md"

    assert branch_archive.exists(), "branch-paired archive should be created"
    assert "first" in branch_archive.read_text()
    # Main archive untouched
    assert "first" not in (docs / "decisions-archive.md").read_text()


def test_log_outside_git_repo_falls_back_to_decisions_md(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    docs = _make_docs_dir(tmp_path)

    log("No git here", reasoning="r", docs_path=docs, auto_commit=False)

    assert "No git here" in (docs / "decisions.md").read_text()
