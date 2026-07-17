//go:build !windows

// filelock_unix.go — flock-based acquireRepoLock for every GOOS with
// syscall.Flock (Linux, macOS, BSDs, ...). See filelock.go for the
// shared constants/type and the rationale for a repo-scoped rather
// than per-target-file lock.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// acquireRepoLock takes an OS-advisory flock on .logmind/.lock inside
// cwd, serializing this process's `logmind log` read-modify-write-
// commit sequence against every other concurrent `logmind log` in the
// same repo.
//
// flock is released automatically by the kernel when the holding
// process exits for ANY reason — normal exit, panic, SIGKILL — so
// there is no stale-lock cleanup problem to solve here, unlike a
// plain O_EXCL lockfile: a crashed holder can never leave a lock
// nobody can take.
//
// LOCK_EX blocks natively, but a bare blocking call has no timeout —
// if a holder somehow wedges (stopped, deadlocked before release)
// every other `logmind log` would hang forever. We poll with LOCK_NB
// instead so a stuck holder can't wedge callers indefinitely: after
// lockAcquireTimeout of retries this returns a clear error rather
// than hanging, or (worse) proceeding unlocked.
func acquireRepoLock(cwd string) (*fileLock, error) {
	lockPath := repoLockPath(cwd)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(lockPath), err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", lockPath, err)
	}

	deadline := time.Now().Add(lockAcquireTimeout)
	for {
		flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if flockErr == nil {
			return &fileLock{release: func() error {
				defer f.Close()
				return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			}}, nil
		}
		if flockErr != syscall.EWOULDBLOCK && flockErr != syscall.EAGAIN {
			f.Close()
			return nil, fmt.Errorf("flock %s: %w", lockPath, flockErr)
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf(
				"could not acquire lock on %s after %s — another `logmind log` appears stuck; "+
					"refusing to write unlocked to avoid silently dropping a decision",
				lockPath, lockAcquireTimeout)
		}
		time.Sleep(lockPollInterval)
	}
}
