---
name: logmind
description: |
  MUST be loaded for any task in a project that uses logmind (detect by:
  .logmind/config.yml at repo root, or AGENTS.md / CLAUDE.md mentioning
  logmind, or docs/decisions-branches/ present). Use BEFORE writing >20 lines of
  new code, BEFORE choosing between alternatives, BEFORE adding a
  dependency, BEFORE modifying existing functionality, BEFORE making any
  security or performance trade-off, BEFORE renaming or moving any
  significant module. Logging is part of the work, not after it. Also use
  to read prior decisions before starting any task in such a project so
  you don't re-litigate something already decided.
---

# logmind: log decisions as you work

If the current project contains `.logmind/config.yml` or its `AGENTS.md`
mentions logmind, this skill applies. Log a decision **before** writing
non-trivial code, not after.

## When to log

Log a decision whenever you:

- Make an architectural or design choice
- Choose between alternative approaches
- Write significant new code (> 20 lines or a new module)
- Modify existing functionality in a non-obvious way
- Add or remove a dependency
- Make a security or performance decision

When in doubt, log it. A short decision entry is cheap; recovering missing
context months later is not.

## How to log

```bash
logmind log "Use PostgreSQL for primary database" \
  -r "Need ACID compliance and complex joins" \
  -a "MongoDB" -a "SQLite" \
  -i "Connection pooling required" \
  -i "Schema migrations needed"
```

`logmind log` *is* the commit primitive: default `--stage all` sweeps the
whole working tree into one commit (the decision and the code that
prompted it, together). Pass `--stage scoped` to commit only the decision
file + its companion docs when you have unrelated WIP.

## Branch-aware logging

Every entry is written to `docs/decisions-branches/<sanitized-branch>.md`
for the branch it was made on — the default branch included, where the
file is `main.md`. One rule, no exception: the default branch is a branch
like any other. You do not need to manage this routing — `logmind log`
does it automatically.

`docs/decisions.md` is a legacy source. Where it still exists it is read,
never written, except on the three paths where the router cannot resolve a
branch name to route by: `decisions.branch_aware: false`; `git` reporting
this is not a repository, which also fires when the `git` binary itself is
unreachable — even inside a real repo, on a real branch; and a detached
HEAD. A repo with no commits yet is not one of them — `git symbolic-ref
--short HEAD` succeeds before the first commit, so a fresh `git init` routes
to `main.md` like any other branch.

## Reading prior context

Before starting non-trivial work, read in order:

1. **`docs/timeline.md`** — the main-canonical, source-derived union of
   every decision across every branch; start here. Its older half
   continues in **`docs/timeline-archive.md`**.
2. **`docs/decisions-branches/<your-branch>.md`** (if present) — decisions
   made earlier on the branch you are on, in full detail. On the default
   branch that file is `docs/decisions-branches/main.md`.
3. **`docs/file-structure.md`** — current project tree.
4. The project's **spec file**, if configured (`context.spec_file` in
   `.logmind/config.yml`, typically `docs/spec.md`) — the forward-looking
   contract to build toward, not just the history behind you.

`logmind context` bundles the file structure + timeline (and the spec
file, when configured) into one cache-friendly read — prefer it over
piecing this together by hand.

```bash
logmind show               # recent decisions on the current branch
logmind show --all         # include every other branch's decision file
logmind search "postgres"  # full-text across every decision file that exists
```

As an agent, set `LOGMIND_QUIET=1` for terse, chainable machine output on
the read/emit verbs (`doctor`, `file-structure`, `guard-commit`,
`headline`, `log`, `repomap`, `search`, `show`, `timeline`): each
suppresses progress chatter and prints a single `ok <key=value>` line.
Other verbs currently ignore this flag.

## Enforcement: raw `git commit` is blocked

A substantive commit that bypasses `logmind log` is **blocked**, not just
discouraged. Two layers enforce it: the git `commit-msg` hook, and (inside
Claude Code) a PreToolUse hook that intercepts the `git commit` Bash call
before it runs. Use `logmind log` in place of `git add` + `git commit` +
`git push`.

Escape hatches, for genuinely no-decision commits (typo fixes, dependency
bumps):
- Add `[skip-logmind]` to the commit subject.
- Set `LOGMIND_ALLOW_GIT_COMMIT=1` for one command.
- `git.enforce_commits: false` in `.logmind/config.yml` disables
  enforcement for the whole repo.

## Branch summaries (headline)

Set a one-sentence, plain-English summary of what the **whole branch**
does — the canonical timeline shows this line for the branch, and it's the
first thing the next agent reads:

```bash
logmind headline "Add JWT session auth with refresh-token rotation"

# or bundle it into a decision commit:
logmind log "Wire refresh-token rotation" -r "..." -H "Add JWT session auth with refresh-token rotation"
```

A no-op on the default branch. `logmind doctor --fix` backfills a headline
for any branch file that's missing one.

## The pulse: read the advisories after `logmind log`

After a successful commit, `logmind log` may print advisories to stderr —
a stale component (workflow/hook/AGENTS.md drift; run `logmind doctor
--fix`) or spec staleness (the spec file hasn't been touched in 20+
decisions; worth a review). These never block the commit that already
landed — act on them anyway.

## Setup (one-time, per project)

If the project doesn't yet have logmind:

```bash
brew install thrillmade/tap/logmind   # or: curl -fsSL https://logmind.dev/install.sh | bash
logmind init                          # scaffolds docs/, AGENTS.md, GH Actions, hooks
logmind doctor                        # confirm a clean install
```

brew/curl installs the latest tagged release (v1.2.0), which lacks
`context`, `show`, `search`, `headline`, and `guard-commit` (PreToolUse
harness) enforcement — those verbs require building from source/the dev
branch (`2.0.0-dev`).

## Don'ts

- **Don't run `git add` + `git commit` directly** for changes that carry a
  decision — the enforcement hooks above will block it anyway. `logmind
  log` writes the decision file, stages the working tree, and creates the
  commit in one step.
- Don't log every tiny edit. The 20-line rule is a guideline; use judgement.
- Don't write the decision after the fact in past tense for trivial code.
- Don't reword a decision someone else already logged — link or extend it.
- Don't bypass the auto-commit (`--no-commit`) unless you know the project's
  branch protection requires it.
