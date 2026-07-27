← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-26-implement-spec-1-3-2-decisions-rotation-and-restore-derived- -->
- **2026-07-26** — Implement SPEC 1.3.2 decisions rotation, and restore derived docs to the merge-base instead of HEAD
<!-- logmind-entry-end -->

## 2026-07-26 22:26 - Implement SPEC 1.3.2 decisions rotation, and restore derived docs to the merge-base instead of HEAD

**Reasoning:** Two SPEC MUSTs the shipping binary did not satisfy. 1.3.2 requires the oldest entry be moved verbatim to the archive at the cap and logmind never archived at all, so decisions.md grew unbounded against the SPEC, the file own header, and the README promise. Separately the derived-doc restore targeted HEAD, but the invariant is defined against the merge-base, so on an already-diverged branch the restore re-applied the divergence and the gate own repair advice silently no-opped

**Alternatives considered:** Rotate branch decision files too (rejected: SPEC 1.4 states branch files have no capacity cap, they are durable detail pages), Keep HEAD and only correct the error message (rejected: the merge-base IS the invariant definition, so restoring to it is the more correct implementation and it self-repairs)

**Implications:**
- Byte-exactness is structural — consecutive raw entry slices abut in the original file, so a contiguous subset reproduces the exact bytes and no entry is re-rendered. The build agent found unprompted that the pre-commit hook body hardcoded checkout HEAD in shell, which would have silently undone the Go-side fix in any repo with hooks installed; fixed there too. Archive template prose corrected to append-on-overflow per SPEC 1.5

---

