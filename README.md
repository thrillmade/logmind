# logmind

[![CI](https://github.com/thrillmade/logmind/actions/workflows/test.yml/badge.svg)](https://github.com/thrillmade/logmind/actions/workflows/test.yml)
[![skills.sh](https://www.skills.sh/b/thrillmade/agent-skills/logmind)](https://www.skills.sh/thrillmade/agent-skills/logmind)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

AI decision logging for development projects — branch-aware by default.

## Why logmind

Codebases lose the *why* behind their code faster than the *what*. logmind
captures architectural and implementation decisions as you make them,
attaches them to the relevant git branch, and surfaces them to the next
human or AI that works in the repo. One CLI command per decision; the
package handles the docs, the branch routing, and the merge-time
aggregation.

**Key concept:** Install once, init anywhere, log everything. Feature
branches get their own decision file. `docs/timeline.md` is the
main-canonical, source-derived union of every decision file in the repo —
the one start-here doc for a cold agent.
AGENTS.md is the canonical agent-instruction file; per-tool files
(CLAUDE.md, .cursorrules, ...) are 2-line stubs pointing to it.

`logmind log` *is* the commit primitive, not a wrapper around one. A
commit-msg hook plus a Claude Code PreToolUse hook (both installed by
`logmind init` / refreshed by `logmind doctor --fix`) block a substantive
raw `git commit` and steer you back to `logmind log` — escape hatches are
`[skip-logmind]`, `LOGMIND_ALLOW_GIT_COMMIT=1`, and `git.enforce_commits:
false`. After every log, stderr may carry an advisory pulse — a stale
component or a spec that's drifted 20+ decisions behind — worth a
`logmind doctor --fix` or a look at the project's spec file
(`context.spec_file`, scaffolded via `logmind init --spec`).

## Install

### On your laptop

```bash
# Homebrew (recommended on macOS + Linux)
brew install thrillmade/tap/logmind

# Or — curl one-liner (auto-fetches the latest release)
curl -fsSL https://logmind.dev/install.sh | bash

# Pin to a specific version on either path:
LOGMIND_VERSION=v1.2.0 curl -fsSL https://logmind.dev/install.sh | bash
```

Verify the install:

```bash
logmind --version
# logmind 1.2.0 (spec 0.1.1)
```

The curl installer is idempotent — re-running it when the same version
is already installed is a fast no-op, so you can drop it into a
shell-rc or laptop bootstrap script without worrying about churn. It
also auto-detects `GITHUB_ACTIONS=true` and nudges you toward the
dedicated CI install path described next.

### In GitHub Actions

Use the [`thrillmade/setup-logmind`](https://github.com/thrillmade/setup-logmind)
action. It handles platform detection, version pinning, and step
caching:

```yaml
- uses: thrillmade/setup-logmind@v1
  with:
    token: ${{ github.token }}
- run: logmind check-links
```

The `token: ${{ github.token }}` line matters: composite actions can't
default an input to `github.token`, so without it setup-logmind's
release-lookup call is anonymous — shared GitHub-hosted-runner IP
ranges routinely exhaust the unauthenticated `api.github.com` rate
limit and 403 before logmind installs. Pass it explicitly.

`logmind init` (v1.1.0+) installs a `.github/dependabot.yml` block that
tells Dependabot to bump the action ref on every new logmind release.
You pin once, and the ecosystem keeps you current — no manual sweeps,
no `pip install logmind==...` lines, no curl-install inside CI.

Both paths deliver signed + notarized binaries from the
[GitHub Releases page](https://github.com/thrillmade/logmind/releases) —
no Python toolchain, no pyenv shim version skew, no pip cache surprises.
See [docs/install.md](docs/install.md) for the full install matrix
(`go install` for builds-from-source, manual binary download, checksum
verification, and the legacy Python path).

> **Heads up on `pip install logmind`** — the Python wheel is **frozen at
> v0.6.16** as the last published Python release. New installs should use
> the Go binary above. The PyPI package is kept on PyPI only to honour
> old pinning; it receives no further updates. The [legacy install
> section](#legacy-install-python-frozen-at-v0616) at the bottom has the
> details for users migrating off it.

## Recommended repo settings

`docs/timeline.md` and `docs/file-structure.md` are **purely derived** — a
branch never edits them; they regenerate only on `main`. This zero-conflict
invariant is **unconditional for every repo** — there is no opt-in, no
config key, nothing to adopt — and is enforced in layers, each closing a
gap the previous one can't reach:

- **L0** — the `post-merge`/`post-rewrite` git hooks regenerate on the
  default branch only; a feature branch is never touched.
- **L1** — `logmind log` restores both files to `HEAD` before staging, on
  a non-default branch.
- **L2 (pin-preservation)** — catches a raw `git commit` that L0/L1 can't
  reach, e.g. after `logmind warp` deliberately pulls `main`'s newer copy
  into your working tree for review. **L2a** is a `pre-commit` git hook
  (pure git, no `logmind` binary required, never blocks a commit),
  installed by `logmind init`. **L2b** is the same restore run inside the
  Claude Code harness's PreToolUse guard, before its allow/block decision
  — this additionally catches `git commit --no-verify` (which skips every
  git hook, including L2a) and works in a fresh clone (git hooks aren't
  cloned; `.claude/settings.json` is).
- **L3** — the `regen-timeline.yml` GitHub Action's `check-derived-docs`
  job **blocks a PR** that modified either derived doc, and on every push
  to `main` regenerates both files and commits + pushes them back (via a
  minted `skdd-steward[bot]` App token; without it, the workflow just
  warns that `main` is momentarily stale — never a conflict risk, only a
  freshness gap).

L0-L2 are local guardrails, not guarantees — every one is bypassable
(`--no-verify`, a deleted or disabled hook, hand-editing
`.claude/settings.json`, or a tool that never goes through git or Claude
Code). **L3 (CI) is the only non-bypassable enforcement**, since it runs
server-side on every PR regardless of what did or didn't happen locally.

For the derived files to stay conflict-free across *concurrent* PRs, the
following is **recommended** (belt-and-suspenders — L0-L3 already make a
derived-doc conflict impossible by construction on any single merge):

- **Strict required status checks on `main`** —
  `Settings → Branches → Branch protection rule (or Ruleset)` →
  *"Require branches to be up to date before merging"*. This forces a
  PR to be rebased on latest `main` before merge, which keeps
  `docs/timeline.md` (and any other derived file) conflict-free across
  concurrent PRs.

Without that toggle, the derived-doc invariant still holds per merge — a
branch's copy is always byte-identical to its own merge-base, so git takes
`main`'s (regenerated) side automatically, and the `logmind-timeline` /
`logmind-timeline-archive` / `logmind-file-structure` merge drivers — one per
derived doc — are belt-and-suspenders for the rare case a branch diverges
anyway.
Being up to date mainly helps avoid unrelated conflicts elsewhere (e.g. two
branches editing the same decision file) and keeps history linear.

### One-command setup via reporulez

If you'd rather not click through the GitHub UI, the
[`clud-bug-logmind`](https://github.com/thrillmade/reporulez) variant of
`reporulez` ships the canonical ruleset for repos using both logmind
and clud-bug — strict status checks pinned to logmind's check names,
required thread resolution, squash-only, the works:

```bash
curl -fsSL https://raw.githubusercontent.com/thrillmade/reporulez/main/bin/apply.sh \
  | bash -s -- owner/your-repo clud-bug-logmind
```

## Quick Start

```bash
# After brew/curl install, `logmind` is globally available

# Initialize in your project
cd your-project
logmind init

# OR — install the full SkDD toolchain (logmind + clud-bug):
logmind init && npx clud-bug@latest init

# Log decisions via CLI
logmind log "Use PostgreSQL for database" \
  -r "Need ACID compliance" \
  -a "MongoDB" -a "SQLite"

# View and search decisions
logmind show
logmind search "postgres"

# One-read agent cold-start context: file-structure + timeline in one go
logmind context
logmind context --stats     # token receipt instead of the payload

# Repo signature skeleton (Go + TS/JS, function bodies dropped)
logmind repomap
logmind repomap --map-tokens 4000   # pack to a token budget, ranked by importance

# Health check — version drift, hook drift, missing timeline markers
logmind doctor
logmind doctor --fix

# Set the branch's one-sentence timeline headline
logmind headline "Added JWT session auth with refresh-token rotation"

# Terse, chainable machine output for scripted/agent invocations
LOGMIND_QUIET=1 logmind log "Use PostgreSQL" -r "Need ACID compliance"

# Enforce decision logging with a pre-commit hook
logmind install-hook          # installs .git/hooks/pre-commit
logmind check-decisions       # run manually or in CI

# Manage AI agents
logmind agents list
logmind agents add windsurf

# Publish a local skill to the public catalog (local → catalog PR)
logmind skill push critical-issues-only          # opens PR on thrillmade/agent-skills
logmind skill push my-skill --dry-run            # preview without clone/push
logmind skill push my-skill --catalog acme/private-skills

# View and modify configuration
logmind config list
logmind config get git.auto_push
logmind config set git.auto_push false

# Upgrade logmind
brew upgrade thrillmade/tap/logmind   # or re-run the curl installer
```

### `logmind skill push` privacy gate

Skills are AUTHORED in the consumer repo first (`.claude/skills/<name>/SKILL.md`),
then optionally promoted to a catalog repo via `logmind skill push`. Four
layered guards keep proprietary skills from leaking into a public catalog:

- **Layer 1 — frontmatter markers** — `private: true` or `do-not-promote: true`
  in the SKILL.md frontmatter blocks the push before any clone happens.
- **Layer 2 — directory convention** — skills placed under
  `.claude/skills-private/<name>/` are private by default (Vault-style).
  Placement wins over an explicit `private: false` override.
- **Layer 3 — content scanner** — every SKILL.md body is scanned for
  credential-shaped tokens (Stripe, Slack, GitHub, npm, AWS, GCP),
  internal-process keywords (`confidential`, `proprietary`, `nda`, …),
  org-internal domain references (configurable via `.logmind/config.yml
  privacy_scanner.org_domains`), and local-machine paths (`/Users/<name>/`,
  `/home/<name>/`). Hits are block-severity (rejects the push) or
  warn-severity (prints to stderr, continues). Config can WIDEN the
  deny set but never weaken the hardcoded baseline.
- **Layer 4 — repo-visibility check** — if the source repo is private
  (or GHEC "internal") and the catalog target is public, the push is
  rejected. Set `allow_promote_from_private: true` in
  `.logmind/config.yml` to acknowledge cross-visibility promotion.
  Layers 1-3 still run.

There is no `--force` flag — these are guard rails, not toggles. See
`logmind skill push --help` for the full surface.

## Contributing / Development Setup

Working on logmind itself? It's a Go module — clone, build, run.

```bash
# Clone the repo
git clone https://github.com/thrillmade/logmind.git
cd logmind

# Build + install the dev binary
go build -o ./bin/logmind ./cmd/logmind
./bin/logmind --version

# Run tests
go test ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full dev loop (lint, snapshot
tests, release workflow).

## Documentation

- **[Install](docs/install.md)** - Full install matrix (brew, curl, go install, manual, legacy Python)
- **[Architecture](docs/plan.md)** - Vision, approach, and technical details
- **[Roadmap](docs/roadmap.md)** - What ships next, in what order, and why
- **[AI Agent Files](docs/ai-agent-files.md)** - How logmind integrates with AI instruction files
- **[First Decision Example](docs/first-decision-example.md)** - What the initial decision looks like

## How It Works

1. **Install** the `logmind` binary (brew / curl)
2. **Init** creates `docs/` folder, inserts the canonical block into
   `AGENTS.md` (2-line stubs for per-tool files), and installs the
   commit-msg + Claude Code PreToolUse hooks that block a substantive
   raw `git commit`
3. **Log** a decision — `logmind log` *is* the commit: appends the entry,
   archives old ones (keeps 20 recent), regenerates `docs/timeline.md`
   (the main-canonical, cross-branch union) and `docs/file-structure.md`,
   commits, and pushes. Stderr may carry a pulse advisory afterward — a
   stale component or a spec falling behind.
4. **Context** — `logmind context` gives an agent the file structure +
   timeline in one cache-friendly read instead of reconstructing state
   from `git log` / `ls -R` / `grep`; `logmind repomap` adds the API
   surface on top

## Why logmind?

- **Simple:** Two markdown files (recent + archive), no database
- **Focused:** Only 20 most recent decisions for relevant AI context
- **Git-native:** Every decision is a commit, git history is your audit trail
- **AI-friendly:** Recent decisions + file structure = complete context
- **Automatic:** Commits and pushes on every log

See [docs/plan.md](docs/plan.md) for the architecture and
[docs/roadmap.md](docs/roadmap.md) for the roadmap.

## Legacy install (Python, frozen at v0.6.16)

**Deprecated.** The Python wheel `logmind` is frozen at v0.6.16 — the last
published Python release before the v1.0 Go rewrite. New installs should
use the Go binary at the top of this README. The PyPI package stays
listed only so consumer repos that pinned `logmind==0.6.x` keep
resolving; it receives no further updates, no security backports, and no
feature parity with v1.0+.

If you're already on the Python wheel and want to migrate, swap one line
in your install step:

```bash
# Before — pinned to v0.6.x Python wheel
pipx install 'logmind==0.6.16'   # OR: pip install 'logmind==0.6.16'

# After — Go binary, signed + notarized
brew install thrillmade/tap/logmind
# OR
curl -fsSL https://logmind.dev/install.sh | sh
```

See [docs/install.md#deprecated-python-install](docs/install.md#deprecated-python-install)
for the full migration matrix (CI YAML one-liners, dependency-import
hand-off, and what features have moved or been retired). The Python
v0.6.x release notes are preserved at
[docs/changelog-python.md](docs/changelog-python.md) for historical
reference.

---

## Part of the thrillmade SkDD toolchain

[Skills-Driven Development](https://zakelfassi.com/skdd-skills-driven-development) (Zak Elfassi's methodology) gives you the loop; the thrillmade toolchain ships the parts:

- **[logmind](https://github.com/thrillmade/logmind)** — the *why* behind every change (decision logging as commit primitive); skill-creation + testing + auditing
- **[clud-bug](https://github.com/thrillmade/clud-bug)** — skill-driven PR review at gate time; every finding cites the skill that motivated it
- **[agent-skills](https://github.com/thrillmade/agent-skills)** — public catalog of reusable skills
- **[skills.sh](https://skills.sh)** — skill discovery + install

End-to-end agentic auto dev: write skills first → log the *why* → run them against PRs → iterate based on usage. The tools work independently; better together.