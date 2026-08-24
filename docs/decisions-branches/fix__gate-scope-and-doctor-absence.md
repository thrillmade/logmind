← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-24-guard-commit-scope-the-decision-file-predicate-to-the-two-re -->
- **2026-08-24** — guard-commit: scope the decision-file predicate to the two real layouts; doctor: report absent enforcement gates as DRIFT
<!-- logmind-entry-end -->

## 2026-08-24 14:20 - guard-commit: scope the decision-file predicate to the two real layouts; doctor: report absent enforcement gates as DRIFT

**Reasoning:** Two independent gate holes. B1: isDecisionFile matched any path ending '/decisions.md', so a well-formed entry at internal/x/decisions.md cleared the commit gate — measured exit 0 on dev, exit 65 now, with a must-pass control (a real entry in docs/decisions-branches/main.md) staying exit 0 on both. The predicate is now the exact image of resolveDecisionsPath, built from shared layout constants. B3: doctor excluded 'missing' and 'markerless' from DRIFT, so a repo with all three enforcement surfaces deleted reported Stack status OK exit 0 — SPEC 3.4 says failing open MUST NOT be silent. GateAbsences is now the only non-advisory list and flips Overall.

**Alternatives considered:** For B1, a suffix rule with a denylist — rejected: it enumerates what to reject rather than what to accept, and the next unanticipated layout reopens the hole. For B3, a second config key to opt out of absence reporting — rejected: git.enforce_commits already means 'logmind does not gate commits here' to guard-commit, the config template and AGENTS.md; a second key is a second owner for one fact. Cost accepted: local-off/CI-on is not expressible.

**Implications:**
- The rename/copy half of #335 is NOT closed and this predicate cannot close it: a pathspec limits git's tree walk before rename detection, so a staged rename renders as new-file with every line added. Documented in situ at guardcommit.go with both candidate mechanisms and their costs — --find-copies-harder silently disables past diff.renameLimit, which is fail-open under load and SPEC 3.4 forbids it. Named as a design fork, not picked. B3 is scoped to three surfaces and suppressed in linked worktrees, where .git is a file and probeHook cannot see the shared hooks dir.

---

