# AGENTS.md

This is the canonical instruction file for AI coding agents working in this
repository. Tools that understand `AGENTS.md` (Cursor, Codex, Windsurf, Claude
Code, Cline, Continue, Aider, Amazon Q, ...) read this file directly. Per-tool
files like `CLAUDE.md` or `.cursorrules` are stubs that point here so the
guidance lives in one place.

<!-- logmind-start -->
## Decision Logging (logmind)

**IMPORTANT:** This project uses [logmind](https://github.com/logmind/logmind) for decision tracking.

### REQUIREMENT: AI Agents MUST Log All Decisions

**You MUST log a decision whenever you:**
- Make architectural or design choices
- Choose between alternative approaches
- Write significant new code (>20 lines)
- Modify existing functionality
- Add dependencies or libraries
- Make security or performance decisions

**BEFORE writing code, ask yourself: "Should this be logged?" If yes, log it IMMEDIATELY.**

### How to Log Decisions

**Python API:**
```python
from logmind import log

log("Decision summary",
    reasoning="Why this approach",
    alternatives=["Option A", "Option B"],
    implications=["Impact 1", "Impact 2"])
```

**CLI:**
```bash
logmind log "Use PostgreSQL for database" \
  -r "Need ACID compliance" \
  -a "MongoDB" -a "SQLite" \
  -i "Need connection pooling"
```

### Branch-aware logging

When you log on a feature branch, the entry is written to
`docs/decisions-branches/<branch>.md` rather than `docs/decisions.md`. On PR
merge, a workflow appends a one-line summary linking the PR + the per-branch
file to `docs/decisions.md`. Run `logmind show` and `logmind search` as usual
— they read both.

### Viewing Past Decisions

```bash
logmind show                  # recent decisions on current branch
logmind show --all            # include archive
logmind search "postgres"     # search across both files
```

### Required Reading

Before starting work in this repo, read:
- **[docs/decisions.md](docs/decisions.md)** — 20 most recent decisions on the default branch
- **[docs/file-structure.md](docs/file-structure.md)** — current project structure

### Additional Reference

- **[docs/decisions-archive.md](docs/decisions-archive.md)** — historical decisions (searchable reference)
- **docs/decisions-branches/** — per-branch decision logs written during feature work
- **README.md** at the repo root — project overview, if present
<!-- logmind-end -->

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
