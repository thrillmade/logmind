# logmind: Plan & Architecture

## Vision

logmind is a decision logging system for AI-assisted development. It
automatically tracks decisions made during development, maintains
up-to-date documentation, and provides rich context for AI agents — while
minimizing the tokens an agent has to burn reconstructing that context.

## Core Concept

**A single native binary, initialized per project:**
- Install once via Homebrew, the curl installer, or `thrillmade/setup-logmind`
  in CI
- Run `logmind init` in any project to scaffold `docs/` + AGENTS.md
- AI agents and developers use `logmind log` to record decisions as they
  make them
- The binary handles staging, committing, and keeping the derived docs
  (`docs/timeline.md`, `docs/file-structure.md`) in sync

Similar to `git init` or `terraform init`, `logmind init` creates a
standardized structure in any project, then gets out of the way. See
[AGENTS.md](../AGENTS.md) for the day-to-day usage contract agents follow.

## Architecture

logmind is a Go CLI (module `github.com/thrillmade/logmind`). Entry point
`cmd/logmind/main.go`; each concern lives in its own `internal/` package:

```
internal/
├── cli/           # cobra command wiring — one file per subcommand
├── config/        # .logmind/config.yml loading + defaults
├── decisions/     # parses decision-log markdown into typed entries
├── timeline/      # main-canonical ("newspaper") timeline assembly
├── repomap/       # Go + TypeScript/JS signature-skeleton extraction
├── tokens/        # deterministic token estimator (context --stats, etc.)
├── doctor/        # stack-status: version drift, hook drift, agent blocks
├── tree/          # deterministic, gitignore-aware file-structure tree
├── linkcheck/     # markdown link integrity (broken + orphaned links)
├── hooks/         # post-merge / post-rewrite git hook bodies
├── guardcommit/   # shared decision engine: should this git commit be blocked for bypassing `logmind log`?
├── claudehook/    # installs/inspects the Claude Code PreToolUse guard entry in .claude/settings.json
├── gitattr/       # .gitattributes block + merge-driver git-config
├── gitcli/        # thin exec.Command wrapper over git porcelain
├── inserter/      # AGENTS.md / per-agent-stub / dependabot scaffolding
├── agents/        # registry of supported AI agent instruction files
├── templates/     # embedded canonical templates (embed.FS)
├── skill/         # `logmind skill` — skill authoring/test/bench/audit
├── clierr/        # shared error sentinels crossing package boundaries (skill ↔ cli)
└── version/       # build-time Version / SpecVersion constants
```

### Project structure (after `logmind init`)

```
my-ai-project/
├── AGENTS.md                  # canonical agent instructions
├── CLAUDE.md, .cursorrules...  # 2-line stubs pointing to AGENTS.md
├── .logmind/
│   └── config.yml             # project configuration
├── docs/
│   ├── decisions.md           # 20 most recent decisions (default branch)
│   ├── decisions-branches/    # per-feature-branch decision logs
│   ├── decisions-archive.md   # older decisions, chronological
│   ├── timeline.md            # auto-generated cross-branch overview
│   └── file-structure.md      # auto-generated project tree
└── [existing project files]
```

## Core Features

### 1. Decision logging

`logmind log "<summary>" -r "<why>" -a "<alternative>" -i "<implication>"`
writes the decision, stages it plus its companion derived docs, and
commits — one command, no separate `git add` / `git commit`. Branch-aware:
entries on the default branch go to `docs/decisions.md`; entries on a
feature branch go to `docs/decisions-branches/<sanitized-branch>.md`. On
PR merge, CI appends a one-line pointer from the branch file into
`docs/decisions.md`.

### 2. The timeline — main-canonical, unconditionally

As of v2.0.0 there is exactly one timeline assembly model:
**main-canonical**, a deterministic, source-derived union of every
decision file in the repo (SPEC §1.6.4). The older branch-divergent
full-regen renderer has been removed entirely — there is no config knob
to opt back into it, and `logmind timeline --full` is accepted but
ignored. The agent-authored branch headline, the per-log timeline marker,
and `doctor --fix`'s marker backfill are always on.

### 3. The token-killer surface

logmind's second thesis, alongside decision capture, is that a
self-describing repo should cost an agent as few tokens as possible to
orient in:

- **`logmind context [--stats]`** — the one-read agent cold-start
  payload: `docs/file-structure.md` (the "what") + `docs/timeline.md`
  (the "why"), framed for prompt-cache reuse. `--stats` prints a token
  receipt. `context.repomap: true` in config folds in the repomap below.
- **`logmind repomap`** — deterministic Go and TypeScript/JS signature
  skeletons (no full file bodies), with importance ranking and a
  `--map-tokens` budget-packing flag, so an agent can see a codebase's
  shape for a fraction of the tokens a raw file listing costs.
