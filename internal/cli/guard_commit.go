// guard_commit.go — `logmind guard-commit`, the cobra surface over the
// internal/guardcommit decision engine.
//
// guard-commit itself is deliberately Hidden (plumbing, not a
// user-facing verb) and does NOT install itself anywhere — no hook
// registration, no .git/hooks writes, no harness settings.json edits. It
// exists purely so the two hook layers have a single binary entry point
// to shell out to:
//
//   - `logmind guard-commit --layer git-hook --msg-file <path>` from the
//     commit-msg hook body (internal/hooks.BuildCommitMsgBody).
//   - `logmind guard-commit --layer harness` (JSON payload on stdin) from
//     the Claude Code harness's PreToolUse hook entry
//     (internal/claudehook.CanonicalCommand).
//
// Both layers ARE wired up automatically as of v2.0.0's enforcement
// PR2/3: `logmind init` and `logmind doctor --fix` install/refresh the
// commit-msg hook and the .claude/settings.json PreToolUse entry
// alongside the rest of the idempotent remediation pass (see
// internal/cli/refresh.go). guard-commit remains directly invocable too
// (e.g. for testing, or a hand-rolled hook setup) — this command doesn't
// care who calls it.
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

This command does not install anything itself — see 'logmind init' /
'logmind doctor --fix' for the commit-msg hook + Claude Code
PreToolUse guard installers that call it automatically. It's also
directly invocable, e.g. for testing:

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
				// other command in this CLI, but it is WRONG here — for
				// BOTH hook layers, as of the stale-binary hardening PR:
				//
				//   - harness layer: the Claude Code PreToolUse hook
				//     protocol only treats exit code 2 as "BLOCK this tool
				//     call" — exit 1 is non-blocking (the tool call
				//     proceeds as if nothing happened).
				//   - git-hook layer: the commit-msg hook body
				//     (internal/hooks.BuildCommitMsgBody) maps ONLY exit
				//     code 65 (EX_DATAERR) to its own `exit 1` abort. Any
				//     OTHER nonzero code — a stale-but-present logmind on
				//     PATH that doesn't know the `guard-commit` subcommand
				//     (an old 1.x Cobra binary's "unknown command" exit 1,
				//     or the frozen Python v0.6.16 argparse CLI's exit 2),
				//     a crash, a usage error — is deliberately treated as
				//     "not our block signal" and falls through to the
				//     hook's fail-open `exit 0`. Before this hardening the
				//     git-hook layer returned `cli.ErrSilent` here, which
				//     main.go turns into the SAME generic exit 1 a stale
				//     binary's "unknown command" error already produces —
				//     making the two indistinguishable and bricking every
				//     commit on any machine with a stale logmind ahead of
				//     the current one on PATH (including `logmind log`'s
				//     own internal commit). 65 was picked specifically
				//     because it is vanishingly unlikely to collide with
				//     any stale binary's own generic/crash exit code.
				//
				// If this branch returned an error instead of calling
				// os.Exit directly, guard-commit would silently degrade
				// into a no-op (harness) or an over-broad block (git-hook)
				// the moment main.go's blanket exit-1 mapping swallowed the
				// distinction, while still LOOKING like it worked (main.go
				// would print "Error: ..." and exit 1, easy to miss in hook
				// output). So: this is the one place in this command that
				// deliberately bypasses the ErrSilent/exit-1 convention and
				// calls os.Exit directly from inside RunE. Do not "fix"
				// this to `return ErrSilent`; that would reintroduce the
				// silent-bypass bug this comment exists to prevent.
				// runGuardCommit itself never calls os.Exit — only this
				// RunE wrapper does, which is what keeps both layers
				// unit-testable (see guard_commit_test.go) without killing
				// the test binary.
				//
				// Known accepted gap, NOT fixed by this PR: a stale
				// PYTHON logmind (the deprecated pre-Go pathway) on PATH
				// argparse-errors on the unrecognized `guard-commit`
				// subcommand with exit code 2 — the SAME code the harness
				// layer's PreToolUse contract treats as "block." Layer 1's
				// exit code is fixed by the external Claude Code harness
				// protocol, so it can't be made as distinctive as the
				// git-hook layer's 65 without breaking that contract. This
				// is accepted: the frozen Python CLI is a deprecated
				// pathway repos are expected to have moved off of, and an
				// escape hatch still exists (disable/edit the PreToolUse
				// entry in .claude/settings.json, or fix PATH so the
				// current binary resolves first) — unlike the git-hook
				// layer's pre-hardening bug, this doesn't brick EVERY
				// commit unconditionally, only Bash tool calls routed
				// through the harness.
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
// call os.Exit(exitCode) directly (see the LOUD COMMENT above) — now used
// by BOTH layers' block paths: harness blocks via exit 2 (the Claude Code
// PreToolUse contract), git-hook blocks via exit 65 / EX_DATAERR (the
// commit-msg hook's distinctive block signal — see the stale-binary
// hardening note in the LOUD COMMENT and in internal/hooks.BuildCommitMsgBody).
// Every other path returns exitCode 0 and lets `err` carry the result
// through the normal nil / plain-error convention: nil on allow (including
// every carve-out and the enforce_commits:false off-ramp), or a plain
// error for genuine misuse (bad --layer, missing --msg-file, an unreadable
// msg file).
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
		return guardCommitGitHook(repoRootFlag, msgFile, thresholdFlag, thresholdExplicit, quiet, stdout, stderr)
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

	// L2b — harness-layer restore (v2.0.0 derived-docs pin-preservation; see
	// internal/cli/derived.go). ONLY runs when this repo has opted into
	// `derived_docs: {mode: integration-point}` — see
	// internal/cli/derived.go's integrationPointMode; "driver" (the
	// default) never restores here. Deliberately runs BEFORE the
	// enforce_commits off-ramp below and independent of its outcome:
	// pin-preservation (derived_docs.mode) and commit enforcement
	// (git.enforce_commits) are separate knobs, and a repo that opted OUT
	// of enforcement may still want the zero-conflict invariant protected.
	//
	// Harness-layer ONLY — deliberately NOT added to guardCommitGitHook (the
	// commit-msg layer): commit-msg fires AFTER git has already built the
	// commit's tree from the index, so restoring files at that point can't
	// change what's being committed — it would be a confusing no-op. This
	// layer fires BEFORE git runs at all (Claude Code's PreToolUse guard
	// intercepts the Bash tool call itself), so it catches two things L2a's
	// pre-commit git hook can't: `git commit --no-verify` (which skips EVERY
	// git hook, including pre-commit) and a fresh clone (git hooks aren't
	// cloned — nothing under .git/ travels with `git clone` — but
	// .claude/settings.json is regular repo content that IS cloned).
	//
	// Silent + best-effort, like every other restore call site: the docs
	// regenerate deterministically from the committed decision files, so
	// there's nothing to protect by surfacing a failure here, and doing so
	// must never perturb the allow/block decision below.
	//
	// Restore target is HEAD, NOT the merge-base with the default branch
	// (v2.0.0 4b-ter reversal — same staleness reasoning as commitDecision's
	// L1 in log.go): the merge-base target depends on a freshly-fetched
	// refs/remotes/origin/<default>, which nothing on this harness-intercept
	// path ever refreshes, so it can be arbitrarily stale and would restore
	// to — and then let the commit carry — an OLDER snapshot than the
	// branch's true merge-base. Restoring to HEAD needs no such freshness:
	// it only depends on already-committed local state, so it's correct
	// offline and always. This surface's job is narrower than "repair a
	// diverged branch" — it only keeps an ALREADY-clean branch clean by
	// stopping a stray dirty working-tree copy from riding into the commit.
	// Repairing an already-diverged branch needs a trustworthy, just-fetched
	// origin/<default> — that's `logmind warp`'s job now (see runWarp in
	// warp.go), the one surface that fetches before it restores.
	if integrationPointMode(repoRoot) && onNonDefaultBranch(repoRoot) {
		_ = gitcli.RestorePathsToHead(repoRoot, derivedDocPaths...)
	}

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
// time a commit-msg hook runs), and translate the result into an
// (exitCode, err) pair.
//
// CTO design amendment (stale-binary hardening): the BLOCK path returns
// exit code 65 (EX_DATAERR — sysexits.h's "input data was incorrect",
// chosen here purely because it is a fixed, deliberately unusual code no
// well-behaved command uses for anything else) instead of the generic
// `cli.ErrSilent` (which main.go turns into exit 1 — indistinguishable
// from an old Cobra binary's "unknown command" exit, or a dozen other
// mundane failures). The commit-msg hook body
// (internal/hooks.BuildCommitMsgBody) checks for EXACTLY 65 before
// aborting the commit; every other nonzero code — including a STALE
// logmind on PATH that doesn't know the `guard-commit` subcommand at all
// — falls through to the hook's fail-open `exit 0`. Do not repurpose 65
// for anything else in this CLI; its entire value is being a signal nothing
// else produces by accident.
//
// Return shape: (0, nil) on allow — including every carve-out and the
// enforce_commits:false off-ramp; (65, nil) on block (the reason is
// already written to stderr); (0, plain error) for genuine misuse (an
// unreadable msg file) — the 0 here is deliberate: it keeps "we couldn't
// even evaluate this" distinct from "we evaluated it and it's blocked,"
// and main.go's normal plain-error-prints-and-exit-1 path handles it fine
// since exit 1 was never claimed as a git-hook signal.
func guardCommitGitHook(repoRootFlag, msgFile string, thresholdFlag int, thresholdExplicit bool, quiet bool, stdout, stderr io.Writer) (int, error) {
	evalCwd := evalCwdOr("", repoRootFlag)
	repoRoot, enforce, threshold := resolveRepoAndConfig(evalCwd, thresholdFlag, thresholdExplicit)
	if !enforce {
		return 0, nil // the repo off-ramp: git.enforce_commits: false
	}

	subject, err := firstLineOfFile(msgFile)
	if err != nil {
		return 0, err
	}

	d := guardcommit.Evaluate(repoRoot, subject, threshold, guardcommit.StagedOnly)
	q := newQout(quiet, stdout, stderr)
	if d.Allow {
		reason := string(d.CarveOut)
		if reason == "" {
			reason = "not a git repository"
		}
		q.chat("✓ guard-commit: allowed (%s)\n", reason)
		return 0, nil
	}
	fmt.Fprintln(stderr, d.Reason)
	return exGitHookBlock, nil
}

// exGitHookBlock is the git-hook layer's distinctive block exit code —
// EX_DATAERR from BSD's sysexits.h. See guardCommitGitHook's doc comment
// for why a fixed, unusual code (not the generic ErrSilent/exit-1) is
// required here.
const exGitHookBlock = 65

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
