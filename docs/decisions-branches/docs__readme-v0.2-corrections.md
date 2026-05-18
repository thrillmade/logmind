## 2026-05-18 12:36 - docs: correct README required-repo-settings for v0.2 + add reporulez one-command

**Reasoning:** v0.2 switched regen-timeline.yml from auto-commit to fail-fast (workflow permissions: contents:read only, no write). README still claimed workflow write permissions were required — that's outdated. Removed. Also added a reporulez clud-bug-logmind variant link as the one-command setup path, since reporulez just shipped that bundle.

**Implications:**
- Bonus: this PR triggers a fresh Vercel production deploy (rate-limit expired 3 days ago, last attempt failed red)

---
