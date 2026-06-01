## 2026-06-01 00:12 - v0.6.3: skill bench — per-call token-cost measurement (Stream 6 follow-on)

**Reasoning:** Closes the measure arrow of the SkDD loop. Every SKILL.md load enters the agent's context window verbatim; bench reports exactly what that costs (bytes + est_tokens + status bucket + section breakdown + trim suggestions). Pairs with clud-bug usage --health (the enforcement read) for cost-vs-earning visibility per skill.

**Alternatives considered:** Pipe SKILL.md through tiktoken for an exact token count (rejected: adds heavy dep for marginal precision gain — bytes/4 is good enough for the bucket-status decision; precision matters only for finance-grade reporting which isn't this command's job), Bench the loading-by-clud-bug end-to-end (rejected: that's what clud-bug usage --health measures via real review runs; this command is the author-side equivalent that runs offline)

**Implications:**
- Section-breakdown parser splits on level-2 (##) headers; level-3+ stays bundled. Sufficient for most SKILL.md structures
- Suggestions are heuristic, not rules — designed to start the human conversation about what to trim, not auto-edit the file
- Thresholds (2KB/6KB/8KB) match the existing soft cap from v0.6.0's check_size_cap so behavior is consistent across logmind skill commands

---
