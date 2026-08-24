← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-01-refresh-docs-plan-md-against-spec-2-0-and-current-source -->
- **2026-08-01** — Refresh docs/plan.md against SPEC 2.0 and current source
<!-- logmind-entry-end -->

## 2026-08-01 02:18 - Refresh docs/plan.md against SPEC 2.0 and current source

**Reasoning:** The plan documented a system that no longer exists. It described derived-docs enforcement as opt-in via a derived_docs.mode config gate with a min_binary floor — both were removed when the invariant became unconditional; internal/config has no derived_docs key, this repo's own .logmind/config.yml has none, and version.SatisfiesMin has no caller. It described L3 as reading the repo's config and passing unadopted repos, when L3 is checkout-free and always asserts. It cited SPEC §15, §16, §14, §1.6.4, §3.1.1 and §0.3.2 — every one absent from the live 1475-line document (control: §0.4 and §1.6 resolve). It described docs/decisions.md as the recent-decisions store when that file is dead: newest entry 2026-07-16, archive holds zero entries, because PR-required means nothing commits to main.

**Alternatives considered:** Do the docs/architecture.md split first and fix the content there. Rejected for now: the split is a separate approved deliverable, and leaving a document that misdescribes the enforcement surface in place while restructuring it would carry every error across.

**Implications:**
- Citations repointed to live sections: §3.4 enforcement, §1.5 cold-start payload, §3.3 the history and the map, §2.7/2.8 token discipline, §7.3 the version declaration. One §3.1.1 reference remains deliberately, framed as the predecessor's number, because SPEC 2.0 does not mention a pulse at all (zero occurrences; control: 'decision' returns 48) — which is context #268 needs. Adds a Known gaps section recording what was verified against each consumer repo's INSTALLED copy: check-decisions is defeated four independent ways, four repos run template v2 against the v4 logmind ships, reporulez runs an unversioned pre-marker copy, and skdd has no gate at all.

---

