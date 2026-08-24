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

## 2026-08-24 15:55 - check-derived-docs v15: check rule B's conflict-freedom instead of asserting it, and print a remedy that actually runs

**Reasoning:** The panel found the house defect inside the fix. Rule A is unconditionally conflict-free — the head side is unchanged, so git takes the base's side. Rule B CHANGES the head side, so it is conflict-free only if the base TIP blob still matches its merge-base blob, which the gate never read. Measured: dev drifts, a PR warps green, dev drifts again, gate still exits 0 saying 'the merge cannot conflict on them', and git merge conflicts — with no re-trigger, because neither merge-base moves. The gate now reads base_tip_blob and accepts rule B only when it equals base_blob or head_blob. Row 5, the entire reason v15 exists, still PASSES.

**Alternatives considered:** Require branches up to date before merging and drop rule B. Rejected: that is a repo setting we cannot ship in a template, and it forces a rebase on every unrelated push to the integration branch — the friction v2.0.0 exists to remove. Also rejected: comparing merge RESULTS rather than blobs, which would need a workspace and break SPEC 6.3's checkout-free requirement.

**Implications:**
- Stated limits, not claims. The base branch moving AFTER the last check run is not closed — pull_request_target fires on head changes only, so the window shrinks from forever to since-the-last-run; only 'require branches up to date' closes it. F1b's rule incoherence is unchanged: a branch that merged main in makes pin_mb main's tip, so rule B accepts tip content while the message forbids restoring to the tip — measured conflict-correct but incoherent, and it sits with #362/#363. Errors now lean false-RED: two sides editing different hunks that git would auto-merge are reported raced, because the gate compares blobs not merge results. Marker stays v15 rather than bumping to v16 — origin/dev carries v14, so v15 was never shipped; digestRegionA repinned, B/D/F unmoved. The PR body's G3 row claim was wrong: the true set against shipped v15 is {2,3}, not {2,3,4,7}.

---

