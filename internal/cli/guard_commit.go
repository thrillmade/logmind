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

// runGuardCommit is the testable core shared by both layers. It resolves
// config (git.enforce_commits, git.commit_line_threshold) once against
// repoRootFlag-or-cwd, then dispatches to the layer-specific evaluation.
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
	repoRootForConfig := repoRootFlag
	if repoRootForConfig == "" {
		cwd, wdErr := os.Getwd()
		if wdErr != nil {
			return 0, wdErr
		}
		repoRootForConfig = cwd
	}

	// config.Load degrades to defaults on a missing/unparseable
	// .logmind/config.yml (see internal/config's documented "broken
	// config is user-fixing-it-later" stance) — guard-commit inherits
	// that same best-effort posture rather than treating a config
	// problem as a reason to block commits.
	cfg, _ := config.Load(repoRootForConfig)
	threshold := resolveThreshold(cfg, thresholdFlag, thresholdExplicit)

	switch layer {
	case "harness":
		if !cfg.Git.EnforceCommits {
			// The repo off-ramp: exit 0, no output, for both layers.
			return 0, nil
		}
		return guardCommitHarness(stdin, stderr, repoRootFlag, threshold), nil
	case "git-hook":
		if !cfg.Git.EnforceCommits {
			return 0, nil
		}
		if msgFile == "" {
			return 0, fmt.Errorf("--msg-file is required for --layer git-hook")
		}
		return 0, guardCommitGitHook(repoRootForConfig, msgFile, threshold, quiet, stdout, stderr)
	default:
		return 0, fmt.Errorf("--layer must be %q or %q (got %q)", "harness", "git-hook", layer)
	}
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
// that isn't a git-commit invocation), or 2 to block (with the reason
// already written to stderr).
func guardCommitHarness(stdin io.Reader, stderr io.Writer, repoRootFlag string, threshold int) int {
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

	repoRoot := payload.Cwd
	if repoRoot == "" {
		repoRoot = repoRootFlag
	}
	if repoRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			repoRoot = cwd
		}
	}

	subject := extractSubjectHint(payload.ToolInput.Command)
	d := guardcommit.Evaluate(repoRoot, subject, threshold, guardcommit.WorkingTreeUnion)
	if d.Allow {
		return 0
	}
	fmt.Fprintln(stderr, d.Reason)
	return 2
}

// guardCommitGitHook implements the --layer git-hook evaluation: read the
// commit subject from msgFile's first line, evaluate under StagedOnly (the
// index is final by the time a commit-msg hook runs), and translate the
// result into the standard nil / ErrSilent convention.
func guardCommitGitHook(repoRoot, msgFile string, threshold int, quiet bool, stdout, stderr io.Writer) error {
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

// extractSubjectHint does a best-effort, quote-aware scan for a
// `-m <value>` / `-m<value>` / `--message <value>` / `--message=<value>`
// flag in a raw shell command string. It exists solely so the harness
// layer's [skip-logmind] carve-out can see the intended commit message
// BEFORE git has recorded anything (the harness runs at PreToolUse, ahead
// of the actual `git commit` invocation). This is not a shell parser: a
// message that comes from $EDITOR, a template, or is buried inside a
// substitution simply yields "" here, which just means the
// [skip-logmind] carve-out won't apply — every other carve-out (env var,
// decision-file-staged, under-threshold) still works normally.
func extractSubjectHint(command string) string {
	words := splitCommandWords(command)
	for i, w := range words {
		switch {
		case w == "-m" || w == "--message":
			if i+1 < len(words) {
				return words[i+1]
			}
		case strings.HasPrefix(w, "--message="):
			return strings.TrimPrefix(w, "--message=")
		case strings.HasPrefix(w, "-m") && w != "-m":
			// git's short-flag-with-attached-value form, e.g. `-mSubject`.
			return strings.TrimPrefix(w, "-m")
		}
	}
	return ""
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
