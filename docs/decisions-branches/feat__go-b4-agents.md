## 2026-06-02 01:21 - feat(go-b4): port agents (list/add/remove/update/migrate) + inserter package + embed AGENTS.md templates

**Reasoning:** B4 wave is the agent-file templating layer of the Go rewrite. The inserter package's surgical marker-block rewrite is the load-bearing primitive — it lets agents update --apply refresh template bodies WITHOUT touching user content above or below the markers. Templates are embed.FS bytes so the binary stays single-file. The pin updater preserves quote style (single/double/none) to match Python v0.6.11+ for reporulez-convention single-quoted pins.

**Alternatives considered:** Keep templates on disk under $XDG_DATA_HOME (rejected: defeats single-binary distribution), Re-derive AGENTS.md from scratch on every update (rejected: would clobber user-content above/below markers), Auto-migrate full v5 template to slim v7-pointer (rejected: that's an init decision, not an update decision; never silently flip flavors)

**Implications:**
- Future waves B5/B6 use this inserter package — init/self-update/configure-github all call EnsureAgentsMD / FindOutdatedMarkerBlocks / UpdateWorkflowPin
- Templates byte-identical to Python via templates.TestTemplates_ByteIdenticalToPython parity gate
- Marker-block round-trip invariant proved via TestReplaceMarkerBlock_RoundTrip
- Workflow pin sweep limited to canonical pin workflows — never rewrites user-owned workflows

---
## 2026-06-02 01:34 - fix(go-b4): ReplaceMarkerBlock inverted-marker guard + InsertLogmindSection no-heading test (clud-bug PR #118)

**Reasoning:** clud-bug-review on PR #118 flagged 2 issues. (1) ReplaceMarkerBlock missing the 'end < start' guard that ExtractMarkerBlock has — on a file with malformed markers (end before start), the function silently corrupted the output by duplicating the inter-marker region. Added the same guard; added test pinning the safe behavior. (2) InsertLogmindSection's no-heading branch (when file lacks a '# ' line, section gets prepended at position 0) had zero test coverage — a regression on that path would be invisible. Added test using a .cursorrules-shaped fixture (no H1 heading); asserts marker precedes user content + idempotency.

**Alternatives considered:** Document inverse-marker handling but not fix it — would defer corruption risk, Make ReplaceMarkerBlock return an error on inverted markers — breaks its (content, body) → content signature; cascades

**Implications:**
- Both primitives (Extract + Replace) now share the same well-formedness contract: bail on missing OR inverted markers, return content unchanged
- Future fixture-based tests on InsertLogmindSection cover the no-heading branch — protects against regression when B6's init flow uses this function

---
