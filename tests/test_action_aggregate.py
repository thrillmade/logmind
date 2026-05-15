"""Tests for logmind.actions.aggregate (Phase 6 PR-merge aggregator)."""

from __future__ import annotations

from datetime import datetime
from pathlib import Path

from logmind.actions.aggregate import aggregate, main


def _seed_branch_file(docs: Path, sanitized: str, decisions: list[str]) -> Path:
    branch_dir = docs / "decisions-branches"
    branch_dir.mkdir(parents=True, exist_ok=True)
    p = branch_dir / f"{sanitized}.md"
    body = ["# Decision Log\n", "---\n"]
    for i, d in enumerate(decisions):
        body.append(f"## 2026-05-14 12:0{i} - {d}\n\n**Reasoning:** test\n\n---\n")
    p.write_text("\n".join(body))
    return p


def _make_docs(root: Path) -> Path:
    docs = root / "docs"
    docs.mkdir()
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n")
    return docs


def test_aggregate_appends_merge_entry(tmp_path):
    docs = _make_docs(tmp_path)
    _seed_branch_file(docs, "feat__x", ["one", "two", "three"])

    out = aggregate(
        branch="feat/x",
        pr_number=42,
        pr_url="https://github.com/owner/repo/pull/42",
        docs_path=docs,
        timestamp=datetime(2026, 5, 14, 9, 30),
    )

    assert out == docs / "decisions.md"
    content = out.read_text()
    assert "Merged: feat/x (#42)" in content
    assert "https://github.com/owner/repo/pull/42" in content
    assert "decisions-branches/feat__x.md" in content
    assert "Decisions:** 3" in content


def test_aggregate_returns_none_when_branch_file_missing(tmp_path):
    docs = _make_docs(tmp_path)
    out = aggregate("missing-branch", 1, "url", docs)
    assert out is None
    # decisions.md untouched (no merge entry)
    assert "Merged" not in (docs / "decisions.md").read_text()


def test_aggregate_returns_none_when_branch_file_has_no_decisions(tmp_path):
    docs = _make_docs(tmp_path)
    _seed_branch_file(docs, "feat__empty", [])  # empty body
    out = aggregate("feat/empty", 7, "url", docs)
    assert out is None


def test_aggregate_uses_sanitized_branch_for_link(tmp_path):
    docs = _make_docs(tmp_path)
    _seed_branch_file(docs, "user__jane__feature", ["one"])

    aggregate(
        branch="user/jane/feature",
        pr_number=99,
        pr_url="https://example.com/pr/99",
        docs_path=docs,
    )

    content = (docs / "decisions.md").read_text()
    assert "[decisions-branches/user__jane__feature.md]" in content
    assert "Merged: user/jane/feature (#99)" in content


def test_main_uses_env_vars(tmp_path, monkeypatch, capsys):
    docs = _make_docs(tmp_path)
    _seed_branch_file(docs, "feat__y", ["alpha"])
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv("BRANCH_NAME", "feat/y")
    monkeypatch.setenv("PR_NUMBER", "11")
    monkeypatch.setenv("PR_URL", "https://example.com/pr/11")

    rc = main()
    assert rc == 0
    out = capsys.readouterr().out
    assert "appended merge summary for feat/y" in out
    assert "Merged: feat/y (#11)" in (docs / "decisions.md").read_text()


def test_main_errors_when_env_missing(monkeypatch, capsys):
    monkeypatch.delenv("BRANCH_NAME", raising=False)
    monkeypatch.delenv("PR_NUMBER", raising=False)
    monkeypatch.delenv("PR_URL", raising=False)
    monkeypatch.delenv("GITHUB_HEAD_REF", raising=False)
    rc = main()
    assert rc == 2
    err = capsys.readouterr().err
    assert "BRANCH_NAME" in err


def test_main_errors_on_non_int_pr_number(tmp_path, monkeypatch, capsys):
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv("BRANCH_NAME", "feat/z")
    monkeypatch.setenv("PR_NUMBER", "not-a-number")
    monkeypatch.setenv("PR_URL", "https://example.com/pr/12")
    rc = main()
    assert rc == 2
    assert "PR_NUMBER must be an integer" in capsys.readouterr().err


def test_main_no_op_when_no_branch_file(tmp_path, monkeypatch, capsys):
    """When the branch file is missing the action should exit 0 silently."""
    docs = _make_docs(tmp_path)
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv("BRANCH_NAME", "feat/nonexistent")
    monkeypatch.setenv("PR_NUMBER", "100")
    monkeypatch.setenv("PR_URL", "https://example.com/pr/100")
    rc = main()
    assert rc == 0
    out = capsys.readouterr().out
    assert "nothing to do" in out
