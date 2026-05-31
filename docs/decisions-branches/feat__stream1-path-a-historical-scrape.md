## 2026-05-31 11:20 - feat: Stream 1 Path A — historical clud-bug-review scrape + Layer 1 formula in Python

**Reasoning:** User-approved (2026-05-31). Goal: get a magnitude check on the v0.6.25 Layer 1 estimator NOW by scraping historical clud-bug-review runs across all 6 thrillmade repos. Result: 138 runs scraped, p50 actual/predicted = 0.60, p90 = 1.22, p99 = 1.66. ZERO cap-hits across 138 PRs. Estimator is sound + ~30% over-conservative at p50 — L5 auto-retry can ship with high confidence in v0.6.29, and v0.6.30+ could tighten the 1.2x safety margin without risking cap-hits.

**Alternatives considered:** use gh run view --log only — rejected, returns empty for newer runs (silent reliability bug); switched to gh api .../logs ZIP extraction, parse num_turns directly from Anthropic API result — rejected, no clean signal in current claude-code-action log format; tool_use count is reliable lower-bound proxy, do Path B sandbox first — rejected, Path A gives broader historical coverage faster (~3 hours vs ~1 day)

**Implications:**
- Layer 1 formula validated against 3 real Layer 1.5 markers (logmind PRs #86, #87, #90) within ±3 turns. Cross-check tests in tests/test_calibration_layer1.py catch coefficient drift loudly.
- Per-repo p50: tokenomics=0.32 (most over-conservative, possibly because their PRs are mostly small typos/docs). logmind=0.93 (formula tightest fit). Suggests formula is sound across repo diversity.
- 138 records cached at bench/scripts/.cache/ (gitignored). Re-runs are fast after first scrape (~1 min vs ~10 min cold). JSON output at /tmp/scrape_full.json for downstream analysis.
- Direct input to clud-bug v0.6.29 design: L5 auto-retry's 2x retry multiplier is appropriate; could ship with high confidence + add telemetry to confirm in production rather than gate on more calibration data.

---
