## 2026-06-02 09:27 - feat(v0.6.15): post-merge hook skips regen on default branch (tokenomics-agent feedback)

**Reasoning:** v0.6.14 verification cycle on tokenomics surfaced that the post-merge hook still leaves docs/timeline.md + docs/file-structure.md unstaged on main after every squash-merge. The hook regens locally, but the result differs from origin/main (the squash compressed history that the local regen reads from). Users had to git checkout -- docs/ after every merge. Fix: hook exits 0 when current branch = refs/remotes/origin/HEAD's symbolic ref (default branch). regen-timeline.yml already handles main server-side; local main converges on next pull. Trade-off: lose 'immediate local view of just-merged entry' but gain 'main matches origin/main after every merge' — the latter is what users expect.

**Alternatives considered:** Pre-merge regen via server-side workflow committing to PR branch — invasive (extra commit per PR, PAT, CI time); deferred to v0.7.x, Auto-amend the merge commit locally — surprising; rewrites history mid-pull, Document the cleanup step — doesn't actually solve 'main should match origin/main'

**Implications:**
- Closes the v0.6.x post-merge hook saga at its FINAL endpoint: hook never causes user-visible state divergence on main
- Local main is now ALWAYS clean after a merge cycle; the regen catches up on next pull
- v0.7.x candidate: re-evaluate server-side pre-merge regen if some users want immediate-view behavior

---
