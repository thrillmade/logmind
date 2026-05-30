"""Command-line interface for logmind."""

import re
import subprocess
import sys
from pathlib import Path
from typing import Any, Optional, Tuple

import click

from logmind.core.config import load_config
from logmind.core.git_handler import commit_and_push, is_git_repo
from logmind.core.gitattributes import (
    configure_merge_drivers,
    ensure_block as ensure_gitattributes_block,
    install_post_merge_hook,
    install_post_rewrite_hook,
)
from logmind.core.gitignore import ensure_block as ensure_gitignore_block
from logmind.core.skill_install import (
    DEFAULT_SKILL_NAME,
    DEFAULT_SKILL_SOURCE,
    install_globally as install_logmind_skill,
    is_skills_available,
)
from logmind.core.inserter import (
    AGENT_REGISTRY,
    _replace_marker_block,
    create_agent_file,
    find_outdated_marker_blocks,
    find_outdated_workflow_pins,
    get_agent_file_path,
    get_agent_status,
    get_all_agent_names,
    insert_into_all_ai_files,
    insert_logmind_section,
    migrate_to_agents_md,
    update_workflow_pin,
    remove_agent_file,
    sync_agent_files_from_config,
)
from logmind.core.aggregator import aggregate_projects, project_summary
from logmind.core.analytics import ascii_bar_chart, compute_stats
from logmind.core.decision_templates import get_template, list_templates
from logmind.core.logger import log as log_decision, log_first_decision
from logmind.core.search import format_search_results, search_decisions
from logmind.core.tree_gen import update_file_structure


import os

# Quiet-mode mechanism (v0.5.1+, extended in v0.5.3 to also patch secho).
#
# Strategy: monkey-patch BOTH click.echo and click.secho at module load
# so progress chatter is suppressed without touching 250+ call sites.
# secho needs special handling — it carries semantic color: red/yellow
# mean error/warning and MUST still print when LOGMIND_QUIET=1, while
# green/cyan/blue/default are progress chatter and get suppressed.
# ok() uses the UNPATCHED echo so its single-line summary always emits.
#
# Activation: LOGMIND_QUIET=1 env var OR --quiet/-q on the group.
_QUIET = os.environ.get("LOGMIND_QUIET") == "1"
_orig_click_echo = click.echo
_orig_click_secho = click.secho

# Colors that always print regardless of quiet mode — these carry
# error/warning semantics agents need to see. Everything else (green
# success, cyan info, default/none) is progress chatter and suppressed.
_LOUD_COLORS = frozenset({"red", "yellow", "bright_red", "bright_yellow"})


def _quiet_aware_echo(*args, **kwargs):
    """Drop-in for click.echo that no-ops when _QUIET."""
    if _QUIET:
        return
    return _orig_click_echo(*args, **kwargs)


def _quiet_aware_secho(*args, **kwargs):
    """Drop-in for click.secho. Suppresses when _QUIET unless fg is one
    of the loud colors (red/yellow) — those carry error/warning
    semantics and must still print in quiet mode."""
    if _QUIET and kwargs.get("fg") not in _LOUD_COLORS:
        return
    return _orig_click_secho(*args, **kwargs)


# Install the patches immediately so subcommand functions captured at
# import time pick them up. _QUIET is re-read on every call so the
# group-level --quiet flag can flip it mid-run.
click.echo = _quiet_aware_echo
click.secho = _quiet_aware_secho


def _set_quiet(flag: bool) -> None:
    global _QUIET
    _QUIET = bool(flag)


def _ok(msg: str, *, err: bool = False) -> None:
    """Single-line success summary — ALWAYS emits, even with --quiet.
    Format: `ok <key-value>` so agents can parse it for chaining
    (commit SHA / file count / depth).

    Pass `err=True` to route the line to stderr (used when stdout is
    reserved for parseable primary output, e.g. `show --json`). Agents
    still see the ok line; pipeline consumers (jq, etc.) get a clean
    stdout."""
    _orig_click_echo(f"ok {msg}", err=err)


@click.group()
@click.version_option(version="0.6.0", prog_name="logmind")
@click.option(
    "--quiet",
    "-q",
    "quiet_flag",
    is_flag=True,
    help="Token-frugal mode for agent invocations. Suppresses progress "
    "chatter; emits exactly one `ok <key-value>` summary line per command. "
    "Errors and warnings still print. Also honored via LOGMIND_QUIET=1 env var.",
)
def main(quiet_flag: bool):
    """logmind - AI decision logging system for development projects."""
    # Re-evaluate quietness on every invocation so test runs (and any
    # other long-lived processes that call main() multiple times via
    # CliRunner) don't leak stale state from a prior call. The flag
    # OR the env var enables quiet; absence of both disables it.
    _set_quiet(quiet_flag or os.environ.get("LOGMIND_QUIET") == "1")


_AGENT_FILE_CANDIDATES = (
    "AGENTS.md",
    "CLAUDE.md",
    ".cursorrules",
    ".windsurfrules",
    ".continuerules",
    ".clinerules",
    ".aiderrules",
    "CONVENTIONS.md",
    ".github/copilot-instructions.md",
    ".amazonq/rules.md",
    ".sourcegraph/cody.json",
    ".zed/settings.json",
)


def _changed_agent_files(root_path: Optional[Path] = None) -> list:
    """Return agent files that are currently modified or untracked.

    Used by `logmind log` (v0.1.3+) so the agent-file sync that ran just
    before the commit can include refreshed files in the scoped staging
    instead of leaving the working tree dirty after the commit.
    """
    if root_path is None:
        root_path = Path.cwd()
    changed: list = []
    for rel in _AGENT_FILE_CANDIDATES:
        path = root_path / rel
        if not path.exists():
            continue
        try:
            result = subprocess.run(
                ["git", "status", "--porcelain", "--", rel],
                cwd=root_path,
                capture_output=True,
                text=True,
                check=True,
            )
        except (subprocess.CalledProcessError, FileNotFoundError):
            continue
        if result.stdout.strip():
            changed.append(rel)
    return changed


def _render_workflow_template(template_text: str) -> str:
    """
    Substitute install-time placeholders in a workflow template.

    Currently:
      __LOGMIND_VERSION__ → the logmind version that ran `init`
          (pins downstream CI's `pip install logmind==...` to a known-good
          release rather than tracking whatever is latest on PyPI).

    Uses str.replace, not str.format, so YAML's literal `${{ ... }}`
    expressions don't conflict with Python format syntax.
    """
    from logmind import __version__ as logmind_version

    return template_text.replace("__LOGMIND_VERSION__", logmind_version)


_TEMPLATE_VERSION_RE = re.compile(
    r"^#\s*logmind-template-version:\s*(\S+)\s*$", re.MULTILINE
)


def _extract_template_version(text: str) -> Optional[str]:
    """Return the `# logmind-template-version: vN` value from a workflow
    template (or an installed copy), or None if missing.
    """
    m = _TEMPLATE_VERSION_RE.search(text)
    return m.group(1) if m else None


