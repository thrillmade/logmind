← back to [docs/timeline.md](../timeline.md)

## 2026-08-16 11:45 - gates: read the job definition from the base ref, so a pull request cannot rewrite the gate that judges it

**Reasoning:** On a pull_request trigger the forge reads the workflow file from the pull request's own merge commit, so a PR that edits the gate's YAML has already replaced the job. Both gates carried their whole logic inline on that trigger: a PR changing check-decisions' Enforce step to exit 0 passed its own gate. Worse, regen-timeline's check-derived-docs carried a comment asserting immunity — 'NO checkout, deliberately... nothing here for a PR to influence about its own gate' — which is false and load-bearing in the bad direction: checkout-free protects the WORKSPACE, and what a pull request rewrites is the JOB DEFINITION. That comment is why nobody looked again, so deleting it is part of the fix rather than tidying after it.

**Alternatives considered:** Move the logic into a reusable workflow or composite action pinned to an immutable ref — rejected, and the reasoning is the useful part: the CALLER stays in the gate's file, so on pull_request a PR rewrites uses:/with:/if: as easily as run:. Making that shape safe needs a base-read trigger anyway, at which point the indirection buys nothing — and a ./-local callee is itself read from the PR's ref, so it would have to live in a logmind-owned repo that every consumer's gate then depends on existing.

**Implications:**
- pull_request_target reads the workflow from the base ref, and SPEC §6.3's condition on it — MUST NOT check out the PR's content — is what both jobs were already built to do. The workspace holds base content only, pinned to base.sha rather than the branch tip; the PR's commits arrive as git OBJECTS via refs/pull/N/head and are never checked out; the binary comes from a pinned released action rather than the checkout, so a PR cannot rebuild the thing judging it; and the SHAs reach the script through env: rather than inline interpolation. Verified: zero head-ref checkouts in either gate, while check-doc-links keeps its head checkout safely on plain pull_request. Behaviour is unchanged — the decision-logic regions diff IDENTICAL at 51/51, 13/13 and 17/17 operative lines, with a control proving the probe shows a diff when one exists. Markers move check-decisions v6 to v7 and regen-timeline v13 to v14, which is what propagates the fix to consumers. CODEOWNERS is deliberately NOT added: alone it enforces nothing, the ruleset half lives in another repo, and a single-maintainer repo would block every solo change.

---

