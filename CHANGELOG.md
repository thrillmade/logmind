# Changelog

All notable changes to logmind will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.10] - 2026-06-01

### Added — Hook-version drift detection (tokenomics 2026-06-01 bug report)

The tokenomics agent flagged a recurrence of the post-merge hook
checkout-blocking bug AFTER v0.6.9 propagation. Root cause (correctly
diagnosed by the reporter): the local CLI binary was still v0.3.4, so
every `logmind log` overwrote `.git/hooks/post-merge` with v0.3.4's
body (which stages). The workflow-pin upgrade alone does NOT refresh
local hooks; that requires a binary upgrade on the user's box.

v0.6.10 makes this drift LOUDLY visible:

1. **Hook bodies now embed a version marker** — every installed
   post-merge + post-rewrite hook contains a
   `# logmind-hook-version: <X.Y.Z>` line written by the binary that
   created it.

2. **`logmind doctor` extracts the marker** and compares to the
   currently-running binary's `__version__`. Drift surfaces as
   `stale` (marker version differs) or `markerless` (pre-v0.6.10
   hook). Both are aggregated into the overall-drift signal so
   doctor exits non-zero.

3. **`install_post_merge_hook` always rewrites when the body
   differs** — so when a user upgrades binary (v0.3.4 → v0.6.10),
   the next `logmind log` automatically refreshes the local hook
   to v0.6.10's body. Self-healing on first upgrade.

### What this fixes vs doesn't fix

- **Fixes**: future drift is loud. After upgrading to v0.6.10+,
  `logmind doctor` always tells the user when their on-disk hook
  is stale relative to the binary running. One-command fix:
  `pip install --upgrade logmind && logmind log` self-heals.
- **Doesn't fix on its own**: a user stuck at v0.3.4 today can't
  benefit until they upgrade. The reporter's immediate remediation:
  `PYENV_VERSION=3.11.8 pip install --upgrade logmind` then
  `logmind log` once.

### Code

- `src/logmind/core/gitattributes.py` — `HOOK_VERSION_PREFIX` constant,
  `_build_post_merge_hook_body()` + `_build_post_rewrite_hook_body()`
  builders that inject `__version__`, new helpers
  `installed_post_merge_hook_version` / `installed_post_rewrite_hook_version`.
- `src/logmind/core/doctor.py` — probes now extract the marker,
  compare versions, return `drift="stale" | "markerless" | "current"`.
- `tests/test_merge_driver.py` — 5 new tests pinning marker shape,
  extractor, doctor drift detection, and auto-refresh path.

### Deferred to v0.6.11

The full root-cause fix (skip regen when current branch is orphaned
post-merge-away) is captured in the plan as a v0.6.11 candidate. Ships
after v0.6.10 propagates if the doctor warning isn't enough to close
the loop in practice.

## [0.6.9] - 2026-06-01

### Added — `logmind file-structure --check` (symmetric with `timeline --check`)

`logmind timeline --check` has shipped since v0.5.13 — exits non-zero when
the on-disk `docs/timeline.md` differs from a fresh regen, so CI can fail
the build before the auto-fix step. The mirror command `logmind
file-structure` has been missing the same flag; v0.6.9 closes that
asymmetry.

```bash
# Used by CI / pre-commit gates: "are derived docs stale?"
logmind file-structure --write docs/file-structure.md --check
#   → exit 0 + "✓ up to date"  if current
#   → exit 1 + "✗ stale — re-run …" if regen would change the file
#   → exit 2 + "requires --write" if --write is missing
```

The `regen-timeline.yml` workflow template can now use explicit `--check`
calls for both derived docs (cleaner than the current
`--write` + `git diff --quiet` pattern, though that still works fine).

### Context — closes #93 (the v0.5.14 "conflict-tolerant regen merge driver" candidate)

The original framing of "always-fire merge driver to catch silent-success
3-way merge interleaves on `docs/timeline.md`" turned out to be infeasible
by git design — merge drivers only invoke on conflict, and
`pre-merge-commit` hooks don't fire on GitHub's server-side squash-merge
flow (the dominant case). Practical coverage of the original problem has
been built up across multiple releases:

- **v0.5.11** post-rewrite hook (rebases + amends regen + stage)
- **v0.5.12** driver auto-install on fresh clones / CI
- **v0.5.13** `logmind rebase` convenience + doctor stale-derived warning
- **v0.6.7** post-merge leaves regen unstaged (next `logmind log` picks
  up cleanly)
- `regen-timeline.yml` v3 template with `LOGMIND_AUTO_REGEN_PAT`-driven
  auto-fix mode (regenerates derived docs + pushes back to the PR branch
  when the PAT secret is configured; PAT-pushed commits re-trigger
  downstream checks, unlike GITHUB_TOKEN-pushed ones)
- `logmind timeline --check` (CI gate) since v0.5.13

v0.6.9's `file-structure --check` completes the CI-gate symmetry. The
practical problem #93 surfaced is now covered end-to-end:

```text
local merge → post-merge hook regens → unstaged → next `logmind log` stages
       OR
GitHub squash-merge → check-derived-docs workflow on next PR catches stale
       OR (with PAT)
GitHub squash-merge → check-derived-docs auto-regens + pushes back
       OR (in pre-commit / CI)
`logmind timeline --check` + `logmind file-structure --check` exit non-zero
```

No new merge-driver mechanism is shipping. Issue #93 closed with this
context.

## [0.6.8] - 2026-06-01

### Added — `logmind init --with-skdd` (unified install, Python entry)

One-command bootstrap for the full SkDD toolchain. Previously the
install sequence was 3 commands across 2 ecosystems:

```bash
pip install logmind
logmind init
npx clud-bug init
```

The new `--with-skdd` flag collapses the last two:

```bash
pip install logmind
logmind init --with-skdd   # logmind + clud-bug, one command
```

#### Why the bundle naming

The flag name describes the BUNDLE TARGET (the SkDD toolchain),
not a specific tool. As the toolchain grows beyond logmind + clud-bug
(future skills authoring tools, etc.), the same flag stays — no
flag-explosion API surface. The Node side ships a symmetric mirror
in clud-bug v0.6.33 (`clud-bug init --with-skdd`).

#### Behavior

- Default (no flag): logmind init unchanged — Python-only install.
- With `--with-skdd` + `npx` on PATH: subprocesses to
  `npx --yes clud-bug@latest init` after logmind setup completes.
  Stream output to user (not silenced).
- With `--with-skdd` + no `npx`: emits a clear warning with the
  recovery command (`npx --yes clud-bug@latest init`), exit code 0.
- Subprocess failure (non-zero exit, OSError): warning surfaced but
  logmind init still succeeds — clud-bug is an additive layer.

#### Anti-loop guarantee

`logmind init --with-skdd` invokes `npx clud-bug init` (NOT
`clud-bug init --with-skdd`). v0.6.33's mirror does the same
in reverse. Each opt-in only goes one level deep; no mutual
recursion possible.

#### Implementation

- `src/logmind/cli.py`: new `--with-skdd` flag (mirrors the existing
  `--install-hook` opt-in pattern) + `_install_skdd_via_npx()` helper
  with `shutil.which` check + graceful subprocess error handling.
- `tests/test_init_with_skdd.py`: 6 new tests covering flag
  acceptance / no-flag baseline / npx-invocation / no-npx-warning /
  non-zero-exit-warning / OSError-warning.

## [0.6.7] - 2026-06-01

### Fixed — post-merge hook no longer blocks `git checkout main`

Bug surfaced by a downstream agent (2026-06-01): the post-merge hook
auto-staged `docs/timeline.md` + `docs/file-structure.md` after every
merge. The staged-but-uncommitted files then blocked `git checkout main`
on every PR cycle with:

> `Your local changes to the following files would be overwritten by checkout.`

Required the workaround `git reset HEAD <files> && git checkout -- <files>`
every time. Hit every contributor every PR.

#### Root cause

`_POST_MERGE_HOOK_BODY` in `src/logmind/core/gitattributes.py` had a
`git add docs/timeline.md docs/file-structure.md` line after the
regen invocations. The auto-stage is correct semantics INSIDE
`logmind log` (where the regens bundle into the decision commit) but
WRONG from the post-merge hook (which fires after a `git pull --rebase`
following a squash merge — there's no commit being constructed, so
staging just blocks subsequent checkouts).

