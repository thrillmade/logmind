// guard_commit.go — `logmind guard-commit`, the cobra surface over the
// internal/guardcommit decision engine.
//
// This is PR1/3 of the "force-logmind-usage enforcement" feature (plan.md
// §v2.0.0). guard-commit is deliberately Hidden (plumbing, not a
// user-facing verb) and does NOT install itself anywhere — no hook
// registration, no .git/hooks writes, no harness settings.json edits. It
// exists purely so the two hook layers built in a follow-up PR have a
// single binary entry point to shell out to:
//
//   - `logmind guard-commit --layer git-hook --msg-file <path>` from a git
//     commit-msg hook.
//   - `logmind guard-commit --layer harness` (JSON payload on stdin) from
//     the Claude Code harness's PreToolUse hook.
//
// Both layers are manually invocable today (this PR); wiring either one up
// automatically is out of scope here.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/gitcli"
	"github.com/thrillmade/logmind/internal/guardcommit"
)

func newGuardCommitCmd() *cobra.Command {
	var layer string
	var threshold int
	var msgFile string
	var repoRootFlag string
	cmd := &cobra.Command{
		Use:    "guard-commit --layer {harness|git-hook}",
		Short:  "Decision engine behind logmind's commit-enforcement hooks (plumbing)",
		Hidden: true,
		Long: `guard-commit is the shared decision engine two hook layers call to decide
whether a git commit must be steered through 'logmind log' before it's
allowed to proceed.

  --layer harness    the Claude Code harness's PreToolUse hook. Reads a
                      PreToolUse JSON payload ({"tool_name","tool_input":
                      {"command"},"cwd"}) from stdin. Blocks by exiting 2 —
                      PreToolUse only treats exit 2 as "block this tool
                      call." Fails open (exit 0) on anything that isn't a
                      recognizable Bash git-commit invocation.

  --layer git-hook    the git commit-msg hook. Reads --msg-file (first
                      line = the commit subject). Blocks via the standard
                      nonzero-exit convention (git only needs a nonzero
                      status to abort the commit).

This command does not install anything. The hooks that call it
automatically are wired up in a separate PR — today it is invoked
manually, e.g. for testing:

    echo '{"tool_name":"Bash","tool_input":{"command":"git commit -m x"},"cwd":"."}' \
      | logmind guard-commit --layer harness`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode, err := runGuardCommit(
				repoRootFlag, layer, msgFile, threshold, cmd.Flags().Changed("threshold"),
				quietEnabled(cmd), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(),
			)
			if exitCode != 0 {
				// LOUD COMMENT — read this before touching this branch.
				//
				// cmd/logmind/main.go maps EVERY non-nil Execute() error to
				// exit 1 (including cli.ErrSilent — that's the whole point
				// of the ErrSilent convention: "exit non-zero, message
				// already printed"). That convention is correct for every
				// other command in this CLI, but it is WRONG here: the
				// Claude Code harness's PreToolUse hook protocol only
				// treats exit code 2 as "BLOCK this tool call" — exit 1 is
				// non-blocking (the tool call proceeds as if nothing
				// happened). If this branch returned an error instead of
				// calling os.Exit directly, guard-commit's harness layer
				// would silently degrade into a no-op the moment it tried
				// to block anything, while still LOOKING like it worked
				// (main.go would print "Error: ..." and exit 1, easy to
				// miss in hook output). So: this is the one place in this
				// command — and, as of this PR, the only place in the
				// whole CLI — that deliberately bypasses the
				// ErrSilent/exit-1 convention and calls os.Exit directly
				// from inside RunE. Do not "fix" this to `return
				// ErrSilent`; that would reintroduce the silent-bypass bug
				// this comment exists to prevent. runGuardCommit itself
				// never calls os.Exit — only this RunE wrapper does, which
				// is what keeps the harness layer unit-testable (see
				// guard_commit_test.go) without killing the test binary.
				os.Exit(exitCode)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&layer, "layer", "", "Which hook layer is calling: 'harness' or 'git-hook' (required)")
	cmd.Flags().IntVar(&threshold, "threshold", 20,
		"Substantive-line threshold. Explicit use of this flag wins over config git.commit_line_threshold, which wins over the built-in default of 20.")
	cmd.Flags().StringVar(&msgFile, "msg-file", "", "Path to the commit message file (required for --layer git-hook; its first line is the subject)")
	cmd.Flags().StringVar(&repoRootFlag, "repo-root", "", "Repo root to evaluate against. --layer harness prefers the PreToolUse payload's cwd when present; both layers fall back to this flag, then the process's working directory.")
	return cmd
}

// runGuardCommit is the testable core shared by both layers. It dispatches
// to the layer-specific evaluation, which is where the git toplevel is
// resolved from the EVALUATED cwd (see resolveRepoAndConfig) and used for
// BOTH config loading AND the guardcommit.Evaluate call.
//
// Return shape: (exitCode, err). exitCode != 0 tells the RunE wrapper to
// call os.Exit(exitCode) directly (see the LOUD COMMENT above) — used ONLY
// by the harness layer's block path (exit 2). Every other path returns
// exitCode 0 and lets `err` carry the result through the normal
// nil / ErrSilent / plain-error convention: nil on allow, ErrSilent on a
// git-hook block (git just needs nonzero to abort the commit — no special
// exit code required there), or a plain error for genuine misuse (bad
// --layer, missing --msg-file).
//
// This function never calls os.Exit itself, which is what makes it safe to
// call directly from unit tests.
func runGuardCommit(
	repoRootFlag, layer, msgFile string,
	thresholdFlag int, thresholdExplicit bool,
	quiet bool,
	stdin io.Reader, stdout, stderr io.Writer,
) (exitCode int, err error) {
	switch layer {
	case "harness":
		return guardCommitHarness(stdin, stderr, repoRootFlag, thresholdFlag, thresholdExplicit), nil
	case "git-hook":
		if msgFile == "" {
			// Pure CLI-usage error, independent of any repo/config state.
			return 0, fmt.Errorf("--msg-file is required for --layer git-hook")
		}
		return 0, guardCommitGitHook(repoRootFlag, msgFile, thresholdFlag, thresholdExplicit, quiet, stdout, stderr)
	default:
		return 0, fmt.Errorf("--layer must be %q or %q (got %q)", "harness", "git-hook", layer)
	}
}

// resolveRepoAndConfig is the single choke point that fixes the
// "evaluate/config against the wrong directory" family of silent bypasses.
// Given the EVALUATED cwd (which, for the harness layer, may be a
// SUBDIRECTORY of the repo — PreToolUse payloads carry the tool call's
// cwd), it resolves the git toplevel ONCE and returns:
//
//   - repoRoot: the directory to hand guardcommit.Evaluate. When evalCwd
//     is inside a repo this is the TOPLEVEL, so every git diff/status op
//     Evaluate runs (whose --porcelain paths are root-relative) resolves
//     correctly AND config is read from the repo root's
//     .logmind/config.yml. When evalCwd isn't in a repo, we hand back
//     evalCwd unchanged so Evaluate's own IsRepo(evalCwd) check fails and
//     it takes its not-a-repo Allow path (handing back "" instead would
//     make IsRepo inspect the PROCESS cwd, which could wrongly BE a repo).
//   - enforce: git.enforce_commits from the toplevel's config (default
//     true). The full repo off-ramp.
//   - threshold: the effective substantive-line threshold.
//
// config.Load degrades to defaults on a missing/unparseable
// .logmind/config.yml (internal/config's documented "broken config is
// user-fixing-it-later" stance), so a config problem never becomes a
// reason to block — or to wrongly allow — a commit.
func resolveRepoAndConfig(evalCwd string, thresholdFlag int, thresholdExplicit bool) (repoRoot string, enforce bool, threshold int) {
	toplevel, ok := gitcli.TopLevel(evalCwd)
	if !ok {
		// Not in a repo: config falls back to defaults, and we hand
		// Evaluate the original evalCwd so its IsRepo check fails cleanly.
		cfg := config.DefaultConfig()
		return evalCwd, cfg.Git.EnforceCommits, resolveThreshold(cfg, thresholdFlag, thresholdExplicit)
	}
	cfg, _ := config.Load(toplevel)
	return toplevel, cfg.Git.EnforceCommits, resolveThreshold(cfg, thresholdFlag, thresholdExplicit)
}

// evalCwdOr returns the first of preferred / repoRootFlag / the process
// working directory that is non-empty. Used to pick the directory the
// git toplevel is resolved from.
func evalCwdOr(preferred, repoRootFlag string) string {
	if preferred != "" {
		return preferred
	}
	if repoRootFlag != "" {
		return repoRootFlag
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

// resolveThreshold implements the documented precedence: an explicit
// --threshold flag wins; otherwise a positive git.commit_line_threshold
// from config wins; otherwise the hardcoded fallback of 20.
func resolveThreshold(cfg config.Config, flagValue int, flagExplicit bool) int {
	if flagExplicit {
		return flagValue
	}
	if cfg.Git.CommitLineThreshold > 0 {
		return cfg.Git.CommitLineThreshold
	}
	return 20
}

// harnessPayload is the subset of Claude Code's PreToolUse hook JSON
// guard-commit needs. Unknown fields are ignored by encoding/json by
// default, so this stays forward-compatible with payload fields we don't
// care about.
type harnessPayload struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
	Cwd string `json:"cwd"`
}

// guardCommitHarness implements the --layer harness evaluation. Returns
// the process exit code the caller should use: 0 to allow (INCLUDING every
// fail-open case — bad/foreign JSON, a non-Bash tool call, a Bash command
// that isn't a git-commit invocation, and the enforce_commits:false
// off-ramp), or 2 to block (with the reason already written to stderr).
//
// The evaluated cwd is the payload's own cwd when present (a PreToolUse
// payload carries the Bash tool call's working directory, which may be a
// SUBDIRECTORY of the repo), falling back to --repo-root then the process
// cwd. resolveRepoAndConfig then resolves that to the git toplevel so both
// config AND every git op use the correct base directory.
func guardCommitHarness(stdin io.Reader, stderr io.Writer, repoRootFlag string, thresholdFlag int, thresholdExplicit bool) int {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return 0 // fail open: couldn't even read the payload
	}

	var payload harnessPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0 // fail open: unparseable payload
	}
	if payload.ToolName != "Bash" {
		return 0 // fail open: not a shell invocation we can inspect
	}
	if !guardcommit.InvokesGitCommit(payload.ToolInput.Command) {
		return 0 // not a git-commit shape at all — nothing to evaluate
	}

	evalCwd := evalCwdOr(payload.Cwd, repoRootFlag)
	repoRoot, enforce, threshold := resolveRepoAndConfig(evalCwd, thresholdFlag, thresholdExplicit)
	if !enforce {
		return 0 // the repo off-ramp: git.enforce_commits: false
	}

	subject := extractSubjectHint(payload.ToolInput.Command)
	d := guardcommit.Evaluate(repoRoot, subject, threshold, guardcommit.WorkingTreeUnion)
	if d.Allow {
		return 0
	}
	fmt.Fprintln(stderr, d.Reason)
	return 2
}

