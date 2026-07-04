← back to [docs/timeline.md](../timeline.md)

## 2026-07-03 23:03 - token-killer Phase 1b: LOGMIND_QUIET output discipline across log/timeline/file-structure/doctor/headline

**Reasoning:** Agents pay tokens to read and skip multi-line ✓ progress chatter; a single chainable 'ok <k=v>' receipt per verb (borrowed from clud-bug's CLUD_BUG_QUIET) is far cheaper and still machine-parseable

**Implications:**
- New opt-in quiet MODE only (--quiet flag OR LOGMIND_QUIET env); default output stays byte-identical so timeline/tree/cli goldens pass unchanged; errors + hints always route to stderr under quiet, never suppressed

---

