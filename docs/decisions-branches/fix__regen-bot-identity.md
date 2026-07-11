← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-11-fix-auto-regen-bot-identity-to-canonical-github-actions-bot -->
- **2026-07-11** — Fix the auto-regen bot commit identity to the canonical github-actions[bot], and bump the two workflow templates so consumer repos pick it up via doctor --fix
<!-- logmind-entry-end -->

## 2026-07-11 14:12 - Fix auto-regen bot identity to canonical github-actions bot

**Reasoning:** The fake logmind-auto-regen email on auto-regen commits is rejected by Vercel and clud-bug; the canonical github-actions bot identity is accepted, unblocking derived-doc self-heal on PRs

**Alternatives considered:** Keep the fake identity and disable the auto-regen push path, which would lose the self-heal automation

**Implications:**
- Live regen-timeline.yml fixed immediately; both workflow templates bumped (regen-timeline v5 to v6, check-doc-links v6 to v7) so consumer repos refresh via doctor --fix; templates_test updated

---

