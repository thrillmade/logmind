// Package gitcli wraps the small set of git porcelain/plumbing commands
// the logmind binary needs. Each function is a thin shim over
// `exec.Command("git", ...)` so the rest of the codebase never has to
// remember subprocess invocation details (cwd plumbing, stderr capture,
// error-vs-empty distinction, etc.).
//
// Design choices preserved from the Python v0.6.14 implementation
// (src/logmind/core/git_handler.py):
//
//   - Best-effort semantics. Most callers want "did git answer cleanly?
//     here it is — or nothing". Wrappers that ran git and got a
//     non-zero exit code, a missing binary, or any other failure return
//     an empty result + (in IsRepo's case) false; callers decide
//     whether to treat that as fatal. This matches the Python
//     except-clause shape used throughout gitattributes.py and
//     git_handler.py — see `IsRepo`, `RevParseTopLevel`, `DiffCached`.
//
//   - cwd is always explicit. The Python helpers default cwd to
//     `Path.cwd()`; the Go variants take a `repoRoot` argument so
//     callers can't accidentally invoke git from the wrong directory.
//
//   - stderr stays buried. Per the Python pattern `>/dev/null 2>&1`
//     (see git_handler.git_add, gitattributes.configure_merge_drivers),
//     wrappers do not surface git's stderr to the user unless the
//     caller explicitly asks for it (CombinedOutput). Hooks and
//     install routines treat git as fire-and-forget.
//
// All wrappers run the literal `git` binary off PATH — they do NOT
// shell out, so spaces / quoting in filenames are handled correctly.
package gitcli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ErrGitNotFound is returned when the `git` binary is missing from PATH.
// Callers can errors.Is(...) it to distinguish "no git installed" from
// "git ran but exited non-zero".
var ErrGitNotFound = errors.New("git binary not found on PATH")

// IsRepo reports whether repoRoot is inside a git work tree.
//
// Mirrors src/logmind/core/git_handler.is_git_repo: returns false on
// any error path (missing git binary, permission denied on .git/,
// unreachable network mount). Callers decide whether to treat false
// as fatal.
func IsRepo(repoRoot string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = repoRoot
	// Suppress stderr — the Python helper passes capture_output=True for
	// the same reason. We only care about the exit code.
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	return cmd.Run() == nil
}

// RevParseTopLevel returns the absolute path of the repository root
// containing repoRoot, or an empty string + non-nil error if git
// can't resolve it (not a repo, no git binary, permission error).
//
// Used by `logmind install-hook` so the hook is written to the
// repo's true root even when the user invoked the CLI from a
// subdirectory — matches the Python pattern (cli.py line 2833-2839).
func RevParseTopLevel(repoRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrGitNotFound
		}
		return "", &GitError{Op: "rev-parse --show-toplevel", Err: err, Stderr: stderr.String()}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// TopLevel resolves the absolute repository root that contains cwd via
// `git -C cwd rev-parse --show-toplevel`. Returns (toplevel, true) when
// cwd is inside a work tree, or ("", false) for every failure path (not a
// repo, missing git binary, permission error) — the boolean is the single
// "is this a repo?" signal callers switch on, so they never have to
// discriminate error kinds.
//
// This is the RIGHT resolver for guard-commit: the harness may hand it a
// SUBDIRECTORY of the repo as the evaluated cwd (PreToolUse payloads carry
// the tool call's cwd, not the repo root). Resolving to the toplevel means
// both config loading (`.logmind/config.yml` lives at the repo root) AND
// every git diff/status op (whose --porcelain paths are root-relative) use
// the same, correct base directory — otherwise an untracked file staged
// from a subdir would be miscounted and silently bypass enforcement.
//
// RevParseTopLevel (above) is the older error-returning twin kept for
// install-hook's diagnostics; TopLevel is the boolean-returning form the
// guard-commit hot path wants.
func TopLevel(cwd string) (string, bool) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", false
	}
	top := strings.TrimSpace(stdout.String())
	if top == "" {
		return "", false
	}
	return top, true
}

// CurrentBranch returns the current branch name (e.g. "v1-go-rewrite")
// or an empty string when HEAD is detached, when cwd is not a repo, or on
// any other error path. Mirrors git_handler.current_branch.
//
// Implementation: `git symbolic-ref --short HEAD`. Returns "" on detached
// HEAD — symbolic-ref exits non-zero there because HEAD holds a raw SHA
// rather than a ref.
//
// An UNBORN repo (no commits yet) is NOT an empty-string case, and callers
// have assumed otherwise. symbolic-ref resolves HEAD's ref WITHOUT
// dereferencing it to a commit, so it succeeds before the first commit and
// answers with the branch HEAD already points at (a fresh `git init` gives
// `main`, exit 0) even while `git rev-parse --verify HEAD` fails. Anything
// that needs "does this repo have a commit" must ask rev-parse, not this.
func CurrentBranch(repoRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

// RemoteHEAD returns the default-branch name from
// `refs/remotes/origin/HEAD`, e.g. "main" or "master". Empty string
// on any failure. Used by `logmind rebase` (B3) and as input to
// `default_branch` resolution.
func RemoteHEAD(repoRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ""
	}
	out := strings.TrimSpace(stdout.String())
	// Strip the "origin/" prefix to match git_handler.default_branch
	// which slices off the prefix manually.
	if strings.HasPrefix(out, "origin/") {
		return strings.TrimPrefix(out, "origin/")
	}
	return out
}

