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

