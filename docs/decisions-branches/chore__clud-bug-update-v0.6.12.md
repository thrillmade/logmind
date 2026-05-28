## 2026-05-28 13:15 - chore: clud-bug v0.6.7 → v0.6.12 (Sonnet pin, incremental-diff, --max-turns, secho lessons, self-update YAML fix)

**Reasoning:** Final clud-bug propagation to this consuming repo. Brings clud-bug-review workflow to v0.6.12 with the remaining Phase A wins not yet in v0.6.7: --max-turns 15 + MAX_THINKING_TOKENS=8000 (v0.6.8), Sonnet 4.6 model pin (v0.6.11), incremental-diff handshake via last-reviewed-sha marker (v0.6.10), and the self-update.yml.tmpl YAML literal-block fix (v0.6.12).

**Implications:**
- After merge, every logmind PR's clud-bug-review runs on Sonnet with caching + incremental-diff. ~80% per-token reduction vs Opus baseline. The self-update workflow_dispatch will work going forward for the next cap-raise + cross-repo propagation cycle.

---
