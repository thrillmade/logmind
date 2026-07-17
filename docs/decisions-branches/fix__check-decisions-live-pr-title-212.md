← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-fix-ci-check-decisions-honors-skip-logmind-on-rerun-fetch-th -->
- **2026-07-17** — fix(ci): check-decisions honors [skip-logmind] on rerun — fetch the live PR title (#212)
<!-- logmind-entry-end -->

## 2026-07-17 00:09 - fix(ci): check-decisions honors [skip-logmind] on rerun — fetch the live PR title (#212)

**Reasoning:** check-decisions.yml read the skip-logmind override from github.event.pull_request.title, which is frozen at trigger time. Rerunning a failed check after a maintainer retitles the PR replays the original payload, so the override silently does not take effect. Protocol SPEC section 15.3 and Appendix A.2 make skip-logmind a normative escape hatch, so this is a conformance bug, not just an inconvenience. The fix fetches the live title at run time via gh pr view using the PR number from the event, which stays stable across reruns, and computes a skip_logmind step output the Enforce step gates on instead. A gh API failure fails open to the old event-payload title so a transient hiccup never crashes the check.

**Alternatives considered:** Tell maintainers to close and reopen the PR instead of rerunning — rejected, error-prone and contradicts the documented rerun workflow, Add a brand-new dedicated step just for the title fetch — rejected in favor of folding it into the existing Compute diff vs base step to keep the diff minimal

**Implications:**
- Template version bumped v3 to v4 on check-decisions.yml.template; consumers refresh via the Monday self-update wave
- Workflow permissions gained pull-requests: read so gh pr view can succeed on private consumer repos
- agent-skills is still on template v2 for this workflow and needs the self-update wave or a manual refresh to pick this up

---

