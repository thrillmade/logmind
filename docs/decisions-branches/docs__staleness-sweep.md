← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-01-correct-19-documentation-claims-that-contradict-current-sour -->
- **2026-08-01** — Correct 19 documentation claims that contradict current source
<!-- logmind-entry-end -->

## 2026-08-01 11:36 - Correct 19 documentation claims that contradict current source

**Reasoning:** A verification sweep of every prose doc against origin/dev found 27 stale claims, 8 of them factually wrong rather than merely dated. README described derived-docs enforcement as opt-in via derived_docs.mode with a min_binary floor — both deleted, the invariant is unconditional. README, CONTRIBUTING and skill/SKILL.md all described a GitHub Action that appends a one-line summary to docs/decisions.md on PR merge; no such action exists (control: grep for 'one-line summary' hit only the three docs asserting it, never a producing workflow). README and docs/install.md told users to pin LOGMIND_VERSION=v2.0.0 and showed --version printing the 2.0.0 two-line output — v2.0.0 has never been tagged, and the released 1.2.0 binary prints one line. README also cited a LOGMIND_AUTO_REGEN_PAT secret that does not exist; regen-timeline.yml mints a skdd-steward[bot] App token. first-decision-example.md carried a fabricated init transcript and a Python-dict template block.

**Alternatives considered:** Fix only the version strings and leave the structural claims. Rejected: the version strings were the least harmful of the set — a reader who follows the derived_docs.mode instructions configures a key that does not exist, and one who waits for the PR-merge Action to write their decision summary waits forever.

**Implications:**
- Verified against the actually-released binary, not against source: docs now show 'logmind 1.2.0 (spec 0.1.1)', which is byte-identical to what the installed homebrew cask prints. Every reported zero was control-tested — 'derived_docs' returns 0 in README while 'derived doc' still returns 1; 'one-line summary' returns 0 while 'decisions-branches' still returns 3. Four findings are deliberately NOT fixed: the required-reading entries naming docs/decisions.md in AGENTS.md, skill/SKILL.md, docs/ai-agent-files.md and internal/templates/logmind-section.md, because #265 deletes that layout and the template one ships fleet-wide via #257 — changing them ahead of those issues would desync the fleet. Four more live in a dated implementation plan under docs/superpowers/plans/ and correctly record intent at the time; editing them would falsify the record.

---

