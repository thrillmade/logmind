# logmind: Plan & Architecture

## Vision

logmind is a decision logging system for AI-assisted development. It automatically tracks decisions made during development, maintains up-to-date documentation, and provides rich context for AI agents.

## Core Concept

**Package with initialization:**
- Install via `pip install logmind`
- Run `logmind init` in any project to scaffold structure
- AI agents and developers use logmind to log decisions and maintain context
- Package handles all the complexity of documentation management

## Approach: Package with Init Command

Similar to `git init`, `npm init`, or `terraform init`, logmind creates a standardized structure in any project.

### Why This Approach?

1. **Works everywhere**: New projects, existing projects, any tech stack
2. **Centralized updates**: Bug fixes and features ship via package updates
3. **Standardized structure**: All logmind projects have consistent layouts
4. **Low friction**: Single command to get started
5. **Portable**: Works with any AI framework or development tool

### Installation & Usage

```bash
# Install
pip install logmind

# Initialize in a project
cd my-ai-project
logmind init  # Creates docs/, inserts instructions into CLAUDE.md

# Log a decision (appends, commits, pushes)
from logmind import log

log("Chose FastAPI over Flask for async support",
    reasoning="Need async/await for WebSocket handling")
```

**What `logmind init` does:**
1. Creates `docs/` folder with template files
2. Finds or creates `CLAUDE.md` (or other AI instruction files)
3. Inserts logmind usage instructions **without overwriting** existing content
4. Logs the first decision: "Initialize logmind decision tracking"
5. Commits all changes with message: "logmind: Initialize decision tracking"

## Architecture

### Package Structure

```
logmind/
├── cli.py              # CLI commands (init, log, etc.)
├── core/
│   ├── logger.py       # Decision logging engine
│   ├── git_handler.py  # Auto-commit and push
│   ├── tree_gen.py     # File structure generator
│   └── inserter.py     # CLAUDE.md instruction inserter
├── templates/          # Files created during init
│   ├── logmind-section.md      # Section to insert into CLAUDE.md
│   ├── CLAUDE.md.template      # Full template if creating new
│   ├── decisions.md.template
│   └── file-structure.md.template
└── integrations/       # Hooks for AI frameworks (future)
```

### Project Structure (After `logmind init`)

```
my-ai-project/
├── CLAUDE.md                  # AI agent instructions
├── docs/
│   ├── decisions.md           # 20 most recent decisions
│   ├── decisions-archive.md   # Older decisions (chronological)
│   └── file-structure.md      # Auto-generated tree output
└── [existing project files]
```

**Key files:**
- **decisions.md**: Only the 20 most recent decisions (keeps AI context focused)
- **decisions-archive.md**: Full history of older decisions (21+)
- **file-structure.md**: Auto-generated using `tree` command, updated on each log
- **CLAUDE.md**: References all docs, but AI reads recent decisions first

## Core Features

### 1. Decision Logging

Log decisions which automatically:
1. Appends to `docs/decisions.md`
2. If > 20 decisions, moves oldest to `docs/decisions-archive.md`
3. Updates `docs/file-structure.md` using `tree`
4. Commits all changed files to git
5. Pushes to remote

```python
from logmind import log

log(
    decision="Use PostgreSQL for primary database",
    reasoning="Need ACID compliance and complex queries",
    alternatives=["MongoDB", "SQLite"],
    implications=["Need to set up connection pooling", "Schema migrations required"]
)
```

**What happens:**
```bash
# Appends to docs/decisions.md:
## 2025-10-17 14:32 - Use PostgreSQL for primary database
**Reasoning:** Need ACID compliance and complex queries
**Alternatives considered:** MongoDB, SQLite
**Implications:** Need to set up connection pooling, Schema migrations required

# If decisions.md has > 20 entries:
# - Move oldest decision to top of decisions-archive.md
# - Keep only 20 most recent in decisions.md

# Regenerates docs/file-structure.md with `tree` output

# Auto-commits:
git add docs/decisions.md docs/decisions-archive.md docs/file-structure.md
git commit -m "logmind: Use PostgreSQL for primary database"
git push
```

