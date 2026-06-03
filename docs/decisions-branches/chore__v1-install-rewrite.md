## 2026-06-03 13:23 - Site: lead install with brew/curl Go binary; mark pip wheel deprecated (frozen v0.6.16)

**Reasoning:** Public-facing logmind.dev still pitched pip install as command 01 — exactly the surface that sends new users to the obsolete v0.6.16 Python wheel post-v1.0. Mirrors the README rewrite shipped in #133.

**Alternatives considered:** Deploy a separate /v1 page (rejected: single-page site, splits SEO and adds maintenance)., Drop the pip command entirely (rejected: still need a copyable install line for the small set of users migrating off pinned 0.6.x).

**Implications:**
- Install section reordered — homebrew (01), curl (02), agent skill (03), verify (04). Hero marginalia bumped to v1.0.0 / 2026-06-03. Nav pypi link replaced with releases (latest). New deprecated subsection inside install card shows pip install 'logmind==0.6.16' with strong deprecation language + link to docs/install.md migration guide. Footer version bumped to v1.0.0.

---