// RemoteRepoName returns the repository name parsed from origin's URL — the
// last path segment with any ".git" suffix stripped. Handles both scp-style
// ("git@github.com:thrillmade/logmind.git" → "logmind") and URL-style
// ("https://github.com/thrillmade/logmind" → "logmind"). Empty string on any
// failure (no origin, parse miss). Used only for the OPTIONAL
// file_structure.root_label: "auto" convenience, which falls back to the
// checkout basename when this is empty.
func RemoteRepoName(repoRoot string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ""
	}
	url := strings.TrimSpace(stdout.String())
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, ".git")
	if i := strings.LastIndexAny(url, "/:"); i >= 0 {
		url = url[i+1:]
	}
	return url
}

// CommonDirRepoName returns the basename of the directory containing the
// repository's SHARED git dir — `git rev-parse --git-common-dir`. This is
// deliberately not `--show-toplevel`: inside a `git worktree` checkout,
// `--show-toplevel` resolves to the WORKTREE's own path (e.g.
// ".../agent-<id>"), which is exactly the checkout-basename bug this
// exists to route around. `--git-common-dir` instead points at the ONE
// .git directory every worktree of a repo shares, so its parent's
// basename is the actual repository name no matter which worktree (or how
// deep a subdirectory of one) the command runs from.
//
// git prints `--git-common-dir` relative to repoRoot in the common case
// (".git" at the top level, "../../.git" three directories down) and only
// switches to an absolute path when relative wouldn't resolve correctly
// (observed from inside a worktree). We join against repoRoot before
// taking the parent's basename so both forms resolve the same way.
//
// Empty string on any failure (not a repo, missing git binary, unexpected
// path shape) — callers fall back to the checkout directory's basename,
// the same degrade pattern as RemoteRepoName.
func CommonDirRepoName(repoRoot string) string {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ""
	}
	dir := strings.TrimSpace(stdout.String())
	if dir == "" {
		return ""
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoRoot, dir)
	}
	name := filepath.Base(filepath.Dir(dir))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	return name
}

// DiffCachedNames returns the list of staged file paths
// (`git diff --cached --name-only`). Empty slice on any failure or
// when nothing is staged.
//
// Output preserves git's exact path encoding — `core.quotepath` may
// cause non-ASCII filenames to be octal-escaped, but check-decisions
// only uses the paths for prefix/suffix string comparison so the
// encoding round-trip is invisible.
func DiffCachedNames(repoRoot string) []string {
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil
	}
	trimmed := strings.TrimRight(stdout.String(), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// NumstatLine is a single row from `git diff --cached --numstat`.
// Added/Removed are the line-change counts. Binary files appear with
// Added == "-" and Removed == "-" — the strings (not zero ints) are
// preserved so callers can replicate the Python `added == "-"` skip
// rule byte-identically.
type NumstatLine struct {
	Added   string
	Removed string
	Path    string
}

// DiffCachedNumstat parses `git diff --cached --numstat` into typed
// rows. Lines that don't split into three tab-separated fields are
// silently skipped (matching the Python `if len(parts) != 3: continue`
// path in cli.py:2495).
//
// Returns an empty slice on git failure — check-decisions treats that
// as "0 lines changed" rather than blowing up the pre-commit hook.
func DiffCachedNumstat(repoRoot string) []NumstatLine {
	cmd := exec.Command("git", append([]string{"diff", "--cached"}, numstatFlags...)...)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil
	}
	return parseNumstat(stdout.String())
}

// DiffNumstat parses `git diff --numstat` (unstaged changes to
// TRACKED files — working tree vs. index, no --cached) into typed
// rows. Same parsing rules as DiffCachedNumstat; returns nil on git
// failure. Used by guardcommit's WorkingTreeUnion diff mode: the
// harness PreToolUse hook fires BEFORE a compound `git add -A &&
// git commit` stages anything, so `--cached` alone would undercount a
// change that is still sitting unstaged at evaluation time.
func DiffNumstat(repoRoot string) []NumstatLine {
	cmd := exec.Command("git", append([]string{"diff"}, numstatFlags...)...)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil
	}
	return parseNumstat(stdout.String())
}

// rangeSpec renders the three-dot range `check-decisions --base/--head`
// evaluates: base...head is the diff of head against the MERGE BASE of
// the two, which is what a pull request actually proposes to add. A
// two-dot range would also attribute everything that landed on base
// since the branch forked, failing a PR for its neighbours' lines.
func rangeSpec(base, head string) string { return base + "..." + head }

