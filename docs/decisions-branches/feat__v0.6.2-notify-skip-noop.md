## 2026-05-31 13:56 - v0.6.2: notify-agent-skills skips PR creation when SKILL.md diff is no-op

**Reasoning:** User flagged churn: 5 of 10 recent notify-bot PRs were +0/-0 for skills/logmind/SKILL.md (TODO context file only). Pre-v0.6.2 logic unconditionally opened a PR even when Claude judged the release skill-irrelevant or proposed byte-identical content.

**Alternatives considered:** Open PR then auto-close it (rejected: still creates feed noise + extra workflow runs), Auto-add 'noise' label so maintainers can filter (rejected: still requires opening + manual filter rule; doesn't solve the underlying churn), Defer to a manual maintainer-only review tool (rejected: noise grows linearly with release cadence; root cause should be fixed at source)

**Implications:**
- notify-agent-skills.yml fires only on logmind tag pushes — affects logmind's own release pipeline, no consumer-side propagation needed
- Workflow-run logs preserve Claude's reasoning + the CHANGELOG section even when no PR opens; debugging is still possible
- First test of the fix happens on the next logmind tag — likely v0.6.2 itself (recursive: this very release's notify-workflow run will exercise the new diff-check step)

---
## 2026-05-31 14:13 - v0.6.2 fix: remove unreachable else-branch in notify-skill commit step (claude-review PR #96)

**Reasoning:** claude-review flagged dead code: the new diff-check step guarantees proposed-skill.md exists AND differs, so the else-branch handling 'Claude judged skill-irrelevant' (TODO-only PR shape) can never execute. Simplifying to unconditional cp + SHAPE assignment, plus dropping the now-stale 'TODO-only PR' line from the reviewer checklist.

**Alternatives considered:** Leave dead code as defense-in-depth comment (rejected: rots if diff-check logic changes later; cleaner to delete now), Resolve thread + ship as-is (rejected: dead code is the kind of soft signal that becomes load-bearing wrong when someone refactors next year)

**Implications:**
- Reviewer checklist now has 2 items instead of 4 — every PR has the same shape, no per-PR conditional reading needed
- Commit message SHAPE string is now constant; could be inlined but keeping the variable preserves a single edit-point if shape descriptor evolves

---
