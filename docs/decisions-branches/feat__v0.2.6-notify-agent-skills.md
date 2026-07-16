<!-- logmind-entry-start: 2026-05-26-feat-v0-2-6-notify-agent-skills-workflow-closes-the-release- -->
- **2026-05-26** — feat: v0.2.6 — notify-agent-skills workflow closes the release→skill update gap
<!-- logmind-entry-end -->

## 2026-05-26 15:43 - feat: v0.2.6 — notify-agent-skills workflow closes the release→skill update gap

**Reasoning:** Until today, every logmind release left the canonical skill on agent-skills out of date until someone manually noticed and opened a PR. Mirror the agent-skills→clud-bug notify pattern: on every tag push, open an issue on thrillmade/agent-skills prompting a SKILL.md review. Closes the structural gap that produced today's batch update covering v0.2.3→v0.2.5.

**Alternatives considered:** Move SKILL.md into logmind repo and auto-push to agent-skills on release — couples the two repos too tightly; agent-skills is a multi-skill collection with its own release cadence, Bundle SKILL.md content INTO logmind package and have logmind init copy it locally — wouldn't help skills.sh consumers who fetch from the canonical repo

**Implications:**
- Future logmind releases will auto-open an issue on agent-skills; maintainer reviews and either updates SKILL.md or closes as no-op
- Requires AGENT_SKILLS_NOTIFY_PAT repo secret on logmind (fine-grained PAT, Issues:write on agent-skills). Without it, degrades to ::warning:: rather than failing the release
- Reusable pattern: same shape as notify-clud-bug.yml on agent-skills; both close manual-sync chores between paired repos

---
