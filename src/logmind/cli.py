"""Command-line interface for logmind."""

import sys
from pathlib import Path
from typing import Any, Optional

import click

from logmind.core.config import load_config
from logmind.core.git_handler import commit_and_push, is_git_repo
from logmind.core.inserter import (
    AGENT_REGISTRY,
    create_agent_file,
    get_agent_file_path,
    get_agent_status,
    get_all_agent_names,
    insert_into_all_ai_files,
    insert_logmind_section,
    migrate_to_agents_md,
    remove_agent_file,
    sync_agent_files_from_config,
)
from logmind.core.aggregator import aggregate_projects, project_summary
from logmind.core.analytics import ascii_bar_chart, compute_stats
from logmind.core.decision_templates import get_template, list_templates
from logmind.core.logger import log as log_decision, log_first_decision
from logmind.core.search import format_search_results, search_decisions
from logmind.core.tree_gen import update_file_structure


@click.group()
@click.version_option(version="0.1.0", prog_name="logmind")
def main():
    """logmind - AI decision logging system for development projects."""
    pass


def _install_github_action_templates(root_path: Path) -> list:
    """
    Copy every ``templates/github/*.yml.template`` into
    ``<root>/.github/workflows/<name>.yml``. Existing workflow files with the
    same name are NOT overwritten — logmind treats user-customised workflows
    as canonical.

    Returns a list of relative paths that were newly created.
    """
    template_root = Path(__file__).parent / "templates" / "github"
    if not template_root.exists():
        return []
    workflows_dir = root_path / ".github" / "workflows"
    workflows_dir.mkdir(parents=True, exist_ok=True)
    created = []
    suffix = ".template"
    for tmpl in sorted(template_root.glob("*.yml.template")):
        target_name = tmpl.name[: -len(suffix)]
        target = workflows_dir / target_name
        if target.exists():
            continue
        target.write_text(tmpl.read_text())
        created.append(str(target.relative_to(root_path)))
    return created


