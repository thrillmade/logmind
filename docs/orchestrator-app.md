# `skdd-steward[bot]` GitHub App

**Renamed from `thrillmade-orchestrator[bot]`.** This file documents
**thrillmade's own installed instance** of the App (App ID 3951953) — the
instance wired into `thrillmade/logmind`'s release pipeline and, since the
weekly skill census shipped, into `thrillmade/agent-skills` too. It is not
the registration guide for other organizations — see "Registering your own
instance" under Purpose below.

Reference manifest and click-by-click registration guide for the
`skdd-steward[bot]` GitHub App. Committed to logmind for reproducibility —
if the App is ever rotated, deleted, or migrated to a different org, the
steps below produce an identical replacement.

Mirrors the schema of `/Users/ludlow/clud-bug-app/github-app-manifest.json`
(thrillmade's first App). Permissions and event subscriptions differ — the
steward has a narrower surface than `clud-bug[bot]`.

## Purpose

`skdd-steward[bot]` is the App identity behind all cross-repo writes that
originate from a thrillmade-owned workflow.

**Duties today:**

- **Release cask bumps.** Since v1.0, bumps the Homebrew cask formula on
  `thrillmade/homebrew-tap` after every `logmind` release (replacing the
  personal `HOMEBREW_TAP_PAT` that was a key-person single point of
  failure).
- **Weekly skill census.** Powers `thrillmade/agent-skills`'
  `.github/workflows/skill-census.yml`, which mints a per-run installation
  token to read skill manifests across every repo in the org and file
  census verdicts. See agent-skills'
  [`docs/integrating-with-agent-skills.md`](https://github.com/thrillmade/agent-skills/blob/main/docs/integrating-with-agent-skills.md)
  for the census's design and cadence — that doc is the operator's manual
  for this duty, this file is the App spec.

**Roadmap:**

- **Catalog fan-out** — skill-catalog PRs to `thrillmade/agent-skills` via
  `logmind skill push` (G7.n in the master plan).
- **Chore loop** — org-wide maintenance PRs across consumer repos,
  replacing today's per-repo `*-self-update.yml` crons with one watcher
  under this identity.
- **Steward service** — a standing service (not just workflow-minted
  tokens) running under this App identity. Contract details are drafted in
  [`thrillmade/protocol#39`](https://github.com/thrillmade/protocol/issues/39)
  and tracked in SPEC §0.3's Planned list, "Unified install and the
  steward — [skdd#6](https://github.com/thrillmade/skdd/issues/6)".

Per the master plan at
`/Users/ludlow/.claude/plans/ok-here-is-recent-distributed-chipmunk.md`,
section "Architectural decisions in force — GitHub Apps are the primitive
for release infrastructure", we register an App rather than mint a PAT or
spin up a machine-user account: PATs require periodic rotation and tie the
release pipeline to one human's GitHub account, and machine-user accounts
add real-account-maintenance overhead with no narrower audit story than an
App. Tokens are 1-hour installation tokens minted at workflow time by the
official `actions/create-github-app-token@v3` action — the only App-token
action compatible with the repo's `allowed_actions: selected` workflow
allowlist.

### Registering your own instance

This file documents **thrillmade's own installed instance** only. Other
organizations adopting the SkDD toolchain register a separate instance of
the same App design under their own account — that consumer-facing,
org-agnostic registration guide lives in `thrillmade/skdd`'s
`docs/skdd-steward-app.md` (introduced in `skdd#2`). Use that guide for a
fresh registration in a new org; use this file only to reproduce or reason
about thrillmade's specific instance (App ID 3951953, secrets named
`THRILLMADE_ORCHESTRATOR_*` — see "Secrets storage" below for why those
legacy names stick around here specifically).

## App identity

- **Name:** `skdd-steward`
- **Slug:** `skdd-steward` (bot identity: `skdd-steward[bot]`)
- **Homepage URL:** `https://github.com/thrillmade` (org page — the App is internal infra, not a logmind feature)
- **Owner:** `thrillmade` organization
- **Public:** no (private to the thrillmade org)

Formerly registered as `thrillmade-orchestrator` (slug
`thrillmade-orchestrator`, bot identity `thrillmade-orchestrator[bot]`).
The rename changed name, slug, and bot identity only — App ID (3951953),
private key, and installation are all unchanged; no re-registration
occurred. Anything recorded before the rename (commits, audit-log entries,
decision-log history in this repo) still shows the old
`thrillmade-orchestrator[bot]` identity — see "Audit log expectations"
below.

## Permissions

Repository-level only. No user permissions. No organization permissions.

| Permission | Access | Why |
|---|---|---|
| Contents | Read and write | Direct push of `Casks/logmind.rb` bumps to `thrillmade/homebrew-tap/main`; future skill-catalog file writes. |
| Pull requests | Read and write | v1.x expansion (G7.n) opens skill-catalog PRs on `thrillmade/agent-skills`; reserved now to avoid a re-registration when that lands. |
| Issues | Read and write | Census verdict filing (`gap:` / `placement:` / `promotion-candidate:` / `demotion-candidate:` / `revise:` issues, plus the weekly digest) opened by the skill census under the steward identity. **Pending installation approval at the time of writing** — requested on the App's permissions page but not yet accepted by an org owner. |
| Metadata | Read | Mandatory baseline for every App; grants read on repo name, default branch, topics. |

## Events

None.

The steward is workflow-driven — it never subscribes to webhook events.
Tokens are minted on demand inside `release.yml` (and, for the census,
inside `thrillmade/agent-skills`' `skill-census.yml`); there is no Vercel
function and no event listener.

## Installation scope

`repository_selection: all` — the App is installed across every repository
in the `thrillmade` org.

This is a **CDO-blessed** decision, not the original design. Earlier
revisions of this file documented "Selected repositories,
`thrillmade/homebrew-tap` only," with expansion planned repo-by-repo as
each new duty shipped. That plan was superseded once the weekly skill
census needed org-wide manifest reads on day one — a `selected`-repos
install would have silently blinded the census to any repo left off the
list, and the planned catalog fan-out and chore loop need the same
coverage.

Why the wider blast radius is acceptable:

- **Branch rulesets do NOT force PRs on every repo — 12 of 20 accept a
  direct push from the App.** Bypass is evaluated per ruleset, and rulesets
  *aggregate*: a push must satisfy every ruleset matching the ref. The
  steward is a bypass actor on both **organization-level** rulesets
  (`18502737 org-baseline`, `16898453 org-default-protection`), which apply
  everywhere. A repo is protected from the App only if it *additionally*
  carries its own ruleset requiring pull requests without naming the steward.
  Seven do: `.github`, `agent-skills`, `clud-bug`, `clud-bug-app`,
  `homebrew-logmind`, `setup-logmind`, `tremendous-machine`. An eighth, `arlyn-working`, is protected by **classic branch protection**
  instead of a ruleset (`enforce_admins: true`), and the steward holds no
  Administration permission to override it. The other twelve — including
  `logmind`, `protocol`, `skdd`, `reporulez` and `homebrew-tap` — accept a
  direct push. See "Ruleset bypass"
  below for the commands.
- **All-repos is what the census reads today, and what the catalog
  fan-out / chore loop will require** — see "Purpose" above.

**Residual risk, stated plainly:** a compromised App private key can mint
an installation token scoped to every repository in the org, for that
token's 1-hour lifetime. **Twelve of the org's twenty repos are exposed to
a direct, unreviewed push under compromise** — not one. The org-level
bypass, not `homebrew-tap`'s own entry, is what grants it; only the eight repos carrying either their own
PR-requiring ruleset or classic branch protection are limited by required review on merge — real exposure (an
attacker could open PRs, comment, file issues, read contents across the
org), but gated by human review rather than by installation scope.

Expansion or contraction of the installed-repo list happens via the App's
Installation settings page on the org — never re-register the App.

## Secrets storage

Two org-level secrets on `thrillmade`, now **org-wide visibility** (every
repo in `thrillmade` can read them). v1.0 shipped with `selected`
visibility limited to `thrillmade/logmind`; visibility widened to
org-wide once `thrillmade/agent-skills` needed the same secrets for the
weekly skill census — adding repos to a `selected` list one at a time
stopped scaling once a second consumer showed up.

- `THRILLMADE_ORCHESTRATOR_APP_ID` — the App's numeric ID (not a secret in
  the cryptographic sense, but stored as a secret to keep the wiring
  uniform across workflows).
- `THRILLMADE_ORCHESTRATOR_PRIVATE_KEY` — the RSA PEM downloaded from the
  App's settings page.

**Names predate the rename, and are kept on purpose.** Both secrets keep
the legacy `THRILLMADE_ORCHESTRATOR_*` labels — renaming an org secret
means touching every consuming workflow (`release.yml` here,
`skill-census.yml` in agent-skills, and any future consumer) in lockstep,
for a purely cosmetic win. New orgs registering their own instance use
`SKDD_STEWARD_*` naming per the public guide (`thrillmade/skdd`
`docs/skdd-steward-app.md`); this repo's secret names are a
thrillmade-specific legacy exception, not the convention to copy.

Exact commands (run after the user provides `APP_ID` and the `.pem` path
via terminal — never paste private-key material into chat):

```sh
APP_ID=12345
gh secret set THRILLMADE_ORCHESTRATOR_APP_ID \
  --org thrillmade \
  --visibility all \
  --body "$APP_ID"

cat ~/Downloads/skdd-steward.YYYY-MM-DD.private-key.pem \
  | gh secret set THRILLMADE_ORCHESTRATOR_PRIVATE_KEY \
      --org thrillmade \
      --visibility all
```

The legacy `HOMEBREW_TAP_PAT` org secret is retired after G1.f passes.

## Ruleset bypass on `thrillmade/homebrew-tap`

Ruleset 17128312 on `main` enforces deletion + non_fast_forward +
required_linear_history + pull_request (0 reviews, squash only). The
steward App is added as a bypass actor so its direct pushes are exempt;
human pushes still go through PR review. The bypass IS the audit trail —
every direct push appears in the org audit log under the App identity.
**`homebrew-tap` IS the only repo whose own ruleset names the steward as a
bypass actor** — that part was right, and an earlier revision of this section
wrongly called it false. What it omitted is that the steward ALSO bypasses via
two *organization-level* rulesets that apply to every repo, so a repo's own
ruleset is not the only thing granting exemption:

```sh
gh api repos/thrillmade/<repo>/rulesets --jq '.[] | "\(.id) \(.name)"'
gh api repos/thrillmade/<repo>/rulesets/<id> \
  --jq '.bypass_actors[]? | "\(.actor_type) \(.actor_id) \(.bypass_mode)"'
gh api /apps/skdd-steward --jq .id     # the id to look for
```

Measured 2026-08-15 across all **20** org repos: `18502737 org-baseline` and
`16898453 org-default-protection` both list the steward with
`bypass_mode: always`, and both apply everywhere. Control: `20570854`,
`18292238`, `16434011` and `17128312` all have a `bypass_actors` key, and the
first three hold zero entries — so the empty result is a real zero, not a
missing field.

**Rulesets aggregate.** A push must satisfy every ruleset matching the ref, so
bypassing the org pair is not sufficient where a repo adds its own. Seven
repos do — `.github`, `agent-skills`, `clud-bug`, `clud-bug-app`,
`homebrew-logmind`, `setup-logmind`, `tremendous-machine` — and the steward is
blocked in those.

**Rulesets are not the only mechanism, and checking only them is how this
section was wrong twice.** `arlyn-working` carries no ruleset but is protected
by classic branch protection with `enforce_admins: true`, which nothing bypasses
without Administration permission — and the steward has none
(`gh api /apps/skdd-steward --jq .permissions` → contents, issues, metadata,
pull_requests). Any audit here MUST check both:

```sh
gh api repos/thrillmade/<repo>/rulesets              # mechanism 1
gh api repos/thrillmade/<repo>/branches/main/protection   # mechanism 2
```

So eight are blocked and the remaining twelve accept a direct push.

`logmind` is in the second group: it carries no repo-level ruleset, so
`regen-on-main`'s push to its default branch is exempt. That is the fact the
release path depends on, and it is measured rather than generalised — an
earlier revision of this section claimed exemption "in any org repo", which is
wrong for the seven above.

Corroboration, and a caution: `homebrew-tap` is the only default branch in the
org carrying direct steward commits. `logmind`, `agent-skills` and `clud-bug`
have zero. So for every repo except the tap this exemption is inferred from
configuration and has never actually been exercised.

The GitHub Rulesets API requires a full ruleset body on `PUT` — it does
not accept a partial patch. Use a read-merge-PUT pattern: fetch the
current ruleset, splice the new actor into `bypass_actors`, and PUT the
merged body back.

```sh
APP_ID=12345
gh api /repos/thrillmade/homebrew-tap/rulesets/17128312 \
  | jq --argjson app_id "$APP_ID" '
      .bypass_actors += [{
        actor_type: "Integration",
        actor_id:   $app_id,
        bypass_mode: "always"
      }]
      | {name, target, enforcement, bypass_actors, conditions, rules}
    ' \
  | gh api -X PUT /repos/thrillmade/homebrew-tap/rulesets/17128312 \
      --input -
```

The `jq` step drops fields the API rejects on PUT (`id`, `source`,
`source_type`, `node_id`, `_links`, `created_at`, `updated_at`, etc.) —
only `name`, `target`, `enforcement`, `bypass_actors`, `conditions`, and
`rules` are accepted in the request body.

## Click-by-click registration

USER ACTION. The agent cannot perform these steps — App creation requires
an authenticated browser session as a thrillmade org owner.

These steps describe registering **thrillmade's own instance** from
scratch (e.g. if the App is ever deleted and must be recreated
identically). If you're standing up a *different* organization's own
instance of this App design, use the public guide instead —
`thrillmade/skdd` `docs/skdd-steward-app.md`.

### Step 1 — Open the new-App page

Navigate to:

```
https://github.com/organizations/thrillmade/settings/apps/new
```

### Step 2 — Fill the form

Paste each value exactly as shown. Fields not listed should be left at
their defaults.

| Field | Value |
|---|---|
| GitHub App name | `skdd-steward` |
| Homepage URL | `https://github.com/thrillmade` |
| Description | `Org-level steward for thrillmade release infrastructure — pushes cask bumps, runs the weekly skill census, and (planned) skill-catalog updates and self-update fan-out across thrillmade repos.` |
| Identifying and authorizing users → Callback URL | LEAVE BLANK |
| Post installation → Setup URL | LEAVE BLANK |
| Post installation → Redirect on update | unchecked |
| Webhook → Active | UNCHECKED |
| Webhook URL | LEAVE BLANK |
| Webhook secret | LEAVE BLANK |

**Repository permissions** (every other permission stays at "No access"):

| Permission | Set to |
|---|---|
| Contents | Read and write |
| Pull requests | Read and write |
| Issues | Read and write |
| Metadata | Read-only (default; cannot be unset) |

**Organization permissions:** all "No access".

**User permissions:** all "No access".

**Subscribe to events:** check NOTHING.

**Where can this GitHub App be installed?** Only on this account.

### Step 3 — Create the App

Click "Create GitHub App" at the bottom of the form. GitHub redirects to
the App's settings page. Record the **App ID** displayed near the top of
that page (also visible in the URL after a brief redirect:
`https://github.com/organizations/thrillmade/settings/apps/skdd-steward`).

### Step 4 — Generate the private key

On the App's settings page, scroll to the "Private keys" section. Click
"Generate a private key". The browser downloads a `.pem` file named
`skdd-steward.YYYY-MM-DD.private-key.pem` to your default download
directory.

Move the file somewhere outside the repo working tree — do not commit it,
do not check it into any logmind clone. The `~/Downloads/` default
location is fine until Step 6.

### Step 5 — Install the App on every repository in the org

In the left sidebar of the App's settings page, click "Install App". Find
the `thrillmade` row and click "Install" next to it. On the install page:

1. Select "All repositories" (`repository_selection: all` — see
   "Installation scope" above for why this is the blessed default now,
   not `thrillmade/homebrew-tap` alone).
2. Click "Install".

GitHub records the installation and returns you to the App settings.

### Step 6 — Hand secrets to the agent

Provide `APP_ID` and the `.pem` to the agent **via the terminal only** —
never paste private-key material into chat. Run these commands yourself
(replace `12345` with the actual App ID, and the date in the `.pem`
filename with the real download date):

```sh
APP_ID=12345
gh secret set THRILLMADE_ORCHESTRATOR_APP_ID \
  --org thrillmade \
  --visibility all \
  --body "$APP_ID"

cat ~/Downloads/skdd-steward.2026-06-03.private-key.pem \
  | gh secret set THRILLMADE_ORCHESTRATOR_PRIVATE_KEY \
      --org thrillmade \
      --visibility all
```

Once both secrets are stored, securely delete the local `.pem`:

```sh
rm ~/Downloads/skdd-steward.2026-06-03.private-key.pem
```

Note: `rm` is not a secure-erase on APFS/HFS+/most filesystems — the file
remains recoverable until the blocks are reused. If your threat model
requires unrecoverable deletion, use `shred -u` (Linux) or `srm` (macOS,
if installed), or simply rotate the key (see "Rotation procedure" below)
whenever you have reason to suspect compromise; rotation invalidates the
old PEM regardless of whether physical bits survived.

## Workflow integration

Add the following block to `.github/workflows/release.yml`, immediately
before the GoReleaser steps. Pass `steps.app_token.outputs.token` as the
`HOMEBREW_TAP_PAT` env value on both the snapshot and release GoReleaser
steps — the env-var name stays unchanged for a minimal diff; the secret
behind it is now an App token.

```yaml
      # actions/create-github-app-token@v3 is the official action and the
      # only App-token action compatible with this repo's
      # `allowed_actions: selected` workflow allowlist (tibdex/github-app-token
      # is NOT permitted — verified-Marketplace and actions/* only).
      - name: Mint orchestrator App installation token
        id: app_token
        uses: actions/create-github-app-token@v3
        with:
          app-id: ${{ secrets.THRILLMADE_ORCHESTRATOR_APP_ID }}
          private-key: ${{ secrets.THRILLMADE_ORCHESTRATOR_PRIVATE_KEY }}
          owner: thrillmade
          repositories: homebrew-tap
```

Then, on the release-path GoReleaser step:

```yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          # Was secrets.HOMEBREW_TAP_PAT (personal PAT, retired post-G1.f).
          # Now sourced from the steward App's installation token.
          HOMEBREW_TAP_PAT: ${{ steps.app_token.outputs.token }}
```

The snapshot path keeps `HOMEBREW_TAP_PAT: ${{ secrets.GITHUB_TOKEN }}`
unchanged — `--skip=homebrew` is active there, so no real cross-repo
push happens.

The `.goreleaser.yaml` `homebrew_casks` block needs three matching edits
(handled in G1.e.2):

- Drop the `pull_request:` sub-block — the App bypasses the ruleset, so
  direct push to `thrillmade/homebrew-tap/main` is permitted.
- Keep `repository.owner: thrillmade` and
  `repository.token: "{{ .Env.HOMEBREW_TAP_PAT }}"` (env-var name
  unchanged).
- Update `commit_author.name` to `skdd-steward[bot]` and
  `commit_author.email` to
  `<APP_ID>+skdd-steward[bot]@users.noreply.github.com` —
  GitHub's canonical App-bot author format.

## Verification

Before tagging `v1.0.0-rc3`, confirm the wiring with three checks.

Verify both org secrets exist (visibility should now be org-wide):

```sh
gh api /orgs/thrillmade/actions/secrets | jq
```

Verify the ruleset bypass entry is in place (look for an `Integration`
actor entry with `actor_id` matching the App ID):

```sh
gh api /repos/thrillmade/homebrew-tap/rulesets/17128312 \
  | jq '.bypass_actors'
```

Smoke-test the token mint without tagging a real release by triggering a
`workflow_dispatch` dry-run on `release.yml`. Confirm the
"Mint orchestrator App installation token" step succeeds and the snapshot
GoReleaser step completes — even though `--skip=homebrew` means no actual
cross-repo push happens in dry-run mode, a successful token mint proves
secrets + App-install + permission scope are all wired correctly.

```sh
gh workflow run release.yml -f dry_run=true
gh run watch
```

## Rotation procedure

If the App's private key needs rotation (suspected compromise, scheduled
rotation cycle, or after the lossy delete in Step 6 if you want a clean
slate), follow these steps. Generate a new key first, then revoke the old
one — never the reverse, or you'll lock out the workflow mid-rotation.
Navigate to
`https://github.com/organizations/thrillmade/settings/apps/skdd-steward`,
scroll to "Private keys", click "Generate a private key" to download a
new `.pem`, then re-run the `gh secret set THRILLMADE_ORCHESTRATOR_PRIVATE_KEY`
command from Step 6 with the new file. Confirm a `workflow_dispatch`
dry-run succeeds with the new key in place, then return to the Private
keys section and click "Delete" on the old key. The App ID never changes
during a key rotation, so `THRILLMADE_ORCHESTRATOR_APP_ID` does not need
to be updated.

## Audit log expectations

Every direct push, PR comment, file write, or API call made under the
steward's installation token appears in the org audit log under the
App's bot identity. Verify in the GitHub UI at
`https://github.com/organizations/thrillmade/settings/audit-log` and
filter with `actor:skdd-steward[bot]`. Expect one entry per cask-bump
direct push on `thrillmade/homebrew-tap` (`git.push` event, authored by
`skdd-steward[bot]`) for each `v*` tag release, plus, since the census
shipped, periodic `issues.create` / `issues.comment` entries from the
weekly `skill-census.yml` run. The bypass entry on ruleset 17128312 is the
policy expression of the cask-bump audit trail — the ruleset itself
records `rules.bypass` events when the App exercises its bypass, providing
a second independent log of the same activity.

**Events from before the rename** (the App was previously named
`thrillmade-orchestrator`) still appear in the audit log and in this
repo's own decision-log history under the old actor name,
`thrillmade-orchestrator[bot]`. Filtering `actor:skdd-steward[bot]` will
not surface those — filter `actor:thrillmade-orchestrator[bot]` instead
for anything dated before the rename.
