<!-- logmind-entry-start: 2026-05-26-v0-3-1-pre-transfer-url-update-thrillmot-thrillmade -->
- **2026-05-26** — v0.3.1: pre-transfer URL update — thrillmot → thrillmade
<!-- logmind-entry-end -->

## 2026-05-26 23:16 - v0.3.1: pre-transfer URL update — thrillmot → thrillmade

**Reasoning:** Move 7 of the org migration. logmind is the linchpin: every shipped template and downstream install references thrillmot/<repo> URLs. This PR rewrites every runtime + docs ref to thrillmade/<repo> BEFORE the GitHub org transfer, so the merged commit on main IS the new canonical state. Tags cut from this commit forward (starting with v0.3.1) ship thrillmade URLs natively. AGENTS.md template block-version bumped v4 → v5 (slim: v4-slim → v5-slim) so v0.2.1+ refresh-mode auto-rewrites the AGENTS.md block in every installed downstream repo on next logmind init.

**Alternatives considered:** Transfer the repo first, update URLs after — leaves a window where new installs from main get thrillmot URLs that still work via redirect but read wrong; templating is forward-looking, Skip the marker bump — would leave downstream AGENTS.md blocks stuck on the old skill URL pointing at thrillmot/agent-skills (works via redirect but explicit is better)

**Implications:**
- After this PR merges: gh api transfer thrillmot/logmind → thrillmade/logmind; old PyPI Trusted Publisher (thrillmot) deletable after v0.3.1 publishes successfully from new path
- v0.3.1 publishes from thrillmade/logmind via the NEW PyPI Trusted Publisher (already configured by user); old thrillmot publisher remains live until successful publish confirms zero-downtime
- Existing pinned references (pip install logmind==0.3.0, brew tap thrillmot/logmind, etc.) continue to work via GitHub auto-redirect indefinitely; thrillmade URLs are the new canonical for fresh installs

---
