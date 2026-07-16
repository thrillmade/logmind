<!-- logmind-entry-start: 2026-05-26-feat-v0-2-4-new-logmind-doctor-stack-status-command -->
- **2026-05-26** — feat: v0.2.4 — new logmind doctor stack-status command
<!-- logmind-entry-end -->

## 2026-05-26 11:28 - feat: v0.2.4 — new logmind doctor stack-status command

**Reasoning:** No way today to ask a logmind+clud-bug repo 'what versions are you on and is anything drifted?' without manual greps across config files. logmind doctor reads .github/workflows/*.yml pin lines + template-version markers, optionally probes PyPI + npm for the latest releases, then prints a status table. Read-only by design — prints the suggested fix but never runs it. Markerless workflows (dogfood / customized) explicitly never count as drift, preserving v0.2.1's no-marker-leave-alone heuristic. Exits non-zero on drift so it's CI-pluggable.

**Alternatives considered:** logmind doctor --fix auto-upgrade flag — deliberately deferred to keep doctor read-only and sidestep the 'what if the upgrade breaks something' support load, Use PyYAML to parse config.yml — overkill; we only need one pin line out of regen-timeline.yml + one field out of .clud-bug.json (json.load is fine for the latter), Add a runtime HTTP dep like httpx — unnecessary; urllib.request is stdlib and the probes are simple GETs with 2s timeout

**Implications:**
- logmind doctor exits 0 on OK, 1 on DRIFT; --exit-zero flag for informational CI runs; --json for scripting; --offline to skip PyPI/npm probes
- doctor is read-only; the suggested action ('pip install --upgrade logmind && logmind init') is printed, not executed
- 13 new tests in tests/test_doctor.py covering OK/drift/markerless/missing/network-failure/offline/JSON modes

---
