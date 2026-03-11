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
logmind init  # Creates docs/, inserts instructions into AI agent files

# Log a decision (appends, commits, pushes)
from logmind import log

log("Chose FastAPI over Flask for async support",
    reasoning="Need async/await for WebSocket handling")
```

**What `logmind init` does:**
1. Creates `docs/` folder with template files
2. Finds or creates AI instruction files (CLAUDE.md, .cursorrules, etc.)
3. Inserts logmind usage instructions **without overwriting** existing content
4. Logs the first decision: "Initialize logmind decision tracking"
5. Commits all changes with message: "logmind: Initialize decision tracking"

## Architecture

### Package Structure

```
logmind/
├── cli.py                    # CLI commands (init, log, show, search, stats, aggregate, ...)
├── core/
│   ├── logger.py             # Decision logging engine
│   ├── git_handler.py        # Auto-commit and push
│   ├── tree_gen.py           # File structure generator
│   ├── config.py             # Configuration management
│   ├── search.py             # Decision search functionality
│   ├── inserter.py           # AI instruction file inserter
│   ├── analytics.py          # Stats, monthly chart, keyword analysis
│   ├── aggregator.py         # Multi-project decision aggregation
│   └── decision_templates.py # Built-in templates for common decisions
├── decorators.py             # @log_decision, @log_choice
├── integrations/
│   ├── __init__.py           # Exports LangChainLogger
│   ├── base.py               # BaseIntegration for custom frameworks
│   └── langchain.py          # LangChain callback handler
└── templates/                # Files created during init
```

### Project Structure (After `logmind init`)

```
my-ai-project/
├── CLAUDE.md                  # AI agent instructions
├── .logmind/
│   └── config.yml             # Project configuration
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
- **AI instruction files**: CLAUDE.md, .cursorrules, etc. - links to all docs

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
- AI instruction files - Links to all docs

**Why limit to 20 recent decisions?**
- Keeps AI context focused on current state
- Prevents overwhelming context window
- Recent decisions most relevant to ongoing work
- Full history still available in archive + git history

## Development History

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

### Phase 2: Enhanced Features ✅ COMPLETE
- [x] Configuration options (git auto-push on/off, custom commit messages)
- [x] CLI command to view recent decisions: `logmind show`
- [x] Search decisions: `logmind search "postgres"`
- [x] Git integration checks (warn if not in git repo)
- [x] Test suite expanded to 95 tests (all passing)

### Phase 3: AI Integrations ✅ COMPLETE (Core Features)
- [x] Decorators for automatic logging (@log_decision, @log_choice)
  - [x] Template string support with {arg_name} placeholders
  - [x] Support for reasoning, alternatives, implications
  - [x] @log_choice for return-value-based decisions
  - [x] 15 comprehensive tests (110 total tests passing)
- [x] Universal AI agent support (11 agents)
  - [x] Agent registry with file paths and formats
  - [x] CLI commands: `agents list`, `agents add`, `agents remove`
  - [x] Init flags: `--agents`, `--all-agents`
  - [x] JSON support for Cody and Zed
  - [x] Test suite expanded to 160+ tests

### Phase 4: Advanced Features ✅ COMPLETE
- [x] LangChain callback integration (`LangChainLogger`)
- [x] `BaseIntegration` pattern for custom framework integrations
- [x] Custom integrations documentation (`docs/custom-integrations.md`)
- [x] `logmind check-decisions` — git pre-commit hook
- [x] `logmind install-hook` — one-command hook installation
- [x] Decision templates (`logmind log --template`, `logmind templates`)
- [x] Analytics dashboard (`logmind stats` with ASCII chart + keywords)
- [x] Multi-project aggregation (`logmind aggregate`)
- [x] Homebrew tap formula (`homebrew-tap/Formula/logmind.rb`)
- [x] Test suite expanded to 301 tests (all passing)

### Test Progression
- Phase 1: 65 tests
- Phase 2: 95 tests
- Phase 3: 178 tests
- Phase 4: 301 tests (all passing)

## Completed Features

See [README.md](../README.md) for usage documentation.

