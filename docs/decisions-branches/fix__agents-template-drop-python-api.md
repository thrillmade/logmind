← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-03-templates-drop-the-last-dead-python-api-block-from-agents-md -->
- **2026-07-03** — templates: drop the last dead Python-API block from AGENTS.md.template (completes the off-Python template purge)
<!-- logmind-entry-end -->

## 2026-07-03 13:00 - templates: drop the last dead Python-API block from AGENTS.md.template (completes the off-Python template purge)

**Reasoning:** AGENTS.md.template — the canonical agent-instruction file init writes to every consumer — still showed a 'from logmind import log' Python-API block, dead and misleading for the Go binary (there is no importable module). This was the LAST logmind-is-Python cruft in the shipped templates; the 'logmind log' CLI form directly above it is the real interface.

**Alternatives considered:** Also bump the block marker v6→v7 to force-propagate to existing installs (deferred: the SPEC hardcodes v6 as the full-variant marker, so a bump needs coordinated SPEC + agent-skills convention work — that rides the v1.0 flip; the content removal itself needs no bump)

**Implications:**
- Shipped templates are now Python-API-free (grep-clean). Marker stays v6 (a within-variant content refresh, not a variant change; the v6-pinned test strings are untouched). KEPT as intentional: the polyglot file-structure ignore patterns (.pytest_cache etc. — for Python/JS/Go CONSUMER repos) and the pip→Go migration off-ramp (self-update pip-detection + the live inserter.UpdateWorkflowPin).

---

