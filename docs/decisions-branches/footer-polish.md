## 2026-05-15 13:32 - site: footer polish — version, skill URL, breathing room, single-line nav

**Reasoning:** Footer on logmind.dev was cramped: hardcoded v0.1 (stale), pointing skills.sh badge at the renamed-away thrillmot/logmind-skill URL, four-stacked-row left column, four nav links wrapping with 'security' orphaned to its own line. User flagged via screenshot. Fixes: (1) bump version to v0.1.4 with sync-with-pyproject comment; (2) switch badge URL to skills.sh/b/thrillmot/agent-skills (per-skill /logmind suffix returns 'custom badge: invalid' since skills.sh doesn't expose per-skill badges); (3) py-10 to py-16 + gap-8 between columns; (4) inline by-thrllmt with the version/license line so left column drops from 4 rows to 2; (5) responsive nav — flex-wrap on under-640px so 'security' wraps cleanly within viewport, whitespace-nowrap + inline at sm:+ so the four dot-separated links stay on one row at 768px+.

**Alternatives considered:** Drop the 'made for things you'd miss if forgotten' editorial — cleaner but lost the voice; removed only the standalone by-thrllmt row instead, Pull version dynamically from pyproject.toml or PyPI at build time — heavyweight infra for a value edited once per release; comment-as-guard is sufficient, Keep flex-wrap unconditionally — but then 'security' orphans at 768px which was the original complaint

**Implications:**
- Footer at desktop: logo / v0.1.4 · MIT licensed · by thrllmt; right column has skills.sh badge above changelog/contributing/issues/security on one line
- Mobile 375px: left + right columns stack; nav drops 'security' to its own row but stays within viewport, no horizontal overflow
- Skills.sh badge now reads 'Skills 1' (collection total) and will increment as more skills land in agent-skills

---
