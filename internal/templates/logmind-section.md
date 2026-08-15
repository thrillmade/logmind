
<!-- logmind-start -->
## Decision Logging (logmind)

**IMPORTANT:** This project uses [logmind](https://logmind.dev) for decision tracking.

### REQUIREMENT: AI Agents MUST Log All Decisions

**You MUST log a decision whenever you:**
- Make architectural or design choices
- Choose between alternative approaches
- Write significant new code (>20 lines)
- Modify existing functionality
- Add dependencies or libraries
- Make security or performance decisions

**BEFORE writing code, ask yourself: "Should this be logged?" If yes, log it IMMEDIATELY.**

### How to Log Decisions

**CLI (example for shell commands):**
```bash
logmind log "Use PostgreSQL for database" \
  -r "Need ACID compliance" \
  -a "MongoDB" -a "SQLite" \
  -i "Need connection pooling"
```

### Viewing Past Decisions

```bash
# View recent decisions
logmind show

# View all decisions including archive
logmind show --all

# Search for specific topics
logmind search "postgres"
logmind search "API" --case-sensitive
logmind search "database" --no-archive
```

### Required Reading

**READ THESE FILES FIRST** before starting any work:
- **[docs/timeline.md](docs/timeline.md)** - Main-canonical decision timeline across every branch (REQUIRED, start here)
- **`docs/decisions-branches/<your-branch>.md`** - decisions made earlier on the branch you're on (REQUIRED)
- **[docs/file-structure.md](docs/file-structure.md)** - Current project structure (REQUIRED)

These files contain critical context about why the project is structured the way it is.

**Enforcement:** a substantive raw `git commit` that bypasses `logmind log` is blocked (commit-msg + Claude Code PreToolUse hooks) — use `logmind log` instead.

### Additional Reference

- **`docs/timeline-archive.md`** - The history older than the 50 entries in docs/timeline.md, same format (not required reading)
- **docs/decisions-branches/** - Per-branch decision logs; the record itself, append-only and uncapped

**Use `logmind search "keyword"` to find relevant past decisions quickly.**
<!-- logmind-end -->
