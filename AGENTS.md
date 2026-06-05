# AGENTS.md

This is the canonical instruction file for AI coding agents working in this
repository. Tools that understand `AGENTS.md` (Cursor, Codex, Windsurf, Claude
Code, Cline, Continue, Aider, Amazon Q, ...) read this file directly. Per-tool
files like `CLAUDE.md` or `.cursorrules` are stubs that point here so the
guidance lives in one place.

<!-- logmind-start -->
<!-- logmind-block-version: v5-slim -->
## Decision logging — `logmind log` is the commit primitive

This project uses [logmind](https://logmind.dev) for decision logging. **`logmind log` replaces `git add`, `git commit`, and `git push`** for any change that carries a decision. Do not run those git commands directly when logging a decision — `logmind log` does all three in one step.

```bash
logmind log "decision summary" -r "why" -a "alternative" -i "implication"
```

That single command:

1. Writes the decision file (default branch → `docs/decisions.md`, feature branch → `docs/decisions-branches/<branch>.md`).
2. Regenerates `docs/timeline.md` (and `file-structure.md` on the default branch).
3. **`git add`** every change in the working tree (default `--stage all` since v0.2.7).
4. **`git commit`** with message `logmind: <decision>`.
5. **`git push`** to remote.

That's it. No follow-up `git add`, no follow-up `git commit`, no follow-up `git push`, no follow-up `logmind timeline --write`.

### When NOT to use `logmind log`

- **The change has no decision worth recording** (typo fix, formatting, dependency-only patch bump) — use plain `git commit` then.
- **You have unrelated WIP in the working tree** you don't want to commit — pass `--stage scoped` to stage only the decision files (rare for automated agents; they should be working on one thing at a time).

### The full skill

The complete procedure (when something counts as a decision, branch routing, doctor, refresh patterns) lives in the **`logmind` agent skill** at https://github.com/thrillmade/agent-skills/tree/main/skills/logmind. Most agent runtimes auto-load it. If yours doesn't:

```bash
npx skills add https://github.com/thrillmade/agent-skills --skill logmind
```

### Required reading before non-trivial work

1. **[docs/timeline.md](docs/timeline.md)** — chronological overview across all branches; start here.
2. **[docs/decisions.md](docs/decisions.md)** — recent decisions on the default branch.
3. **`docs/decisions-branches/<your-branch>.md`** if present — earlier decisions on this branch.
4. **[docs/file-structure.md](docs/file-structure.md)** — current project tree.

```bash
logmind show               # recent decisions on the current branch
logmind search "keyword"   # full-text across recent + archive
logmind doctor             # check installed version + workflow template drift
```

The team has likely already decided things you'd otherwise re-litigate.
<!-- logmind-end -->

## Release infrastructure

Cross-repo writes from this project's workflows (Homebrew cask bumps today;
future skill-catalog PRs and self-update fan-out) are signed by the
`thrillmade-orchestrator[bot]` GitHub App, NOT a personal PAT. See
[`docs/orchestrator-app.md`](docs/orchestrator-app.md) for the App spec,
permissions, ruleset bypass details, and rotation procedure.

## Project Overview

<!-- Replace with a short description of what this project does. -->

## Development Commands

<!-- Common commands a contributor needs (build, test, lint, run). -->

## From Claude Code

# CLAUDE.md



This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**logmind** is an AI decision logging system that scaffolds documentation structure and tracks development decisions automatically.

**Key concept:** A pip-installable package that runs `logmind init` to create standardized docs, then serves as the decision logging engine.

## Essential Documentation

- **[docs/plan.md](docs/plan.md)** - Complete architecture, approach, and roadmap
- **[README.md](README.md)** - User-facing overview and quick start

## Development Commands

This is a Python package. Standard commands:

```bash
# Setup development environment
python3 -m venv venv
source venv/bin/activate
pip install -e ".[dev]"

# Run tests
pytest                    # All tests
pytest tests/test_*.py -v # Specific test file

# Build package
python -m build
```

## logmind CLI Commands

You can use logmind via CLI or Python API:

```bash
# View recent decisions
logmind show
logmind show --all  # Include archive

# Search decisions
logmind search "postgres"
logmind search "API" --case-sensitive
logmind search "config" --no-archive

# Log decision via CLI
logmind log "Use Redis for caching" \
  -r "Need fast session storage" \
  -a "Memcached" -a "In-memory dict" \
  -i "Need to run Redis server"
```

## Current Status

**Phase 2 Complete** - Configuration system and search functionality implemented

✅ Phase 1: Core package (init, log, show)
✅ Phase 2: Configuration + search
🔲 Phase 3: AI integrations (decorators, plugins)

See [docs/plan.md](docs/plan.md) for complete roadmap.

## From Cursor

# Cursor Rules




## Project Rules

[Add your Cursor rules here]

<!-- clud-bug-start -->
<!-- clud-bug-block-version: v2 -->
## clud-bug — Claude PR review

This repo uses [clud-bug](https://cludbug.dev) for automatic PR reviews.
Full collaboration rules — fix-push flow, skill structure, comment format,
strict-mode mechanics, workflow-edit constraint — live in the bundled
[`clud-bug-collaboration` skill](.claude/skills/clud-bug-collaboration/SKILL.md).
Read that skill before pushing fixes addressing prior review threads.

Strict mode is **off** in this repo (advisory only). Toggle via `.claude/skills/.clud-bug.json`
(read from PR **base ref**, so PRs can't disable strict-mode on themselves).

For agent invocations of the `clud-bug` CLI, prefer `CLUD_BUG_QUIET=1`
(or pass `--quiet`) — suppresses progress chatter and emits a single
`ok <key-value>` summary line per command.

_Installed at clud-bug v0.6.32._
<!-- clud-bug-end -->
