## 2026-06-01 00:55 - v0.6.5: skill suggest — human-gated pattern detection (closes Stream 6 follow-ons)

**Reasoning:** Replaces the KILLED Stream 9 autonomous skill-lifecycle bot. CLI scans recent decision-log entries for kebab-case/PascalCase/acronym/snake_case identifiers appearing across many distinct decisions (heuristic 'we keep talking about X' signal), filters stopwords + existing skills, and emits pre-filled GH-issue drafts matching agent-skills's new-skill.yml template. Human reads, decides, opens (or discards). Never auto-PR. The whole point of the pragmatic SkDD pivot is that humans gate skill lifecycle.

**Alternatives considered:** Use TF-IDF or LLM clustering for higher-precision pattern detection (rejected: heuristic + human review is the design contract — TF-IDF needs a baseline corpus; LLM clustering reintroduces bot behavior we explicitly killed), Auto-open GH issues for surfaced patterns (rejected: violates the 'humans gate' constraint), Score patterns by 'severity' from finding-counts (rejected: would couple skill-suggest to clud-bug-review's data; suggest should standalone)

**Implications:**
- Closes the v0.6.x Stream 6 follow-ons: new (v0.6.0) → test (v0.6.0) → bench (v0.6.3) → audit (v0.6.4) → suggest (v0.6.5). The complete logmind-side skill craftsperson workbench
- Pattern detection is intentionally lo-fi — kebab-case identifiers dominate technical decision logs naturally. Tightening can come in v0.6.6+ if users surface miss-cases
- PostgreSQL → 'postgre-sql' (not 'postgresql') because the regex splits at lowercase→Uppercase. Acceptable for slugs; documented in test_kebab_slug

---