- **Phase 1**: Core package (`init`, `log`, `show`)
- **Phase 2**: Configuration system, search functionality
- **Phase 3**: Decorators (`@log_decision`, `@log_choice`) + Universal agent support (11 agents)
- **Phase 4**: Framework integrations, pre-commit hook, templates, analytics, aggregation, Homebrew

### Supported AI Agents

| Agent | Instruction File |
|-------|------------------|
| Claude Code | `CLAUDE.md` |
| Cursor | `.cursorrules` |
| GitHub Copilot | `.github/copilot-instructions.md` |
| Windsurf | `.windsurfrules` |
| Aider | `CONVENTIONS.md` |
| Continue | `.continuerules` |
| Sourcegraph Cody | `.sourcegraph/cody.json` |
| Zed AI | `.zed/settings.json` |
| Amazon Q | `.amazonq/rules.md` |
| Cline | `.clinerules` |
| OpenAI Codex | `AGENTS.md` |

### Agent CLI Commands

```bash
logmind agents list           # List all agents with status
logmind agents add <name>     # Add agent to project
logmind agents remove <name>  # Remove agent from project
logmind init --agents claude,cursor  # Init with specific agents
logmind init --all-agents            # Init with all agents
```

## Development Roadmap

> **All roadmap items below are complete. See Development History above for the full checklist.**

### Framework Integrations ✅

#### LangChain Callback Integration ✅

Automatically log decisions from LangChain agent runs.

```python
from logmind.integrations import LangChainLogger

chain = LLMChain(llm=llm, callbacks=[LangChainLogger()])
```

**Benefits:**
- **Zero-friction logging** - Decisions captured automatically
- **Agent transparency** - See what AI agents decided and why
- **Debugging** - Trace decision paths in complex chains
- **Audit trail** - Full history of AI-driven decisions

#### Base Integration Pattern ✅

Extensible pattern for custom AI framework integrations. See [docs/custom-integrations.md](custom-integrations.md).

**Benefits:**
- **Framework agnostic** - Works with any AI library
- **Community contributions** - Others can add integrations
- **Consistent interface** - Same logging API everywhere

#### Documentation for Custom Integrations ✅

Step-by-step guide at [docs/custom-integrations.md](custom-integrations.md).

**Benefits:**
- **Self-service** - Users can add their own integrations
- **Adoption** - Lower barrier to extending logmind
- **Quality** - Consistent patterns across integrations
- **Community growth** - Contributors have clear guidance

### Git Pre-commit Hook ✅

`logmind check-decisions` and `logmind install-hook`.

```bash
logmind install-hook          # one command to install
logmind check-decisions       # run manually or in CI
```

**Benefits:**
- **Enforcement** - Can't forget to log decisions
- **Code review aid** - Reviewers see decision context
- **Quality gate** - Ensures documentation stays current
- **Team accountability** - Everyone logs decisions

### Homebrew Tap ✅

Formula at `homebrew-tap/Formula/logmind.rb`.

```bash
brew tap thrillmot/logmind
brew install logmind
```

**Benefits:**
- **Accessibility** - No Python or pip required to install
- **Adoption** - Lower barrier for developers outside the Python ecosystem
- **Discoverability** - Listed in Homebrew search results

### Phase 4: Advanced Features ✅

#### Decision Templates ✅

7 built-in templates: `database`, `api`, `architecture`, `security`, `performance`, `library`, `deployment`.

```bash
logmind log --template database "Use PostgreSQL"
logmind templates   # list all
```

**Benefits:**
- **Consistency** - Standard format for common decisions
- **Speed** - Faster logging with pre-filled fields
- **Best practices** - Guide users to capture key info

#### Analytics Dashboard ✅

ASCII bar chart, velocity trend, top keywords.

```bash
logmind stats
logmind stats --months 6
```

**Benefits:**
- **Insights** - See what areas have most decisions
- **Trends** - Track decision velocity over time
- **Team patterns** - Identify knowledge silos

#### Multi-project Aggregation ✅

Unified feed or summary across multiple repos.

```bash
logmind aggregate ~/projects/api ~/projects/frontend
logmind aggregate --summary ~/work/*/
```

**Benefits:**
- **Organization view** - See all decisions across repos
- **Knowledge sharing** - Learn from other projects
- **Consistency** - Identify conflicting decisions

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
