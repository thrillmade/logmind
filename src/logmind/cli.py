"""Command-line interface for logmind."""

import sys
from pathlib import Path
from typing import Optional

import click

from logmind.core.config import load_config
from logmind.core.git_handler import commit_and_push, is_git_repo
from logmind.core.inserter import insert_into_all_ai_files
from logmind.core.logger import log as log_decision, log_first_decision
from logmind.core.search import format_search_results, search_decisions
from logmind.core.tree_gen import update_file_structure


@click.group()
@click.version_option(version="0.1.0", prog_name="logmind")
def main():
    """logmind - AI decision logging system for development projects."""
    pass


@main.command()
@click.option(
    "--no-git",
    is_flag=True,
    help="Skip git operations (don't commit initialization)",
)
def init(no_git: bool):
    """
    Initialize logmind in the current project.

    Creates docs/ folder, template files, inserts instructions into CLAUDE.md,
    logs first decision, and commits changes.
    """
    root_path = Path.cwd()
    docs_path = root_path / "docs"

    click.echo("Initializing logmind...")
    click.echo()

    # Check if already initialized
    if docs_path.exists() and (docs_path / "decisions.md").exists():
        click.echo("✓ docs/ already exists")
        click.echo("✓ decisions.md already exists")
        click.echo()
        click.secho("logmind is already initialized in this project.", fg="yellow")
        return

    # Check if we're in a git repo
    if not no_git and not is_git_repo():
        click.secho(
            "Warning: Not a git repository. Initialize git first with 'git init' "
            "or use --no-git flag.",
            fg="yellow",
        )
        if not click.confirm("Continue without git?"):
            sys.exit(1)
        no_git = True

    # Create docs directory
    docs_path.mkdir(parents=True, exist_ok=True)
    click.echo("✓ Created docs/")

    # Create template files
    template_dir = Path(__file__).parent / "templates"

    # decisions.md
    decisions_template = (template_dir / "decisions.md.template").read_text()
    (docs_path / "decisions.md").write_text(decisions_template)
    click.echo("✓ Created docs/decisions.md")

    # decisions-archive.md
    archive_template = (template_dir / "decisions-archive.md.template").read_text()
    (docs_path / "decisions-archive.md").write_text(archive_template)
    click.echo("✓ Created docs/decisions-archive.md")

    # file-structure.md (will be generated with actual tree)
    update_file_structure(docs_path)
    click.echo("✓ Created docs/file-structure.md")

    # Create .logmind directory and config file
    logmind_dir = root_path / ".logmind"
    logmind_dir.mkdir(exist_ok=True)
    config_template = (template_dir / "config.yml.template").read_text()
    (logmind_dir / "config.yml").write_text(config_template)
    click.echo("✓ Created .logmind/config.yml")

    # Insert into AI instruction files
    messages = insert_into_all_ai_files(root_path)
    for msg in messages:
        click.echo(msg)

    # Log first decision
    log_first_decision(docs_path)
    click.echo("✓ Logged first decision: \"Initialize logmind decision tracking\"")

    # Commit everything
    if not no_git:
        try:
            files_to_commit = [
                "docs/decisions.md",
                "docs/decisions-archive.md",
                "docs/file-structure.md",
                ".logmind/config.yml",
            ]

            # Add CLAUDE.md if it was created/modified
            claude_path = root_path / "CLAUDE.md"
            if claude_path.exists():
                files_to_commit.append("CLAUDE.md")

            commit_and_push(
                files_to_commit,
                "logmind: Initialize decision tracking",
                push=True,
            )
            click.echo("✓ Committed changes: \"logmind: Initialize decision tracking\"")
        except Exception as e:
            click.secho(f"Warning: Failed to commit: {e}", fg="yellow")

    click.echo()
    click.secho("logmind initialized successfully!", fg="green")
    click.echo()
    click.echo("Start logging decisions with:")
    click.echo("  from logmind import log")
    click.echo("  log(\"Your decision here\")")


