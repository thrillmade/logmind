## 2026-05-30 14:30 - feat(v0.6.1): deterministic auto-rebase on timeline.md gap (opt-in)

**Reasoning:** User-coined direction (2026-05-30): 'auto rebase must must be very deterministic and safe, only the timeline md file'. Closes the tokenomics-Phase-D pain by automating the rebase + regen + force-with-lease push when conditions hold. Saves ~5-8 agent turns per DIRTY incident. 6 hard safety gates: opt-in via config (default OFF), not default branch, fetch succeeds, upstream ref exists, branch behind, gap is EXACTLY {docs/timeline.md} and nothing else. Always --force-with-lease (never --force, tested explicitly). Abort safely on any unexpected conflict.

**Alternatives considered:** default-on with --no-auto-rebase opt-out — rejected, force-push is destructive enough that opt-in is the correct posture, widen scope to file-structure.md too in v0.6.1 — rejected, narrow scope is the safety lever; widen only after observed behavior justifies, ship as v0.5.14 — rejected, v0.5.14 < v0.6.0 numerically would confuse semver; v0.6.1 is the right slot

**Implications:**
- Pairs with v0.5.13's doctor warning + logmind rebase manual command. Same detection logic, automated under opt-in.
- Future: v0.6.2+ may widen the allowed-gap set (file-structure.md) once v0.6.1 is observed safe in production. Don't widen until evidence justifies.
- 8 new tests in test_auto_rebase.py. Most load-bearing: test_uses_force_with_lease_not_force — explicit guard that the push NEVER uses bare --force.

---
