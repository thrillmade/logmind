← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-06-29-drop-vestigial-pip-pypi-machinery-from-doctor-init-slice-4-c -->
- **2026-06-29** — Drop vestigial pip/PyPI machinery from doctor + init (Slice 4 cleanup)
<!-- logmind-entry-end -->

## 2026-06-29 17:17 - Drop vestigial pip/PyPI machinery from doctor + init (Slice 4 cleanup)

**Reasoning:** Go-era logmind ships via brew/curl + thrillmade/setup-logmind, not pip. But 'logmind doctor' still made a live 2s PyPI network call for a 'latest' version and parsed 'pip install logmind==X' pins that no longer exist in the workflows (→ nil → misleading '(dev install)'); init's refreshPin rewrote those non-existent pins. Dead code producing wrong/slow output.

**Alternatives considered:** Leave it — doctor's network call is slow and its version-drift signal is bogus now that pip pins are gone

**Implications:**
- doctor makes NO network calls now; InstalledVersion = the running binary's version.Version; drift = workflow/hook/PATH-probe rows only (the PATH probe still catches a stale on-PATH binary). --offline kept as a no-op for compat; JSON shape (installed_version/latest_version/network_used) preserved. Removed logmindPinRe/httpGetJSON/logmindInstalledVersion (doctor) + refreshPin (init). A separate inserter pip-pin rewriter (pinLineRE/UpdateWorkflowPin) + template pip comments remain — flagged as follow-up.

---

