← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-04-context-repomap-fold-the-go-signature-skeleton-into-logmind- -->
- **2026-07-04** — context.repomap: fold the Go signature skeleton into logmind context (default off)
<!-- logmind-entry-end -->

## 2026-07-04 00:30 - context.repomap: fold the Go signature skeleton into logmind context (default off)

**Reasoning:** Makes the repomap AUTOMATIC — an agent's cold-start context carries the repo's API surface with zero extra action (the built-in-token-saving thesis). Folded stable-first (file-structure -> repomap -> timeline) so the cache prefix stays byte-identical longest. Additive config key context.repomap via the file_structure.root_label recipe: default false -> payload byte-identical, OMITTED from DefaultMap (config-list byte-parity); the v1.0 flip turns it on.

**Alternatives considered:** Write a committed docs/repomap.md derived doc + read it like the other two — rejected: adds another branch-divergent-churn source + hook/CI/doctor regen wiring. Chosen: generate in-memory in contextPayload (deterministic, cheap, no new committed file). contextDoc gains optional gen/enabled fields so contextDocs stays the single ordering authority.

**Implications:**
- New top-level 'context' config section (ContextConfig.Repomap); contextDoc gen/enabled fields + shared writeDoc helper; --stats gains a repomap term (density claimed only when genuinely denser, so toy repos don't print '0.6x'). Paired protocol 0.10.0 SPEC PR retro-specs logmind context + specs the repomap + the context.repomap key + §14.3 cross-link.

---

