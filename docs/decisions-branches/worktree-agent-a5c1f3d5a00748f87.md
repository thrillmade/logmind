← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-serialize-concurrent-decision-writes-and-give-writeatomic-a- -->
- **2026-07-17** — Serialize concurrent decision writes and give writeAtomic a unique temp file per call
<!-- logmind-entry-end -->

## 2026-07-17 09:02 - Serialize concurrent decision writes and give writeAtomic a unique temp file per call

**Reasoning:** Concurrent logmind log invocations raced on writeAtomic's fixed path+.tmp file and had zero synchronization around the read-modify-write-commit sequence, so two processes writing the same decisions file could crash with a rename error and/or silently drop each others decisions while both printed success. Unacceptable data-integrity bug to ship in the v2.0.0 tag.

**Alternatives considered:** Per-target-file lock at docs/decisions.md.lock, finer-grained but requires a new .gitignore pattern that already-initialized repos would not retroactively pick up, A bare blocking syscall.Flock with no timeout, which risks an unbounded hang if a holder wedges

**Implications:**
- writeAtomic (timeline.go) now writes to an os.CreateTemp-generated unique temp file before renaming, closing the shared-tmp race for every caller (log, headline, doctor, file-structure regen), not only logmind log
- runLog now takes a repo-scoped advisory lock at .logmind_lock before its read and holds it through the write and the git add and commit, using syscall.Flock on unix with a bounded 15s retry and an O_CREATE O_EXCL staleness-checked fallback on Windows; acquire failure returns a clear error instead of writing unlocked
- Added internal/cli/log_concurrency_test.go, a subprocess-based regression test spawning 16-20 concurrent logmind log invocations against the same file, verified to fail on the pre-fix tree and pass post-fix under go test -race

---

