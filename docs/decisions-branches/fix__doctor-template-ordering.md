← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-14-teach-doctor-the-same-template-ordering-the-installer-learne -->
- **2026-08-14** — Teach doctor the same template ordering the installer learned
<!-- logmind-entry-end -->

## 2026-08-14 17:27 - Teach doctor the same template ordering the installer learned

**Reasoning:** A retrospective panel found that #289 taught installWorkflowTemplates that markers are ordered but left doctor comparing for equality. So a repo AHEAD of the running binary reported 'STALE (latest: v4)' with both the verdict and the latest label inverted — and because #289's refusal is correct, the row became permanently unclearable: doctor said stale, --fix refused, nothing reconciled them. Measured: a repo initialised at check-decisions v5, probed by a v4-bundling binary, printed 'v5 STALE (latest: v4)' and exit 1. My own #289 PR body called this a misleading parenthetical; the panel was right that it is worse than that.

**Alternatives considered:** Suppress the row when the installed marker is unrecognised. Rejected: that hides a real signal. A repository ahead of the binary is worth saying out loud — it means the operator should upgrade — it just is not drift the tool can fix.

**Implications:**
- classifyMarker now returns a third value, 'ahead', and every DRIFT and exit-1 path keys on 'stale' specifically, so ahead is informational and does not flip the tool. The comparison parses the integer after the v and ignores any -pointer suffix; deliberately NOT a string compare, because 'v11' sorts before 'v4' lexically and that trap looks correct in testing right up until a version reaches double digits — which check-decisions just did, v4 to v5, with regen-timeline already at v11. An unparseable marker on either side falls back to the old equality semantics rather than guessing. Verified end to end: v5 vs v5 current, v99 vs v5 ahead, v1 vs v5 still STALE. Eleven table cases plus three controls, including one proving ahead does not mask a genuinely stale row.

---

