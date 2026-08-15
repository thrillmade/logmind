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

