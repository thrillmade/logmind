## 2026-05-30 09:10 - chore: propagate clud-bug v0.6.22 → v0.6.27 to logmind (Smart Budget self-upgrade)

**Reasoning:** logmind's own .github/workflows/clud-bug-review.yml was at template v10 (clud-bug v0.6.22-era), missing Smart Budget Phases 1/2a/3: Layer 1 line-based estimator + Layer 1.5 calibration outputs (turns_estimated/files/+A/-D/threads) + Layer 2 in-prompt budget awareness + Layer 3 mid-review check-in + Layer 6 fallback render-from-inlines + 0.0.W² widened skip allowlist + concurrency-group cancel-in-progress. All 4 consumers (agent-skills/reporulez/rezgen/tokenomics) already on v0.6.27. User directive (2026-05-30): 'i do want clud bug propagated so bugs are better caught more efficiently — efficiency gainer.'

**Alternatives considered:** wait until next quality batch — rejected, calibration data accumulates faster with logmind itself feeding markers, manual workflow file edit — rejected, structural-fix path; idempotent clud-bug update is what consumers run

**Implications:**
- logmind's PR reviews now emit Layer 1.5 calibration markers — feeds the same 30-day window used to gate clud-bug v0.6.28 (L5 auto-retry). One more data point per logmind PR.
- 0.0.W² auto-skip widens to the allowlist (AGENTS.md, .cursorrules, .clud-bug.json, derived docs) — future workflow-only or doc-only propagation PRs to logmind itself skip the LLM review and save tokens.
- Concurrency-group cancel-in-progress means duplicate triggers (e.g., synchronize on rapid pushes) no longer pile up two concurrent reviews — matches the v0.6.25 fix that hit tokenomics #21.
- This PR may trigger the App-side workflow-self-modification guard (claude-code-action refuses on PRs that modify its own workflow file) — admin-bypass once expected per the documented per-PR-checklist structural exception. Next workflow-only PR in logmind will auto-skip via 0.0.W².

---
