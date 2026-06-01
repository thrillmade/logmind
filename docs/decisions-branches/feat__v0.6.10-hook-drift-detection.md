## 2026-06-01 18:16 - feat(v0.6.10): hook-version drift detection (tokenomics 2026-06-01 bug report)

**Reasoning:** Tokenomics agent flagged post-merge hook checkout-blocking bug RECURRED after the v0.6.9 propagation in PR #48. They correctly diagnosed root cause: their local CLI binary is still v0.3.4, so every logmind log overwrites .git/hooks/post-merge with v0.3.4's body (which stages). The workflow-pin upgrade alone does NOT touch local hooks. v0.6.10 makes the drift LOUDLY visible: (1) hook bodies embed a # logmind-hook-version: <X.Y.Z> line; (2) doctor extracts marker + reports stale/markerless drift; (3) install_post_merge_hook auto-rewrites when marker version differs, so the next logmind log after a binary upgrade self-heals the local hook.

**Alternatives considered:** Defer to v0.6.11 (orphaned-branch skip in hook) — capture root cause more aggressively but more invasive, Land a 'logmind upgrade --refresh-hooks' command — adds CLI surface area; less elegant than auto-heal

**Implications:**
- Future drift becomes loud. After v0.6.10, doctor always tells the user when their on-disk hook is stale relative to the binary running. One-command fix: pip install --upgrade logmind && logmind log.
- Doesn't fix users stuck at pre-v0.6.10 today — only their NEXT logmind log after upgrading. Reporter's immediate remediation: PYENV_VERSION=3.11.8 pip install --upgrade logmind.

---
