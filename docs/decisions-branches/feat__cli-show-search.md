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

## 2026-07-11 14:32 - Address PR #191 review: literal substring search + branch-spanning scope

**Reasoning:** Regex matching gave silent false negatives (a valid-but-non-matching pattern like cost dollar-paren drops the hit) and over-matches (dot, pipe as metachars); a keyword search must be literal. And a feature-branch agent needs to find main's decisions, so search must span decisions.md, not just the current-branch file like show does.

**Alternatives considered:** Keep regex with a literal fallback only on compile error (rejected: still wrong for valid-but-non-matching patterns), Add a --regex flag (rejected: undocumented, widens the promised surface)

**Implications:**
- search now matches via strings.Contains with case folding; highlightLiteral wraps the actual matched substring by index, so regex-special queries highlight correctly with no pattern compiled
- search source order is decisions.md, then the branch file if different, then archive unless --no-archive, deduped by path; show stays current-branch-only per the docs
- empty query now errors via q.fail; quiet ok line gained a sources=N field

---

