# `thrillmade-orchestrator[bot]` GitHub App

Reference manifest and click-by-click registration guide for the
`thrillmade-orchestrator[bot]` GitHub App. Committed to logmind for
reproducibility — if the App is ever rotated, deleted, or migrated to a
different org, the steps below produce an identical replacement.

Mirrors the schema of `/Users/ludlow/clud-bug-app/github-app-manifest.json`
(thrillmade's first App). Permissions and event subscriptions differ — the
orchestrator has a narrower surface than `clud-bug[bot]`.

## Purpose

`thrillmade-orchestrator[bot]` is the App identity behind all cross-repo
writes that originate from a thrillmade-owned workflow. In v1.0 it bumps the
Homebrew cask formula on `thrillmade/homebrew-tap` after every `logmind`
release (replacing the personal `HOMEBREW_TAP_PAT` that was a key-person
single point of failure). Per the master plan at
`/Users/ludlow/.claude/plans/ok-here-is-recent-distributed-chipmunk.md`,
section "Architectural decisions in force — GitHub Apps are the primitive
for release infrastructure", we register an App rather than mint a PAT or
spin up a machine-user account: PATs require periodic rotation and tie the
release pipeline to one human's GitHub account, and machine-user accounts
add real-account-maintenance overhead with no narrower audit story than an
App. Tokens are 1-hour installation tokens minted at workflow time by the
official `actions/create-github-app-token@v2` action — the only App-token
action compatible with the repo's `allowed_actions: selected` workflow
allowlist. The App pre-positions the org for G7.n / D.10 (skill-catalog PRs
to `thrillmade/agent-skills` via `logmind skill push`, and eventually the
release-tag fan-out that consolidates per-repo `*-self-update.yml`
workflows into a single org-wide watcher).

## App identity

- **Name:** `thrillmade-orchestrator`
- **Slug:** `thrillmade-orchestrator` (bot identity: `thrillmade-orchestrator[bot]`)
- **Homepage URL:** `https://github.com/thrillmade` (org page — the App is internal infra, not a logmind feature)
- **Owner:** `thrillmade` organization
- **Public:** no (private to the thrillmade org)

## Permissions

Repository-level only. No user permissions. No organization permissions.

| Permission | Access | Why |
|---|---|---|
| Contents | Read and write | Direct push of `Casks/logmind.rb` bumps to `thrillmade/homebrew-tap/main`; future skill-catalog file writes. |
| Pull requests | Read and write | v1.x expansion (G7.n) opens skill-catalog PRs on `thrillmade/agent-skills`; reserved now to avoid a re-registration when that lands. |
| Metadata | Read | Mandatory baseline for every App; grants read on repo name, default branch, topics. |

## Events

None.

The orchestrator is workflow-driven — it never subscribes to webhook events.
Tokens are minted on demand inside `release.yml`; there is no Vercel
function and no event listener.

## Installation scope

Selected repositories.

- **v1.0:** `thrillmade/homebrew-tap` only.
- **v1.x expansion** (per G7.n in the master plan
  `/Users/ludlow/.claude/plans/ok-here-is-recent-distributed-chipmunk.md`):
  add `thrillmade/agent-skills` when `logmind skill push` lands; later add
  consumer repos in the self-update fan-out wave.

Expansion happens via the App's Installation settings page on the org —
never re-register the App, never widen scope preemptively.

## Secrets storage

Two org-level secrets on `thrillmade` with `selected` visibility. v1.0
ships with only `thrillmade/logmind` in the visibility list. Expand the
repo list as G7.n features ship.

- `THRILLMADE_ORCHESTRATOR_APP_ID` — the App's numeric ID (not a secret in
  the cryptographic sense, but stored as a secret to keep the wiring
  uniform across workflows).
- `THRILLMADE_ORCHESTRATOR_PRIVATE_KEY` — the RSA PEM downloaded from the
  App's settings page.

Exact commands (run after the user provides `APP_ID` and the `.pem` path
via terminal — never paste private-key material into chat):

```sh
APP_ID=12345
gh secret set THRILLMADE_ORCHESTRATOR_APP_ID \
  --org thrillmade \
  --visibility selected \
  --repos thrillmade/logmind \
  --body "$APP_ID"

cat ~/Downloads/thrillmade-orchestrator.YYYY-MM-DD.private-key.pem \
  | gh secret set THRILLMADE_ORCHESTRATOR_PRIVATE_KEY \
      --org thrillmade \
      --visibility selected \
      --repos thrillmade/logmind
```

The legacy `HOMEBREW_TAP_PAT` org secret is retired after G1.f passes.

