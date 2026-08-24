← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-24-check-derived-docs-v15-accept-either-merge-base-state-and-co -->
- **2026-08-24** — check-derived-docs v15: accept either merge-base state and compare content by blob id, so a warp'd repair off a drifted integration branch has a reachable passing commit
<!-- logmind-entry-end -->

## 2026-08-24 14:33 - check-derived-docs v15: accept either merge-base state and compare content by blob id, so a warp'd repair off a drifted integration branch has a reachable passing commit

**Reasoning:** v14 accepted only ONE state, so a branch forked before the default branch's last regen had no commit that could pass: warp writes merge-base content, the gate demanded the tip's. Measured on protocol#106 — head blob 0d6e21b vs merge-base df14fab. v15 accepts (A) equal to merge-base with base.ref and (B) equal to merge-base with default_branch, the SPEC 3.3 pin and exactly what warp writes; when base IS default they collapse to one. Content is compared via the compare API's merge_base_commit.sha plus contents?ref blob ids — git content hashes, no workspace and no execution, so SPEC 6.3 still holds. Case 5 goes FAIL to PASS and the real merge was clean.

**Alternatives considered:** Compare against origin/<default> tip. Rejected: the tip is a moving target — green at 10:00, red at 10:05 after an unrelated merge — and a tip-equal branch is precisely what conflicts on the next merge. SPEC 3.3 says merge-base, and pinning there is what makes the gate and warp agree instead of contradict.

**Implications:**
- Two corrections to my own brief, both measured by the lane. warp.go needs NO functional change — the repair at :150 already computes DefaultBranchMergeBase and restores to it; :79 is the separate read-refresh loop, and my earlier 'code uses the tip' read came from line 79 in isolation. And gh pr diff is the forge's three-dot diff, so v14's file list already WAS a merge-base comparison; the defect was the single accepted state and a remedy that named the tip, not the absence of a content check. Marker v14 to v15 with fingerprint and digestRegionA repinned; regions B/D/F unmoved, which is evidence the change is confined to the gate.

---

