# Decision Log

This file contains the 20 most recent decisions. Older decisions are archived in [decisions-archive.md](decisions-archive.md).

---
## 2026-05-15 04:30 - Merged: dogfood-workflows (#15)

- **PR:** https://github.com/thrillmade/logmind/pull/15
- **Decisions:** 1 from this branch
- **Detail:** [decisions-branches/dogfood-workflows.md](decisions-branches/dogfood-workflows.md)

---
## 2026-05-15 17:34 - Merged: footer-polish (#32)

- **PR:** https://github.com/thrillmade/logmind/pull/32
- **Decisions:** 1 from this branch
- **Detail:** [decisions-branches/footer-polish.md](decisions-branches/footer-polish.md)

---
## 2026-05-28 11:28 - 0.A.9 audit: all 7 consuming repos pass Q4 + <200-line CLAUDE.md; zero Q5 path-scope candidates found

**Reasoning:** Read-only audit per plan. Surveyed AGENTS.md + CLAUDE.md across agent-skills, clud-bug, homebrew-logmind, logmind, reporulez, rezgen (local clones) + tokenomics (gh api). Findings: (1) Q4 pattern intact in every repo with files — AGENTS.md is the canonical real file (109-163 lines, 4.8-6.7KB), CLAUDE.md is a 48-line stub redirecting to AGENTS.md. (2) Every file is under 200 lines (largest = logmind at 163). (3) Q5 path-scope audit: scanned section structure of logmind (largest) and reporulez (second largest). All content is universal — decision logging (logmind block), required reading, dev setup, clud-bug collaboration. No rule blocks ≥20 lines scoped to a subdirectory. Q5 stays deferred to Phase 3 unless future content adds domain-scoped rules. (4) Zero repos use .claude/rules/; zero use symlinks. Pre-existing minor observation: logmind AGENTS.md has duplicate '## Development Commands' and '## Project Overview' sections (lines 62+85, 66+103) — stale agents-sync artifact, not in audit scope.

**Implications:**
- Phase 0.A.9 closes with no PRs needed. Phase A effectively complete (0.A.1-0.A.8, 0.A.10 shipped; 0.A.9 audited clean). 0.X.1 path-scope pilot remains Phase 3 conditional.

---
## 2026-05-29 13:12 - Defer 0.B.5 (docs/decisions.md per-entry compact) to Phase 3+ — structural overhead too small to be worth a release

**Reasoning:** PR #80 (0.B.6) just shipped the higher-leverage AGENTS.md-block trim (69% reduction, ~1.8KB saved per repo per AGENTS.md read). The 0.B.5 rubric ALSO passed (per_file_share[docs/decisions.md] = 0.389 ≥ 0.10, mean 6017 bytes ≥ 2048, 9 sessions with reads ≥ 5) — so by the plan's rubric, 0.B.5 ships. BUT inspecting the actual per-entry shape (logger._format_decision output: 213 bytes for a sample entry with all fields), the structural overhead available to trim is microscopic. Options: (1) drop inter-section blank lines: ~3 bytes per entry. (2) compact labels (Reasoning → Why, Alternatives considered → Alt, Implications → Impact): ~30 bytes per entry but breaks anyone who greps for the existing label pattern. (3) drop trailing blanks: ~1 byte. Net achievable lossless trim: ~5 bytes per entry vs the plan's 40% estimate of ~800 bytes per entry. The plan's estimate was overoptimistic — decisions.md entries are mostly content, not structure. Per-entry content (reasoning text, alternatives, implications) is what users wrote and shouldn't be touched.

