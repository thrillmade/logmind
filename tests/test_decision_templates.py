"""Tests for decision templates feature."""

import pytest
from pathlib import Path
from click.testing import CliRunner

from logmind.cli import main
from logmind.core.decision_templates import get_template, list_templates, TEMPLATES


# ---------------------------------------------------------------------------
# decision_templates module tests
# ---------------------------------------------------------------------------


class TestGetTemplate:
    def test_returns_known_template(self):
        tmpl = get_template("database")
        assert tmpl is not None
        assert "reasoning" in tmpl
        assert "alternatives" in tmpl
        assert "implications" in tmpl

    def test_returns_none_for_unknown(self):
        assert get_template("nonexistent") is None

    def test_case_insensitive(self):
        assert get_template("DATABASE") == get_template("database")
        assert get_template("Api") == get_template("api")

    def test_all_built_in_templates_exist(self):
        expected = ["database", "api", "architecture", "security", "performance", "library", "deployment"]
        for name in expected:
            assert get_template(name) is not None, f"Template '{name}' not found"

    def test_each_template_has_required_fields(self):
        for name, tmpl in TEMPLATES.items():
            assert "description" in tmpl, f"{name} missing description"
            assert "reasoning" in tmpl, f"{name} missing reasoning"
            assert "alternatives" in tmpl, f"{name} missing alternatives"
            assert "implications" in tmpl, f"{name} missing implications"

    def test_alternatives_is_list(self):
        for name, tmpl in TEMPLATES.items():
            assert isinstance(tmpl["alternatives"], list), f"{name} alternatives not a list"

    def test_implications_is_list(self):
        for name, tmpl in TEMPLATES.items():
            assert isinstance(tmpl["implications"], list), f"{name} implications not a list"


class TestListTemplates:
    def test_returns_dict(self):
        result = list_templates()
        assert isinstance(result, dict)

    def test_keys_match_templates(self):
        assert set(list_templates().keys()) == set(TEMPLATES.keys())

    def test_values_are_descriptions(self):
        for name, desc in list_templates().items():
            assert desc == TEMPLATES[name]["description"]


# ---------------------------------------------------------------------------
# CLI: logmind templates command
# ---------------------------------------------------------------------------


def test_templates_command_lists_all(git_repo):
    """logmind templates lists all built-in template names."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(main, ["templates"])

    assert result.exit_code == 0
    for name in TEMPLATES:
        assert name in result.output


def test_templates_command_shows_descriptions(git_repo):
    """logmind templates shows descriptions alongside names."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(main, ["templates"])

    assert result.exit_code == 0
    assert "Database technology" in result.output or "database" in result.output.lower()


def test_templates_command_shows_usage_hint(git_repo):
    """logmind templates shows how to use a template."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        result = runner.invoke(main, ["templates"])

    assert "logmind log --template" in result.output


# ---------------------------------------------------------------------------
# CLI: logmind log --template
# ---------------------------------------------------------------------------


def _setup_docs(path: Path) -> Path:
    """Create docs directory with required files at path."""
    docs = path / "docs"
    docs.mkdir(exist_ok=True)
    (docs / "decisions.md").write_text("# Decision Log\n\n---\n")
    (docs / "decisions-archive.md").write_text("# Decision Archive\n\n---\n")
    (docs / "file-structure.md").write_text("# File Structure\n\n```\n.\n```\n")
    return docs


def test_log_with_template_prefills_reasoning(git_repo):
    """logmind log --template fills reasoning from template when not provided."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        _setup_docs(Path("."))
        result = runner.invoke(
            main, ["log", "--template", "database", "Use PostgreSQL", "--no-commit"]
        )
        assert result.exit_code == 0, result.output
        decisions = (Path(".") / "docs" / "decisions.md").read_text()

    assert "Evaluated data model" in decisions


