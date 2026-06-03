## 2026-06-02 22:54 - Lock SpecVersion to 0.1.0 (drop -draft suffix)

**Reasoning:** thrillmade/protocol SPEC.md tagged v0.1.0 FINAL (commit 86c2212). Binary's --version was still advertising spec 0.1.0-draft, which would mislead downstream tools and any binary the v1.0.0 final tag ships.

**Alternatives considered:** Keep -draft until the next spec drift, Cut SpecVersion to a const

**Implications:**
- Golden file (internal/cli/testdata/version.golden) must match: bumped from 'logmind 1.0.0-dev (spec 0.1.0-draft)' to 'logmind 1.0.0-dev (spec 0.1.0)'. No other consumers of SpecVersion in the tree.

---
## 2026-06-02 22:55 - Doctor remediation: brew/curl install, not pip

**Reasoning:** v1 ships as a Go binary distributed via Homebrew tap + install.sh; the legacy 'pip install --upgrade logmind && logmind init' suggestion would dead-end users who never had Python logmind installed. G1.d smoke test on v1.0.0-rc1 binary surfaced this.

**Alternatives considered:** Keep pip suggestion as a fallback line, Emit only brew (drop curl alternative)

**Implications:**
- Renderer iterates Suggestions one-line-each; split the remediation into 3 entries (brew | # or: curl ... | # then re-run: logmind init) so the formatted output reads as a 3-line stanza under 'Suggested:'. Doctor tests still green; no golden pinned the literal suggestion string.

---
## 2026-06-02 23:01 - Pin brew/curl suggestion stanza in doctor test

**Reasoning:** clud-bug review thread on PR #131 (3345655561) correctly noted that TestCollectStatus_StaleWorkflowFlipsToDrift reaches the suggestion-emitting code path but never asserts on r.Suggestions content — a regression that reverted to the old pip string would still pass. Evidence-based-review: the reviewer quoted classifyLogmindDrift line 263 to prove the test path is live.

**Alternatives considered:** Add a dedicated TestSuggestions_BrewStanza test, Leave the assertion to the manual doctor run

**Implications:**
- Same test now asserts both Overall=DRIFT and the 3-line brew/curl stanza in r.Suggestions (exact byte match per entry). Future revert to pip-install string trips this test.

---
