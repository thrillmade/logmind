← back to [docs/timeline.md](../timeline.md)

## 2026-07-03 11:01 - check-doc-links workflow template: advisory + no GITHUB_TOKEN push (v6)

**Reasoning:** The v5 template stranded consumer PRs three ways, all the class #159 fixed for regen-timeline: (1) the mode-A self-heal pushed a Claude fix to the PR head over GITHUB_TOKEN, moving the head SHA without re-triggering checks, stranding every required check; (2) check-links re-raised the linkcheck exit code, red-blocking the merge on any broken link; (3) it filtered dependabot by actor, and a filtered required check hangs forever.

**Alternatives considered:** Only fix the push; leave the hard-fail and the actor filter as-is, Leave the template; just document that a PAT is required

**Implications:**
- Bumped v5 to v6 so doctor flags the drift and consumers re-install; mode A now requires LOGMIND_AUTO_REGEN_PAT (not just ANTHROPIC_API_KEY) to push, else it falls through to the mode-B PR comment; added a templates_test invariant block mirroring regen-timeline's; workflow token dropped to contents:read.

---

## 2026-07-03 11:20 - Harden check-doc-links self-heal to fully advisory (never red the helper job)

**Reasoning:** The adversarial verifier found two MEDIUMs: mode B's gh pr comment 403s on fork PRs (GitHub caps the token read-only) and set -euo pipefail then reds the self-heal job — contradicting the header's claim that forks take the comment path; mode A similarly reds on a transient API error or an unappliable Claude diff. A non-required helper job going red is noisy and defeats the advisory promise.

**Alternatives considered:** Leave as-is (self-heal is not a required check, so it never blocks the merge), Fix only the mode-B fork 403

**Implications:**
- Both modes now use set -uo (not -e) and guard their fallible call — if ! python3 (mode A) / if ! gh pr comment (mode B) — degrading to a ::warning:: + exit 0 like regen-timeline; header 'NEVER blocks' scoped to check-links; a test pins both guards so a fork-red can't be reintroduced.

---