// DiffRangeNames returns the file paths a base...head range touches
// (`git diff --name-only base...head`).
//
// Unlike DiffCachedNames this reports an ERROR rather than degrading to
// an empty slice. Its caller is the `check-decisions` gate, which is the
// point that actually blocks a merge (SPEC §3.4) — a bad ref there must
// fail loudly, because "git couldn't resolve the range" and "the range
// changed nothing" are the same empty result and only one of them should
// let a pull request through.
func DiffRangeNames(repoRoot, base, head string) ([]string, error) {
	out, err := runDiffRange(repoRoot, base, head, "--name-only")
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimRight(out, "\n")
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// numstatFlags is the ONE flag list every substantive-line count runs
// with. SPEC §3.4 drives all three enforcement points from "one shared
// evaluation," so the flags live here rather than at each call site —
// a flag one surface passes and another doesn't is that one evaluation
// quietly becoming two.
//
// --no-renames is load-bearing, and its absence was a live gate hole.
// With rename detection ON, git renders a cross-directory rename as a
// SINGLE row whose path field is `old => new`:
//
//	150	0	docs/notes.md => src/payload.go
//
// guardcommit.IsExcludedPath prefix-matches that whole string, so the
// row is excluded as `docs/...` and 550 lines of new Go counted zero —
// the gate passed a change it exists to stop. --no-renames splits it
// into a deletion under docs/ (correctly excluded) and an addition under
// src/ (correctly counted).
//
// Deliberately NOT fixed by teaching IsExcludedPath to parse `old =>
// new`: git has two rename renderings (that one, and the compact
// `{docs => src}/sub/notes.md`), so a parser owes both plus whatever
// git adds later. Not asking for renames at all has no such surface.
var numstatFlags = []string{"--numstat", "--no-renames"}

// DiffRangeNumstat parses `git diff --numstat base...head` into typed
// rows. Same parsing rules as DiffCachedNumstat, same loud-on-failure
// contract as DiffRangeNames.
//
// The flags deliberately match DiffCachedNumstat's exactly: SPEC §3.4
// drives every enforcement point from "one shared evaluation," and a flag
// one surface passes and another doesn't is that evaluation quietly
// becoming two. See numstatFlags for why --no-renames is among them.
func DiffRangeNumstat(repoRoot, base, head string) ([]NumstatLine, error) {
	out, err := runDiffRange(repoRoot, base, head, numstatFlags...)
	if err != nil {
		return nil, err
	}
	return parseNumstat(out), nil
}

// AddedHunk is the run of lines ONE diff hunk added, with git's leading
// "+" stripped, in file order.
//
// The type exists so the boundary cannot be dropped by accident. Under
// -U0 a hunk covers a contiguous range of the NEW file — its "+" lines
// are exactly that range, adjacent to each other and to nothing else —
// while two hunks are separated by content the change never touched.
// Returning one flat []string throws that away: non-adjacent hunks
// concatenate with no gap, and a reader that scans for structure across
// the join reads text from one part of the file as if it sat in another.
// That was a live gate hole. guardcommit.WellFormedDecisionAdded scanned
// the joined string for a §3.1 section, so prose added by a LATER,
// unrelated hunk became the body of a reasoning section opened in an
// EARLIER one. Measured on the PR head: 302 lines of new Go, an empty
// `**Reasoning:**` in one hunk and an unrelated bullet in another →
// `git commit` exit 0 "allowed (decision-recorded)" and
// `check-decisions --base --head` exit 0; the same change minus the
// second hunk → exit 65 and exit 1.
type AddedHunk []string

// DiffCachedAddedHunks returns the lines the staged diff ADDS to one
// path, grouped by hunk, with git's leading "+" stripped. Nil on any
// failure, matching its DiffCached* siblings.
//
// -U0 asks for no context lines, so every "+" line inside a hunk is
// genuinely new content rather than an unchanged neighbour. Callers use
// this to judge what a change WROTE to a file, not what the file
// contains — SPEC §3.4: "A decision clears the gate by being written,
// not by existing."
func DiffCachedAddedHunks(repoRoot, path string) []AddedHunk {
	cmd := exec.Command("git", "diff", "--cached", "-U0", "--", path)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil
	}
	return parseAddedHunks(stdout.String())
}

// DiffRangeAddedHunks is DiffCachedAddedHunks over a base...head range,
// with DiffRangeNames' loud-on-failure contract.
func DiffRangeAddedHunks(repoRoot, base, head, path string) ([]AddedHunk, error) {
	out, err := runDiffRange(repoRoot, base, head, "-U0", "--", path)
	if err != nil {
		return nil, err
	}
	return parseAddedHunks(out), nil
}

// runDiffRange runs `git diff <flags...> base...head` (with the range
// inserted ahead of any pathspec the caller passed after "--") and
// returns stdout. Shared by the three DiffRange* wrappers so the range
// spelling and the error shape live in one place.
func runDiffRange(repoRoot, base, head string, flags ...string) (string, error) {
	args := []string{"diff"}
	spec := rangeSpec(base, head)
	inserted := false
	for _, f := range flags {
		if f == "--" && !inserted {
			args = append(args, spec)
			inserted = true
		}
		args = append(args, f)
	}
	if !inserted {
		args = append(args, spec)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrGitNotFound
		}
		return "", &GitError{Op: "diff " + spec, Err: err, Stderr: stderr.String()}
	}
	return stdout.String(), nil
}

// parseAddedHunks pulls the added-content lines out of a unified diff,
// stripping the leading "+", one AddedHunk per "@@" hunk.
//
// It tracks hunk state rather than filtering on a "+++" prefix: a real
// added line whose own content begins with "++" renders as "+++...", so
// a prefix test would silently drop it. Only lines after a "@@" header
// are content; a "diff --git" line starts the next file's header block
// and ends the current one.
//
// Every "@@" opens a new group, so a caller can never see two hunks'
// lines as one run — see AddedHunk for why that boundary is the whole
// point. A hunk that added nothing (a pure deletion) contributes no
// group rather than an empty one, so `len(hunks)` counts what was
// written rather than what was edited.
func parseAddedHunks(out string) []AddedHunk {
	var hunks []AddedHunk
	var current AddedHunk
	inHunk := false
	flush := func() {
		if len(current) > 0 {
			hunks = append(hunks, current)
		}
		current = nil
	}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			inHunk = false
		case strings.HasPrefix(line, "@@ "):
			flush()
			inHunk = true
		case inHunk && strings.HasPrefix(line, "+"):
			current = append(current, strings.TrimPrefix(line, "+"))
		}
	}
	flush()
	return hunks
}

