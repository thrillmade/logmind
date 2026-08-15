// Package gitattr manages the logmind-owned block in `.gitattributes`
// and the per-clone git-config entries that define the matching
// custom merge drivers.
//
// Mental model: this package ships the on-disk artifacts that let git
// invoke `logmind timeline --write %A` and `logmind file-structure
// --write %A` as merge drivers when parallel PRs both touch derived
// docs. The gitattributes file is committed (so every clone gets the
// driver registration); the actual driver definitions live in
// `.git/config` per-clone (git refuses to use a driver that isn't
// explicitly configured locally — security guard against arbitrary-
// command execution from a hostile repo).
//
// Idempotency contract: EnsureBlock + ConfigureMergeDrivers can be
// called on every `logmind init` / refresh / log without producing
// noise. A no-op returns (false, nil); a real change returns
// (true, nil). Mirrors the Python helpers in
// src/logmind/core/gitattributes.py byte-for-byte.
//
// Block format — exactly matches the Python ensure_block output:
//
//	# >>> logmind >>>
//	docs/timeline.md          merge=logmind-timeline
//	docs/file-structure.md    merge=logmind-file-structure
//	# <<< logmind <<<
//
// Whitespace inside the block (the alignment of `merge=` columns) is
// preserved BYTE-for-byte from the Python source — if logmind doctor
// is going to detect drift between binaries that wrote the block,
// the content has to be the same down to the columns.
package gitattr

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/thrillmade/logmind/internal/atomicio"
	"github.com/thrillmade/logmind/internal/gitcli"
)

// BlockStart is the literal sentinel beginning the logmind-managed
// block. Must match src/logmind/core/gitattributes.LOGMIND_GITATTRIBUTES_START.
const BlockStart = "# >>> logmind >>>"

// BlockEnd closes the managed block.
const BlockEnd = "# <<< logmind <<<"

// DefaultLines are the merge-driver registrations logmind installs by
// default. Each line is `<path-pattern> merge=<driver-name>` and must
// match the column alignment used in the Python source — agents and
// doctor compare bytes, not parsed structure.
//
// EVERY derived doc belongs here. The list must stay in step with
// internal/cli/derived.go's derivedDocPaths: a purely-derived file with no
// driver conflicts on an ordinary parallel merge and hands the user conflict
// markers inside a file they are told never to edit by hand. That is exactly
// what happened to docs/timeline-archive.md, which was added to
// derivedDocPaths, the pre-commit restore, warp and the CI gate — and to
// nothing here.
//
// docs/timeline-archive.md gets its OWN driver, not logmind-timeline's:
// git hands a driver the scratch file for one conflicted path, so the driver
// has to render the half that belongs in THAT file. `--half archive` is how
// it says which.
var DefaultLines = []string{
	"docs/timeline.md          merge=logmind-timeline",
	"docs/timeline-archive.md  merge=logmind-timeline-archive",
	"docs/file-structure.md    merge=logmind-file-structure",
}

// MergeDriverConfig is the list of git-config (key, value) pairs that
// define the logmind merge drivers in `.git/config`. Each entry is
// set every time logmind init or a refresh runs; git config no-ops
// when the value already matches, so this is safe to spam.
//
// Order is preserved from the Python source so a `git config --list
// --local` diff between a Python-installed repo and a Go-installed
// repo shows the keys in the same sequence.
// Both timeline drivers pass `--half`: %A is git's scratch file for ONE
// conflicted path, so each driver must render exactly the half that belongs
// in that path and write nothing else. Without it, `logmind timeline --write
// %A` also drops a timeline-archive.md next to the scratch file — at the
// worktree root, untracked, on every merge.
var MergeDriverConfig = []struct {
	Key   string
	Value string
}{
	{"merge.logmind-timeline.driver", "logmind timeline --write %A --half recent"},
	{"merge.logmind-timeline.name", "Regenerate logmind timeline"},
	{"merge.logmind-timeline-archive.driver", "logmind timeline --write %A --half archive"},
	{"merge.logmind-timeline-archive.name", "Regenerate logmind timeline archive"},
	{"merge.logmind-file-structure.driver", "logmind file-structure --write %A"},
	{"merge.logmind-file-structure.name", "Regenerate logmind file structure"},
}

