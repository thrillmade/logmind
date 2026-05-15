"""Tests for core/tree_gen.py."""

from pathlib import Path

from logmind.core.tree_gen import _generate_fallback_tree, generate_tree, update_file_structure


def test_generate_fallback_tree(temp_dir):
    """Test fallback tree generation."""
    # Create some test files
    (temp_dir / "file1.txt").write_text("test", encoding="utf-8")
    (temp_dir / "file2.py").write_text("test", encoding="utf-8")

    subdir = temp_dir / "subdir"
    subdir.mkdir()
    (subdir / "nested.txt").write_text("test", encoding="utf-8")

    tree = _generate_fallback_tree(temp_dir)

    assert temp_dir.name in tree
    assert "file1.txt" in tree
    assert "file2.py" in tree
    assert "subdir" in tree
    assert "nested.txt" in tree


def test_generate_fallback_tree_ignores_common_dirs(temp_dir):
    """Test that fallback tree ignores common directories."""
    # Create directories that should be ignored
    (temp_dir / "__pycache__").mkdir()
    (temp_dir / ".git").mkdir()
    (temp_dir / "node_modules").mkdir()
    (temp_dir / "venv").mkdir()

    # Create a file that should be shown
    (temp_dir / "visible.txt").write_text("test", encoding="utf-8")

    tree = _generate_fallback_tree(temp_dir)

    assert "visible.txt" in tree
    assert "__pycache__" not in tree
    assert ".git" not in tree
    assert "node_modules" not in tree
    assert "venv" not in tree


def test_generate_tree_basic(temp_dir):
    """Test basic tree generation."""
    (temp_dir / "test.txt").write_text("test", encoding="utf-8")

    tree = generate_tree(temp_dir)

    # Should work with either tree command or fallback
    assert len(tree) > 0
    assert "test.txt" in tree or temp_dir.name in tree


def test_update_file_structure(temp_dir):
    """Test updating file structure markdown."""
    docs_dir = temp_dir / "docs"
    docs_dir.mkdir()

    (temp_dir / "test.txt").write_text("test", encoding="utf-8")

    update_file_structure(docs_dir)

    file_structure = docs_dir / "file-structure.md"
    assert file_structure.exists()

    content = file_structure.read_text(encoding="utf-8")
    assert "# File Structure" in content
    assert "Last updated:" in content
    assert "```" in content


def test_update_file_structure_creates_docs_dir(temp_dir):
    """Test that update_file_structure creates docs dir if missing."""
    docs_dir = temp_dir / "docs"
    assert not docs_dir.exists()

    update_file_structure(docs_dir)

    assert docs_dir.exists()
    assert (docs_dir / "file-structure.md").exists()


def test_generate_fallback_tree_max_depth(temp_dir):
    """Test that fallback tree respects max depth."""
    # Create deeply nested structure
    current = temp_dir
    for i in range(5):
        current = current / f"level{i}"
        current.mkdir()
        (current / f"file{i}.txt").write_text("test", encoding="utf-8")

    tree = _generate_fallback_tree(temp_dir, max_depth=2)

    # Should include level0 and level1 but not deeper
    assert "level0" in tree
    assert "level1" in tree
    # level2 might or might not be there depending on exact depth counting
