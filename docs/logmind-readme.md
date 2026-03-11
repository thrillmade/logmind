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

# Log with a built-in template (pre-fills reasoning, alternatives, implications)
logmind log --template database "Use PostgreSQL"
logmind templates   # list all available templates

# Analytics and stats
logmind stats
logmind stats --months 6

# Aggregate decisions across multiple projects
logmind aggregate ~/projects/api ~/projects/frontend
logmind aggregate --summary ~/work/*/

# Enforce decision logging with a pre-commit hook
logmind install-hook          # installs .git/hooks/pre-commit
logmind check-decisions       # run manually or in CI

# Manage AI agents
logmind agents list
logmind agents add windsurf

# View and modify configuration
logmind config list
logmind config get git.auto_push
logmind config set git.auto_push false

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

## Framework Integrations

```python
# LangChain — auto-log agent decisions (pip install logmind[langchain])
from logmind.integrations import LangChainLogger

chain = LLMChain(llm=llm, callbacks=[LangChainLogger()])

# Custom framework — subclass BaseIntegration
from logmind.integrations.base import BaseIntegration

class MyLogger(BaseIntegration):
    def on_decision(self, output):
        self.log(f"Chose: {output}", reasoning="My framework decided")
```

See [custom-integrations.md](custom-integrations.md) for patterns, examples, and publishing guide.

## Documentation

- **[Plan & Architecture](plan.md)** - Vision, approach, and technical details
- **[AI Agent Files](ai-agent-files.md)** - How logmind integrates with AI instruction files
- **[Custom Integrations](custom-integrations.md)** - Build integrations for any AI framework
- **[First Decision Example](first-decision-example.md)** - What the initial decision looks like
- **Development Status** - All phases complete ✅

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