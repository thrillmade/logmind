// filelock.go — cross-process advisory locking for `logmind log`'s
// read-modify-write-commit sequence against docs/decisions*.md.
//
// Concurrent `logmind log` invocations previously raced on the SAME
// decisions file with zero synchronization: two processes could both
// read the pre-write content, both append their own entry in memory,
// and then both call writeAtomic — last rename wins, silently
// dropping the other process's decision (and, if a commit follows,
// producing a commit whose diff doesn't match its own message). This
// file adds acquireRepoLock, which runLog (log.go) takes before the
// read and holds through the write + `git add`/`git commit`, so the
// whole sequence is serialized per repo.
//
// Lock scope: ONE lock per repo (.logmind/.lock), not one per target
// decisions file. Two reasons that was the right call here rather
// than the finer-grained <target>.lock:
//
//  1. .logmind/.lock is ALREADY a reserved, gitignored path — see the
//     "# logmind" block in .gitignore and the defaultLines in
//     init.go's ensureGitignoreBlock (`.logmind/cache/`,
//     `.logmind/.lock`), both of which predate this fix. Reusing it
//     means every already-initialized repo is protected the moment
//     it upgrades its logmind binary, with no .gitignore migration
//     and no risk of a lock file getting swept into `git add -A` —
//     ensureGitignoreBlock is idempotent-once, so it will NOT
//     retrofit a new ignore line (e.g. `docs/*.lock`) into a repo
//     whose .gitignore block was written before this fix shipped.
//  2. `logmind log` bursts targeting genuinely DIFFERENT decision
//     files (two unrelated feature branches logging "at once") are a
//     rare edge case for a single-operator CLI tool, and each
//     critical section here is short (read one small markdown file,
//     append a string, rename, optionally `git add -A && git
//     commit`). The correctness win of a per-file lock isn't worth
//     the extra path-hashing/lock-file-placement complexity.
//
// The two acquireRepoLock implementations (filelock_unix.go using
// syscall.Flock, filelock_windows.go using a share-mode-exclusive
// CreateFile) share the constants and the fileLock type below. Both
// are released by the kernel when the holding process dies, so neither
// needs stale-lock recovery.
package cli

import (
	"path/filepath"
	"time"
)

// lockAcquireTimeout bounds how long runLog waits for the repo lock
// before giving up. A clear failure beats both an unbounded hang and
// (worse) silently proceeding unlocked — which is exactly the bug
// this fixes.
const lockAcquireTimeout = 15 * time.Second

// lockPollInterval is the sleep between non-blocking acquire retries.
const lockPollInterval = 25 * time.Millisecond

// fileLock is a held advisory lock. Unlock releases it; safe to call
// on a nil *fileLock (no-op) so callers can use it defensively.
type fileLock struct {
	release func() error
}

func (l *fileLock) Unlock() error {
	if l == nil || l.release == nil {
		return nil
	}
	return l.release()
}

// repoLockPath returns the path this repo's `logmind log` invocations
// serialize on: <cwd>/.logmind/.lock.
func repoLockPath(cwd string) string {
	return filepath.Join(cwd, ".logmind", ".lock")
}
