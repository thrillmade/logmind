# AI Agent Instruction Files

## Goal

When `logmind init` runs, it inserts logmind usage instructions into existing AI agent instruction files without overwriting existing content.

## Supported AI Agents

logmind supports the following AI coding assistants:

| Agent | Instruction File | Status |
|-------|------------------|--------|
| Claude Code | `CLAUDE.md` | Supported |
| Cursor | `.cursorrules` | Supported |
| GitHub Copilot | `.github/copilot-instructions.md` | Supported |
| Windsurf | `.windsurfrules` | Supported |
| Aider | `CONVENTIONS.md` | Supported |
| Continue | `.continuerules` | Supported |
| Sourcegraph Cody | `.sourcegraph/cody.json` | Supported |
| Zed AI | `.zed/settings.json` | Supported |
| Amazon Q | `.amazonq/rules.md` | Supported |
| Cline | `.clinerules` | Supported |
| OpenAI Codex | `AGENTS.md` | Supported |

## Insertion Strategy

### If an AI instruction file exists:

Check if it already contains logmind instructions by searching for a marker:
```markdown
<!-- logmind-start -->
```

**If marker exists:** Skip (already initialized)

**If marker doesn't exist:** Insert section after the title/first heading:

```markdown
# CLAUDE.md

This file provides guidance to Claude Code...

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

- **[docs/decisions.md](decisions.md)** - 20 most recent decisions
- **[docs/decisions-archive.md](decisions-archive.md)** - Historical decisions
- **[docs/file-structure.md](file-structure.md)** - Current project structure

Read these files to understand project history and architecture.
<!-- logmind-end -->

[Rest of existing content]
```

### If no AI instruction file exists:

Create `CLAUDE.md` with the full template (Claude Code is the default).

## CLI Commands

### View Agent Status

```bash
logmind agents              # List all agents with status
```

Example output:
```
AI Agent Status:
  ✓ claude      CLAUDE.md (configured)
  ✓ cursor      .cursorrules (configured)
  ✗ copilot     .github/copilot-instructions.md (not configured)
  ✗ windsurf    .windsurfrules (not configured)
  ...
```

### Add/Remove Agents

```bash
logmind agents add cursor   # Create .cursorrules with logmind section
logmind agents add windsurf # Create .windsurfrules with logmind section
logmind agents remove cursor # Remove .cursorrules
```

### Initialize with Specific Agents

```bash
logmind init --agents claude,cursor    # Only these agents
logmind init --all-agents              # All supported agents
```

## Edge Cases

1. **File exists but is empty:** Treat as new file
2. **File has no headings:** Insert at very top
3. **Multiple AI instruction files exist:** Insert into all of them
4. **File is not writable:** Show error, continue with other files
5. **Already initialized:** Skip silently (idempotent)

## User Experience

### First initialization:
```bash
$ logmind init

Initializing logmind...
✓ Created docs/decisions.md
✓ Created docs/decisions-archive.md
✓ Created docs/file-structure.md
✓ Added logmind instructions to CLAUDE.md
✓ Logged first decision: "Initialize logmind decision tracking"
✓ Committed changes: "logmind: Initialize decision tracking"

logmind initialized successfully!
```

### With specific agents:
```bash
$ logmind init --agents claude,cursor,windsurf

Initializing logmind...
✓ Created docs/decisions.md
✓ Created docs/decisions-archive.md
✓ Created docs/file-structure.md
✓ Added logmind instructions to CLAUDE.md
✓ Created .cursorrules with logmind instructions
✓ Created .windsurfrules with logmind instructions
✓ Logged first decision: "Initialize logmind decision tracking"
✓ Committed changes: "logmind: Initialize decision tracking"

logmind initialized successfully!
```

### Already initialized:
```bash
$ logmind init

✓ docs/ already exists
✓ CLAUDE.md already has logmind instructions

logmind is already initialized in this project.
```