// guardCommitGitHook implements the --layer git-hook evaluation: resolve
// the git toplevel from --repo-root (or the process cwd), read config +
// the commit subject, evaluate under StagedOnly (the index is final by the
// time a commit-msg hook runs), and translate the result into the standard
// nil / ErrSilent convention.
func guardCommitGitHook(repoRootFlag, msgFile string, thresholdFlag int, thresholdExplicit bool, quiet bool, stdout, stderr io.Writer) error {
	evalCwd := evalCwdOr("", repoRootFlag)
	repoRoot, enforce, threshold := resolveRepoAndConfig(evalCwd, thresholdFlag, thresholdExplicit)
	if !enforce {
		return nil // the repo off-ramp: git.enforce_commits: false
	}

	subject, err := firstLineOfFile(msgFile)
	if err != nil {
		return err
	}

	d := guardcommit.Evaluate(repoRoot, subject, threshold, guardcommit.StagedOnly)
	q := newQout(quiet, stdout, stderr)
	if d.Allow {
		reason := string(d.CarveOut)
		if reason == "" {
			reason = "not a git repository"
		}
		q.chat("✓ guard-commit: allowed (%s)\n", reason)
		return nil
	}
	fmt.Fprintln(stderr, d.Reason)
	return ErrSilent
}

