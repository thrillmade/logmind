<!-- logmind-entry-start: 2026-06-03-site-lead-install-with-brew-curl-go-binary-mark-pip-wheel-de -->
- **2026-06-03** — Site: lead install with brew/curl Go binary; mark pip wheel deprecated (frozen v0.6.16)
<!-- logmind-entry-end -->

## 2026-06-03 13:23 - Site: lead install with brew/curl Go binary; mark pip wheel deprecated (frozen v0.6.16)

**Reasoning:** Public-facing logmind.dev still pitched pip install as command 01 — exactly the surface that sends new users to the obsolete v0.6.16 Python wheel post-v1.0. Mirrors the README rewrite shipped in #133.

**Alternatives considered:** Deploy a separate /v1 page (rejected: single-page site, splits SEO and adds maintenance)., Drop the pip command entirely (rejected: still need a copyable install line for the small set of users migrating off pinned 0.6.x).

**Implications:**
- Install section reordered — homebrew (01), curl (02), agent skill (03), verify (04). Hero marginalia bumped to v1.0.0 / 2026-06-03. Nav pypi link replaced with releases (latest). New deprecated subsection inside install card shows pip install 'logmind==0.6.16' with strong deprecation language + link to docs/install.md migration guide. Footer version bumped to v1.0.0.

---
## 2026-06-03 13:45 - Fix PR #134: regenerate timeline + add /install.sh Vercel route

**Reasoning:** Two failing checks (check-derived-docs, check-links) both stemmed from docs/timeline.md referencing a stale decision file feat__skill-push-go-port.md left behind when an earlier agent auto-routed the branch log to the wrong branch — the actual decision lives in chore__v1-install-rewrite.md. Separately, G3.a consumer-repo migration PRs are pointing at curl logmind.dev/install.sh which 404s today because Next.js has no /install.sh route; installer/install.sh is committed here but unreachable via the CDN.

**Alternatives considered:** Mirror installer/install.sh into site/public/ so Next serves it as a static asset (rejected: duplicates the source of truth — every release would need a sync step and could go stale silently), Add a Next.js route handler at app/install.sh/route.ts that streams the raw GitHub content (rejected: Vercel rewrites do the same job with zero runtime cost and no Function invocation), Put the rewrite in root vercel.json (rejected: site/ is the Vercel project rootDirectory so a root-level rewrites block would be ignored; root vercel.json is reserved for ignoreCommand which is special-cased to run against the git repo root)

**Implications:**
- docs/timeline.md now correctly links to decisions-branches/chore__v1-install-rewrite.md — both check-derived-docs and check-links should turn green on re-run
- site/next.config.ts gains an async rewrites() block proxying /install.sh to raw.githubusercontent.com/thrillmade/logmind/main/installer/install.sh — installer stays single-source-of-truth in installer/, and logmind.dev/install.sh resolves once this lands and deploys
- Long-term cleanup for G3.a consumer-repo PRs: once #134 merges and Vercel deploys, the parallel fixup agent's raw.githubusercontent.com URLs can be replaced with the cleaner https://logmind.dev/install.sh in a future pass

---
