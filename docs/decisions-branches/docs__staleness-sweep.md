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

## 2026-08-07 16:38 - Remove the fabricated PR-merge Action claim from the shipped AGENTS.md template

**Reasoning:** The docs sweep fixed this claim in README, CONTRIBUTING and skill/SKILL.md but missed internal/templates/AGENTS.md.template:93, which is the one copy that ships into consumer repositories. It told every repo running logmind that 'On PR merge, a workflow appends a one-line summary to docs/decisions.md linking the PR and the per-branch file.' No such workflow exists — control: grepping 'one-line summary' across live surfaces now returns 0 while the Python-era changelog still returns 2, where it is correct history because the retired Python line genuinely shipped logmind-aggregate.yml. Caught by an adversarial reviewer on PR #282, not by the sweep.

**Alternatives considered:** Defer to #257 with the other template changes, since template edits ship fleet-wide. Rejected after checking how propagation actually works: the deferred items (the docs/decisions.md required-reading entries) are deferred because #265 has to decide what REPLACES them, so changing them early would desync. This one has no pending decision behind it — the workflow does not exist and never will — so deferring would only mean shipping a known falsehood for longer.

**Implications:**
- Verified the fix actually reaches consumers before landing it: FindOutdatedMarkerBlocks (internal/inserter/inserter.go:458-487) compares block CONTENT — strings.TrimSpace(installed) == strings.TrimSpace(fresh) — and matchingTemplate only selects the full-vs-slim flavour. So a content change is detected as outdated without a block-version bump; I had assumed a v8 to v9 bump was required and that was wrong. Left the surrounding sentence intact because it is still accurate: resolveDecisionsPath (internal/cli/log.go:534) does still route default-branch entries to docs/decisions.md today. That whole model changes under #265.

---

