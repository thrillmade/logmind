← back to [docs/timeline.md](../timeline.md)

## 2026-06-29 19:43 - Slice 2 PR6: confirm + guard the main-canonical roll-up (no new mechanism, no push-to-default)

**Reasoning:** Sixth of 7 PRs. The roll-up needs NO new mechanism: the post-merge hook's existing 'logmind timeline --write' dispatches on config (PR3), so a main-canonical repo rebuilds its union on every local merge with no hook-body change and no version bump. The advisory regen-timeline workflow (PR #159) is the server reconciler. This PR documents that and adds an intent guard.

**Alternatives considered:** Add an on-push-to-default workflow that commits the regenerated timeline to main — rejected: it reintroduces the GITHUB_TOKEN-stranding + self-trigger loop the advisory model exists to avoid. Local post-merge hook + server advisory workflow suffice.

**Implications:**
- Doc note on BuildPostMergeBody (roll-up = local reconciler, inherits config dispatch, branch files KEPT, no push-to-default). TestPostMergeBody_RollupInvariants pins regen-present + no-push as INTENT (survives a golden regen). No behavior change — hook body byte-identical, golden unchanged.

---

