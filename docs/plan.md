# logmind: Architecture

> **Sequencing lives in [roadmap.md](roadmap.md), not here.** This document is
> the durable half — what logmind is, how the enforcement layers fit together,
> and why the load-bearing calls were made. It changes yearly. The release
> board changes weekly, and fusing the two is what let the combined document
> describe a config gate for weeks after that gate was deleted.
>
> One owner per fact: if a date, a count, an issue number or an ordering appears
> in both files, `roadmap.md` is right and this one is stale.

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
│   ├── decisions-branches/    # per-branch decision logs — THE record,
│   │                          #   one file per branch (main included),
│   │                          #   append-only and uncapped
│   ├── timeline.md            # derived: the 50 most recent decisions
│   ├── timeline-archive.md    # derived: everything older, same format
│   └── file-structure.md      # derived: project tree
└── [existing project files]
```

## Core Features

### 1. Decision logging

`logmind log "<summary>" -r "<why>" -a "<alternative>" -i "<implication>"`
writes the decision, stages it plus its companion derived docs, and
commits — one command, no separate `git add` / `git commit`. Branch-aware,
with ONE path rule and no exception to it: an entry goes to
`docs/decisions-branches/<sanitized-branch>.md` for the branch it was made
on, and the default branch is a branch like any other (`main.md`).
`docs/decisions.md` is a legacy source: on the branch-aware path it is read
and never written. `logmind init` still SEEDS it, once, with a pointer at the
branch layout — logmind v1.2.0 tests for that path to decide whether a repo is
already initialised, and re-runs its whole scaffold (overwriting
`.logmind/config.yml`) when it is missing, so a v2 tree stays recognisable to
it (`ensureLegacyPointer`, `internal/cli/init.go`). Seeding is not writing
decisions: the pointer body carries no `## ` header, so it contributes no rows
to the timeline, `show`, `search` or `context`. It is not unwritable, and the
exceptions are not a
default-branch carve-out — `resolveDecisionsPath` falls back to it, creating
and appending, in exactly three cases, all of them cases where the router
cannot resolve a branch name to route by: `decisions.branch_aware: false`;
`git` reporting this is not a repository (which also fires when the `git`
binary itself is unreachable, even inside a real repo on a real branch); and
a detached HEAD. A repo with no commits yet is not one of them: `git
symbolic-ref --short HEAD` succeeds before the first commit, so a fresh `git
init` routes to `main.md`.

### 2. The timeline — main-canonical, unconditionally

As of v2.0.0 there is exactly one timeline assembly model:
**main-canonical**, a deterministic, source-derived union of every
decision file in the repo (SPEC §3.3). The older branch-divergent
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
  §2.7 (token discipline) and §2.8 (truncation leaves a mark), so
  `context`/`repomap` output stays consistent across the SkDD toolchain
  (logmind, clud-bug, agent-skills).

### 4. Reading and auditing decisions

`logmind show` (this branch's decisions, plus any legacy `docs/decisions.md` /
`docs/decisions-archive.md` that HOLDS decisions — those are named after no
branch, so no branch file supersedes them, and one holding none has nothing to
surface; `--all` adds every other branch's file, as does having no current
branch at all, since a detached HEAD — every CI checkout — has nothing for
"other branches" to be other THAN), `logmind search
"<keyword>"` (full-text across every decision file that exists, enumerated —
never resolved from a branch name; `--no-archive` opts the legacy archive
out), `logmind headline`
(sets/reads the agent-authored branch summary), and `logmind doctor
[--fix]` (stack-status: version drift, hook drift, missing markers —
`--fix` backfills what it safely can, including timeline markers and
`--file` targeting) round out the read/audit surface.

**Known gap — `doctor` does not probe the v1.2.0 install sentinel.** A repo
missing `docs/decisions.md` is re-scaffolded by any pre-v2.0 binary, losing
`.logmind/config.yml` (`ensureLegacyPointer`, `internal/cli/init.go`).
`logmind init` restores the file on both its paths; `doctor --fix` goes
through `applyRefresh` directly and does neither restore nor report it, so a
repo whose owner only ever runs `doctor --fix` stays exposed and is told
nothing. Reporting it is NOT a one-line probe: `WorkflowStatus` has no
report-without-fixing state, so a `Drift: "missing"` entry would enrol the
sentinel in `doctor --fix` (the widening we declined) and shift the
stale-component counts the `logmind log` pulse advisory asserts. It needs the
status model extended first — same class as the #318 probes that check a
sentinel rather than the thing.

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

