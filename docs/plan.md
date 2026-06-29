# logmind: Plan & Architecture

> **Heads-up for the next reader:** the "Development History / Phase 1–11"
> material further down this file is the **legacy Python plan** (pre-v1.0).
> The Go rewrite (v1.0+) already shipped most of it under `internal/`.
> The **active plan** is the shape-up below. Start here.

## Shape-up (2026-06): stop logmind from creating friction in consumer repos

**Theme:** logmind v1.2.x core logging works, but it generates real
friction in the repos that depend on it (it *is* the commit primitive for
every thrillmade repo, so any friction is felt on every commit). This
shape-up is **not "more features"** — it is "stop blocking the repos that
already adopted us." Slices are sequenced **blocker-first**.

The cross-repo contract these changes MUST honor lives in the sibling
`protocol` repo's `SPEC.md` (not in-tree, so not linked): **§2** git-hook contract,
**§4** branch routing, **§5** workflow contract. logmind does not own a
single "logmind section" of the SPEC — its surface is spread across those
sections (filesystem §1, hooks §2, CLI §3, branch routing §4, workflows
§5). That is the right shape; no new top-level SPEC section is proposed.
One **drift** is flagged below: the shipped `regen-timeline.yml.template`
is at `v5` (GITHUB_TOKEN single-job self-heal) but the SPEC only documents
through `v4` — §5.1.1 should be extended to describe v5 (tracked in Slice 1).

### Reality check vs the legacy Phase 5–11 checklist

The friction brief described some of these as "half-built." Audited against
the Go source, the real state is:

- **Phase 5 — branch-aware storage: DONE in Go.**
  `internal/cli/log.go::resolveDecisionsPath` routes feature-branch logs to
  `docs/decisions-branches/<sanitized-branch>.md` with the first-creation
  backlink header; `internal/gitcli` has `CurrentBranch`/`DefaultBranch`;
  `config.Decisions.BranchAware` (default true) gates it. Opt-out and
  detached-HEAD/non-git fallbacks to `docs/decisions.md` are implemented.
- **Phase 6 — PR-merge aggregation: superseded, needs a decision (Slice 5).**
  The standalone `logmind-aggregate.yml` was retired; `logmind timeline`
  now aggregates branch files into `docs/timeline.md` directly, so agents
  already see branch decisions without a per-merge append to `decisions.md`.
  The "Merged: <branch> (#NN)" one-liners in `decisions.md` are now legacy.
  Either finish a deterministic merge-append step or formally retire it.
- **Phases 7/8/10/11 — link-check, AGENTS.md consolidation, the skill, OSS
  hygiene: shipped** (`internal/linkcheck`, AGENTS.md `v3-slim` block, the
  `logmind` agent skill, `CONTRIBUTING.md`/`SECURITY.md`/release workflows).
- **Phase 9 — tree-gen hardening: NOT done, and it is now a blocker** for
  putting `docs/file-structure.md` behind the CI gate (Slice 2).

### Friction triage (blocker vs polish)

