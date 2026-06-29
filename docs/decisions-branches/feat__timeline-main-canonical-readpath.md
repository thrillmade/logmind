← back to [docs/timeline.md](../timeline.md)

## 2026-06-29 18:55 - Slice 2 PR3: main-canonical timeline read path (GenerateMainCanonical) + single-point config dispatch

**Reasoning:** Third of 7 PRs. The deterministic-union assembler + ONE dispatch point (GenerateFor) wired into all 5 in-process sites (runTimeline x3 + the two init.go re-renders). Merge driver + hooks shell out so they inherit it. Default stays branch-divergent — existing timeline goldens + a new GenerateFor(canonical=false)==Generate test prove byte-parity.

**Alternatives considered:** A parallel CLI command for the union — rejected; --write and --check MUST use the SAME generator or --check false-wedges a fleet. One dispatch point guarantees consistency.

**Implications:**
- canonical.go assembly: collectMarked (entry-block markers, or a legacy markerless fallback synthesized from the first decision header) + dedupeAndSuffix (same-body collapse; different-body collision -> stable -2/-3 by smallest source path; newest-first, slug desc per §1.6.3.2) + renderCanonical (## YYYY-MM landmarks outside markers). Exported decisions.ListBranchFiles. Determinism proven (shuffled order + 2 checkout paths -> identical bytes). FOR PR7 SPEC BRAINSTORM: the exact synthesized-line format + the folded-and-kept dedup edge are choices to ratify.

---

