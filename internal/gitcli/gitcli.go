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
	var rows []NumstatLine
	for _, line := range strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n") {
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
// before staging.
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