// firstLineOfFile reads path and returns its first line (without the
// trailing newline). Used to pull the commit subject out of the file git
// hands a commit-msg hook.
func firstLineOfFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := string(data)
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	return line, nil
}

// extractSubjectHint does a best-effort, quote-aware scan for git's commit
// message flags — `-m <value>` / `-m<value>` / `--message <value>` /
// `--message=<value>` — in a raw shell command string, and joins EVERY
// occurrence (newline-separated) into one hint string. It exists solely so
// the harness layer's [skip-logmind] carve-out can see the intended commit
// message BEFORE git has recorded anything (the harness runs at PreToolUse,
// ahead of the actual `git commit` invocation).
//
// ALL -m/--message values are collected, not just the first: git
// concatenates multiple -m arguments into the message body, so a
// `[skip-logmind]` marker placed in a SECOND -m (e.g.
// `git commit -m "subject" -m "[skip-logmind]"`) must still be seen —
// reading only the first -m would ignore the marker and OVER-block a
// commit the agent explicitly opted out of.
//
// This is not a shell parser: a message that comes from $EDITOR, a
// template, or is buried inside a substitution simply contributes nothing
// here, which just means the [skip-logmind] carve-out won't apply — every
// other carve-out (env var, decision-file-staged, under-threshold) still
// works normally.
func extractSubjectHint(command string) string {
	words := splitCommandWords(command)
	var messages []string
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case w == "-m" || w == "--message":
			if i+1 < len(words) {
				messages = append(messages, words[i+1])
				i++ // skip the consumed value
			}
		case strings.HasPrefix(w, "--message="):
			messages = append(messages, strings.TrimPrefix(w, "--message="))
		case strings.HasPrefix(w, "-m") && w != "-m":
			// git's short-flag-with-attached-value form, e.g. `-mSubject`.
			messages = append(messages, strings.TrimPrefix(w, "-m"))
		}
	}
	return strings.Join(messages, "\n")
}

// splitCommandWords is a small, self-contained quote-aware whitespace
// tokenizer — deliberately NOT shared with internal/guardcommit's
// tokenizeWords (that one is unexported package-internal plumbing for
// InvokesGitCommit's very different quote/substitution/separator rules).
// This one only needs to turn a command string into words with quotes
// stripped, for the narrow purpose of finding a -m/--message value.
func splitCommandWords(s string) []string {
	var words []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	hasContent := false
	n := len(s)
	flush := func() {
		if hasContent {
			words = append(words, cur.String())
			cur.Reset()
			hasContent = false
		}
	}
	for i := 0; i < n; {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(c)
			}
			i++
		case inDouble:
			if c == '"' {
				inDouble = false
			} else {
				cur.WriteByte(c)
			}
			i++
		case c == '\'':
			inSingle = true
			hasContent = true
			i++
		case c == '"':
			inDouble = true
			hasContent = true
			i++
		case c == ' ' || c == '\t':
			flush()
			i++
		default:
			cur.WriteByte(c)
			hasContent = true
			i++
		}
	}
	flush()
	return words
}
