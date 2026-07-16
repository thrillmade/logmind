<!-- logmind-entry-start: 2026-06-01-feat-v0-6-13-4-consumer-product-ux-fixes-issues-112-113-prop -->
- **2026-06-01** — feat(v0.6.13): 4 consumer-product UX fixes (issues #112 + #113 + propagation friction)
<!-- logmind-entry-end -->

## 2026-06-01 22:12 - feat(v0.6.13): 4 consumer-product UX fixes (issues #112 + #113 + propagation friction)

**Reasoning:** Four related fixes accumulated from today's propagation cycle. (a) post-merge hook detects orphan-branch state via @{u} upstream tracking + refs/remotes test; skips regen entirely when branch was just merged-and-deleted — closes issue #112 chronic recurrence regardless of stale local CLI binary. (b) logmind log no longer triggers self-update — new read-only detect_template_drift() function reports drift as a warning; new logmind self-update command applies refresh explicitly — closes issue #113 piggy-back commits + race conditions. (c) self-update template v6 smart workflow-skip: pin-bump-only releases propagate without PAT; workflow-touching releases politely defer until PAT configured. (d) gh pr create uses PAT too — eliminates the 'Allow Actions to create PRs' per-repo setting that blocked 2/5 thrillmade repos during v0.6.12 self-heal validation.

**Alternatives considered:** Ship one fix at a time across 4 versions — slower; the 4 are tightly related, Wait for D.10 orchestrator App to subsume entire PAT-permission class — multi-week vs 1 day

**Implications:**
- Closes the v0.6.x post-merge hook saga that recurred across v0.6.7/v0.6.9/v0.6.10/v0.6.11/v0.6.12 — structural fix is in the hook body itself
- Decouples decision logging from template refresh — agent behavior on tokenomics/etc. matches reporter's preferred design
- External consumers (no PAT, no Allow-Actions setting) now get clean pin-bump auto-propagation; only template-body releases require their attention

---
