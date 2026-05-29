# Decision Log

This file contains the 20 most recent decisions. Older decisions are archived in [decisions-archive.md](decisions-archive.md).

---
## 2026-05-15 04:30 - Merged: dogfood-workflows (#15)

- **PR:** https://github.com/thrillmade/logmind/pull/15
- **Decisions:** 1 from this branch
- **Detail:** [decisions-branches/dogfood-workflows.md](decisions-branches/dogfood-workflows.md)

---
## 2026-05-15 17:34 - Merged: footer-polish (#32)

- **PR:** https://github.com/thrillmade/logmind/pull/32
- **Decisions:** 1 from this branch
- **Detail:** [decisions-branches/footer-polish.md](decisions-branches/footer-polish.md)

---
## 2026-05-28 11:28 - 0.A.9 audit: all 7 consuming repos pass Q4 + <200-line CLAUDE.md; zero Q5 path-scope candidates found

**Reasoning:** Read-only audit per plan. Surveyed AGENTS.md + CLAUDE.md across agent-skills, clud-bug, homebrew-logmind, logmind, reporulez, rezgen (local clones) + tokenomics (gh api). Findings: (1) Q4 pattern intact in every repo with files — AGENTS.md is the canonical real file (109-163 lines, 4.8-6.7KB), CLAUDE.md is a 48-line stub redirecting to AGENTS.md. (2) Every file is under 200 lines (largest = logmind at 163). (3) Q5 path-scope audit: scanned section structure of logmind (largest) and reporulez (second largest). All content is universal — decision logging (logmind block), required reading, dev setup, clud-bug collaboration. No rule blocks ≥20 lines scoped to a subdirectory. Q5 stays deferred to Phase 3 unless future content adds domain-scoped rules. (4) Zero repos use .claude/rules/; zero use symlinks. Pre-existing minor observation: logmind AGENTS.md has duplicate '## Development Commands' and '## Project Overview' sections (lines 62+85, 66+103) — stale agents-sync artifact, not in audit scope.

**Implications:**
- Phase 0.A.9 closes with no PRs needed. Phase A effectively complete (0.A.1-0.A.8, 0.A.10 shipped; 0.A.9 audited clean). 0.X.1 path-scope pilot remains Phase 3 conditional.

---
## 2026-05-29 13:12 - Defer 0.B.5 (docs/decisions.md per-entry compact) to Phase 3+ — structural overhead too small to be worth a release

**Reasoning:** PR #80 (0.B.6) just shipped the higher-leverage AGENTS.md-block trim (69% reduction, ~1.8KB saved per repo per AGENTS.md read). The 0.B.5 rubric ALSO passed (per_file_share[docs/decisions.md] = 0.389 ≥ 0.10, mean 6017 bytes ≥ 2048, 9 sessions with reads ≥ 5) — so by the plan's rubric, 0.B.5 ships. BUT inspecting the actual per-entry shape (logger._format_decision output: 213 bytes for a sample entry with all fields), the structural overhead available to trim is microscopic. Options: (1) drop inter-section blank lines: ~3 bytes per entry. (2) compact labels (Reasoning → Why, Alternatives considered → Alt, Implications → Impact): ~30 bytes per entry but breaks anyone who greps for the existing label pattern. (3) drop trailing blanks: ~1 byte. Net achievable lossless trim: ~5 bytes per entry vs the plan's 40% estimate of ~800 bytes per entry. The plan's estimate was overoptimistic — decisions.md entries are mostly content, not structure. Per-entry content (reasoning text, alternatives, implications) is what users wrote and shouldn't be touched.

**Alternatives considered:** Ship 0.B.5 with the conservative ~3-byte-per-entry blank-drop. Rejected: doesn't move the needle. Net savings across 9 sessions × 10 entries each = ~270 bytes per per-session run. Not worth a release + propagation., Ship 0.B.5 with compact labels (Reasoning→Why, etc.) for ~30 bytes per entry. Rejected: breaks grep patterns in user scripts + agent skills + clud-bug-collaboration documentation. Information identity is more important than 30 bytes., Re-define 0.B.5 as 'compact docs/decisions-archive.md per-entry shape' (older entries that agents read less). Rejected: deferred-to-Phase-3+ list already includes 'docs/decisions-branches/*.md per-entry compaction' as a similar item. Bundle there.

**Implications:**
- 0.B.6 alone captures the bulk of the AGENTS.md-direction savings. Org-cumulative per_session bench should show the per-file shares shift after consuming repos refresh to v0.5.6's v6-pointer block — agents_md_block_share will drop from 0.51 toward something like 0.20-0.30 (the new block is 69% smaller; remaining AGENTS.md content unchanged). docs/decisions.md share stays ~0.39 because we're not touching its content.
- Re-measure post-consumer-rollout. If the per_session data shows decisions.md reads are still a high share AND specific content patterns emerge that COULD be compressed (e.g. repeated boilerplate in merge entries — a legacy artifact from the removed logmind-aggregate workflow), revisit 0.B.5 with a more targeted trim. Until then, 0.B.5 stays Phase 3+ deferred with explicit measurement in this -i field.

---
