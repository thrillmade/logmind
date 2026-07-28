← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-28-implement-spec-3-9-s-since-update-provenance-write-drafts-fl -->
- **2026-07-28** — Implement SPEC §3.9's --since/--update-provenance/--write-drafts flags for logmind sync
<!-- logmind-entry-end -->

## 2026-07-28 01:04 - Implement SPEC §3.9's --since/--update-provenance/--write-drafts flags for logmind sync

**Reasoning:** SPEC §3.9 names three flags 'logmind sync' didn't have; the code comment called them 'tracked for later waves' and the ruling was that a flag which doesn't do what it says is worse than a missing one (PR #247 deleted five for exactly that). --since is a genuine Go-duration-plus-day-shorthand parser (time.ParseDuration has no 'd' unit) with a hard-error path on malformed input, wired into the existing docs/reviews/PR-*.md mtime scan. --update-provenance is a new, separate reconciliation path (skill.UpdateProvenance) that writes the actual SPEC §1.11.1 NORMATIVE PROVENANCE.md template (the '<!-- maintained-by: logmind sync -->' marker, Source/Last refined/Cited by clud-bug/Derived from decisions/Refinement history) sourced from .claude/skills/.clud-bug.json usage[<slug>].citations and docs/decisions*.md heading matches -- the pre-existing Sync() path writes a different, non-SPEC '<!-- logmind:provenance v1 -->' skeleton from docs/reviews/PR-*.md, which is left untouched as the flag-less default per the no-behavior-change ruling. --write-drafts (skill.WriteSkillDrafts) reuses SuggestFromDecisions's heuristic but renders fresh SPEC §1.9/§1.10.1-conformant frontmatter (source: logmind-derived, status: candidate) rather than reusing suggest_llm.go's GH-issue-body WriteDrafts, which has no frontmatter at all and would violate the same 1.9 contract at the new call site.

**Alternatives considered:** Considered gating the legacy citation-fold path behind --update-provenance so there'd be only one PROVENANCE.md writer; rejected because the task's ruling #5 requires bare 'logmind sync' behavior to stay byte-identical with no new flags passed, and the legacy path already runs unconditionally today. Considered reusing suggest_llm.go's WriteDrafts/FormatIssueDraft directly for --write-drafts; rejected because that renderer has no YAML frontmatter at all, so it would put a correctly-named file at docs/skills-derived/ with SPEC-noncompliant contents -- the same 'flag lies about what it did' failure mode, just relocated. Considered sourcing PROVENANCE.md's Source field from .clud-bug.json installed[].source; rejected because that enum (baseline|remote|custom) doesn't match SKILL.md's own source enum (manual|logmind-derived|skills-sh|clud-bug-baseline) that section 1.10.1 defines -- reading the skill's own frontmatter is the more authoritative, honest source.

**Implications:**
- Two PROVENANCE.md writers now coexist in internal/skill/sync.go: the legacy provenance.go-skeleton writer (default, unconditional) and the new SPEC-conformant writer (--update-provenance only). A follow-up wave should retire the legacy skeleton format once consumers migrate, at which point --update-provenance's gating can likely become the default.
- docs/reviews/PR-*.md citations are deliberately NOT re-read by --update-provenance -- .clud-bug.json's usage[<slug>].citations is the SPEC's own primary citation source for PROVENANCE.md; folding both in would produce two disagreeing counters for the same field.
- The 'Derived from decisions' matching heuristic (case-insensitive skill-name substring match against decision titles) is a best-effort implementation choice -- SPEC §1.11.1 only says the list is 'computed from docs/decisions*.md heading anchors' without specifying a match rule.

---

