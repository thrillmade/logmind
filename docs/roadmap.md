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
`git rev-parse --short origin/dev` and `git rev-parse --short origin/main` —
one revision per call, because `--short` implies `--verify`, and the
two-argument form exits `fatal: Needed a single revision`.

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

**Nothing in CI watches `dev`.** A commit landing on `dev` runs no workflow at
all: every branch-push trigger in this repository names `main`, and the only
other push triggers fire on tags.

Pull requests *into* `dev` do get checks. The Go suite (`test.yml` — matrix
build, `make test`, gofmt and vet), `check-decisions`, `check-derived-docs`
(`regen-timeline.yml`) and `check-doc-links` all trigger on `pull_request`
with no `branches:` filter, so they run on the merge-ref whatever the base
branch is. Of the `pull_request` triggers, only `goreleaser-check`'s is
filtered to `main`.

The bar for `dev` itself is therefore a **local adversarial panel** per change,
plus the full suite run against integrated `dev` — the *integrated* result is
exactly what PR-level CI never sees.

Read the triggers rather than trusting this paragraph; workflows get added:

```sh
for f in .github/workflows/*.yml; do
  printf '### %s\n' "${f##*/}"
  awk '/^on:/{p=1;next} /^[^[:space:]#]/{p=0} p&&!/^[[:space:]]*#/&&NF' "$f"
done
```

