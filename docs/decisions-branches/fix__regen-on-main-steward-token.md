← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-25-fix-regen-on-main-push-as-the-steward-app-and-degrade-instea -->
- **2026-07-25** — Fix regen-on-main: push as the steward App and degrade instead of failing when the push is rejected
<!-- logmind-entry-end -->

## 2026-07-25 15:12 - Fix regen-on-main: push as the steward App and degrade instead of failing when the push is rejected

**Reasoning:** The first real run of regen-on-main went red: main is governed by a changes-must-be-made-through-a-pull-request rule, so the credentialed push was rejected with GH013 regardless of token scope. The pushing identity must carry a ruleset bypass, which is the skdd-steward App (already used by release.yml), not a PAT. A rejected push is also now a warning rather than a job failure, matching what SPEC 5.1.3 already specifies: on the default branch a stale derived doc is a freshness gap, never a conflict risk, so it must not red-light main

**Alternatives considered:** Have the job open a PR with the regen instead of pushing (rejected: PR churn on every merge, and that PR would itself need review), Drop main regeneration entirely (rejected: main is the only place the derived docs are allowed to be current, so the freshness half of the feature would be gone)

**Implications:**
- The consumer template keeps the PAT path since consumers have no steward App, but gains the same degrade-never-fail behavior plus a note that the pushing identity needs a ruleset bypass; ORG ACTION still required to make the push actually land — the steward App must be added to the main ruleset bypass list

---

