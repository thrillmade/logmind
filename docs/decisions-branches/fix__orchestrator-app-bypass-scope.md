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

## 2026-08-15 15:15 - orchestrator-app: rulesets aggregate, so the org-level bypass alone does not decide exemption

**Reasoning:** My previous correction replaced one over-claim with another. It was right that the steward bypasses two organisation-level rulesets applying to every repository, and wrong to conclude that a direct push is therefore exempt anywhere. Bypass is evaluated per ruleset and rulesets aggregate, so a repository that additionally carries its own pull-request rule without naming the steward still blocks it. Seven do. The original sentence I called false was true under its natural reading: the tap is the only repository whose own ruleset names the steward.

**Alternatives considered:** Revert to the original sentence. Rejected, because it was true but incomplete in a way that mattered: read alongside the risk paragraph it produced a materially wrong picture of blast radius. The fix is to state the aggregation rule that makes both facts consistent, not to choose between them.

**Implications:**
- The load-bearing copy of this fact sat in the installation-scope section and said every other repository rejects a direct push from the App, which understated exposure. Measured across all twenty repositories rather than the seven previously recited: thirteen accept a direct push, including logmind, protocol, skdd and reporulez, and seven are blocked. The residual-risk paragraph said one repository is exposed under key compromise; it is thirteen. Both copies now derive from the same measurement, and the tap is noted as the only default branch that has ever carried a direct steward commit, so for every other repository the exemption is inferred from configuration and never exercised.

---