Some of those files are also the workflow templates logmind ships to consumer
repositories. [The fleet, measured](#the-fleet-measured) owns which ones, and
at what version.

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
["OrganizationAdmin:null","Integration:3951953"]

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
   2 failure
  13 success
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

---

## The order

Gate integrity came first because **#257 is a distribution mechanism**: anything
wrong in the binary, hooks or templates when it ships is pushed to every consumer
repo and must then be fixed twice.

### 1 — Gate integrity ✅ on `dev`

#278 · #260 · #284 · #286 · #285 · #267 · #270. All landed.

### 2 — Prerequisites for the fleet migration

Each is a hard gate on #257, not a parallel cleanup.

**This paragraph is the only place the tag boundary is decided, and it decides
one thing: the fleet move, #257, is the only work in the run to the tag that
waits for the tag.** #277 closes with it rather than on a schedule of its own —
it is a payload of the migration, not a second waiter. Everything in §3 lands
before the tag; §5 is the backlog that follows it.

The reason is mechanical: `setup-logmind` installs from `/releases/latest`, and
the released v1.2.0 answers `logmind check-decisions --base` with
`Error: unknown flag: --base` (ruling recorded in #243). No consumer repo can
take the new gate template until v2.0.0 exists, so the fleet move cannot start
until the tag does.

Numbering in this list is dependency order, not calendar order. Two of the
three rows below are already answered — template v12 is on `dev`, and the
bypass #288 reported missing has been granted (see #288 above). Only "a release
carrying the verb" is still waiting, and what it waits for is the tag.

| | why it blocks |
|---|---|
| **#288** | the current template turns a refused push into `::error` + `exit 1`. Migrating first makes every repo go red on every merge to `main` touching a derived doc — permanently, and the job says it will not self-heal. Strictly worse than the storm it replaces. |
| **a release carrying the verb** | the new gate template calls `logmind check-decisions --base/--head`, and `setup-logmind` installs the latest **release**. v1.2.0 does not have the verb. |
| **template v12** | **Built and on `dev`** (#314). The shipped template pushed with a raw `LOGMIND_AUTO_REGEN_PAT` while logmind's own copy minted a steward token; it now degrades App → PAT → `GITHUB_TOKEN`, resolves the default branch instead of hardcoding it in four places, and pins every action ref. The pin was `@v1.0.0` against seven consumers on `@v1.0.1` — not five. (measured 2026-08-16: `gh search code "thrillmade/setup-logmind@v1.0.1" --owner thrillmade --limit 100` returns **8** repos — seven consumers plus logmind's own `logmind-self-update.yml`. Control, **same unit**: the `@v1.0.0` query returns **2** repos. Count repos, never rows — two measurements the same day gave 25 and 23 rows for one query, so a row figure is wrong by the time it is read. Derive with `gh search code "thrillmade/setup-logmind@v1.0.1" --owner thrillmade --limit 100 --json repository --jq '[.[].repository.nameWithOwner] \| unique \| length'`.) |

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
Its body no longer contradicts the ruling: its `## Sequencing` section was
corrected 2026-08-01 and now reads "Pre-tag — in v2.0.0 scope by Ruling 12."
Nothing to do.

**#241** — `logmind auto <profile>`: one command sets a repository up to be
handed over and run unattended. Pre-tag by Ruling 12. **Built and on `dev`**
(#300). It writes the standing directive `.logmind/auto.yml`, reports which
required skills are present, and prints the handover a human must give — it
never starts the mode, because the mode may only begin from an explicit human
handover. Both of the issue's open questions were answered by shipping. The
profile vocabulary is one name, `unattended`. `night` is refused *with* its
reason — the mode starts from a human handover, never from the clock. `skdd`,
the word the issue itself used, gets only the generic
`unknown profile "skdd". Known profiles: unattended`, because nothing yet
defines what an skdd profile would declare; an operator typing the original
ask is told the name is unknown and nothing more. The limit-watch stays
behavioural — the threshold rule lives in the written directive and is owned by
the `session-heartbeat` skill, not tooled here. The command is done.

**#265** — collapse the decision layout: `main` is a branch like any other, no
cap, no archive, `rotateDecisions` deleted rather than ported. It adds
`docs/timeline-archive.md` as a **third** derived file that every restore path
and `check-derived-docs` must learn; all of them name two today. **In flight** —
PR #301 carries it, open against `dev`.

**#279** — the site's `--version` example is one line; §7.3 requires two, and the
tag-time flip would not have added the `areas:` line. **Built and on `dev`**
(#302). The issue stays open until `dev` reaches `main`, per the rule above.
The panel verified it by building the site in both states rather than reading
the JSX, and by byte-comparing the page's `AREAS` against `version.go`.

**#297, #298, #299, #271** — **built and on `dev`**, and not one pull request
each. Ask git which carried which rather than reading a pairing from here:
`git log origin/main..origin/dev --oneline --grep='#297' -i`.

**#310** is the class behind those four, and the only one of the five not done.
The rule: **no code may create or truncate a repository file with a raw stdlib
call.** Those calls follow symlinks, so a planted link redirects the write
outside the repository — and a dangling link makes the preceding "does it
exist?" check answer *no*, which is what turns the hole into an ordinary-looking
create. Every site routes through `internal/atomicio` instead, and
`internal/writeaudit` parses the tree on each test run so the next raw call
fails the build rather than shipping. Its allowlist holds the exceptions, each
with the reason it is safe — all but one. The two sites in
`internal/gitattr/gitattr.go` are still vulnerable and close when #301 lands.

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

### 4 — #257, the fleet migration (post-tag — see §2)

One fleet move: every consumer repository takes the current templates at once,
after the tag exists to install from.

**#277 rides here too.** It reports `check-derived-docs` enforcing the opposite
of §3.3 — regenerating from the branch's own sources and failing on the diff, so
it demands the branch commit exactly what §3.3 forbids. logmind's own copy and
its shipped `v12` template are **already correct**; the defect is a stale
installed copy in the consumer repos. So it is a #257 payload, not code to
write here.

### 5 — After the tag

#263 (§6.7 pre-push gate, wholly new scope) · #251 · #280 · #210 · #217 ·
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

§4 sizes "one fleet move" off this table, so an undercount here understates the
migration. Regenerate it rather than trusting it:

```sh
for r in $(gh repo list thrillmade --limit 200 --json name --jq '.[].name'); do
  for w in check-decisions regen-timeline; do
    printf '%s %s %s\n' "$r" "$w" \
      "$(raw=$(gh api "repos/thrillmade/$r/contents/.github/workflows/$w.yml" \
             --jq .content 2>/dev/null | base64 -d 2>/dev/null)
         if [ -z "$raw" ]; then echo 'file absent'
         else echo "$raw" | head -1 | grep -oE 'v[0-9]+' || echo 'present, unmarked'
         fi)"
  done
done
```

Three states, not two — `|| echo absent` collapses "no file" and "a file with no
marker" into one word, and the table distinguishes them (`reporulez` has both
files present and unmarked; `skdd`'s are absent from its default branch and
carry markers on `dev`). The loop reads **default branches only**, so `skdd`'s
row was measured separately. `logmind` is the producer rather than a consumer
and is deliberately not a row — but the loop still prints one, reading
`present, unmarked`. That is not the reporulez defect. logmind hand-maintains
its own copies of `check-decisions.yml`, `regen-timeline.yml` and
`check-doc-links.yml` — markerless on purpose, so refresh leaves them alone.
The exception is `logmind-self-update.yml`: it carries the marker, is refreshed
like any consumer's, and is byte-identical to the template listed above.

**logmind ships** `check-decisions` at **`v6`** and `regen-timeline` at **`v12`** (`v13` once #301 lands), `check-doc-links` at `v9` (`v10` once #301 lands), `logmind-self-update` at `v11` — measured 2026-08-16 with `head -1 internal/templates/github/*.template`. Template versions are **per file** — "the fleet is on
v4" is not one number.

Two corrections to #257's original inventory:

- **reporulez is unreachable by refresh, and always will be.** A workflow file
  is logmind's to overwrite only when the `# logmind-template-version` marker is
  on line 1 — that is what `TemplateMarker.Writable()` means, and
  `installWorkflowTemplatesMode` refuses everything else, a displaced marker
  included. reporulez's two files carry no marker at all, so no refresh will
  ever touch them. The refusal is reported on stderr rather than skipped
  quietly, but a reported refusal still leaves the stale file in place. It needs
  hand-replacement.
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

1. `dev` → `main`. That one merge fires every `closes #N` on the branch at
   once — a good deal more than the gate-integrity cluster. Read the list from
   git immediately before merging, never from here; the command is below.
2. Run `release.yml` with `dry_run=true`.
3. **Separately** re-confirm the homebrew-tap ruleset bypass — the dry run skips
   that push, so it never exercises the identity that failed with GH013.
4. Tag. That is the CEO's action, not the lane's.

What step 1 closes:

```sh
git log origin/main..origin/dev --format='%B' \
  | grep -oiE '(close[sd]?|fix(e[sd])?|resolve[sd]?)[[:space:]]+#[0-9]+([[:space:]]*,[[:space:]]*#[0-9]+)*' \
  | grep -oE '#[0-9]+' | sort -u -V
```

The comma branch is load-bearing, not decoration: one commit on this branch
spells it `closes #278, #260, #284`, and a probe that stops at the first `#N`
loses `#260`. `#284` survives that probe only because a different commit
happens to spell `Closes #284.` on its own — luck, not coverage. Confirm after
the merge that each issue really closed, and hand-close any that did not.
