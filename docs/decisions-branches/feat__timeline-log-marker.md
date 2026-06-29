← back to [docs/timeline.md](../timeline.md)

## 2026-06-29 19:20 - Slice 2 PR4: logmind log writes the §1.6.3 timeline marker; union computes the detail link

**Reasoning:** Fourth of 7 PRs. logmind log opens a main-canonical branch file with its entry-block headline (written once, preserved on append, inserted if absent for pre-opt-in files). The marker body is LINK-FREE; the union (renderCanonical) computes the detail link from the source path — because check-links resolves links relative to the source file's dir, a docs/-relative link baked into a branch file would be broken there.

**Alternatives considered:** Bake the detail link into the marker body — rejected: it would fail check-links from the branch file's own directory. Compute at render time from the known source path instead.

**Implications:**
- log.go: buildTimelineMarker (key excludes the (#NN) suffix per §1.6.3.1; visible line includes it) + insertMarkerAfterHeader + prSuffixFromEnv (LOGMIND_PR, offline — no gh call on the hot path). canonical.go: HeadlineLine single-sources the link-free body; renderCanonical appends a detail link built from the source path. Gated on isBranchFile + IsMainCanonical so default branch files stay byte-identical. FOR PR7 SPEC: the link-free marker body + union-computed link convention to ratify.

---

