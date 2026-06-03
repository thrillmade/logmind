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
