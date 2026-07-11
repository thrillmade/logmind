← back to [docs/timeline.md](../timeline.md)

## 2026-06-08 11:46 - rename 4 logmind workflows with 'logmind /' display-name prefix

**Reasoning:** CTO flagged PR check names (decision-log check, doc link integrity, check derived docs, logmind self-update) as ambiguous about owner — PR check listings make it impossible to scan at a glance which checks are logmind vs project vs clud-bug. Branded prefix unifies the workflow set under a recognisable namespace, matching GitHub's own 'CodeQL / Analyze' style.

**Alternatives considered:** Keep current names + add owner badge in PR comment. Rejected: PR check display name is the load-bearing scan surface; comments don't help., Use 'logmind:' prefix without spaces. Rejected: GitHub's convention is 'WorkflowName / job-name' with spaces around the slash; matches CodeQL/Dependency Review.

**Implications:**
- Job IDs (check-decisions, check-links, check-derived-docs) deliberately unchanged so org-default-protection ruleset still matches.
- Template-version markers bumped (v2→v3 check-decisions, v5→v6 check-doc-links, v4→v5 regen-timeline, v8→v9 logmind-self-update) so consumer repos auto-pick-up via the logmind-self-update.yml workflow on the next refresh cycle.
- Dogfood .github/workflows/*.yml files in this repo also renamed in lockstep so logmind's own PRs show the new branded names immediately.
- internal/templates/templates_test.go updated (TestCheckDocLinksTemplate_V5 → V6) to pin both the v6 marker AND the 'name: logmind / check-links' branded line so the parity test trips on any future name regression.
- CONTRIBUTING.md branch-protection prose updated to reference 'logmind / check-links' instead of 'doc link integrity'.

---

