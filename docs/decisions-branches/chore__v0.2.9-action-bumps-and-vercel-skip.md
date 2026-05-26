## 2026-05-26 16:35 - chore: v0.2.9 — bump actions to v6 across templates + dogfood; add vercel.json skip-on-non-site

**Reasoning:** Two-fold maintenance batch: (1) GitHub deprecated Node 20 runtime in 2026-09; all 4 shipped workflow templates were stuck on actions/checkout@v4 (Node 20) and actions/setup-python@v5 (Node 20). Bump everything to @v6 (Node 24) before downstream installs start emitting deprecation warnings. Same bump in this repo's dogfood workflows absorbs dependabot PRs #43 + #44 (those only saw dogfood; shipped templates were the bigger gap). (2) Vercel rate-limited the preview deploys today because Python-only logmind PRs keep rebuilding the marketing site for no reason. Add vercel.json with ignoreCommand=git diff --quiet HEAD^ HEAD -- site/ so non-site PRs skip the deploy entirely.

**Alternatives considered:** Take only the dependabot PRs — would leave shipped templates stale and downstream installs accumulate deprecation warnings, Ship as v0.3.0 (treat the action bump as breaking) — actions/checkout@v6 is backward-compatible on workflow-level usage; not breaking, Configure Vercel skip via dashboard 'Ignored Build Step' field — same effect but invisible in code; future maintainers wouldn't know it's there

**Implications:**
- Downstream logmind-installed repos pick up v6 actions on next logmind init (refresh-mode detects bumped markers); logmind doctor will report STALE rows in the interim
- Dependabot PRs #43 (setup-python) and #44 (checkout) become redundant once this lands; close them with reference to v0.2.9
- Future logmind PRs that don't touch site/ skip Vercel deploy automatically; site PRs still build as normal

---
