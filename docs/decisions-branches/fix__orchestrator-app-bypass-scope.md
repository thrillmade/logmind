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

## 2026-08-15 19:52 - orchestrator-app: ask the forge which rules apply, rather than enumerating rulesets and counting

**Reasoning:** This is the fourth revision of one paragraph and the third wrong count, so the number was never the problem. A ruleset can exist, be active, require pull requests and name no bypass actor, and still match nothing at all: tremendous-machine's has an empty include list, so enumerating rulesets counted it as protecting a branch it does not apply to. The aggregate endpoint answers the question directly and shows that repository resolving to the same two organisation rulesets as logmind, which is the canonical accepting case. Thirteen accept a direct push and seven refuse one.

**Alternatives considered:** Correct the count a third time and keep enumerating. Rejected: two of the three previous errors came from that method, once by missing classic protection entirely and once by counting an inert ruleset. The document now leads with the aggregate query and keeps enumeration only for showing which actor holds the bypass, which is the one thing the aggregate does not report.

**Implications:**
- A second claim was wrong in a way that mattered more than the count. The blocked repositories were described as limited by required review; every pull-request rule in the organisation and the one classic protection require zero approvals, with no code-owner rule and no required status checks, while the App holds write access to pull requests. It can therefore open a request and merge it itself in every repository called blocked. Those seven slow the App down and none of them puts a person in the path, which the document now says instead of implying otherwise.

---

## 2026-08-15 20:05 - orchestrator-app: review IS required org-wide; the steward escapes it by bypassing the ruleset, not by its absence

**Reasoning:** My previous revision claimed every pull-request rule in the organisation required zero approvals and no code-owner review. That was read from one repository's own ruleset and generalised, which is the identical mistake to the count in the paragraph above it, made twice in the same document within an hour. Measured properly through the aggregate endpoint on three repositories: the organisation default-protection ruleset requires one approval and code-owner review everywhere, while the baseline ruleset requires neither.

**Alternatives considered:** Soften the sentence to say review is inconsistent across repositories. Rejected: it is perfectly consistent, and the inconsistency was in my sampling. The corrected paragraph is also a stronger statement of the risk rather than a weaker one, because the App escapes a requirement that exists rather than benefiting from one that was never set.

**Implications:**
- A human contributor does face review in every repository. The steward does not, because bypassing a ruleset bypasses its review requirement along with its push restriction, and it holds write access to pull requests, so it can open and merge one anywhere. Read with the finding that no ruleset in the organisation requires status checks, an App-authored merge passes no automated gate and no human one; the seven repositories that block its direct push change nothing about that.

---

## 2026-08-15 21:26 - orchestrator-app: say six and list six, and stop asserting a review gate the same file spends a paragraph refuting

**Reasoning:** Two defects, both mine, both introduced while correcting the previous pair. One sentence said seven repositories add their own ruleset and then listed six, so a reader adding the classic-protection case reached eight blocked and twelve accepting, which is the exact wrong split of the previous round re-encoded as a word instead of a numeral. The other left a clause asserting the App is gated by human review, three lines above the paragraph this change added to explain that it is not; the sentence containing it was rewritten in the same edit and the contradicting half survived.

**Alternatives considered:** Restate the totals in each section so every reader meets them. Rejected: this file has now been wrong five times and four of those were a second copy of a fact disagreeing with the first. The arithmetic is stated once, at the point where the two mechanisms combine, and every other mention refers to it rather than recomputing.

**Implications:**
- The exposure sentence now says the App can merge, not merely open, because bypassing a ruleset bypasses its review requirement along with its push restriction, and the bypass is unconditional rather than scoped to pull requests. What limits the blast radius is the token's lifetime and the App's permission set; neither installation scope nor review is doing that work, and the paragraph no longer implies otherwise.

---