@main.command()
@click.argument("decision")
@click.option("--reasoning", "-r", help="Why this decision was made")
@click.option(
    "--alternative",
    "-a",
    multiple=True,
    help="Alternative option considered (can be used multiple times)",
)
@click.option(
    "--implication",
    "-i",
    multiple=True,
    help="Implication of this decision (can be used multiple times)",
)
@click.option(
    "--no-commit",
    is_flag=True,
    help="Don't auto-commit the decision",
)
@click.option(
    "--no-push",
    is_flag=True,
    help="Don't auto-push after committing",
)
def log(
    decision: str,
    reasoning: Optional[str],
    alternative: tuple,
    implication: tuple,
    no_commit: bool,
    no_push: bool,
):
    """
    Log a decision to the decision log.

    Example:
        logmind log "Use PostgreSQL for database" \\
            -r "Need ACID compliance" \\
            -a "MongoDB" -a "SQLite" \\
            -i "Need to set up connection pooling"
    """
    docs_path = Path.cwd() / "docs"

    if not docs_path.exists():
        click.secho(
            "Error: docs/ directory not found. Run 'logmind init' first.",
            fg="red",
        )
        sys.exit(1)

    alternatives = list(alternative) if alternative else None
    implications = list(implication) if implication else None

    # Load config to determine defaults
    config = load_config()

    # Determine commit and push behavior
    # CLI flags override config
    should_commit = config.auto_commit if not no_commit else False
    should_push = config.auto_push if not no_push else False

    try:
        log_decision(
            decision=decision,
            reasoning=reasoning,
            alternatives=alternatives,
            implications=implications,
            docs_path=docs_path,
            auto_commit=should_commit,
            auto_push=should_push,
        )

        click.secho(f"✓ Logged decision: \"{decision}\"", fg="green")

        if should_commit:
            if should_push:
                click.echo("✓ Committed and pushed changes")
            else:
                click.echo("✓ Committed changes (push disabled)")

    except Exception as e:
        click.secho(f"Error: {e}", fg="red")
        sys.exit(1)


@main.command()
@click.option(
    "--all",
    "-a",
    "show_all",
    is_flag=True,
    help="Show all decisions including archived",
)
def show(show_all: bool):
    """Show recent decisions."""
    docs_path = Path.cwd() / "docs"

    if not docs_path.exists():
        click.secho(
            "Error: docs/ directory not found. Run 'logmind init' first.",
            fg="red",
        )
        sys.exit(1)

    decisions_path = docs_path / "decisions.md"

    if not decisions_path.exists():
        click.secho("No decisions logged yet.", fg="yellow")
        return

    click.echo(decisions_path.read_text())

    if show_all:
        archive_path = docs_path / "decisions-archive.md"
        if archive_path.exists():
            click.echo("\n" + "=" * 80)
            click.echo("ARCHIVED DECISIONS")
            click.echo("=" * 80 + "\n")
            click.echo(archive_path.read_text())


@main.command()
@click.argument("query")
@click.option(
    "--case-sensitive",
    "-c",
    is_flag=True,
    help="Perform case-sensitive search",
)
@click.option(
    "--no-archive",
    is_flag=True,
    help="Don't search archived decisions",
)
@click.option(
    "--no-context",
    is_flag=True,
    help="Don't show context lines around matches",
)
@click.option(
    "--context-lines",
    "-C",
    type=int,
    default=2,
    help="Number of context lines to show (default: 2)",
)
def search(
    query: str,
    case_sensitive: bool,
    no_archive: bool,
    no_context: bool,
    context_lines: int,
):
    """
    Search through decision logs for a term or pattern.

    Supports regex patterns. By default, search is case-insensitive
    and includes archived decisions.

    Examples:
        logmind search "postgres"
        logmind search "database.*choice" -c
        logmind search "API" --no-archive
    """
    docs_path = Path.cwd() / "docs"

    if not docs_path.exists():
        click.secho(
            "Error: docs/ directory not found. Run 'logmind init' first.",
            fg="red",
        )
        sys.exit(1)

    try:
        results = search_decisions(
            query=query,
            docs_path=docs_path,
            case_sensitive=case_sensitive,
            include_archive=not no_archive,
            context_lines=context_lines,
        )

        if not results:
            click.secho(f"No matches found for: {query}", fg="yellow")
            return

        # Show result count
        click.secho(
            f"Found {len(results)} match{'es' if len(results) != 1 else ''} for: {query}",
            fg="green",
        )
        click.echo()

        # Format and display results
        formatted = format_search_results(
            results,
            show_context=not no_context,
            highlight_term=query if not case_sensitive else None,
        )
        click.echo(formatted)

    except Exception as e:
        click.secho(f"Error during search: {e}", fg="red")
        sys.exit(1)


if __name__ == "__main__":
    main()
