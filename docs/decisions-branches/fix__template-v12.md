← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-templates-v12-degrade-the-credential-chain-resolve-the-defau -->
- **2026-08-15** — templates v12: degrade the credential chain, resolve the default branch, and stop shipping a path logmind abandoned
<!-- logmind-entry-end -->

## 2026-08-15 15:08 - templates v12: degrade the credential chain, resolve the default branch, and stop shipping a path logmind abandoned

**Reasoning:** The shipped regen template pushed with a raw personal access token while logmind's own copy minted an App installation token, so every repository that scaffolded from it deployed the credential path this project had already moved off. It also pinned an action one release behind what all seven consumer repositories actually run, and it hardcoded the branch name in four places: the push refspec, the trigger filter, the remediation checkout and the error text. The CEO's ruling is that logmind must work standalone, without this organisation, without any App, and without assuming how a repository is set up, so a hardcoded branch is the same defect as a hardcoded credential.

**Alternatives considered:** Broaden the trigger to every branch and gate the job with a condition. Rejected on evidence: a job skipped by its own condition reports success to required status checks, and this file also carries the pull-request-blocking derived-docs gate, so every push to a branch would add a second passing run of that gate on the same commit without evaluating anything. That converts a security-relevant check into one with a silent bypass. The trigger is rendered at scaffold time instead, so a stale value can cost a trigger but never a wrong-ref write, and the gate warns when the rendered value no longer matches the live default branch.

**Implications:**
- Anything evaluated at run time reads the default branch from the event payload and is passed through the environment rather than interpolated into a shell line, because a branch name is attacker-influenceable text and a run block is an injection site. A mutation that survived led somewhere useful: init.defaultBranch equal to main ships inside Apple's command line tools gitconfig, which a system-scope check misses, so the resolver's final fallback is unreachable on any macOS machine and a test written against it passes for the wrong reason. The duplicate fallback was deleted rather than kept as an unreachable safety net. The two-name filter on the link checker was dropped after measuring that all eight repositories running it default to main, and because covering two names out of many is strictly worse than rendering the one that is correct.

---

