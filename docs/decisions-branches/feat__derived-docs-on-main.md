← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-adopt-derived-docs-on-main-plan-one-invariant-branch-derived -->
- **2026-07-17** — Adopt derived-docs-on-main plan: one invariant (branch derived docs == merge-base) enforced by L0 hooks/L1 log/L3 CI gate, plus warp/context/pulse freshness
<!-- logmind-entry-end -->

## 2026-07-17 20:16 - Adopt derived-docs-on-main plan: one invariant (branch derived docs == merge-base) enforced by L0 hooks/L1 log/L3 CI gate, plus warp/context/pulse freshness

**Reasoning:** The derived docs are pure functions of the decision files, so a branch never needs to edit them; keeping them byte-identical to the merge-base makes git 3-way merge conflict-free with no merge-driver dependency, which is the only thing that works on GitHub servers

**Alternatives considered:** Merge-driver-only auto-resolution (rejected: GitHub cannot run logmind git merge-driver, so PRs still show conflicts), Detect-and-fix conflicts after they occur (rejected: conflicts burn agent tokens; the bar is structurally impossible not merely rare)

**Implications:**
- warp is the only network caller; logmind log hot path stays network-free; the CI check-derived-docs job becomes blocking and existing derived-doc-editing PRs must revert

---

## 2026-07-17 20:26 - Foundation: gitcli MergeBase/Fetch/ShowFile/RestorePathsToHead + shared derivedDocPaths/onNonDefaultBranch

**Reasoning:** Thin git-wrapper helpers and the single definition of the two governed docs plus the non-default-branch predicate, consumed by L1 log, warp, the pulse probe, and context refresh

**Implications:**
- Fetch is the only network helper and is never called from the log hot path; onNonDefaultBranch is conservative-false so non-repo/detached/default paths keep pre-v2.0.0 behavior

---

