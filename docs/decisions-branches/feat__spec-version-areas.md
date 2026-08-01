← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-01-repoint-every-spec-citation-at-spec-2-0-s-live-numbering -->
- **2026-08-01** — Repoint every spec citation at SPEC 2.0's live numbering
<!-- logmind-entry-end -->

## 2026-08-01 01:50 - Repoint every spec citation at SPEC 2.0's live numbering

**Reasoning:** The PR advanced SpecVersion to 2.0.0 but left the surfaces that STATE the spec version citing the archived predecessor's numbering. docs/spec.md — the file whose entire job is declaring which spec version and sections logmind implements — cited §15, §16 and §3.1.1; all three return zero against the live 1475-line SPEC (control: 'Section 3' returns 1). README.md and docs/install.md showed '(spec 1.5.0)' as the expected --version output, and site/app/page.tsx published that same stale string to the public site. version.go contradicted itself: its SpecVersion docstring correctly noted 1.5.0 was measured against the archived document, while its package docstring cited §15/§16/§3.1.1 as if current.

**Alternatives considered:** Leave docs/spec.md for a follow-up issue, since the PR title is scoped to the version constant. Rejected: that file is the declaration this PR changes, so shipping the constant while the declaration still points at dead numbering is a half-change, and the adversarial panel is the only gate that reads it.

**Implications:**
- Every section number now in docs/spec.md was verified present in the live SPEC (§1.5 cold-start payload, §2.7/2.8 token discipline, §3 Record, §5.2 nomination, §6.2 the checks, §7.3 what a tool declares) with an absent-section control. The two-line --version output in the docs is byte-identical to the binary's. The dead numbering survives in ~20 code comments (internal/cli/log.go §3.1.1, templates §15.3) and in docs/plan.md + docs/orchestrator-app.md — a systematic sweep filed separately, deliberately not folded into a version PR.

---

