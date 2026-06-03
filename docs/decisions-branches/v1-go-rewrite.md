## 2026-06-03 10:13 - Wire thrillmade-orchestrator[bot] App for cask-bump (G1.e.2)

**Reasoning:** rc2 cask-bump failed because PUT /contents on protected main was blocked by ruleset 17128312 (PR-only, empty bypass_actors). Personal HOMEBREW_TAP_PAT cannot bypass the ruleset and ties release infra to one human account (rotation cost + key-person risk + audit blur). Orchestrator App as ruleset bypass actor solves access + identity together; 1-hour installation tokens minted via actions/create-github-app-token@v2 (allowlist-compatible — tibdex would have been rejected by allowed_actions selected). See docs/orchestrator-app.md for App spec + rotation procedure.

**Alternatives considered:** Option C (personal PAT + GoReleaser fork-flow + manual cask-bump PR merge), Machine user thrillmade-bot with repo-scoped PAT (unfashionable; no narrower audit story than an App; real-account maintenance overhead), Disable branch protection on thrillmade/homebrew-tap (loses guardrails for human pushes)

**Implications:**
- Replaces secrets.HOMEBREW_TAP_PAT with steps.app_token.outputs.token in release.yml; legacy PAT retired post-G1.f
- Orchestrator App (id 3951953) added as bypass actor on tap ruleset 17128312 with actor_type=Integration, bypass_mode=always
- .goreleaser.yaml homebrew_casks: pull_request sub-block dropped (App bypass enables direct push); commit_author updated to thrillmade-orchestrator[bot] / 3951953+thrillmade-orchestrator[bot]@users.noreply.github.com
- release.yml gains an unconditional Mint orchestrator App installation token step so dry-runs sanity-check the wiring
- Future expansion (G7.n / D.10): add App as bypass actor on additional rulesets (agent-skills, consumer repos) as features ship — never re-register the App
- Gitignores .claude/worktrees/ (agent-local scratch state, never commit)

---
## 2026-06-03 11:54 - fix(B7): GORELEASER_CURRENT_TAG to disambiguate double-tagged commits

**Reasoning:** v1.0.0 release run on 2026-06-03 15:39 failed because tag v1.0.0 was pushed on the same commit (f6c38ba) as v1.0.0-rc3. GoReleaser falls back to `git describe --tags` when GITHUB_REF is not explicitly read, and git describe returned v1.0.0-rc3 (the older of the two tags pointing at that SHA). GoReleaser rebuilt the rc3 artifacts and tried to re-upload them to the rc3 GitHub Release, hitting 422 already_exists. Setting GORELEASER_CURRENT_TAG=github.ref_name forces the value from the workflow trigger, making the resolution unambiguous.

**Alternatives considered:** Always tag releases on unique commits — works but adds release-engineering friction (need a placeholder commit between rc and final), Use goreleaser/goreleaser-action input `args: release --release-notes-file ...` with explicit tag — no such flag exists, Delete the rc tag before tagging final — would orphan the rc GitHub Release that the cask currently points at

**Implications:**
- Gated on event_name==push so snapshot mode (workflow_dispatch with --snapshot) is unaffected — ref_name is a branch there, not a tag
- Future re-tagging on a shared SHA (rc-and-then-final, or hotfix tagged off a release commit) now works without git describe ambiguity
- rc3 release at f6c38ba stays canonical; v1.0.0 needs a fresh commit (this commit) plus the re-tag — that is exactly what comes next

---
