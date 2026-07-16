<!-- logmind-entry-start: 2026-06-01-v0-6-6-doctor-surfaces-clud-bug-skill-usage-upload-drift-pos -->
- **2026-06-01** — v0.6.6: doctor surfaces clud-bug skill-usage upload drift (post-v0.6.31 hardening)
<!-- logmind-entry-end -->

## 2026-06-01 11:12 - v0.6.6: doctor surfaces clud-bug skill-usage upload drift (post-v0.6.31 hardening)

**Reasoning:** Today's v0.6.31 hotfix revealed the silent-failure pattern in actions/upload-artifact@v4 (dot-file exclusion). Without a consumer-side preflight, repos can lag on the upload fix indefinitely with no error surface. New doctor check warns when clud-bug-review.yml is missing the v0.6.29 Upload skill-usage step OR the v0.6.31 include-hidden-files flag. Anchored regex matches the v0.6.32 clud-bug release-discipline guard's pattern — the two gates stay aligned.

**Alternatives considered:** Auto-run npx clud-bug update when drift detected (rejected: violates 'humans gate' constraint + breaks the doctor's read-only contract), Make this a hard DRIFT signal instead of a suggestion line (rejected: consumer might be intentionally on an older pin during gradual rollout; predictive heads-up is the right severity)

**Implications:**
- Pairs the v0.6.32 clud-bug-side guard with a logmind-side consumer-side check. Each gate catches the drift on a different surface: discipline guard catches at clud-bug release time, doctor check catches at consumer use time
- Tokenomics specifically: when the user runs tokenomics work soon, a tokenomics-side logmind doctor invocation will confirm the v0.6.31 upload fix is in place before any review fires

---
