← back to [docs/timeline.md](../timeline.md)

## 2026-07-04 00:53 - repomap ranking + --map-tokens budget packing (token-killer R3)

**Reasoning:** With a token budget the map can't carry every file, so it must carry the MOST IMPORTANT first. Deterministic ranking (the caching invariant): decision-linked files (logmind-native — the file's path is named in a decision doc/timeline) rank above unlinked; then intra-repo import fan-in (centrality — the repomap's core signal, Aider's insight); then path. --map-tokens N greedily keeps whole files within an est. ceil(len/4) budget, appends a §14.4 truncation marker for the rest, and honors §14.5 never-worse (packed is a subset). Default (no budget) stays byte-stable (0 = Generate).

**Alternatives considered:** Iterated PageRank (deferred — first-order in-degree fan-in is a cheap, deterministic proxy; iteration is a later refinement). Symbol-granularity packing (deferred — file-granularity is cleaner for v1). A git-recency signal (rejected — non-deterministic across shallow clones, would break the caching invariant).

**Implications:**
- FileSymbols gains Imports (captured during the existing parse); new rank.go (Rank/Pack/importFanIn/decisionLinkedPaths/moduleImportPath); GenerateBudget + RenderWithOmitted + a shared fileBlock; --map-tokens flag; the repomap receipt gains an omitted count. The §3.26 SPEC --map-tokens note bundles with the R4 multi-language bump. Context (R2) still folds the FULL map; a context budget cap is a later refinement.

---

