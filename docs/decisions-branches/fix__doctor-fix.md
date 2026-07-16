← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-06-29-add-logmind-doctor-fix-one-command-idempotent-repo-refresh-s -->
- **2026-06-29** — Add `logmind doctor --fix`: one-command idempotent repo refresh (Slice 1)
<!-- logmind-entry-end -->

## 2026-06-29 15:28 - Add `logmind doctor --fix`: one-command idempotent repo refresh (Slice 1)

**Reasoning:** Consumers whose hooks/workflows/AGENTS.md/merge-driver config drift had no self-service fix — doctor only REPORTED drift. --fix completes the de-friction story (handoff deliverable c): one command brings a drifted repo back to spec. The idempotent installers already existed and init's refresh mode already orchestrated them; doctor just never called them.

**Alternatives considered:** Keep doctor read-only + add a separate fix command — doctor already owns the probe→artifact map, so --fix reuses it, Duplicate the refresh sequence in doctor — extracted a shared applyRefresh so init refresh-mode and doctor --fix share one path

**Implications:**
- New internal/cli/refresh.go (applyRefresh + refreshResult); doctor.go gains --fix → runDoctorFix (idempotent; quiet 'ok doctor-fix …' line; residual PATH/foreign-hook drift warned not failed; hard write error exits 1). runInitRefresh refactored onto applyRefresh (now also surfaces previously-swallowed AGENTS.md/.gitattributes write errors). Never writes docs/ content, .logmind/config.yml, or a foreign hook. Symlink-following writes are a pre-existing limitation across all installers (noted, not fixed here).

---

