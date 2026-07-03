# AGENTS.md

This is the canonical instruction file for AI coding agents working in this
repository. Tools that understand `AGENTS.md` (Cursor, Codex, Windsurf, Claude
Code, Cline, Continue, Aider, Amazon Q, ...) read this file directly. Per-tool
files like `CLAUDE.md` or `.cursorrules` are stubs that point here so the
guidance lives in one place.

<!-- logmind-start -->
<!-- logmind-block-version: v3-slim -->
## Decision logging — see the `logmind` skill

This project uses [logmind](https://logmind.dev). The full procedure
(when to log, how to log, what counts as a decision, branch routing) lives
in the **`logmind` agent skill** which your runtime should auto-load.

If the skill isn't loaded for some reason, install it once:

```bash
npx skills add https://github.com/thrillmot/agent-skills --skill logmind
```

### Project-specific paths

- **[docs/timeline.md](docs/timeline.md)** — auto-generated chronological overview across all branches; start here.
- Recent decisions on the default branch: **[docs/decisions.md](docs/decisions.md)**
- Per-branch decisions (in-flight feature work): **docs/decisions-branches/**
- Archived decisions: **[docs/decisions-archive.md](docs/decisions-archive.md)**
- Project tree (regenerated on main-branch logs + post-PR-merge): **[docs/file-structure.md](docs/file-structure.md)**

### Quick reference

```bash
logmind log "decision summary" -r "why" -a "alternative" -i "implication"
logmind show               # recent decisions on the current branch
logmind search "keyword"   # full-text across recent + archive
```

**Use `logmind log` for the commit, not `git add` + `git commit`.** The
`log` command writes the decision file, stages the decision log + its
companion files, and creates the commit in one step. Use
`--stage all` to also stage the rest of the working tree.

**Read `docs/decisions.md` and the matching `docs/decisions-branches/<branch>.md` (if any) before starting any non-trivial task.** The team has likely already decided things you'd otherwise re-litigate.
<!-- logmind-end -->

## Release infrastructure

Cross-repo writes from this project's workflows (Homebrew cask bumps today;
future skill-catalog PRs and self-update fan-out) are signed by the
`thrillmade-orchestrator[bot]` GitHub App, NOT a personal PAT. See
[`docs/orchestrator-app.md`](docs/orchestrator-app.md) for the App spec,
permissions, ruleset bypass details, and rotation procedure.

## Project Overview

**logmind** is a decision-logging CLI (Go) — the commit primitive for this
repo and its consumers. It scaffolds `docs/` structure via `logmind init`,
records the "why" behind changes via `logmind log`, and keeps the derived
`docs/timeline.md` + `docs/file-structure.md` in sync as agent-readable
context. One leg of the thrillmade SkDD toolchain (with clud-bug = review,
agent-skills = catalog). Entry point `cmd/logmind/main.go`; code under
`internal/`. See [docs/plan.md](docs/plan.md) for architecture + roadmap.

## Development Commands

```bash
go build ./cmd/logmind      # build the binary
go test ./...               # run the full suite
go vet ./...                # static checks
gofmt -l .                  # list any unformatted files
```

<!-- clud-bug-start -->
<!-- clud-bug-block-version: v3-app -->
## clud-bug — Claude PR review

**PR reviews:** automated via the `clud-bug[bot]` GitHub App (installed at the thrillmade org). No per-repo workflow needed. See <https://github.com/thrillmade/clud-bug-app> for the App source and the `.claude/skills/.clud-bug.json` manifest for skill selection.

Collaboration rules — fix-push flow, skill structure, comment format — live in the bundled [`clud-bug-collaboration` skill](.claude/skills/clud-bug-collaboration/SKILL.md). Read that skill before pushing fixes addressing prior review threads.

For agent invocations of the `clud-bug` CLI, prefer `CLUD_BUG_QUIET=1` (or pass `--quiet`) — suppresses progress chatter and emits a single `ok <key-value>` summary line per command.
<!-- clud-bug-end -->
