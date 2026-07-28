← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-28-make-the-derived-docs-zero-conflict-invariant-unconditional- -->
- **2026-07-28** — Make the derived-docs zero-conflict invariant unconditional; delete the derived_docs.mode/min_binary opt-in entirely
<!-- logmind-entry-end -->

## 2026-07-28 01:18 - Make the derived-docs zero-conflict invariant unconditional; delete the derived_docs.mode/min_binary opt-in entirely

**Reasoning:** The opt-in gate (v2.0.0 B6) shipped to fix a HOLD finding where L0/L1 applied unconditionally with no per-repo signal, silently breaking driver-mode repos. But opt-in also means most repos get NO protection by default, and the CI gate itself carried a base-ref-read adoption check whose only reason to exist was that gate. Once the invariant is the whole point of v2.0.0, gating it behind a config key every consumer repo has to remember to set is the wrong default — it should just always be true, the same way v2.0.0's other structural guarantees (main-only regen, blocking CI) aren't configurable either.

**Alternatives considered:** Keep the config key but flip its default to integration-point (opt-out instead of opt-in). Rejected: that still leaves a key to delete/ignore, a driver escape hatch nobody should use, and doctor/CI logic that has to keep reading and validating it — strictly more surface than deleting it outright, for no real benefit once the invariant is meant to be universal.

**Implications:**
- Every repo running this binary now gets the four restore/regen layers (L0 hooks, L1 logmind log, L2a pre-commit hook, L2b Claude Code harness guard) and the CI blocking gate unconditionally, with no way to opt out from within a repo. warp's merge-base repair step loses its gate too, as a direct consequence of deleting the resolver it depended on — every non-default-branch warp call now also repairs and stages the two derived docs, not just read-refreshes them, which is a real behavior change beyond the four named layers (documented in guard_commit_test.go/log_test.go/warp_test.go's rewritten fixtures). The regen-timeline workflow + template pair bump v9->v10 and lose the base-ref adoption read entirely; their two 'stale on main' warnings were also rewritten (coordinator-added scope) to stop promising self-heal on a GH013 ruleset-bypass rejection, which is a permanent policy refusal, not a one-cycle blip.

---

