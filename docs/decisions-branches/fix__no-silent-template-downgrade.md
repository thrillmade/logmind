← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-07-refuse-to-move-a-workflow-template-backwards-and-say-so -->
- **2026-08-07** — Refuse to move a workflow template backwards, and say so
<!-- logmind-entry-end -->

## 2026-08-07 17:35 - Refuse to move a workflow template backwards, and say so

**Reasoning:** installWorkflowTemplates tested marker INEQUALITY, not ordering: 'installedVer != bundledVer' overwrote in either direction. A repo carrying v11 refreshed by a binary bundling v4 was silently rewritten to v4 and told '↻ Refreshed ... to current template', which reads as an upgrade. It was seven marker versions backwards. This is not hypothetical: brew install gives v1.2.0, which bundles v4, so any fleet migration run with a released binary installs v4 AND reverts every repo already moved forward — including protocol, which is taking v11 by hand in protocol#92 right now.

**Alternatives considered:** Compare markers as strings. Rejected outright — 'v11' < 'v4' lexically, so a string compare gets this exactly backwards and would have looked like it worked on v4-vs-v9. The comparison parses the integer after the 'v' and ignores any '-pointer' suffix.

**Implications:**
- Only the downgrade direction changes; upgrades and equal versions behave exactly as before. An unparseable marker on either side falls back to the old refresh-on-inequality path rather than silently doing nothing, so an unreadable marker cannot become a way to pin a stale template forever. The refusal surfaces through BOTH callers — logmind init and doctor --fix — via refreshResult.WorkflowsDeclined and one shared formatter, placed before doctor's --json branch so the refusal survives --json without polluting stdout. Regression tests fail against the old behaviour, verified by deleting the guard and watching them reproduce #286 verbatim. Probing found the same downgrade shape in the AGENTS.md marker block (different root cause — matchingTemplate's hardcoded enum, which is #267) and in git hooks (where last-binary-wins may be intended); both left alone and reported.

---

