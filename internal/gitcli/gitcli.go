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
	"errors"
	"os"
	"os/exec"
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
// or an empty string when HEAD is detached, the repo is unborn without
// a usable symbolic-ref answer, or any error path. Mirrors
// git_handler.current_branch.
//
// Implementation: `git symbolic-ref --short HEAD`. Returns "" on
// detached HEAD (symbolic-ref exits non-zero in that case).
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

// UpstreamRef returns the value of `@{u}` for the current branch
// (typically "origin/<branch>"), or an empty string if no upstream
// is configured. Used by the post-merge hook orphan-branch check,
// but ALSO callable from Go for testing the hook logic in-process.
func UpstreamRef(repoRoot string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "@{u}")
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
	cmd := exec.Command("git", "diff", "--cached", "--numstat")
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
	cmd := exec.Command("git", "diff", "--numstat")
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil
	}
	return parseNumstat(stdout.String())
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

// DefaultBranch resolves the repo's default branch following the same
// 5-step search Python's git_handler.default_branch uses:
//
//  1. refs/remotes/origin/HEAD                  (set by `git clone` or
//     `git remote set-head`)
//  2. local `main` if it exists, else `master`
//  3. single-branch repo: that branch IS the default
//  4. `git config init.defaultBranch`
//  5. hard fallback: "main"
//
// Used by `logmind rebase` (B3) when --base isn't supplied. Same
// resolution order as Python so a consuming repo configured to point
// at `master` via `git config init.defaultBranch master` keeps
// working after the v1 cutover.
func DefaultBranch(repoRoot string) string {
	// 1. origin/HEAD
	if name := RemoteHEAD(repoRoot); name != "" {
		return name
	}

	// 2. local main / master
	for _, candidate := range []string{"main", "master"} {
		cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+candidate)
		cmd.Dir = repoRoot
		cmd.Stdout = &bytes.Buffer{}
		cmd.Stderr = &bytes.Buffer{}
		if cmd.Run() == nil {
			return candidate
		}
	}

	// 3. Single-branch repo
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	cmd.Dir = repoRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	if cmd.Run() == nil {
		branches := strings.Fields(out.String())
		if len(branches) == 1 {
			return branches[0]
		}
	}

	// 4. init.defaultBranch
	if value, ok := ConfigGet(repoRoot, "init.defaultBranch"); ok && value != "" {
		return value
	}

	// 5. Hard fallback
	return "main"
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
func Push(repoRoot string, args ...string) error {
	gitArgs := append([]string{"push"}, args...)
	cmd := exec.Command("git", gitArgs...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes",
	)
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
