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

## 2026-08-24 16:24 - gate + doctor: one Layout primitive both the writer and the gate route through; ask git where hooks live; treat inert gates as absent

**Reasoning:** The panel blocked this PR twice over. B1: the predicate matched the literal docs/decisions.md while the writer used filepath.Join(cwd,'docs'), so on a case-insensitive FS with a pre-existing Docs/ dir, or with logmind initialised below the git root, logmind log's own output became uncommittable — measured 65, now 0, with the decoys internal/x/decisions.md and internal/x/docs/decisions.md still 65. The shared-constants claim was false where it mattered: DocsDirName had one consumer and the writer never read it. There is now one Layout that owns the docs dir, resolved through the filesystem via os.SameFile rather than inferred from the platform. B3: doctor reported OK with core.hooksPath set (and doctor --fix wrote a hook git never reads, manufacturing a false OK), with an empty or chmod -x hook, and with a workflow gutted below an intact marker. Hooks now ask 'git rev-parse --git-path hooks'; inert is treated as absent.

**Alternatives considered:** Patch each call site and keep the constants. Rejected: a constant only one side reads is not a shared owner, and that is precisely how the docs-dir half drifted. Also rejected for F5: requiring the job to invoke 'logmind check-decisions', which wrongly reported this repo's own installed gate — the check is now 'has a pull-request trigger and a step that runs something'.

**Implications:**
- Stated limits. The read paths (Collect, timeline, search, context, pulse, skill) still join filepath.Join(cwd,'docs') and are not routed through Layout — no divergence today, but not closed. logmind log still resolves from cwd rather than the git root, so running it in a plain subdirectory still errors. init/self-update/install-hook still PRINT the literal .git/hooks/<name> even though they now write where git reads. The Claude PreToolUse guard's inertness is still presence-only. A workflow job that runs 'exit 0' is still not detectable. gate_absences is now [] on every path; the four sibling lists still serialize null. THIS ENTRY ALSO CORRECTS the earlier one on this branch, which is now false in two places: the predicate is built from a shared Layout, not shared constants, and hook rows are no longer suppressed in linked worktrees — worktrees now answer correctly.

---

