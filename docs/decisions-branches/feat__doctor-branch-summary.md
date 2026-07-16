← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-06-29-slice-2-pr10-doctor-branch-summary-health-advisory-fix-backf -->
- **2026-06-29** — Slice 2 PR10: doctor branch-summary health (advisory + --fix backfill) + logmind headline --file
<!-- logmind-entry-end -->

## 2026-06-29 21:29 - Slice 2 PR10: doctor branch-summary health (advisory + --fix backfill) + logmind headline --file

**Reasoning:** The graceful migration path for the deterministic fallback (CEO: doctor finds un-summarized branches and asks the agent to fix the important ones; fail-graceful, not a hard block). In main-canonical mode, logmind doctor advisorily lists branch files that are markerless or still on the placeholder headline (== first-decision title); doctor --fix backfills the marker into markerless files (the deterministic structural half — the rich summary stays the agent's job via logmind headline --file). The advisory NEVER flips Overall to DRIFT. Default branch-divergent repos see ZERO change (gated).

**Alternatives considered:** doctor itself generates rich summaries via an LLM — rejected: adds an API-key dependency + non-determinism to the deterministic core. doctor does the mechanical backfill; the agent (the LLM with context) writes the sentences.

**Implications:**
- doctor.go: StatusReport.SummariesNeeded (advisory, kept OFF Tools[].Workflows so Overall + residualProbes are untouched) + collectSummariesNeeded (markerless/placeholder via stripPRSuffix) + renderer block. cli/doctor.go: runDoctorFix backfills markerless files (reuses cli-local marker helpers; summaries-backfilled=N in the ok-line); --fix docs updated (backfills markers, never rewrites decision text). headline.go: setHeadlineInFile extracted + --file flag. All main-canonical-gated.

---

