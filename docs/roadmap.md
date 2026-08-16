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

**Verified 2026-08-15** against the live SPEC at `thrillmade/protocol` blob
`cd64e5c` (1,475 lines, Sections 0–8, no appendices) — an immutable blob, so
that pin cannot rot.

This line used to name the `dev` and `main` SHAs it was checked against. Those
are gone, and their absence is the point: the pin was stale three minutes before
the commit that introduced it, which is the fourth recurrence of the defect this
file exists to prevent — in the file's own header, two paragraphs above the rule
forbidding it. A branch tip is not a fact this file may own. Run
`git rev-parse --short origin/dev origin/main` instead.

---

## State

**This file does not carry SHAs, counts, or "N ahead".** Two revisions tried and
both were stale within hours — once *during* the review that checked them. Git
already owns those facts, and a hand-kept second copy reads as true until one
quietly isn't. That is the same one-owner-per-fact rule this file was created to
enforce, applied to itself.

Run these instead:

```sh
git fetch origin --prune
git log --oneline origin/main..origin/dev          # what is on dev, not yet on main
gh issue list --state open --limit 200             # open issues
gh pr list --state open --limit 200                # open pull requests
# --limit is load-bearing: gh defaults to 30 and truncates SILENTLY, with no
# notice. 200 is not a fact about this repo, it is headroom — cross-check the
# total, which cannot truncate:
gh api 'search/issues?q=repo:thrillmade/logmind+type:issue+state:open' --jq .total_count
gh release list --limit 1                          # what `brew install` gives
```

The one durable fact worth stating: **everything built since June is unreleased.**
The released binary predates the commit gate, the harness guard, the zero-conflict
invariant and the `areas:` declaration — so `brew install` does not get any of
them until the tag.

### How work is verified

`dev` deliberately has **no CI** — of the two workflows, only
`check-derived-docs` (`regen-timeline.yml`) carries a `push: branches: [main]`
trigger. `check-decisions` has **no push trigger at all**; it runs only on
`pull_request`. The bar is a **local adversarial panel** per change, plus the
full suite run against integrated `dev`. Pull requests into `dev` still get
CI on their own merge-ref (both workflows' `pull_request` trigger carries no
`branches:` filter); what the panel and the local suite cover is the
*integrated* result, which PR-level CI never sees.

---

## Landed on `dev`, not yet on `main`

```sh
git log --oneline origin/main..origin/dev
```

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

The push has still never landed a commit, and the reason is *not* a refusal.
**Measured 2026-08-15** against `logmind / check-derived-docs`
(`.github/workflows/regen-timeline.yml`, workflow id `277546816`):

```sh
$ git log origin/main --format='%s' | grep -c '^chore: regen derived docs$'
0
$ gh api repos/thrillmade/protocol/pulls/75/commits --paginate \
    --jq '[.[] | select(.commit.author.name == "logmind-auto-regen[bot]")] | length'
30
# --paginate is load-bearing: PR#75 has 63 commits over 3 pages and `gh api`
# returns only the first without it, which answers 15.
$ gh api "repos/thrillmade/logmind/actions/workflows/277546816/runs?per_page=100&event=push" \
    --paginate --jq '.workflow_runs[] | .conclusion' | sort | uniq -c
     13 success
      2 failure
```

| probe | result |
|---|---|
| regen commits on `main`, by exact subject line | **0** |
| control — same probe, protocol PR#75 bot commits | **30** |
| `push`-event runs of `regen-timeline` | **15** |
| `push` runs that **succeeded** | **13** |
| `push` runs that **failed** | **2** |

Thirteen green push runs and zero commits means the job runs, finds the derived
docs already current, and exits 0 on that path — it never reaches the push at
all. **Both** `push` failures carry the GH013 `Changes must be made through a
pull request` refusal: `fcf7268` (**2026-07-25**) and `0aa9049`
(**2026-08-01**), each dated *before* the bypass was added
(**2026-08-07**). So the original diagnosis was correct when filed and the
remedy has since been applied.

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
| protocol#89 | is `!pattern` valid in `file_structure.ignore_patterns` | **nothing — answered by shipping.** #269 conformed to §1.4 as written, so no ruling was needed. Left open for the SPEC-wording question it raises. |
| protocol#90 | §7.3's example declares 3 areas for logmind; we implement 5 | what the tag bakes in |
| protocol#93 | §3.4 requires non-empty reasoning; §3.1 says an entry without it is well-formed | nothing — gate follows §3.4 today |

### #241 — pre-tag by Ruling 12, but unbuildable as written