// UntrackedFiles returns the repo-relative paths of every untracked
// file (`git status --porcelain -z --untracked-files=all`, "??" rows).
// `--untracked-files=all` expands untracked directories to their
// individual files rather than reporting the directory as one row, so
// every file gets its own numstat below. gitignored files are NOT
// included (git's default; we don't pass --ignored). Nil on any
// failure or when there are no untracked files.
//
// The `-z` flag is load-bearing, not a nicety: without it, git applies
// core.quotepath and octal-ESCAPES any non-ASCII path (e.g. "é.go"
// becomes "\303\251.go" wrapped in double quotes). That escaped string
// then can't be opened by the downstream `git diff --no-index` call in
// UntrackedNumstat, so a unicode-named untracked file would be silently
// dropped from the LOC count — a real enforcement bypass. `-z` emits raw,
// NUL-terminated, unquoted paths instead, and we split on NUL.
func UntrackedFiles(repoRoot string) []string {
	cmd := exec.Command("git", "status", "--porcelain", "-z", "--untracked-files=all")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil
	}
	var files []string
	// -z records are NUL-terminated (not NUL-separated) "XY <path>"
	// entries, no quoting. Untracked files never carry the rename form's
	// second path, so each record is exactly one status+path pair.
	for _, entry := range strings.Split(stdout.String(), "\x00") {
		if !strings.HasPrefix(entry, "?? ") {
			continue
		}
		path := strings.TrimPrefix(entry, "?? ")
		if path != "" {
			files = append(files, path)
		}
	}
	return files
}

// UntrackedNumstat synthesizes numstat-shaped rows for every untracked
// file, treating each as entirely new content. Computed via
// `git diff --no-index --numstat -- <devnull> <path>` per file so
// binary detection and line-counting exactly match git's own numstat
// semantics instead of reimplementing them by hand.
//
// `git diff --no-index` follows plain `diff(1)` exit-status
// conventions: 0 = no difference, 1 = difference found (the expected
// case for every untracked file here — /dev/null vs. a real file is
// always "different"), 2+ = trouble. We accept 0 and 1; anything else
// (including a missing git binary) silently skips that file, matching
// the best-effort posture of every other wrapper in this file.
//
// The raw numstat output echoes the devnull path (e.g.
// "3\t0\t/dev/null => new.txt"), so we discard its Path and
// substitute the real untracked path ourselves.
func UntrackedNumstat(repoRoot string) []NumstatLine {
	var rows []NumstatLine
	for _, path := range UntrackedFiles(repoRoot) {
		cmd := exec.Command("git", "diff", "--no-index", "--numstat", "--", os.DevNull, path)
		cmd.Dir = repoRoot
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() > 1 {
				continue
			}
		}
		for _, row := range parseNumstat(stdout.String()) {
			rows = append(rows, NumstatLine{Added: row.Added, Removed: row.Removed, Path: path})
		}
	}
	return rows
}

// parseNumstat is the shared tab-split parser behind DiffCachedNumstat
// and DiffNumstat — kept as one routine so the "3 tab-separated
// fields or skip the line" rule can't drift between the two callers.
func parseNumstat(out string) []NumstatLine {
	var rows []NumstatLine
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			continue
		}
		rows = append(rows, NumstatLine{Added: parts[0], Removed: parts[1], Path: parts[2]})
	}
	return rows
}

// GitDir resolves the repository's actual git directory for repoRoot
// (`git rev-parse --absolute-git-dir`). Callers that need to inspect
// state files like MERGE_HEAD or rebase-merge/ MUST use this rather
// than naively joining "<repoRoot>/.git" — inside a linked worktree,
// ".git" at the worktree root is a FILE (not a directory) whose
// contents point at "<main-repo>/.git/worktrees/<name>", and the
// per-worktree state files live under THAT directory, not the main
// repo's top-level .git/. Empty string + error on any failure (not a
// repo, no git binary).
func GitDir(repoRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--absolute-git-dir")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrGitNotFound
		}
		return "", &GitError{Op: "rev-parse --absolute-git-dir", Err: err, Stderr: stderr.String()}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// ConfigGet returns the value of `git config --get <key>` for the
// repo at repoRoot. Returns "" + ok=false when the key is unset
// (git exits non-zero in that case — distinct from "set to empty"
// which we don't currently need to disambiguate).
func ConfigGet(repoRoot, key string) (value string, ok bool) {
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return strings.TrimSpace(stdout.String()), true
}

// ConfigSet runs `git config <key> <value>` against repoRoot. Returns
// a non-nil error only if git itself failed (missing binary, write
// permission on .git/config). Idempotent: setting a key to its
// existing value is a no-op at the git layer.
func ConfigSet(repoRoot, key, value string) error {
	cmd := exec.Command("git", "config", key, value)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrGitNotFound
		}
		return &GitError{Op: "config " + key, Err: err, Stderr: stderr.String()}
	}
	return nil
}

