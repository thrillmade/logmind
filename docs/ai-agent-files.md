# AI Agent Instruction Files

## Goal

`AGENTS.md` is the canonical instruction file. `logmind init` creates (or
refreshes) it with the logmind decision-logging block, then writes a
2-line **stub** for every enabled per-tool file (`CLAUDE.md`,
`.cursorrules`, ...) that just points back at `AGENTS.md`. The guidance
lives in one place; per-tool files never carry a second copy that can
drift out of sync.

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

Which agents get a stub is controlled by the `agents:` block in
`.logmind/config.yml` (`claude` and `cursor` default `true`; the rest
default `false`) or overridden per-run with `--agents` / `--all-agents`.
Codex's file *is* `AGENTS.md`, so it's always "configured" once init has
run — there's no separate stub to write.

## Insertion Strategy

### AGENTS.md (canonical)

`logmind init` inserts the logmind block between
`<!-- logmind-start -->` / `<!-- logmind-end -->` markers, versioned via
`<!-- logmind-block-version: v10-pointer -->` for the slim body `init`
installs by default (the full body is a generation behind at `v9`, and the
two flavours bump independently — read the current pair off the templates
with `grep -h logmind-block-version internal/templates/AGENTS.md*.template`
rather than trusting this line). If the marker is already
present, the block is left alone on a plain re-run (`logmind doctor --fix`
or a version bump refreshes a *stale* block in place); any other existing
content in `AGENTS.md` — `## Project Overview`, `## Development Commands`,
etc. — is preserved as-is.

### Per-tool files (stubs)

For every enabled agent, `logmind init` / `logmind agents add <name>`
writes (or overwrites) a 2-line stub:

```
<!-- logmind-stub: AI agent instructions for this project live in AGENTS.md -->
See [AGENTS.md](AGENTS.md) for project-specific AI agent instructions, including
the decision-logging requirement (logmind) and required reading.
```

If a per-tool file already exists with real hand-written content (not yet
a stub), `logmind agents add <name>` falls back to splicing a
`<!-- logmind-start -->` / `<!-- logmind-end -->` section into that file
in place (the legacy, pre-AGENTS.md insertion path) rather than clobbering
it. `logmind agents migrate` consolidates any such files into stubs.

### Context files

The required-reading list an agent should read before starting work,
pointed to from the `AGENTS.md` block:

- **[docs/timeline.md](timeline.md)** — auto-generated, main-canonical
  union of every decision across every branch; start here
- **`docs/timeline-archive.md`** — the history older than the 50 entries in
  `docs/timeline.md`, same format (both are written by one regeneration)
- **[docs/decisions-branches/](decisions-branches/)** — per-branch
  decision logs; the record itself, one file per branch (the default
  branch included), append-only and uncapped
- **[docs/file-structure.md](file-structure.md)** — current project tree

## CLI Commands

### View Agent Status

```bash
logmind agents list         # List all agents with status
```

Example output:
```
AI Agent Status:

  ✓ claude       CLAUDE.md                                (configured)
  ✓ cursor       .cursorrules                              (configured)
  ✗ copilot      .github/copilot-instructions.md          (not configured)
  ✗ windsurf     .windsurfrules                            (not configured)
  ✗ aider        CONVENTIONS.md                            (not configured)
  ✗ continue     .continuerules                            (not configured)
  ✗ cody         .sourcegraph/cody.json                    (not configured)
  ✗ zed          .zed/settings.json                        (not configured)
  ✗ amazonq      .amazonq/rules.md                         (not configured)
  ✗ cline        .clinerules                               (not configured)
  ✓ codex        AGENTS.md                                 (configured)

Supported agents: claude, cursor, copilot, windsurf, aider, continue, cody, zed, amazonq, cline, codex
```

### Add/Remove/Migrate Agents

```bash
logmind agents add cursor    # Write .cursorrules as a stub pointing to AGENTS.md
logmind agents add windsurf  # Write .windsurfrules as a stub pointing to AGENTS.md
logmind agents remove cursor # Remove .cursorrules
logmind agents update        # Reports stale logmind blocks + CI workflow pins (dry-run; add --apply to rewrite)
logmind agents migrate       # Consolidate any full per-agent files into stubs
```

### Initialize with Specific Agents

```bash
logmind init --agents claude,cursor    # Only these agents
logmind init --all-agents              # All supported agents
```

## Edge Cases

1. **File exists but is empty:** Treat as new file
2. **File has no headings:** Insert at very top (legacy splice path)
3. **File is not writable:** Show error, continue with other files
4. **Already initialized:** Refresh mode — templates/hooks refreshed in
   place, decision docs and `.logmind/config.yml` left untouched

## User Experience

### First initialization:

```
$ logmind init

Initializing logmind...

✓ Created docs/
✓ Created docs/file-structure.md
✓ Created docs/timeline.md
✓ Created docs/timeline-archive.md
✓ Created .logmind/config.yml
Created AGENTS.md (canonical agent instructions)
✓ Created CLAUDE.md
✓ Created .cursorrules
✓ Created .github/workflows/check-decisions.yml
✓ Created .github/workflows/check-doc-links.yml
✓ Created .github/workflows/logmind-self-update.yml
✓ Created .github/workflows/regen-timeline.yml
✓ Created .github/dependabot.yml
✓ Added logmind block to .gitignore
✓ Added logmind block to .gitattributes
✓ Installed Claude Code guard-commit hook (.claude/settings.json)
✓ Logged first decision: "Initialize logmind decision tracking"

logmind initialized successfully!

Start logging decisions with:
  logmind log "Your decision here" -r "why" -a "alternative" -i "implication"
```

### With specific agents:

```
$ logmind init --agents claude,cursor,windsurf

Initializing logmind...

✓ Created docs/
✓ Created docs/file-structure.md
✓ Created docs/timeline.md
✓ Created docs/timeline-archive.md
✓ Created .logmind/config.yml
Created AGENTS.md (canonical agent instructions)
✓ Created CLAUDE.md
✓ Created .cursorrules
✓ Created .windsurfrules
✓ Created .github/workflows/check-decisions.yml
✓ Created .github/workflows/check-doc-links.yml
✓ Created .github/workflows/logmind-self-update.yml
✓ Created .github/workflows/regen-timeline.yml
✓ Created .github/dependabot.yml
✓ Added logmind block to .gitignore
✓ Added logmind block to .gitattributes
✓ Installed Claude Code guard-commit hook (.claude/settings.json)
✓ Logged first decision: "Initialize logmind decision tracking"

logmind initialized successfully!
```

### Already initialized (refresh mode):

```
$ logmind init

Initializing logmind...

logmind is already initialized — running in refresh mode.

  All workflow templates already current.
✓ Refreshed .git/hooks/post-merge
✓ Refreshed .git/hooks/post-rewrite
✓ Refreshed .git/hooks/commit-msg

Done. docs/ and .logmind/ left untouched.
```
