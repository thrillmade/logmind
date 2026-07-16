<!-- logmind-entry-start: 2026-05-30-feat-v0-5-13-5-item-quality-batch-4b-2-workflow-pin-auto-upd -->
- **2026-05-30** — feat(v0.5.13): 5-item quality batch — §4b.2 + workflow pin auto-update + doctor non-zero + doctor stale-derived-docs warning + logmind rebase
<!-- logmind-entry-end -->

## 2026-05-30 12:11 - feat(v0.5.13): 5-item quality batch — §4b.2 + workflow pin auto-update + doctor non-zero + doctor stale-derived-docs warning + logmind rebase

**Reasoning:** Closes recurring gotcha #1 (workflow pin staleness across clud-bug update cycles) + addresses the tokenomics agent's 2026-05-30 Phase D merge-order pain (3-PR batch went DIRTY when middle PR merged first). Five items shipped in one batch since they share a logical theme (logmind quality + merge-resilience) and consumers prefer one workflow-re-render PR over five.

**Alternatives considered:** ship items one-by-one as v0.5.13/14/15/16/17 — rejected, batching reduces propagation overhead for the same release surface, skip §4b.2 — rejected, it was the original v0.5.8 promise that got deferred, defer #4/#5 to v0.5.14 (tokenomics enhancements separate) — rejected, they're load-bearing for the merge-order story #2/#3 enable

**Implications:**
- Bumps AGENTS.md.slim.template marker from v6-pointer to v7-pointer. Consumer repos will need to run logmind agents update --apply on their next logmind upgrade to pick up the new lead-line ordering.
- logmind agents update --apply now does TWO things instead of one (AGENTS.md block + CI workflow pin). Surfaces both in the dry-run summary so users see what changes.
- logmind doctor now exits non-zero on missing merge-driver config inside a git repo. CI pipelines that gate on 'logmind doctor' will start surfacing pre-merge config drift before it causes check-derived-docs failures.
- Tokenomics agent's Phase D pain (out-of-order merge → DIRTY trailing PRs) is now predicted + remediated by tools: doctor #4 warns before push; rebase #5 fixes in one command.
- Outreach to tokenomics agent unblocked once v0.5.13 publishes — they can upgrade from logmind==0.5.6 → logmind==0.5.13 and get auto-install (v0.5.12) + new rebase command + stale-derived-docs warning in one bump.

---
