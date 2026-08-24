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

## 2026-08-24 14:45 - docs/plan.md: correct my own A4 claim — the self-update skip marker is a live defect, not a legitimate carve-out, and the four defeats are closed only in the template source

**Reasoning:** The #356 verifier refuted the claim I had told it to attack hardest, and it was right. I had reclassified logmind-self-update's [skip-logmind] commit-subject prefix as legitimate. It is not: guardcommit.Evaluate is reached only from 'logmind guard-commit', never from check_decisions.go, and SPEC 3.4 says a commit-subject marker is invisible to the gate and MUST NOT be honoured there. A synthetic self-update-shaped commit exits 1 through 'logmind check-decisions --base/--head'. Separately, 'closed in this repo' was overstated: logmind's own installed check-decisions.yml is an unversioned pre-v5 variant on both main and dev carrying the *.md exclusion, the live-title override and a hardcoded THRESHOLD 20.

**Alternatives considered:** Leave A4 and narrow only the 'in this repo' phrase. Rejected: the two are the same error. Our own gate honours the PR title, which is exactly why the self-update defect has never surfaced — reporting one without the other would leave the plan explaining why we are safe using the mechanism that hides the bug.

**Implications:**
- This is the house defect appearing in my own prose for the second time today: a true statement about a narrow thing presented as a conclusion about a wider one. The template source IS fixed; I wrote that as 'this repo'. Filed the two live consequences as #364 (our own gate is pre-v5 and defeatable by retitling, pre-tag) and #365 (self-update PRs fail their own gate on any repo running v7; the fix is a design call at protocol, not a patch).

---

