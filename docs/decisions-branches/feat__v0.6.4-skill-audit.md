<!-- logmind-entry-start: 2026-06-01-v0-6-4-skill-audit-author-s-side-staleness-read-stream-6-fol -->
- **2026-06-01** — v0.6.4: skill audit — author's-side staleness read (Stream 6 follow-on)
<!-- logmind-entry-end -->

## 2026-06-01 00:41 - v0.6.4: skill audit — author's-side staleness read (Stream 6 follow-on)

**Reasoning:** Pairs with clud-bug usage --health for complete skill-lifecycle visibility. audit reports what's HERE (filesystem + git side); usage reports what's USED (load + cite side). Together they show whether each skill earns its place: ghost (loaded but never iterated AND not cited), aging (untouched + uncited), active (touched + cited).

**Alternatives considered:** Combine audit + usage into one logmind skill status command (rejected: violates clean separation between author-side data and review-side data; users may not have clud-bug installed, audit should standalone), Use heuristic LLM scoring for staleness (rejected: deterministic thresholds match the pragmatic SkDD pivot — humans interpret, not algorithms guess)

**Implications:**
- decision_count uses substring match — works for both top-level decisions.md + decisions-branches/*.md; doesn't distinguish 'X was used' from 'X was deprecated', but the count signal is enough for the dashboard's coarse bucketing
- Threshold for ghost (decision_count==0 AND bytes>2000) intentionally picks the same tight threshold as bench's 'tight' bucket. Small skills with no decision mentions are treated as active because they may just be new/simple

---
