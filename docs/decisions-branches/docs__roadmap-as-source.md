← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-14-make-docs-roadmap-md-the-source-of-truth-for-sequencing -->
- **2026-08-14** — Make docs/roadmap.md the source of truth for sequencing
<!-- logmind-entry-end -->

## 2026-08-14 17:02 - Make docs/roadmap.md the source of truth for sequencing

**Reasoning:** The roadmap lived in a scratchpad HTML file that the artifact was published from. That file was wiped twice today by session restarts, taking the only copy of the sequencing with it — the published page survived but could not be updated, so it drifted to showing 26 issues and a superseded critical path while the real count was 31 and the critical path had changed to #288. A roadmap that cannot survive a restart is not a source of truth. It now lives in the repository, versioned and reviewable, and the artifact renders FROM it rather than being it.

**Alternatives considered:** Keep the roadmap inside docs/plan.md. Rejected on half-life: plan.md is architecture and changes yearly; the release board changed six times in one day. Fusing them is exactly why plan.md spent weeks describing a derived_docs.mode config gate that had already been deleted.

**Implications:**
- One owner per fact, stated in both files: if a date, count, issue number or ordering appears in both, roadmap.md is right and plan.md is stale. plan.md is retitled from 'Plan & Architecture' to 'Architecture' to stop implying it owns sequencing. Every number in roadmap.md carries the command that produced it, and every claimed zero carries a control proving the probe finds a non-zero when one exists — including the load-bearing one that regen bot commits have a null author.login, so a login-keyed probe reports zero on every PR and looks clean. All seven SPEC citations verified LIVE against blob cd64e5c; check-links passes.

---

