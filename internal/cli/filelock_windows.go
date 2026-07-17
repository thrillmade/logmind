//go:build windows

// filelock_windows.go — kernel-released, share-mode-exclusive
// acquireRepoLock for Windows (which has no syscall.Flock). See
// filelock.go for the shared constants/type and the rationale for a
// repo-scoped rather than per-target-file lock.
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// errSharingViolation is Windows ERROR_SHARING_VIOLATION (0x20 = 32),
// returned by CreateFile when another handle holds the file open without
// granting write-sharing. Go's stdlib syscall package does not export a
// named constant for it, so we define it locally to keep the lock
// dependency-free (no golang.org/x/sys).
const errSharingViolation = syscall.Errno(32)

// acquireRepoLock takes an exclusive lock on .logmind/.lock inside cwd,
// serializing this process's `logmind log` read-modify-write-commit
// sequence against every other concurrent `logmind log` in the same repo.
//
// Windows has no syscall.Flock, but a CreateFile that requests write
// access while granting only FILE_SHARE_READ (and NOT FILE_SHARE_WRITE)
// is itself a mandatory lock: the first opener holds it, and every other
// CreateFile that requests write access fails with
// ERROR_SHARING_VIOLATION until the first handle is closed.
//
// Crucially — exactly like unix flock, and UNLIKE a plain O_EXCL
// lockfile — the kernel closes the handle when the holding process exits
// for ANY reason (normal exit, panic, SIGKILL), releasing the lock
// instantly. So there is NO stale-lock problem to solve: no mtime, no
// age threshold, no "steal", and no lock file is ever removed on
// release. That last point matters — because release is a pure
// CloseHandle with no os.Remove, a concurrent waiter can never delete
// (and thereby clobber) another live holder's lock file.
//
// A bare blocking acquire has no timeout, so we poll: retry on
// ERROR_SHARING_VIOLATION every lockPollInterval and, after
// lockAcquireTimeout, return a clear error rather than hanging or
// (worse) proceeding unlocked.
func acquireRepoLock(cwd string) (*fileLock, error) {
	lockPath := repoLockPath(cwd)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(lockPath), err)
	}
	namePtr, err := syscall.UTF16PtrFromString(lockPath)
	if err != nil {
		return nil, fmt.Errorf("lock path %s: %w", lockPath, err)
	}

	deadline := time.Now().Add(lockAcquireTimeout)
	for {
		// GENERIC_WRITE access + a share mask of FILE_SHARE_READ only
		// (no FILE_SHARE_WRITE) means a second concurrent writer gets
		// ERROR_SHARING_VIOLATION. OPEN_ALWAYS creates the file when it
		// does not yet exist and opens it otherwise.
		h, err := syscall.CreateFile(
			namePtr,
			syscall.GENERIC_READ|syscall.GENERIC_WRITE,
			syscall.FILE_SHARE_READ,
			nil,
			syscall.OPEN_ALWAYS,
			syscall.FILE_ATTRIBUTE_NORMAL,
			0,
		)
		if err == nil {
			return &fileLock{release: func() error {
				return syscall.CloseHandle(h)
			}}, nil
		}
		if !errors.Is(err, errSharingViolation) {
			return nil, fmt.Errorf("open lock file %s: %w", lockPath, err)
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