# Matches the install pin line in a rendered workflow, e.g.
#   `pip install "logmind==0.2.1"`  or  `pip install 'logmind==0.2.1'`
# Capture groups: 1 = prefix up through `==`, 2 = version string, 3 = trailing quote.
_LOGMIND_PIN_RE = re.compile(r'(pip install\s+["\']?logmind==)([\d][\w.+\-]*)(["\']?)')


def _maybe_refresh_pin(installed_text: str) -> Tuple[str, Optional[str]]:
    """Surgically update the `pip install "logmind==X.Y.Z"` line if X.Y.Z
    doesn't match the running logmind's ``__version__``.

    Returns ``(new_text, previous_version_or_None)``. ``previous_version`` is
    the old pin string when a rewrite happened, or ``None`` when nothing
    changed (no pin line, or pin already matched). Touches one line —
    preserves any other body customizations the user kept.
    """
    from logmind import __version__ as logmind_version

    m = _LOGMIND_PIN_RE.search(installed_text)
    if m is None:
        return installed_text, None  # no pin line (dogfood / unpinned)
    found_version = m.group(2)
    if found_version == logmind_version:
        return installed_text, None  # already current
    new_text = _LOGMIND_PIN_RE.sub(
        lambda mo: f"{mo.group(1)}{logmind_version}{mo.group(3)}",
        installed_text,
        count=1,
    )
    return new_text, found_version


