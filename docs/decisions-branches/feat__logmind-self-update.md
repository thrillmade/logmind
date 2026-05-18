## 2026-05-18 14:18 - feat: ship logmind-self-update.yml workflow template

**Reasoning:** Mirrors clud-bug's self-update.yml.tmpl pattern (Monday 12:00 UTC cron, opens PR on bump). Closes architectural gap F from the 5-repo seamless-updates plan: previously repos with logmind installed had to manually  to pick up new releases. Now they get a weekly auto-PR. Reads installed version from the v0.2.1+ workflow pin ( in regen-timeline.yml). Opt-out via pinVersion in .logmind/config.yml.

**Implications:**
- Future logmind releases auto-propagate to all repos that have logmind installed and have the workflow active
- Pre-v0.2.1 installs (no pin marker) get a clear notice telling them to re-run init once manually
- Pinning to a specific version is a one-line addition to .logmind/config.yml — same UX as clud-bug's manifest

---