// StatusPorcelain runs `git status --porcelain -- <path>` and returns
// the raw stdout (trimmed). Empty string means clean (no
// modifications/untracked); non-empty means there's something to
// commit. Used by `logmind log` (B3) to detect changed agent files
// before staging, and by the spec pulse (internal/cli/pulse.go) to skip
// the staleness advisory when the tracked spec file has uncommitted
// changes right now (the log that's editing the spec shouldn't nag about
// it).
func StatusPorcelain(repoRoot, path string) string {
	cmd := exec.Command("git", "status", "--porcelain", "--", path)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

// IsTrackedFile reports whether relPath (repo-relative; forward slashes
// work regardless of OS since git normalizes them) is tracked in
// repoRoot's index (`git ls-files --error-unmatch`). Returns false on any
// failure path — not a repo, no git binary, or the path simply isn't
// tracked — matching this package's best-effort posture. Used by the
// `logmind log` spec pulse: an untracked or uncommitted spec file has no
// deterministic "last touched" date across clones, so the pulse skips
// rather than guessing from mtime.
func IsTrackedFile(repoRoot, relPath string) bool {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", "--", relPath)
	cmd.Dir = repoRoot
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	return cmd.Run() == nil
}

// LastCommitTime returns the committer date of the most recent commit that
// touched relPath (`git log -1 --format=%cI`, the committer date in strict
// ISO 8601 — parseable with time.RFC3339). Returns the zero Time + false on
// any failure: no commit history for the path, not a repo, or no git
// binary. Committer date (not author date) is deliberate — it reflects
// when the change actually landed on this history, which is what "has the
// spec been touched since these decisions were logged" wants to know.
func LastCommitTime(repoRoot, relPath string) (time.Time, bool) {
	cmd := exec.Command("git", "log", "-1", "--format=%cI", "--", relPath)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return time.Time{}, false
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, out)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// DefaultBranch resolves the repo's default branch. The search descends
// from Python's git_handler.default_branch but is no longer that function:
// step 2 was rebuilt and step 4 is new (both explained below).
//
//  1. refs/remotes/origin/HEAD                  (set by `git clone` or
//     `git remote set-head`)
//  2. the conventional names, RESOLVED rather than ranked (see below)
//  3. single-branch repo: that branch IS the default
//  4. UNBORN HEAD: the branch the first commit will create
//  5. `git config init.defaultBranch`
//  6. hard fallback: "main"
//
// Used by `logmind rebase` (B3) when --base isn't supplied, and — since
// the workflow `on:` filter became a scaffold-time render — by
// `logmind init`, where a wrong answer installs a workflow that never
// fires. The init.defaultBranch rung is kept from Python so a consuming
// repo pointed at `master` via `git config init.defaultBranch master`
// keeps working after the v1 cutover — and step 4 hands an unborn one of
// those the same answer anyway, since `git init` reads that very key to
// decide what to write into HEAD.
//
// Step 2 used to be a fixed PREFERENCE — "local `main` if it exists, else
// `master`" — which answers "main" for a `master` repo that happens to
// carry a stray local `main` (a leftover from a rename, or a branch
// somebody created by reflex). That was tolerable while every caller only
// needed a rebase base; it is not tolerable now that the answer is written
// into a workflow trigger, because the wrong name there is a check that
// silently never runs. It now resolves instead: if only one of the two
// conventional names exists, it IS the answer; when both do, the tie is
// broken by evidence about which one this repository actually uses, and
// only a tie no evidence can break still resolves to "main".
//
// Step 4 exists because a repo with NO COMMITS YET is not a repo with no
// evidence — the sentence above once claimed the whole search had that
// property, and this is the case that made it false. `git init -b trunk`
// writes `trunk` into HEAD; that is where the first commit lands and what
// the forge will call the default branch. But a repo that has only ever
// been `git init`-ed carries no refs at all, so steps 1-3 each came up
// empty and `logmind init` scaffolded `branches: [main]` — a trigger
// matching no branch the repo will ever have, on the README's own Quick
// Start, at exit 0.
//
// The step is guarded on UNBORN specifically. HEAD read unconditionally
// would make every feature branch its own default and collapse
// onNonDefaultBranch (internal/cli/derived.go) to false everywhere; step
// 2's tiebreak (b) consults HEAD for exactly that reason, and only as a
// tiebreak. Unborn is NOT the one state where HEAD is "the only branch
// this repository has" — that was the premise here until a repo with
// commits on `develop` and `feature` (no origin), then `git checkout
// --orphan gh-pages`, disproved it: HEAD was unborn on gh-pages, and step
// 4 answered `gh-pages` anyway, though the repository plainly has other
// branches. Unborn is only evidence of that when refs/heads/ is EMPTY —
// no born branch exists yet at all — so the step is gated on that,
// reusing step 3's ref listing rather than re-querying it.
//
// Its PLACE in the order is the other half of the answer:
//
//   - below 1-3, so no answer they already give can change. origin/HEAD
//     still wins for a clone whose local HEAD points elsewhere, and that is
//     right on the merits too: the remote's declared default outranks a
//     local ref that has never been pushed.
//   - above init.defaultBranch, because that config is what `git init`
//     CONSULTED in order to write HEAD, and `-b` overrides it. HEAD is the
//     answer; the config is the guess that may already have been overruled.
func DefaultBranch(repoRoot string) string {
	// 1. origin/HEAD
	if name := RemoteHEAD(repoRoot); name != "" {
		return name
	}

	// 2. the conventional names — resolved, not ranked.
	if name := resolveConventionalBranch(repoRoot); name != "" {
		return name
	}

	// 3. Single-branch repo
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	cmd.Dir = repoRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	var branches []string
	if cmd.Run() == nil {
		branches = strings.Fields(out.String())
		if len(branches) == 1 {
			return branches[0]
		}
	}

	// 4. Unborn HEAD — the branch the first commit will create. Gated on
	// refs/heads/ being EMPTY (see the doc comment above): reuses the
	// listing step 3 already fetched rather than querying it twice. An
	// established repo — branches already exist — that runs `git checkout
	// --orphan` also leaves HEAD unborn, on a name that is not the
	// repository's default, just a new branch nobody has committed to yet.
	if len(branches) == 0 {
		if name := unbornHEAD(repoRoot); name != "" {
			return name
		}
	}

	// 5. init.defaultBranch
	if value, ok := ConfigGet(repoRoot, "init.defaultBranch"); ok && value != "" {
		return value
	}

	// 6. Hard fallback
	return "main"
}

// unbornHEAD returns the branch name HEAD points at when that branch does
// not exist yet — a repo `git init` has created but nothing has committed
// to. Empty string for every other state: a born branch, a detached HEAD, a
// HEAD pointing outside refs/heads/, or a directory that is not a repo.
//
// Read the CurrentBranch contract above before touching this: `git
// symbolic-ref HEAD` SUCCEEDS on an unborn repo, resolving HEAD's ref
// without dereferencing it to a commit. So the probe for unborn-ness is not
// "does symbolic-ref fail" (it does not; a previous round wrote that false
// premise into seven places) but "does the ref it names exist yet".
//
// The full ref is used rather than CurrentBranch's `--short`: --short
// answers `custom/x` for a HEAD at refs/custom/x, which git does allow
// committing to, and which is not a branch. Requiring the refs/heads/
// prefix keeps this from reporting one as an unborn default branch.
func unbornHEAD(repoRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "HEAD")
	cmd.Dir = repoRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{}
	if cmd.Run() != nil {
		return ""
	}
	ref := strings.TrimSpace(stdout.String())
	name, ok := strings.CutPrefix(ref, "refs/heads/")
	if !ok || name == "" {
		return ""
	}
	if refExists(repoRoot, ref) {
		return "" // born: the branch HEAD names already has a commit.
	}
	return name
}

