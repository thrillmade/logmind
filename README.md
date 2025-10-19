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
2. **Init** creates `docs/` folder with template files
3. **Log** a decision - appends, archives old ones (keeps 20 recent), regenerates tree, commits, and pushes
4. **Context** AI agents read the 20 most recent decisions and current file structure

## Why logmind?

- **Simple:** Two markdown files (recent + archive), no database
- **Focused:** Only 20 most recent decisions for relevant AI context
- **Git-native:** Every decision is a commit, git history is your audit trail
- **AI-friendly:** Recent decisions + file structure = complete context
- **Automatic:** Commits and pushes on every log

See [docs/plan.md](docs/plan.md) for complete architecture and roadmap.