### 2. File Structure Tracking

Every decision log automatically regenerates the file structure:

```bash
# Runs: tree -I '__pycache__|.git|node_modules|venv' > docs/file-structure.md
```

Gives AI agents fresh context about project layout.

### 3. Git Integration

**Automatic workflow:**
- Every `log()` creates a git commit
- Commit message: `"logmind: [decision summary]"`
- Auto-pushes to keep remote in sync
- Git history = audit trail of all decisions

### 4. Simple Context for AI

AI agents read files for full project context:
- `docs/decisions.md` - **20 most recent decisions** (focused, current reasoning)
- `docs/decisions-archive.md` - Historical decisions (reference when needed)
- `docs/file-structure.md` - What exists in the project
- `CLAUDE.md` - Links to all docs

**Why limit to 20 recent decisions?**
- Keeps AI context focused on current state
- Prevents overwhelming context window
- Recent decisions most relevant to ongoing work
- Full history still available in archive + git history

## Development Phases

### Phase 1: Core Package (MVP) ✅ COMPLETE
- [x] Package structure and setup.py/pyproject.toml
- [x] `logmind init` command that:
  - [x] Creates docs/ folder and template files (decisions.md, decisions-archive.md, file-structure.md)
  - [x] Detects AI instruction files (CLAUDE.md, .cursorrules, .github/copilot-instructions.md)
  - [x] Inserts logmind section into these files without overwriting existing content
  - [x] Creates CLAUDE.md if it doesn't exist
  - [x] Logs first decision: "Initialize logmind decision tracking"
  - [x] Commits all changes: "logmind: Initialize decision tracking"
- [x] `logmind.log()` function that:
  - [x] Appends to docs/decisions.md
  - [x] Archives oldest decision if > 20 entries (moves to decisions-archive.md)
  - [x] Regenerates docs/file-structure.md using `tree`
  - [x] Git commits all changed files
  - [x] Pushes to remote
- [x] Basic CLI: `logmind log "decision"` from command line
- [x] Comprehensive test suite (65 tests, 100% passing)

### Phase 2: Enhanced Features (50% complete)
- [ ] Configuration options (git auto-push on/off, custom commit messages)
- [x] CLI command to view recent decisions: `logmind show`
- [ ] Search decisions: `logmind search "postgres"`
- [x] Git integration checks (warn if not in git repo)

### Phase 3: AI Integrations (Optional)
- [ ] Decorators for automatic logging
- [ ] Framework-specific plugins (LangChain, AutoGen)
- [ ] Hooks for automatic decision detection

### Phase 4: Advanced Features (Future)
- [ ] Decision templates for common patterns
- [ ] Analytics on decision frequency
- [ ] Multi-project decision aggregation

## Technical Decisions

### Language: Python
- Most AI frameworks use Python
- Rich ecosystem for CLI tools (Click, Typer)
- Easy packaging and distribution
- Subprocess module for git and tree commands

### Storage: Two Markdown Files
- **No database** - decisions.md (recent) + decisions-archive.md (historical)
- Recent file capped at 20 entries for focused AI context
- Git provides full versioning and history
- Human-readable, AI-friendly
- Works offline
- Easy to search with grep/ripgrep
- Archive preserves complete chronological history

### Git as Audit Trail
- Every decision = one commit
- Git history provides timeline
- Diffs show what changed when
- Remote backup via push
- Collaboration-ready

### File Structure via `tree`
- Standard Unix tool (available everywhere)
- Simple text output
- Perfect for AI context
- Updated automatically on each log
- Ignores common directories (.git, node_modules, venv, __pycache__)

## Success Metrics

- AI agents can navigate projects faster with logmind context
- Developers spend less time explaining architecture to AI
- Decision history helps with onboarding and auditing
- Documentation stays in sync with code automatically
