← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-04-flip-the-default-timeline-to-main-canonical-pin-branch-diver -->
- **2026-07-04** — flip the default timeline to main-canonical; pin branch-divergent in opt-out tests (v1.0 flip, R5a)
<!-- logmind-entry-end -->

## 2026-07-04 02:57 - flip the default timeline to main-canonical; pin branch-divergent in opt-out tests (v1.0 flip, R5a)

**Reasoning:** The v1.0 milestone flip: DefaultConfig now sets timeline.canonical main-canonical (the conflict-free source-derived union). branch-divergent (the v0.1.x byte floor) remains a SUPPORTED but DEPRECATED opt-out. Version-gated: repos on pre-flip logmind are undisturbed until they upgrade. The 10 tests that verified branch-divergent opt-out/no-op behavior now pin branch-divergent explicitly (still-supported mode; coverage kept); the default-mode assertion flips to main-canonical; a new lock test proves a DEFAULT-config log writes a §1.6.3 marker and renders the entry-block format. The branch-divergent byte-floor goldens are UNCHANGED (verified).

**Alternatives considered:** Regen the cli timeline goldens to main-canonical (rejected — keep the branch-divergent byte floor under test; pin the mode instead). Delete the opt-out tests (rejected — branch-divergent still exists and must stay covered).

**Implications:**
- main-canonical is the default; branch-divergent is opt-out + deprecated. NO version bump here — the release version (1.3.0 vs 2.0.0) is a CEO call at the release cut (current is 1.2.0-dev). The SPEC section 8.7 deprecation, AGENTS v6 to v7, and agent-skills land coordinated; DefaultMap surfacing is a follow-up. HELD for CEO coordination + the release gate.

---