def test_log_with_template_prefills_alternatives(git_repo):
    """logmind log --template fills alternatives from template."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        _setup_docs(Path("."))
        result = runner.invoke(
            main, ["log", "--template", "database", "Use PostgreSQL", "--no-commit"]
        )
        assert result.exit_code == 0, result.output
        decisions = (Path(".") / "docs" / "decisions.md").read_text()

    assert "PostgreSQL" in decisions or "MySQL" in decisions


def test_log_with_template_prefills_implications(git_repo):
    """logmind log --template fills implications from template."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        _setup_docs(Path("."))
        result = runner.invoke(
            main, ["log", "--template", "database", "Use PostgreSQL", "--no-commit"]
        )
        assert result.exit_code == 0, result.output
        decisions = (Path(".") / "docs" / "decisions.md").read_text()

    assert "Connection pooling" in decisions or "migration" in decisions.lower()


def test_log_explicit_reasoning_overrides_template(git_repo):
    """Explicit -r flag overrides template reasoning."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        _setup_docs(Path("."))
        result = runner.invoke(
            main,
            [
                "log", "--template", "database",
                "-r", "My custom reasoning",
                "Use PostgreSQL",
                "--no-commit",
            ],
        )
        assert result.exit_code == 0, result.output
        decisions = (Path(".") / "docs" / "decisions.md").read_text()

    assert "My custom reasoning" in decisions
    assert "Evaluated data model" not in decisions


def test_log_explicit_alternatives_override_template(git_repo):
    """Explicit -a flags override template alternatives."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        _setup_docs(Path("."))
        result = runner.invoke(
            main,
            [
                "log", "--template", "database",
                "-a", "CockroachDB",
                "Use PostgreSQL",
                "--no-commit",
            ],
        )
        assert result.exit_code == 0, result.output
        decisions = (Path(".") / "docs" / "decisions.md").read_text()

    assert "CockroachDB" in decisions


def test_log_explicit_implications_override_template(git_repo):
    """Explicit -i flags override template implications."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        _setup_docs(Path("."))
        result = runner.invoke(
            main,
            [
                "log", "--template", "database",
                "-i", "Custom implication here",
                "Use PostgreSQL",
                "--no-commit",
            ],
        )
        assert result.exit_code == 0, result.output
        decisions = (Path(".") / "docs" / "decisions.md").read_text()

    assert "Custom implication here" in decisions


def test_log_unknown_template_fails(git_repo):
    """logmind log --template with unknown name exits with error."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        _setup_docs(Path("."))
        result = runner.invoke(
            main, ["log", "--template", "nonexistent", "Some decision", "--no-commit"]
        )

    assert result.exit_code == 1
    assert "Unknown template" in result.output


def test_log_unknown_template_shows_available(git_repo):
    """Error message for unknown template lists available templates."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        _setup_docs(Path("."))
        result = runner.invoke(
            main, ["log", "--template", "nonexistent", "Some decision", "--no-commit"]
        )

    assert "database" in result.output
    assert "api" in result.output


def test_log_all_templates_work(git_repo):
    """Every built-in template can be used without error."""
    runner = CliRunner()

    for tmpl_name in TEMPLATES:
        with runner.isolated_filesystem(temp_dir=git_repo):
            _setup_docs(Path("."))
            result = runner.invoke(
                main,
                ["log", "--template", tmpl_name, f"Decision using {tmpl_name}", "--no-commit"],
            )

        assert result.exit_code == 0, f"Template '{tmpl_name}' failed: {result.output}"


def test_log_api_template(git_repo):
    """api template pre-fills API-relevant alternatives."""
    runner = CliRunner()

    with runner.isolated_filesystem(temp_dir=git_repo):
        _setup_docs(Path("."))
        result = runner.invoke(
            main, ["log", "--template", "api", "Use REST API", "--no-commit"]
        )
        assert result.exit_code == 0, result.output
        decisions = (Path(".") / "docs" / "decisions.md").read_text()

    assert "REST" in decisions or "GraphQL" in decisions
