## 2026-06-02 09:27 - feat(v0.6.15): post-merge hook skips regen on default branch (tokenomics-agent feedback)

**Reasoning:** v0.6.14 verification cycle on tokenomics surfaced that the post-merge hook still leaves docs/timeline.md + docs/file-structure.md unstaged on main after every squash-merge. The hook regens locally, but the result differs from origin/main (the squash compressed history that the local regen reads from). Users had to git checkout -- docs/ after every merge. Fix: hook exits 0 when current branch = refs/remotes/origin/HEAD's symbolic ref (default branch). regen-timeline.yml already handles main server-side; local main converges on next pull. Trade-off: lose 'immediate local view of just-merged entry' but gain 'main matches origin/main after every merge' — the latter is what users expect.

**Alternatives considered:** Pre-merge regen via server-side workflow committing to PR branch — invasive (extra commit per PR, PAT, CI time); deferred to v0.7.x, Auto-amend the merge commit locally — surprising; rewrites history mid-pull, Document the cleanup step — doesn't actually solve 'main should match origin/main'

**Implications:**
- Closes the v0.6.x post-merge hook saga at its FINAL endpoint: hook never causes user-visible state divergence on main
- Local main is now ALWAYS clean after a merge cycle; the regen catches up on next pull
- v0.7.x candidate: re-evaluate server-side pre-merge regen if some users want immediate-view behavior

---
## 2026-06-02 12:32 - fix(v0.6.15): logmind log regens derived docs before committing

**Reasoning:** Caught when v0.6.15's own PR failed check-derived-docs — the tokenomics agent's confusion about 'timeline should just automatically work' was a real bug, not a misunderstanding. Every logmind log produced a commit with a fresh decision file but stale docs/timeline.md + docs/file-structure.md. The check-derived-docs CI workflow then failed on every PR. Users were silently filing broken PRs. Fix: log_cmd calls update_file_structure + write_timeline BEFORE log_decision, gated on config.auto_update_file_structure (default true). The regen output gets staged via the same --stage all default as the decision file. Best-effort try/except shape matches the post-merge hook's pattern: if regen fails (rare), the log still completes.

**Alternatives considered:** Add a pre-commit hook that runs the regen — works but invisible to logmind itself, can be bypassed with --no-verify, Document the manual regen step — defeats the 'one command per decision' promise that defines logmind, Only regen when --regen flag passed — opt-in puts the burden on users who already trust the tool

**Implications:**
- Closes the silent footgun where every logmind log call produced a broken PR. Tokenomics agent's surprise was correct
- logmind log is now genuinely 'one command per decision' — derived docs always travel with the decision file
- Users on file_structure.auto_update: false keep the old behavior; nothing breaks for opted-out users

---
## 2026-06-02 12:32 - fix(v0.6.15): logmind log regens derived docs before committing

**Reasoning:** Caught when v0.6.15's own PR failed check-derived-docs. The tokenomics agent's 'timeline should just automatically work' surprise was a real bug — every logmind log committed a fresh decision file but stale docs/timeline.md + docs/file-structure.md. The check-derived-docs CI workflow then failed on every PR, silently filing broken PRs. Fix: log_cmd calls update_file_structure + write_timeline BEFORE log_decision; gated on config.auto_update_file_structure (default true). Best-effort try/except shape matches the post-merge hook pattern.

**Alternatives considered:** Pre-commit hook for regen — invisible to logmind, can be bypassed with --no-verify, Document manual regen — defeats the 'one command per decision' promise, Only regen when --regen flag — opt-in burdens users who trust the tool

**Implications:**
- Closes the silent footgun where every logmind log produced a broken PR
- logmind log is now genuinely 'one command per decision' — derived docs always travel with the decision
- config.auto_update_file_structure: false keeps the old behavior; nothing breaks for opted-out users

---