// EnsureBlock makes sure path (a `.gitattributes`) contains the
// logmind-managed block AND that every line in DefaultLines is registered
// inside it. Returns (true, nil) if the file was created, had the block
// appended, or had a missing registration added; (false, nil) when there was
// nothing to do.
//
// Byte-identical to src/logmind/core/gitattributes.ensure_block:
//
//   - If the file doesn't end in `\n`, append one before writing.
//   - If the file's last two chars aren't `\n\n`, append another so
//     the block is visually separated from prior content.
//   - The block ends with a trailing `\n` so the file is well-formed.
func EnsureBlock(path string) (bool, error) {
	return ensureBlockWithLines(path, DefaultLines)
}

func ensureBlockWithLines(path string, lines []string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	existing := string(data)
	if strings.Contains(existing, BlockStart) {
		return addMissingLines(path, existing, lines)
	}

	var b strings.Builder
	b.WriteString(BlockStart)
	for _, line := range lines {
		b.WriteByte('\n')
		b.WriteString(line)
	}
	b.WriteByte('\n')
	b.WriteString(BlockEnd)
	b.WriteByte('\n')
	block := b.String()

	// Mirror the Python padding logic — append a `\n` if the file
	// doesn't already end with one, then another if it doesn't yet
	// have a blank-line separator. This is the EXACT condition from
	// Python (lines 79-82): two cascading checks, not a single rule.
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	if existing != "" && !strings.HasSuffix(existing, "\n\n") {
		existing += "\n"
	}

	// atomicio.WriteFile makes its own parent dir (MkdirAll) and refuses to
	// write through a symlink at path — dangling or not. A bare os.WriteFile
	// here used to follow a dangling .gitattributes symlink via open(2) and
	// write the block wherever it pointed, OUTSIDE the repo, while init still
	// printed "✓ Added logmind block to .gitattributes": the write "succeeded"
	// because os.WriteFile has no opinion about symlinks. This is a plain
	// text file with no reason to assert a mode independent of what's already
	// there, so WriteFile (not WriteFileMode) is the right call.
	if err := atomicio.WriteFile(path, []byte(existing+block), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// addMissingLines registers any `lines` entry whose PATH PATTERN is absent
// from an existing logmind block, inserting it just before the closing
// sentinel. Returns (true, nil) when it wrote.
//
// This is how a repo initialised by an older binary picks up a newly-shipped
// merge driver. Without it, EnsureBlock's "block present → nothing to do"
// meant docs/timeline-archive.md would only ever get a driver in repos
// created after this release — every existing repo would keep handing its
// users conflict markers in a derived file forever, and nothing in the
// system would ever say so.
//
// It matches on the path pattern (the line's first field), never on the whole
// line, so a user who retargeted or renamed a driver keeps their edit: this
// only ever ADDS a pattern logmind owns and has no registration for. Nothing
// inside the block is rewritten or removed, which is the same promise
// EnsureBlock has always made about manual edits.
func addMissingLines(path, existing string, lines []string) (bool, error) {
	startIdx := strings.Index(existing, BlockStart)
	endIdx := strings.Index(existing[startIdx:], BlockEnd)
	if endIdx < 0 {
		// Malformed (unterminated) block — leave it for the user, same as
		// RemoveBlock does.
		return false, nil
	}
	endIdx += startIdx
	block := existing[startIdx:endIdx]

	registered := make(map[string]bool)
	for _, l := range strings.Split(block, "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			registered[f[0]] = true
		}
	}
	var missing []string
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) == 0 || registered[f[0]] {
			continue
		}
		missing = append(missing, l)
	}
	if len(missing) == 0 {
		return false, nil
	}

	updated := existing[:endIdx] + strings.Join(missing, "\n") + "\n" + existing[endIdx:]
	// See ensureBlockWithLines: atomicio.WriteFile refuses a symlinked path
	// (dangling or not) instead of following it, and a plain text file has no
	// mode to assert here either.
	if err := atomicio.WriteFile(path, []byte(updated), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// HasBlock returns true iff path exists and contains the logmind
// block sentinel. Cheap to call; safe on missing files (returns
// false). Mirrors src/logmind/core/gitattributes.has_block.
func HasBlock(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), BlockStart)
}

