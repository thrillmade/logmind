<!-- logmind-entry-start: 2026-05-30-feat-v0-5-12-logmind-log-auto-installs-merge-driver-hooks-on -->
- **2026-05-30** — feat(v0.5.12): `logmind log` auto-installs merge driver + hooks on every invocation (fresh-clone auto-resolve)
<!-- logmind-entry-end -->

## 2026-05-30 10:12 - feat(v0.5.12): `logmind log` auto-installs merge driver + hooks on every invocation (fresh-clone auto-resolve)

**Reasoning:** Pre-v0.5.12: logmind init was the only path that installed the per-clone git config + hooks for timeline.md / file-structure.md to merge cleanly. Fresh clones / CI runners / agents in throwaway worktrees had the committed .gitattributes reference to merge=logmind-timeline but no driver definition registered locally — git refused to invoke the unconfigured driver (security guard) and silently fell back to ort 3-way merge → text-valid but semantically incomplete timeline.md → check-derived-docs failed downstream. Hit live on tokenomics #21. User stance (memory project_timeline_conflict_should_auto_resolve): 'conflicts bugs like this shouldn't happen on our own timeline file and logmind should auto resolve.'

**Alternatives considered:** logmind doctor exits non-zero + nags — rejected, requires user to run doctor (won't catch agent flows). v0.5.13 will add the doctor-exit anyway for CI gate use, but log() is the load-bearing self-heal path., Commit driver script to repo + reference via .gitattributes wrapper — rejected, git's security model requires .git/config registration regardless of where the script lives., Ship driver as python -m logmind.merge_drivers.timeline — rejected for the same reason; git still needs .git/config to know about the driver name.

**Implications:**
- Cost per logmind log: ~3 git config --get calls + 2 file stats. Negligible.
- All three installers are individually idempotent + silent no-ops outside a git repo. The cost is fully amortized — first logmind log in a fresh clone leaves the clone fully configured; subsequent calls are pure no-ops.
- Closes the auto-resolve gap for fresh clones. Combined with v0.5.11's post-rewrite hook + v0.3.0's post-merge hook + merge driver, the derived-docs self-healing story is now end-to-end complete: clone → log → merge / rebase / amend all work without manual intervention.
- Stream 4b website messaging ('Your docs stay in sync. Always.') now backed by full implementation across all 3 git operations (merge / rebase / amend) and all clone states (fresh / configured).

---
