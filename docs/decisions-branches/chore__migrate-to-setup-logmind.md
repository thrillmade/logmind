<!-- logmind-entry-start: 2026-06-05-migrate-logmind-templates-to-setup-logmind-v1-0-1-dependabot -->
- **2026-06-05** — Migrate logmind templates to setup-logmind@v1.0.1 + dependabot group
<!-- logmind-entry-end -->

## 2026-06-05 23:50 - Migrate logmind templates to setup-logmind@v1.0.1 + dependabot group

**Reasoning:** Closes out the v1.1.0 distribution-lock wave by bumping the workflow-template setup-logmind pin from v1.0.0 → v1.0.1 (final immutable ref) and adding the `thrillmade` group to this repo's existing dependabot github-actions ecosystem entry. Future `logmind init` runs scaffold v1.0.1, and Dependabot now bundles all `thrillmade/*` action bumps into one PR per release — same shape we're rolling out to the 6 consumer repos in this same cleanup wave. Last manual setup-logmind version touch on the logmind side; from here Dependabot owns it.

**Alternatives considered:** Leave templates at v1.0.0 and let Dependabot bump them on first install (rejected — new `logmind init` runs would scaffold a stale pin until Dependabot ran, creating a one-cycle drift window for fresh consumers); bump only the action templates without adding the dependabot group here in logmind (rejected — logmind is itself a consumer repo for things like actions/checkout, and the orchestrator-app pattern means it consumes thrillmade/* actions too; the group keeps the propagation pipeline consistent across this repo and the 6 downstreams).

**Implications:**
- `internal/templates/github/check-doc-links.yml.template`, `regen-timeline.yml.template`, `logmind-self-update.yml.template` bump `thrillmade/setup-logmind@v1.0.0` → `@v1.0.1`. Template-version markers left unchanged (v4 / v4 / v8) — pure pin bumps are Dependabot's job, not the self-update workflow's. Marker bumps only ship when the template BODY shape changes.
- `.github/dependabot.yml` gains a `groups: thrillmade: patterns: ["thrillmade/*"]` block on the existing `github-actions` ecosystem entry. Bundles future thrillmade/* action bumps (setup-logmind, agent-skills future actions, etc.) into one PR per release.
- Existing tests pass: `internal/templates/templates_test.go` and `internal/cli/init_test.go` both use prefix matching (`thrillmade/setup-logmind@v`), so they're version-agnostic. No test churn.
- Companion PRs land in the other 6 consumer repos this same wave (clud-bug, clud-bug-app, agent-skills, tokenomics, reporulez, rezgen). Each replaces the legacy curl-install / pip-install block with `uses: thrillmade/setup-logmind@v1.0.1` and merges the dependabot group block.
- READMEs (`README.md`, `docs/install.md`) and the installer's CI-advisory message (`installer/install.sh`) still reference `v1.0.0` — intentionally untouched in this PR. They're user-facing docs, not the scaffold path; Dependabot won't bump them. Separate doc-update sweep if/when v1.0.x churns again.

---

