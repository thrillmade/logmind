# AGENTS.md

This is the canonical instruction file for AI coding agents working in this
repository. Tools that understand `AGENTS.md` (Cursor, Codex, Windsurf, Claude
Code, Cline, Continue, Aider, Amazon Q, ...) read this file directly. Per-tool
files like `CLAUDE.md` or `.cursorrules` are stubs that point here so the
guidance lives in one place.

<!-- logmind-start -->
<!-- logmind-block-version: v9-pointer -->
## Decision logging — `logmind log` is REQUIRED for substantive commits

**`logmind log` replaces `git add` + `git commit` + `git push` for any change that carries a decision** — do not run those git commands directly.

> **DO NOT run raw `git add` / `git commit` / `git push` for substantive code changes.**
> The commit-msg hook and the Claude Code PreToolUse hook installed by
> `logmind init` / `logmind doctor --fix` **BLOCK** a substantive commit that
> skips `logmind log`. Genuinely no-decision commit? Add `[skip-logmind]` to
> the subject, or set `LOGMIND_ALLOW_GIT_COMMIT=1` for one command.
> `git.enforce_commits: false` disables enforcement per-repo. Typo / whitespace
> / dep-bump-only commits MAY use raw git.

```bash
logmind log "summary" -r "why" -a "alternative" -i "implication"
```

This project uses [logmind](https://logmind.dev). What counts as a decision, branch routing, `--stage scoped` for unrelated WIP, `logmind doctor`, and the required-reading list ([`docs/timeline.md`](docs/timeline.md), [`docs/file-structure.md`](docs/file-structure.md), `docs/decisions-branches/<branch>.md`) all live in the **`logmind` agent skill** at https://github.com/thrillmade/agent-skills/tree/main/skills/logmind.
<!-- logmind-end -->

## Release infrastructure

Cross-repo writes from this project's workflows (Homebrew cask bumps today;
future skill-catalog PRs and self-update fan-out) are signed by the
`skdd-steward[bot]` GitHub App (renamed from `thrillmade-orchestrator[bot]`;
same App ID, key, and installation), NOT a personal PAT. See
[`docs/orchestrator-app.md`](docs/orchestrator-app.md) for the App spec,
permissions, ruleset bypass details, and rotation procedure.

## Project Overview

**logmind** is a decision-logging CLI (Go) — the commit primitive for this
repo and its consumers. It scaffolds `docs/` structure via `logmind init`,
records the "why" behind changes via `logmind log`, and keeps the derived
`docs/timeline.md` + `docs/file-structure.md` in sync as agent-readable
context. One leg of the thrillmade SkDD toolchain (with clud-bug = review,
agent-skills = catalog). Entry point `cmd/logmind/main.go`; code under
`internal/`. See [docs/plan.md](docs/plan.md) for architecture, and
[docs/roadmap.md](docs/roadmap.md) for what ships next and in what order.

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
