← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-11-wire-commit-enforcement-live-claude-code-pretooluse-guard-in -->
- **2026-07-11** — Wire commit enforcement live: Claude Code PreToolUse guard (internal/claudehook) + upgrade commit-msg hook to enforce + init/doctor --fix install wiring (enforcement PR2/3)
<!-- logmind-entry-end -->

## 2026-07-11 16:47 - Wire commit enforcement live: Claude Code PreToolUse guard (internal/claudehook) + upgrade commit-msg hook to enforce + init/doctor --fix install wiring (enforcement PR2/3)

**Reasoning:** PR1 shipped guard-commit as a manually-invocable, Hidden decision engine with zero install wiring. This PR makes enforcement LIVE: Layer 1 is a new internal/claudehook package that surgically merges a PreToolUse guard entry into .claude/settings.json (marker-based ownership via a logmind guard-commit substring in hooks[].command, mirroring hooks.go's git-hook marker convention), so a non-compliant git commit Bash tool call is blocked by the Claude Code harness before it ever runs. Layer 2 upgrades BuildCommitMsgBody from warn-only to delegate to logmind guard-commit --layer git-hook, so the existing hookVersion() marker auto-upgrades every consumer's installed hook on the next init/doctor --fix with no new install-side code. Both layers are wired into logmind init (fresh install + refresh mode) and logmind doctor --fix via a new refreshOpts.claudeAgentEnabled gate, resolved from the agents flag list at init time and from .logmind/config.yml at doctor --fix time (default true, matching agents.DefaultEnabled). Added probeClaudePreToolUseHook to internal/doctor mirroring probeHook's missing/markerless/stale/current shape.

**Alternatives considered:** Gate the Layer 1 install on git-repo presence like the git hooks - rejected: .claude/settings.json is repo content, not git-clone state, so it must install under --no-git too (and a repo that adds git later should already be protected)., Flip doctor Overall to DRIFT whenever the Claude probe is missing, not just when stale - rejected: every existing hook probe treats missing as benign (pinned by TestCollectStatus_FreshRepoListsAllProbes asserting Overall==OK on an all-missing fresh repo); the new probe reuses the exact same classifyLogmindDrift mechanism so only stale flips, for consistency., Add a command -v logmind existence guard to the harness's canonical PreToolUse command, mirroring the git hook - rejected: the harness command must stay one cross-platform string valid under both bash and PowerShell, and a missing binary already fails open via a non-2 command-not-found exit, which is the correct behavior.

**Implications:**
- New internal/claudehook package: EnsurePreToolUseGuard is idempotent and non-destructive, replacing only the hook object whose command contains the logmind guard-commit marker, appending into an existing Bash PreToolUse group or creating one, and leaving every foreign hook/group/event untouched; Inspect() gives doctor a read-only probe of the same state.
- config.yml.template documents git.enforce_commits/commit_line_threshold (struct defaults already matched these, so config list/DefaultMap are untouched, avoiding golden churn); git.enforce_commits:false and agents.claude:false remain the two full off-ramps.
- New BuildCommitMsgBody golden (testdata/commit-msg.golden) plus a real git commit integration test in internal/hooks proving the installed hook actually blocks a substantive no-decision commit and allows via [skip-logmind], LOGMIND_ALLOW_GIT_COMMIT=1, and a staged decision file.

---

