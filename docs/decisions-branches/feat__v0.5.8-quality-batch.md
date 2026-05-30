## 2026-05-29 23:41 - feat(v0.5.8): quality batch — issue #57 dry-run output + issue #66 file-structure on feature branches

**Reasoning:** Two of the five open quality issues filed 2026-05-27. (#57) Pre-fix 'All agent files are current' message conflated three distinct cases — no AGENTS.md, AGENTS.md without marker, AGENTS.md with current marker — and misled users into thinking everything was installed when really logmind hadn't been initialized. Split into three case-specific messages, plus refined the dry-run prefix from 'Found' to 'Would update' so the language is prospective. (#66) Pre-fix logmind log on a feature branch regenerated timeline.md but skipped docs/file-structure.md. The skip self-perpetuated a 1-entry-stale cycle on main hit live on agent-skills PRs #37→#38→would-be-#39. Original rationale (rebase conflicts) made obsolete by v0.3.0's merge driver. Feature branches now regen on every logmind log — same as timeline.md.

**Alternatives considered:** Ship 5 issues together as one big batch — rejected: user direction 'small changes over and over'. #57 + #66 are the simplest 2; #60 (check-links code-fence stripping) + #59 (--stage scoped warning) + #58 (multi-commit rebase merge driver) land as v0.5.9 / v0.5.10 / v0.5.11 separately.

**Implications:**
- Bumps to v0.5.8. 3 new tests for #57 per-case messaging; 1 test renamed + inverted for #66 regen-on-feature-branch. 620/620 tests pass. The Trusted Publishing OIDC workflow ships this to PyPI on tag push.

---