// RemoveBlock strips the logmind block (and its surrounding sentinels)
// from path. Returns (true, nil) when a block was removed,
// (false, nil) when no block was present.
//
// Newlines around the block are normalised so removing the block from
// a file that had it appended doesn't leave a trailing blank line
// gap. This matches the symmetry: EnsureBlock + RemoveBlock is a
// no-op modulo whitespace.
//
// NOTE: the Python source doesn't currently expose a remove helper —
// the Go side adds it because uninstall paths (B3) and tests both
// need a way to reset state. The byte format is symmetric with
// EnsureBlock so a Python-installed block is removable.
func RemoveBlock(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	existing := string(data)
	startIdx := strings.Index(existing, BlockStart)
	if startIdx < 0 {
		return false, nil
	}
	// End-of-block is the line AFTER the BlockEnd sentinel.
	endIdx := strings.Index(existing[startIdx:], BlockEnd)
	if endIdx < 0 {
		// Malformed block: half-written, leave it for the user.
		return false, nil
	}
	endIdx += startIdx + len(BlockEnd)
	// Consume the trailing newline that EnsureBlock writes.
	if endIdx < len(existing) && existing[endIdx] == '\n' {
		endIdx++
	}
	// Also consume the leading blank-line separator EnsureBlock
	// inserted (the `\n` that preceded BlockStart if the prior
	// content didn't already terminate with two newlines).
	leadStart := startIdx
	if leadStart >= 2 && existing[leadStart-1] == '\n' && existing[leadStart-2] == '\n' {
		leadStart--
	}

	cleaned := existing[:leadStart] + existing[endIdx:]
	// Same reasoning as ensureBlockWithLines: route through atomicio.WriteFile
	// so a symlinked .gitattributes is refused rather than followed.
	if err := atomicio.WriteFile(path, []byte(cleaned), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// ConfigureMergeDrivers writes the per-clone git-config keys that
// define the logmind merge drivers. Returns (true, nil) when at
// least one key was changed (or there was a write); (false, nil)
// when every key already held its expected value.
//
// Silently no-ops outside a git repo — matches the Python guard at
// gitattributes.configure_merge_drivers line 118-120. Per-key
// errors are SWALLOWED (matching the Python `except`) because the
// driver is a merge-time optimization, not a commit-time requirement
// — losing it shouldn't break `logmind log`.
func ConfigureMergeDrivers(repoRoot string) bool {
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return false
	}
	changed := false
	for _, entry := range MergeDriverConfig {
		current, ok := gitcli.ConfigGet(repoRoot, entry.Key)
		if ok && current == entry.Value {
			continue
		}
		if err := gitcli.ConfigSet(repoRoot, entry.Key, entry.Value); err != nil {
			// Mirror the Python `continue` on hiccup — doctor will
			// surface the drift on the next invocation.
			continue
		}
		changed = true
	}
	return changed
}

// DriverConfigured returns true iff every key in MergeDriverConfig
// is set to its expected value in repoRoot's local git config.
// Mirrors src/logmind/core/gitattributes.driver_configured.
func DriverConfigured(repoRoot string) bool {
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return false
	}
	for _, entry := range MergeDriverConfig {
		current, ok := gitcli.ConfigGet(repoRoot, entry.Key)
		if !ok || current != entry.Value {
			return false
		}
	}
	return true
}
