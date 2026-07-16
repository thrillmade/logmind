<!-- logmind-entry-start: 2026-05-30-bench-stream-1-path-c-calibration-aggregate-py -->
- **2026-05-30** — bench: Stream 1 Path C — calibration_aggregate.py
<!-- logmind-entry-end -->

## 2026-05-30 11:08 - bench: Stream 1 Path C — calibration_aggregate.py

**Reasoning:** Productionize the inline script that aggregated 26 Layer 1.5 calibration markers across the org (logmind=3, clud-bug=6, tokenomics=2, agent-skills=15, reporulez=0, rezgen=0). Path C of Stream 1 (calibration acceleration) per the token-cost-compression plan. Lives in bench/scripts/ alongside future Path A (historical scrape) + Path B (synthetic stress sandbox). Read-only, runnable today against whatever data has accumulated — no 30-day window wait. User directive (2026-05-30): 'why do we need to wait so long? is there any benefit? other ways to test?'

**Alternatives considered:** ship as a real CLI command (logmind bench-calibration) — rejected, this is QA tooling not a user-facing surface; lives in bench/ same as the 4-angle scripts, PyPI-ship bench/ — captured as deferred feature in plan; not now, REST API integration test mocking gh subprocess — rejected for v1; regex test exercises the load-bearing parsing logic without I/O

**Implications:**
- 26 markers already accumulated organically + N grows by ~3/day at current PR cadence. Tokenomics velocity boost (Stream 3 user-side) amplifies further.
- Distribution stats (cap/est ratio p50=1.24, p90=1.36) confirm Layer 1's 1.2x safety margin is being applied consistently. To validate actual_used / estimated, Path A (action log scrape) is the next piece — captures tool_use count from completed runs.
- Marker regex is locked to v0.6.25 format. Any clud-bug release that changes the marker shape needs a corresponding bench/scripts/calibration_aggregate.py bump + the regex test surfaces the change loudly.

---
