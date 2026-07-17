← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-fix-cli-stderr-tty-gate-bounded-interactive-nudge-spec-order -->
- **2026-07-17** — fix(cli): stderr TTY gate + bounded interactive nudge + spec-ordered receipt + TZ-stable pulse (#228 #220 #222 #206)
<!-- logmind-entry-end -->

## 2026-07-17 14:06 - fix(cli): stderr TTY gate + bounded interactive nudge + spec-ordered receipt + TZ-stable pulse (#228 #220 #222 #206)

**Reasoning:** four SPEC conformance + robustness fixes in internal/cli: the TTY gate now checks isatty(stderr) per SPEC 3.1.1 not stdin (#220); the interactive branch-summary nudge stdin reads are bounded by a 10s deadline under the 15s lock timeout so a paused human no longer makes a concurrent logmind log fail with appears-stuck (#228); the nudge prompt moved to stderr with its stdout receipt deferred after required line 3 so the 3-line stdout contract holds (#206); the spec pulse compares at calendar-day granularity so its verdict no longer flips by machine timezone (#222)

**Alternatives considered:** redesign the lock scope for the nudge (rejected: bounding the read is smaller and keeps the same-commit edit coupling); keep isatty(stdin) (rejected: SPEC mandates stderr); instant-level TZ localization (rejected: zoneless headers have no recoverable instant cross-machine, so day-granularity bounds the error to 1 day)

**Implications:**
- the human-TTY interactive path may now emit a 4th stdout line (the summary-updated receipt) strictly after the 3 required lines per 3.1.1; the non-interactive/agent path stays byte-exact 3 lines

---

