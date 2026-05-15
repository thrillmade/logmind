## 2026-05-15 12:22 - v0.1.4: optional LOGMIND_BOT_PAT lets aggregator PRs satisfy required checks

**Reasoning:** v0.1.3's aggregator opens a fallback PR when direct push to a protected base ref is blocked. The PR is opened with GITHUB_TOKEN, but GitHub deliberately blocks GITHUB_TOKEN-opened PRs from triggering downstream workflows (anti-recursion safety). Result: any repo with required status checks on its base ref (clud-bug PR review, check-decisions, check-links) gets a stuck aggregator PR — checks never fire, PR never merges. Same dead-end thrillmot/clud-bug hit in v0.1.1. v0.1.3 fixed 'aggregator crashes' but not 'aggregator PR is mergeable.' Tier 1 fix: env fallback secrets.LOGMIND_BOT_PAT || secrets.GITHUB_TOKEN. Users with required-check rulesets opt in by setting the secret; users without continue to get the v0.1.3 behavior unchanged.

**Alternatives considered:** Bypass-list github-actions[bot] on the ruleset — weakens enforcement globally, harder to revert, Drop the PR fallback entirely and emit ::error::, force users to manually merge — worse UX than current v0.1.3, Ship the logmind-bot GitHub App now — right long-term answer, but multi-day work; queued for v0.2

**Implications:**
- Aggregator template now: GH_TOKEN: ${{ secrets.LOGMIND_BOT_PAT || secrets.GITHUB_TOKEN }}, HAS_BOT_PAT for runtime branching
- When falling back to PR creation without a PAT, workflow emits ::warning:: explaining the cause + the fix
- logmind init prints a Tip block at end recommending the secret (always, no autodetection — cheap and offline)
- Inline comment block on the env: section of the workflow template self-documents the requirement for anyone reading the generated file

---
