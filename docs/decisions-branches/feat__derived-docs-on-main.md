← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-adopt-derived-docs-on-main-plan-one-invariant-branch-derived -->
- **2026-07-17** — Adopt derived-docs-on-main plan: one invariant (branch derived docs == merge-base) enforced by L0 hooks/L1 log/L3 CI gate, plus warp/context/pulse freshness
<!-- logmind-entry-end -->

## 2026-07-17 20:16 - Adopt derived-docs-on-main plan: one invariant (branch derived docs == merge-base) enforced by L0 hooks/L1 log/L3 CI gate, plus warp/context/pulse freshness

**Reasoning:** The derived docs are pure functions of the decision files, so a branch never needs to edit them; keeping them byte-identical to the merge-base makes git 3-way merge conflict-free with no merge-driver dependency, which is the only thing that works on GitHub servers

**Alternatives considered:** Merge-driver-only auto-resolution (rejected: GitHub cannot run logmind git merge-driver, so PRs still show conflicts), Detect-and-fix conflicts after they occur (rejected: conflicts burn agent tokens; the bar is structurally impossible not merely rare)

**Implications:**
- warp is the only network caller; logmind log hot path stays network-free; the CI check-derived-docs job becomes blocking and existing derived-doc-editing PRs must revert

---

## 2026-07-17 20:26 - Foundation: gitcli MergeBase/Fetch/ShowFile/RestorePathsToHead + shared derivedDocPaths/onNonDefaultBranch

**Reasoning:** Thin git-wrapper helpers and the single definition of the two governed docs plus the non-default-branch predicate, consumed by L1 log, warp, the pulse probe, and context refresh

**Implications:**
- Fetch is the only network helper and is never called from the log hot path; onNonDefaultBranch is conservative-false so non-repo/detached/default paths keep pre-v2.0.0 behavior

---

## 2026-07-17 20:32 - L1: logmind log restores derived docs to HEAD before staging on a non-default branch

**Reasoning:** Primary defense of the zero-conflict invariant on the commit path — even if a hook or manual edit dirties timeline.md/file-structure.md, the branch commit never carries the divergent copy; applies in both stage=all and scoped so a dirty derived doc can never leak, and touches only the two derived docs, never the branch decision file or its marker

**Alternatives considered:** Pathspec-exclude from git add (rejected: leaves a dirty working tree a later raw git add could sweep)

**Implications:**
- No-op on the default branch (main stays current) and lossless everywhere since the docs regenerate from the committed decision files

---

## 2026-07-17 20:45 - L0: post-merge and post-rewrite hooks regenerate derived docs on the default branch only

**Reasoning:** A rebase/amend/merge on a feature branch previously regenerated (post-rewrite even git-added) the derived docs, diverging the branch from its main merge-base; gating regen to the default branch keeps branches clean so merges never conflict. Adversarial panel confirmed no feature-branch regen path; test strengthened to pin the full guard against an operator-precedence bug

**Alternatives considered:** Rely on the merge driver alone (rejected: GitHub cannot run it, so PRs still conflict)

**Implications:**
- Over-gating when origin/HEAD is unset and the default is not literally main is freshness-only never a conflict, and CI regen-on-main is the authority for main freshness; golden fixtures updated for the sanctioned v2.0.0 body change

---

## 2026-07-17 21:04 - Freshness layer: logmind warp + main-decisions pulse probe + context origin-refresh

**Reasoning:** A branch pins its derived docs to the merge-base (invariant), so it lags main; warp fetches origin and refreshes the working copy read-only, the 3rd pulse probe nudges warp when main advanced, and context renders the 2 derived docs from the last-fetched origin ref so cold-start reflects main without a network call

**Alternatives considered:** warp merges origin/main into the branch (rejected: creates merge commits and can trip the invariant); a --refresh flag on context instead of a verb (rejected: the pulse needs a short verb to point at)

**Implications:**
- warp is the ONLY network caller (fetch); pulse+context read the local origin ref with no fetch, preserving the network-free hot path; context payload format is byte-unchanged, only the content source differs

---

## 2026-07-17 21:19 - L3: check-derived-docs becomes a blocking PR gate; derived docs regenerate on main only (v7 to v8)

**Reasoning:** The structural wall — a PR that edits a derived doc cannot merge (gh pr diff --name-only is the fork-correct branch-vs-merge-base delta), so even an unforeseen write path can never reintroduce a conflict. Adversarial review caught that permissions: contents:write alone zeroes pull-requests and would 403 gh pr diff on every PR — fixed by adding pull-requests: read in lockstep

**Alternatives considered:** merge-base diff in shell (rejected: base.sha may be absent in a fork checkout; gh pr diff is simpler and fork-correct)

**Implications:**
- Regen moves to a main-only job that no-ops when current (no push loop); the main push needs a PAT with ruleset bypass and degrades to a freshness-only warning without it; a one-way rename of a derived doc evades --name-only but self-heals and causes no conflict (accepted); existing derived-doc-editing PRs now block until reverted; template bumped v7 to v8

---

