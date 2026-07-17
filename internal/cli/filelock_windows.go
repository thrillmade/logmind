//go:build windows

// filelock_windows.go — staleness-checked-lockfile acquireRepoLock
// fallback for GOOS without syscall.Flock support. See filelock.go
// for the shared constants/type and the rationale for a repo-scoped
// rather than per-target-file lock.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// acquireRepoLock is the non-flock fallback for Windows. Uses an
// O_CREATE|O_EXCL lockfile: creation is atomic, so exactly one
// concurrent `logmind log` wins the create and every other one sees
// ErrExist and retries.
//
// Unlike flock, a crashed holder can't have its exclusive-create
// lockfile auto-released by the kernel — so this fallback adds a
// staleness check: a lockfile whose mtime is older than
// lockStaleAfter is presumed abandoned (holder died without cleaning
// up) and is stolen. Bounded retry (lockAcquireTimeout) still applies
// on top so a genuinely live-but-slow holder doesn't wedge callers
// forever; on timeout this returns a clear error rather than
// proceeding unlocked.
func acquireRepoLock(cwd string) (*fileLock, error) {
	lockPath := repoLockPath(cwd)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(lockPath), err)
	}

	deadline := time.Now().Add(lockAcquireTimeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return &fileLock{release: func() error {
				return os.Remove(lockPath)
			}}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create lock file %s: %w", lockPath, err)
		}

		if info, statErr := os.Stat(lockPath); statErr == nil {
			if time.Since(info.ModTime()) > lockStaleAfter {
				os.Remove(lockPath) // holder presumed dead; steal the lock
				continue
			}
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf(
				"could not acquire lock on %s after %s — another `logmind log` appears stuck; "+
					"refusing to write unlocked to avoid silently dropping a decision",
				lockPath, lockAcquireTimeout)
		}
		time.Sleep(lockPollInterval)
	}
}