Underspecified — the setup surface itself has open questions (profile
vocabulary, whether the limit-watch is tooled or purely behavioural). It is
**no longer blocked on missing skills**: agent-skills#207 (merged
2026-08-15) shipped both `session-heartbeat` and `unattended-operation`,
implementing agent-skills#173 and #174. Those two tracking issues remain
open on GitHub — that is bookkeeping, not a build blocker; the skills exist
in the catalog today.

---

## The order

Gate integrity came first because **#257 is a distribution mechanism**: anything
wrong in the binary, hooks or templates when it ships is pushed to every consumer
repo and must then be fixed twice.

### 1 — Gate integrity ✅ on `dev`

#278 · #260 · #284 · #286 · #285 · #267 · #270. All landed.

### 2 — Prerequisites for the fleet migration

Each is a hard gate on #257, not a parallel cleanup. **#257 itself is
post-tag, not pre-tag** — `setup-logmind` installs from `/releases/latest`,
and the released v1.2.0 answers `logmind check-decisions --base` with
`Error: unknown flag: --base` (ruling recorded in #243). The fleet cannot
take the new gate template until v2.0.0 exists, so this whole cluster —
including #288 below and #265+#257 in §4 — runs after the tag by
construction, even though it is numbered ahead of "§3 — Remaining pre-tag
work" in this list: numbering here is dependency order, not calendar order
across the tag boundary.

| | why it blocks |
|---|---|
| **#288** | the current template turns a refused push into `::error` + `exit 1`. Migrating first makes every repo go red on every merge to `main` touching a derived doc — permanently, and the job says it will not self-heal. Strictly worse than the storm it replaces. |
| **a release carrying the verb** | the new gate template calls `logmind check-decisions --base/--head`, and `setup-logmind` installs the latest **release**. v1.2.0 does not have the verb. |
| **template v12** | **Built and on `dev`** (#314). The shipped template pushed with a raw `LOGMIND_AUTO_REGEN_PAT` while logmind's own copy minted a steward token; it now degrades App → PAT → `GITHUB_TOKEN`, resolves the default branch instead of hardcoding it in four places, and pins every action ref. The pin was `@v1.0.0` against seven consumers on `@v1.0.1` — not five. (measured 2026-08-16: `gh search code "thrillmade/setup-logmind@v1.0.1" --owner thrillmade --limit 100` returns **8** repos — seven consumers plus logmind's own `logmind-self-update.yml`. Control, **same unit**: the `@v1.0.0` query returns **2** repos (25 vs 13 raw rows — an earlier revision quoted the row count against the repo count, which is why this now states the unit).) |

### 3 — Remaining pre-tag work

**#261** — gate logic inline on `pull_request` (§6.3). *Reordered:* originally
ahead of the gate cluster; doing it after means relocating a **correct**
evaluation rather than a wrong one. Its remedy restructures `regen-timeline.yml`,
so that file moves twice regardless of what else we do.

**#269** — `ignore_patterns` replaced the 16 built-in defaults instead of merging.
**Built and on `dev`** (#303). protocol#89 turned out not to gate it: the answer
was to give the defaults their own source and resolve positionally, which
*conforms* to §1.4 as written rather than needing a ruling. Verified against
`git check-ignore` as an oracle across seven fixtures.

**#259** — the derived-doc restore does not refuse while a merge, rebase,
cherry-pick or revert is in progress. Restoring mid-conflict discards the
resolution someone is part-way through, and the restore paths run automatically.

**#266** — `logmind config set` has no refusal for gate settings. §1.6's point is
that some keys are humans-only; an agent that can lower `commit_line_threshold`
to unblock itself has bought exactly what §3.4's gate exists to prevent.

**#244** — branch→issue binding plus comment-back on merge. Pre-tag by **Ruling
12**, not by defect: nothing in current behaviour is wrong, the feature simply
does not exist. Recorded here rather than in §5 so the ruling stays visible.
Its body no longer contradicts the ruling: it was corrected 2026-08-01 and now
opens "Pre-tag — in v2.0.0 scope by Ruling 12." Nothing to do.

**#241** — `logmind auto`. Also pre-tag by Ruling 12. It previously needed two
skills that did not exist; those shipped in agent-skills#207 (merged
2026-08-15). What remains is #241's own underspecification — see above.

**#279** — the site's `--version` example is one line; §7.3 requires two, and the
tag-time flip would not have added the `areas:` line. **Built and on `dev`**
(#302). The issue stays open until `dev` reaches `main`, per the rule above.
The panel verified it by building the site in both states rather than reading
the JSX, and by byte-comparing the page's `AREAS` against `version.go`.

**#297, #298, #299, #271, #310** — filed after this section was written, all
**built and on `dev`** (#306, #307, #308, #313). Two were live data-loss paths:
`self-update` wrote a marker-block fragment over the whole of `AGENTS.md`, and
`doctor --fix` overwrote a file it had misjudged as markerless. #310 is the
class behind both, and is **still open** — its last two raw writes live in
`internal/gitattr/gitattr.go` and close only when #301 lands. #313 covered the
rest of the tree and added the guard that fails on the next one — `os.WriteFile` follows symlinks, so a planted link
redirected writes outside the repository from 26 call sites.

**What is genuinely left is shorter than this list.** Rather than maintaining a
second copy of it here, ask git:

```sh
git log --oneline origin/main..origin/dev     # built, awaiting the merge to main
gh issue list --state open --limit 200         # everything still open
# --limit is load-bearing: gh defaults to 30 and truncates SILENTLY.
# Cross-check the total, which cannot truncate:
gh api 'search/issues?q=repo:thrillmade/logmind+type:issue+state:open' --jq .total_count
```

An issue listed above may already be built — `closes #N` only fires on the
default branch, so anything merged to `dev` stays open until `dev` reaches
`main`. That is expected, not drift.

### 4 — #265 + #257, together (post-tag — see §2)

One fleet move, per standing constraint. #265 collapses the decision layout —
main is a branch like any other, no cap, no archive, `rotateDecisions` deleted
rather than ported — and adds `docs/timeline-archive.md` as a **third** derived
file that every restore path and `check-derived-docs` must learn. All of them
name two today.

**#277 rides here too.** It reports `check-derived-docs` enforcing the opposite
of §3.3 — regenerating from the branch's own sources and failing on the diff, so
it demands the branch commit exactly what §3.3 forbids. logmind's own copy and
its shipped `v12` template are **already correct**; the defect is a stale
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
| rezgen | `v2` | `v4` | **yes** |
| tokenomics | `v2` | `v4` | **yes** |
| reporulez | *unversioned* | *unmarked* | no |
| skdd | *absent* on `main`, **`v4`** on `dev` | *absent* on `main`, **`v11`** on `dev` | no |

`rezgen` and `tokenomics` were missing from an earlier revision of this table
while the search two sections above named them — §4 sizes "one fleet move" off
this table, so an undercount here understates the migration. Regenerate it,
rather than trusting it:

```sh
for r in $(gh repo list thrillmade --limit 200 --json name --jq '.[].name'); do
  for w in check-decisions regen-timeline; do
    printf '%s %s %s\n' "$r" "$w" \
      "$(gh api "repos/thrillmade/$r/contents/.github/workflows/$w.yml" --jq .content \
         2>/dev/null | base64 -d 2>/dev/null | head -1 | grep -oE 'v[0-9]+' || echo absent)"
  done
done
```

**logmind ships** `check-decisions` at **`v6`** and `regen-timeline` at **`v12`** (`v13` once #301 lands), `check-doc-links` at `v9`, `logmind-self-update` at `v11` — measured 2026-08-16 with `head -1 internal/templates/github/*.template`. Template versions are **per file** — "the fleet is on
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

`thrillmade/protocol`, last 10 **merged** pull requests by creation date
(#92, #88, #85, #84, #83, #80, #76, #75, #70, #69) — **measured 2026-08-15**:

```sh
$ gh api "repos/thrillmade/protocol/pulls?state=closed&sort=created&direction=desc&per_page=30" \
    --jq '[.[] | select(.merged_at != null)] | .[0:10] | .[].number'
$ for pr in 92 88 85 84 83 80 76 75 70 69; do
    gh api "repos/thrillmade/protocol/pulls/$pr/commits" --paginate \
      --jq '[.[] | .commit.author.name]'
  done
# 109 commits total, 43 with .commit.author.name == "logmind-auto-regen[bot]"
```

**109 commits, 43 of them `logmind-auto-regen[bot]` — 39%.** PR#75 alone
carries 30, every subject byte-identical (`chore: regen derived docs`); its 63
commits alternate human/bot 1:1 for the first 60, confirmed by commit order.
PR#70 contains a *human* commit titled `[skip-logmind] chore: regen derived docs` —
someone hand-running the regeneration to pre-empt the bot.

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

1. `dev` → `main`, which closes #264, #278, #260, #284, #285, #286, #267, #270.
2. Run `release.yml` with `dry_run=true`.
3. **Separately** re-confirm the homebrew-tap ruleset bypass — the dry run skips
   that push, so it never exercises the identity that failed with GH013.
4. Tag. That is the CEO's action, not the lane's.
