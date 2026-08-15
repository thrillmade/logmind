# logmind — roadmap to v2.0.0

> **This file is the source of truth for sequencing.** The roadmap artifact
> renders from it.
>
> [#243](https://github.com/thrillmade/logmind/issues/243) is the tracking
> thread and points here. It carried its own ordering table until 2026-08-14,
> which meant two orderings of record — the exact defect this split was made to
> remove, recreated one revision later. That table is gone; if the two ever
> disagree again, this file is right.
>
> Architecture lives in [plan.md](plan.md) — that changes yearly,
> this changes weekly, and fusing them is why the old combined document went
> stale.
>
> **Every number here was measured, with the command shown.** Nothing is carried
> from memory or from an issue body. A claimed zero is stated with the control
> that proves the probe finds a non-zero when one exists.

**Verified 2026-08-14** against `origin/dev 7b2b4f8`, `origin/main 0aa9049`, and
the live SPEC at `thrillmade/protocol` blob `cd64e5c` (1,475 lines, Sections 0–8,
no appendices).

---

## State

| | |
|---|---|
| latest **release** | **v1.2.0** (2026-06-07) — what `brew install` gives |
| `main` | `0aa9049` |
| `dev` | `a0f6339`, **7 ahead** of main |
| open issues | 31 |
| open PRs | 0 |

Everything built since June is unreleased. The released binary predates the
commit gate, the harness guard, the zero-conflict invariant, and the `areas:`
declaration.

### How work is verified

`dev` deliberately has **no CI** — both workflows trigger on
`push: branches: [main]` only. The bar is a **local adversarial panel** per
change, plus the full suite run against integrated `dev`. Pull requests into
`dev` still get CI on their own merge-ref (the `pull_request` trigger carries no
branch filter); what the panel and the local suite cover is the *integrated*
result, which PR-level CI never sees.

---

## Landed on `dev`, not yet on `main`

| commit | what | closes on merge to main |
|---|---|---|
| `a0f6339` | CI gate calls the verb it claimed to mirror | #278, #260, #284 |
| `4499da1` | file-structure root from the repository, not the checkout dir | #285 |
| `4120584` | refuse to move a workflow template backwards | #286 |
| `9889a25` | one shared §3.4 evaluation | — |
| `575272b` | 19 documentation claims corrected | — |
| `46a3a58` | `plan.md` refreshed against SPEC 2.0 | — |
| `d79c430` | declare spec 2.0.0 + the §7.3 `areas:` line | #264 |

Those issues stay **open** until `dev` reaches `main` — `closes #N` only fires on
the default branch. That is expected, not drift.

---

## Blocked on a person

### #288 — the bypass is granted; it has never been exercised

**Not a blocker on the tag.** An earlier revision of this file said the steward
App was on no bypass list and called that "the tightest constraint on the tag."
**That was wrong**, and the correction matters more than the original claim.

```
$ gh api repos/thrillmade/logmind/rulesets --jq '.[]|"\(.id) \(.name)"'
18502737 org-baseline
16898453 org-default-protection

$ gh api repos/thrillmade/logmind/rulesets/18502737 \
    --jq '[.bypass_actors[]|"\(.actor_type):\(.actor_id)"]'
["OrganizationAdmin:null", "Integration:3951953"]

$ gh api /apps/skdd-steward --jq .id
3951953
```

Two active rulesets, not three — `reporulez-default` (18292242) returns **404**
here and no longer covers this repository. Both survivors carry
`Integration:3951953`, which is `skdd-steward`. Their `updated_at` is
**2026-08-07T17:1x**.

### What is actually unresolved

The push has still never landed a commit, and the reason is *not* a refusal:

| probe | result |
|---|---|
| regen commits on `main`, by subject line | **0** |
| control — same probe, protocol PR#75 | **30** |
| `regen-timeline` runs, all events | 503 |
| runs on the `push` event | **15** |
| `push` runs that **succeeded** | **13** |

Thirteen green push runs and zero commits means the job runs, finds the derived
docs already current, and exits 0 on that path — it never reaches the push at
all. The one `push` failure, on `0aa9049`, carried the GH013 annotation and dates
to **2026-08-01**, six days *before* the bypass was added. So the original
diagnosis was correct when filed and the remedy has since been applied.

**The bypass is therefore untested, not missing.** It gets exercised the first
time `main` receives a merge that leaves a derived doc stale — which is the
`dev` → `main` merge at the tag, not something to manufacture beforehand.

> **A caution this file has already earned.** An earlier revision reported
> "seven `success` runs, and the single `push` event failed." The real figures
> are 15 push runs and 13 successes. The conclusion — that the green column
> never described `regen-on-main` — survived, but the evidence for it did not,
> and an auditor who finds thirteen greens discounts the section that is right.
> Every count in this file names the command that produced it for exactly this
> reason.

### Awaiting protocol rulings

| issue | question | blocks |
|---|---|---|
| protocol#77 | who owns the §1.2 per-tool redirect files | skdd#6 scope |
| protocol#89 | is `!pattern` valid in `file_structure.ignore_patterns` | the fix shape for #269 |
| protocol#90 | §7.3's example declares 3 areas for logmind; we implement 5 | what the tag bakes in |
| protocol#93 | §3.4 requires non-empty reasoning; §3.1 says an entry without it is well-formed | nothing — gate follows §3.4 today |

### #241 — pre-tag by Ruling 12, but unbuildable as written

Underspecified, and blocked on agent-skills#173 and #174, both unbuilt.

---

## The order

Gate integrity came first because **#257 is a distribution mechanism**: anything
wrong in the binary, hooks or templates when it ships is pushed to every consumer
repo and must then be fixed twice.

### 1 — Gate integrity ✅ on `dev`

#278 · #260 · #284 · #286 · #285. All landed.

### 2 — Prerequisites for the fleet migration

Each is a hard gate on #257, not a parallel cleanup.

| | why it blocks |
|---|---|
| **#288** | the current template turns a refused push into `::error` + `exit 1`. Migrating first makes every repo go red on every merge to `main` touching a derived doc — permanently, and the job says it will not self-heal. Strictly worse than the storm it replaces. |
| **a release carrying the verb** | the new gate template calls `logmind check-decisions --base/--head`, and `setup-logmind` installs the latest **release**. v1.2.0 does not have the verb. |
| **template v12** | the shipped `regen-timeline` template pushes with a raw `LOGMIND_AUTO_REGEN_PAT`; logmind's own copy mints a `skdd-steward[bot]` token via `create-github-app-token@v3`. Shipping as-is deploys the credential path logmind abandoned. Also fix the action-pin regression — the template pins `setup-logmind@v1.0.0` while five repos run `@v1.0.1`. |

### 3 — Remaining pre-tag work

**#261** — gate logic inline on `pull_request` (§6.3). *Reordered:* originally
ahead of the gate cluster; doing it after means relocating a **correct**
evaluation rather than a wrong one. Its remedy restructures `regen-timeline.yml`,
so that file moves twice regardless of what else we do.

**#267** — `EnsureAgentsMD` silently downgrades a newer `AGENTS.md` block. Same
*shape* as #286 but a different root cause: `matchingTemplate` recognises markers
by hardcoded `strings.Contains` of known generations, so an unknown newer marker
returns `""` and the slim default is applied. No version compare exists to fix.
Fires exactly during a staggered rollout — the condition #257 creates.

**#270** — hooks resolve their engine by bare name, so PATH skew silently
disables the local gate. §3.4 requires fail-open *and* requires that failing open
not be silent; today it is.

**#269** — `ignore_patterns` replaces the 16 built-in defaults instead of merging.
§1.4 line 202 says the three sources are **merged**, so this is a spec violation,
not a wart. Fix shape depends on protocol#89.

**#259** — the derived-doc restore does not refuse while a merge, rebase,
cherry-pick or revert is in progress. Restoring mid-conflict discards the
resolution someone is part-way through, and the restore paths run automatically.

**#266** — `logmind config set` has no refusal for gate settings. §1.6's point is
that some keys are humans-only; an agent that can lower `commit_line_threshold`
to unblock itself has bought exactly what §3.4's gate exists to prevent.

**#244** — branch→issue binding plus comment-back on merge. Pre-tag by **Ruling
12**, not by defect: nothing in current behaviour is wrong, the feature simply
does not exist. Recorded here rather than in §5 so the ruling stays visible —
its own issue body still says "after the v2.0.0 tag", which contradicts the
ruling that governs it and needs correcting.

**#241** — `logmind auto`. Also pre-tag by Ruling 12, and the longest pole among
them: it installs two skills that did not exist, so those had to be written
first (agent-skills#207, open). Build once that merges.

**#279** — the site's `--version` example is one line; §7.3 requires two, and the
tag-time flip will not add the `areas:` line.

### 4 — #265 + #257, together

One fleet move, per standing constraint. #265 collapses the decision layout —
main is a branch like any other, no cap, no archive, `rotateDecisions` deleted
rather than ported — and adds `docs/timeline-archive.md` as a **third** derived
file that every restore path and `check-derived-docs` must learn. All of them
name two today.

**#277 rides here too.** It reports `check-derived-docs` enforcing the opposite
of §3.3 — regenerating from the branch's own sources and failing on the diff, so
it demands the branch commit exactly what §3.3 forbids. logmind's own copy and
its shipped `v11` template are **already correct**; the defect is a stale
installed copy in the consumer repos. So it is a #257 payload, not code to write
here, and it closes when the fleet takes the current template.

### 5 — After the tag

#263 (§6.7 pre-push gate, wholly new scope) · #251 · #271 · #280 · #210 · #217 ·
#258 · #268 · #273 · #274 · #283.

---

## The fleet, measured

Read from each repository's **installed** file, not inferred from what this repo
ships.

| repo | `check-decisions` | `regen-timeline` | pushes to PR branch |
|---|---|---|---|
| protocol | `v2` | `v4` | **yes** |
| clud-bug | `v2` | `v4` | **yes** |
| clud-bug-app | `v2` | `v4` | **yes** |
| agent-skills | `v2` | `v4` | **yes** |
| reporulez | *unversioned* | *unmarked* | no |
| skdd | *absent* on `main`, **`v4`** on `dev` | — | no |

**logmind ships** `check-decisions` at **`v5`** and `regen-timeline` at `v11`. Template versions are **per file** — "the fleet is on
v4" is not one number.

Two corrections to #257's original inventory:

- **reporulez is structurally unreachable by refresh.** `installWorkflowTemplates`
  guards on `installedVer != ""`, so a file with no `# logmind-template-version`
  marker is skipped **forever**. It needs hand-replacement or it is silently left
  behind.
- **skdd was misread, and the misreading mixed two branches.** Its `main` has no
  `.github/workflows` at all (404); its `dev` carries `check-decisions` at **v4**,
  added 2026-08-01. An earlier revision of this file said "already on v11, so
  migrating it is a no-op" — that was wrong in both halves, and it would have
  removed a repository from the migration list that genuinely needs migrating.
  Its clean `check-derived-docs` history is still no validation: green because
  neither derived doc exists on any skdd branch.

### The cost of not migrating

`thrillmade/protocol`, last 10 pull requests: **98 commits, 42 of them
`logmind-auto-regen[bot]` — 42%.** PR#75 alone carries 30, every subject
byte-identical. 1:1 alternation confirmed by timestamp. PR#70 contains a *human*
commit titled `[skip-logmind] chore: regen derived docs` — someone hand-running
the regeneration to pre-empt the bot.

It has already produced one governance failure: **protocol#75, the SPEC 2.0
rewrite itself, merged over a red required `check-derived-docs` via admin
bypass.** A repository where every push needs a rebase is one where people start
bypassing gates.

> **Method note, load-bearing.** Those bot commits have `.author.login == null` —
> no linked account. A probe keyed on `.author.login` reports **zero on every
> pull request** and looks like a clean result. Identity must be read from
> `.commit.author.name`.

---

## Pre-tag checklist

1. `dev` → `main`, which closes #264, #278, #260, #284, #285, #286.
2. Run `release.yml` with `dry_run=true`.
3. **Separately** re-confirm the homebrew-tap ruleset bypass — the dry run skips
   that push, so it never exercises the identity that failed with GH013.
4. Tag. That is the CEO's action, not the lane's.
