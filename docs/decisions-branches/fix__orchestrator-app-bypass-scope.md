← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-orchestrator-app-the-steward-bypasses-in-every-org-repo-not- -->
- **2026-08-15** — orchestrator-app: the steward bypasses in every org repo, not only the tap
<!-- logmind-entry-end -->

## 2026-08-15 14:33 - orchestrator-app: the steward bypasses in every org repo, not only the tap

**Reasoning:** The template v12 lane read this doc and concluded logmind's own regen push might be getting refused, which would have been a real blocker for dev reaching main. It is not: the doc was stale. Measured across all seven org repos, the two organization-level rulesets 18502737 org-baseline and 16898453 org-default-protection both list the steward App as a bypass actor with mode always, and both apply to every repo. The claim that the tap is the only repo with a bypass entry was presumably true when a repo-specific ruleset carried it and stopped being true when the entry moved to the org level, with nothing noticing.

**Alternatives considered:** Leave the sentence and let readers measure for themselves. Rejected: a doc that states a false fact about who can push to a protected branch is worse than one that says nothing, because the next lane to read it will draw the same wrong conclusion this one did, and the conclusion it invites is that a release path is blocked when it is not.

**Implications:**
- The correction carries the commands that produce it rather than only the answer, and names the three repo-specific rulesets that do not list the steward, so a reader can see the probe discriminates instead of matching everything. This also confirms issue 288's premise: the bypass logmind needs for regen-on-main is granted, and what remains untested is only whether a regen commit actually lands.

---

