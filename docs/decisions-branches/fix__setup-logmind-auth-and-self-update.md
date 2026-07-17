← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-authenticated-every-setup-logmind-call-site-and-un-bricked-t -->
- **2026-07-17** — Authenticated every setup-logmind call site and un-bricked the dead self-update workflow (v-prefix + apples-to-oranges version compare).
<!-- logmind-entry-end -->

## 2026-07-17 00:24 - fix(ci): authenticate setup-logmind calls + un-brick the self-update workflow

**Reasoning:** setup-logmind#4/#5: every uses: thrillmade/setup-logmind@... call site omitted token:, so the release-lookup call went out anonymous and 403'd on shared-runner IPs before logmind installed. Separately, logmind-self-update.yml.template's fetch_latest() stripped the release tag's v prefix before feeding it to setup-logmind's version: input, which requires the v-prefix -- every run failed with unrecognized version input. Its INSTALLED/LATEST comparison also parsed the setup-logmind ACTION's own pin (independently versioned from the logmind CLI) as a proxy for the installed CLI version, which is never a valid comparison.

**Alternatives considered:** Add a composite-action default for token: -- not possible; GitHub Actions composite actions cannot default an input to github.token, the fix has to live at each call site., Keep the old same-job version-equality skip gate after fixing INSTALLED's source -- rejected: since v1.1.0 every setup-logmind call installs latest by default, so a same-job INSTALLED-vs-LATEST comparison is equal by construction and would have made the workflow permanently silent instead of merely broken. Replaced with the pre-existing git-diff-after-init check as the sole gate (plus the pinVersion opt-out).

**Implications:**
- internal/templates/github/{check-doc-links,regen-timeline,logmind-self-update}.yml.template and the live .github/workflows/logmind-self-update.yml all gained token: on every setup-logmind step; marker bumps v7->v8, v6->v7, v9->v10 respectively so doctor --fix / self-update refresh consumers onto the fix.
- Consumers only heal via a refresh pass (self-update itself has been dead ~6 weeks) -- a one-time doctor --fix / init-refresh on agent-skills + clud-bug-app is a follow-up coordination item, not part of this PR.

---

