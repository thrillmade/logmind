← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-06-29-make-the-derived-doc-ci-gate-advisory-slice-1-de-friction-ne -->
- **2026-06-29** — Make the derived-doc CI gate advisory (Slice 1 de-friction): never block, never GITHUB_TOKEN-push
<!-- logmind-entry-end -->

## 2026-06-29 15:03 - Make the derived-doc CI gate advisory (Slice 1 de-friction): never block, never GITHUB_TOKEN-push

**Reasoning:** The v4 consumer template fail-fasts (exit 1) when no LOGMIND_AUTO_REGEN_PAT is set — the common consumer case — hard-blocking every PR with stale derived docs. #158 tried auto-pushing via GITHUB_TOKEN, but that moves the PR head SHA without re-triggering checks, stranding every required check (it only looked fine because logmind's own repo has no branch protection). Adversarial review caught two further reintroductions of blocking: fork PRs died at checkout (missing repository:), and head -80 SIGPIPE'd (exit 141) under pipefail on large diffs.

**Alternatives considered:** Auto-push via GITHUB_TOKEN (#158) — strands required checks in protected repos, continue-on-error advisory with no regen — lets derived docs drift unrecoverably

**Implications:**
- v5 template + logmind own gate regenerate both docs; PAT path commits [skip-logmind] and pushes via explicit PAT URL (re-triggers checks); no-PAT/fork path warns and exits 0 (never blocks). Token kept contents:read + persist-credentials:false (no creds during make build); staleness via git status --porcelain; diff SIGPIPE-guarded; dependabot actor filter removed. Supersedes #158.

---

