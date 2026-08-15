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

## 2026-08-15 16:11 - orchestrator-app: rulesets are not the only mechanism, and checking only them is how this section was wrong twice

**Reasoning:** The correction I made was itself wrong, in the same shape as the thing it corrected. I swept every repository's rulesets and concluded thirteen accept a direct push. A panel checked classic branch protection as well and found arlyn-working carries it with admin enforcement on, which nothing bypasses without an administration permission the steward does not hold. The real split is twelve accepting and eight blocked. Measuring one mechanism and reporting a total is the same defect as reading one copy of a fact and believing it.

**Alternatives considered:** State the ruleset finding and note that other mechanisms were out of scope. Rejected: this section is read to answer whether a push will land, and a number qualified by a methodology footnote will be quoted without the footnote. The section now names both mechanisms and shows the command for each, so the next audit cannot repeat the omission by accident.

**Implications:**
- Both probes are written into the document rather than only their answers. The steward's permission set is quoted, because the reason admin enforcement blocks it is that it holds contents, issues, metadata and pull requests and nothing else, which is checkable rather than assumed. This is the third revision of one paragraph; the first two were each internally consistent and each wrong against something they had not measured.

---