// conventionalDefaultBranches are the two names a repository's default
// branch is called when nobody has said otherwise. Order here is NOT a
// preference — resolveConventionalBranch never returns one because it is
// listed first; the last tiebreak below is the only place the order shows.
var conventionalDefaultBranches = []string{"main", "master"}

// resolveConventionalBranch answers step 2 of DefaultBranch: which of the
// conventional names, if either, is this repository's default branch.
// Returns "" when neither exists locally, so DefaultBranch falls through
// to its remaining steps.
//
// The defect this replaces: a fixed `main`-before-`master` preference
// returned "main" for a repo whose default is `master` merely because a
// stray local `main` existed. Covering two names blindly (the old
// `branches: [main, master]` workflow filter) covered that case by
// accident; rendering ONE name only helps if the one rendered is right.
//
// So when both names exist the tie is broken by evidence, strongest first:
//
//	a. the remote — exactly one of origin/main, origin/master exists.
//	   origin/HEAD is already gone (DefaultBranch step 1), but which
//	   branches the remote actually publishes still outranks anything
//	   local: a stray local `main` in a clone of a `master` repo has no
//	   origin/main behind it.
//	b. HEAD — the branch currently checked out, when it is one of the two.
//	   Only ever consulted as a tiebreak: HEAD is the CURRENT branch, and
//	   returning it unconditionally would make every feature branch its own
//	   "default" and collapse onNonDefaultBranch to false everywhere.
//	c. init.defaultBranch, when it names one of the two. Scoped to the tie
//	   deliberately — DefaultBranch step 5 already reads this key, but as a
//	   free-form answer; here it only gets to pick between two branches
//	   that both exist.
//	d. the conventional order. A repo with both names, no origin, no
//	   matching HEAD and no config has told us nothing.
func resolveConventionalBranch(repoRoot string) string {
	var present []string
	for _, candidate := range conventionalDefaultBranches {
		if refExists(repoRoot, "refs/heads/"+candidate) {
			present = append(present, candidate)
		}
	}
	if len(present) == 0 {
		return ""
	}
	if len(present) == 1 {
		return present[0]
	}

	// a. the remote's own branch set.
	var onRemote []string
	for _, candidate := range present {
		if refExists(repoRoot, "refs/remotes/origin/"+candidate) {
			onRemote = append(onRemote, candidate)
		}
	}
	if len(onRemote) == 1 {
		return onRemote[0]
	}

	// b. the checked-out branch, if it is one of the candidates.
	if head := CurrentBranch(repoRoot); slices.Contains(present, head) {
		return head
	}

	// c. init.defaultBranch, but only as a choice between the candidates.
	if value, ok := ConfigGet(repoRoot, "init.defaultBranch"); ok && slices.Contains(present, value) {
		return value
	}

	// d. no evidence at all.
	return present[0]
}

// refExists reports whether a fully-qualified ref (refs/heads/main,
// refs/remotes/origin/master, …) resolves in repoRoot. False on any error,
// including "not a git repository".
func refExists(repoRoot, ref string) bool {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = repoRoot
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	return cmd.Run() == nil
}

// RunCaptured runs `git <args>` against repoRoot and returns stdout,
// stderr, and the run error. Used by the B3 `rebase` wrapper which
// needs to surface git's stderr to the user verbatim on failure.
//
// Unlike the higher-level wrappers above (which swallow stderr), this
// is the explicit "expose everything" variant. Callers print the
// stderr themselves to match Python's `e.stderr` formatting.
func RunCaptured(repoRoot string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err = cmd.Run()
	return so.String(), se.String(), err
}

// Fetch runs `git fetch <remote> <ref>` (a NETWORK call). Used only by explicit
// commands (logmind warp) — never the `logmind log` hot path.
func Fetch(repoRoot, remote, ref string) error {
	_, _, err := RunCaptured(repoRoot, "fetch", remote, ref)
	return err
}

// ShowFile returns the content of path at ref (`git show <ref>:<path>`).
// ("", false) if the path does not exist at ref or on any error.
func ShowFile(repoRoot, ref, path string) (string, bool) {
	out, _, err := RunCaptured(repoRoot, "show", ref+":"+path)
	if err != nil {
		return "", false
	}
	return out, true
}

