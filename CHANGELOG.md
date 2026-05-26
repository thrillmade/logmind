# Changelog

All notable changes to logmind will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-05-26

### Added — custom git merge driver for derived files
- **`logmind init` registers a custom merge driver for `docs/timeline.md` and `docs/file-structure.md`.** Closes the parallel-PR conflict class: previously, two PRs that both ran `logmind log` produced textual merge conflicts on rebase because git did three-way merge on the derived snapshot. Now git delegates conflict resolution on these files to logmind, which regenerates them from the per-branch decision files (which never collide). Clean rebases on parallel work.
  - **`.gitattributes` block** added by `logmind init` (idempotent, marker-bracketed, preserves user edits) — registers `merge=logmind-timeline` and `merge=logmind-file-structure` for the two derived files.
  - **Per-clone `git config`** also set by `logmind init` — defines the merge drivers themselves (`merge.logmind-timeline.driver = 'logmind timeline --write %A'`, similar for file-structure). Lives in `.git/config`, not committed (git refuses to auto-run a merge driver that wasn't explicitly configured locally — security guard against untrusted repos).
  - **`logmind init` refresh-mode** re-runs `configure_merge_drivers()` every invocation, so fresh clones get the per-clone config after a single `logmind init` even if the committed `.gitattributes` was already in place.
- **New `logmind file-structure --write <path>` CLI command** — mirror of `logmind timeline --write`. The merge driver invokes it as `logmind file-structure --write %A` where git passes the resolved-content target path.
- **`.git/hooks/post-merge` installed by `logmind init`** — re-regenerates `docs/timeline.md` and `docs/file-structure.md` from the FULL post-merge working tree. Belt + suspenders with the merge driver: the driver runs per-file during conflict resolution, before other merged-in files (e.g. the merged-in branch's `docs/decisions-branches/<branch>.md`) are checked out, so its output can miss decisions. The hook runs once at end-of-merge and sweeps any incomplete regen. Verified end-to-end: two branches both running `logmind log`, merge succeeds without conflict, resulting timeline contains both decisions.
- **`logmind doctor` reports merge-driver drift** as three new rows: `.gitattributes (merge driver)`, `git config (merge driver)`, and `post-merge hook`. Each shows `current`/`missing`. Missing rows do NOT count as drift (they're "not yet installed for this logmind version", not "wrong") — the next `logmind init` resolves them silently.

### Why a minor version bump (0.2.x → 0.3.0)
The previous v0.2.x line accumulated feature-grade additions under patch bumps (`logmind doctor` in v0.2.4, `--stage all` default in v0.2.7, changelog-on-upgrade in v0.2.10). v0.3.0 introduces a new install-time surface (git config setup, `.gitattributes` ownership) that's clearly minor-grade, and marks a clean reset of the under-bumping pattern.

### Migration from v0.2.10
Run `logmind init` in each logmind-installed repo to get the merge driver. v0.2.5+'s refresh-mode handles the workflow updates automatically; the `.gitattributes` block is added; `git config` is set per-clone. `logmind doctor` reports two new STALE rows until you do.

**CI runners** (fresh-clone, no `logmind init`) don't get the per-clone config and won't use the driver — but `regen-timeline.yml` already regenerates derived files in CI as a separate safety net (the existing `check-derived-docs` gate). Belt + suspenders.

## [0.2.10] - 2026-05-26

### Added
- **`logmind init` refresh-mode prints CHANGELOG sections between the prior pinned version and the currently installed `__version__`.** Closes the agent-memory propagation gap from the other direction: instead of relying on agents to re-read AGENTS.md or the skill, the actual behavior changes show up inline in the init command output. When agents (or humans) observe `logmind init` after a `pip install --upgrade logmind`, they see "📋 What's new in logmind since vX.Y.Z" followed by every CHANGELOG section between old and new.
- **`logmind.core.changelog` module** with `extract_sections_between(text, after, up_to)` (slice CHANGELOG by version range, descending) and `render_upgrade_prompt(prior_version, current_version)` (compose the printed block; returns None when no upgrade applies).
- **CHANGELOG.md bundled in the wheel** via `pyproject.toml` `package-data`. `publish.yml` adds a build-time copy step (`cp CHANGELOG.md src/logmind/CHANGELOG.md`) since the canonical file stays at repo root for GitHub auto-rendering. Editable installs fall back to the repo-root copy via `_changelog_path()`.

### Fixed
- **`logmind-self-update.yml.template:50` — escape backticks around `pip install`.** Bug hunter caught: the `:notice::` line had ` `pip install` ` (unescaped backticks) in a double-quoted bash string, triggering command substitution. `pip install` (no args) exits non-zero, prints "ERROR: You must give at least one requirement…" to stderr, and the words `pip install` get swallowed from the rendered notice. Only fires on pre-v0.2.1 fresh-install path (no pin in regen-timeline.yml). One-character fix: `` `pip install` `` → `` \`pip install\` ``. Template marker bumped `v3 → v4` so the refresh sweeps it into downstream repos automatically.

### Migration from v0.2.9
Run `logmind init` in each logmind-installed repo. The v0.2.10 init will detect the prior pin (likely v0.2.7–v0.2.9), print the CHANGELOG since then, and refresh `logmind-self-update.yml` to the corrected `v4` marker.

## [0.2.9] - 2026-05-26

### Changed
- **Bump shipped workflow templates from `actions/checkout@v4` and `actions/setup-python@v5` to `@v6`** across all four templates: `regen-timeline.yml`, `check-doc-links.yml`, `check-decisions.yml`, `logmind-self-update.yml`. GitHub deprecated Node 20 actions runtime in 2026; v6 of both actions runs on Node 24. Without this, every downstream install would silently emit deprecation warnings until the Node 20 cutoff (2026-09), then fail.
- **Same bump in this repo's dogfood workflows.** Absorbs Dependabot PRs #43 (`setup-python@v5 → v6`) and #44 (`checkout@v4 → v6`) — Dependabot only saw the dogfood copies, but the shipped templates were the broader gap; this bundles both. PRs #43 and #44 will be closed once this lands (target files already updated).
- **Template-version markers bumped** so downstream `logmind init` refresh-mode picks up the new templates automatically:
  - `regen-timeline.yml`: `v1 → v2`
  - `check-decisions.yml`: `v1 → v2`
  - `check-doc-links.yml`: `v2 → v3`
  - `logmind-self-update.yml`: `v2 → v3`

### Added
- **`vercel.json` at repo root with `ignoreCommand: git diff --quiet HEAD^ HEAD -- site/`.** Skips Vercel preview deployments on PRs that don't touch `site/`. The marketing site rebuilds were burning the deploy quota on every Python-only change — `logmind` and `agent-skills` PRs frequently get rate-limited by Vercel mid-release for this reason. The ignoreCommand exits 0 (skip) when no site/ files changed, 1 (build) otherwise.
- **`logmind log` now emits a visible notice** when default `--stage all` sweeps the working tree: `ℹ Default --stage all (v0.2.7+): every working-tree change is staged into this decision commit. Pass --stage scoped to keep unrelated WIP unstaged.` Agents whose memory predates v0.2.7 (and who keep prefixing `git add -A &&` out of habit) now see the actual behavior in command output, no AGENTS.md re-read required.
- **`logmind doctor` now checks `AGENTS.md` block-version drift.** Reports the embedded `<!-- logmind-block-version: vN -->` marker against the bundled template's marker, in the same table as workflow probes. Stale markers count as drift (exit 1). Markerless AGENTS.md (user customized) doesn't — same heuristic as workflow probes. This closes the propagation gap where an agent's session memory still holds pre-v0.2.7 instructions even though the repo's AGENTS.md on disk was refreshed by `logmind init`.

### Migration from v0.2.8
Run `logmind init` in each logmind-installed repo. v0.2.1+'s refresh-mode auto-detects the bumped markers and rewrites the workflow files; `logmind doctor` reports STALE rows until the refresh runs.

## [0.2.8] - 2026-05-26

### Fixed
- **`logmind-self-update.yml.template`: replace PyYAML+Python pinVersion detection with `grep`.** The previous block called `python3 -c "import yaml, sys; try: ..."` with `import yaml` OUTSIDE the try, so if the workflow runner lacked PyYAML the import raised before the try could catch it. The surrounding `2>/dev/null || echo ""` then swallowed the failure into empty pin — silently breaking opt-out via `pinVersion` whenever the runner shipped without PyYAML. Reported by clud-bug-review across multiple repos (it kept flagging the pattern on every propagation PR even when the bug was dormant on that specific install).

  Replacement uses `grep -E '^[[:space:]]*pinVersion:[[:space:]]*' .logmind/config.yml` plus `sed` to strip optional quotes/whitespace. Tested against 8 input variants (quoted, unquoted, indented, trailing whitespace, absent key, key-as-substring, etc.). No Python, no YAML lib, works on every runner.

- **Template marker bumped `v1 → v2`** so v0.2.7's idempotent refresh logic rewrites `logmind-self-update.yml` on the next `logmind init`.

### Migration from v0.2.7
Run `logmind init` in each logmind-installed repo to pick up the corrected template. v0.2.1+'s refresh mode auto-detects the stale v1 marker and rewrites the workflow — no manual edits needed. Doctor will report the stale marker if you forget.

## [0.2.7] - 2026-05-26

### Changed (default behavior — backwards-compatible flag still works)
- **`logmind log` now defaults to `--stage all`**, staging every change in the working tree alongside the decision rather than just the decision log + companion files. The whole point of `logmind log` is to be a single add+commit+push primitive for automated agents — the previous `--stage scoped` default forced agents into the same two-step pattern (`git add` + `git commit`) that `logmind log` exists to replace.

  The previous default (`scoped`) is still available via `--stage scoped` — useful when you have unrelated WIP in the working tree you don't want to commit. But for the common case (an agent making a focused change + logging the decision for it), one `logmind log "summary" -r "why"` invocation now does everything: writes the decision file, regenerates the derived docs, stages every change in the working tree, commits, pushes.

### Added (documentation clarity — propagates to AGENTS.md via `logmind init` refresh)
- **AGENTS.md.slim.template + AGENTS.md.template rewritten to lead with the single-command model.** Was: "Use `logmind log` for the commit, not `git add` + `git commit`. Use `--stage all` to also stage the rest of the working tree." Now: `logmind log` IS the commit primitive that handles `git add`, `git commit`, and `git push` together; manual git commands are explicitly off-script for any change that carries a decision.

### Migration from v0.2.6
**No `logmind init` required** for the CLI default change — that takes effect as soon as the new logmind is installed. Optional: run `logmind init` in installed repos to pick up the refreshed AGENTS.md block.

**If you have scripts that relied on the old scoped default** (i.e. they ran `logmind log "..."` expecting unrelated working-tree changes to stay unstaged), pass `--stage scoped` explicitly to preserve the old behavior.

## [0.2.6] - 2026-05-26

### Added
- **`notify-agent-skills.yml` workflow** on this repo. Mirrors the `notify-clud-bug.yml` pattern shipped on `thrillmot/agent-skills`: on every tag push (`v*`), opens an issue on `thrillmot/agent-skills` titled `logmind <tag> shipped — review skills/logmind/SKILL.md`. Closes the manual-sync gap that left the skill out of date with v0.2.3 → v0.2.5 features until someone (an agent, in this case) noticed and shipped a batch update.

  - Same auth model as the agent-skills→clud-bug notifier: needs `AGENT_SKILLS_NOTIFY_PAT` repo secret (fine-grained PAT scoped to `thrillmot/agent-skills` with `Issues: write`). Without it, the notifier degrades to a `::warning::` and the release itself succeeds.
  - Internal-only releases (refactor / CI / test additions) can close the issue as no-op; the prompt is the value.

### Migration from v0.2.5
None — this is a logmind-repo-internal workflow, not a downstream template. No `logmind init` needed in installed repos.

## [0.2.5] - 2026-05-26

### Fixed
- **`logmind init` refresh-mode now updates stale `pip install "logmind==X.Y.Z"` pins** in installed workflow templates even when the template-version marker hasn't moved. Pin drift is independent of body drift — versions like 0.2.2 → 0.2.4 didn't change any templates, so refresh-mode left the pin at 0.2.1 across multiple releases. Now: if the installed file's body is current (or markerless) but its `==X.Y.Z` doesn't match the running logmind's `__version__`, the pin line is surgically rewritten in place. One line touched; user body customizations preserved. Caught by an agent working in clud-bug whose `regen-timeline.yml` was still pinned to 0.2.1 after we shipped 0.2.4.

### Migration from v0.2.4
None — behavior-only refinement of refresh-mode. Run `logmind init` in any repo whose `logmind doctor` reports a stale `installed_version` to pick up the fresh pin.

## [0.2.4] - 2026-05-26

### Added
- **New `logmind doctor` command** reports installed-vs-latest versions for logmind + clud-bug, scans workflow templates for stale `# logmind-template-version:` / `# clud-bug-template-version:` markers, and exits non-zero on drift so it can gate CI. Read-only — prints the suggested fix (`pip install --upgrade logmind && logmind init`) but never runs it.
  - `--json` emits the report as machine-readable JSON.
  - `--offline` skips PyPI/npm probes; uses only locally-readable signals.
  - `--exit-zero` always exits 0 even on drift, for informational CI runs.
  - Markerless workflows (the dogfood / heavily-customized case) are reported as `markerless` and never count as drift — they predate the v0.2.1 marker convention and v0.2.1's refresh mode deliberately leaves them alone.
  - clud-bug section is omitted entirely if `.claude/skills/.clud-bug.json` is not present, so doctor stays useful in logmind-only repos.
  - Network failures degrade to `?` in the "latest" column rather than crashing; the marker check + installed-version diff are the load-bearing drift signals.

### Migration from v0.2.3
None — additive change. No template change, no `logmind init` needed in installed repos. Run `logmind doctor` to get a status table; nothing else to do.

## [0.2.3] - 2026-05-26

### Fixed
- **`logmind log` now regenerates and stages `docs/timeline.md` automatically.** Previously the command wrote the new decision file to `docs/decisions-branches/<branch>.md` but left the derived `docs/timeline.md` index out of date, so every decision PR required an extra `logmind timeline --write docs/timeline.md` + push before `check-derived-docs` would pass. PR #42 was the last one bitten by this — the workflow caught the stale index as designed but the manual heal was friction we shouldn't be paying. Now `logmind log` produces a self-consistent commit: the new decision file, the regenerated tree, archived rotations, and the timeline index are all staged together. Timeline regen runs on every branch (not just default) because the CI gate runs on PR branches and timeline merges three-way-merge trivially.

### Migration from v0.2.2
None — this is a CLI behavior change with no template change. No `logmind init` needed in installed repos.

## [0.2.2] - 2026-05-18

### Fixed
- **`check-doc-links.yml.template`: removed paths filter that silently blocked merges.** The shipped template had `paths: ["**/*.md", ".logmind/config.yml"]` on both `pull_request:` and `push:` triggers. When a PR doesn't change any markdown, GitHub Actions skips the workflow — no status report. But if `check-links` is in the ruleset's `required_status_checks` list (like reporulez's `clud-bug-logmind` variant ships it), GitHub treats the missing report as **"expected but never reported"** and blocks the merge forever. Logmind's own dogfood copy fixed this months ago; the shipped template never got the backport. Bit clud-bug PR #52 today — the template-marker PR had no markdown changes and sat blocked until a CHANGELOG entry was added as a fake-trigger.
- **Template marker bumped `v1 → v2`.** v0.2.1's idempotent refresh logic will rewrite the workflow on the next `logmind init` because the version marker differs.

### Migration from v0.2.1
Run `logmind init` in each logmind-installed repo to pick up the corrected template. v0.2.1's refresh mode auto-detects the stale v1 marker and rewrites the workflow — no manual edits needed.

## [0.2.1] - 2026-05-18

### Fixed (audit-driven)
- **P0 — workflow templates now pin the logmind version.** `logmind init` substitutes `__LOGMIND_VERSION__` → the installed `logmind.__version__` when writing each `.github/workflows/*.yml`, so downstream CI runs `pip install "logmind==<exact-version>"` instead of tracking whatever is latest on PyPI. Eliminates silent CI breakage after upstream breaking changes.
- **P1 — `logmind init` is now idempotent on already-initialized repos.** Re-running `logmind init` after a logmind upgrade no longer hard-exits. It now runs in refresh mode: refreshes any workflow whose `# logmind-template-version:` marker is stale, runs `logmind agents update` semantics, and leaves `docs/`, `.logmind/`, and agent files untouched. No flag needed. Eliminates the `mv docs /tmp` + init + `mv docs back` dance reported by reporulez.
- **P3a — narrowed exception handling for git failures.** `is_git_repo` and `current_branch` in `core/git_handler.py` now safely swallow `OSError`/`PermissionError` (in addition to the existing `CalledProcessError`/`FileNotFoundError`) and return False/None respectively. The unreachable bare `except Exception` in `logger.py` was removed. Pre-v0.2.1 a permission error on `.git/` would crash `logmind log`.
- **P3b — atomic writes for all logmind-managed state files.** `decisions.md`, `decisions-archive.md`, `file-structure.md`, `timeline.md`, and per-branch decision logs now write via the temp-file + `os.replace` pattern (new `core/atomic_io.py`). Concurrent `logmind log` invocations from multiple agents in the same repo can no longer truncate one another's writes.

### Added
- `# logmind-template-version: v1` header in every workflow template (`check-decisions.yml.template`, `check-doc-links.yml.template`, `regen-timeline.yml.template`) — drives the v0.2.1 refresh-mode logic and gives future template revisions a clean migration path.

### Migration from v0.2.0
None. v0.2.1 is a strict superset; existing installs are unaffected. To pick up the workflow-version pinning + refresh-mode-on-reinit benefits, run `logmind init` again in each existing repo. Pre-existing workflows that have no `# logmind-template-version:` marker are treated as user-customized and left alone — strip your customizations and re-run init if you want the v1 baseline.

## [0.2.0] - 2026-05-15

### Changed (BREAKING — see migration below)
- **Aggregator removed.** The per-merge `logmind-aggregate.yml` workflow that opened a bookkeeping PR after every feature merge is gone. Replaced by a derived-file architecture: `docs/timeline.md` is now an auto-regenerated chronological view computed from per-branch logs + `docs/decisions.md` + archive on every PR commit. Two PRs in flight can't conflict on the derived file (same inputs → same output).
- **New CLI:** `logmind timeline` prints the unified timeline; `--write PATH` regenerates a file; `--check` exits nonzero if the file is stale (CI gate).
- **New workflow template:** `regen-timeline.yml` runs `logmind timeline --write docs/timeline.md` + `logmind tree` on every PR push, commits the result to the PR branch via `GITHUB_TOKEN` (no PAT needed).
- **AGENTS.md template bumped to v3 / v3-slim.** Adds `docs/timeline.md` to the required reading list as the high-level entry point.
- **`LOGMIND_BOT_PAT` is now vestigial.** The secret is no longer needed anywhere in the v0.2 install footprint. Existing secrets can stay (harmless) or be removed.

### Migration from v0.1.x
1. Delete `.github/workflows/logmind-aggregate.yml` from your repo.
2. Run `logmind init` again to install `regen-timeline.yml` (it skips files that already exist, so other workflows are untouched).
3. Verify branch protection on `main` has "Require branches to be up to date before merging" enabled (strict status checks). Without it, two concurrent PRs editing `docs/timeline.md` may still merge-conflict.
4. Verify `Settings → Actions → General → Workflow permissions = Read and write`. The regen workflow needs to push to PR branches.
5. Run `logmind agents update` to refresh `AGENTS.md` / `CLAUDE.md` / etc. to the v3 marker block.
6. Optional: remove the `LOGMIND_BOT_PAT` repo secret (no longer used).

### Removed
- `src/logmind/templates/github/logmind-aggregate.yml.template`
- `src/logmind/actions/aggregate.py`
- `tests/test_action_aggregate.py`
- LOGMIND_BOT_PAT init-time tip (replaced by the workflow-permissions + strict-status-checks tip)

## [0.1.4] - 2026-05-15
### Added
- Optional `LOGMIND_BOT_PAT` fallback for aggregator PRs under required-check rulesets (made vestigial by v0.2).

## [0.1.3] - 2026-05-15
### Fixed
- file-structure.md regen skipped on feature branches; only regenerates on default branch or in aggregator (replaced by v0.2 regen-timeline workflow).
- `logmind log` now syncs agent files BEFORE the commit so refreshes don't leave dirty trees.

## [0.1.2] - 2026-05-15
### Fixed (from clud-bug PR #21 review)
- Richer default `ignore_patterns` in config.yml (Node/Next.js + general patterns).
- Path-aware `.gitignore` matching in `tree_gen.py`.
- `check-decisions.yml`: `[skip-logmind]` PR-title override actually wired; `THRESHOLD` env threaded; `--no-renames` on `git diff --numstat`.
- `logmind-aggregate.yml`: PR fallback under branch protection (removed entirely in v0.2).
- Scoped staging in `logmind log` (`--stage scoped` default).
- Skill repo restructure to `thrillmot/agent-skills` collection layout.

## [0.1.1] - 2026-05-15

### Added — v0.1.1 polish (Phase 13)
- **`logmind agents update` CLI command** — explicit dry-run / `--apply` / `--commit` upgrade path for the AGENTS.md marker block, complementing the silent auto-refresh on every `logmind log / show / search / agents list`. New public helper `inserter.find_outdated_marker_blocks(root_path)`.
- **Prominent skill-install prompt at init** — the `logmind` agent skill is now surfaced in a visible box BEFORE any files are written, so it doesn't get lost in the "✓ Created ..." output. Adds a closing tip when skill install is skipped.
- **`check-decisions.yml` GitHub Action template** — third workflow shipped by `logmind init` (alongside `logmind-aggregate.yml` and `check-doc-links.yml`). Mirrors the local pre-commit hook against the PR diff; fails the build on >20 non-docs lines without a decision log update.
- **`--install-hook` flag on `logmind init`** — opt-in local pre-commit hook installation in the same step as init.
- **skills.sh install-counter badge** on `README.md` and the `logmind.dev` landing page footer.

### Changed
- **`.github/workflows/claude-review.yml` → `clud-bug` (`thrillmot/clud-bug`)** — replaced the hand-rolled Claude-code-action workflow with the user's first-party `clud-bug` install. Generated workflow skips Dependabot + Renovate PRs (best-practice pattern).
- **`AGENTS.md` template is now adaptive**: ships the slim variant (defers to the `logmind` skill) when skills.sh is on PATH; ships the full variant (procedure inline) when it isn't.
- **`reporulez` (`thrillmot/reporulez`, `external` variant) applied to all 3 repos** (logmind, homebrew-logmind, logmind-skill) — standardised ruleset replacing the manual `gh api branch-protection` rule from v0.1.0. Squash-only merges, linear history, force-push + delete blocked, thread resolution required.
- **`check-doc-links.yml` runs unconditionally** (dropped the `paths: ["**/*.md"]` filter). Required-status-check interacts badly with path-filtered workflows on the reporulez ruleset; ~15s unconditional cost is acceptable.

### Fixed
- **`logmind check-decisions` is now branch-aware**. The CLI command + pre-commit hook used to only accept `docs/decisions.md` updates as documented changes, which made the hook impossible to satisfy on a feature branch under `branch_aware: true` (the default since v0.1). Now accepts `docs/decisions-branches/<branch>.md` too.

### Tests
- 455 → 458 tests passing (3 added: agents update CLI dry-run / apply / idempotency, check-decisions branch-aware regression).

## [0.1.0] - 2026-05-15

### Added — branch-aware logging & open-source readiness (Phases 5–11)
- **Branch-aware decision storage**: feature-branch decisions route to `docs/decisions-branches/<sanitized-branch>.md`; default-branch decisions stay in `docs/decisions.md`. New `decisions.branch_aware` config knob (default `true`).
- **PR-merge aggregator GitHub Action** (`.github/workflows/logmind-aggregate.yml`): on PR close+merge, appends a one-line summary to `docs/decisions.md` linking the PR and the per-branch detail file.
- **Markdown link integrity**: new `logmind check-links` CLI subcommand and `.github/workflows/check-doc-links.yml` workflow. Walks README + agent files + `docs/` for broken or orphan markdown links. Configurable via `linkcheck.allow_orphans` and `linkcheck.roots`.
- **AGENTS.md as canonical agent-instructions hub**: per-tool files (CLAUDE.md, .cursorrules, .windsurfrules, ...) are now 2-line stubs pointing at AGENTS.md. New `logmind agents migrate` consolidates legacy per-agent content into AGENTS.md and replaces files with stubs. JSON agents (cody, zed) unchanged.
- **Tree generation hardening**: `generate_tree()` and the Python fallback now augment `DEFAULT_IGNORES` with the project's `.gitignore` basenames; fallback is unbounded by default with stable dirs-first ordering. New `logmind tree` CLI subcommand for on-demand regeneration. `update_file_structure()` no longer depends on caller cwd.
- **Managed `.gitignore` block**: `logmind init` appends a marker-bracketed block (`.logmind/cache/`, `.logmind/.lock`); idempotent and preserves manual edits inside the markers.
- **logmind agent skill (skills.sh)**: new `skill/SKILL.md` content for the standalone `thrillmot/logmind-skill` repo. `logmind init` offers to install it globally via the user's `skills` CLI (or `npx`). New `--skill-install / --no-skill-install` flag.
- **Open-source readiness**: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, issue + PR templates, `.github/dependabot.yml`, `.pre-commit-config.yaml`, `.github/workflows/test.yml` (Python 3.8/3.10/3.12/3.13 on Ubuntu + 3.12 on macOS/Windows), `.github/workflows/publish.yml` (tag-driven PyPI publish via OIDC + GitHub Release), README badges, `[tool.ruff]` + `mypy` config in `pyproject.toml`.
- Test suite expanded to 446 tests (all passing).

### Added — earlier this cycle (legacy [Unreleased])
- `logmind stats` — analytics dashboard with ASCII bar chart, monthly activity, velocity trend, and top keywords
- `logmind aggregate` — view decisions across multiple projects in a unified feed; `--summary` for per-project counts
- `logmind check-decisions` — pre-commit hook check: exits 1 if staged changes exceed threshold without updating `decisions.md`
- `logmind install-hook` — one-command installation of logmind as a git pre-commit hook
- `logmind templates` — list all built-in decision templates
- `logmind log --template` — pre-fill reasoning, alternatives, and implications from a named template
- 7 built-in templates: `database`, `api`, `architecture`, `security`, `performance`, `library`, `deployment`
- `logmind.integrations.LangChainLogger` — LangChain callback handler for automatic decision logging
- `logmind.integrations.base.BaseIntegration` — base class for building custom framework integrations
- `docs/custom-integrations.md` — guide for building and publishing custom integrations

## [0.1.0] - 2025-10-19

### Added
- Initial release of logmind
- `logmind init` command to scaffold decision logging structure
- `logmind log` command (CLI) and `log()` function (Python API) for logging decisions
- `logmind show` command to view recent decisions
- `logmind search` command to search through decision history
- Automatic git integration (commits and pushes on each decision)
- Configuration system via `.logmind/config.yml`
- Decision archival (keeps 20 most recent, archives older ones)
- Automatic file structure tracking using `tree` command
- Smart insertion into AI instruction files (CLAUDE.md, .cursorrules, etc.)
- `@log_decision` and `@log_choice` decorators for automatic logging
- Template string support in decorators (`{arg_name}` placeholders)
- Comprehensive test suite (110 tests)
- Full documentation in README and docs/

### Features
- **Core Logging**: Append-only markdown files with automatic archival
- **Git Integration**: Every decision = one commit, full audit trail
- **Search**: Regex-based search with context lines and highlighting
- **Configuration**: Customizable commit messages, auto-push toggle, archival threshold
- **Decorators**: Automatic decision logging via function decorators
- **AI-Friendly**: Designed for AI agents (Claude, GPT, Copilot) with clear context

[0.1.0]: https://github.com/thrillmot/logmind/releases/tag/v0.1.0
