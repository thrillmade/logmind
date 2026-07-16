<!-- logmind-entry-start: 2026-06-03-wire-thrillmade-orchestrator-bot-app-for-cask-bump-g1-e-2 -->
- **2026-06-03** — Wire thrillmade-orchestrator[bot] App for cask-bump (G1.e.2)
<!-- logmind-entry-end -->

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
## 2026-06-03 12:04 - Link docs/orchestrator-app.md from AGENTS.md to satisfy check-links

**Reasoning:** check-links workflow flagged docs/orchestrator-app.md as orphan markdown on PR #132 (the v1.0 cutover). The doc IS the canonical reference for the thrillmade-orchestrator[bot] App spec; adding a short Release-infrastructure section in AGENTS.md provides the inbound link and surfaces the App spec to agents working in this repo (which is exactly where the link belongs — release infra is project-level guidance).

**Implications:**
- Reduces blocking checks on the cutover PR to just paths-check (which is structural — PR diff exceeds GitHub 20k-line API limit and cannot be made to pass without splitting the cutover)

---
## 2026-06-03 13:07 - Replace pytest matrix with Go test in test.yml; retire Python-publish workflows (B+C step B)

**Reasoning:** Cutover PR #132 is blocked because the org-default-protection ruleset on main requires a status named test, and the existing producer (test.yml pytest matrix) is incompatible with the Go codebase that this PR lands. Per a B+C plan vetted by two sub-agents (world-class CTO + world-class Principal Engineer perspectives, independent + convergent), the correct play is to FIRST land the Go test producer on v1-go-rewrite so the squash payload merging into main already satisfies the required-check name. Removes the obsolete pytest matrix in the same commit (cutover is the right moment, not later) and also retires three Python-only publishing workflows whose target distribution (PyPI + Python brew formula) is superseded by the Go binary release + GoReleaser cask flow. After this commit, the cutover PR has check-links + check-decisions + check-derived-docs + test all passing; only clud-bug-review remains in skipping state (correctly recused from whole-codebase rewrites). That narrow recusal is the documented bypass for the upcoming admin-squash merge.

**Alternatives considered:** Option A — remove test from required_status_checks on main ruleset. Permanently weakens the check list to ship one PR; sets a bad precedent (rule inconvenient → edit rule). Both sub-agents identified this as the canonical tactical-fix-becomes-load-bearing mistake., Option C-only — admin merge tonight + add test.yml as follow-up. Window where main has a required check with no producer; next PR hits the same wall and the obvious unblock is then Option A. Socializes the wrong precedent., Keep pytest matrix alive as a transitional gate. Adds maintenance burden (CI installs Python + pip every run) on a codebase that no longer ships Python.

**Implications:**
- test.yml job name "test" pinned to match the ruleset context exactly; matrix-expanded cells produce per-cell contexts that the gate does not read directly — the aggregator is what satisfies the required check
- go-test.yml deleted (consolidated into test.yml — single source of truth for the test gate)
- publish.yml + testpypi.yml deleted: PyPI wheel publication ends at v0.6.16 (last-published frozen); G1.h adds a PyPI deprecation notice as a follow-up
- homebrew-bump.yml deleted: Python brew formula no longer maintained; cask bumps go through GoReleaser via thrillmade-orchestrator[bot] now
- Next: wait for test check to go green on PR #132, then narrowly-scoped admin squash-merge with documented justification (clud-bug-review skipping is the only remaining bypass)

---