// RestorePathsToHead restores each path to its committed (HEAD) content in BOTH
// the index and the working tree (`git checkout HEAD -- <path>`), discarding any
// staged or unstaged change. Per-path and best-effort: a path untracked at HEAD
// errors for that path only and is skipped; the first error (if any) is returned
// for logging but callers generally ignore it (the derived docs are purely
// generated, so a failed restore just leaves the pre-existing state).
//
// A thin wrapper over RestorePathsToRef(repoRoot, "HEAD", paths...) — kept as
// its own named function because "HEAD" is meaningful, reusable shorthand
// wherever the caller genuinely wants the committed tip, as opposed to the
// derived-docs pin target (DefaultBranchMergeBase), which is a DIFFERENT ref
// on a diverged branch — see that function's doc comment for why.
func RestorePathsToHead(repoRoot string, paths ...string) error {
	return RestorePathsToRef(repoRoot, "HEAD", paths...)
}

// RestorePathsToRef restores each path to ref's content in BOTH the index and
// the working tree (`git checkout <ref> -- <path>`), discarding any staged or
// unstaged change. Per-path and best-effort: a path untracked at ref errors
// for that path only and is skipped; the first error (if any) is returned for
// logging but callers generally ignore it.
func RestorePathsToRef(repoRoot, ref string, paths ...string) error {
	var firstErr error
	for _, p := range paths {
		if _, _, err := RunCaptured(repoRoot, "checkout", ref, "--", p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// IsPathStaged reports whether path currently differs between the index and
// HEAD — i.e. whether it has a STAGED change (`git diff --cached --quiet --
// <path>` exits 1, meaning a difference exists).
//
// This is the signal commitDecision's L1 restore (internal/cli/log.go) and
// guardCommitHarness's L2b restore (internal/cli/guard_commit.go) use to
// distinguish a DELIBERATE staged change to a derived doc — chiefly
// `logmind warp`'s merge-base repair, which stages the fix so it survives
// into the caller's next commit (see runWarp, internal/cli/warp.go) — from
// an ACCIDENTAL unstaged dirty working tree (a stray hook regen, a hand
// edit nobody `git add`ed yet). Both callers restore only the paths this
// reports NOT staged, leaving an already-staged path alone.
//
// Best-effort like every other helper in this package, and conservative in
// the direction that matters for those callers: only a confirmed exit-code
// of exactly 1 ("yes, the index differs from HEAD for this path") reports
// true. Every other outcome — no difference (exit 0), a missing git
// binary, an unborn HEAD, a bad pathspec, any other failure — reports
// false. Callers use `true` to SKIP a restore, so a false negative here
// just falls back to "restore it" (the pre-existing, unconditional
// behavior), never a new way to silently let a genuinely accidental dirty
// copy through.
func IsPathStaged(repoRoot, path string) bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet", "--", path)
	cmd.Dir = repoRoot
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	err := cmd.Run()
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == 1
	}
	return false
}

// DefaultBranchMergeBase resolves the ref the derived-docs pin-preservation
// restore (commitDecision's L1, guardCommitHarness's L2b — see
// internal/cli/derived.go) should target: the merge-base between HEAD and the
// best LOCALLY known ref for repoRoot's default branch. The invariant those
// callers enforce is defined in terms of the merge-base ("byte-identical to
// its merge-base with the default branch"), so restoring TO the merge-base —
// rather than to HEAD — is the more correct implementation, and it is
// SELF-REPAIRING: a branch whose HEAD already carries a diverged copy of the
// derived docs (an old binary regenerated them locally and committed, or a
// hand edit landed) gets corrected back to the invariant's own definition
// instead of having the diverged HEAD copy silently re-affirmed on every
// subsequent `logmind log`.
//
// Resolution order:
//
//  1. merge-base(origin/<default>, HEAD) — the canonical, up-to-date view a
//     `git fetch` would have populated; the same base CI's derived-docs gate
//     effectively diffs against.
//  2. merge-base(<default>, HEAD) — a local branch of the same name, for a
//     repo with no origin tracking ref (no remote, or origin/HEAD unset).
//  3. "HEAD" — neither resolves (shallow clone, detached HEAD, a fresh
//     single-branch repo with no fork point to diff against). HEAD is always
//     well-defined, and it changes nothing for the common, UNDIVERGED case:
//     when the branch has never touched these files, HEAD's content for them
//     already equals the merge-base's, so this fallback is a genuine no-op
//     there — it only matters (and only helps) on a branch that actually
//     diverged.
//
// Pure LOCAL computation — no `git fetch` — so this stays safe to call from
// `logmind log`'s network-free hot path. If origin/<default> is stale (the
// caller never ran `logmind warp` / `git fetch`), the merge-base is computed
// against whatever was last fetched — still strictly better than blindly
// trusting the branch's own possibly-diverged HEAD, which is the bug this
// exists to fix.
func DefaultBranchMergeBase(repoRoot string) string {
	def := DefaultBranch(repoRoot)
	for _, ref := range []string{"origin/" + def, def} {
		if sha, ok := MergeBase(repoRoot, ref); ok {
			return sha
		}
	}
	return "HEAD"
}

// MergeBase returns the merge-base commit SHA of ref and HEAD. Best-effort:
// ("", false) on any error (ref missing, no common ancestor, not a repo).
//
// This was briefly deleted as unused — the derived-docs gate settled on
// `gh pr diff` for its branch-vs-base comparison, which needed no local
// merge-base. DefaultBranchMergeBase above makes it live again: the
// zero-conflict invariant is DEFINED against the merge-base, so the local
// restore path has to be able to compute one.
func MergeBase(repoRoot, ref string) (string, bool) {
	out, _, err := RunCaptured(repoRoot, "merge-base", ref, "HEAD")
	if err != nil {
		return "", false
	}
	sha := strings.TrimSpace(out)
	return sha, sha != ""
}

// AddPaths runs `git add <path>...` against repoRoot. Returns a
// non-nil error only when git itself fails — callers wrap it in
// best-effort logic where appropriate (e.g., installer hooks).
//
// Pass zero paths and AddPaths is a no-op (matches git_handler.git_add
// line 54-55).
func AddPaths(repoRoot string, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrGitNotFound
		}
		return &GitError{Op: "add", Err: err, Stderr: stderr.String()}
	}
	return nil
}

