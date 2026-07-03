← back to [docs/timeline.md](../timeline.md)

## 2026-07-03 11:25 - Route the main-canonical non-TTY headline nudge to stderr (§3.1.1 conformance)

**Reasoning:** nudgeBranchSummary wrote its non-TTY advisory to stdout, appending a 4th line after the three required logmind log lines under main-canonical — violating the §3.1.1 contract that non-TTY stdout is EXACTLY the three lines, byte-identical to the §6.6 fixtures. The adversarial red-team of the SPEC 0.8.0 pass caught it; latent today (main-canonical isn't the default, no fixture covers it) but it blocks the 0.8.0 conformance claim.

**Alternatives considered:** Keep it on stdout and drop the byte-parity claim for main-canonical non-TTY (forfeits the core guarantee)

**Implications:**
- Non-TTY nudge now writes to stderr; the interactive TTY path is unchanged (§3.1.1 permits TTY-on-stdout). The test now splits stdout/stderr (the old one merged them, so it couldn't catch contamination) and asserts the nudge is stderr-only + absent from stdout. Pairs with SPEC 0.8.0 Edit 14.

---