def _install_github_action_templates(
    root_path: Path,
    refresh_stale: bool = False,
) -> Tuple[list, list]:
    """
    Copy every ``templates/github/*.yml.template`` into
    ``<root>/.github/workflows/<name>.yml``.

    Two modes:
      - First-install (``refresh_stale=False``): existing workflow files
        with the same name are NOT overwritten — logmind treats user-
        customised workflows as canonical.
      - Refresh (``refresh_stale=True``): if the installed file's
        ``# logmind-template-version:`` marker is older than the
        template's, overwrite it. User-customised workflows that have
        had their version marker stripped are still left alone.
        Additionally (v0.2.5+), the `pip install "logmind==X.Y.Z"` pin
        is surgically refreshed to the current ``__version__`` even when
        the template marker hasn't moved — pin drift is independent of
        body drift, and the previous behaviour left stale pins in place
        across versions like 0.2.1 → 0.2.4 that didn't touch templates.

    Returns ``(created, refreshed)`` — two lists of relative paths.
    """
    template_root = Path(__file__).parent / "templates" / "github"
    if not template_root.exists():
        return [], []
    workflows_dir = root_path / ".github" / "workflows"
    workflows_dir.mkdir(parents=True, exist_ok=True)
    created: list = []
    refreshed: list = []
    suffix = ".template"
    for tmpl in sorted(template_root.glob("*.yml.template")):
        target_name = tmpl.name[: -len(suffix)]
        target = workflows_dir / target_name
        rendered = _render_workflow_template(tmpl.read_text(encoding="utf-8"))
        if not target.exists():
            # Explicit utf-8 — templates use unicode (→, em-dash, etc.).
            target.write_text(rendered, encoding="utf-8")
            created.append(str(target.relative_to(root_path)))
            continue
        if not refresh_stale:
            continue
        installed = target.read_text(encoding="utf-8")
        installed_version = _extract_template_version(installed)
        template_version = _extract_template_version(rendered)
        # Body refresh: only if BOTH have a marker AND they differ. A missing
        # installed marker means the user stripped it — treat as customised
        # and leave alone. A missing template marker means a logmind bug;
        # skip silently rather than break user installs.
        if (
            installed_version is not None
            and template_version is not None
            and installed_version != template_version
        ):
            target.write_text(rendered, encoding="utf-8")
            refreshed.append(str(target.relative_to(root_path)))
            continue
        # Body is current (or markerless = user-canonical). Still check the
        # pin line — it can drift independently of body content across
        # logmind releases that don't touch the template.
        if installed_version is not None:
            new_text, prev = _maybe_refresh_pin(installed)
            if prev is not None:
                target.write_text(new_text, encoding="utf-8")
                refreshed.append(str(target.relative_to(root_path)))
    return created, refreshed


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
@click.option(
    "--skill-install/--no-skill-install",
    "skill_install_flag",
    default=None,
    help=(
        "Install the logmind agent skill globally via skills.sh. "
        "Default: prompt interactively when stdin is a TTY."
    ),
)
@click.option(
    "--install-hook",
    is_flag=True,
    help=(
        "Also run `logmind install-hook` to set up a local pre-commit hook "
        "that fails commits with >threshold lines changed without a decision "
        "log update. Note: hooks live in .git/hooks/ and are NOT committed, "
        "so each contributor must run this once on their machine."
    ),
)
def init(
    no_git: bool,
    agents_list: Optional[str],
    all_agents: bool,
    github_actions: bool,
    skill_install_flag: Optional[bool],
    install_hook: bool,
):
    """
    Initialize logmind in the current project.

    Creates docs/ folder, template files, inserts instructions into AI agent files,
    logs first decision, and commits changes.
    """
    root_path = Path.cwd()
    docs_path = root_path / "docs"

    click.echo("Initializing logmind...")
    click.echo()

    # If logmind is already initialized in this repo, run in refresh mode:
    # leave docs/ and agent files alone, but refresh workflow templates if
    # their `# logmind-template-version:` marker is stale. Lets users run
    # `logmind init` after a logmind upgrade to pick up new CI workflows
    # without the mv docs /tmp + init + mv docs back dance.
    already_initialized = (
        docs_path.exists()
        and (docs_path / "decisions.md").exists()
        and (root_path / ".logmind" / "config.yml").exists()
    )
    if already_initialized:
        # v0.2.10: detect the prior pinned logmind version BEFORE refresh
        # rewrites the workflow files. The pin lives in regen-timeline.yml's
        # `pip install "logmind==X.Y.Z"` line. We use it to decide whether
        # there's an upgrade-relevant CHANGELOG to print after the refresh.
        from logmind import __version__ as current_version
        from logmind.core.doctor import _logmind_installed_version

        prior_version = _logmind_installed_version(root_path)

        click.secho(
            "logmind is already initialized — running in refresh mode.",
            fg="yellow",
        )
        click.echo()
        if github_actions:
            created, refreshed = _install_github_action_templates(
                root_path, refresh_stale=True
            )
            for wf in created:
                click.echo(f"✓ Created {wf}")
            for wf in refreshed:
                click.echo(f"↻ Refreshed {wf} to current template")
            if not created and not refreshed:
                click.echo(
                    "  All workflow templates already current."
                )
        # Refresh agent files (AGENTS.md marker block, etc.) — this is
        # idempotent on the version markers in inserter.py and only
        # rewrites the marker block when the template version changed.
        sync_messages = sync_agent_files_from_config(root_path)
        for msg in sync_messages:
            click.echo(msg)

        # v0.3.0: ensure the .gitattributes block + git config merge
        # drivers are present. Both are idempotent — `.gitattributes`
        # already in place is a no-op, and `git config` no-ops when the
        # value already matches. Setting them every refresh covers the
        # common case where someone freshly cloned a repo that already
        # has .gitattributes committed but no .git/config driver entries.
        gitattributes_path = root_path / ".gitattributes"
        gitattributes_changed = ensure_gitattributes_block(gitattributes_path)
        if gitattributes_changed:
            click.echo("✓ Added logmind block to .gitattributes")
        configure_merge_drivers(root_path)
        install_post_merge_hook(root_path)
        install_post_rewrite_hook(root_path)

        click.echo()
        click.secho("Done. docs/ and .logmind/ left untouched.", fg="green")

        # v0.2.10: print the CHANGELOG sections between prior pinned and
        # current installed version. Closes the propagation gap where
        # agents' session memory keeps using pre-upgrade patterns even
        # though `logmind init` refreshed the repo's instructions on disk.
        if prior_version and prior_version != current_version:
            from logmind.core.changelog import render_upgrade_prompt

            prompt = render_upgrade_prompt(
                prior_version=prior_version,
                current_version=current_version,
            )
            if prompt:
                click.echo(prompt)
        return

    # Surface the skill recommendation prominently BEFORE we write any files,
    # so the user sees it in a single uninterrupted prompt rather than buried
    # after pages of "✓ Created ..." output.
    _show_skill_recommendation(skill_install_flag)

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
    decisions_template = (template_dir / "decisions.md.template").read_text(encoding="utf-8")
    (docs_path / "decisions.md").write_text(decisions_template, encoding="utf-8")
    click.echo("✓ Created docs/decisions.md")

    # decisions-archive.md
    archive_template = (template_dir / "decisions-archive.md.template").read_text(encoding="utf-8")
    (docs_path / "decisions-archive.md").write_text(archive_template, encoding="utf-8")
    click.echo("✓ Created docs/decisions-archive.md")

    # file-structure.md (will be generated with actual tree)
    update_file_structure(docs_path)
    click.echo("✓ Created docs/file-structure.md")

    # timeline.md — v0.2+ derived artifact. AGENTS.md links to it, so seed
    # it now to avoid a broken link on the freshly-initialized repo's first
    # CI run. Subsequent PRs will regenerate it via regen-timeline.yml.
    from logmind.core.timeline import write_timeline

    write_timeline(docs_path / "timeline.md", docs_path)
    click.echo("✓ Created docs/timeline.md")

    # (logmind-readme.md was a copy of README.md kept under docs/ for legacy
    # CLAUDE.md links; AGENTS.md now links to README.md at the root directly,
    # so the copy is redundant and no longer created during init.)

    # Create .logmind directory and config file
    logmind_dir = root_path / ".logmind"
    logmind_dir.mkdir(exist_ok=True)
    config_template = (template_dir / "config.yml.template").read_text(encoding="utf-8")
    (logmind_dir / "config.yml").write_text(config_template, encoding="utf-8")
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
        installed_workflows, _ = _install_github_action_templates(root_path)
        for wf in installed_workflows:
            click.echo(f"✓ Created {wf}")

    # Ensure logmind block in .gitignore (idempotent; preserves user content)
    gitignore_path = root_path / ".gitignore"
    gitignore_changed = ensure_gitignore_block(gitignore_path)
    if gitignore_changed:
        click.echo("✓ Added logmind block to .gitignore")

    # v0.3.0: register the custom merge driver for derived files
    # (docs/timeline.md + docs/file-structure.md). Two parts:
    #   1) .gitattributes block (committed) telling git which files
    #      use the driver
    #   2) git config (per-clone) defining what the driver does
    # Without (2), git refuses to invoke the driver — security guard
    # against untrusted repos running arbitrary commands.
    gitattributes_path = root_path / ".gitattributes"
    gitattributes_changed = ensure_gitattributes_block(gitattributes_path)
    if gitattributes_changed:
        click.echo("✓ Added logmind block to .gitattributes")
    if not no_git:
        configure_merge_drivers(root_path)
        install_post_merge_hook(root_path)
        install_post_rewrite_hook(root_path)

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
                "docs/timeline.md",
                ".logmind/config.yml",
            ]

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

            # .gitignore (if logmind block was added)
            if gitignore_changed:
                files_to_commit.append(".gitignore")

            # .gitattributes (if logmind merge-driver block was added)
            if gitattributes_changed:
                files_to_commit.append(".gitattributes")

            commit_and_push(
                files_to_commit,
                "logmind: Initialize decision tracking",
                push=True,
            )
            click.echo("✓ Committed changes: \"logmind: Initialize decision tracking\"")
        except Exception as e:
            click.secho(f"Warning: Failed to commit: {e}", fg="yellow")

    # Act on the skill-install decision captured at the start of init.
    _maybe_install_skill(skill_install_flag)

    # Optionally install the local pre-commit hook (opt-in flag only;
    # hooks aren't committed, so we never prompt — only install when asked).
    if install_hook and is_git_repo():
        try:
            from logmind.cli import install_hook as install_hook_cmd  # type: ignore
            # Programmatic invocation: call the Click command's callback
            ctx = click.Context(install_hook_cmd)
            ctx.invoke(install_hook_cmd, force=True)
        except Exception as e:
            click.secho(f"Warning: --install-hook failed: {e}", fg="yellow")

    click.echo()
    click.secho("logmind initialized successfully!", fg="green")
    click.echo()
    click.echo("Start logging decisions with:")
    click.echo("  from logmind import log")
    click.echo("  log(\"Your decision here\")")
    click.echo()
    if skill_install_flag is False or (skill_install_flag is None and not is_skills_available()):
        click.echo(
            "Tip: install the logmind agent skill once globally so every AI "
            "agent in every project picks up the procedure automatically:"
        )
        click.secho(
            f"  npx skills add -g {DEFAULT_SKILL_SOURCE} --skill logmind",
            fg="cyan",
        )

    click.echo()
    click.echo(
        "Tip: for docs/timeline.md to auto-regenerate cleanly, your repo needs:"
    )
    click.secho(
        "  • Branch ruleset on main: \"Require branches to be up to date "
        "before merging\" (strict status checks)",
        fg="cyan",
    )
    click.secho(
        "  • Settings → Actions → General → Workflow permissions: "
        "\"Read and write permissions\"",
        fg="cyan",
    )
    click.echo(
        "  Without these, two concurrent PRs editing docs/timeline.md may "
        "produce a merge conflict, OR the regen step may fail to push back "
        "to the PR branch."
    )

    # Final agent-friendly summary (always emits, even with --quiet).
    from logmind import __version__ as _v
    _ok(f"initialized: docs/ .logmind/ workflows @v{_v}")


