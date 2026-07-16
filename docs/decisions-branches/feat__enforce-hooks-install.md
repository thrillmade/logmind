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

## 2026-07-16 16:02 - Review fixes for PR 195: doctor --fix degrades gracefully on malformed .claude/settings.json, self-update installs the PreToolUse guard, commit-msg guard uses an explicit if

**Reasoning:** Review verdict was SHIP-after-one-fix. M1: applyRefresh fed claudehook's error into firstErr, and runDoctorFix converts any refreshErr into ErrSilent/exit-1 with the ok summary suppressed - so a consumer with a JSONC-style trailing-comma settings.json got a persistently failing doctor --fix that also skipped the branch-summary backfill and residual re-probe over a file --fix cannot repair anyway. The sibling git-hook installers deliberately swallow their errors so a bad hook degrades to residual drift; the claudehook install now does the same, and the doctor probe already classifies an unparseable settings.json as missing (benign). m1: self-update refreshed the three git hooks but never installed Layer 1, so a repo whose only refresh path is self-update would get an enforcing commit-msg hook while the PreToolUse guard stayed missing forever with no doctor nudge. m2: the one-liner msg-file guard relied on shell operator precedence; replaced with an explicit if block per review.

**Alternatives considered:** Have runDoctorFix special-case claudehook errors instead of swallowing in applyRefresh - rejected: applyRefresh is the shared choke point (init refresh-mode hits the same path), and the established convention there is per-installer error swallowing for user-content surfaces., Surface the malformed-settings condition as a stderr warning from applyRefresh - rejected for now: the doctor probe reports the state and keeping the swallow symmetric with the git-hook installers is the smaller, convention-matching change.

**Implications:**
- New tests: doctor --fix with malformed settings.json exits 0, prints ok doctor-fix with claude-hook=current, still installs the git hooks, and leaves the file byte-untouched; self-update installs the guard by default and skips it under agents.claude:false. commit-msg.golden regenerated for the explicit-if body; the hookVersion marker means consumers auto-upgrade on their next refresh as before.

---

