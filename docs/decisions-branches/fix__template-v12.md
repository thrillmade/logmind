← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-templates-v12-degrade-the-credential-chain-resolve-the-defau -->
- **2026-08-15** — templates v12: degrade the credential chain, resolve the default branch, and stop shipping a path logmind abandoned
<!-- logmind-entry-end -->

## 2026-08-15 15:08 - templates v12: degrade the credential chain, resolve the default branch, and stop shipping a path logmind abandoned

**Reasoning:** The shipped regen template pushed with a raw personal access token while logmind's own copy minted an App installation token, so every repository that scaffolded from it deployed the credential path this project had already moved off. It also pinned an action one release behind what all seven consumer repositories actually run, and it hardcoded the branch name in four places: the push refspec, the trigger filter, the remediation checkout and the error text. The CEO's ruling is that logmind must work standalone, without this organisation, without any App, and without assuming how a repository is set up, so a hardcoded branch is the same defect as a hardcoded credential.

**Alternatives considered:** Broaden the trigger to every branch and gate the job with a condition. Rejected on evidence: a job skipped by its own condition reports success rather than skipped, so every push to a branch would add a second passing run of the derived-docs gate on the same commit without evaluating anything. In a repository that has made that gate a required status check this is a silent bypass; in this one it is a misleading green, because logmind's own main ruleset configures no required status checks at all — a claim the template used to make and no longer does. The trigger is rendered at scaffold time instead, so a stale value can cost a trigger but never a wrong-ref write, and the gate warns when the rendered value no longer matches the live default branch.

**Implications:**
- Anything evaluated at run time reads the default branch from the event payload and is passed through the environment rather than interpolated into a shell line, because a branch name is attacker-influenceable text and a run block is an injection site. A mutation that survived led somewhere useful: init.defaultBranch equal to main ships inside Apple's command line tools gitconfig, which a system-scope check misses, so the resolver's final fallback is unreachable on any macOS machine and a test written against it passes for the wrong reason. The duplicate fallback was deleted rather than kept as an unreachable safety net.
- The two-name filter on the link checker was dropped after measuring that all eight repositories running it default to main. Rendering the one correct name beats covering two out of many only when the resolution is correct, and it was not: the resolver preferred a local main over a local master regardless of which the repository actually used, so a master repository carrying a stray main scaffolded a trigger pointing at a branch nothing is pushed to. The two-name literal covered that case by accident. The resolver now resolves rather than ranks — one candidate present wins outright, and a tie is broken by the remote's branch set, then HEAD, then init.defaultBranch — so the argument for rendering one name holds on measurement rather than on assertion.
- Adding the always-present GITHUB_TOKEN as the last credential rung makes the chain's no-credential branch unreachable on GitHub-hosted Actions. That is the fix working, not a preserved distinction: v11 reached warn-and-exit-0 whenever no PAT was configured, which was most repositories, and "configured nothing" now attempts the push. The branch is kept and documented as unreachable at its own site, because a runner that injects no token still lands there and because it is what stops set -u from aborting on an unbound token with a message that names nothing.

---

## 2026-08-15 16:16 - init: add the refresh flag the templates already prescribe, and stop ranking main ahead of master

**Reasoning:** The drift warning shipped in two templates told the user to run a flag that did not exist, and bare init no-ops in exactly the case the warning fires, because it rewrites only when marker versions differ and a renamed default branch moves no version. Two tests pinned the broken command. The force mode re-renders files whose marker already matches, which is the case a version-ordered refresh structurally cannot repair, and it does not widen ownership: an unmarked file is skipped and a newer marker is still declined. Separately the branch resolver ranked main ahead of master regardless of which existed, so a repository defaulting to master with a stray local main rendered the wrong trigger.

**Alternatives considered:** Reword the message to name a command that already works. Rejected: there was no such command for this case, so the message would have had to say that a renamed default branch cannot be repaired without editing the file by hand, which is worse than adding the flag. Also rejected: making the no-credential rung reachable by pre-flighting token scope, because that conflates a permissions problem with a policy refusal, and the distinction between degrading quietly and failing loudly is the thing the previous version established.

**Implications:**
- The lockstep gap that let the original credential divergence ship is now content-checked on both sides rather than skipped, and two smuggled probes prove it: a step exfiltrating the job environment, which holds the App private key, and a regen writing its output somewhere harmless so the job reports success forever. Both go red, and a mutation neutering the comparison turns the first green again, which is the control. The header no longer claims the earlier distinction survived unchanged, because it did not: the no-credential outcome moved, and that was the point.

---