> **Section numbers below are from the LIVE SPEC.** SPEC 2.0 was a
> from-scratch rewrite: it has Sections 0–8 and no appendices, and its
> numbering does **not** map to the predecessor archived in the protocol
> repository at `thrillmade/protocol:docs/SPEC-0.7.2-archive.md`. Never
> reason from that document. Citations elsewhere in this repo that still
> point at the old numbering are tracked in
> [#280](https://github.com/thrillmade/logmind/issues/280).

- **Shipped:** main-canonical is now the sole timeline model
  (branch-divergent removed, see Core Features above); the token-killer
  surface (`context`, `repomap` with ranking + `--map-tokens`,
  `LOGMIND_QUIET`, SPEC §2.7–2.8); `show`, `search`, `headline`, and
  `doctor --fix` are all live.
- **Spec version + areas declaration (SPEC §7.3):** `logmind --version`
  prints two lines — `logmind <semver> (spec <semver>)` then
  `areas: orient, work, record, propagate, gates`, from the fixed
  vocabulary. `review` and `versioning` are deliberately not claimed.
  Landed on `dev` as `d79c430`; closes
  [#264](https://github.com/thrillmade/logmind/issues/264) when `dev`
  reaches `main`.
- **Force-logmind-usage enforcement (SPEC §3.4):** the commit-msg hook
  (git layer) and the Claude Code PreToolUse hook (harness layer) block a
  substantive raw `git commit` that bypasses `logmind log`
  (`internal/guardcommit` + `internal/claudehook`); escape hatches are
  `[skip-logmind]`, `LOGMIND_ALLOW_GIT_COMMIT=1`, and
  `git.enforce_commits: false`. The git layer signals a block via exit
  65 and fails open on any other error; the harness layer signals a block
  via exit 2 (the only code Claude Code treats as blocking) and fails open
  on every other.
  **Both layers report their own absence, by different means** — §3.4's
  "failing open MUST NOT be silent" applies to each, and what reaches a
  human differs per layer. The git hook prints a stderr notice on every
  no-decision exit, and carries its installed-by version to the engine in
  `LOGMIND_HOOK_VERSION`. The harness hook cannot do either: it is one line
  run through whatever shell the OS gives Claude Code, where `command -v`
  and inline `VAR=x cmd` are not portable. Instead a missing binary
  announces itself through the shell's own exit 127 — which Claude Code
  surfaces as a non-blocking hook error carrying the command string and its
  version marker, whereas an exit-0 hook's stderr is surfaced to nobody —
  and engine skew is reported from inside the binary, which reads the
  installed marker back out of `.claude/settings.json` and emits a
  `systemMessage`. Both layers share one skew decision (`engineSkewNotice`).
  **§3.4 requires the local hatches and forbids them at the CI gate** —
  "the gate has no self-service escape, and MUST NOT be given one". The
  shipped `check-decisions` template satisfies this as of v7, which
  delegates the judgement to `logmind check-decisions --base/--head`; what
  remains is propagation to the fleet — see *Known gaps* below.
- **Canonical spec-file contract (SPEC §1.5):** an optional,
  forward-looking spec document that leads the cold-start payload,
  defined SkDD-wide (shared across logmind, clud-bug, and agent-skills)
  rather than logmind-specific. logmind's own spec is the pointer doc
  [docs/spec.md](spec.md) (`context.spec_file: docs/spec.md`).
- **The pulse — shipped, and NOT specified by SPEC 2.0:** `logmind log`
  prints stderr advisories after every commit — stale components (per
  `logmind doctor`) and spec staleness (the spec file hasn't been touched
  in 20+ decisions). Advisory only; it never blocks the commit that
  already landed. The predecessor document specified this as §3.1.1;
  **the live SPEC does not mention a pulse at all** (zero occurrences of
  the word). It is therefore a logmind-local feature with no current
  normative backing, which is context for
  [#268](https://github.com/thrillmade/logmind/issues/268) — widening it
  means widening something the protocol does not yet describe.

## Technical Decisions

### Language: Go
- Single static binary — no runtime dependency for end users
- Cross-platform distribution (brew, curl, `setup-logmind` action,
  `go install`)
- Superseded the original Python implementation at v1.0.0. Byte-identical
  output against the frozen Python v0.6.16 baseline was the *migration*
  check for that port; it is now dead history, not a live constraint
- The contract is the SkDD SPEC (`thrillmade/protocol/SPEC.md`, declared by
  `version.SpecVersion`). Conformance is judged against a cited SPEC
  section — never against Python archaeology

### Storage: markdown, no database
- `docs/decisions-branches/<branch>.md` — one file per branch, and the
  default branch is not an exception (SPEC §3.2) — is where every decision
  is written. Files are append-only and **uncapped**: "a decision written
  is a decision kept." Nothing rotates, nothing overflows, nothing is
  archived. There is no separate main log, because `docs/timeline.md`
  already is one.
- What is bounded is the VIEW, not the record (SPEC §3.3): `docs/timeline.md`
  renders the 50 most recent entries and `docs/timeline-archive.md` the
  remainder, both from the branch files on every regeneration. Moving that
  number is a regeneration, not a migration — nothing is transferred between
  the two files and neither is ever read to produce the other.
- `docs/timeline.md`, `docs/timeline-archive.md` and `docs/file-structure.md`
  are derived, regenerated automatically — never hand-edited
- Human-readable, AI-friendly, greppable, works offline, git provides full
  versioning

### Timeline: main-canonical only
- One deterministic, source-derived assembly (SPEC §3.3, "The history and
  the map") replaces the old dual-mode (branch-divergent vs.
  main-canonical) design — less config surface, no risk of the two
  renderers drifting apart
- **§3.3 also caps the rendered timeline at 50 entries**, with everything
  older rendered to `docs/timeline-archive.md`. It is a split in a
  *rendering*, not a move: nothing transfers between files, and both
  regenerate from the same sources every time. It is a **third** derived
  file, and `derivedDocPaths`, the pre-commit restore, `warp`,
  `.gitattributes` (its own driver, `logmind-timeline-archive`) and
  `regen-timeline.yml` all name it.
- **`logmind timeline --write PATH` writes PATH and no other file.**
  `--half recent|archive` picks which half of the split goes into it;
  regenerating both means naming both, which is what the hooks and the
  workflow do. A `--write` that also wrote an inferred sibling put an
  untracked `timeline-archive.md` at the worktree root on every merge
  (git hands a merge driver a scratch file there) and replaced the
  contents of a tracked file of that name where one existed.

### Derived docs: regenerated at the integration point (v2.0.0) — unconditional

> **Superseded design note.** An earlier cut of this feature was gated on a
> per-repo `derived_docs.mode` (`driver` | `integration-point`) adoption
> signal plus a `min_binary` version floor. **Both are gone.** The invariant
> is now unconditional and there is no `derived_docs` key in
> `internal/config`, none in this repo's own `.logmind/config.yml`, and no
> caller of the version floor — `version.SatisfiesMin` survives only as an
> unused general-purpose helper. Per the protocol owner: *do not rebuild the
> `min_binary` floor — it shipped once and was deleted for cause.* The
> surviving mentions of `derived_docs` in `internal/` are comments recording
> the removal, not live config.

- **The invariant:** on a non-default branch,
  `docs/timeline.md` and `docs/file-structure.md` stay byte-identical to
  their merge-base with the default branch. A branch never edits them; they
  are regenerated on the default branch after a merge. Because the branch
  side of the file is unchanged, git takes the default branch's copy
  silently — **a merge conflict on a derived doc is impossible by
  construction**, without relying on the merge driver (which is client-side
  and so cannot run on a server-side merge — the reason PRs still showed
  conflicts before v2.0.0).
- **Why reverting a branch-side edit is always safe:** both files are pure
  functions of committed sources (`timeline.md` of the decision files,
  `file-structure.md` of the tracked tree). Discarding a branch-local edit
  cannot lose information, which is what licenses silent self-heal.
- **Restore target is HEAD, not the merge-base.** Preventive surfaces
  (L0–L2) restore to the branch's own last commit; only the repair path
  (`logmind warp`) may fetch the default branch. Restoring to the
  merge-base on the commit path was ruled against and must not be
  reintroduced.
- **Enforced in four layers, all unconditional:**
  - **L0** — the `post-merge`/`post-rewrite` git hooks regenerate on the
    default branch only, restoring nothing on a non-default branch. The
    hook body is a standalone `sh` script, so the branch check is inlined
    rather than calling the Go binary — one canonical body per hook, so
    `doctor`'s byte-comparison drift detection keeps working unchanged.
  - **L1** — `logmind log`'s `commitDecision` restores both files to HEAD
    before staging, on a non-default branch.
  - **L2 (pin-preservation)** — closes the raw-`git commit` gap L0/L1
    can't reach (e.g. `logmind warp` deliberately writing the default
    branch's newer copy into the working tree, then a plain
    `git commit -am` sweeping it in). **L2a** is a `pre-commit` git hook —
    pure git (`git checkout HEAD -- <path>`), no `logmind` binary
    required, always exits 0 — that restores both files before the commit
    is built. **L2b** is the SAME restore performed inside
    `logmind guard-commit --layer harness` (the Claude Code PreToolUse
    guard), before its allow/block decision — this is what additionally
    catches `git commit --no-verify` (which skips every git hook,
    including L2a) and works in a fresh clone (git hooks aren't cloned;
    `.claude/settings.json`, which invokes guard-commit, is).
  - **L3** — CI's `check-derived-docs` job. It takes **no checkout at
    all**: it reads only the PR's file list via `gh pr diff --name-only`
    and fails if any of the three derived docs appears. Two separate
    things make it non-satisfiable by the change it judges (SPEC §6.3),
    and conflating them is how this document was wrong for a release:
    the missing checkout keeps the **workspace** free of the PR's
    content, and the `pull_request_target` **trigger** is what makes the
    forge read the job's own definition from the base branch. Only the
    second addresses the gate itself. Under the `pull_request` trigger
    this job used to carry, a branch could have edited the `grep` in the
    same diff that edited a derived doc, and no amount of checkout-free
    discipline inside the body would have noticed — the body was the
    branch's. §6.3: "a workspace-free job is not exempt: what the pull
    request rewrote is the definition, not the workspace." The two are
    now both required, and pinned together by
    `TestGateWorkflows_AreNotRewritableByTheChangeTheyJudge`
    ([#261](https://github.com/thrillmade/logmind/issues/261)).
  - **Honest caveat:** L0–L2 are local guardrails, not guarantees — every
    one of them is bypassable (`--no-verify`, deleting or disabling a
    hook, hand-editing `.claude/settings.json`, or simply using a tool
    that never goes through git or Claude Code). **L3 (CI) is the only
    enforcement a contributor's machine cannot reach** — it runs
    server-side on every PR regardless of what did or didn't happen
    locally. "Cannot reach" is the accurate claim and "non-bypassable"
    was not: until #261 the PR itself could rewrite L3's workflow file,
    which is a bypass that needs no local anything. What remains is the
    ruleset bypass of §6.6, which belongs to people on purpose.
- **Staying current on a branch:** `logmind warp` refreshes the working copy
  from the default branch (read-only — it never commits, which would break the
  pin), `logmind context` renders the last-fetched default-branch copy, and the
  pulse nudges when the default branch has advanced.
- Design record: [derived-docs-on-main implementation plan](superpowers/plans/2026-07-17-derived-docs-on-main.md).
  Normative backing: SPEC §3.3 ("Only the default branch writes them. A
  non-default branch MUST NOT modify any derived file — the history, its
  archive, or the map. Each committed copy stays byte-identical to the
  merge-base with the default branch.")

## Known gaps — the enforcement surface is not yet what this document describes

Verified against current source and against each consumer repository's
*installed* copy, not against these templates.

- **The `check-decisions` gate's five defeats are closed in the TEMPLATE
  SOURCE — not in the gate that judges this repo**
  ([#260](https://github.com/thrillmade/logmind/issues/260),
  [#278](https://github.com/thrillmade/logmind/issues/278),
  [#364](https://github.com/thrillmade/logmind/issues/364)). Measured against
  `internal/templates/github/check-decisions.yml.template` on `dev`: the `*.md`
  exclusion and the live-PR-title `[skip-logmind]` read survive only as
  past-tense comments recording the former defects, and the path-match test is
  gone — the gate now shells to `logmind check-decisions --base "$BASE_SHA"
  --head "$HEAD_SHA"` (`:183`), which applies the §3.1 shape check in Go.

  **But logmind's own installed `.github/workflows/check-decisions.yml` is an
  unversioned pre-v5 variant, on both `main` and `dev`, and carries ALL FIVE**:
  the `*.md` exclusion (2 hits), the live-title override (`skip_logmind=true`,
  `dev:130` / `main:77` — the two files differ, 156 vs 103 lines), the
  path-match decision test, a hardcoded `THRESHOLD: "20"` (2 hits, with
  `commit_line_threshold` read 0 times), and its own copy of the exclusion
  list. **Nothing is modernised** — `grep -cE '^[^#]*logmind check-decisions'`
  returns **0** on both branches; the single non-anchored hit is a comment
  saying the file does not yet do this. (Control: the same anchored grep
  returns 2 against the template on `dev`. Scoped deliberately: `main`'s copy of that
  template is still at `v4` (marker present, pre-rewrite) and returns 0 on the
  same probe, so the control demonstrates the probe on `dev` only.) So the gate judging every logmind PR is
  defeatable by retitling — no actor or maintainer check gates `skip_logmind`,
  so any PR author can do it — and we do not run the gate we ship. That is
  #364, and it is pre-tag.

- **`logmind-self-update`'s `[skip-logmind]` marker does not work, and is not
  a legitimate carve-out** ([#365](https://github.com/thrillmade/logmind/issues/365)).
  The template prefixes its own commits `[skip-logmind]` (`:237`) and claims at
  `:230` that consumer gates honour it. They do not: the subject check in
  `guardcommit.Evaluate` (`internal/guardcommit/guardcommit.go:155`) is reached
  only from `logmind guard-commit`, never from `check_decisions.go`, and §3.4
  states a commit-subject marker "is invisible to [the gate] and MUST NOT be
  honoured there." A synthetic self-update-shaped commit run through
  `logmind check-decisions --base/--head` exits **1**. Self-update PRs will fail
  their own gate on any repo running v7 — it has not surfaced only because our
  own gate still honours the title, per the bullet above. Fixing it is a design
  call at protocol, not a patch here.
- **The fleet is running much older copies than this repo ships.** protocol,
  clud-bug, clud-bug-app and agent-skills all run an older
  `check-decisions.yml`; reporulez runs an unversioned copy predating the
  marker; skdd has none on `main` at all. Every installed copy carries both
  the `*.md` exclusion and the skip-marker read. Template versions are **per
  file**, so "the fleet is on vN" is not a single number.

  The version numbers themselves — what this repo ships and what each fleet
  repo runs — live in [roadmap.md § The fleet,
  measured](roadmap.md#the-fleet-measured), which owns that table and is where
  the migration is sequenced. They are deliberately not repeated here: they
  move most releases (this branch alone advances two of them), and a second
  hand-kept copy in an architecture document reads as true until one quietly
  is not. Re-derive what this repo ships with:

  ```bash
  head -1 internal/templates/github/*.template
  ```

  Tracked by [#257](https://github.com/thrillmade/logmind/issues/257).
- **`check-derived-docs` in the fleet is an older shape**, which regenerates
  from the branch's own sources and fails on the diff — enforcing the
  opposite of §3.3.
  [#277](https://github.com/thrillmade/logmind/issues/277) reports this;
  logmind's own copy and its shipped template are already correct, so it is a
  propagation problem carried by #257, not new code here.
- **Hooks resolve their engine by bare name**
  ([#270](https://github.com/thrillmade/logmind/issues/270)), so any PATH
  skew disables the local gate. §3.4 requires fail-open, and that is
  correct; §3.4 also requires that failing open **not be silent**, and that
  half is now satisfied — both fail-open arms print `logmind: commit gate NOT
  RUN` to stderr with the reason and the installing version
  (`internal/hooks/hooks.go:411`, `:426`). The open remainder is the bare-name
  resolution itself.
- **`file_structure.auto_update`** is still declared, defaulted, and written
  into every generated config while being read by nothing
  ([#251](https://github.com/thrillmade/logmind/issues/251)) — including
  this repo's own `.logmind/config.yml`.

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
