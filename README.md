# logmind

[![PyPI](https://img.shields.io/pypi/v/logmind.svg)](https://pypi.org/project/logmind/)
[![Python versions](https://img.shields.io/pypi/pyversions/logmind.svg)](https://pypi.org/project/logmind/)
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
branches get their own decision file; on PR merge a GitHub Action appends
a one-line summary to `docs/decisions.md` linking the PR + the branch
detail. AGENTS.md is the canonical agent-instruction file; per-tool files
(CLAUDE.md, .cursorrules, ...) are 2-line stubs pointing to it.

## Installation

```bash
# Homebrew (recommended on macOS + Linux)
brew install thrillmade/tap/logmind

# curl one-liner
curl -fsSL logmind.dev/install.sh | bash
```

The brew + curl paths deliver signed + notarized binaries from the
[GitHub Releases page](https://github.com/thrillmade/logmind/releases) —
no Python toolchain, no pyenv shim version skew, no pip cache surprises.
See [docs/install.md](docs/install.md) for the full install matrix
(including `go install` for builds-from-source and the deprecated pip
path for v0.6.x consumers).

### v0.6.x (Python, deprecated)

The legacy Python distribution still works for users pinned to v0.6.x
consumer-repo workflows. Migration to the brew/curl binary is a one-line
swap in your CI YAML — see [docs/install.md](docs/install.md#deprecated-python-install).

```bash
# Deprecated — pinned to v0.6.x; pre-cutover consumer repos only
pipx install logmind  # OR: pip install logmind
```

## v1.0 Go rewrite in progress

`main` ships the Python package (v0.6.x — current stable, on PyPI). The
v1.0 Go rewrite lives on the long-lived
[`v1-go-rewrite`](https://github.com/thrillmade/logmind/tree/v1-go-rewrite)
branch and lands as wave PRs targeting that branch; the final
`v1-go-rewrite → main` cutover PR becomes `v1.0.0`. Python source under
`src/logmind/` stays put through the rewrite — both implementations
coexist until cutover, gated by a byte-identical parity snapshot suite.
See [`docs/plan.md`](docs/plan.md) for the wave breakdown.

## Required repo settings

`logmind init` ships a GitHub Action (`regen-timeline.yml`) that VERIFIES
`docs/timeline.md` is up to date on every PR — fail-fast if stale, no
auto-commit. For the derived-file architecture to stay conflict-free
between concurrent PRs, your repo needs:

- **Strict required status checks on `main`** —
  `Settings → Branches → Branch protection rule (or Ruleset)` →
  *"Require branches to be up to date before merging"*. This forces a
  PR to be rebased on latest `main` before merge, which is what makes
  `docs/timeline.md` (and any other derived file) conflict-free across
  concurrent PRs.

Without that toggle, two PRs in flight can both regenerate against
different base commits and merge-conflict on the derived file.

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
# If installed via pipx/brew, it's already available globally

# Initialize in your project
cd your-project
logmind init

# OR — install the full SkDD toolchain (logmind + clud-bug) in one go:
logmind init --with-skdd      # subprocesses to `npx clud-bug init` (requires Node 20+)

# Log decisions - Python API
from logmind import log
log("Chose FastAPI over Flask",
    reasoning="Need async/await for WebSocket handling")

# Or use CLI
logmind log "Use PostgreSQL for database" \
  -r "Need ACID compliance" \
  -a "MongoDB" -a "SQLite"

# View and search decisions
logmind show
logmind search "postgres"

# Log with a built-in template (pre-fills reasoning, alternatives, implications)
logmind log --template database "Use PostgreSQL"
logmind templates   # list all available templates

# Analytics and stats
logmind stats
logmind stats --months 6

# Aggregate decisions across multiple projects
logmind aggregate ~/projects/api ~/projects/frontend
logmind aggregate --summary ~/work/*/

# Enforce decision logging with a pre-commit hook
logmind install-hook          # installs .git/hooks/pre-commit
logmind check-decisions       # run manually or in CI

# Manage AI agents
logmind agents list
logmind agents add windsurf

# View and modify configuration
logmind config list
logmind config get git.auto_push
logmind config set git.auto_push false

# Upgrade logmind
logmind update

# Auto-log with decorators
from logmind import log_decision, log_choice

@log_decision(
    decision="Authenticate user with {method}",
    reasoning="Security checkpoint"
)
def authenticate(method="oauth"):
    # Your auth code
    return True

@log_choice(
    choices={
        "redis": "Use Redis for caching",
        "memory": "Use in-memory caching",
    }
)
def select_cache():
    return "redis" if is_production() else "memory"
```

## Contributing / Development Setup

Working on logmind itself? Set it up like any CLI tool:

```bash
# Clone the repo
git clone https://github.com/thrillmade/logmind.git
cd logmind

# Install globally in editable mode (like npm, git, docker)
pipx install -e .

# Now just use it!
logmind log "Add new feature" -r "Reasoning here"
logmind show
logmind search "keyword"

# Run tests
python3 -m venv venv
source venv/bin/activate
pip install -e ".[dev]"
pytest
```

**Why pipx?** logmind is a CLI tool, not a library. It should be globally available like `git` or `npm`.

## Framework Integrations

```python
# LangChain — auto-log agent decisions (pip install logmind[langchain])
from logmind.integrations import LangChainLogger

chain = LLMChain(llm=llm, callbacks=[LangChainLogger()])

# Custom framework — subclass BaseIntegration
from logmind.integrations.base import BaseIntegration

class MyLogger(BaseIntegration):
    def on_decision(self, output):
        self.log(f"Chose: {output}", reasoning="My framework decided")
```

See [docs/custom-integrations.md](docs/custom-integrations.md) for patterns, examples, and publishing guide.

## Documentation

- **[Plan & Architecture](docs/plan.md)** - Vision, approach, and technical details
- **[AI Agent Files](docs/ai-agent-files.md)** - How logmind integrates with AI instruction files
- **[Custom Integrations](docs/custom-integrations.md)** - Build integrations for any AI framework
- **[First Decision Example](docs/first-decision-example.md)** - What the initial decision looks like
- **Development Status** - All phases complete ✅

## How It Works

1. **Install** logmind as a package
2. **Init** creates `docs/` folder and inserts instructions into `CLAUDE.md` (preserving existing content)
3. **Log** a decision - appends, archives old ones (keeps 20 recent), regenerates tree, commits, and pushes
4. **Context** AI agents read the 20 most recent decisions and current file structure

## Why logmind?

- **Simple:** Two markdown files (recent + archive), no database
- **Focused:** Only 20 most recent decisions for relevant AI context
- **Git-native:** Every decision is a commit, git history is your audit trail
- **AI-friendly:** Recent decisions + file structure = complete context
- **Automatic:** Commits and pushes on every log

See [docs/plan.md](docs/plan.md) for complete architecture and roadmap.

---

## Part of the thrillmade SkDD toolchain

[Skills-Driven Development](https://zakelfassi.com/skdd-skills-driven-development) (Zak Elfassi's methodology) gives you the loop; the thrillmade toolchain ships the parts:

- **[logmind](https://github.com/thrillmade/logmind)** — the *why* behind every change (decision logging as commit primitive); skill-creation + testing + auditing
- **[clud-bug](https://github.com/thrillmade/clud-bug)** — skill-driven PR review at gate time; every finding cites the skill that motivated it
- **[agent-skills](https://github.com/thrillmade/agent-skills)** — public catalog of reusable skills
- **[skills.sh](https://skills.sh)** — skill discovery + install

End-to-end agentic auto dev: write skills first → log the *why* → run them against PRs → iterate based on usage. The tools work independently; better together.