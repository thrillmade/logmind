## 2026-05-26 16:24 - fix: v0.2.8 — replace PyYAML+Python pinVersion detection with grep

**Reasoning:** Bug caught by clud-bug-review across multiple repos: logmind-self-update.yml.template's pinVersion block called python3 -c 'import yaml, sys' with the import OUTSIDE the try block. If the runner lacked PyYAML, the import raised, the surrounding 2>/dev/null || echo '' swallowed the failure into empty pin, and opt-out via pinVersion silently broke. Fix uses grep+sed on the flat top-level scalar — no Python, no YAML lib, works on every runner. Tested against 8 input variants (quoted/unquoted/indented/trailing-ws/absent/substring-only/etc.). Template marker bumped v1→v2 so refresh fires on next logmind init.

**Alternatives considered:** Move import yaml inside try — still requires PyYAML on runner; ubuntu-latest may have it but minimal images don't, Add pip install pyyaml step — slows the workflow and adds a dep just for one field read, Wait for the bug to re-surface on real users — already surfaced via clud-bug-review on multiple repos; fix shouldn't keep slipping

**Implications:**
- Downstream logmind-installed repos pick up the fix on next logmind init (v0.2.5+ refresh-mode auto-detects stale v1 marker)
- logmind doctor will report logmind-self-update.yml as STALE (v1, latest v2) for any repo that hasn't refreshed
- The agent in clud-bug can now ship its propagation PR without the bot re-flagging the same bug

---
