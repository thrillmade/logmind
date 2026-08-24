// Package atomicio provides this codebase's file-write primitive: write to
// a temp sibling in the destination's own directory, then atomically rename
// it into place.
//
// Why a rename at all: os.WriteFile on an EXISTING path opens with O_TRUNC,
// then writes — two separate syscalls. A crash (power loss, OOM kill,
// SIGKILL) between the truncate and the write completing leaves the file
// empty or partially written on disk. WriteFile here never opens the
// destination path: the temp file is written in full, then os.Rename swaps
// it in — a single filesystem operation that is atomic on the platforms Go
// supports (same-directory rename).
//
// # THE ONE RULE
//
// An atomic replace swaps the destination NAME. It does not write into the
// destination's inode. Every consequence below follows from that one fact,
// and every caller — and every deliberate NON-caller — is justified by it:
//
//  1. MODE. Because the bytes land in a new file, the mode is whatever we
//     give it, so it has to be chosen deliberately. WriteFile reproduces
//     os.WriteFile exactly: perm is the CREATE mode and is subject to the
//     process umask; an existing regular file KEEPS the mode it already has.
//     A caller that means to assert a mode says so, by calling WriteFileMode.
//
//  2. SYMLINKS. A symlink on the destination is refused — not followed, not
//     silently replaced. That refusal is what closes the dangling-symlink
//     arbitrary-write primitive. It also means a DELIBERATELY symlinked
//     target is refused rather than updated through the link; see
//     RefuseSymlink, which spells out both halves and what it does not cover.
//
//  3. IDENTITY. Rename gives the destination a NEW inode. Hardlinks to the
//     old inode are severed and keep the OLD content; an open file
//     descriptor, an flock, or an inode-keyed cache still refers to the old
//     inode and no longer to this path. A caller that needs the inode
//     preserved — appending through a link the user set up on purpose,
//     writing a file something else holds a lock on — must NOT use this
//     package. TestWriteFile_RenameSeversHardlinks pins this so it is a
//     stated contract rather than a surprise.
//
// # DURABILITY
//
// WriteFile fsyncs the temp file before renaming, so the bytes are on disk
// before the name points at them: the destination has either its OLD full
// content or its NEW full content, never a partial one. It does NOT fsync
// the containing directory, so a crash immediately after the rename may lose
// the rename itself and leave the old content in place. That is the entire
// promise — there is no stronger one.
//
// This consolidates a pattern that already existed twice in this codebase
// under different names — internal/skill/sync.go's realAtomicWriteFile and
// internal/cli/timeline.go's writeAtomic — so callers across packages share
// one implementation instead of each re-deriving the temp+rename dance
// (and, per audit, three sites — internal/claudehook, internal/hooks,
// internal/inserter — hadn't derived it at all, using a bare os.WriteFile
// against an existing file).
package atomicio

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
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
// SAY IT PLAINLY: this cuts both ways. A symlink somebody planted is
// refused, and so is a symlink somebody MEANT — point docs/timeline.md at a
// shared file and logmind will decline to write it rather than detach the
// link. The error names the link's target so the caller can write that path
// directly instead. Refusing is the deliberate trade: the tool cannot tell a
// deliberate link from a planted one, and quietly picking either behaviour
// is worse than saying so.
//
// WHAT THIS DOES NOT COVER: only the FINAL path component is checked. If an
// ancestor DIRECTORY is a symlink (`ln -s /elsewhere repo/.claude`), every
// path beneath it resolves through that link and the write lands wherever it
// points — this function returns nil and WriteFile proceeds. That is
// deliberate: symlinked ancestors are ordinary (a repo under a symlinked
// ~/code, /tmp -> /private/tmp on macOS), refusing them would break normal
// setups, and the only real fix is a beneath-the-root resolve (openat2
// RESOLVE_BENEATH) that is Linux-only and belongs at the layer that knows
// the repo root, not here. Do not read a nil return as "this path is inside
// the repository".
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
		return fmt.Errorf("refusing to write %s: it is a symlink to %s. "+
			"Writes here replace the file by rename, which would detach the link instead of "+
			"updating what it points at — if the link is deliberate, run logmind against %s "+
			"(or write that path directly); if it is not, remove the link and retry",
			path, target, target)
	}
	return fmt.Errorf("refusing to write %s: it is a symlink. "+
		"Writes here replace the file by rename, which would detach the link instead of "+
		"updating what it points at — remove the link and retry, or write the file it names directly",
		path)
}

