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

## 2026-07-17 14:59 - fix(cli): single shared stdin reader, one nudge deadline, stdin-readable self-heal gate

**Reasoning:** Adversarial dual-review of PR #234 found three lock/stdin bugs in the interactive nudge plus self-heal path. Finding 1: the two sequential nudge reads were each bounded at the full timeout, so the two-read path could hold the repo lock about 20s and exceed the 15s lockAcquireTimeout, reviving the appears-stuck failure for a concurrent log; fixed by sharing ONE nudgeBudget deadline of 12s across both reads. Finding 2: a timed-out nudge left a goroutine parked on its own reader over stdin, so the later self-heal opened a second reader and the parked one stole the humans first answer; fixed with a single shared stdinLines reader draining one goroutine to one channel, reused by both nudge and self-heal. Finding 3: after #220 moved the interactivity gate to isatty stderr, the unbounded self-heal read hung when stderr was a TTY but stdin an open-but-never-delivering pipe; fixed by only entering the block-and-wait when stdin is readable (terminal or regular file), else falling through to the non-interactive stderr advisory and exit 0. Output routing stays on isatty stderr per section 3.1.1.

**Alternatives considered:** Bound the self-heal read with a short timeout (rejected: regresses the legitimate human-at-TTY case where fixing docs can take arbitrarily long), Gate self-heal on isatty stdin unconditionally (rejected: breaks scripted file-redirected input and the in-process test readers)

**Implications:**
- nudgeBudget must stay under lockAcquireTimeout; a static test asserts this invariant
- Pipe/socket/fifo stdin no longer enters the interactive self-heal wait; it gets the non-interactive advisory and exit 0
- Byte-exact 3-line non-TTY stdout contract (SPEC 3.1) unchanged; no golden moved

---