**Alternatives considered:** Ship 0.B.5 with the conservative ~3-byte-per-entry blank-drop. Rejected: doesn't move the needle. Net savings across 9 sessions × 10 entries each = ~270 bytes per per-session run. Not worth a release + propagation., Ship 0.B.5 with compact labels (Reasoning→Why, etc.) for ~30 bytes per entry. Rejected: breaks grep patterns in user scripts + agent skills + clud-bug-collaboration documentation. Information identity is more important than 30 bytes., Re-define 0.B.5 as 'compact docs/decisions-archive.md per-entry shape' (older entries that agents read less). Rejected: deferred-to-Phase-3+ list already includes 'docs/decisions-branches/*.md per-entry compaction' as a similar item. Bundle there.

**Implications:**
- 0.B.6 alone captures the bulk of the AGENTS.md-direction savings. Org-cumulative per_session bench should show the per-file shares shift after consuming repos refresh to v0.5.6's v6-pointer block — agents_md_block_share will drop from 0.51 toward something like 0.20-0.30 (the new block is 69% smaller; remaining AGENTS.md content unchanged). docs/decisions.md share stays ~0.39 because we're not touching its content.
- Re-measure post-consumer-rollout. If the per_session data shows decisions.md reads are still a high share AND specific content patterns emerge that COULD be compressed (e.g. repeated boilerplate in merge entries — a legacy artifact from the removed logmind-aggregate workflow), revisit 0.B.5 with a more targeted trim. Until then, 0.B.5 stays Phase 3+ deferred with explicit measurement in this -i field.

---
## 2026-06-02 19:59 - test(B6 follow-up): port commit-msg hook tests from main to v1-go-rewrite

**Reasoning:** clud-bug-review on #128 flagged 163 LOC of new commit-msg hook code with zero test coverage. Foreign-hook preservation is the critical untested path — a logic error would silently overwrite a user's custom commit-msg hook. Idempotency on re-install and version-regex on installed_commit_msg_hook_version are also uncovered

**Alternatives considered:** skip the tests — rejected: the foreign-hook overwrite path is a real correctness risk, write tests from scratch — rejected: tests already exist on main from the v0.6.16 PR #123; just port them

**Implications:**
- 9 commit-msg tests added covering install creation + idempotency + foreign-hook preservation + version detection + runtime warn/strict modes
- tests/test_merge_driver.py grew from 52 → 52+ tests; full suite passes

---
## 2026-06-02 21:01 - fix(B7): tag-trigger glob pattern (v[1-9]+ → v[1-9]*)

**Reasoning:** v1.0.0-rc1 push produced startup_failure because GitHub Actions tag globs treat + as a LITERAL character, not a regex quantifier. The first-draft pattern v[1-9]+.[0-9]+.[0-9]+ never matched a real tag name; the workflow was effectively unreachable

**Alternatives considered:** use regex syntax with paths-ignore — rejected: GH tag triggers only support globs, not regex, use plain v* — rejected: would also fire for v0.x.x tags that still belong to the Python publish.yml pipeline

**Implications:**
- v[1-9]* matches v1.0.0, v1.0.0-rc1, v2.0.0, etc. while excluding v0.x
- v1.0.0-rc1 tag retag required after this fix lands

---
## 2026-06-02 21:05 - fix(B7): replace missing HOMEBREW_TAP_PAT secret with GITHUB_TOKEN

**Reasoning:** release.yml repeatedly hits startup_failure with no jobs created. HOMEBREW_TAP_PAT secret was never provisioned on the repo. GitHub may be failing the secret-resolution phase at startup. Falling back to GITHUB_TOKEN (always present) gets us past the startup gate. Cross-repo write to homebrew-tap will fail at runtime if needed, but that's a known follow-up — we'll provision HOMEBREW_TAP_PAT before the real release

**Alternatives considered:** leave the broken reference and hope startup_failure is caused by something else — rejected: ruling out the obvious first, provision HOMEBREW_TAP_PAT now — rejected: needs a fine-grained PAT with write access to thrillmade/homebrew-tap; takes time and user authorization

**Implications:**
- release.yml workflow should at least START on next tag retrigger
- homebrew cask auto-bump won't work until HOMEBREW_TAP_PAT is provisioned (TODO before v1.0.0 final)

---
## 2026-06-02 21:08 - fix(B7): remove empty env: block from GoReleaser snapshot step

**Reasoning:** Empty env: blocks (containing only comments after a prior cleanup) are invalid GitHub Actions YAML schema. GH startup_failure with no jobs created → the YAML parser rejects the workflow before any job runs

**Implications:**
- release.yml workflow should now START on next tag retrigger

---
## 2026-06-02 21:08 - fix(B7): bisect release.yml — minimal version to identify startup_failure cause

**Reasoning:** Despite actionlint passing, GH keeps reporting startup_failure with no jobs. Replacing release.yml with a minimal version to see if the workflow can run AT ALL on this repo's runner setup. If this works, the bug is somewhere in the original complex version. If this fails, the bug is environmental (runner/permissions/org config)

**Implications:**
- release.yml will only echo a hello message and tag name. No GoReleaser, no signing, no release artifacts. Pure smoke test of workflow infrastructure

---