// WriteFile atomically writes data to path, whether path already exists or
// not. On any failure the temp file is removed and path is left completely
// untouched.
//
// MODE — os.WriteFile's rule, reproduced: perm applies only when this call
// CREATES the file, and the kernel applies the process umask to it there. An
// existing regular file keeps the mode it already has, and perm is ignored.
// Converting an os.WriteFile call site to this one therefore does not change
// what the user sees in `ls -l`. Call WriteFileMode when the mode is the
// point.
//
// SYMLINKS — refused up front, via RefuseSymlink, when path is currently a
// symlink. Read that doc comment: it refuses deliberate links too, and it
// only inspects the final path component.
//
// IDENTITY — the rename gives path a NEW inode. Hardlink twins are severed
// and keep the old content; open descriptors and advisory locks on the old
// inode stop referring to this path. If that matters at your call site, this
// is the wrong primitive.
//
// DURABILITY — the temp file is fsynced before the rename; the containing
// directory is not. See the package doc.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, false)
}

// WriteFileMode is WriteFile with the mode ASSERTED rather than inherited:
// the file ends up at mode, whether it already existed or not, and the umask
// does not apply. It is the atomic equivalent of os.WriteFile followed by an
// unconditional os.Chmod.
//
// Use it only where the mode is part of what the call site is for — a git
// hook that must stay executable, a config the tool keeps at 0600, a copy
// that must reproduce its source's bits. Everywhere else use WriteFile, so
// that routing a write through this package never silently re-permissions a
// file the user owns.
//
// Every other clause of the one rule is unchanged: symlinks are refused, and
// the rename still severs hardlinks and changes the inode.
func WriteFileMode(path string, data []byte, mode os.FileMode) error {
	return write(path, data, mode, true)
}

// syncFile fsyncs f. A package-level var, not a direct tmp.Sync() call
// inline in write(), so TestWriteFile_FsyncsBeforeRename can substitute a
// spy that records the call (still delegating to the real fsync, so
// behaviour is unchanged) — the same "swap in an observer, restore after"
// shape internal/skill/sync.go uses for atomicWriteFile, applied here
// because deleting the fsync call is otherwise a silent, compiling,
// suite-green mutation: nothing else in this package's tests reads through
// the OS page cache after a simulated crash to notice its absence.
var syncFile = func(f *os.File) error { return f.Sync() }

func write(path string, data []byte, perm os.FileMode, assertMode bool) error {
	if err := RefuseSymlink(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Two ways to land the mode, and which one applies is the whole of
	// rule 1:
	//
	//   createMode is handed to open(2), so the kernel applies the umask
	//   to it — the same path os.WriteFile takes when it creates a file.
	//
	//   chmodTo is applied to the temp file before the rename, which the
	//   umask does NOT touch — used to carry an existing file's mode
	//   across the replace (WriteFile), or to assert one (WriteFileMode).
	//
	// When a chmod is coming, the file is created 0600 first so it is
	// never briefly wider than it will end up.
	createMode := perm
	chmodTo := os.FileMode(0)
	chmod := false
	switch {
	case assertMode:
		chmodTo, chmod, createMode = perm, true, 0o600
	default:
		// os.Lstat, not os.Stat: RefuseSymlink already established the
		// final component is not a link, and Lstat cannot be talked into
		// resolving one that appears between the two calls.
		if fi, err := os.Lstat(path); err == nil && fi.Mode().IsRegular() {
			chmodTo, chmod, createMode = fi.Mode().Perm(), true, 0o600
		}
	}

	tmp, tmpName, err := createTemp(dir, filepath.Base(path), createMode)
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if chmod {
		if err := tmp.Chmod(chmodTo); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
	}
	// Data on disk before the name points at it — see DURABILITY. Routed
	// through syncFile, not called directly, so the durability promise is
	// pinned by TestWriteFile_FsyncsBeforeRename rather than resting on the
	// "19 mutations, all killed" claim alone: deleting this call is a
	// one-line, compiling change with no other observable effect (no test
	// in this package reads through the OS page cache after a simulated
	// crash), so nothing else in the suite would go red for it.
	if err := syncFile(tmp); err != nil {
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

// createTemp makes an exclusively-created sibling of base in dir at exactly
// mode (modulo umask, which is the point — see write).
//
// os.CreateTemp would be the obvious call and is what this used to use, but
// it hardcodes 0600 and offers no way to hand a mode to open(2); the umask
// then never gets a say, and `logmind init` under `umask 077` produced 0644
// files where a plain os.WriteFile produced 0600. The name still carries an
// unguessable random suffix and is still created O_EXCL, which is what makes
// planting a symlink at the temp path useless.
func createTemp(dir, base string, mode os.FileMode) (*os.File, string, error) {
	prefix := filepath.Join(dir, base+".tmp-")
	for i := 0; i < 1000; i++ {
		name := prefix + randomSuffix()
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			return f, name, nil
		}
		if os.IsExist(err) {
			continue
		}
		return nil, "", err
	}
	return nil, "", fmt.Errorf("atomicio: could not create a temp sibling for %s after 1000 attempts", filepath.Join(dir, base))
}

func randomSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on any platform this ships to. If it
		// somehow does, a time+pid suffix keeps the write working; the
		// O_EXCL retry loop above still guarantees exclusivity, only the
		// unguessability weakens.
		return strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.Itoa(os.Getpid())
	}
	return hex.EncodeToString(b[:])
}