// AddAll runs `git add -A` against repoRoot. Mirrors Python's
// git_handler.git_add_all — used by `logmind log --stage all` (the
// default since v0.2.7) so every working-tree change is folded into
// the decision commit. Returns the same GitError shape as AddPaths.
//
// Note: callers who only want to stage specific files should use
// AddPaths instead. This is the "sweep everything" surface; the memory
// note `feedback_logmind_stage_scoped.md` documents the user
// preference for `--stage all` as the default to avoid silently
// dropping unstaged work.
func AddAll(repoRoot string) error {
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrGitNotFound
		}
		return &GitError{Op: "add -A", Err: err, Stderr: stderr.String()}
	}
	return nil
}

// Commit runs `git commit -m <message>` against repoRoot. Returns a
// GitError carrying git's stderr when commit fails (e.g., nothing
// staged, hook rejection). Used by `logmind log` after AddAll /
// AddPaths.
//
// We don't expose --amend or --no-verify here — both are foot-guns
// for a decision-logging primitive. Agents who need either should
// shell out via RunCaptured.
func Commit(repoRoot, message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrGitNotFound
		}
		return &GitError{Op: "commit", Err: err, Stderr: stderr.String()}
	}
	return nil
}

// Push runs `git push` against repoRoot. Returns a GitError carrying
// git's stderr on failure (e.g., no upstream configured, network error,
// rejected non-fast-forward). Used by `logmind log` per SPEC §3.1: on
// success the third stdout line reads "✓ Committed and pushed changes";
// when push is suppressed OR fails (including the common "no upstream"
// case for a fresh local repo), the caller falls back to
// "✓ Committed changes".
//
// No --force / --force-with-lease here — `logmind log` is an append-only
// primitive and pushing a regular fast-forward is the right shape. The
// `logmind rebase` wrapper carries the force-with-lease surface for the
// rare case a rebase is involved.
//
// Args are appended verbatim (e.g., Push(cwd, "origin", "main") runs
// `git push origin main`). Zero args runs plain `git push` and relies
// on the branch's upstream tracking.
//
// Fail-fast on auth. Push is the FIRST network git op in this package,
// and `logmind log` runs it on the hot path (auto_push defaults true).
// git's credential/passphrase prompts read from the controlling TTY,
// NOT from cmd.Stdin — so on a box that HAS a controlling terminal and
// no credential helper, an https/ssh remote needing auth would BLOCK
// indefinitely, defeating the "push is non-fatal, never blocks the log"
// guarantee. We force non-interactive auth so any auth-needed case
// errors out immediately and the caller falls back to the
// "✓ Committed changes" line:
//   - GIT_TERMINAL_PROMPT=0 disables git's own username/password and
//     "authenticity of host" prompts (https + git's ssh prompts).
//   - GIT_SSH_COMMAND=ssh -oBatchMode=yes tells OpenSSH never to prompt
//     for a passphrase / password (ssh remotes).
//
// pushTimeout bounds how long a single `git push` may run before it is
// killed. Push is the first NETWORK op in this package, and `logmind log`
// runs it on the hot path (git.auto_push defaults true) — a wedged remote
// (dead TCP connection, hung auth handshake, etc.) must not hang every log
// forever. Var (not const) so tests can shrink it for a fast, hermetic
// hang test.
var pushTimeout = 45 * time.Second

// pushWaitDelay bounds how long Run's internal Wait keeps blocking on the
// command's I/O pipes AFTER pushTimeout's context kills the direct child —
// mirrors doctor.probePathResolution's WaitDelay hardening (see that
// function's comment for the full daemonizing-wrapper rationale: a context
// timeout only signals the direct child, so a process that leaves a
// grandchild holding the stdout/stderr pipe open can still block Wait
// indefinitely without this). Var so tests can shrink it too.
var pushWaitDelay = 2 * time.Second

func Push(repoRoot string, args ...string) error {
	gitArgs := append([]string{"push"}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), pushTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes",
	)
	cmd.WaitDelay = pushWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrGitNotFound
		}
		return &GitError{Op: "push", Err: err, Stderr: stderr.String()}
	}
	return nil
}

// GitError wraps a git subprocess failure with the captured stderr
// so callers can present meaningful diagnostics. The Python helpers
// raise GitError(f"...{e.stderr}") in the same style.
type GitError struct {
	Op     string
	Err    error
	Stderr string
}

func (e *GitError) Error() string {
	if e.Stderr != "" {
		return "git " + e.Op + ": " + strings.TrimSpace(e.Stderr)
	}
	return "git " + e.Op + ": " + e.Err.Error()
}

func (e *GitError) Unwrap() error { return e.Err }
