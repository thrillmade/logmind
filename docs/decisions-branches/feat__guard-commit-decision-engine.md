← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-11-add-guard-commit-decision-engine-internal-guardcommit-logmin -->
- **2026-07-11** — Add guard-commit decision engine (internal/guardcommit) + logmind guard-commit cobra command, PR1/3 of force-logmind-usage enforcement
<!-- logmind-entry-end -->

## 2026-07-11 15:46 - Add guard-commit decision engine (internal/guardcommit) + logmind guard-commit cobra command, PR1/3 of force-logmind-usage enforcement

**Reasoning:** Enforcement needs one shared decision engine two future hook layers (git commit-msg, Claude Code harness PreToolUse) can both call; splitting the LOC-counting/decision-file logic out of check_decisions.go into internal/guardcommit avoids the two enforcement surfaces drifting on what counts as substantive. WorkingTreeUnion diff mode exists because the harness fires before a compound git add -A && git commit stages anything, so StagedOnly alone would silently bypass enforcement in that shape.

**Alternatives considered:** Have the harness hook call git diff --cached only (StagedOnly everywhere) - rejected, this is the exact bug WorkingTreeUnion prevents: a staged-only check run before staging happens always sees zero lines and allows., Return ErrSilent from guard-commit's harness layer like every other command - rejected, main.go maps ErrSilent to exit 1, but Claude Code's PreToolUse protocol only blocks on exit 2, so ErrSilent would make the harness layer a silent no-op.

**Implications:**
- guard-commit is Hidden:true and not wired into any hook yet - it is manually invocable only until PR2 installs the git commit-msg hook and the harness PreToolUse hook.
- Added git.enforce_commits (default true) and git.commit_line_threshold (default 20) to GitConfig as the repo-level off-ramp and threshold override; not yet exposed via logmind config list/get/set (DefaultMap untouched) to keep this PR's diff scoped to the decision engine.

---

## 2026-07-11 16:11 - Close five guard-commit silent-bypass holes found in PR #194 review: env-assignment prefix, subdir cwd for config+diffs, unicode untracked filenames, and second -m skip-marker

**Reasoning:** Rigorous review found a compliant agent could bypass the commit gate three ways. (1) An inline env-var prefix (FOO=1 git commit, GIT_AUTHOR_DATE=x git commit, HUSKY=0 git commit, env git commit) tokenized so words[0] != git and InvokesGitCommit returned false. (2) The harness evaluated Evaluate and loaded config from the payload cwd, which can be a subdirectory of the repo; git status/diff root-relative paths did not resolve from a subdir so untracked files were miscounted to zero, and enforce_commits/threshold were read from the wrong or missing config. (3) git status --porcelain octal-escapes non-ASCII paths so git diff --no-index could not open them, dropping unicode-named untracked files from the count. Plus extractSubjectHint only read the first -m, over-blocking a commit whose skip-marker was in a second -m.

**Alternatives considered:** Leave InvokesGitCommit as-is and rely only on the git-hook layer - rejected, the harness layer is the pre-stage gate and must catch env-prefixed and compound git add -A && git commit shapes before staging., Pass the raw payload cwd to Evaluate - rejected, that is the exact subdir bug; resolving the git toplevel once via gitcli.TopLevel and using it for BOTH config load AND every git op is the single correct fix., Strip quotes from git status --porcelain output for unicode paths - rejected, core.quotepath octal-escapes bytes not just quoting; -z NUL-terminated output is the only robust fix.

**Implications:**
- Added gitcli.TopLevel(cwd) (string, bool) returning the toplevel from an arbitrary cwd; guard-commit now resolves it once and passes it everywhere. UntrackedFiles switched to git status --porcelain -z. InvokesGitCommit stripWrapperPrefix now skips env assignments and the env command. extractSubjectHint collects all -m/--message values.
- The same-line staged+unstaged double-count in WorkingTreeUnion is left intentional (over-counts toward Block, the safe direction) with a code comment. Added regression tests for every hole: env-prefix table cases, untracked-only-from-subdir Block, unicode-untracked-from-subdir Block, enforce_commits:false-from-subdir Allow, and skip-marker-in-second-m Allow.

---

