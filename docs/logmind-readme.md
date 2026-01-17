# logmind

AI decision logging system for development projects.

## Overview

logmind automatically tracks decisions made during AI-assisted development, maintains up-to-date documentation, and provides rich context for AI agents.

**Key concept:** Install once, init anywhere, log everything.

## Quick Start

```bash
# Install (once)
pipx install logmind

# Initialize in your project
cd your-project
logmind init

# Log decisions - Python API
from logmind import log
log("Chose FastAPI over Flask",
    reasoning="Need async/await for WebSocket handling")

# Or use CLI
logmind log "Use PostgreSQL for database" \
  -r "Need ACID compliance" \
  -a "MongoDB" -a "SQLite"

# View and search decisions
logmind show
logmind search "postgres"

# Manage AI agents
logmind agents list
logmind agents add windsurf

# Upgrade logmind
logmind update

# Auto-log with decorators
from logmind import log_decision, log_choice

@log_decision(
    decision="Authenticate user with {method}",
    reasoning="Security checkpoint"
)
def authenticate(method="oauth"):
    # Your auth code
    return True

@log_choice(
    choices={
        "redis": "Use Redis for caching",
        "memory": "Use in-memory caching",
    }
)
def select_cache():
    return "redis" if is_production() else "memory"
```

## Contributing / Development Setup

Working on logmind itself? Set it up like any CLI tool:

```bash
# Clone the repo
git clone https://github.com/thrillmot/logmind.git
cd logmind

# Install globally in editable mode (like npm, git, docker)
pipx install -e .

# Now just use it!
logmind log "Add new feature" -r "Reasoning here"
logmind show
logmind search "keyword"

# Run tests
python3 -m venv venv
source venv/bin/activate
pip install -e ".[dev]"
pytest
```

**Why pipx?** logmind is a CLI tool, not a library. It should be globally available like `git` or `npm`.

## Documentation

- **[Plan & Architecture](docs/plan.md)** - Vision, approach, and technical details
- **[AI Agent Files](docs/ai-agent-files.md)** - How logmind integrates with AI instruction files
- **[First Decision Example](docs/first-decision-example.md)** - What the initial decision looks like
- **Development Status** - Phase 3 Complete (decorators + agents + auto-update)

## How It Works

1. **Install** logmind as a package
2. **Init** creates `docs/` folder and inserts instructions into `CLAUDE.md` (preserving existing content)
3. **Log** a decision - appends, archives old ones (keeps 20 recent), regenerates tree, commits, and pushes
4. **Context** AI agents read the 20 most recent decisions and current file structure

## Why logmind?

- **Simple:** Two markdown files (recent + archive), no database
- **Focused:** Only 20 most recent decisions for relevant AI context
- **Git-native:** Every decision is a commit, git history is your audit trail
- **AI-friendly:** Recent decisions + file structure = complete context
- **Automatic:** Commits and pushes on every log

See [docs/plan.md](docs/plan.md) for complete architecture and roadmap.