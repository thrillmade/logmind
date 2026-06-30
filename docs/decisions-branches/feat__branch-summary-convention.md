← back to [docs/timeline.md](../timeline.md)

## 2026-06-29 20:46 - Slice 2 PR8: add clud-bug reviewContext (byte-parity / marker-injection / GITHUB_TOKEN / determinism); defer branch-summary convention wiring

**Reasoning:** The PR7 review caught a MAJOR marker-injection the clud-bug pass had only flagged as a watch-item. Encoding logmind's 4 load-bearing invariants as a trusted reviewContext (read from the base ref, injected into every hosted + local review) bakes the lessons in so every future PR checks them. The branch-summary AGENTS.md-template + canonical-skill wiring is DEFERRED: the convention is dormant until the v1.0 main-canonical default flip; changing the AGENTS.md template would bump the block-version and churn every repo's regenerated block for a dormant instruction; and the load-bearing canonical surface (the agent-skills logmind skill) is another agent's repo.

**Alternatives considered:** Bump the AGENTS.md slim/full templates + in-repo skill now — rejected: dormant convention + a block-version bump pushes churn to every repo's regenerated AGENTS.md; the canonical skill is cross-repo. It rides the v1.0 flip + agent-skills coordination, specced in PR9.

**Implications:**
- reviewContext added to .claude/skills/.clud-bug.json (4 invariants). Cross-repo follow-up: the canonical agent-skills logmind skill must flip its 'you do not manage this' line to 'maintain the branch summary via logmind headline'; the AGENTS.md templates get the convention at the v1.0 main-canonical flip.

---

