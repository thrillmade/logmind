## 2026-05-31 13:56 - v0.6.2: notify-agent-skills skips PR creation when SKILL.md diff is no-op

**Reasoning:** User flagged churn: 5 of 10 recent notify-bot PRs were +0/-0 for skills/logmind/SKILL.md (TODO context file only). Pre-v0.6.2 logic unconditionally opened a PR even when Claude judged the release skill-irrelevant or proposed byte-identical content.

**Alternatives considered:** Open PR then auto-close it (rejected: still creates feed noise + extra workflow runs), Auto-add 'noise' label so maintainers can filter (rejected: still requires opening + manual filter rule; doesn't solve the underlying churn), Defer to a manual maintainer-only review tool (rejected: noise grows linearly with release cadence; root cause should be fixed at source)

**Implications:**
- notify-agent-skills.yml fires only on logmind tag pushes — affects logmind's own release pipeline, no consumer-side propagation needed
- Workflow-run logs preserve Claude's reasoning + the CHANGELOG section even when no PR opens; debugging is still possible
- First test of the fix happens on the next logmind tag — likely v0.6.2 itself (recursive: this very release's notify-workflow run will exercise the new diff-check step)

---
