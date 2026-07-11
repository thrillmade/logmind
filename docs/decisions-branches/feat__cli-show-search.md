← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-11-implement-logmind-show-and-search-the-documented-decision-re -->
- **2026-07-11** — Implement logmind show and search — the documented decision-read commands salvaged from retired PR #154
<!-- logmind-entry-end -->

## 2026-07-11 14:18 - Implement logmind show and search — the documented decision-read commands salvaged from retired PR #154

**Reasoning:** SKILL.md, AGENTS.md, and internal/templates/logmind-section.md all document show and search but the v1.0 Go rewrite dropped both, so the documented interface silently 404s. PR #154 built these against the pre-branch-aware v1 shape; main has since moved to branch-aware decision routing via resolveDecisionsPath, so a straight cherry-pick would regress current-branch semantics and the v2 quiet contract.

**Alternatives considered:** Cherry-pick PR #154 verbatim, Leave the commands undocumented instead of implementing them

**Implications:**
- show and search now resolve the SAME file logmind log/headline just wrote via resolveDecisionsPath, so current-branch scoping is correct out of the box; matches the documented current-branch contract more precisely than PR 154 did.
- Both commands follow the repomap.go quiet precedent: --quiet suppresses the payload entirely and emits one ok k=v receipt, since the verbatim/search output is the deliverable, not chatter.
- search's flag surface is intentionally narrower than PR 154 (only --case-sensitive and --no-archive, the documented flags); context lines are a fixed default of 2, not a flag.

---