#### Fix

Remove the `git add` line. The hook now regenerates but leaves the
files as unstaged modifications. The next `logmind log` (or any
explicit `git add`) picks them up cleanly. `git checkout main` no
longer blocks.

#### Auto-propagation

v0.5.12+ `logmind log` self-installs the post-merge hook on every
invocation, with content-drift detection that rewrites stale hooks
(marker + body comparison). So once a consumer pip-upgrades to
v0.6.7 and runs ANY `logmind log`, the fix is in place — no
`logmind init` re-run required.

#### Regression guard

`tests/test_merge_driver.py::test_post_merge_hook_does_not_stage_derived_docs`
asserts the installed hook body contains no anchored `git add docs/timeline.md`
or `git add docs/file-structure.md` line. Future template edits that
accidentally re-add the auto-stage will fail CI with a citation-style
error message pointing back to this entry.

## [0.6.6] - 2026-06-01

### Added — `logmind doctor` surfaces clud-bug skill-usage upload drift

New diagnostic check that warns when a consumer's
`.github/workflows/clud-bug-review.yml` is out of sync with the
clud-bug v0.6.29-v0.6.31 skill-usage upload-step contract:

- Missing `Upload skill-usage artifact` step → consumer pre-v0.6.29.
  Skill-usage data won't accumulate. Suggests `npx clud-bug update`.
- Has the step but missing `include-hidden-files: true` → consumer on
  v0.6.29 or v0.6.30 and silently dropping every artifact (dot-file
  exclusion bug). Suggests `npx clud-bug update` to pick up v0.6.31.
