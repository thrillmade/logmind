<!-- logmind-entry-start: 2026-05-26-feat-v0-3-0-custom-git-merge-driver-for-derived-files-post-m -->
- **2026-05-26** — feat: v0.3.0 — custom git merge driver for derived files + post-merge hook
<!-- logmind-entry-end -->

## 2026-05-26 17:46 - feat: v0.3.0 — custom git merge driver for derived files + post-merge hook

**Reasoning:** Parallel-PR conflict class: two PRs both running logmind log → textual merge conflict on docs/timeline.md when one rebases onto the other (hit twice in 30 minutes earlier today). v0.3.0 ships a custom git merge driver that delegates conflict resolution to logmind, which regenerates from the per-branch decision files (which never collide). Three install-time pieces: (1) .gitattributes block (committed) registers merge=logmind-timeline for the two derived files, (2) per-clone git config defines the drivers (lives in .git/config, not committed, security guard), (3) .git/hooks/post-merge sweeps the regen after merge completes (the driver fires per-file during conflict resolution, before other merged-in files are checked out — hook ensures the final state reflects the FULL post-merge tree). Smoke-tested end-to-end: two branches both run logmind log, merge succeeds without conflict, resulting timeline shows both decisions.

**Alternatives considered:** Merge driver alone (no post-merge hook) — driver fires before sibling non-conflicted files are checked out; smoke test showed regenerated timeline missed the merged-in branch's decision. Hook is the belt + suspenders., Post-merge hook alone (no merge driver) — git would still produce a textual conflict and halt the merge; hook never runs. Driver IS needed to avoid the conflict; hook is needed for correctness afterward., Patch bump to v0.2.11 — under-bumps the change. Strict semver: new install-time surface (gitattributes + git config + hook) is minor-grade. Also marks a clean reset of the v0.2.x under-bumping pattern (doctor, --stage all default, changelog-on-upgrade all shipped as patches and arguably should have been minor too).

**Implications:**
- logmind init in any v0.3.0+ repo installs the merge driver + post-merge hook; refresh-mode runs them every invocation (idempotent) so fresh checkouts get the per-clone config automatically after one init
- Three new doctor rows surface drift: .gitattributes block, git config drivers, post-merge hook. drift=missing (not stale) when absent — doesn't false-positive fresh repos or test fixtures
- Phase 0 of the thrillmade org migration plan; every logmind init we run in Phase B repos installs the conflict-resistant config from day one

---
