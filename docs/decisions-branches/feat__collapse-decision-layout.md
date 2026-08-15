← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-14-collapse-the-decision-layout-main-is-a-branch-the-timeline-g -->
- **2026-08-14** — Collapse the decision layout: main is a branch, the timeline gets the cap
<!-- logmind-entry-end -->

## 2026-08-14 22:14 - Collapse the decision layout: main is a branch, the timeline gets the cap

**Reasoning:** SPEC 3.2 makes decision files append-only and uncapped, and 3.3 moves the bound to the rendering: timeline.md carries the 50 most recent, everything older renders to timeline-archive.md, both from the same sources every time. The old model capped the RECORD and archived by moving, which is why five repos showed 18 main-log entries and zero archive entries: nothing could commit to main directly, so the cap never fired and the archive never filled.

**Alternatives considered:** Work from the deletion list in the issue. Rejected in favour of grepping for the existing pair and each deleted symbol, which found sites the list omits and one defect the list could not contain: git checkout HEAD -- a b c and git add a b c are ALL-OR-NOTHING. With any pathspec untracked they restore or stage nothing, exit 1 and 128, both files left dirty. Naively adding the third path to the shell hooks would have silently switched the L2a restore OFF in every repo that has not yet regenerated on main. Both hook bodies now loop one path at a time.

**Implications:**
- The split cannot become a move by construction: timeline.Generate returns both halves from one collectMarked, neither output is ever an input, and there is no flag to write one without the other. Measured here: 50 recent, 134 archive. Two mutations SURVIVED on the first pass and both were tests on the thing changed rather than on observable output — a config guard placed on LoadAsMap instead of Load, and an integration test that ITERATED the very list it was pinning. Both replaced; twelve mutations now die. docs/decisions.md stays readable because 18 entries exist across five un-migrated repos and dropping the read would lose them the moment a repo upgrades. One consequence needs a ruling and is raised separately: a branch file contributes exactly one timeline row, so main.md 12 entries collapse to one dated at its oldest, and main will accumulate forever behind a row frozen at 2026-05-15.

---

