<!-- logmind-entry-start: 2026-05-27-feat-v0-4-0-notify-agent-skills-yml-opens-a-claude-proposed- -->
- **2026-05-27** — feat(v0.4.0): notify-agent-skills.yml opens a Claude-proposed SKILL.md PR (not an issue)
<!-- logmind-entry-end -->

## 2026-05-27 15:27 - feat(v0.4.0): notify-agent-skills.yml opens a Claude-proposed SKILL.md PR (not an issue)

**Reasoning:** Sentinel NO_SKILL_UPDATE_NEEDED for internal releases (CI tweaks, refactors). Always writes .skill-update-todo/vX.Y.Z.md with full context. Failure-mode fallback to v0.3.x issue notification preserves shipping discipline

**Alternatives considered:** Auto-merge from day one — rejected; quality has to be observed across 1-3 dogfooded releases first, Issue-with-attached-content-suggestion instead of full PR — rejected; PR shape forces the diff into review surface where it belongs

**Implications:**
- First Anthropic API direct integration in the org (not via claude-code-action). The pattern generalizes to future downstream-sync surfaces: (changelog slice + current target) → propose. Worth keeping the helper script's interface stable so a future workflow can reuse it
- Verification: v0.4.0 release itself dogfoods — its tag push fires the new workflow, which should open a PR on agent-skills proposing an update for v0.4.0's own changes

---