| # | Friction | Verdict | Slice |
|---|----------|---------|-------|
| 1 | Derived-doc CI gate **fail-exits** on stale `timeline.md`/`file-structure.md` — wedges every PR, no grace, no auto-fix | **Consumer BLOCKER (#1 pain)** | **Slice 1 — ship first** |
| 2 | Branch divergence / merge conflicts on derived files when branch protection isn't strict (`logmind log` auto-pushes; CI commits regen) | Consumer blocker (latent) | Slice 4 |
| 3 | Version drift in hooks / workflow templates / AGENTS.md blocks; `doctor` reports DRIFT but never fixes; no "clear a repo" path | Consumer friction (silent rot) | Slice 3 |
| 4 | Phase 6 merge-aggregation incomplete / superseded | Polish (timeline.md already covers the need) | Slice 5 |
| 5 | Stale `AGENTS.md` — duplicate `## Development Commands` / `## Project Overview` + an entire CLAUDE.md/Cursor dump (Python→Go migration artifact) | Polish (cosmetic, this repo only) | Slice 6 |
| — | `file-structure.md` non-determinism (tree root = cwd basename; `.playwright-mcp/` + nested-gitignore artifacts leak in) | Blocker for Slice 1's file-structure coverage | Slice 2 |

### Slice 1 — derived-doc CI gate must not block PRs **(SHIPPING FIRST)**

The repo's own `.github/workflows/regen-timeline.yml` is still the
**fail-fast** design (`permissions: contents: read`, exit 1 on stale
`timeline.md`, no auto-commit, and it doesn't even check
`file-structure.md`). A stale derived doc red-lights every open PR with no
self-service fix other than "regenerate locally and push" — which races the
next concurrent PR. Meanwhile the **consumer template is already at v5**
(GITHUB_TOKEN single-job self-heal). The repo's own gate never got the
upgrade — that is the gate that blocked this very work session repeatedly.

**Fix:** convert the repo's own gate to the **self-heal** model, adapted to
build-from-source (the repo can't `setup-logmind` itself):

- Regenerate **both** `timeline.md` and `file-structure.md`.
- **Same-repo PRs + pushes to `main`:** on drift, auto-commit + push the
  regen via `GITHUB_TOKEN` (`github-actions[bot]` identity), then re-verify
  in the same job. Never blocks.
- **Forked PRs:** `GITHUB_TOKEN` can't push to the fork head ref → emit a
  `::warning::` advisory and **exit 0** (per SPEC §5.1.1: forks MUST NOT
  fail the check; the post-merge regen on `main` reconciles). This is
  strictly less blocking than the shipped v5 template, which currently
  *fails* forks — reconcile the template + SPEC §5.1.1 to "v5 advisory on
  fork" as a fast follow.

Eventually-correct guarantee: even when a fork PR merges stale, the push to
`main` self-heals on the next run. No PAT, no per-repo secret, no
branch-protection prerequisite to *not block*.

### Slice 2 — tree-gen determinism hardening (legacy Phase 9)

Required before `file-structure.md` can be a *clean* gate (Slice 1 makes it
non-blocking via auto-commit, but a non-deterministic generator churns a
regen commit on every PR opened from a differently-named worktree):

- Root label is `filepath.Base(repoRoot)` (`internal/tree/tree.go:227`) —
  make it deterministic (fixed label or git-derived repo name), not the
  checkout directory name.
- Tree walker reads only the **root** `.gitignore`; nested `site/.gitignore`
  entries (`next-env.d.ts`, `tsconfig.tsbuildinfo`) leak in. Honor nested
  ignores or add them to `file_structure.ignore_patterns`.
- `.playwright-mcp/` (local MCP scratch) is untracked + un-ignored → leaks
  into the tree. Add it to `.gitignore` (done opportunistically in Slice 1).

### Slice 3 — `logmind doctor --fix` + a one-command "clear a repo"

`doctor` is read-only today (`internal/doctor/doctor.go`); it reports DRIFT
on hooks / workflow templates / AGENTS.md blocks but never heals, so
consumer repos rot silently. Add:

- `logmind doctor --fix` — re-write drifted git hooks (the installers in
  `internal/hooks` already refuse-foreign / overwrite-ours), re-apply
  drifted workflow templates and the AGENTS.md marker block to the bundled
  version. Dry-run by default surface; `--fix` opts into writes.
- `logmind reset` (or `doctor --clear`) — a single command to remove
  logmind-managed hooks / `.gitattributes` block / config so a consumer can
  cleanly detach or re-init.

### Slice 4 — robustness to branch divergence on derived files (friction #2)

Make the auto-push + CI-regen interaction not require strict branch
protection to avoid local↔remote divergence: keep derived files out of the
local commit on feature branches (regen is CI's job on the canonical
checkout), and/or have `logmind log` rebase-pull before pushing when the
remote moved. Pairs with Slice 1 — once CI is the single source of truth for
derived docs, local logs shouldn't fight it.

### Slice 5 — finish or formally retire Phase 6 merge-aggregation (friction #4)

Decide: (a) a deterministic post-merge step that appends a `Merged: <branch>
(#NN)` one-liner to `decisions.md`, or (b) retire the concept (timeline.md
already aggregates branch decisions) and document `timeline.md` as the
canonical cross-branch view. Lean (b); it removes a moving part.

### Slice 6 — AGENTS.md cleanup (friction #5)

`AGENTS.md` carries a duplicate `## Development Commands` / `## Project
Overview` pair and an entire inlined CLAUDE.md + Cursor-rules dump — a
Python→Go `agents-sync` artifact. Trim to the canonical block + project
sections. Cosmetic, this-repo-only; do last.

### Org-level actions (not code — flag for a human)

- **Branch-protection default:** the canonical `clud-bug-logmind` ruleset in
  `reporulez` should ship "Require branches up to date before merging" so
  derived files stay conflict-free across concurrent PRs. Slice 1 removes
  this as a *hard* requirement (gate self-heals), but it remains the
  recommended default. No per-repo secret is needed — v5/Slice 1 use
  `GITHUB_TOKEN`, so the old `LOGMIND_AUTO_REGEN_PAT` is **not** required.
- **SPEC update:** extend `protocol/SPEC.md §5.1.1` to document
  `regen-timeline.yml` **v5** (GITHUB_TOKEN single-job self-heal) — the
  template shipped ahead of the spec.

### Future cross-repo spec idea (flagged, NOT built here)

logmind could be **clud-bug-aware**: when a `logmind log` commit lands in a
repo whose `.claude/skills/.clud-bug.json` declares clud-bug installed,
logmind could signal/trigger the clud-bug review for that commit (SkDD:
log the *why*, then review against the skills that motivated it). This is a
cross-repo concern — it belongs in a future SPEC section (interop, §10
neighborhood), not in logmind's own code yet. Recorded here so it isn't lost.

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

### Phase 5: Branch-aware decision storage 🔲 IN PROGRESS

Goal: support multi-developer / multi-branch workflows without merge-conflict churn on `docs/decisions.md`.

- [ ] `git_handler.current_branch()` and `git_handler.default_branch()` helpers
- [ ] `logger.log()` resolves write target by branch:
  - Default branch (or non-git repo) → `docs/decisions.md`
  - Other branches → `docs/decisions-branches/<sanitized-branch>.md`
- [ ] `_archive_oldest_decision()` operates on whichever file was just written
- [ ] New config knob `decisions.branch_aware: true` (default) — opt-out preserves legacy behaviour
- [ ] `tests/test_branch_aware_logging.py` covers default-branch, feature-branch, opt-out, archival, non-git-repo cases

### Phase 6: PR-merge aggregation GitHub Action 🔲 IN PROGRESS

Goal: when a feature branch merges, append a one-line entry to `docs/decisions.md` linking the PR + the per-branch detail file.

- [ ] `src/logmind/actions/aggregate.py` reads merged-branch file and appends merge entry
- [ ] `src/logmind/templates/github/logmind-aggregate.yml.template` triggers on `pull_request: closed && merged`
- [ ] `logmind init` copies the workflow into `.github/workflows/`
- [ ] `tests/test_action_aggregate.py` exercises the aggregator against a mock PR payload

### Phase 7: Markdown link integrity CI 🔲 IN PROGRESS

Goal: ship a `logmind check-links` command + GitHub Action that fails CI on broken or orphaned `.md` links — links are how agents stay in context.

- [ ] `src/logmind/actions/link_check.py` walks README + docs/, reports broken + orphan links
- [ ] `logmind check-links` CLI subcommand
- [ ] `src/logmind/templates/github/check-doc-links.yml.template` runs the checker on every PR
- [ ] Config: `linkcheck.allow_orphans` and `linkcheck.roots` in `.logmind/config.yml`
- [ ] `tests/test_link_check.py`

### Phase 8: AGENTS.md as canonical, others as stubs 🔲 IN PROGRESS

Goal: collapse 11 duplicated agent-instruction files to one canonical AGENTS.md + 2-line stubs.

- [ ] `src/logmind/templates/AGENTS.md.template` — full canonical template
- [ ] `src/logmind/templates/agent-stub.md` — 2-line "see AGENTS.md" stub
- [ ] `inserter.py` writes canonical content to AGENTS.md, stubs to other agent files
- [ ] `logmind agents migrate` command — merges existing per-agent content into AGENTS.md, replaces with stub
- [ ] `tests/test_agents_consolidation.py`

### Phase 9: Tree generation hardening + .gitignore management 🔲 IN PROGRESS

Goal: deterministic tree output and a managed `.gitignore` block written by `logmind init`.

- [ ] `tree_gen.py` fallback: stable sort, gitignore-aware, byte-identical across platforms
- [ ] `logmind tree` CLI subcommand for on-demand regeneration
- [ ] `src/logmind/core/gitignore.py` with `ensure_block()` helper (idempotent, preserves manual edits inside the marker block)
- [ ] `logmind init` writes a `# logmind` block to `.gitignore` (`.logmind/cache/`, `.logmind/.lock`)
- [ ] `tests/test_file_structure_updates.py` and `tests/test_gitignore_management.py`

### Phase 10: `logmind` agent skill + install offer 🔲 IN PROGRESS

Goal: publish a skills.sh skill so any AI agent in any project knows how to use logmind, and have `logmind init` offer to install it globally.

- [ ] Author SKILL.md (separate `logmind-skill` repo, scaffolded under this branch but pushed at release time)
- [ ] `src/logmind/core/skill_install.py` with `is_skills_available()` and `install_globally()`
- [ ] `logmind init` prompts (and accepts `--skill-install/--no-skill-install`)
- [ ] Submit skill to the skills.sh registry

### Phase 11: Publication & open-source readiness 🔲 IN PROGRESS

Goal: make `pip install logmind` and `brew install logmind` work publicly, meet open-source quality bars (CI matrix, contribution docs, security policy), and enable `logmind update` to self-upgrade in any repo.

**11a. Repository hygiene files**

- [ ] `CONTRIBUTING.md` — dev setup, test command, code style, PR checklist
- [ ] `CODE_OF_CONDUCT.md` — Contributor Covenant 2.1
- [ ] `SECURITY.md` — vulnerability reporting, supported versions
- [ ] `.github/ISSUE_TEMPLATE/{bug_report,feature_request,config}.{md,yml}`
- [ ] `.github/PULL_REQUEST_TEMPLATE.md`
- [ ] `CHANGELOG.md` — Keep-a-Changelog header, full v0.1.0 entry

**11b. CI matrix workflow**

- [ ] `.github/workflows/test.yml` — pytest on Python 3.8 / 3.10 / 3.12 / 3.13 (Ubuntu) + 3.12 (macOS, Windows smoke)
- [ ] Workflow also runs `logmind check-links` against the repo's own docs

**11c. Quality gates**

- [ ] `pyproject.toml` `[tool.ruff]`, `[tool.black]`, `[tool.mypy]` config
- [ ] `.pre-commit-config.yaml` — ruff, black, mypy, link checker, file hygiene
- [ ] `.github/dependabot.yml` — weekly pip + github-actions updates

**11d. Discoverability**

- [ ] `pyproject.toml` `[project.urls]`, `keywords`, `classifiers`
- [ ] README badges (PyPI, CI, license, Python versions)
- [ ] GitHub repo description + topics (manual)

**11e. Release automation**

- [ ] `.github/workflows/publish.yml` — tag-triggered build + PyPI publish via OIDC (Trusted Publisher)
- [ ] Workflow also creates a GitHub Release with the artifacts and CHANGELOG excerpt

**11f. PyPI / Homebrew publish**

- [ ] Build distribution artifacts (`python -m build`)
- [ ] First publish to PyPI (`twine upload dist/*` or auto via tag)
- [ ] Verify `pip install logmind && logmind --version`
- [ ] Create GitHub Release with tag `v0.1.0`
- [ ] Create separate `homebrew-logmind` GitHub repo (tap convention: `homebrew-<name>`)
- [ ] Compute real SHA256 from PyPI tarball; update formula
- [ ] Verify `brew tap thrillmade/logmind && brew install logmind && logmind --version`
- [ ] Verify `logmind update` self-upgrades in another repo

### Test Progression
- Phase 1: 65 tests
- Phase 2: 95 tests
- Phase 3: 178 tests
- Phase 4: 301 tests
- Phases 5–10 (this branch): targeting 400+ tests with new branch-aware, aggregator, link-check, agents-consolidation, gitignore, tree, and skill-install suites

## Completed Features

See [README.md](../README.md) for usage documentation.

- **Phase 1**: Core package (`init`, `log`, `show`)
- **Phase 2**: Configuration system, search functionality
- **Phase 3**: Decorators (`@log_decision`, `@log_choice`) + Universal agent support (11 agents)
- **Phase 4**: Framework integrations, pre-commit hook, templates, analytics, aggregation, Homebrew
- **Phases 5–10** (in progress — current branch `virtual-kurzweil`): branch-aware decision storage, PR-merge aggregator GH Action, markdown link-integrity CI, AGENTS.md consolidation, tree/`.gitignore` hardening, `logmind` skill on skills.sh
- **Phase 11**: Open-source publication readiness + PyPI / Homebrew publish

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
brew tap thrillmade/logmind
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
