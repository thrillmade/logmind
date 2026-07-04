← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-04-bumps-the-full-agents-md-template-to-v7-carrying-the-branch- -->
- **2026-07-04** — Bumps the full AGENTS.md template to v7 carrying the branch-summary (headline) convention, and makes doctor --fix / init-refresh flavour-preserving so legacy full-block repos refresh to full v7 instead of flipping to slim.
<!-- logmind-entry-end -->

## 2026-07-04 15:18 - Bump AGENTS.md full block v6 to v7 with the branch-summary (headline) convention

**Reasoning:** The full AGENTS.md template is the tool-owned block installed into consumer repos; bumping its marker v6 to v7 with new content makes every full-block repo show drift so doctor --fix and agents update refresh them to v7. That is the delivery mechanism that carries the Slice-2 branch-summary convention into existing repos.

**Alternatives considered:** Ship the convention only via the logmind skill plus slim template, leaving full-block repos without it -- rejected because legacy full installs render the procedure inline and never load the skill body., Leave EnsureAgentsMD refreshing against the slim default -- rejected because it silently downgrades a legacy full v6 repo to slim on doctor --fix, contradicting the documented full-slim guard and the task goal of refreshing to full v7.

**Implications:**
- matchingTemplate now maps v5, v6 and v7 all to the full flavour so older full blocks refresh forward to v7.
- EnsureAgentsMD (the doctor --fix and init-refresh path) now selects the refresh template by the installed flavour via matchingTemplate, so a full v6 repo refreshes to full v7 rather than flipping to slim; verified idempotent.
- Fresh logmind init still writes the slim v8-pointer default; the v7 full body is only what legacy full-block repos refresh to.

---