- **`LOGMIND_QUIET`** — a single `ok key=value` line per command instead
  of human-formatted prose, for scripted/agent invocations.
- All of the above are grounded in the shared thrillmade/protocol SPEC
  §14 token-efficiency contract, so `context`/`repomap` output stays
  consistent across the SkDD toolchain (logmind, clud-bug, agent-skills).

### 4. Reading and auditing decisions

`logmind show` (recent decisions on the current branch), `logmind search
"<keyword>"` (full-text across recent + archive), `logmind headline`
(sets/reads the agent-authored branch summary), and `logmind doctor
[--fix]` (stack-status: version drift, hook drift, missing markers —
`--fix` backfills what it safely can, including timeline markers and
`--file` targeting) round out the read/audit surface.

## Development History

### Python era (pre-v1.0, archived)

The original Python package (`pip install logmind`) shipped decision
logging, branch-aware storage, `show`/`search`, framework integrations
(LangChain, and a [`BaseIntegration` pattern for custom
frameworks](custom-integrations.md)), a pre-commit hook, decision
templates, an analytics dashboard, and multi-project aggregation. See
[docs/changelog-python.md](changelog-python.md) for the full history —
that surface is frozen and superseded by the Go rewrite below.

### v1.0.0 — Go rewrite (2026-06-03)

Byte-identical Go port of the frozen Python v0.6.16 behavior (the
port's hard contract). Ships as a native binary via Homebrew, a curl
installer, and the `thrillmade/setup-logmind` GitHub Action — no Python
runtime required. `docs/plan.md` (this file) and `AGENTS.md` describe the
Go-era architecture going forward.

### v1.1–v1.2 — distribution + self-heal hardening

`install.sh` fetch-latest mode, `setup-logmind` action adoption across
consumer repos, and a 3-layer markdown self-healing gate on `logmind log`
(interactive retry loop locally, deterministic CI comment / Anthropic
auto-fix in the `check-doc-links` workflow).

### v2.0.0

- **Shipped:** main-canonical is now the sole timeline model
  (branch-divergent removed, see Core Features above); the token-killer
  surface (`context`, `repomap` with ranking + `--map-tokens`,
  `LOGMIND_QUIET`, protocol §14); `show`, `search`, `headline`, and
  `doctor --fix` are all live.
- **Force-logmind-usage enforcement (SPEC §15):** the commit-msg hook
  (git layer) and the Claude Code PreToolUse hook (harness layer) block a
  substantive raw `git commit` that bypasses `logmind log`
  (`internal/guardcommit` + `internal/claudehook`); escape hatches are
  `[skip-logmind]`, `LOGMIND_ALLOW_GIT_COMMIT=1`, and
  `git.enforce_commits: false`. The git layer signals a block via exit
  65 and fails open on any other error.
- **Canonical spec-file contract (SPEC §16):** an optional,
  forward-looking spec document surfaced via `logmind context`, defined
  SkDD-wide (shared across logmind, clud-bug, and agent-skills) rather
  than logmind-specific. logmind's own spec is the pointer doc
  [docs/spec.md](spec.md) (`context.spec_file: docs/spec.md`).
- **The pulse (SPEC §3.1.1):** `logmind log` prints stderr advisories
  after every commit — stale components (per `logmind doctor`) and spec
  staleness (the spec file hasn't been touched in 20+ decisions).
  Advisory only; it never blocks the commit that already landed.

## Technical Decisions

### Language: Go
- Single static binary — no runtime dependency for end users
- Cross-platform distribution (brew, curl, `setup-logmind` action,
  `go install`)
- Superseded the original Python implementation at v1.0.0; the port's
  hard contract was byte-identical output against the frozen Python
  v0.6.16 baseline

### Storage: markdown, no database
- `docs/decisions.md` (recent, default branch) + `docs/decisions-branches/`
  (in-flight feature work) + `docs/decisions-archive.md` (historical)
- `docs/timeline.md` and `docs/file-structure.md` are derived, regenerated
  automatically — never hand-edited
- Human-readable, AI-friendly, greppable, works offline, git provides full
  versioning

### Timeline: main-canonical only
- One deterministic, source-derived assembly (SPEC §1.6.4) replaces the
  old dual-mode (branch-divergent vs. main-canonical) design — less
  config surface, no risk of the two renderers drifting apart

### Git as audit trail
- Every decision is one commit; git history is the timeline's ground
  truth; diffs show what changed when

## Success Metrics

- AI agents can orient in a project in one read (`logmind context`)
  instead of reconstructing state from `git log` / `ls -R` / `grep`
- Decision history stays cheap to read as it grows (20-recent + archive +
  per-branch split) so agent context windows aren't overwhelmed
- Documentation (`timeline.md`, `file-structure.md`) stays in sync with
  code automatically, with no hand-maintained step
- Decision history helps with onboarding, auditing, and cross-agent
  hand-off
