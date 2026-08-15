← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-14-make-docs-roadmap-md-the-source-of-truth-for-sequencing -->
- **2026-08-14** — Make docs/roadmap.md the source of truth for sequencing
<!-- logmind-entry-end -->

## 2026-08-14 17:02 - Make docs/roadmap.md the source of truth for sequencing

**Reasoning:** The roadmap lived in a scratchpad HTML file that the artifact was published from. That file was wiped twice today by session restarts, taking the only copy of the sequencing with it — the published page survived but could not be updated, so it drifted to showing 26 issues and a superseded critical path while the real count was 31 and the critical path had changed to #288. A roadmap that cannot survive a restart is not a source of truth. It now lives in the repository, versioned and reviewable, and the artifact renders FROM it rather than being it.

**Alternatives considered:** Keep the roadmap inside docs/plan.md. Rejected on half-life: plan.md is architecture and changes yearly; the release board changed six times in one day. Fusing them is exactly why plan.md spent weeks describing a derived_docs.mode config gate that had already been deleted.

**Implications:**
- One owner per fact, stated in both files: if a date, count, issue number or ordering appears in both, roadmap.md is right and plan.md is stale. plan.md is retitled from 'Plan & Architecture' to 'Architecture' to stop implying it owns sequencing. Every number in roadmap.md carries the command that produced it, and every claimed zero carries a control proving the probe finds a non-zero when one exists — including the load-bearing one that regen bot commits have a null author.login, so a login-keyed probe reports zero on every PR and looks clean. All seven SPEC citations verified LIVE against blob cd64e5c; check-links passes.

---

## 2026-08-14 21:24 - Correct seven defects an adversarial panel found in the roadmap

**Reasoning:** The file whose entire purpose is being accurate had seven. Two drove what gets built next. FIRST: it said the steward App was on no ruleset bypass list and called that the tightest constraint on the tag — measured now, there are two active rulesets not three, reporulez-default 18292242 returns 404, and both survivors carry Integration:3951953 which IS skdd-steward. Their updated_at is 2026-08-07T17:1x, a week before that commit, so it was likely never true when written. I had verified the bypass was granted and said so out loud, then wrote the opposite into the file from older survey text. SECOND: 'seven success runs, the single push event failed' — actually 503 runs, 15 on push, 13 of them green. The conclusion survived, the evidence did not, and an auditor who finds thirteen greens discounts the section that is right.

**Alternatives considered:** Fix the two critical ones and file the rest. Rejected: five of the seven are the file describing a state that is not the current one, which is the single failure mode it exists to avoid. A roadmap that is 70% accurate is worse than none, because it is trusted.

**Implications:**
- The #288 section is rewritten around what is actually unresolved: the bypass is granted, the push has still never landed a commit, and the reason is not a refusal — 13 green push runs with 0 commits means the job finds the docs current and exits before pushing. The one failure, on 0aa9049, predates the bypass by six days, so the original diagnosis was right when filed and the remedy has since been applied. skdd was misread across two branches: main has no workflows at all, dev carries check-decisions v4, and the earlier 'already on v11, migrating is a no-op' would have dropped a repo that genuinely needs migrating. One-owner-per-fact was claimed and not achieved — AGENTS.md, README twice and docs/spec.md all still pointed at plan.md for the roadmap, so all four are repointed. #243 carried its own ordering table that reversed this file's, recreating the two-owners defect one revision after the split; its body is rewritten to point here. Four pre-existing issues were unplaced and now are. The verified date was impossible — 2026-08-07 against a SHA created 2026-08-14.

---

## 2026-08-15 09:18 - roadmap: state the command, not the answer, for every fact git already owns

**Reasoning:** This file went stale twice, the second time during the adversarial panel that was checking it, which blocked it. The cause was structural rather than careless: the file copied SHAs, commit counts, ahead-counts and open-PR counts that git and gh already own, and a hand-kept second copy reads as true until one quietly is not. That is the one-owner-per-fact rule the file exists to enforce, and it was not being applied to the file itself. The State section and the landed-commits table now carry the commands that produce those numbers instead of the numbers.

**Alternatives considered:** Correct the numbers a third time and keep the shape. Rejected because the failure repeats on a timescale shorter than the review that catches it. The nine findings the panel raised were almost all instances of one defect wearing nine faces, and correcting instances leaves the tenth to be found by a reader who trusts it.

**Implications:**
- Numbers that survive are load-bearing for a judgment rather than descriptive of the moment, and each is stamped with the date it was measured and the command that measures it again. Also corrected in this pass, all re-measured against live gh: the protocol bot-commit share is 43 of 109 on the last ten merged pull requests rather than 42 of 98, there are two push failures rather than one, skdd carries regen-timeline at v11 rather than nothing, check-decisions has no push trigger at all, and agent-skills 207 is merged.

---

## 2026-08-15 13:53 - roadmap: make each shown command reproduce its own number, and drop a claim that was false when written

**Reasoning:** The re-panel blocked on two defects. Both gh api calls over a pull request commit list omitted --paginate, so the command printed beside the number answered a different question than the number: PR 75 carries 63 commits over three pages, and without the flag gh returns only the first, which counts 15 against a stated 30. A command shown to make a number self-verifying is worse than no command when it does not reproduce it. Separately, the file asserted that issue 244 still contradicted Ruling 12 in its body. That body was corrected on 2026-08-01, two weeks before the claim was written, and I had verified it myself earlier in the same session and left the stale sentence standing.

**Alternatives considered:** Drop the commands and keep only the prose numbers. Rejected because the command is the whole mechanism by which this file avoids the staleness that blocked it twice; the answer is to make the commands correct rather than to remove the check. Also rejected: leaving the 244 paragraph as a harmless overstatement, since a roadmap that invents remaining work is exactly as misleading as one that hides it.

**Implications:**
- Both aggregate and single-PR probes now carry --paginate, and re-running them yields 109 commits with 43 from the regen bot, a 39.4 percent share, matching what the file states. The control confirms the probe distinguishes bot from human rather than matching everything.

---

## 2026-08-15 14:12 - roadmap: remove the branch-tip pin from the header — the file's own rule, broken in the file's own header

**Reasoning:** The fourth review found the verification header naming the dev and main tips it was checked against, two paragraphs above the rule forbidding exactly that. Worse, the pin was already wrong when written: the commit introducing it is timestamped three minutes after PR 302 moved dev past the SHA it names. A rule stated and then broken in the same document teaches readers the rule is decorative.

**Alternatives considered:** Update the header to the current tip. Rejected for the fourth time on the same grounds: any branch tip written into this file is wrong within hours, and the previous three corrections all proved it. The SPEC blob pin stays because a blob hash is immutable and cannot rot; a branch tip is a moving reference that this file may not own.

**Implications:**
- The header now pins only the SPEC blob and tells the reader to run git rev-parse for the tips. The absence is documented rather than silent, so the next person does not helpfully add it back. A grep for a dev or main tip in the file returns nothing, controlled against the two immutable commit references in the historical sections, which are allowed and still present.

---

