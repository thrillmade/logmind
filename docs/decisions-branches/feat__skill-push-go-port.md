<!-- logmind-entry-start: 2026-06-03-site-lead-install-with-brew-curl-go-binary-mark-pip-wheel-de -->
- **2026-06-03** — Site: lead install with brew/curl Go binary; mark pip wheel deprecated (frozen v0.6.16)
<!-- logmind-entry-end -->

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
## 2026-06-03 14:04 - fix(skill push): preserve file mode; reject path-traversal skill names (review #136)

**Reasoning:** Two clud-bug-review threads on PR #136 flagged correctness gaps in 'logmind skill push'. Bug 3: copyTree always wrote dest files with 0o644, silently dropping the executable bit from source scripts (e.g., .claude/skills/<name>/scripts/helper.sh). The catalog clone would ship an unrunnable helper.sh and consumers would have to re-chmod +x after install. Fixed by reading source mode via os.Lstat and passing info.Mode().Perm() to os.WriteFile + a follow-up os.Chmod so the mode lands consistently whether dp is freshly created or pre-existing. Bug 4: opts.SkillName flowed in from args[0] without sanitization, so a caller passing '../foo' would have filepath.Join escape both the local skills tree (on read) and the catalog clone's skills tree (on write). Fixed by validating SkillName against the SPEC §1.10.1 kebab-slug regex before any filepath.Join call, returning ErrInvalidSkillName (wraps the existing error conventions) on rejection.

**Alternatives considered:** Bug 3 alt: replicate full Unix mode bits including setuid/sticky — rejected as overreach for a markdown-skill catalog; preserving just Perm() keeps the surface minimal-surprise, Bug 3 alt: io.Copy with os.OpenFile(perm) instead of ReadFile/WriteFile — rejected because skills are small markdown/script files where the buffered ReadFile is fine and the existing shape is preserved minus one line, Bug 4 alt: strings.ContainsAny(name, '/\\.') rejection — rejected because the spec already pins the slug shape (^[a-z0-9][a-z0-9._-]*$); we lift that exact constraint here so push agrees with frontmatter validation everywhere else, Bug 4 alt: validate inside SkillDir() so every caller benefits — rejected because SkillDir is a pure path helper called from many sites; tightening at the push entry point keeps the blast radius small and matches where the user input actually enters

**Implications:**
- internal/skill/push.go gains a package-level skillNameRE and a new ErrInvalidSkillName sentinel; pushWith now returns the sentinel for empty / path-traversal / non-slug skill names before touching the filesystem
- copyTree now requires the source file to exist at os.Lstat time — concurrent unlink would surface a clean error instead of silently writing a 0o644 dest
- New tests TestCopyTree_PreservesExecutableBit, TestPush_RejectsPathTraversalSkillName (9 sub-cases), and TestPush_AcceptsValidSlugs (7 sub-cases) pin the new invariants

---
