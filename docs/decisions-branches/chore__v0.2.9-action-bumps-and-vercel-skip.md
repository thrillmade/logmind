## 2026-05-26 16:35 - chore: v0.2.9 — bump actions to v6 across templates + dogfood; add vercel.json skip-on-non-site

**Reasoning:** Two-fold maintenance batch: (1) GitHub deprecated Node 20 runtime in 2026-09; all 4 shipped workflow templates were stuck on actions/checkout@v4 (Node 20) and actions/setup-python@v5 (Node 20). Bump everything to @v6 (Node 24) before downstream installs start emitting deprecation warnings. Same bump in this repo's dogfood workflows absorbs dependabot PRs #43 + #44 (those only saw dogfood; shipped templates were the bigger gap). (2) Vercel rate-limited the preview deploys today because Python-only logmind PRs keep rebuilding the marketing site for no reason. Add vercel.json with ignoreCommand=git diff --quiet HEAD^ HEAD -- site/ so non-site PRs skip the deploy entirely.

**Alternatives considered:** Take only the dependabot PRs — would leave shipped templates stale and downstream installs accumulate deprecation warnings, Ship as v0.3.0 (treat the action bump as breaking) — actions/checkout@v6 is backward-compatible on workflow-level usage; not breaking, Configure Vercel skip via dashboard 'Ignored Build Step' field — same effect but invisible in code; future maintainers wouldn't know it's there

**Implications:**
- Downstream logmind-installed repos pick up v6 actions on next logmind init (refresh-mode detects bumped markers); logmind doctor will report STALE rows in the interim
- Dependabot PRs #43 (setup-python) and #44 (checkout) become redundant once this lands; close them with reference to v0.2.9
- Future logmind PRs that don't touch site/ skip Vercel deploy automatically; site PRs still build as normal

---
## 2026-05-26 16:42 - feat: v0.2.9 propagation-gap follow-up — visible notice in logmind log + AGENTS.md drift check in doctor

**Reasoning:** Other agent in clud-bug ran v0.2.7+ but kept prefixing 'git add -A &&' from old habit. Their memory was from pre-v0.2.7 (scoped default). The skill on agent-skills + AGENTS.md template were updated when v0.2.7 shipped, but the agent's session memory is independent of those refreshes — they don't know to re-read AGENTS.md mid-task. Two fixes: (1) logmind log now prints a visible notice when --stage all sweeps the tree, so the actual behavior shows up in command output regardless of memory state; (2) doctor now reports AGENTS.md block-version drift, so agents (or CI) can detect stale embedded instructions explicitly.

**Alternatives considered:** Auto-refresh AGENTS.md on every logmind log — destructive and noisy; logmind init refresh-mode is the canonical path. doctor + the new log notice are gentler, Print the notice from logger.py instead of cli.py — would also fire for library API calls, but those are typically programmatic and don't want the chatter

**Implications:**
- Agents observing logmind log output will see the v0.2.7+ behavior banner; old git-add-first habit fades within one execution
- doctor now exits 1 on stale AGENTS.md block-version; downstream repos that haven't logmind init'd since v0.2.7 will surface in CI immediately
- logmind doctor row count goes from 4 → 5 (added AGENTS.md alongside the 4 workflows); test fixtures updated

---
