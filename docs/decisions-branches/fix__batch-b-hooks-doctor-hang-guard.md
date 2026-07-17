← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-hang-guard-the-git-hook-logmind-calls-add-a-shell-exec-guard -->
- **2026-07-17** — Hang-guard the git-hook logmind calls, add a shell-exec guard test, and fix doctor's --version regex (#213 #224 #214)
<!-- logmind-entry-end -->

## 2026-07-17 14:08 - Hang-guard the git-hook logmind calls, add a shell-exec guard test, and fix doctor's --version regex (#213 #224 #214)

**Reasoning:** The pre-commit and commit-msg hook bodies shelled out to PATH-resolved logmind with no timeout, so a wedged binary stalled every git commit forever. timeout(1) is not on macOS by default, so both bodies now background the logmind call under a POSIX sleep-then-kill watchdog, wait for the real exit code, and fail open (exit 0) on the deadline. Separately, doctor's version regex expected the literal word version from Click's legacy Python output and never matched a real Go binary line, so an on-PATH logmind was always mis-classified markerless; and the harness guard command was only string-compared, never run through a real shell.

**Alternatives considered:** Only document the hang exposure (rejected: a hung commit is a silent productivity sink a doc note cannot prevent), Bump hookVersion to force drift detection (rejected: hookVersion returns version.Version, which is out of scope and changes logmind --version; decoupling it would break doctor.probeHook, which compares the installed marker against version.Version and would then mislabel a freshly installed hook as stale; doctor's content-drift fast-path already flags the changed body at the same version), Use timeout(1) to bound the hook (rejected: not installed on macOS by default; a background plus watchdog is portable POSIX sh)

**Implications:**
- The watchdog subshell redirects its fds to /dev/null so its possibly-orphaned sleep child cannot hold the caller's captured output pipe open, preventing a 10-second post-commit stall for any tool that captures git output
- The deny exit codes are preserved only on the normal path: commit-msg maps guard-commit's 65 to exit 1, and a timeout or crash yields a signal exit above 128 that falls through to fail-open
- Fixing the doctor regex makes it detect a genuinely stale on-PATH logmind, so several non-hermetic doctor tests were made hermetic by isolating PATH; commit-msg.golden was regenerated and hookVersion/version.Version were left unchanged

---

## 2026-07-17 14:29 - fix(doctor): version regex also accepts the legacy Click shape — stale Python binary stays classified (#214 review)

**Reasoning:** the dual-review caught that the re-anchored #214 regex no longer matched Click's comma-form (logmind, version X), so a stale on-PATH Python binary degraded to markerless — blinding the drift row to the exact stale binary it exists to catch; widened to accept both the Go and Click shapes

**Alternatives considered:** keep the Go-only anchor (rejected: silently drops detection of stale Python binaries, a real regression); a separate Click-only branch (rejected: one alternation covers both)

**Implications:**
- a stale Python logmind on PATH classifies stale/DRIFT again; added TestProbePathResolution_LegacyClickVersionClassified

---

