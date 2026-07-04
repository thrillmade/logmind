← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-04-v2-0-0-breaking-remove-branch-divergent-entirely-main-canoni -->
- **2026-07-04** — v2.0.0 BREAKING: remove branch-divergent entirely — main-canonical is the sole timeline model
<!-- logmind-entry-end -->

## 2026-07-04 09:54 - v2.0.0 BREAKING: remove branch-divergent entirely — main-canonical is the sole timeline model

**Reasoning:** branch-divergent's only purpose was byte-parity with the RETIRED Python v0.1.x floor; keeping it as a dual-mode toggle was obsolete baggage. CEO decided to REMOVE (not deprecate) it. main-canonical — the conflict-free source-derived entry-block union — becomes the SOLE unconditional timeline. Removed: the timeline.canonical config key, the branch-divergent renderer, the GenerateFor dispatch, every mode gate, and the branch-divergent goldens/tests. headline / per-log marker / doctor --fix backfill are now always active (gated only on isBranchFile). Legacy configs carrying timeline.canonical are ignored gracefully. Dual-reviewed CLEAN (main-canonical output byte-identical, legacy configs graceful, no test weakened, merge-driver union verified).

**Alternatives considered:** Deprecate branch-divergent (§8.7 keep-as-opt-out, staged removal) — rejected by the CEO: it's obsolete Python-floor baggage, cleaner as a single mode. Keep the renderer code unexposed — rejected in favor of full removal.

**Implications:**
- One timeline model, no config toggle, no Python ghosts. logmind Version 2.0.0-dev, SpecVersion 1.0.0. --full is inert (backward-compat). Coordinated with the SPEC 1.0.0 removal (major bump, removes branch-divergent from the spec). Release v2.0.0 on CEO go.

---

