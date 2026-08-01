← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-01-regen-timeline-a-refused-push-must-fail-the-job-not-report-s -->
- **2026-08-01** — regen-timeline: a refused push must fail the job, not report success (logmind#262a)
<!-- logmind-entry-end -->

## 2026-08-01 01:22 - regen-timeline: a refused push must fail the job, not report success (logmind#262a)

**Reasoning:** SPEC-2 §3.3 requires three push outcomes that MUST NOT look alike: nothing-to-push (success), no credential (warning + exit 0, since a merge that was otherwise fine should not fail over a missing secret), and a push attempted and refused (a failure, MUST be reported as one). regen-on-main's push step conflated the last two — both used ::warning + exit 0 — so a refused push (GH013, main's ruleset rejecting a non-bypassed identity) reported success identically to a run with nothing to do. Real evidence: the job ran green for 11 days while 14 branch decision files never reached docs/timeline.md. §6.5 generalizes this beyond gates: any producer writing something another party depends on must report a write it attempted and could not complete.

**Alternatives considered:** Considered leaving the refusal path at exit 0 with only a stronger warning message. Rejected: §3.3 is explicit that a refused write MUST be reported as a failure, and a warning that still exits 0 is indistinguishable from success to a reader scanning job status or a dashboard that only checks conclusion — which is exactly the failure mode #262 documents.

**Implications:**
- regen-on-main (a push-to-main job, not a required PR check) now fails its own run when the steward token mints but the push is rejected — this surfaces the staleness in the Actions tab/commit status without blocking any merge, since nothing merge-time depends on this job. The no-credential path is untouched: still ::warning + exit 0. .github/workflows/regen-timeline.yml and internal/templates/github/regen-timeline.yml.template were changed in lockstep (marker bumped v10 -> v11); TestRegenTimelineWorkflow_LockstepWithTemplate gained a trailing anchor pinning 'exit 1' as shared structure in the previously-unconstrained credential gap, and TestRegenTimelineTemplate_V10_UnconditionalBlockingGate gained assertions scoping ::warning+exit0 to the no-credential branch and ::error+exit1 to the push-refusal branch specifically. Does not touch part (b) (making the push land) — no credential, secret, or push-mechanism change.

---

