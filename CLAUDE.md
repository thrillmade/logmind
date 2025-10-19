# CLAUDE.md


<!-- logmind-start -->
## Decision Logging (logmind)

This project uses [logmind](https://github.com/logmind/logmind) for decision tracking.

### For AI Agents

When making significant decisions or writing important code:

```python
from logmind import log

log("Decision summary",
    reasoning="Why this approach",
    alternatives=["Option A", "Option B"],
    implications=["Impact 1", "Impact 2"])
```

### Context Files

- **[docs/decisions.md](docs/decisions.md)** - 20 most recent decisions
- **[docs/decisions-archive.md](docs/decisions-archive.md)** - Historical decisions
- **[docs/file-structure.md](docs/file-structure.md)** - Current project structure

Read these files to understand project history and architecture.
<!-- logmind-end -->

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**logmind** is an AI decision logging system that scaffolds documentation structure and tracks development decisions automatically.

**Key concept:** A pip-installable package that runs `logmind init` to create standardized docs, then serves as the decision logging engine.

## Essential Documentation

- **[docs/plan.md](docs/plan.md)** - Complete architecture, approach, and roadmap
- **[README.md](README.md)** - User-facing overview and quick start

## Development Commands

This is a Python package. Standard commands will be:

```bash
# Setup (when implemented)
pip install -e .

# Run tests (when implemented)
pytest

# Build package (when implemented)
python -m build
```

## Current Status

**Phase:** Planning and initial setup

See [docs/plan.md](docs/plan.md) for development phases and technical decisions.
