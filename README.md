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

# Start logging decisions
from logmind import log_decision

log_decision("Chose FastAPI over Flask for async support")
```

## Documentation

- **[Plan & Architecture](docs/plan.md)** - Vision, approach, and technical details
- **Development Status** - Currently in planning phase

## How It Works

1. **Install** logmind as a package
2. **Init** creates standardized docs structure in your project
3. **Log** decisions automatically as you develop
4. **Context** AI agents get rich, up-to-date project understanding

## Why logmind?

- **For AI agents:** Consistent context across all projects
- **For developers:** Automatic decision history and documentation
- **For teams:** Shared understanding of architectural choices

See [docs/plan.md](docs/plan.md) for complete architecture and roadmap.