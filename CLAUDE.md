# CLAUDE.md


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

**Python API (example):**
```python
from logmind import log

log("Decision summary",
    reasoning="Why this approach",
    alternatives=["Option A", "Option B"],
    implications=["Impact 1", "Impact 2"])
```

**CLI (example for shell commands):**
```bash
logmind log "Use PostgreSQL for database" \
  -r "Need ACID compliance" \
  -a "MongoDB" -a "SQLite" \
  -i "Need connection pooling"
```

### Viewing Past Decisions

```bash
# View recent decisions
logmind show

# View all decisions including archive
logmind show --all

# Search for specific topics
logmind search "postgres"
logmind search "API" --case-sensitive
logmind search "database" --no-archive
```

### Required Reading

**READ THESE FILES FIRST** before starting any work:
- **[docs/logmind-readme.md](docs/logmind-readme.md)** - Complete logmind documentation and usage guide (REQUIRED)
- **[docs/decisions.md](docs/decisions.md)** - 20 most recent decisions (REQUIRED)
- **[docs/file-structure.md](docs/file-structure.md)** - Current project structure (REQUIRED)

These files contain critical context about why the project is structured the way it is.

### Additional Reference

- **[docs/decisions-archive.md](docs/decisions-archive.md)** - Historical decisions (searchable reference, not required reading)

**Use `logmind search "keyword"` to find relevant past decisions quickly.**
<!-- logmind-end -->

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