## Ruleset bypass on `thrillmade/homebrew-tap`

Ruleset 17128312 on `main` enforces deletion + non_fast_forward +
required_linear_history + pull_request (0 reviews, squash only). The
orchestrator App is added as a bypass actor so its direct pushes are
exempt; human pushes still go through PR review. The bypass IS the audit
trail — every direct push appears in the org audit log under the App
identity.

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
| GitHub App name | `thrillmade-orchestrator` |
| Homepage URL | `https://github.com/thrillmade` |
| Description | `Org-level orchestrator for thrillmade release infrastructure — pushes cask bumps, skill-catalog updates, and self-update fan-out across thrillmade repos.` |
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
| Metadata | Read-only (default; cannot be unset) |

**Organization permissions:** all "No access".

**User permissions:** all "No access".

**Subscribe to events:** check NOTHING.

**Where can this GitHub App be installed?** Only on this account.

### Step 3 — Create the App

Click "Create GitHub App" at the bottom of the form. GitHub redirects to
the App's settings page. Record the **App ID** displayed near the top of
that page (also visible in the URL after a brief redirect:
`https://github.com/organizations/thrillmade/settings/apps/thrillmade-orchestrator`).

### Step 4 — Generate the private key

On the App's settings page, scroll to the "Private keys" section. Click
"Generate a private key". The browser downloads a `.pem` file named
`thrillmade-orchestrator.YYYY-MM-DD.private-key.pem` to your default
download directory.

Move the file somewhere outside the repo working tree — do not commit it,
do not check it into any logmind clone. The `~/Downloads/` default
location is fine until Step 6.

### Step 5 — Install the App on `thrillmade/homebrew-tap`

In the left sidebar of the App's settings page, click "Install App". Find
the `thrillmade` row and click "Install" next to it. On the install page:

1. Select "Only select repositories".
2. Check `thrillmade/homebrew-tap`.
3. Click "Install".

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
  --visibility selected \
  --repos thrillmade/logmind \
  --body "$APP_ID"

cat ~/Downloads/thrillmade-orchestrator.2026-06-03.private-key.pem \
  | gh secret set THRILLMADE_ORCHESTRATOR_PRIVATE_KEY \
      --org thrillmade \
      --visibility selected \
      --repos thrillmade/logmind
```

Once both secrets are stored, securely delete the local `.pem`:

```sh
rm ~/Downloads/thrillmade-orchestrator.2026-06-03.private-key.pem
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
      # actions/create-github-app-token@v2 is the official action and the
      # only App-token action compatible with this repo's
      # `allowed_actions: selected` workflow allowlist (tibdex/github-app-token
      # is NOT permitted — verified-Marketplace and actions/* only).
      - name: Mint orchestrator App installation token
        id: app_token
        uses: actions/create-github-app-token@v2
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
          # Now sourced from the orchestrator App's installation token.
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
- Update `commit_author.name` to `thrillmade-orchestrator[bot]` and
  `commit_author.email` to
  `<APP_ID>+thrillmade-orchestrator[bot]@users.noreply.github.com` —
  GitHub's canonical App-bot author format.

## Verification

Before tagging `v1.0.0-rc3`, confirm the wiring with three checks.

Verify both org secrets exist (visibility list should include
`thrillmade/logmind`):

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
`https://github.com/organizations/thrillmade/settings/apps/thrillmade-orchestrator`,
scroll to "Private keys", click "Generate a private key" to download a
new `.pem`, then re-run the `gh secret set THRILLMADE_ORCHESTRATOR_PRIVATE_KEY`
command from Step 6 with the new file. Confirm a `workflow_dispatch`
dry-run succeeds with the new key in place, then return to the Private
keys section and click "Delete" on the old key. The App ID never changes
during a key rotation, so `THRILLMADE_ORCHESTRATOR_APP_ID` does not need
to be updated.

## Audit log expectations

Every direct push, PR comment, file write, or API call made under the
orchestrator's installation token appears in the org audit log under the
App's bot identity. Verify in the GitHub UI at
`https://github.com/organizations/thrillmade/settings/audit-log` and
filter with `actor:thrillmade-orchestrator[bot]`. Expect one entry per
cask-bump direct push on `thrillmade/homebrew-tap` (`git.push` event,
authored by `thrillmade-orchestrator[bot]`) for each `v*` tag release.
The bypass entry on ruleset 17128312 is the policy expression of this
audit trail — the ruleset itself records `rules.bypass` events when the
App exercises its bypass, providing a second independent log of the
same activity.
