## 2026-05-26 17:02 - feat: v0.2.10 changelog-on-upgrade + escape backticks in self-update notice

**Reasoning:** Two changes bundled. (1) Main feature: logmind init refresh-mode prints CHANGELOG sections between the prior pinned version and the currently installed __version__. Closes the agent-memory propagation gap from the inside — when a refresh happens, the agent observing the command output sees every behavior change inline, no AGENTS.md re-read required. New core/changelog.py with extract_sections_between + render_upgrade_prompt; CHANGELOG.md bundled via package-data with build-time copy in publish.yml; editable installs fall back to repo-root copy. Smoke-tested in a fresh repo: simulating a v0.2.6 pin produced a clean 4-section prompt for v0.2.7→v0.2.10. (2) Drive-by fix: bug hunter caught unescaped backticks in logmind-self-update.yml.template line 50; pip install (no args) triggered command-substitution and corrupted the pre-v0.2.1 notice. One-char fix. Template marker bumped v3 → v4 so refresh sweeps it downstream automatically.

**Alternatives considered:** Fetch CHANGELOG from PyPI at runtime instead of bundling — adds network dep + slows init; bundling is simpler, Move CHANGELOG.md into src/logmind/ as canonical — breaks GitHub repo-page auto-rendering + publish.yml's release-notes step that reads from root, Ship the backtick fix as a separate v0.2.11 — adds another release cycle for a one-char fix that's right here

**Implications:**
- logmind init in any repo with a stale workflow pin will now show the full changelog inline; agents whose memory predates the upgrade see actual behavior changes
- publish.yml gains a one-line copy step (cp CHANGELOG.md src/logmind/CHANGELOG.md) before python -m build; src/logmind/CHANGELOG.md is gitignored
- 9 new tests in tests/test_changelog.py covering version parse + range extraction + missing-changelog graceful fallback; pin test in test_v0_2_1_audit_fixes.py updated to v4 marker for logmind-self-update

---
