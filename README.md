# logmind

AI decision logging system for development projects.

## Overview

logmind automatically tracks decisions made during AI-assisted development, maintains up-to-date documentation, and provides rich context for AI agents.

**Key concept:** Install once, init anywhere, log everything.

## Quick Start

```bash
# Install
pip install logmind

# Initialize in your project
cd your-project
logmind init

# Log a decision (appends to file, commits, pushes)
from logmind import log

log("Chose FastAPI over Flask for async support",
    reasoning="Need async/await for WebSocket handling")
```

## Documentation

- **[Plan & Architecture](docs/plan.md)** - Vision, approach, and technical details
- **Development Status** - Currently in planning phase

## How It Works

1. **Install** logmind as a package
2. **Init** creates `docs/` folder with `decisions.md` and `file-structure.md`
3. **Log** a decision - it appends to the file, regenerates tree, commits, and pushes
4. **Context** AI agents read the history and current structure

## Why logmind?

- **Simple:** One append-only file, no database
- **Git-native:** Every decision is a commit, git history is your audit trail
- **AI-friendly:** Two files (decisions + structure) give complete context
- **Automatic:** Commits and pushes on every log

See [docs/plan.md](docs/plan.md) for complete architecture and roadmap.