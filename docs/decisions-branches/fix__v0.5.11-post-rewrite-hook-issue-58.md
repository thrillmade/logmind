## 2026-05-30 09:19 - feat(v0.5.11): post-rewrite hook so multi-commit rebases don't leave timeline.md stale (#58)

**Reasoning:** Pre-fix: merge driver in .gitattributes only fires when a merge produces conflicts on derived files; post-merge hook only fires on merges. Neither covers git rebase or git commit --amend. Multi-commit rebase replayed all commits but only the FIRST commit's regen survived — subsequent commits left docs/timeline.md stale relative to the replayed docs/decisions-branches/<branch>.md entries. check-derived-docs failed. Hit live on agent-skills PRs #21 + #22 in the 2026-05-27 merge cascade.

**Alternatives considered:** make merge driver re-fire on every merge_file call — rejected, driver layer is the wrong abstraction for 'rebase touched N commits' (driver is per-conflict not per-commit), remove timeline.md from check-derived-docs gate — rejected, defeats the entire point of the gate, ship as part of v0.5.12 with auto-resolve work — rejected, user wants small one-at-a-time ships (feedback_logmind_for_fix_commits memory says ship substantive fixes individually)

**Implications:**
- post-rewrite hook is per-clone (lives in .git/hooks/, not committed); logmind init installs it the same way it installs post-merge. Fresh clones still need init — auto-resolve-on-fresh-clone is the v0.5.12 work captured in memory project_timeline_conflict_should_auto_resolve.
- Same idempotency contract as post-merge hook: refuses to overwrite foreign user-authored hook; logmind doctor surfaces drift; logmind init re-installs canonical body.
- doctor.collect_status() now reports 'post-rewrite hook' drift alongside 'post-merge hook' — same drift semantics.

---
