← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-16-fix-pulse-subprocess-free-hot-path-local-time-comparison-adv -->
- **2026-07-16** — fix(pulse): subprocess-free hot path + local-time comparison (adversarial-review findings)
<!-- logmind-entry-end -->

## 2026-07-16 23:40 - fix(pulse): subprocess-free hot path + local-time comparison (adversarial-review findings)

**Reasoning:** Adversarial review found 4 defects in the v2.0.0 log pulse: (1) the drift pulse ran doctor's full probe set on every logmind log, including a PATH lookup plus a live --version subprocess with no WaitDelay, so a hung or daemonizing PATH binary could stall or hang the log outright; (2) the spec pulse compared local-wall-clock decision headers (parsed as UTC by decisions.Iter) directly against a real git commit instant, skewing the comparison by the full UTC offset (false fires in positive zones, missed fires in negative zones); (3) the spec pulse could nag about a spec file the same log call was mid-editing; (4) printAdvisory's docstring still claimed stdout-only after PR 207 moved non-interactive advisories to stderr

**Alternatives considered:** Keep StaleCount unchanged and only add a subprocess timeout - rejected, still pays PATH-lookup and subprocess latency on every single log, Relabel decisions.Iter's time.Parse to time.Local globally - rejected, Iter output also feeds timeline dedupeAndSuffix which compares Iter-derived dates against entry-block-marker dates always parsed as bare UTC, so relabeling would change docs/timeline.md byte output on non-UTC hosts

**Implications:**
- doctor.StaleCountFast/collectLogmindStatusFast is the new hot-path probe subset: every workflow marker, AGENTS.md block, git hook, and the Claude PreToolUse guard, all file reads only; PATH resolution and merge-driver git config are excluded and only surface via on-demand logmind doctor
- probePathResolution now sets cmd.WaitDelay = 2 seconds so doctor itself is bounded even against a daemonizing PATH wrapper
- specPulseLine reinterprets decision headers as local time via localizeDecisionDate before comparing to the git commit instant, and skips entirely when gitcli.StatusPorcelain reports uncommitted changes on the spec file

---

