← back to [docs/timeline.md](../timeline.md)

## 2026-07-03 13:22 - skill: drop the dead Python-API section from the logmind skill (off-Python completeness)

**Reasoning:** skill/SKILL.md — the canonical logmind agent skill (source-of-truth for the agent-skills catalog, auto-loaded by agents) — still showed a 'from logmind import log' Python section: the same Go-binary-misleading cruft #177 removed from the templates, flagged by the #177 review. The CLI is the real interface.

**Alternatives considered:** Leave it (rejected — it's the skill agents AUTO-LOAD; advertising a nonexistent Python API is actively misleading)

**Implications:**
- skill/SKILL.md is now Python-free; the 'logmind log' CLI is the sole how-to-log; propagates to the agent-skills catalog on the next skill push. Completes the off-Python tail (item 1 of the build-all-then-flip v1.0 plan). docs/plan.md's copies are historical planning prose, left as-is.

---