def _show_skill_recommendation(flag: Optional[bool]) -> None:
    """
    Print a prominent prompt about the logmind agent skill BEFORE any files
    are written, so the user sees the recommendation up-front rather than
    buried in init output.

    Side effect: if interactive + flag is None + user confirms, we ALSO
    note the decision so the later _maybe_install_skill call actually
    installs. We do this by mutating sys.stdin signal? No — simplest is to
    write to a module-level flag... actually cleanest: the existing
    _maybe_install_skill already prompts; calling this function just shows
    the framing up-front and defers the actual prompt to the later call.

    For non-interactive runs the recommendation prints as a static note.
    """
    available = is_skills_available()

    # Make the box bright + bordered so it's visible without ANSI being weird.
    rule = "═" * 64
    click.secho(rule, fg="cyan")
    click.secho("  logmind agent skill — recommended", fg="cyan", bold=True)
    click.secho(rule, fg="cyan")
    click.echo()
    click.echo(
        "  Skills install the logmind decision-logging procedure into every"
    )
    click.echo(
        "  AI agent that supports skills.sh (Claude Code, Cursor, Codex, Cline,"
    )
    click.echo(
        "  Continue, ...). Install it once globally and any project that uses"
    )
    click.echo(
        "  logmind picks up the procedure automatically — no per-project setup."
    )
    click.echo()
    if available:
        if flag is False:
            click.secho(
                "  --no-skill-install was passed; skipping install.",
                fg="yellow",
            )
        elif flag is True:
            click.echo("  --skill-install passed; will install after files are written.")
        else:
            click.echo(
                "  skills CLI detected on your machine. You'll be prompted "
                "after files are written."
            )
    else:
        click.secho(
            "  skills CLI not detected. Install Node.js / npx to enable, then re-run:",
            fg="yellow",
        )
        click.secho(
            f"    npx skills add -g {DEFAULT_SKILL_SOURCE} --skill logmind",
            fg="cyan",
        )
    click.secho(rule, fg="cyan")
    click.echo()


