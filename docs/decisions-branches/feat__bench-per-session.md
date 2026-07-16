<!-- logmind-entry-start: 2026-05-29-implement-bench-per-session-py-q7-logmind-s-4th-angle-now-re -->
- **2026-05-29** — Implement bench/per_session.py — Q7-logmind's 4th angle now real, surfaces 0.B.5/0.B.6 decision data
<!-- logmind-entry-end -->

## 2026-05-29 11:42 - Implement bench/per_session.py — Q7-logmind's 4th angle now real, surfaces 0.B.5/0.B.6 decision data

**Reasoning:** bench/per_session.py was a stub returning net_pct=None — the 4-angle Q7-logmind gate was missing its key informational angle, and the conditional 0.B.5 (decisions.md per-entry compact) + 0.B.6 (AGENTS.md logmind-block trim) candidates had no data to decide on. Implementation walks ~/.claude/projects/*/*.jsonl (depth-2, last 30 days, cap 50), filters to repos with .logmind/config.yml, joins tool_use:Read events to tool_result via tool_use_id, buckets read bytes into docs/decisions.md / docs/timeline.md / docs/file-structure.md / AGENTS.md, plus an AGENTS.md-logmind-block sub-bucket via <!-- logmind-start/end --> marker scan. Per-session net_pct compares against git log --oneline -100 baseline. New PerSessionResult fields: sessions_sampled, sessions_with_decision_reads, per_file_share, agents_md_block_share, detail. __main__.py treats per-session as INFORMATIONAL (doesn't gate exit) because the git-log baseline is conceptually too thin — agents wouldn't get equivalent context from raw git log alone, so absolute pct isn't a quality signal. The per-file shares ARE the load-bearing data that gates 0.B.5/0.B.6 downstream. _session_cwd scans first 50 lines for cwd field (not always on line 1 — early events are session boot/snapshot metadata). 5 new bench tests in tests/test_bench.py: stub-invariant-when-no-sessions (renamed), no-logmind-repos, fixture-aggregation, zero-decision-reads, malformed-line-skipped. 15/15 bench tests pass.

**Alternatives considered:** Use a heavier git baseline (e.g. git log --format=full -100 with commit messages) to make net_pct interpretable. Rejected: even with messages, raw git is still not the no-logmind equivalent — agents would derive context differently (read more code, run more git commands). The honest answer is the baseline is fuzzy; mark the angle informational and use the SHARES (which are exact)., Implement per-session as 0.B.4 candidate before the per-session work itself. Rejected: that's circular — the candidate was 'implement per-session' itself, and the data we need to decide 0.B.5/0.B.6 comes FROM that implementation. Calling it Step 1 of the active plan instead.

**Implications:**
- Real data measured on this machine: 14 sessions sampled (logmind repos), 9 with decision-doc reads. per_file_share: docs/decisions.md=39%, AGENTS.md=36%, timeline=15%, file-structure=11%. agents_md_block_share=51%. Mean decisions.md per session-with-reads = 6017 bytes. Both 0.B.5 and 0.B.6 EASILY clear the rubric thresholds from the plan — next two PRs ship the trims.
- Org-cumulative angle is still a stub but its blocker (per-session real impl) is now lifted — implementing org-cumulative is a follow-up Phase 0.5 polish PR, not blocking on this plan's ship work.

---
## 2026-05-29 11:50 - PR #78 fix: empty-git-baseline must not count session-with-reads; clarify inline comment on AGENTS.md block fallback

**Reasoning:** clud-bug-review caught two issues. (1) Critical: sessions_with_reads += 1 ran BEFORE the git_bytes == 0 guard, so a session with decision reads but an empty git baseline (fresh git init, no commits) would fall through to the aggregate as 'stub=False, net_pct=0.0' — a fake 'break-even' verdict instead of 'no usable measurement'. Moved the increment AFTER the guard. (2) Nit: inline comment in _agents_md_block_bytes said 'report 0 to keep the metric honest' but the code below returned a non-zero estimate. Rewrote to match actual behavior (heuristic estimate for older installs is better than muting the metric).

**Implications:**
- Added test_per_session_empty_git_baseline_does_not_count_session as a regression pin — fresh git init + AGENTS.md read = stub/None, not break-even. 16/16 bench tests pass.

---
