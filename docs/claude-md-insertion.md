# CLAUDE.md Insertion Strategy

## Goal

When `logmind init` runs, it should insert logmind usage instructions into existing AI agent instruction files (like CLAUDE.md) without overwriting the user's existing content.

## Files to Check

In priority order:
1. `CLAUDE.md` (Claude Code)
2. `.cursorrules` (Cursor)
3. `.github/copilot-instructions.md` (GitHub Copilot)

## Insertion Strategy

### If CLAUDE.md exists:

Check if it already contains logmind instructions by searching for a marker:
```markdown
<!-- logmind-start -->
```

**If marker exists:** Skip (already initialized)

**If marker doesn't exist:** Insert section at the top, after the title:

```markdown
# CLAUDE.md

This file provides guidance to Claude Code...

<!-- logmind-start -->
## Decision Logging (logmind)

This project uses [logmind](https://github.com/user/logmind) for decision tracking.

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

[Rest of existing CLAUDE.md content]
```

### If CLAUDE.md doesn't exist:

Create it with full template:

```markdown
# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

<!-- logmind-start -->
## Decision Logging (logmind)

This project uses [logmind](https://github.com/user/logmind) for decision tracking.

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

## Project Overview

[To be filled in by the user]

## Development Commands

[To be filled in by the user]
```

## Implementation Details

### File Detection
```python
def find_ai_instruction_files():
    """Returns list of (path, file_type) tuples"""
    files = []

    if Path("CLAUDE.md").exists():
        files.append(("CLAUDE.md", "claude"))

    if Path(".cursorrules").exists():
        files.append((".cursorrules", "cursor"))

    if Path(".github/copilot-instructions.md").exists():
        files.append((".github/copilot-instructions.md", "copilot"))

    return files
```

### Insertion Logic
```python
def insert_logmind_instructions(file_path, file_type):
    """Insert logmind section into AI instruction file"""

    # Read existing content
    if Path(file_path).exists():
        content = Path(file_path).read_text()

        # Check if already initialized
        if "<!-- logmind-start -->" in content:
            print(f"✓ {file_path} already has logmind instructions")
            return False

        # Find insertion point (after first heading)
        lines = content.split("\n")
        insert_index = 0
        for i, line in enumerate(lines):
            if line.startswith("# "):
                insert_index = i + 1
                # Skip any blank lines after title
                while insert_index < len(lines) and not lines[insert_index].strip():
                    insert_index += 1
                break

        # Insert instructions
        instructions = get_logmind_section(file_type)
        lines.insert(insert_index, instructions)

        Path(file_path).write_text("\n".join(lines))
        print(f"✓ Added logmind instructions to {file_path}")
        return True

    else:
        # Create new file
        template = get_full_template(file_type)
        Path(file_path).write_text(template)
        print(f"✓ Created {file_path} with logmind instructions")
        return True
```

### File Type Adaptations

Different AI tools may need slightly different instructions:

**Claude Code (CLAUDE.md):**
- Standard markdown format
- Python code examples

**Cursor (.cursorrules):**
- May be plain text or markdown
- Adapt language if needed

**GitHub Copilot (.github/copilot-instructions.md):**
- Standard markdown
- May need different phrasing

## Edge Cases

1. **File exists but is empty:** Treat as new file
2. **File has no headings:** Insert at very top
3. **Multiple AI instruction files exist:** Insert into all of them
4. **File is not writable:** Show error, continue with other files
5. **Already initialized:** Skip silently (idempotent)

## User Experience

```bash
$ logmind init

Creating logmind structure...
✓ Created docs/decisions.md
✓ Created docs/decisions-archive.md
✓ Created docs/file-structure.md
✓ Added logmind instructions to CLAUDE.md
✓ Logged first decision: "Initialize logmind decision tracking"
✓ Committed changes: "logmind: Initialize decision tracking"

logmind initialized! Start logging decisions with:
  from logmind import log
  log("Your decision here")
```

If already initialized:
```bash
$ logmind init

✓ docs/ already exists
✓ CLAUDE.md already has logmind instructions

logmind is already initialized in this project.
```
