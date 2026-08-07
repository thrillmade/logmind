← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-07-give-the-gate-one-shared-evaluation-per-spec-3-4 -->
- **2026-08-07** — Give the gate one shared evaluation, per SPEC §3.4
<!-- logmind-entry-end -->

## 2026-08-07 17:09 - Give the gate one shared evaluation, per SPEC §3.4

**Reasoning:** SPEC §3.4 says 'Both interception points and the gate MUST use this one list. Two lists that mean the same thing are two lists that will disagree.' We had two, and neither matched the spec. §3.4 names exactly four exclusions 'and nothing else is': docs/, AGENTS.md plus the §1.2 per-tool files, the toolchain's own config, and forge-reported binary. guardcommit.SubstantiveLines excluded only docs/ and binary — so an AGENTS.md refresh counted as substantive and could block a commit §3.4 exempts. The CI template excluded all four and then added *.md wholesale, which §3.4 names outright: 'A skill file counts. So does an agent definition. Excluding markdown wholesale switches the rule off in the repositories where writing is the work.' Under-excluding locally and over-excluding in CI is the disagreement §3.4 predicts, already fulfilled.

**Alternatives considered:** Patch the four defects separately in the bash. Rejected: the template's own header claims it 'mirrors the local logmind check-decisions hook' and it does not — it reimplements it, and the copy has now drifted five ways. A fifth patch would leave two encodings and a sixth drift.

**Implications:**
- The §1.2 exclusion set is read from agents.FilePatterns() over the existing registry, never re-listed — a second list is the defect being removed. check-decisions gains --base/--head so CI can call the verb instead of reimplementing it, resolves the git toplevel ONCE for both config and every git command (§3.4's subdirectory-bypass rule), takes its threshold from git.commit_line_threshold, and checks §3.1 well-formedness against the range's ADDED lines rather than accepting a touched file. Full suite green; every pin verified by reverting the behaviour and watching the test fail, not by reasoning. Three §3.4-vs-existing conflicts surfaced: logmind log now warns that a reasoning-less entry will not clear the gate, --no-renames is dropped from BOTH numstat wrappers so they cannot diverge, and §3.1 contradicts §3.4 on whether reasoning is required — raised as thrillmade/protocol#93. The workflow template is deliberately untouched; it ships fleet-wide and follows separately.

---

