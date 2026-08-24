← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-24-docs-plan-md-correct-two-stale-claims-about-the-commit-gate- -->
- **2026-08-24** — docs/plan.md: correct two stale claims about the commit gate — the four defeats are closed and the hook fail-open is no longer silent
<!-- logmind-entry-end -->

## 2026-08-24 14:17 - docs/plan.md: correct two stale claims about the commit gate — the four defeats are closed and the hook fail-open is no longer silent

**Reasoning:** Both claims were measured false against dev today. The template's *.md exclusion, live-PR-title [skip-logmind] read, and path-match decision test now survive only as past-tense comments; the gate shells to 'logmind check-decisions --base/--head' at :183, applying the SPEC 3.1 shape check in Go. The hook fail-open prints 'logmind: commit gate NOT RUN' at hooks.go:411 and :426, so 'today it is [silent]' is false. A plan that overstates its own known gaps is as misleading as one that hides them — a reader triaging the tag would rank two closed items as blockers.

**Alternatives considered:** Strike both bullets. Rejected: the remainder is real and would be lost. Propagation to the fleet is still open, and #270's bare-name engine resolution is still open — the remainder becomes the item rather than disappearing with the stale half.

**Implications:**
- The fourth 'defeat' is reclassified rather than closed: logmind-self-update still prefixes its commits [skip-logmind], but that is matched against the commit SUBJECT (guardcommit.go:155), not the mutable PR title SPEC 3.4 forbids reading, so it is a legitimate carve-out. #270's title still carries the false 'silently' wording and needs the same correction.

---