@main.command()
@click.option(
    "--no-git",
    is_flag=True,
    help="Skip git operations (don't commit initialization)",
)
@click.option(
    "--agents",
    "agents_list",
    type=str,
    help="Comma-separated list of agents to configure (e.g., claude,cursor,windsurf)",
)
@click.option(
    "--all-agents",
    is_flag=True,
    help="Configure all supported AI agents",
)
@click.option(
    "--github-actions/--no-github-actions",
    default=True,
    help="Install logmind GitHub Actions (decision aggregator, link checker). Default on.",
)
def init(no_git: bool, agents_list: Optional[str], all_agents: bool, github_actions: bool):
    """
    Initialize logmind in the current project.

    Creates docs/ folder, template files, inserts instructions into AI agent files,
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

    # Parse agents list
    agents = None
    if all_agents:
        agents = get_all_agent_names()
    elif agents_list:
        agents = [a.strip() for a in agents_list.split(",") if a.strip()]
        # Validate agent names
        valid_agents = get_all_agent_names()
        for agent in agents:
            if agent not in valid_agents:
                click.secho(f"Warning: Unknown agent '{agent}'. Valid agents: {', '.join(valid_agents)}", fg="yellow")

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

    # Copy README.md to docs/logmind-readme.md if it exists
    readme_path = root_path / "README.md"
    if readme_path.exists():
        logmind_readme_path = docs_path / "logmind-readme.md"
        logmind_readme_path.write_text(readme_path.read_text())
        click.echo("✓ Created docs/logmind-readme.md")

    # Create .logmind directory and config file
    logmind_dir = root_path / ".logmind"
    logmind_dir.mkdir(exist_ok=True)
    config_template = (template_dir / "config.yml.template").read_text()
    (logmind_dir / "config.yml").write_text(config_template)
    click.echo("✓ Created .logmind/config.yml")

    # If no agents specified, use config defaults (claude + cursor enabled by default)
    if agents is None:
        config = load_config(logmind_dir / "config.yml")
        agents = config.get_enabled_agents()

    # Insert into AI instruction files
    messages = insert_into_all_ai_files(root_path, agents=agents)
    for msg in messages:
        click.echo(msg)

    # Install GitHub Action templates (decision aggregator, link checker)
    installed_workflows: list = []
    if github_actions:
        installed_workflows = _install_github_action_templates(root_path)
        for wf in installed_workflows:
            click.echo(f"✓ Created {wf}")

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

            # Add logmind-readme.md if it was created
            logmind_readme_path = docs_path / "logmind-readme.md"
            if logmind_readme_path.exists():
                files_to_commit.append("docs/logmind-readme.md")

            # Add any created AI instruction files
            for agent_name in (agents or ["claude"]):
                file_path = get_agent_file_path(agent_name, root_path)
                if file_path and file_path.exists():
                    # Get relative path
                    rel_path = file_path.relative_to(root_path)
                    files_to_commit.append(str(rel_path))

            # Also check for CLAUDE.md specifically (in case it was auto-created)
            claude_path = root_path / "CLAUDE.md"
            if claude_path.exists() and "CLAUDE.md" not in files_to_commit:
                files_to_commit.append("CLAUDE.md")

            # GH Action workflows installed during this init
            files_to_commit.extend(installed_workflows)

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
@click.option(
    "--template",
    "-T",
    type=str,
    default=None,
    help="Pre-fill from a built-in template (database, api, architecture, security, performance, library, deployment)",
)
def log(
    decision: str,
    reasoning: Optional[str],
    alternative: tuple,
    implication: tuple,
    no_commit: bool,
    no_push: bool,
    template: Optional[str],
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

    # Apply template defaults (explicit CLI flags take precedence)
    if template:
        tmpl = get_template(template)
        if tmpl is None:
            available = ", ".join(list_templates().keys())
            click.secho(
                f"Unknown template '{template}'. Available: {available}", fg="red"
            )
            sys.exit(1)
        if reasoning is None:
            reasoning = tmpl["reasoning"]
        if alternatives is None:
            alternatives = tmpl["alternatives"]
        if implications is None:
            implications = tmpl["implications"]

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

        # Sync agent files from config
        sync_messages = sync_agent_files_from_config()
        for msg in sync_messages:
            click.echo(msg)

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

    # Sync agent files from config
    sync_messages = sync_agent_files_from_config()
    for msg in sync_messages:
        click.echo(msg)

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

    # Sync agent files from config
    sync_messages = sync_agent_files_from_config()
    for msg in sync_messages:
        click.echo(msg)

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


@main.command("aggregate")
@click.argument("projects", nargs=-1, type=click.Path(exists=True, file_okay=False))
@click.option(
    "--limit",
    "-n",
    default=20,
    type=int,
    help="Maximum number of decisions to show (default: 20)",
)
@click.option(
    "--no-archive",
    is_flag=True,
    help="Exclude archived decisions",
)
@click.option(
    "--summary",
    "show_summary",
    is_flag=True,
    help="Show per-project counts instead of a decision feed",
)
def aggregate(projects: tuple, limit: int, no_archive: bool, show_summary: bool):
    """
    View decisions aggregated across multiple projects.

    Pass one or more project directories (paths containing a docs/ folder).

    Examples:
        logmind aggregate ~/projects/api ~/projects/frontend
        logmind aggregate --summary ~/work/*/
        logmind aggregate --limit 50 --no-archive ~/projects/app
    """
    if not projects:
        click.secho("Error: provide at least one project path.", fg="red")
        sys.exit(1)

    project_paths = [Path(p) for p in projects]
    missing = [p for p in project_paths if not (p / "docs").exists()]

    if missing:
        for m in missing:
            click.secho(f"Warning: {m} has no docs/ directory — skipping.", fg="yellow")
        project_paths = [p for p in project_paths if (p / "docs").exists()]

    if not project_paths:
        click.secho("No valid logmind projects found.", fg="red")
        sys.exit(1)

    if show_summary:
        summary = project_summary(project_paths)
        click.secho("Project Summary", bold=True)
        click.echo("─" * 40)
        total = 0
        for name, count in sorted(summary.items(), key=lambda x: -x[1]):
            click.echo(f"  {name:<30} {count} decisions")
            total += count
        click.echo("─" * 40)
        click.secho(f"  {'Total':<30} {total} decisions", bold=True)
        return

    entries = aggregate_projects(
        project_paths,
        include_archive=not no_archive,
        limit=limit,
    )

    if not entries:
        click.secho("No decisions found across the specified projects.", fg="yellow")
        return

    click.secho(
        f"Showing {len(entries)} most recent decisions across "
        f"{len(project_paths)} project{'s' if len(project_paths) != 1 else ''}:",
        bold=True,
    )
    click.echo()

    for entry in entries:
        date_str = entry.date.strftime("%Y-%m-%d")
        click.secho(f"[{entry.project}]", fg="cyan", nl=False)
        click.echo(f"  {date_str}  {entry.title}")


@main.command("stats")
@click.option(
    "--months",
    "-m",
    default=12,
    type=int,
    help="Number of recent months to show in the chart (default: 12)",
)
def stats(months: int):
    """
    Show analytics and statistics for your decision log.

    Displays total decisions, monthly activity chart, velocity trend,
    and top keywords across all logged decisions.
    """
    docs_path = Path.cwd() / "docs"

    if not docs_path.exists():
        click.secho(
            "Error: docs/ directory not found. Run 'logmind init' first.",
            fg="red",
        )
        sys.exit(1)

    data = compute_stats(docs_path)

    if data["total"] == 0:
        click.secho("No decisions logged yet. Run 'logmind log' to get started.", fg="yellow")
        return

    # Header
    click.secho("Decision Log Analytics", bold=True)
    click.echo("─" * 40)

    # Totals
    click.echo(f"Total decisions:  {data['total']}")
    click.echo(f"  Recent:         {data['recent_count']}")
    click.echo(f"  Archived:       {data['archive_count']}")
    click.echo()

    # Velocity
    v30 = data["velocity_30"]
    vp30 = data["velocity_prior_30"]
    trend = ""
    if vp30 > 0:
        pct = int(((v30 - vp30) / vp30) * 100)
        trend = f"  ({'+' if pct >= 0 else ''}{pct}% vs prior 30 days)"
    click.echo(f"Last 30 days:     {v30} decisions{trend}")
    if data["most_active_month"]:
        click.echo(
            f"Most active:      {data['most_active_month']} ({data['most_active_count']} decisions)"
        )
    click.echo()

    # Monthly chart (last N months)
    by_month = data["by_month"]
    sorted_months = sorted(by_month.keys())[-months:]
    chart_data = {m: by_month[m] for m in sorted_months}

    if chart_data:
        click.secho(f"Activity (last {min(months, len(chart_data))} months):", bold=True)
        click.echo(ascii_bar_chart(chart_data))
        click.echo()

    # Keywords
    if data["keywords"]:
        click.secho("Top keywords:", bold=True)
        for word, count in data["keywords"]:
            click.echo(f"  {word:<20} {count}")


@main.command("templates")
def templates_list():
    """
    List available decision templates.

    Templates pre-fill reasoning, alternatives, and implications for common
    decision types. Use with: logmind log --template <name> "Your decision"

    Example:
        logmind log --template database "Use PostgreSQL"
    """
    click.echo("Available decision templates:\n")
    for name, description in list_templates().items():
        click.echo(f"  {name:<14} {description}")
    click.echo(
        "\nUsage: logmind log --template <name> \"Your decision here\""
    )


# Agents command group
@main.group()
def agents():
    """Manage AI agent configuration files."""
    pass


@agents.command("list")
def agents_list():
    """List all supported agents and their status."""
    root_path = Path.cwd()

    # Sync agent files from config
    sync_messages = sync_agent_files_from_config(root_path)
    for msg in sync_messages:
        click.echo(msg)

    status = get_agent_status(root_path)

    click.echo("AI Agent Status:")
    click.echo()

    for agent_name, info in status.items():
        if info["configured"]:
            icon = click.style("✓", fg="green")
            status_text = click.style("configured", fg="green")
        elif info["exists"]:
            icon = click.style("~", fg="yellow")
            status_text = click.style("exists (no logmind)", fg="yellow")
        else:
            icon = click.style("✗", fg="red")
            status_text = click.style("not configured", fg="red")

        click.echo(f"  {icon} {agent_name:12} {info['file']:40} ({status_text})")

    click.echo()
    click.echo(f"Supported agents: {', '.join(get_all_agent_names())}")


@agents.command("add")
@click.argument("agent_name")
@click.option(
    "--no-commit",
    is_flag=True,
    help="Don't commit the new file",
)
def agents_add(agent_name: str, no_commit: bool):
    """Add an AI agent configuration file."""
    root_path = Path.cwd()

    # Validate agent name
    if agent_name not in AGENT_REGISTRY:
        click.secho(
            f"Error: Unknown agent '{agent_name}'. Valid agents: {', '.join(get_all_agent_names())}",
            fg="red",
        )
        sys.exit(1)

    file_path = get_agent_file_path(agent_name, root_path)

    if file_path and file_path.exists():
        # File exists - try to insert logmind section
        from logmind.core.inserter import is_agent_json

        if is_agent_json(agent_name):
            click.secho(f"✓ {file_path.name} already exists (JSON format)", fg="yellow")
        else:
            inserted = insert_logmind_section(file_path)
            if inserted:
                click.secho(f"✓ Added logmind instructions to {file_path.name}", fg="green")
                # Commit if requested
                if not no_commit and is_git_repo():
                    try:
                        rel_path = file_path.relative_to(root_path)
                        commit_and_push(
                            [str(rel_path)],
                            f"logmind: Add instructions to {agent_name} agent file",
                            push=True,
                        )
                        click.echo("✓ Committed changes")
                    except Exception as e:
                        click.secho(f"Warning: Failed to commit: {e}", fg="yellow")
            else:
                click.secho(f"✓ {file_path.name} already has logmind instructions", fg="yellow")
    else:
        # Create new file
        created = create_agent_file(agent_name, root_path)
        if created:
            click.secho(f"✓ Created {created.name} with logmind instructions", fg="green")

            # Commit if requested
            if not no_commit and is_git_repo():
                try:
                    rel_path = created.relative_to(root_path)
                    commit_and_push(
                        [str(rel_path)],
                        f"logmind: Add {agent_name} agent configuration",
                        push=True,
                    )
                    click.echo("✓ Committed changes")
                except Exception as e:
                    click.secho(f"Warning: Failed to commit: {e}", fg="yellow")
        else:
            click.secho(f"Error: Failed to create file for agent '{agent_name}'", fg="red")
            sys.exit(1)


@agents.command("remove")
@click.argument("agent_name")
@click.option(
    "--no-commit",
    is_flag=True,
    help="Don't commit the removal",
)
@click.option(
    "--force",
    "-f",
    is_flag=True,
    help="Remove without confirmation",
)
def agents_remove(agent_name: str, no_commit: bool, force: bool):
    """Remove an AI agent configuration file."""
    root_path = Path.cwd()

    # Validate agent name
    if agent_name not in AGENT_REGISTRY:
        click.secho(
            f"Error: Unknown agent '{agent_name}'. Valid agents: {', '.join(get_all_agent_names())}",
            fg="red",
        )
        sys.exit(1)

    file_path = get_agent_file_path(agent_name, root_path)

    if not file_path or not file_path.exists():
        click.secho(f"Agent '{agent_name}' is not configured (file does not exist)", fg="yellow")
        return

    # Confirm removal
    if not force:
        if not click.confirm(f"Remove {file_path.name}?"):
            click.echo("Cancelled.")
            return

    # Remove the file
    removed = remove_agent_file(agent_name, root_path)

    if removed:
        click.secho(f"✓ Removed {file_path.name}", fg="green")

        # Commit if requested
        if not no_commit and is_git_repo():
            try:
                rel_path = file_path.relative_to(root_path)
                commit_and_push(
                    [str(rel_path)],
                    f"logmind: Remove {agent_name} agent configuration",
                    push=True,
                )
                click.echo("✓ Committed changes")
            except Exception as e:
                click.secho(f"Warning: Failed to commit: {e}", fg="yellow")
    else:
        click.secho(f"Error: Failed to remove {file_path.name}", fg="red")
        sys.exit(1)


@agents.command("migrate")
@click.option(
    "--no-commit",
    is_flag=True,
    help="Don't commit the migration changes",
)
def agents_migrate(no_commit: bool):
    """
    Consolidate per-agent files into AGENTS.md and replace each with a stub.

    For each existing markdown agent file (CLAUDE.md, .cursorrules, etc.):
      - Strip the logmind marker block.
      - Append any remaining user content under "## From <name>" in AGENTS.md.
      - Replace the file with a 2-line stub pointing at AGENTS.md.

    Idempotent — re-running on an already-migrated tree is a no-op.
    """
    root_path = Path.cwd()

    messages = migrate_to_agents_md(root_path)
    if not messages:
        click.secho("No agent files to migrate (already consolidated).", fg="yellow")
        return

    for msg in messages:
        click.echo(msg)

    if not no_commit and is_git_repo():
        try:
            commit_and_push(
                ["AGENTS.md"]
                + [
                    pattern
                    for _, (pattern, _, json_) in AGENT_REGISTRY.items()
                    if not json_ and (root_path / pattern).exists()
                ],
                "logmind: Consolidate AI agent files into AGENTS.md",
                push=True,
            )
            click.echo("✓ Committed migration")
        except Exception as e:
            click.secho(f"Warning: Failed to commit: {e}", fg="yellow")


# Config command group
@main.group()
def config():
    """View and modify logmind configuration."""
    pass


@config.command("list")
def config_list():
    """Show all configuration settings."""
    import yaml

    cfg = load_config()
    click.echo(yaml.dump(cfg._config, default_flow_style=False, sort_keys=False))


@config.command("get")
@click.argument("key")
def config_get(key: str):
    """
    Get a configuration value by key (dot notation).

    Examples:
        logmind config get git.auto_push
        logmind config get decisions.max_recent
    """
    cfg = load_config()
    value = cfg.get(key)
    if value is None:
        click.secho(f"Key '{key}' not found", fg="red", err=True)
        sys.exit(1)
    click.echo(value)


@config.command("set")
@click.argument("key")
@click.argument("value")
def config_set(key: str, value: str):
    """
    Set a configuration value.

    Values are auto-converted: "true"/"false" -> bool, digits -> int.

    Examples:
        logmind config set git.auto_push false
        logmind config set decisions.max_recent 30
    """
    cfg = load_config()

    # Parse value type
    parsed_value: Any
    if value.lower() == "true":
        parsed_value = True
    elif value.lower() == "false":
        parsed_value = False
    elif value.isdigit():
        parsed_value = int(value)
    elif value.replace(".", "", 1).isdigit() and value.count(".") == 1:
        parsed_value = float(value)
    else:
        parsed_value = value

    cfg.set(key, parsed_value)
    click.secho(f"Set {key} = {parsed_value}", fg="green")


@main.command("check-decisions")
@click.option(
    "--threshold",
    "-t",
    default=20,
    type=int,
    help="Minimum lines changed to require a decision log entry (default: 20)",
)
@click.option(
    "--no-fail",
    is_flag=True,
    help="Warn but exit with code 0 (don't block the commit)",
)
def check_decisions(threshold: int, no_fail: bool):
    """
    Check that significant code changes have corresponding decision logs.

    Designed for use as a git pre-commit hook. Exits with code 1 if staged
    changes exceed the line threshold without an update to docs/decisions.md.

    To install as a pre-commit hook, run: logmind install-hook

    Examples:
        logmind check-decisions
        logmind check-decisions --threshold 50
        logmind check-decisions --no-fail
    """
    import subprocess

    if not is_git_repo():
        click.echo("Not a git repository, skipping check.")
        return

    # Get list of staged files
    result = subprocess.run(
        ["git", "diff", "--cached", "--name-only"],
        capture_output=True,
        text=True,
    )
    staged_files = result.stdout.strip().split("\n") if result.stdout.strip() else []

    # If decisions.md is staged, changes are documented
    if any("decisions.md" in f for f in staged_files):
        click.secho("✓ docs/decisions.md is staged — changes are documented.", fg="green")
        return

    # Count lines changed outside of docs/
    numstat = subprocess.run(
        ["git", "diff", "--cached", "--numstat"],
        capture_output=True,
        text=True,
    )

    total_lines = 0
    for line in numstat.stdout.strip().split("\n"):
        if not line:
            continue
        parts = line.split("\t")
        if len(parts) != 3:
            continue
        added, removed, filepath = parts
        # Skip docs/ files and binary files (shown as "-")
        if filepath.startswith("docs/") or added == "-":
            continue
        try:
            total_lines += int(added) + int(removed)
        except ValueError:
            pass

    if total_lines >= threshold:
        click.secho(
            f"⚠  {total_lines} lines changed without updating docs/decisions.md.\n"
            f"   Log this decision: logmind log \"Your decision here\"\n"
            f"   To skip this check: git commit --no-verify",
            fg="yellow",
        )
        if not no_fail:
            sys.exit(1)
    else:
        click.secho(
            f"✓ {total_lines} lines changed (below {threshold}-line threshold).",
            fg="green",
        )


@main.command("check-links")
def check_links():
    """
    Verify all relative markdown links resolve and no docs/*.md is orphaned.

    Walks README.md, AGENTS.md, CLAUDE.md, and the entire docs/ tree by
    default. Configure roots and orphan allowlist via .logmind/config.yml:

        linkcheck:
          roots: [README.md, docs]
          allow_orphans: [docs/legacy.md]

    Exits 0 on a clean run, 1 if any broken or orphan links are found.
    """
    from logmind.actions.link_check import main as _link_check_main

    sys.exit(_link_check_main())


@main.command("install-hook")
@click.option(
    "--force",
    is_flag=True,
    help="Add logmind to an existing pre-commit hook without prompting",
)
def install_hook(force: bool):
    """
    Install logmind check-decisions as a git pre-commit hook.

    Creates or appends to .git/hooks/pre-commit so that every commit
    is checked for undocumented decisions.
    """
    import subprocess

    if not is_git_repo():
        click.secho("Error: not a git repository.", fg="red")
        sys.exit(1)

    # Use git to find the actual repository root (handles subdirectory invocation)
    root_result = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        capture_output=True,
        text=True,
    )
    root_path = Path(root_result.stdout.strip())
    hook_path = root_path / ".git" / "hooks" / "pre-commit"
    hook_line = "logmind check-decisions\n"

    if hook_path.exists():
        content = hook_path.read_text()
        if "logmind check-decisions" in content:
            click.echo("✓ logmind hook already installed.")
            return
        if not force:
            click.secho(
                "A pre-commit hook already exists. Use --force to append logmind to it.",
                fg="yellow",
            )
            sys.exit(1)
        hook_path.write_text(content.rstrip("\n") + "\n" + hook_line)
        click.secho(
            "✓ Added logmind check-decisions to existing pre-commit hook.", fg="green"
        )
    else:
        hook_path.parent.mkdir(parents=True, exist_ok=True)
        hook_path.write_text("#!/bin/sh\n" + hook_line)
        hook_path.chmod(0o755)
        click.secho("✓ Installed logmind pre-commit hook.", fg="green")


@main.command()
def update():
    """
    Update logmind to the latest version.

    Runs 'pip install --upgrade logmind' and shows version changes.
    """
    import subprocess

    # Get current version
    try:
        from importlib.metadata import version as get_version
        current_version = get_version("logmind")
    except Exception:
        current_version = "unknown"

    click.echo(f"Current version: {current_version}")
    click.echo("Checking for updates...")

    try:
        result = subprocess.run(
            [sys.executable, "-m", "pip", "install", "--upgrade", "logmind"],
            capture_output=True,
            text=True,
        )

        if result.returncode != 0:
            click.secho(f"Error updating: {result.stderr}", fg="red")
            sys.exit(1)

        # Get new version
        try:
            # Force reimport to get new version
            import importlib
            import logmind
            importlib.reload(logmind)
            from importlib.metadata import version as get_version
            new_version = get_version("logmind")
        except Exception:
            new_version = "unknown"

        if current_version == new_version:
            click.secho(f"✓ Already at latest version ({current_version})", fg="green")
        else:
            click.secho(f"✓ Updated: {current_version} → {new_version}", fg="green")

    except Exception as e:
        click.secho(f"Error: {e}", fg="red")
        sys.exit(1)


if __name__ == "__main__":
    main()
