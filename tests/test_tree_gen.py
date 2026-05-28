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
    assert "```" in content
    assert "test.txt" in content


def test_update_file_structure_is_deterministic(temp_dir):
    """Two regens of a *fully-realized* tree (file-structure.md already
    present) produce byte-identical output. This is the v0.3.3 fix: the
    prior wall-clock ``Last updated:`` line caused the post-merge hook
    to re-stage docs/file-structure.md on every ``git pull`` even when
    nothing in the tree had changed since the regen that was just merged.

    Note: we run an initial regen first to materialize file-structure.md
    in the tree (mirroring the post-merge state where CI has already
    committed the file). The byte-stability assertion is on the second
    and third regens, both of which see the same fully-realized tree.
    """
    docs_dir = temp_dir / "docs"
    docs_dir.mkdir()
    (temp_dir / "test.txt").write_text("test", encoding="utf-8")

    # First regen: materializes docs/file-structure.md in the tree.
    update_file_structure(docs_dir)

    # Subsequent regens see the same tree (now including the just-written
    # file-structure.md) and must produce byte-identical output.
    update_file_structure(docs_dir)
    first = (docs_dir / "file-structure.md").read_bytes()

    update_file_structure(docs_dir)
    second = (docs_dir / "file-structure.md").read_bytes()

    assert first == second, (
        "file-structure.md is not byte-stable across regens of an "
        "unchanged tree; the post-merge hook will re-stage it on every "
        "git pull"
    )


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


# --- 0.B.1 (v0.5.0): generate_file_structure ships max_depth=2 default ---
# Activates the depth-truncation path that already existed in
# _generate_fallback_tree but wasn't reached from the public API.
# Every consuming repo's docs/file-structure.md regen now ships
# token-frugal across the org.

def test_generate_file_structure_defaults_to_depth_2(temp_dir):
    """v0.5.0+: depth defaults to 2 (token-frugal). Deeply nested dirs
    must NOT appear at default depth."""
    from logmind.core.tree_gen import generate_file_structure

    current = temp_dir
    for i in range(5):
        current = current / f"level{i}"
        current.mkdir()
        (current / f"file{i}.txt").write_text("test", encoding="utf-8")

    rendered = generate_file_structure(temp_dir)
    # depth 2 = root + 2 levels → level0, level1 visible; level2+ truncated
    assert "level0" in rendered
    assert "level1" in rendered
    # Deeply nested files MUST be excluded at default depth
    assert "file4.txt" not in rendered
    assert "file3.txt" not in rendered


def test_generate_file_structure_max_depth_none_is_unbounded(temp_dir):
    """Explicit None unrolls the full tree (CLI --max-depth 0 path)."""
    from logmind.core.tree_gen import generate_file_structure

    current = temp_dir
    for i in range(5):
        current = current / f"level{i}"
        current.mkdir()
        (current / f"file{i}.txt").write_text("test", encoding="utf-8")

    rendered = generate_file_structure(temp_dir, max_depth=None)
    assert "level4" in rendered
    assert "file4.txt" in rendered


def test_generate_file_structure_default_shorter_than_unbounded(temp_dir):
    """The depth-2 default MUST produce a strictly shorter output than
    unbounded on a sufficiently deep tree. This is the load-bearing
    property — if this regresses, every consuming repo's file-structure.md
    silently grows back."""
    from logmind.core.tree_gen import generate_file_structure

    current = temp_dir
    for i in range(8):
        current = current / f"level{i}"
        current.mkdir()
        for j in range(3):
            (current / f"f{j}.txt").write_text("x", encoding="utf-8")

    default = generate_file_structure(temp_dir)
    full = generate_file_structure(temp_dir, max_depth=None)
    assert len(default) < len(full), (
        f"depth-2 default ({len(default)} chars) should be shorter than "
        f"unbounded ({len(full)} chars) on an 8-deep tree"
    )


def test_write_file_structure_default_depth_writes_truncated_file(temp_dir):
    """Round-trip: write_file_structure with default depth produces a
    file that excludes deeply nested entries."""
    from logmind.core.tree_gen import write_file_structure

    current = temp_dir
    for i in range(5):
        current = current / f"level{i}"
        current.mkdir()
        (current / f"file{i}.txt").write_text("test", encoding="utf-8")

    target = temp_dir / "docs" / "file-structure.md"
    write_file_structure(target, repo_root=temp_dir)
    contents = target.read_text(encoding="utf-8")
    assert "level0" in contents
    # level3 file MUST NOT appear at default depth
    assert "file3.txt" not in contents


def test_generate_tree_binary_and_fallback_agree_at_same_max_depth(temp_dir):
    """0.B.1 regression (caught by clud-bug on PR #68): the tree(1) binary
    path and the Python fallback path MUST produce the same depth bound
    for the same max_depth argument. The first PR revision had a spurious
    `-L max_depth + 1` that made tree(1) display one level deeper than
    the fallback. This test exposes the off-by-one by checking that
    deeply-nested file names appear or don't appear in BOTH paths
    consistently."""
    import shutil
    from logmind.core.tree_gen import _generate_fallback_tree, generate_tree

    if shutil.which("tree") is None:
        # No tree(1) binary on this system — the binary/fallback comparison
        # has no signal. Skip.
        return

    current = temp_dir
    for i in range(5):
        current = current / f"level{i}"
        current.mkdir()
        (current / f"file{i}.txt").write_text("x", encoding="utf-8")

    # At max_depth=2, both paths should:
    # - include level0 + level1 (their names appear as listed by their
    #   parent's iteration)
    # - NOT recurse into level1 (so level1's contents level2/file1.txt
    #   are NOT visible)
    binary_out = generate_tree(temp_dir, max_depth=2)
    fallback_out = _generate_fallback_tree(temp_dir, max_depth=2)

    for path in [binary_out, fallback_out]:
        assert "level0" in path, "level0 should be visible at depth 2"
        assert "level1" in path, "level1 should be visible at depth 2"
        # The off-by-one: tree(1) -L 3 would show level2 + file1.txt here.
        # With the fix (-L 2), neither path shows them.
        assert "level2" not in path, (
            "level2 should NOT be visible at depth 2 — if this fails, "
            "the tree(1) path is rendering one level deeper than Python "
            "(the off-by-one PR #68 was supposed to fix)"
        )
        assert "file1.txt" not in path
