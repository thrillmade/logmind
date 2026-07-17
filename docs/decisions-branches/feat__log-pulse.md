← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-16-feat-log-the-pulse-doctor-drift-spec-staleness-advisories-on -->
- **2026-07-16** — feat(log): the pulse - doctor-drift + spec-staleness advisories on every log
<!-- logmind-entry-end -->

## 2026-07-16 21:48 - feat(log): the pulse - doctor-drift + spec-staleness advisories on every log

**Reasoning:** Agents burn tokens rediscovering repo-health problems (stale hooks, a spec nobody updated) that logmind already has the data to surface for free at the one moment every session touches: logmind log. Putting the signal on stderr, outside both the byte-exact 3-line stdout contract and the quiet single-ok-line contract, means every caller gets it for free without any output-parsing risk.

**Alternatives considered:** A separate logmind pulse subcommand agents would have to remember to run, Folding the checks into doctor's Overall so log fails/warns loudly, Polling repo health on a timer via a background process

**Implications:**
- New internal/doctor.StaleCount and internal/gitcli.IsTrackedFile/LastCommitTime exported helpers, reused rather than duplicated logic
- specPulseThreshold=20 is a hardcoded package const, not a config key, until real usage shows it needs to be tunable
- Pulse computation is fully best-effort: any git/file error silently skips the pulse rather than failing or slowing logmind log

---

