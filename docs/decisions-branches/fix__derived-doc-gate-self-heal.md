← back to [docs/timeline.md](../timeline.md)

## 2026-06-29 14:10 - Self-heal the derived-doc CI gate so it never blocks PRs (friction #1, Slice 1)

**Reasoning:** The repo's own regen-timeline.yml was still fail-fast (permissions: contents: read, exit 1 on stale timeline.md, no auto-commit, didn't even check file-structure.md) — a stale derived doc red-lit every PR with no self-service fix but 'regen locally and push', which races concurrent PRs and wedged even docs-only changes. The consumer template was already at v5 self-heal; the repo's own gate never got it.

**Alternatives considered:** Downgrade the gate to advisory-only (continue-on-error) — rejected: leaves derived docs drifting; self-heal keeps them eventually-correct AND non-blocking, Require a regen PAT — rejected: v5's GITHUB_TOKEN single-job regen+commit+verify needs no PAT or per-repo secret

**Implications:**
- Same-repo PRs + pushes to main auto-commit the regen via GITHUB_TOKEN then re-verify in one job; forked PRs get an advisory warning + exit 0 (SPEC §5.1.1: forks MUST NOT fail)
- Adds .playwright-mcp/ to .gitignore so local MCP scratch stops leaking into file-structure.md; README 'Required'->'Recommended' repo settings since strict branch protection is no longer needed to avoid blocking
- Tree-gen non-determinism (root label = cwd basename; nested-gitignore artifacts) is handled operationally (CI is the canonical generator) and tracked as Slice 2 in the plan

---

