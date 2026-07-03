← back to [docs/timeline.md](../timeline.md)

## 2026-07-03 12:03 - templates: drop dead Python-API blocks, fix wrong-org URLs + dead required-reading

**Reasoning:** The consumer-facing templates shipped by 'logmind init' contained a 'from logmind import log' Python-API block (the Go binary has no importable module), a wrong-org github.com/logmind/logmind URL (dead), and CLAUDE.md.template listed a nonexistent docs/logmind-readme.md as REQUIRED reading — which, post-#172, actively trips consumers' check-links. Audit finding.

**Alternatives considered:** Also bump AGENTS.md.template's Python-API removal + block version (deferred: it's governed v6, coupled to the §8.3 marker-alignment work in the SPEC pass)

**Implications:**
- Scoped to the ungoverned templates (CLAUDE.md.template, logmind-section.md — bare logmind-start, no version marker) + the repo doc docs/ai-agent-files.md; Python examples replaced with the real 'logmind log' CLI; new inits get correct templates. AGENTS.md.template's cleanup + marker bump deferred to the coordinated marker work.

---

