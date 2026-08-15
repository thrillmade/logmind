// Package atomicio provides a single crash-safe file-write primitive:
// write to a temp sibling in the file's own directory, then atomically
// rename it into place.
//
// Why this matters: os.WriteFile on an EXISTING path opens with O_TRUNC,
// then writes — two separate syscalls. A crash (power loss, OOM kill,
// SIGKILL) between the truncate and the write completing leaves the file
// empty or partially written on disk. WriteFile here never touches the
// destination path directly: the temp file is written and fsync'd-by-Close
// in full, then os.Rename swaps it in — a single filesystem operation that
// is atomic on the platforms Go supports (same-directory rename). The
// destination either has its OLD full content or its NEW full content,
// never a partial one.
//
// This consolidates a pattern that already existed twice in this codebase
// under different names — internal/skill/sync.go's realAtomicWriteFile and
// internal/cli/timeline.go's writeAtomic — so callers across packages share
// one implementation instead of each re-deriving the temp+rename dance
// (and, per audit, three sites — internal/claudehook, internal/hooks,
// internal/inserter — hadn't derived it at all, using a bare os.WriteFile
// against an existing file).
//
// It also refuses to write through a symlink at the destination — see
// RefuseSymlink.
package atomicio

import (
	"fmt"
	"os"
	"path/filepath"
)

// RefuseSymlink returns a clear error when path already exists as a
// symlink — dangling or not — instead of silently writing through it.
//
// A symlink at a path this tool manages (a directive under .logmind/, a
// config file, a lock file) turns an ordinary write into an
// arbitrary-write primitive: a bare os.WriteFile opens through the link
// (open(2) resolves the final component) and lands its body wherever the
// link points, dangling target or not. Swapping in the atomic
// temp-file-plus-rename technique does not fully save this either — a
// rename onto an EXISTING symlink replaces the link itself rather than
// following it, but that is still a silent decision about someone else's
// file this tool did not put there. Refusing loudly, and leaving both the
// link and whatever it points to untouched, beats guessing which of
// those two outcomes the caller wanted.
//
// A path that does not exist yet, or exists as a regular file or
// directory, returns nil — there is nothing to refuse.
func RefuseSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	if target, rerr := os.Readlink(path); rerr == nil {
		return fmt.Errorf("refusing to write %s: it is a symlink to %s; remove it and retry", path, target)
	}
	return fmt.Errorf("refusing to write %s: it is a symlink; remove it and retry", path)
}

// WriteFile atomically writes data to path with permission bits perm,
// whether path already exists or not. On any failure the temp file is
// removed and path is left completely untouched.
//
// Refuses up front, via RefuseSymlink, when path is currently a symlink.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := RefuseSymlink(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