- Step present + flag present → silent (everything's wired correctly).

Surfaces propagation drift early — before the consumer wastes review
cycles producing artifacts that never reach GitHub. Matches the shape
of `check_stale_derived_docs_warning` (predictive heads-up in
suggestions list, not a fatal DRIFT flip).

#### Implementation

- `src/logmind/core/doctor.py`: new `check_clud_bug_skill_usage_integration(project_root)`
  function. Anchored-regex check (`^\s+include-hidden-files:\s*true`)
  matches the v0.6.32 release-discipline guard's pattern in clud-bug —
  the two gates stay aligned. Wired into `collect_status` after the
  existing stale-derived-docs check.
- `tests/test_clud_bug_skill_usage_check.py`: 6 new tests covering
  no-workflow / pre-v0.6.29 / pre-v0.6.31 / current / commented-out /
  defensive-directory cases.

#### Recovery flow

A consumer hitting either warning runs `npx clud-bug update`. That
single command re-renders all workflow files from the latest
templates + bumps composite pins. No manual editing required.

## [0.6.5] - 2026-06-01

### Added — `logmind skill suggest` (Stream 6 follow-on, killed-Stream-9 replacement)

Human-initiated pattern detection that scans recent decision-log
entries for tokens appearing across many distinct decisions — a
heuristic signal that "we keep talking about X, maybe X should have
its own skill." Output is a pre-filled GH-issue draft matching
agent-skills's `new-skill.yml` template. The human reads, decides,
opens (or discards).

**Never auto-PR. Never auto-create a skill.** The whole point of the
pragmatic SkDD pivot (2026-05-30) is that humans gate skill lifecycle.

#### Detection heuristics

- Tokenize decision entries into kebab-case / PascalCase / acronym /
  snake_case identifiers (regex: `_INTERESTING_TOKEN_RE`).
- Filter stop words + generic structural terms (`_SUGGEST_STOPWORDS`).
- Count distinct-decision occurrences (a token mentioned 5x in one
  decision = 1 entry, not 5).
- Drop tokens that match an existing skill name (already covered).
- Rank by # distinct decisions; cap at `--top` results.

#### CLI surface

- `logmind skill suggest` — scan last 30 days, ≥3 decisions, top 5.
- `logmind skill suggest --since 7d --top 10`
- `logmind skill suggest --min-decisions 5` — tighter signal
- `logmind skill suggest --write-drafts /tmp/proposals/` — write each
  suggestion's pre-filled GH-issue body to a `.md` file
- `logmind skill suggest --json` — machine-readable output

#### Implementation

- `src/logmind/core/skill_cli.py`:
  - `suggest_skills_from_decisions(repo_root, since_days, min_decisions, top_n)`
    — returns ranked suggestions
  - `format_suggest_issue_draft(suggestion)` — renders one suggestion
    as the pre-filled GH-issue markdown body
  - `_gather_recent_decisions`, `_excerpt_around`, `_kebab_slug` —
    internal helpers
- `src/logmind/cli.py`: `@skill.command("suggest")` with `--since`,
  `--min-decisions`, `--top`, `--write-drafts`, `--json`.
- `tests/test_skill_cli.py`: 14 new tests (kebab-case detection,
  stopword filter, existing-skill exclusion, threshold semantics,
  intra-entry dedup, main + branch file aggregation, slug
  normalization, draft formatting, CLI rendering, JSON output,
  --write-drafts, bad --since validation).

## [0.6.4] - 2026-06-01

### Added — `logmind skill audit` (Stream 6 follow-on)

Author's-side staleness read for every SKILL.md in `.claude/skills/`.
Pairs with clud-bug's `usage --health` (the enforcement read) for a
complete picture of skill lifecycle:

- **audit**: what's HERE, how big, last-touched, decision-log mention
  count, and a deterministic status (`active` / `aging` / `ghost`).
- **usage --health**: which skills earn their context budget (loads
  vs. citations from real review runs).

#### Status thresholds

- **ghost**: `decision_count == 0` AND `bytes > 2000` — loaded into
  every context but author never iterates; candidate for usage-side
  confirmation + archive.
- **aging**: last-modified > 90 days ago — was useful once, hasn't
  been touched in a quarter.
- **active**: otherwise.

#### CLI surface

- `logmind skill audit` — human-readable table sorted by skill name
  with `name`, `status`, `bytes`, `decisions`, `last touched`. Summary
  line: `ok skill: audit 7 skills (3 active, 2 aging, 2 ghost)`.
- `logmind skill audit --json` — machine-readable array per skill,
  status enriched.

#### Implementation

- `src/logmind/core/skill_cli.py`: `audit_skills(repo_root)` walks
  `.claude/skills/*/SKILL.md`, counts decision-log mentions across
  `docs/decisions.md` + `docs/decisions-branches/*.md`, prefers
  `git log -1 --format=%cs --` for `last_modified` (falls back to
  file mtime). `classify_audit_row(row, now=)` applies thresholds;
  injectable `now` for testability.
- `src/logmind/cli.py`: `@skill.command("audit")` wiring.
- `tests/test_skill_cli.py`: 13 new tests covering directory walking,
  decision counting (incl. branch decision files), every status
  bucket, CLI rendering, JSON output, edge cases (no skills, empty
  directory, invalid date).

## [0.6.3] - 2026-06-01

### Added — `logmind skill bench <name>` (Stream 6 follow-on)

Closes the "measure" arrow of the SkDD loop. Every time an agent loads
a SKILL.md, the whole file enters the context window. `bench` reports
exactly what that costs in bytes + estimated tokens + a status bucket
(`tight` / `typical` / `verbose` / `over-budget`) + per-section
breakdown + trim suggestions when over budget.

Pair with clud-bug's `usage --health` (v0.6.28+) for the complete
picture: bench tells you what each skill COSTS, usage tells you whether
it EARNS that cost in citations.

#### Thresholds

- **tight**: ≤ 2000 bytes (~500 tokens) — focused, well-trimmed
- **typical**: ≤ 6000 bytes (~1500 tokens) — most skills land here
- **verbose**: ≤ 8000 bytes (~2000 tokens) — past budget, suggestions emitted
- **over-budget**: > 8000 bytes — past hard cap, split recommended

#### Suggestions emitted when verbose+

- Dominant section (>30% of total): "Section 'Examples' is N bytes
  (35% of total) — consider linking out OR moving to its own skill."
- Large HTML comment volume: agents load them too; move authoring
  notes to a sibling NOTES.md.
- Past the 8KB hard cap: "split into multiple focused skills."

#### CLI surface

- `logmind skill bench <name>` — human-readable table with section
  breakdown + suggestions.
- `logmind skill bench <name> --json` — machine-readable for automation.

#### Implementation

- `src/logmind/core/skill_cli.py`: new `bench_skill(content, target?, budget?)`
  pure function returning `{bytes, est_tokens, status, sections, suggestions}`.
- `src/logmind/cli.py`: new `@skill.command("bench")` wiring.
- `tests/test_skill_cli.py`: 11 new tests covering all status buckets,
  section parsing, suggestion heuristics, CLI integration.

## [0.6.2] - 2026-05-31

### Fixed — notify-agent-skills no longer opens churn PRs

User flagged (2026-05-30): _"is there any metric around if we should
be doing a change or update? look at them, are they material and
important?"_ — half of recent notify-bot PRs were `+0/-0` for
`skills/logmind/SKILL.md` (just the `.skill-update-todo/<TAG>.md`
context file). 5 stale ones got closed manually; this release
prevents new ones from being opened.

#### Root cause

`notify-agent-skills.yml` (logmind's own workflow, fires on
`v*` tag push) was unconditionally opening a PR even when:

- Claude judged the release skill-irrelevant (`proposed-skill.md` never written), OR
- Claude proposed an update that was byte-identical to the current
  `SKILL.md` on `main`.

Both cases generated a "diff" of `.skill-update-todo/<TAG>.md` only
— pure churn with no actionable signal.

#### Fix

New `Check if SKILL.md actually changed` step runs after the
propose + fetch-current steps. Compares
`/tmp/notify/proposed-skill.md` (if it exists) to
`/tmp/notify/current-skill.md` via `cmp --silent`. Sets
`steps.diff-check.outputs.skill-changed`. The two downstream steps
(checkout agent-skills + commit/push/open-PR) gate on
`skill-changed == 'true'`.

When the diff check shorts the workflow, an
`::notice title=notify-agent-skills::` annotation explains why no
PR was opened. Workflow-run logs preserve Claude's reasoning + the
CHANGELOG section for debugging.

Behavior matrix:

| Claude's verdict | Diff vs current | Pre-v0.6.2 | Post-v0.6.2 |
|---|---|---|---|
| Skill-irrelevant | n/a | TODO-only PR opened | No PR, notice emitted |
| Proposed update | byte-identical | TODO-only PR opened | No PR, notice emitted |
| Proposed update | substantive | PR opened | PR opened (unchanged) |

No CLI changes. No behavior changes for consumers — this is
logmind's own release workflow only.

## [0.6.1] - 2026-05-30

### Added — deterministic auto-rebase on timeline.md gap (opt-in)

User-coined direction (2026-05-30): _"auto rebase must must be very
deterministic and safe, only the timeline md file as you noted"_.

Saves **~5-8 agent turns per DIRTY incident** (the
tokenomics-Phase-D scenario): when a PR batch lands out-of-order,
the trailing branch goes DIRTY because main's timeline.md gained
new entries. Recovery is mechanical (rebase + regen timeline +
force-with-lease push) and entirely contained in one derived doc —
exactly the right case to automate behind a strong opt-in flag.

#### Opt-in configuration

Default OFF. Enable in `.logmind/config.yml`:

```yaml
git:
  auto_rebase: true
```

When enabled, `logmind log` checks 6 conditions BEFORE any local
state changes:

1. `auto_rebase: true` in config
2. Branch is NOT the default branch
3. `git fetch origin <default>` succeeds
4. `origin/<default>` ref exists
5. Branch is behind `origin/<default>` (commits_behind > 0)
6. **The gap touches EXACTLY `docs/timeline.md`** — no code, no
   other derived docs, not even `docs/file-structure.md`

If all 6 hold: rebase + (on conflict only in timeline.md) regen +
continue, then `git push --force-with-lease`. On any unexpected
conflict surface: `git rebase --abort` + bail safely (no
half-applied state).

#### Why the narrow scope

`docs/timeline.md` is fully derived (regenerated from
`docs/decisions.md` + per-branch decision files). Conflict
resolution = re-run the generator. Deterministic by construction.

Other files — even `docs/file-structure.md` — depend on broader
repo state and have subtler conflict shapes. v0.6.1's safety
posture: **single-file scope = trusted scope**. Future versions
may widen, but only after observed behavior justifies it.

#### Always `--force-with-lease`, never `--force`

Tested explicitly in `test_uses_force_with_lease_not_force`.

#### User-visible logging

```
↻ logmind auto-rebased 'feature' onto origin/main
  (was 2 commits behind, only docs/timeline.md affected
  — safely deterministic).
```

When enabled but conditions don't hold, emits a predictive heads-up
naming the disqualifying files.

### Added

- `logmind.core.auto_rebase.maybe_auto_rebase()` — pure function
  returning an `AutoRebaseResult` dataclass; never raises.
- `Config.auto_rebase` property — opt-in via `git.auto_rebase`.
- `logmind.core.logger.log()` — early integration block calling
  `maybe_auto_rebase` BEFORE any local state changes.

### Tests

8 new tests in `tests/test_auto_rebase.py`:
- 6 safety-gate tests (refuses when disabled / on default branch /
  not in git repo / branch up-to-date / gap includes code / gap
  includes file-structure.md)
- 1 happy-path test (fires when gap is exactly `{docs/timeline.md}`)
- 1 force-with-lease invariant test (must NEVER use `--force`)

Full suite: 706 passed, 1 skipped (was 693 at v0.6.0).

### Upstream context

Pairs with v0.5.13's `logmind doctor --check-stale-derived-docs`
(predictive heads-up) + `logmind rebase` (manual action). v0.6.1
is the same detection + same action wrapped in opt-in config.

Does NOT widen scope to `file-structure.md` this release — narrow
scope is the safety lever.

## [0.6.0] - 2026-05-30

### Added — `logmind skill` subgroup (Stream 6 v0.6.0)

First step of the **end-to-end agentic auto-dev loop** the user
coined (2026-05-30): _"development with skills, logging with
logmind to document the changes, then using logmind runnerbot to
forge and update skills, and then cludbug reviews with those
skills and against them, rinse repeat."_

Two CLI subcommands ship in v0.6.0; bench + log subcommands queued
for v0.6.1+.

#### `logmind skill new <name>`

Scaffolds a new SKILL.md against the
[agentskills.io/v1](https://agentskills.io) spec. Composes with
Zak Elfassi's [`@zakelfassi/skdd`](https://zakelfassi.com/skdd-skills-driven-development):

- If `skdd` is on PATH → delegates to `skdd forge <name>` (canonical
  SkDD authoring path)
- Otherwise → scaffolds a basic SKILL.md with required frontmatter
  fields + a TODO trigger description (the discovery surface)

Auto-decision-logs the skill creation via `logmind log` (audit
trail of every skill iteration). Pass `--no-log` to skip.

```bash
logmind skill new my-team-conventions \
    --description "Apply our team's review checklist on every PR"
```

#### `logmind skill test <name>`

Validates a SKILL.md against the spec + logmind layered checks.

- If `skdd` is on PATH → delegates to `skdd validate`
- Layers logmind-specific checks: frontmatter required fields
  (`name` + `description`), soft size cap (8KB — guards against
  skills that bloat every agent load)
- Exits non-zero on any check failure so CI can gate on it

```bash
logmind skill test my-team-conventions
# ✓ skdd validate passed
# ✓ frontmatter required fields: ok
# ✓ size cap: 2934 bytes (cap: 8000)
```

### Architecture (per explore-agent strategic analysis, 2026-05-30)

- **Don't reimplement Zak's `forge` / `validate`** when `skdd` is on
  PATH. Wire to it instead — the canonical SkDD methodology lives
  upstream.
- **Layer logmind-specific value** AFTER his CLI does its work:
  decision-log on creation; size cap + frontmatter checks on test.
- **Scope discipline**: v0.6.0 ships `new` + `test` only.
  - v0.6.1: `bench` (per-call token-cost measurement per skill)
  - v0.6.2: `log` (decision-log a skill iteration, automatable from
    a parallel bot per Stream 9)

### Tests

20 new tests in `tests/test_skill_cli.py`:
- 3 cover `scaffold_basic_skill` (creates valid SKILL.md, refuses
  clobber, TODO when no description)
- 7 cover validation helpers (`check_frontmatter_required_fields`,
  `check_size_cap` — passing + failing cases)
- 8 cover the CLI commands (basic-scaffold path, --no-log, missing
  skill, broken frontmatter, oversized skill)
- 2 cover the skdd delegate path via subprocess mocking

Full suite: 693 passed, 1 skipped (was 673 at v0.5.13).

### Upstream collaboration context

Zak Elfassi's `@zakelfassi/skdd` (npm) is the canonical SkDD CLI.
This release positions logmind as the **complementor**: when his
CLI is available, we wire to it; when it's not, we degrade
gracefully to a basic scaffold. The vendored `skillforge` skill
also lands in agent-skills in parallel (PR #73 there), preserving
his attribution.

### v0.6.0 vs v0.5.x

Minor version bump because `logmind skill` is a new CLI subgroup —
new user-visible surface. No breaking changes; all v0.5.x APIs
unchanged. Consumers upgrading from v0.5.13 → v0.6.0 see the new
subcommand + zero regressions.

## [0.5.13] - 2026-05-30

### Added — 5-item quality batch

1. **AGENTS.md slim template promoted to v7-pointer (§4b.2)** —
   the load-bearing "`logmind log` replaces `git add` + `git commit`
   + `git push`" rule was buried as the second sentence; now lands
   first after the heading. Cap stays under 1500 bytes.
2. **`logmind agents update --apply` sweeps CI workflow pins** —
   closes recurring gotcha #1 (clud-bug update re-renders workflows
   without bumping the `pip install "logmind==X.Y.Z"` line).
   `find_outdated_workflow_pins()` + `update_workflow_pin()` helpers
   in `core/inserter.py`. Sweeps the canonical 4 workflows.
3. **`logmind doctor` exits non-zero on missing merge-driver
   config in git repos** — pre-v0.5.13 a fresh clone silently
   reported OK while one merge away from a check-derived-docs
   failure. Outside a git repo: still OK (no false positives).
4. **`logmind doctor` predictive heads-up: stale-derived-docs
   warning** — when the current branch is behind
   `origin/<default-branch>` AND the gap touches `docs/timeline.md`
   or `docs/file-structure.md`, doctor surfaces "next push will
   likely DIRTY this PR; consider `logmind rebase` now."
   Predictive (doesn't flip overall to DRIFT). Addresses the
   tokenomics agent's Phase D pain.
5. **`logmind rebase` convenience subcommand** — one-command
   wrapper for `git fetch origin && git rebase origin/<base> &&
   git push --force-with-lease`. Flags: `--base`, `--no-push`,
   `--no-fetch`. Refuses on detached HEAD or default branch.
   Clear recovery hints on rebase / push failures.

### Tests

30+ new tests across `tests/test_agents_consolidation.py`,
`tests/test_rebase_cmd.py`, `tests/test_doctor.py`,
`tests/test_workflow_pin_update.py`,
`tests/test_stale_derived_docs_warning.py`. Full suite: 673 pass,
1 skipped.

### Upstream context

Items #4 + #5 address the tokenomics agent's 2026-05-30 Phase D
report (3-PR batch went DIRTY when middle PR merged first). Items
#1 + #3 close prior-plan quality items preserved into this batch.
Item #2 closes a recurring gotcha surfaced across every clud-bug
propagation cycle.

## [0.5.12] - 2026-05-30

### Fixed — `timeline.md` auto-resolves on fresh clones / CI / throwaway worktrees

Pre-fix, `logmind init` was the only path that installed the
per-clone git config + hooks needed for `timeline.md` and
`file-structure.md` to merge cleanly. Fresh clones, CI runners,
and agents working in throwaway worktrees had the committed
`.gitattributes` reference to `merge=logmind-timeline` but no
driver definition registered locally. Git refuses to invoke an
unconfigured merge driver (security guard against untrusted
repos), so it silently fell back to the default ort 3-way merge.
The result was text-valid but semantically incomplete `timeline.md`
output that `check-derived-docs` later flagged — failing the PR
gate even though the merge "succeeded."

Hit live on tokenomics #21 when merging main into a branch
produced `Auto-merging docs/timeline.md / Merge made by the 'ort'
strategy` instead of the expected `logmind-timeline` driver
output. User stance (memory `project_timeline_conflict_should_auto_resolve`):
_"conflicts bugs like this shouldn't happen on our own timeline
file and logmind should auto resolve."_

v0.5.12 makes `logmind log` self-healing. Every invocation now
runs the three idempotent installers from `logmind.core.gitattributes`:

- `configure_merge_drivers(repo_root)` — sets `merge.logmind-timeline.driver` + `merge.logmind-file-structure.driver` in `.git/config`.
- `install_post_merge_hook(repo_root)` — writes the canonical post-merge hook (v0.3.0+).
- `install_post_rewrite_hook(repo_root)` — writes the post-rewrite hook (v0.5.11+).

All three are silent no-ops outside a git repo (the fresh-clone
case isn't always in a git checkout — e.g. when an agent works
in a temp directory). Cost per `logmind log`: ~3 `git config --get`
calls + 2 file stats. Negligible.

The first `logmind log` in any fresh clone now leaves the clone
fully configured for future merges, rebases, and amends. No more
"why isn't the merge driver firing?" mysteries on CI / agents.

### Added

- `logmind.core.logger.log()` — auto-install block at top of
  function, runs unconditionally (idempotent + git-safe).

### Tests

3 new tests in `tests/test_logger.py`:

- `test_log_auto_installs_merge_driver_on_fresh_clone` — fresh-clone
  simulation (.gitattributes committed, no per-clone driver). After
  `logmind log` with `auto_commit=False`, driver + both hooks are
  configured.
- `test_log_auto_install_is_silent_noop_outside_git_repo` — non-git-repo
  invocation doesn't crash. Decision is still recorded.
- `test_log_auto_install_is_idempotent_across_repeated_invocations`
  — second `logmind log` doesn't re-write hook files (regression
  guard against double-install at log() layer).

## [0.5.11] - 2026-05-30

### Fixed — issue #58: multi-commit rebases left `docs/timeline.md` stale

Pre-fix, rebasing a feature branch with 2+ `logmind log` commits
against a moved `main` produced a stale `docs/timeline.md`. The
merge driver in `.gitattributes` only fires when a merge produces
conflicts on the derived files; a clean rebase replays multiple
commits without ever invoking the driver. The companion
`post-merge` hook (added in v0.3.0) also doesn't fire on rebases —
it's strictly a merge-time sweep. Result: only the FIRST rebased
commit's regen survived; subsequent commits left `timeline.md`
unsynced relative to the replayed `docs/decisions-branches/<branch>.md`
entries. `check-derived-docs` failed the resulting PR.

Hit live on agent-skills PRs #21 + #22 during the 2026-05-27
merge cascade. Manual recovery required `logmind timeline --write
docs/timeline.md && git add docs/timeline.md && git commit -m
'chore: regen timeline'`.

logmind now installs a `post-rewrite` hook (companion to
`post-merge`) that regenerates `timeline.md` + `file-structure.md`
after every `git rebase` or `git commit --amend`. Git invokes
post-rewrite once at end-of-rewrite with `$1 = "rebase"` or
`"amend"` — the regen behaviour is identical in both cases, so we
don't branch on `$1`. Same idempotency contract as the post-merge
hook (refuses to overwrite a foreign user-authored hook; canonical
body re-installed on every `logmind init`).

### Added

- `gitattributes.install_post_rewrite_hook(repo_root)` — mirrors
  `install_post_merge_hook`. Idempotent; preserves foreign hooks.
- `gitattributes.post_rewrite_hook_installed(repo_root)` —
  detection helper for `logmind doctor`.
- `logmind doctor` reports `post-rewrite hook` drift alongside
  the existing `post-merge hook` row.
- `logmind init` (both refresh and main paths) installs the
  post-rewrite hook on every invocation — idempotent, so existing
  projects pick it up the next time they re-run `init`.

### Tests

6 new tests in `tests/test_merge_driver.py`:

- `test_install_post_rewrite_hook_creates_executable_hook`
- `test_install_post_rewrite_hook_is_idempotent`
- `test_install_post_rewrite_hook_does_not_overwrite_foreign_hook`
- `test_post_rewrite_hook_installed_detects_logmind_hook`
- `test_post_rewrite_hook_installed_returns_false_for_foreign_hook`
- `test_doctor_surfaces_post_rewrite_hook_status` —
  before-install reports `missing`, after-install reports `current`.
- `test_logmind_init_installs_post_rewrite_hook` — end-to-end via
  `CliRunner` that `logmind init` wires the hook into a fresh repo.

## [0.5.10] - 2026-05-30

### Fixed — issue #59: `--stage scoped` silent-failure on forgotten `git add`

`logmind log "<title>" --stage scoped` stages only logmind-owned
files (decisions.md, file-structure.md, etc.). When a user forgot
`git add` before running it — the failure mode that hit clud-bug
PR #87 and reporulez PR #20 in the 2026-05-27 wrap-up session —
the commit shipped ONLY the decision-log entry. The intended file
change stayed unstaged; the PR diff didn't match its description;
CI reviewed unchanged code; reviewer flagged "PR does not match
description"; user had to push a follow-up commit.

logmind now detects this case and emits a stderr warning naming
the unstaged tracked file(s) plus the fix hint
(`git add <files> && git commit --amend --no-edit`). Warn-not-block
per the Q6 invariant: the user may legitimately have unrelated
tracked WIP they want to keep unstaged, so the commit still
proceeds.

Untracked files (scratch debug artifacts, generated PNGs, etc.) do
NOT trigger the warning — users opting into `--stage scoped`
typically have intentional untracked WIP. Only tracked-but-modified
files surface the warning (i.e., the actual silent-failure
scenario).

### Added

- `git_handler.unstaged_tracked_modifications(path)` helper —
  returns the list of tracked files with unstaged modifications.
  Used by the `--stage scoped` warning code path; reusable for
  future scoped-stage tooling.

### Tests

4 new tests in `tests/test_logger.py`:

- `test_log_scoped_stage_warns_on_unstaged_tracked_modifications`
  — the actual silent-failure scenario; warning fires AND commit
  still proceeds (non-blocking).
- `test_log_scoped_stage_silent_when_working_tree_clean` —
  regression guard against warning-spam on clean scoped commits.
- `test_log_scoped_stage_ignores_untracked_files` — untracked
  scratch files (the `--stage scoped` raison d'être) do NOT
  trigger the warning.
- `test_log_stage_all_never_warns` — `--stage all` sweeps the
  whole tree so the warning code path must never fire.

## [0.5.9] - 2026-05-30

### Fixed — issue #60: check-links parses markdown links inside backticks / code fences

Pre-fix, mentioning markdown link syntax inside backticks (a
documentation-of-the-syntax pattern, e.g. `` `[text](path)` ``) tripped
the link checker even though the bracket-paren wasn't a live link.
Recursion: writing a decision-log entry explaining "I fixed broken
`[text](missing.md)` examples" persisted that very example as plain
markdown, breaking the next CI run. Hit constantly by agents writing
decision-log reasoning that needed to discuss link syntax.

Other markdown linters (markdown-link-check, lychee, etc.) skip code
regions because that's specifically where you discuss the syntax.
logmind now matches that convention via `_strip_code_regions()` which
substitutes fenced blocks (` ``` ... ``` `) and inline-code spans
(`` `...` ``) with whitespace of equivalent length before scanning
for link patterns. Whitespace-replacement (vs. deletion) preserves
line numbers + byte offsets so any broken-link error messages still
point at the correct location in the original file.

### Tests

4 new tests in `tests/test_link_check.py`:
- `test_links_inside_inline_code_span_are_ignored`
- `test_links_inside_fenced_code_block_are_ignored`
- `test_links_outside_code_still_detected_alongside_code_examples`
  (regression guard against silencing legitimate broken-link detection)
- `test_strip_code_regions_preserves_line_numbers`

### Note on v0.5.8

This release is v0.5.9 because v0.5.8 (#82) is in parallel review;
v0.5.9's #60 fix is independent and can ship without depending on
v0.5.8's changes. Once v0.5.8 lands, v0.5.9 rebases cleanly atop it.

## [0.5.7] - 2026-05-29

### Added — `bench/org_cumulative.py` real impl (Phase 0.5 §2)

Closes the last Q7-logmind bench stub. The 4-angle frame is now
fully populated: `per-call` ✅, `worst-case` ✅, `per-session` ✅
(v0.5.6), `org-cumulative` ✅ (this release).

Same data source as `per-session` (real agent session logs at
`~/.claude/projects/*/*.jsonl`), but rolls up differently: instead
of averaging `net_pct` across sessions, sums bytes across all
sessions and all repos to produce one global cumulative `net_pct`
plus per-repo share.

Same informational-only treatment as `per-session` — both share the
`git log --oneline -100` baseline which is too thin to interpret
sign as a quality signal. The load-bearing data:

- `per_repo_share` (dict by cwd) — which consuming repos contribute
  most of the sampled byte volume. Used by Step 4 validation to
  spot per-consumer outliers (>2× median share signals a cache-key
  regression in that consumer's prompt prefix).
- `repos_sampled`, `total_logmind_bytes`, `total_git_bytes` —
  cross-checks against `per-session` (informational invariants
  hold when both populate cleanly).

Sample output on a real dev machine:

```
org-cumulative +130% bytes amortized (across 7 repos, 90 KB logmind / 39 KB git) ℹ info (Step 4 / 0.B.5 / 0.B.6 inputs)
```

### Changed

- `bench/__main__.py` informational set: `per-session` AND
  `org-cumulative` both excluded from exit-gate (both share the
  thin baseline). Verdict label updated from "gates 0.B.5/0.B.6 via
  shares" to "Step 4 / 0.B.5 / 0.B.6 inputs" to reflect the dual
  consumer (org-cumulative feeds Step 4; per-session gates
  0.B.5/0.B.6).
- `run_org_cumulative_stub` retained as a back-compat alias that
  delegates to `run_org_cumulative`; pin-tested by
  `test_org_cumulative_stub_shim_calls_real_impl` so the alias
  can't quietly diverge to the old placeholder shape.

### Tests

3 new tests in `tests/test_bench.py`:

- `test_org_cumulative_no_sessions_returns_stub` — stub-invariant.
- `test_org_cumulative_aggregates_across_sessions_and_repos` —
  end-to-end fixture: two sessions across two distinct logmind
  repos, validates global sum aggregation (not per-session
  averaging) AND `per_repo_share` keys both repos with correct
  weights.
- `test_org_cumulative_per_repo_share_isolates_outlier` — 3:1
  byte ratio across 2 repos, asserts the dominant repo's share
  matches `3000 / 3500` and small repo's matches `500 / 3500`.
  This is the metric Step 4 uses to flag per-consumer cache-key
  regressions.

## [0.5.6] - 2026-05-29

### Added — `bench/per_session.py` real impl (Phase 0.5 Step 1)

The 4th angle of `logmind-bench` (Q7-logmind enforcement) was a stub.
Now walks `~/.claude/projects/*/*.jsonl`, joins `tool_use: Read` events
to their `tool_result` siblings via `tool_use_id`, buckets read bytes
into `docs/decisions.md` / `docs/timeline.md` / `docs/file-structure.md`
/ `AGENTS.md` (+ AGENTS.md-logmind-block sub-bucket), compares to a
`git log --oneline -100` baseline.

Per-session is **informational only** (does NOT gate the exit code) —
the `git log --oneline` baseline is conceptually too thin for the
`net_pct` sign to be a quality signal. The load-bearing data is the
per-file shares (`per_file_share`, `agents_md_block_share`), which
gate the conditional 0.B.5 / 0.B.6 candidates.

Sample output:

```
ok: 4-angle Q7-logmind compliance
  per-call       -18% bytes vs git equivalent      ✅ saver
  worst-case     -58% even on never-read           ✅ saver
  per-session    +352% bytes amortized (9/14 sessions, AGENTS.md=35%, decisions=38%) ℹ info
  org-cumulative (stub — not yet implemented)
```

`bench/` is internal-only (not shipped to PyPI); the change is at the
repo level for nightly bench + decision-rubric inputs.

### Changed — `AGENTS.md` logmind-block trimmed v5-slim → v6-pointer (Phase 0.5 / 0.B.6)

Block compressed from 2526 bytes / 48 lines to ~770 bytes / 12 lines
(**~ 69 % reduction**) — drops the inline 5-step procedure +
"Required reading" list, keeps the load-bearing "logmind log is the
commit primitive" rule + bash example + skill pointer to
`agent-skills/skills/logmind`. Full procedure now lives entirely in
the skill (which most agent runtimes auto-load).

**Per-session data justifying the ship** (`python -m bench` on a real
machine with 14 sampled logmind-repo sessions, 9 with decision-doc
reads):

- `per_file_share[AGENTS.md] = 0.36` ≥ 0.20 threshold ✅
- `agents_md_block_share = 0.51` ≥ 0.30 threshold ✅

(See `/Users/ludlow/.claude/plans/ok-here-is-recent-distributed-chipmunk.md`
Step 3 rubric.)

**Savings:** ~1.8 KB per repo per AGENTS.md read. Across 6 consuming
repos × per-session reads, compounds quickly.

### Migration

Consuming repos pick up the v6-pointer block on next `logmind doctor`
+ refresh cycle. `inserter._replace_marker_block` handles the swap
idempotently. The v6 block is a strict subset of v5's information
(everything trimmed lives in the linked skill); no agent action lost.

### Tests

`test_get_agents_md_template_returns_slim_when_skills_available` updated
to assert `v6-pointer` marker, the "commit primitive" rule, the
git-trio-replacement phrase, the skill-URL pointer, AND a 1500-byte
hard cap on the slim block to prevent future re-bloating.

### Composite pin

No clud-bug composite changes this release.

## [0.5.5] - 2026-05-29

### Added — RTK-inspired fail-safe patterns (Phase 0.5 / 0.0.T, logmind side)

Two surgical patterns lifted from RTK (MIT-licensed) into logmind's
parser + atomic-write paths. RTK is not adopted as a dependency; only
the patterns ship.

**Parser warn-not-silent** (`src/logmind/core/parser.py`):
`iter_decisions` previously caught `ValueError` on malformed decision
header dates (e.g. `"## 2026-13-45 25:99 - title"` — regex matches but
the date doesn't parse) and silently dropped the entry. Now emits a
stderr warning naming the file + lineno + parse error, then skips.
Silent drops were the Phase 0 hindsight bug this addresses.

```
  ! logmind: skipping malformed decision header at
  docs/decisions-branches/foo.md:14: month must be in 1..12
```

**Atomic-write orphan cleanup** (`src/logmind/core/atomic_io.py`):
`atomic_write_text` previously left an orphaned `.tmp` sibling behind
if the write failed mid-flight. Now catches `BaseException` (covers
`KeyboardInterrupt`), unlinks the orphan with `missing_ok=True`, then
re-raises the original exception. Cleanup is best-effort: if the
unlink ALSO fails, the original exception still propagates so the
real error is never masked.

### Tests

+6 tests in `tests/test_fail_safe_0_0_T.py`: malformed-date warning,
no warning on clean / missing files, tmp cleanup on write failure,
original exception preserved when cleanup also fails, happy path
unchanged. Full suite: 614 passed, 1 skipped.

### Impact

Quality boost (Q6 — never silent-drop warnings). No measurable token
cost change.

## [0.5.4] - 2026-05-28

### Added — `docs/timeline.md` ships brief format on disk (Phase 0.B.4)

The on-disk timeline now defaults to a token-frugal brief layout: per
month, render the newest + oldest decisions with an elision line
between them, plus the month-total count in the header. Months with
≤2 decisions render every entry (nothing to compress).

Example (brief, default):

```
## 2026-05 (23 decisions)

- **2026-05-28** — newest title *(main)* — [docs/decisions.md](docs/decisions.md)
- *... 21 more decisions ...*
- **2026-05-01** — oldest title *(feat/foo)* — [link](link)
```

Pass `--full` for the legacy per-decision listing:

```bash
logmind timeline --full                              # full to stdout
logmind timeline --write docs/timeline.md --full     # full on disk
```

### Impact

Every consuming repo's `docs/timeline.md` shrinks on next regen. For
a repo with 5 months × 20 avg decisions, `docs/timeline.md` drops
from ~100 lines to ~25 lines (~75% reduction). Source files
(`decisions.md`, `decisions-branches/*.md`, `decisions-archive.md`)
remain user-owned and untouched.

### Tests

- `tests/test_timeline.py`: brief default produces month header with
  count; ≥3-entry months show first+elision+last; ≤2-entry months show
  every entry; `--full` recovers legacy format; brief strictly shorter
  than full on representative fixtures; rendering still deterministic.

## [0.5.3] - 2026-05-28

### Fixed — `LOGMIND_QUIET=1` now also suppresses `click.secho` progress chatter

v0.5.1 monkey-patched `click.echo` but intentionally left `click.secho`
untouched on the rationale "errors via secho(fg='red'/'yellow') still
print." Side effect: progress lines that also use secho (the `ℹ Default
--stage all` cyan notice, `✓ Logged decision` green success) slipped
through unsuppressed — visibly defeating the token-frugal mode for the
most common command (`logmind log`).

Fix: monkey-patch `click.secho` too, with a small wrapper that
suppresses when `_QUIET` UNLESS `fg` is one of `_LOUD_COLORS` =
`{red, yellow, bright_red, bright_yellow}`. Errors and warnings still
print; progress chatter goes away.

Concrete user-visible change with `LOGMIND_QUIET=1 logmind log "..."`:

```
# Before (v0.5.1, v0.5.2):                 # After (v0.5.3):
ℹ Default --stage all (v0.2.7+): ...       ok logged: <sha> "..."
✓ Logged decision: "..."
ok logged: <sha> "..."
```

3 lines → 1 line. ~67% reduction on the most common quiet-mode call.

### Tests

- `tests/test_cli.py`: assert `LOGMIND_QUIET=1 logmind log` emits only
  the single `ok logged:` line (no `ℹ`, no `✓`); assert error/warning
  secho paths still print under quiet mode.

## [0.5.2] - 2026-05-28

### Added — `logmind show --brief` / `--limit` / `--json` (Phase B.2)

Agent-friendly views on the `show` command. Pure additive — existing
`logmind show` and `logmind show --all` behavior unchanged.

- `--brief`: one-line summary per decision (`YYYY-MM-DD HH:MM — title [source]`). Cheap recall for agents that need context but don't need full prose. Uses `iter_decisions` from `logmind.core.parser` (already exists).
- `--limit N` / `-n N`: cap to N most-recent entries (newest-first). Matches the `logmind aggregate --limit` convention. Combinable with `--brief` and `--json`.
- `--json`: stable structured output for downstream tools. Array of `{date: ISO8601, title: str, source: "main" | "archive"}`. Bypasses the `--quiet` patch so JSON is always emitted to stdout (it's primary output, not progress).
- All three combine: `logmind show --brief --limit 5` for quick last-5 recall, `logmind show --json --limit 10 --all` for parsed access across main + archive.

### Tests

- `tests/test_cli.py` (+3): brief shape (newest-first), `--limit 2` caps correctly, `--json` produces a valid parseable array.

## [0.5.1] - 2026-05-27

### Added — `--quiet/-q` flag + `LOGMIND_QUIET=1` env var (Phase B.3)

Mirrors clud-bug v0.6.7's RTK-style `ok <key-value>` pattern. Agent
invocations of `logmind log` / `init` / `show` / `tree --write` /
`file-structure --write` / `timeline --write` can now opt into a
token-frugal mode that suppresses progress chatter and emits exactly
one final `ok <key-value>` summary line.

| Command | Quiet output (always emitted, even without --quiet) |
|---|---|
| `logmind log "..."` | `ok logged: <sha> "<decision[:60]>"` |
| `logmind init` | `ok initialized: docs/ .logmind/ workflows @vX.Y.Z` |
| `logmind show` | `ok show: docs/decisions.md (N bytes[ + archive])` |
| `logmind tree` | `ok docs/file-structure.md (N bytes, depth=N/default)` |
| `logmind file-structure` | `ok <path-or-stdout> (N bytes, depth=N/unbounded)` |
| `logmind timeline` | `ok timeline: <path-or-stdout> (N bytes)` |

### Activation

Either pass `--quiet` / `-q` to the `logmind` group, or export
`LOGMIND_QUIET=1` in the environment.

### Behavior

- `_ok(...)` ALWAYS emits, even without quiet (positive confirmation
  for agents that parse stdout).
- `click.echo` is monkey-patched at module load so all 185 existing
  call sites become quiet-aware without per-call edits.
- `click.secho(fg="red"/"yellow")` (warnings + errors) is UNTOUCHED —
  quiet doesn't silence real problems.

### Tests

- `tests/test_cli.py` (+3 new): `--help` advertises the flag,
  `show --quiet` on a fresh repo emits a single `ok` line,
  `LOGMIND_QUIET=1` env var doesn't break `--help`.

## [0.5.0] - 2026-05-27

### Changed — `docs/file-structure.md` ships at max-depth 2 by default

Phase B.1 of the org-wide token-cost compression roadmap. The biggest
single token sink in the org (logmind's own `docs/file-structure.md` at
103 KB) drops to ~10 KB on next regen across every consuming repo.

- `generate_file_structure(repo_root, max_depth=2)` is now the default. The depth-truncation code already existed in `_generate_fallback_tree`; v0.5.0 activates it via the public API.
- `generate_tree(...)` accepts a new `max_depth: int | None = None` keyword. The system `tree(1)` path passes `-L max_depth+1`; the Python fallback recurses with the existing `_current_depth` check.
- `write_file_structure(target, max_depth=...)` threads the cap through. `update_file_structure()` (called by `logmind init` + the post-merge auto-regen) picks up the default automatically.
- New CLI flag on `logmind file-structure` and `logmind tree`: `--max-depth N` (default 2). `--max-depth 0` requests the full tree.
- `file-structure.md` template footer notes the truncation and directs readers to `--max-depth 0` / `logmind tree --max-depth 0` for the full view.

### Why this compounds

Every clud-bug review on a PR that touches the tree ingests the
file-structure.md diff. Every agent session that `cat`s the file pays
the byte cost. Dropping from 103 KB → 10 KB in logmind alone, and
similar reductions in 6 other consuming repos, compounds on every
future read.

### Tests

- `tests/test_tree_gen.py` (+4 new): default depth=2 truncates deep trees; `max_depth=None` is unbounded; depth-2 default is strictly shorter than unbounded on a deep tree; `write_file_structure` honors the default.

## [0.4.0] - 2026-05-27

### Changed — `notify-agent-skills.yml` opens a PR (not an issue) with a Claude-generated proposed SKILL.md update
- **The reshape.** Every logmind tag push has been opening an *issue* on `thrillmade/agent-skills` titled `logmind vX.Y.Z shipped — review skills/logmind/SKILL.md`. The maintainer then has to read the CHANGELOG, decide what's skill-relevant, write the edit, and PR it themselves. The notification is issue-shaped; the actual work is PR-shaped. v0.4.0 inverts this: the workflow now opens a **PR** on agent-skills with a **Claude-generated proposed SKILL.md edit** based on the release's CHANGELOG section. Reviewer-checklist body, no auto-merge, full reasoning attached.
- **Mechanism.** New helper script `.github/scripts/propose_skill_update.py` calls the Anthropic API (`claude-sonnet-4-6`) once per release with: the CHANGELOG section sliced via `logmind.core.changelog.extract_sections_between`, plus the current `skills/logmind/SKILL.md` fetched from agent-skills' main. Claude emits either (a) `<reasoning>` + `<skill_md>` XML blocks containing the proposed edit, or (b) a `NO_SKILL_UPDATE_NEEDED` sentinel for releases with no user-facing changes worth surfacing to AI agents. Either way, a stub `.skill-update-todo/vX.Y.Z.md` is always written with full context (CHANGELOG section, Claude's reasoning, release URL).
- **Auth (already in place).** Same `AGENT_SKILLS_NOTIFY_PAT` from v0.3.x (just expanded scope to Contents + PRs: write, configured earlier in the migration session). `ANTHROPIC_API_KEY` is the existing org-level secret. No setup step required on existing installs.
- **Failure-mode degradation.** If the Anthropic API call fails or the PAT is missing, a fallback job opens the v0.3.x-shaped issue so the release still gets acknowledged. Shipping discipline preserved — a release shouldn't break because the new flow flakes.
- **Tests.** `tests/test_propose_skill_update.py` mocks the Anthropic client and exercises: full-proposal parsing, sentinel parsing, sentinel precedence over stray blocks, malformed-response degradation, missing-key/missing-package guards, end-to-end main() with both proposal and sentinel paths.

### Future enhancements considered (not in v0.4.0)
After 1-3 release dogfoods we'll know whether to:
- **Prompt-tune** if Claude's proposed edits are consistently off (e.g. N-shot examples drawn from past SKILL.md commit diffs).
- **Auto-merge gate** on a confidence-score threshold (request a JSON confidence in the structured output, auto-merge when >0.9). Today's design intentionally keeps auto-merge OFF — content is discretionary.
- **Extend to symmetric flows** if another downstream-sync surface emerges (clud-bug → some-future-consumer, etc.). The two-file shape (changelog slice + current target → propose) generalizes cleanly.

### Migration from v0.3.4
None required on existing installs. The reshape is server-side (the workflow file inside logmind itself). After this release tags, the next agent-skills notify will arrive as a PR rather than an issue — open it, scan Claude's reasoning, merge or close.

## [0.3.4] - 2026-05-27

### Added — `check-derived-docs` auto-fixes stale docs/timeline.md + docs/file-structure.md when configured
- **The pain.** v0.2+'s derived-doc model treats `docs/timeline.md` + `docs/file-structure.md` as artifacts that must always be in sync with sources. Today's `check-derived-docs` workflow VERIFIES sync — if stale, it fails red and tells the author to regenerate locally. That works, but it makes merging harder: every rebase, every concurrent PR, every multi-commit branch push hits the same papercut. We were burning multiple cycles per session on "regen, add, commit, push" sequences that shouldn't need a human.
- **The shape.** The shipped `regen-timeline.yml` template now runs in two modes. When the repo (or its org) has a `LOGMIND_AUTO_REGEN_PAT` secret configured, the workflow regenerates stale derived docs and pushes the fix back to the PR branch using the PAT — downstream CI re-runs naturally, the merge gate clears on its own. When no PAT is configured, falls back to today's fail-fast behavior (the warning explains how to opt in). Forked PRs always run in fail-fast mode (can't push to forks).
- **Why a PAT.** GitHub deliberately blocks `GITHUB_TOKEN`-pushed commits from re-triggering required-status workflows (anti-recursion safety). Auto-commit via `GITHUB_TOKEN` would leave the merge gate stuck on "Expected — Waiting for status to be reported" forever. The PAT path is the documented escape hatch.
- **Setup (one-time per repo or per org).** Create a fine-grained PAT scoped to the target repo(s) with `Contents: write`. Add it as `LOGMIND_AUTO_REGEN_PAT` under Settings → Secrets and variables → Actions. Already-installed `logmind init` repos pick up the new behavior on the next `logmind init` invocation (refresh-mode bumps the workflow's template-version marker from v2 → v3 and rewrites the file).
- **Same context name.** The workflow still reports under the `check-derived-docs` status check — required-status rulesets and existing branch protection keep working without changes.

### Migration from v0.3.3
Run `logmind init` in each installed repo to pull the new `regen-timeline.yml` body. The behavior is backwards-compatible without setup — fail-fast mode is identical to v0.3.3's behavior. Opt into auto-fix by adding the `LOGMIND_AUTO_REGEN_PAT` secret.

## [0.3.3] - 2026-05-27

### Fixed — post-merge hook no longer re-stages `docs/file-structure.md` on every `git pull`
- **Root cause.** `generate_file_structure()` emitted a wall-clock `Last updated: YYYY-MM-DD HH:MM:SS` line on every regen. The `.git/hooks/post-merge` shipped by `logmind init` v0.3.0+ unconditionally calls `logmind file-structure --write docs/file-structure.md` and then `git add`s it. After a `git pull` that brought in a CI-generated snapshot with timestamp T1, the local regen wrote the same tree with timestamp T2 (now) — producing a stale-looking staged-modified diff on every pull, even when the working tree was byte-identical. Users had to either commit timestamp-only noise or `git restore --staged` after every pull.
- **Fix.** Dropped the `Last updated:` line entirely. `docs/file-structure.md` is now a pure function of the working tree: identical trees render to byte-identical files. The header still announces the file is auto-generated by logmind. Git's own `log -- docs/file-structure.md` gives the actual "when" anyway.
- **Downstream effect.** `write_file_structure()` already returned `False` when `existing == rendered`. With deterministic content that path now triggers on unchanged trees — `atomic_write_text` is skipped, the file on disk doesn't change, and `git add` in the post-merge hook is a no-op. No hook change required; the cause was upstream.
- **Test added.** `tests/test_tree_gen.py::test_update_file_structure_is_deterministic` regens twice over a fully-realized tree and asserts byte-stability.

### Migration from v0.3.2
None required for new clones. Existing checkouts get the fix on their next `pip install -U logmind`; the next `logmind file-structure --write` will produce the timestamp-free format. The first pull after upgrade may still show a one-time diff (the merged-in file from a v0.3.2-era CI run still has the timestamp); commit it or pull the regenerated version from CI on the next push.

## [0.3.2] - 2026-05-27

### Changed — homebrew tap bump now self-merges
- **`.github/workflows/homebrew-bump.yml` now squash-merges the tap PR synchronously after opening it.** The bump is mechanical (formula `url` + `sha256`, already verified against `pypi.org/pypi/logmind/<version>/json` upstream in the same workflow). No human judgment adds value, and `thrillmade/homebrew-logmind` has no required checks, so `gh pr merge --squash --delete-branch` resolves the PR immediately without the repo-level `Allow auto-merge` precondition that `--auto` would otherwise require. End-to-end: tag push → PyPI publish → tap PR opened → tap PR merged → `brew install logmind` resolves the new version, all without human touch.
- **Nothing-to-commit guard** — if the formula already points at the target version (workflow re-run on the same tag, or the formula was edited by hand first), the workflow exits 0 cleanly instead of erroring on an empty commit. The desired end-state is reached either way.

### Changed — `site/app/page.tsx` GitHub-org URL refs
- **Marketing site GitHub-org links now point at `thrillmade`.** The v0.3.1 bulk sed sweep used `--include='*.md' '*.yml' '*.json' '*.template' '*.py' '*.js' '*.toml'` and missed `.tsx`, leaving 12 GitHub-org refs in `site/app/page.tsx` (brew tap copy, skill install copy, github navlinks, CHANGELOG / CONTRIBUTING / issues / security-policy links, skills.sh agent-skills badge). Replaced. Personal-brand `thrillmot.com` refs (lines 306–319) intentionally preserved — those point at the human author's site and don't migrate with the org.

### Migration from v0.3.1
None required. The homebrew-bump change takes effect on the next tag push (v0.3.2 itself dogfoods it). The marketing site change is cosmetic for the logmind.dev site.

## [0.3.1] - 2026-05-27

### Changed
- **GitHub org migration: `thrillmot` → `thrillmade`.** All `thrillmot/<repo>` URL references in shipped templates and project metadata now point at `thrillmade/<repo>`. Surfaces touched:
  - **Shipped templates** (auto-refreshed in downstream installs on next `logmind init`):
    - `src/logmind/templates/AGENTS.md.template` + `.slim.template` — skill install URL `npx skills add https://github.com/thrillmade/agent-skills --skill logmind`. Block-version markers bumped `v4 → v5` and `v4-slim → v5-slim` so v0.2.1+ refresh-mode rewrites the AGENTS.md block in-place across every installed repo.
  - **Project metadata** (visible on PyPI package page):
    - `pyproject.toml` `[project.urls]` — Homepage, Documentation, Repository, Bug Tracker, Changelog all → `thrillmade/logmind`.
  - **Repo workflows**:
    - `.github/workflows/notify-agent-skills.yml` — opens issues at `thrillmade/agent-skills` on tag push.
    - `.github/workflows/homebrew-bump.yml` — bumps formula at `thrillmade/homebrew-logmind`.
  - **Code constant**:
    - `src/logmind/core/skill_install.py` `DEFAULT_SKILL_SOURCE` → `https://github.com/thrillmade/agent-skills`.
  - **Docs**: README, AGENTS.md, CLAUDE.md, CONTRIBUTING.md, custom-integrations.md, ISSUE_TEMPLATE, clud-bug-collaboration SKILL.md cache, CHANGELOG historical entries, decision-log entries.
  - **Tests**: `test_skill_install.py` + `test_templates_v0_1_2.py` assertion strings updated to expect the new org.

### Migration from v0.3.0
Run `logmind init` in each installed repo. v0.2.1+ refresh-mode detects the marker bump (v4 → v5) on AGENTS.md and rewrites the block automatically; v0.2.3+'s changelog-on-upgrade prompt prints this section inline.

`thrillmot/<repo>` URLs continue to work indefinitely via GitHub's auto-redirect, so existing pinned references (e.g. `pip install "logmind==0.3.0"` workflows) don't break. The thrillmade URLs are the new canonical.

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
- **`notify-agent-skills.yml` workflow** on this repo. Mirrors the `notify-clud-bug.yml` pattern shipped on `thrillmade/agent-skills`: on every tag push (`v*`), opens an issue on `thrillmade/agent-skills` titled `logmind <tag> shipped — review skills/logmind/SKILL.md`. Closes the manual-sync gap that left the skill out of date with v0.2.3 → v0.2.5 features until someone (an agent, in this case) noticed and shipped a batch update.

  - Same auth model as the agent-skills→clud-bug notifier: needs `AGENT_SKILLS_NOTIFY_PAT` repo secret (fine-grained PAT scoped to `thrillmade/agent-skills` with `Issues: write`). Without it, the notifier degrades to a `::warning::` and the release itself succeeds.
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
- Skill repo restructure to `thrillmade/agent-skills` collection layout.

## [0.1.1] - 2026-05-15

### Added — v0.1.1 polish (Phase 13)
- **`logmind agents update` CLI command** — explicit dry-run / `--apply` / `--commit` upgrade path for the AGENTS.md marker block, complementing the silent auto-refresh on every `logmind log / show / search / agents list`. New public helper `inserter.find_outdated_marker_blocks(root_path)`.
- **Prominent skill-install prompt at init** — the `logmind` agent skill is now surfaced in a visible box BEFORE any files are written, so it doesn't get lost in the "✓ Created ..." output. Adds a closing tip when skill install is skipped.
- **`check-decisions.yml` GitHub Action template** — third workflow shipped by `logmind init` (alongside `logmind-aggregate.yml` and `check-doc-links.yml`). Mirrors the local pre-commit hook against the PR diff; fails the build on >20 non-docs lines without a decision log update.
- **`--install-hook` flag on `logmind init`** — opt-in local pre-commit hook installation in the same step as init.
- **skills.sh install-counter badge** on `README.md` and the `logmind.dev` landing page footer.

### Changed
- **`.github/workflows/claude-review.yml` → `clud-bug` (`thrillmade/clud-bug`)** — replaced the hand-rolled Claude-code-action workflow with the user's first-party `clud-bug` install. Generated workflow skips Dependabot + Renovate PRs (best-practice pattern).
- **`AGENTS.md` template is now adaptive**: ships the slim variant (defers to the `logmind` skill) when skills.sh is on PATH; ships the full variant (procedure inline) when it isn't.
- **`reporulez` (`thrillmade/reporulez`, `external` variant) applied to all 3 repos** (logmind, homebrew-logmind, logmind-skill) — standardised ruleset replacing the manual `gh api branch-protection` rule from v0.1.0. Squash-only merges, linear history, force-push + delete blocked, thread resolution required.
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
- **logmind agent skill (skills.sh)**: new `skill/SKILL.md` content for the standalone `thrillmade/logmind-skill` repo. `logmind init` offers to install it globally via the user's `skills` CLI (or `npx`). New `--skill-install / --no-skill-install` flag.
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

[0.1.0]: https://github.com/thrillmade/logmind/releases/tag/v0.1.0
