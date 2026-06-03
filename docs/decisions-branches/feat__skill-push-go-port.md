## 2026-06-03 13:23 - Site: lead install with brew/curl Go binary; mark pip wheel deprecated (frozen v0.6.16)

**Reasoning:** Public-facing logmind.dev still pitched pip install as command 01 — exactly the surface that sends new users to the obsolete v0.6.16 Python wheel post-v1.0. Mirrors the README rewrite shipped in #133.

**Alternatives considered:** Deploy a separate /v1 page (rejected: single-page site, splits SEO and adds maintenance)., Drop the pip command entirely (rejected: still need a copyable install line for the small set of users migrating off pinned 0.6.x).

**Implications:**
- Install section reordered — homebrew (01), curl (02), agent skill (03), verify (04). Hero marginalia bumped to v1.0.0 / 2026-06-03. Nav pypi link replaced with releases (latest). New deprecated subsection inside install card shows pip install 'logmind==0.6.16' with strong deprecation language + link to docs/install.md migration guide. Footer version bumped to v1.0.0.

---
## 2026-06-03 13:37 - Port logmind skill push to Go (G4.b): local->catalog skill promotion via PR

**Reasoning:** Auth via user's gh CLI: catalog repo gates merging via its own clud-bug-review + maintainer approval; this command just opens the PR cleanly

**Alternatives considered:** Inline in skill.go vs new file: chose internal/cli/skill_push.go + internal/skill/push.go for separation of concerns (cobra wiring vs business logic), Direct git/gh exec vs go-github lib: chose subprocess so we reuse user's gh auth without re-implementing GH App tokens, Vendored PR template vs synthesised in code: catalog repo has no PULL_REQUEST_TEMPLATE.md today, so we synthesise (verified via gh api 2026-06-03)

**Implications:**
- New config key catalog_target (default thrillmade/agent-skills) overridable via --catalog flag
- Cache dir at os.UserCacheDir()/logmind/skill-push/<sanitised-target>; fresh clone every push to avoid stale base
- Branch shape: skill/<name>-from-<source-repo>-<short-sha> — encodes provenance into the branch name itself

---
