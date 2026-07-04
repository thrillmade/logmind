← back to [docs/timeline.md](../timeline.md)

## 2026-07-04 00:53 - repomap ranking + --map-tokens budget packing (token-killer R3)

**Reasoning:** With a token budget the map can't carry every file, so it must carry the MOST IMPORTANT first. Deterministic ranking (the caching invariant): decision-linked files (logmind-native — the file's path is named in a decision doc/timeline) rank above unlinked; then intra-repo import fan-in (centrality — the repomap's core signal, Aider's insight); then path. --map-tokens N greedily keeps whole files within an est. ceil(len/4) budget, appends a §14.4 truncation marker for the rest, and honors §14.5 never-worse (packed is a subset). Default (no budget) stays byte-stable (0 = Generate).

**Alternatives considered:** Iterated PageRank (deferred — first-order in-degree fan-in is a cheap, deterministic proxy; iteration is a later refinement). Symbol-granularity packing (deferred — file-granularity is cleaner for v1). A git-recency signal (rejected — non-deterministic across shallow clones, would break the caching invariant).

**Implications:**
- FileSymbols gains Imports (captured during the existing parse); new rank.go (Rank/Pack/importFanIn/decisionLinkedPaths/moduleImportPath); GenerateBudget + RenderWithOmitted + a shared fileBlock; --map-tokens flag; the repomap receipt gains an omitted count. The §3.26 SPEC --map-tokens note bundles with the R4 multi-language bump. Context (R2) still folds the FULL map; a context budget cap is a later refinement.

---

## 2026-07-04 01:06 - R3 review fixes: §14.5 never-worse passthrough + ranking-signal robustness

**Reasoning:** Dual review (adversarial + clud-bug-lens) found a BLOCKER: the ~47-byte truncation marker can exceed a few small omitted blocks, making the packed render LARGER than the full one (worse AND lossy) — a §14.5 violation. Fixed via passthrough in GenerateBudget (emit the full map when packing fails to shrink it). Plus 2 ranking MINORs: decision-link used strings.Contains so 'a.go' matched inside 'data.go' (false boost); go.mod 'module x // comment' glued the comment onto the module path, zeroing all fan-in.

**Alternatives considered:** Account for the marker size inside Pack (rejected — passthrough IS the §14.5 'on doubt, passthrough' rule, cleaner). Require decision-link paths to contain '/' (rejected — misses root-level files; the boundary-aware match is correct). 

**Implications:**
- Budgeting never enlarges the output (verified empirically: tiny repo passes through at every sub-full budget). Ranking is robust to substring false-positives (new mentionsPath boundary check) and vanity-import go.mod (strings.Fields). Regression tests for all three; the never-worse guarantee doc moved from Pack to GenerateBudget.

---