def _maybe_install_skill(flag: Optional[bool]) -> None:
    """
    Offer to install the logmind agent skill globally via skills.sh.

    flag=True  → install without prompting
    flag=False → skip without prompting
    flag=None  → prompt only when stdin is a TTY (else skip silently)
    """
    if flag is False:
        return

    if not is_skills_available():
        if flag is True:
            click.secho(
                "skills CLI not found on PATH (install Node.js / npx, "
                "then re-run with --skill-install).",
                fg="yellow",
            )
        return

    if flag is None:
        if not sys.stdin.isatty():
            return
        if not click.confirm(
            "Install the logmind agent skill globally so all your AI agents "
            "know how to log decisions?",
            default=True,
        ):
            return

    click.echo(f"Installing logmind skill ({DEFAULT_SKILL_SOURCE} → {DEFAULT_SKILL_NAME})...")
    rc, output = install_logmind_skill()
    if rc == 0:
        click.secho("✓ logmind skill installed globally", fg="green")
    else:
        click.secho(
            f"Skill install exited {rc}. Output:\n{output.strip()}", fg="yellow"
        )


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
@click.option(
    "--stage",
    type=click.Choice(["all", "scoped"]),
    default="all",
    show_default=True,
    help=(
        "What to stage in the decision commit. Default 'all' stages every "
        "change in the working tree alongside the decision — the whole "
        "point of `logmind log` is to be a single add+commit+push primitive. "
        "Use 'scoped' if you have unrelated WIP you want to keep unstaged "
        "(rare for automated agents)."
    ),
)
def log(
    decision: str,
    reasoning: Optional[str],
    alternative: tuple,
    implication: tuple,
    no_commit: bool,
    no_push: bool,
    template: Optional[str],
    stage: str,
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

    # v0.2.9: emit a visible notice when the default --stage all sweeps the
    # working tree. Agents whose memory predates v0.2.7 keep prefixing
    # `git add -A &&` out of habit; making the actual behavior visible in
    # command output is the cheapest way to update that mental model
    # without forcing them to re-read AGENTS.md mid-task.
    if should_commit and stage == "all":
        click.secho(
            "ℹ Default --stage all (v0.2.7+): every working-tree change is "
            "staged into this decision commit. Pass --stage scoped to keep "
            "unrelated WIP unstaged.",
            fg="cyan",
        )

    try:
        # v0.1.3: run agent-file sync BEFORE the commit so refreshed AGENTS.md
        # / CLAUDE.md / etc. are included in the scoped staging instead of
        # left as dirty working-tree changes after the commit.
        sync_messages = sync_agent_files_from_config()
        for msg in sync_messages:
            click.echo(msg)

        # Snapshot agent files that the sync left modified so log_decision
        # can include them in the scoped commit. We list a fixed set of
        # known agent files (broader than the AGENTS.md auto-refresh, since
        # other agents may have stubs).
        extra_scoped_paths = _changed_agent_files()

        log_decision(
            decision=decision,
            reasoning=reasoning,
            alternatives=alternatives,
            implications=implications,
            docs_path=docs_path,
            auto_commit=should_commit,
            auto_push=should_push,
            stage=stage,
            extra_scoped_paths=extra_scoped_paths,
        )

        click.secho(f"✓ Logged decision: \"{decision}\"", fg="green")

        if should_commit:
            if should_push:
                click.echo("✓ Committed and pushed changes")
            else:
                click.echo("✓ Committed changes (push disabled)")

        # Final agent-friendly summary (always emits, even with --quiet).
        # Surface the commit SHA when we made one so agents can chain on it.
        commit_sha = ""
        if should_commit:
            try:
                result = subprocess.run(
                    ["git", "rev-parse", "--short", "HEAD"],
                    cwd=docs_path.parent,
                    capture_output=True,
                    text=True,
                    check=True,
                )
                commit_sha = result.stdout.strip()
            except (subprocess.CalledProcessError, FileNotFoundError):
                pass
        if commit_sha:
            _ok(f"logged: {commit_sha} \"{decision[:60]}\"")
        else:
            _ok(f"logged: \"{decision[:60]}\" (no commit)")

    except Exception as e:
        click.secho(f"Error: {e}", fg="red")
        sys.exit(1)


@main.command()
@click.option(
    "--base",
    default=None,
    help="Base branch to rebase onto. Defaults to the repo's default branch.",
)
@click.option(
    "--no-push",
    is_flag=True,
    help="Skip the push step. Just fetch + rebase.",
)
@click.option(
    "--no-fetch",
    is_flag=True,
    help="Skip the fetch step. Rebase against whatever origin/<base> already points at.",
)
def rebase(base: Optional[str], no_push: bool, no_fetch: bool):
    """Fetch origin, rebase the current branch onto origin/<base>, and force-with-lease push.

    Convenience wrapper for the recurring three-step pattern hit when a PR
    goes DIRTY after another PR's derived-doc regen lands on main:

        git fetch origin
        git rebase origin/<default-branch>
        git push --force-with-lease

    Reported by the tokenomics agent (2026-05-30) as Phase D friction:
    out-of-order merges in a PR batch trigger timeline.md / file-structure.md
    conflicts in the trailing PRs even when their substantive content
    doesn't overlap. Logmind's post-rewrite hook (v0.5.11) makes the
    rebase regen-clean; this wrapper makes the three-command dance one
    command.

    Exits non-zero on any step failure with a clear message about which
    step failed and what to do next. Refuses to run on a detached HEAD
    or on the default branch itself (rebasing main onto main is nonsense).
    """
    from logmind.core.git_handler import current_branch, default_branch, is_git_repo

    if not is_git_repo():
        click.secho("Error: not in a git repository.", fg="red")
        sys.exit(1)

    branch = current_branch()
    if branch is None:
        click.secho(
            "Error: detached HEAD — `logmind rebase` needs a named branch.",
            fg="red",
        )
        sys.exit(1)

    base_branch = base if base else default_branch()
    if branch == base_branch:
        click.secho(
            f"Error: refusing to rebase '{base_branch}' onto itself. "
            f"Check out a feature branch first.",
            fg="red",
        )
        sys.exit(1)

    repo_root = Path.cwd()
    steps_run: List[str] = []

    # Step 1: fetch
    if not no_fetch:
        click.echo(f"→ git fetch origin {base_branch}")
        try:
            subprocess.run(
                ["git", "fetch", "origin", base_branch],
                cwd=repo_root,
                check=True,
                capture_output=True,
                text=True,
            )
            steps_run.append("fetch")
        except subprocess.CalledProcessError as e:
            click.secho(
                f"Error: git fetch failed.\n{e.stderr}",
                fg="red",
            )
            sys.exit(1)

    # Step 2: rebase
    click.echo(f"→ git rebase origin/{base_branch}")
    try:
        subprocess.run(
            ["git", "rebase", f"origin/{base_branch}"],
            cwd=repo_root,
            check=True,
            capture_output=True,
            text=True,
        )
        steps_run.append("rebase")
    except subprocess.CalledProcessError as e:
        click.secho(
            f"Error: git rebase failed.\n{e.stderr}\n"
            f"Resolve conflicts manually, then run "
            f"`git rebase --continue` (or `git rebase --abort` to bail).",
            fg="red",
        )
        sys.exit(1)

    # Step 3: push (unless --no-push)
    if no_push:
        click.echo(f"✓ Rebased '{branch}' onto origin/{base_branch} (push skipped).")
        _ok(f"rebased: {branch} onto origin/{base_branch} (no push)")
        return

    click.echo(f"→ git push --force-with-lease origin {branch}")
    try:
        subprocess.run(
            ["git", "push", "--force-with-lease", "origin", branch],
            cwd=repo_root,
            check=True,
            capture_output=True,
            text=True,
        )
        steps_run.append("push")
    except subprocess.CalledProcessError as e:
        click.secho(
            f"Error: git push --force-with-lease failed.\n{e.stderr}\n"
            f"Rebase succeeded locally; you can retry push manually.",
            fg="red",
        )
        sys.exit(1)

    click.echo(f"✓ Rebased '{branch}' onto origin/{base_branch} and pushed.")
    _ok(f"rebased: {branch} onto origin/{base_branch} (pushed)")


@main.command()
@click.option(
    "--all",
    "-a",
    "show_all",
    is_flag=True,
    help="Show all decisions including archived",
)
@click.option(
    "--brief",
    is_flag=True,
    help="One-line summary per decision (date + title) instead of "
    "full markdown. Reduces ingest cost when agents read prior context.",
)
@click.option(
    "--limit",
    "-n",
    "limit",
    type=int,
    default=None,
    help="Show at most N most-recent decisions. Matches `logmind aggregate "
    "--limit` convention. Default: no limit (full file when --brief absent; "
    "all parsed entries when --brief set).",
)
@click.option(
    "--json",
    "as_json",
    is_flag=True,
    help="Emit a JSON array of {date, title, source} objects. Stable schema "
    "for downstream tools. Mutually exclusive with --brief (JSON wins).",
)
def show(show_all: bool, brief: bool, limit: Optional[int], as_json: bool):
    """Show recent decisions.

    Default: streams docs/decisions.md verbatim (the current 20 most-recent
    entries; older are in decisions-archive.md, surface via --all).

    Agent-friendly views (v0.5.2+):

      --brief                one-line summary per entry
      --limit N              cap to N most-recent
      --json                 structured array for parsing

    Combinations are allowed: `logmind show --brief --limit 5` for a quick
    last-5 recall, `logmind show --json --limit 10 --all` for parsed access
    across main + archive.
    """
    docs_path = Path.cwd() / "docs"

    if not docs_path.exists():
        click.secho(
            "Error: docs/ directory not found. Run 'logmind init' first.",
            fg="red",
        )
        sys.exit(1)

    # Sync agent files from config. In --json mode, suppress sync chatter
    # entirely (it's not the primary output and would corrupt `... | jq`
    # pipelines by printing non-JSON before the array). Sync still runs.
    sync_messages = sync_agent_files_from_config()
    if not as_json:
        for msg in sync_messages:
            click.echo(msg)

    decisions_path = docs_path / "decisions.md"

    if not decisions_path.exists():
        if as_json:
            _orig_click_echo("[]")
        else:
            click.secho("No decisions logged yet.", fg="yellow")
        _ok("show: 0 decisions (none logged yet)", err=as_json)
        return

    # Default verbatim view (preserves pre-v0.5.2 behavior).
    if not (brief or as_json or limit is not None):
        click.echo(decisions_path.read_text(encoding="utf-8"))

        archive_shown = False
        if show_all:
            archive_path = docs_path / "decisions-archive.md"
            if archive_path.exists():
                click.echo("\n" + "=" * 80)
                click.echo("ARCHIVED DECISIONS")
                click.echo("=" * 80 + "\n")
                click.echo(archive_path.read_text(encoding="utf-8"))
                archive_shown = True
        decisions_bytes = decisions_path.stat().st_size
        suffix = " + archive" if archive_shown else ""
        _ok(f"show: docs/decisions.md ({decisions_bytes} bytes{suffix})")
        return

    # Parsed-view paths (brief / limit / json). Load entries with provenance.
    from logmind.core.parser import iter_decisions

    entries = []
    for dt, title in iter_decisions(decisions_path):
        entries.append({"date": dt, "title": title, "source": "main"})
    if show_all:
        archive_path = docs_path / "decisions-archive.md"
        if archive_path.exists():
            for dt, title in iter_decisions(archive_path):
                entries.append({"date": dt, "title": title, "source": "archive"})

    # Sort newest-first so --limit N picks the latest entries.
    entries.sort(key=lambda e: e["date"], reverse=True)
    if limit is not None:
        entries = entries[:limit]

    if as_json:
        import json
        # ALWAYS emit JSON to stdout (bypass quiet patch — JSON is the
        # primary output for downstream parsers, not progress chatter).
        _orig_click_echo(
            json.dumps(
                [
                    {
                        "date": e["date"].isoformat(),
                        "title": e["title"],
                        "source": e["source"],
                    }
                    for e in entries
                ],
                indent=2,
            )
        )
    else:
        # brief OR limit-only: render one-line-per-entry. limit-only falls
        # here because we only carry (date, title) from the parser (no body
        # for verbatim) — one-line-per-entry is the natural "N most recent"
        # answer. Use _orig_click_echo so output isn't suppressed by --quiet.
        for e in entries:
            _orig_click_echo(f"{e['date'].strftime('%Y-%m-%d %H:%M')} — {e['title']} [{e['source']}]")

    # Route the ok line to stderr in JSON mode so stdout is parseable JSON.
    _ok(
        f"show: {len(entries)} decisions ({'json' if as_json else 'brief'})",
        err=as_json,
    )


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


@agents.command("update")
@click.option(
    "--apply",
    "do_apply",
    is_flag=True,
    help="Rewrite stale marker blocks in place. Default is dry-run.",
)
@click.option(
    "--commit",
    is_flag=True,
    help="git-commit the refreshed files after applying. Requires --apply.",
)
def agents_update(do_apply: bool, commit: bool):
    """
    Refresh outdated logmind marker blocks in AGENTS.md.

    Sync (which runs on every `logmind log/show/search/agents list`) does
    this automatically and silently. This command exposes it explicitly
    for users who want a dry-run preview or a one-shot upgrade after
    `pip install -U logmind`.

    Dry-run (default): reports which files have stale blocks.
    `--apply`:         rewrites the block body in place, preserving
                        everything above and below the markers.
    `--commit`:        also git-commits the refresh (requires --apply).
    """
    root_path = Path.cwd()
    outdated = find_outdated_marker_blocks(root_path)
    # v0.5.13: also sweep CI workflow pin lines. The clud-bug update
    # cycle re-renders workflows in consumer repos without bumping the
    # logmind pin — pre-v0.5.13 this required manual `sed -i` after
    # every cycle. Bundling here means one `logmind agents update
    # --apply` refreshes both AGENTS.md AND the CI pins in one shot.
    stale_pins = find_outdated_workflow_pins(root_path)

    if not outdated and not stale_pins:
        # v0.5.8 / issue #57: "All agent files are current" was misleading
        # in two distinct cases — AGENTS.md absent, or AGENTS.md present
        # without a logmind marker block. Split them out so the message
        # accurately describes WHY no update is needed.
        agents_path = root_path / "AGENTS.md"
        if not agents_path.exists():
            click.secho(
                "✓ No AGENTS.md in this repo — nothing to update. "
                "Run `logmind init` to install one.",
                fg="green",
            )
        else:
            from logmind.core.inserter import _extract_marker_block
            installed = _extract_marker_block(
                agents_path.read_text(encoding="utf-8")
            )
            if installed is None:
                click.secho(
                    "✓ AGENTS.md exists but has no logmind marker block. "
                    "Run `logmind init` to install one (will preserve "
                    "existing content above + below the markers).",
                    fg="green",
                )
            else:
                click.secho(
                    "✓ AGENTS.md logmind block is current "
                    "(no update needed).",
                    fg="green",
                )
        return

    # v0.5.8 / issue #57: surface the version delta on dry-run so the
    # user knows what `--apply` would actually do.
    if outdated:
        if not do_apply:
            click.echo(
                f"Would update {len(outdated)} file(s) with stale logmind block(s):"
            )
        else:
            click.echo(f"Found {len(outdated)} file(s) with stale logmind block(s):")
        for file_path, _old, _new in outdated:
            rel = file_path.relative_to(root_path)
            click.echo(f"  - {rel}")

    # v0.5.13: report workflow pin drift alongside AGENTS.md drift.
    if stale_pins:
        if not do_apply:
            click.echo(
                f"Would update {len(stale_pins)} CI workflow pin(s):"
            )
        else:
            click.echo(f"Found {len(stale_pins)} CI workflow pin(s) to bump:")
        for wf_path, old_v, new_v in stale_pins:
            rel = wf_path.relative_to(root_path)
            click.echo(f"  - {rel} (logmind=={old_v} → logmind=={new_v})")

    if not do_apply:
        click.echo()
        click.secho(
            "Dry-run. Re-run with --apply to refresh.",
            fg="yellow",
        )
        return

    refreshed_paths: list = []
    for file_path, _old, fresh in outdated:
        content = file_path.read_text(encoding="utf-8")
        new_content = _replace_marker_block(content, fresh)
        file_path.write_text(new_content, encoding="utf-8")
        rel = file_path.relative_to(root_path)
        refreshed_paths.append(str(rel))
        click.secho(f"✓ Refreshed {rel}", fg="green")

    # v0.5.13: write the refreshed pins.
    for wf_path, _old, new_v in stale_pins:
        content = wf_path.read_text(encoding="utf-8")
        new_content, _ = update_workflow_pin(content, new_v)
        wf_path.write_text(new_content, encoding="utf-8")
        rel = wf_path.relative_to(root_path)
        refreshed_paths.append(str(rel))
        click.secho(f"✓ Bumped logmind pin in {rel}", fg="green")

    if commit and is_git_repo():
        try:
            commit_and_push(
                refreshed_paths,
                "logmind: refresh AGENTS.md marker block to current template",
                push=False,
            )
            click.secho("✓ Committed changes (push disabled)", fg="green")
        except Exception as e:
            click.secho(f"Warning: commit failed: {e}", fg="yellow")


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


# v0.6.0 — `logmind skill` subgroup (SkDD skill authoring + validation)
@main.group()
def skill():
    """Author + validate SKILL.md files (composes with Zak Elfassi's skdd CLI).

    Per the Skills-Driven Development (SkDD) methodology — see
    https://zakelfassi.com/skdd-skills-driven-development.

    These commands compose with `@zakelfassi/skdd` when it's on PATH:

      - `logmind skill new` prefers `skdd forge` if available
      - `logmind skill test` prefers `skdd validate` if available

    Logmind layers on: decision-logging on skill creation (audit
    trail), additional validation (byte cap, frontmatter required
    fields), and the recursive-iteration loop logmind is positioned
    to handle (v0.6.1+).
    """
    pass


@skill.command("new")
@click.argument("name")
@click.option(
    "--description",
    default="",
    help="One-sentence trigger description. The discovery surface.",
)
@click.option(
    "--no-log",
    is_flag=True,
    help="Skip decision-logging the skill creation.",
)
def skill_new(name: str, description: str, no_log: bool):
    """Create a new SKILL.md scaffolded for the agentskills.io/v1 spec.

    Prefers `skdd forge <name>` when on PATH (canonical SkDD authoring).
    Falls back to a basic scaffold otherwise. In both cases, the skill
    creation is decision-logged via `logmind log` (audit trail).

    Refuses to clobber an existing skill of the same name — delete
    first if you want to recreate.
    """
    from logmind.core.skill_cli import (
        delegate_skdd_forge,
        has_skdd,
        scaffold_basic_skill,
        skill_md_path,
    )

    repo_root = Path.cwd()
    target = skill_md_path(repo_root, name)
    if target.exists():
        click.secho(
            f"Error: skill '{name}' already exists at {target}",
            fg="red",
        )
        sys.exit(1)

    if has_skdd():
        click.echo(f"→ skdd forge {name}")
        ok, output = delegate_skdd_forge(repo_root, name)
        if not ok:
            click.secho(
                f"Error: skdd forge failed.\n{output}",
                fg="yellow",
            )
            # v0.6.0 PR #92 review fix: skdd forge may create the SKILL.md
            # AND THEN exit non-zero (e.g., post-creation validation fails).
            # In that case, our fallback scaffold_basic_skill raises
            # FileExistsError (its clobber-guard). Catch it so the user
            # sees a clean error instead of a raw traceback.
            try:
                click.echo("Falling back to basic scaffold.")
                scaffold_basic_skill(repo_root, name, description=description)
            except FileExistsError:
                # skdd already created the file but reported failure on it
                click.secho(
                    f"Error: skdd forge partially succeeded (file exists "
                    f"at {target}) but reported failure. Inspect the file "
                    f"+ skdd output above, then either fix it manually or "
                    f"`rm -r {target.parent}` and re-run.",
                    fg="red",
                )
                sys.exit(1)
        else:
            click.echo(output.strip() if output.strip() else "(skdd produced no output)")
    else:
        click.echo(
            "→ scaffolding basic SKILL.md (`skdd` not on PATH; install "
            "@zakelfassi/skdd for canonical SkDD authoring)"
        )
        scaffold_basic_skill(repo_root, name, description=description)

    click.secho(f"✓ Created skill '{name}' at {skill_md_path(repo_root, name)}", fg="green")

    # Decision-log the skill creation. Skip on --no-log (test scenarios,
    # CI runs that don't want the auto-decision).
    if not no_log:
        docs_path = repo_root / "docs"
        if docs_path.exists():
            try:
                from logmind.core.logger import log as _logmind_log
                _logmind_log(
                    f"Created skill '{name}' via `logmind skill new`",
                    reasoning=(
                        f"Skill scaffolded {'via skdd forge' if has_skdd() else 'via basic scaffold (skdd not on PATH)'}. "
                        f"Trigger description: {description or '(TODO)'}. "
                        f"Per SkDD methodology (Zak Elfassi)."
                    ),
                    docs_path=docs_path,
                    auto_commit=False,  # let the user commit when ready
                )
                click.echo("✓ Decision-logged the skill creation (uncommitted).")
            except Exception as e:
                click.secho(
                    f"Warning: failed to decision-log skill creation: {e}",
                    fg="yellow",
                )
        else:
            click.echo(
                "(skipped decision-log: docs/ not present — run `logmind init` to enable)"
            )

    _ok(f"skill: created {name}")


@skill.command("test")
@click.argument("name")
def skill_test(name: str):
    """Validate a SKILL.md against the agentskills.io/v1 spec + logmind checks.

    Prefers `skdd validate` when on PATH (canonical spec validation).
    Layers logmind-specific checks: frontmatter required fields,
    soft size cap (8KB — guards against skills that bloat every load).

    Exits non-zero on any validation failure so CI can gate on it.
    """
    from logmind.core.skill_cli import (
        check_frontmatter_required_fields,
        check_size_cap,
        delegate_skdd_validate,
        has_skdd,
        skill_md_path,
    )

    repo_root = Path.cwd()
    target = skill_md_path(repo_root, name)
    if not target.exists():
        click.secho(
            f"Error: skill '{name}' not found at {target}",
            fg="red",
        )
        sys.exit(1)

    failed = False

    if has_skdd():
        click.echo(f"→ skdd validate (filtering for '{name}')")
        ok, output = delegate_skdd_validate(repo_root, name)
        if output.strip():
            click.echo(output.strip())
        if not ok:
            failed = True
            click.secho("✗ skdd validate failed", fg="red")
        else:
            click.secho("✓ skdd validate passed", fg="green")
    else:
        click.echo(
            "(skipping skdd validate — `skdd` not on PATH; install "
            "@zakelfassi/skdd for canonical spec checks)"
        )

    # Always run logmind-specific layered checks.
    content = target.read_text(encoding="utf-8")
    for check_name, check_fn in [
        ("frontmatter required fields", check_frontmatter_required_fields),
        ("size cap", check_size_cap),
    ]:
        ok, msg = check_fn(content)
        if ok:
            click.secho(f"✓ {check_name}: {msg or 'ok'}", fg="green")
        else:
            failed = True
            click.secho(f"✗ {check_name}: {msg}", fg="red")

    if failed:
        _ok(f"skill: {name} FAILED validation")
        sys.exit(1)
    _ok(f"skill: {name} validated")


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

    # If decisions.md OR a per-branch decision file is staged, changes are
    # documented. Branch-aware mode (the default) routes feature-branch logs
    # to docs/decisions-branches/<branch>.md, so we must accept either.
    def _is_decision_file(path: str) -> bool:
        return (
            path == "docs/decisions.md"
            or path.endswith("/decisions.md")
            or path.startswith("docs/decisions-branches/")
        )

    if any(_is_decision_file(f) for f in staged_files):
        click.secho(
            "✓ A decision log file is staged — changes are documented.",
            fg="green",
        )
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


@main.command("timeline")
@click.option(
    "--write",
    "write_path",
    type=click.Path(path_type=Path),
    default=None,
    help="Write the rendered timeline to PATH (typically docs/timeline.md). "
    "Without this flag, prints to stdout.",
)
@click.option(
    "--check",
    is_flag=True,
    default=False,
    help="Exit nonzero if writing would change the file. Used in CI to fail "
    "the build before regen so the auto-commit step runs and updates the PR.",
)
@click.option(
    "--full",
    is_flag=True,
    default=False,
    help="Render the legacy per-decision format (one bullet per entry). "
    "Default is brief (v0.5.4+): first + last entry per month with a "
    "`... N more decisions ...` elision line — token-frugal on disk.",
)
def timeline_cmd(write_path: Optional[Path], check: bool, full: bool):
    """
    Print or regenerate the high-level decision timeline.

    Reads docs/decisions.md, docs/decisions-archive.md, and every
    docs/decisions-branches/*.md as sources; renders a chronological
    timeline grouped by year-month. Sources are never modified.

    Examples:
        logmind timeline                              # brief, to stdout
        logmind timeline --full                       # full per-decision
        logmind timeline --write docs/timeline.md     # brief, on disk
        logmind timeline --write docs/timeline.md --check  # CI gate
    """
    from logmind.core.timeline import generate_timeline, write_timeline

    docs_path = Path.cwd() / "docs"
    if not docs_path.exists():
        click.secho(
            "Error: docs/ directory not found. Run 'logmind init' first.",
            fg="red",
        )
        sys.exit(1)

    brief = not full

    if check:
        if write_path is None:
            click.secho(
                "Error: --check requires --write PATH to compare against.",
                fg="red",
            )
            sys.exit(2)
        rendered = generate_timeline(docs_path, brief=brief)
        existing = (
            write_path.read_text(encoding="utf-8") if write_path.exists() else ""
        )
        if existing != rendered:
            click.secho(
                f"✗ {write_path} is stale — re-run "
                f"`logmind timeline --write {write_path}` and commit.",
                fg="yellow",
            )
            sys.exit(1)
        click.secho(f"✓ {write_path} is up to date", fg="green")
        _ok(f"timeline: {write_path} up to date")
        return

    if write_path is None:
        rendered = generate_timeline(docs_path, brief=brief)
        _orig_click_echo(rendered, nl=False)
        # utf-8 byte count, not character count — see file-structure_cmd.
        mode = "brief" if brief else "full"
        _ok(f"timeline: {len(rendered.encode('utf-8'))} bytes (stdout, {mode})")
        return

    changed = write_timeline(write_path, docs_path, brief=brief)
    if changed:
        click.secho(f"✓ Regenerated {write_path}", fg="green")
    else:
        click.echo(f"  {write_path} already up to date")
    out_bytes = Path(write_path).stat().st_size
    mode = "brief" if brief else "full"
    _ok(f"timeline: {write_path} ({out_bytes} bytes, {mode})")


@main.command("doctor")
@click.option(
    "--json", "as_json", is_flag=True, help="Emit the report as JSON."
)
@click.option(
    "--offline",
    is_flag=True,
    help="Skip PyPI / npm probes; use only locally-readable signals.",
)
@click.option(
    "--exit-zero",
    is_flag=True,
    help="Always exit 0, even on drift (for informational CI runs).",
)
def doctor_cmd(as_json: bool, offline: bool, exit_zero: bool):
    """
    Report installed versions and workflow drift for logmind + clud-bug.

    Reads .github/workflows/*.yml pin lines and template-version markers,
    optionally probes PyPI and the npm registry for the latest releases,
    then prints a status table. Exits non-zero on drift so it's CI-pluggable.

    Network is best-effort: a PyPI/npm probe failure degrades to "?" in the
    latest column rather than erroring. Use --offline to skip network entirely.
    """
    from logmind.core.doctor import collect_status, render_status

    report = collect_status(Path.cwd(), offline=offline)

    if as_json:
        click.echo(report.to_json())
    else:
        click.echo(render_status(report))

    if report.overall == "DRIFT" and not exit_zero:
        sys.exit(1)


@main.command("tree")
@click.option(
    "--max-depth",
    "max_depth",
    type=int,
    default=None,
    help="Cap the tree at depth N (root is depth 0). Default: 2 (token-frugal — "
    "matches `logmind file-structure`). Pass 0 for unbounded (full tree); "
    "pass a positive integer to truncate.",
)
def tree_cmd(max_depth: Optional[int]):
    """
    Regenerate docs/file-structure.md with the current project tree.

    Equivalent to the side-effect that runs after every ``logmind log`` when
    ``file_structure.auto_update: true`` is set in ``.logmind/config.yml``.
    Useful as a pre-commit hook step or when an agent has just written
    several files and wants the docs/ snapshot to reflect them immediately.
    """
    docs_path = Path.cwd() / "docs"
    if not docs_path.exists():
        click.secho(
            "Error: docs/ directory not found. Run 'logmind init' first.",
            fg="red",
        )
        sys.exit(1)
    # CLI convention: --max-depth 0 means unbounded (None internally).
    # --max-depth omitted falls through to update_file_structure's default.
    if max_depth is None:
        update_file_structure(docs_path)
        effective = "default"
    else:
        from logmind.core.tree_gen import write_file_structure
        write_file_structure(
            docs_path / "file-structure.md",
            max_depth=None if max_depth == 0 else max_depth,
        )
        effective = "unbounded" if max_depth == 0 else f"depth={max_depth}"
    click.secho("✓ Updated docs/file-structure.md", fg="green")
    out_bytes = (docs_path / "file-structure.md").stat().st_size
    _ok(f"docs/file-structure.md ({out_bytes} bytes, {effective})")


@main.command("file-structure")
@click.option(
    "--write",
    "write_path",
    type=click.Path(path_type=Path),
    default=None,
    help="Write the rendered tree to PATH (typically docs/file-structure.md). "
    "Without this flag, prints to stdout.",
)
@click.option(
    "--max-depth",
    "max_depth",
    type=int,
    default=None,
    help="Cap the tree at depth N (root is depth 0). Default: 2 (token-frugal). "
    "Pass 0 for unbounded (full tree); pass a positive integer to truncate.",
)
def file_structure_cmd(write_path: Optional[Path], max_depth: Optional[int]):
    """
    Print or regenerate the derived docs/file-structure.md tree snapshot.

    Mirror of ``logmind timeline`` for the file-structure derived doc.
    The v0.3.0 git merge driver invokes this as
    ``logmind file-structure --write %A`` to resolve conflicts on
    parallel-PR rebases without falling through to textual three-way merge.

    Examples:
        logmind file-structure                                # depth 2, stdout
        logmind file-structure --max-depth 0                  # full tree
        logmind file-structure --write docs/file-structure.md # regenerate file at depth 2
    """
    from logmind.core.tree_gen import (
        DEFAULT_FILE_STRUCTURE_DEPTH,
        generate_file_structure,
        write_file_structure,
    )

    # CLI convention: --max-depth 0 means unbounded (None internally).
    # --max-depth omitted defaults to DEFAULT_FILE_STRUCTURE_DEPTH (2).
    if max_depth is None:
        effective_depth = DEFAULT_FILE_STRUCTURE_DEPTH
    elif max_depth == 0:
        effective_depth = None
    else:
        effective_depth = max_depth

    depth_label = "unbounded" if effective_depth is None else f"depth={effective_depth}"

    repo_root = Path.cwd()
    if write_path is None:
        rendered = generate_file_structure(repo_root, max_depth=effective_depth)
        # Use unpatched echo so the tree itself isn't suppressed by --quiet;
        # the tree is the command's PRIMARY output, not progress chatter.
        _orig_click_echo(rendered, nl=False)
        # Use utf-8 byte count (not character count) so this matches the
        # write-path's Path.stat().st_size — em-dashes/non-ASCII would
        # otherwise undercount. clud-bug PR #69 caught this.
        _ok(f"file-structure: {len(rendered.encode('utf-8'))} bytes, {depth_label} (stdout)")
        return
    changed = write_file_structure(write_path, max_depth=effective_depth)
    if changed:
        click.secho(f"✓ Regenerated {write_path}", fg="green")
    else:
        click.echo(f"  {write_path} already up to date")
    out_bytes = Path(write_path).stat().st_size
    _ok(f"{write_path} ({out_bytes} bytes, {depth_label})")


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
        content = hook_path.read_text(encoding="utf-8")
        if "logmind check-decisions" in content:
            click.echo("✓ logmind hook already installed.")
            return
        if not force:
            click.secho(
                "A pre-commit hook already exists. Use --force to append logmind to it.",
                fg="yellow",
            )
            sys.exit(1)
        hook_path.write_text(content.rstrip("\n") + "\n" + hook_line, encoding="utf-8")
        click.secho(
            "✓ Added logmind check-decisions to existing pre-commit hook.", fg="green"
        )
    else:
        hook_path.parent.mkdir(parents=True, exist_ok=True)
        hook_path.write_text("#!/bin/sh\n" + hook_line, encoding="utf-8")
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
