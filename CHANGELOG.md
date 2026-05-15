# Changelog

All notable changes to logmind will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-05-15

### Added — v0.1.1 polish (Phase 13)
- **`logmind agents update` CLI command** — explicit dry-run / `--apply` / `--commit` upgrade path for the AGENTS.md marker block, complementing the silent auto-refresh on every `logmind log / show / search / agents list`. New public helper `inserter.find_outdated_marker_blocks(root_path)`.
- **Prominent skill-install prompt at init** — the `logmind` agent skill is now surfaced in a visible box BEFORE any files are written, so it doesn't get lost in the "✓ Created ..." output. Adds a closing tip when skill install is skipped.
- **`check-decisions.yml` GitHub Action template** — third workflow shipped by `logmind init` (alongside `logmind-aggregate.yml` and `check-doc-links.yml`). Mirrors the local pre-commit hook against the PR diff; fails the build on >20 non-docs lines without a decision log update.
- **`--install-hook` flag on `logmind init`** — opt-in local pre-commit hook installation in the same step as init.
- **skills.sh install-counter badge** on `README.md` and the `logmind.dev` landing page footer.

### Changed
- **`.github/workflows/claude-review.yml` → `clud-bug` (`thrillmot/clud-bug`)** — replaced the hand-rolled Claude-code-action workflow with the user's first-party `clud-bug` install. Generated workflow skips Dependabot + Renovate PRs (best-practice pattern).
- **`AGENTS.md` template is now adaptive**: ships the slim variant (defers to the `logmind` skill) when skills.sh is on PATH; ships the full variant (procedure inline) when it isn't.
- **`reporulez` (`thrillmot/reporulez`, `external` variant) applied to all 3 repos** (logmind, homebrew-logmind, logmind-skill) — standardised ruleset replacing the manual `gh api branch-protection` rule from v0.1.0. Squash-only merges, linear history, force-push + delete blocked, thread resolution required.
- **`check-doc-links.yml` runs unconditionally** (dropped the `paths: ["**/*.md"]` filter). Required-status-check interacts badly with path-filtered workflows on the reporulez ruleset; ~15s unconditional cost is acceptable.

### Fixed
- **`logmind check-decisions` is now branch-aware**. The CLI command + pre-commit hook used to only accept `docs/decisions.md` updates as documented changes, which made the hook impossible to satisfy on a feature branch under `branch_aware: true` (the default since v0.1). Now accepts `docs/decisions-branches/<branch>.md` too.

### Tests
- 455 → 458 tests passing (3 added: agents update CLI dry-run / apply / idempotency, check-decisions branch-aware regression).

## [0.1.0] - 2026-05-15

### Added — branch-aware logging & open-source readiness (Phases 5–11)
- **Branch-aware decision storage**: feature-branch decisions route to `docs/decisions-branches/<sanitized-branch>.md`; default-branch decisions stay in `docs/decisions.md`. New `decisions.branch_aware` config knob (default `true`).
- **PR-merge aggregator GitHub Action** (`.github/workflows/logmind-aggregate.yml`): on PR close+merge, appends a one-line summary to `docs/decisions.md` linking the PR and the per-branch detail file.
- **Markdown link integrity**: new `logmind check-links` CLI subcommand and `.github/workflows/check-doc-links.yml` workflow. Walks README + agent files + `docs/` for broken or orphan markdown links. Configurable via `linkcheck.allow_orphans` and `linkcheck.roots`.
- **AGENTS.md as canonical agent-instructions hub**: per-tool files (CLAUDE.md, .cursorrules, .windsurfrules, ...) are now 2-line stubs pointing at AGENTS.md. New `logmind agents migrate` consolidates legacy per-agent content into AGENTS.md and replaces files with stubs. JSON agents (cody, zed) unchanged.
- **Tree generation hardening**: `generate_tree()` and the Python fallback now augment `DEFAULT_IGNORES` with the project's `.gitignore` basenames; fallback is unbounded by default with stable dirs-first ordering. New `logmind tree` CLI subcommand for on-demand regeneration. `update_file_structure()` no longer depends on caller cwd.
- **Managed `.gitignore` block**: `logmind init` appends a marker-bracketed block (`.logmind/cache/`, `.logmind/.lock`); idempotent and preserves manual edits inside the markers.
- **logmind agent skill (skills.sh)**: new `skill/SKILL.md` content for the standalone `thrillmot/logmind-skill` repo. `logmind init` offers to install it globally via the user's `skills` CLI (or `npx`). New `--skill-install / --no-skill-install` flag.
- **Open-source readiness**: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, issue + PR templates, `.github/dependabot.yml`, `.pre-commit-config.yaml`, `.github/workflows/test.yml` (Python 3.8/3.10/3.12/3.13 on Ubuntu + 3.12 on macOS/Windows), `.github/workflows/publish.yml` (tag-driven PyPI publish via OIDC + GitHub Release), README badges, `[tool.ruff]` + `mypy` config in `pyproject.toml`.
- Test suite expanded to 446 tests (all passing).

### Added — earlier this cycle (legacy [Unreleased])
- `logmind stats` — analytics dashboard with ASCII bar chart, monthly activity, velocity trend, and top keywords
- `logmind aggregate` — view decisions across multiple projects in a unified feed; `--summary` for per-project counts
- `logmind check-decisions` — pre-commit hook check: exits 1 if staged changes exceed threshold without updating `decisions.md`
- `logmind install-hook` — one-command installation of logmind as a git pre-commit hook
- `logmind templates` — list all built-in decision templates
- `logmind log --template` — pre-fill reasoning, alternatives, and implications from a named template
- 7 built-in templates: `database`, `api`, `architecture`, `security`, `performance`, `library`, `deployment`
- `logmind.integrations.LangChainLogger` — LangChain callback handler for automatic decision logging
- `logmind.integrations.base.BaseIntegration` — base class for building custom framework integrations
- `docs/custom-integrations.md` — guide for building and publishing custom integrations

## [0.1.0] - 2025-10-19

### Added
- Initial release of logmind
- `logmind init` command to scaffold decision logging structure
- `logmind log` command (CLI) and `log()` function (Python API) for logging decisions
- `logmind show` command to view recent decisions
- `logmind search` command to search through decision history
- Automatic git integration (commits and pushes on each decision)
- Configuration system via `.logmind/config.yml`
- Decision archival (keeps 20 most recent, archives older ones)
- Automatic file structure tracking using `tree` command
- Smart insertion into AI instruction files (CLAUDE.md, .cursorrules, etc.)
- `@log_decision` and `@log_choice` decorators for automatic logging
- Template string support in decorators (`{arg_name}` placeholders)
- Comprehensive test suite (110 tests)
- Full documentation in README and docs/

### Features
- **Core Logging**: Append-only markdown files with automatic archival
- **Git Integration**: Every decision = one commit, full audit trail
- **Search**: Regex-based search with context lines and highlighting
- **Configuration**: Customizable commit messages, auto-push toggle, archival threshold
- **Decorators**: Automatic decision logging via function decorators
- **AI-Friendly**: Designed for AI agents (Claude, GPT, Copilot) with clear context

[0.1.0]: https://github.com/thrillmot/logmind/releases/tag/v0.1.0
