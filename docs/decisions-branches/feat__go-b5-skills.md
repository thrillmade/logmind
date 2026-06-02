## 2026-06-02 16:27 - B5 wave 1/5: skill core package — scaffold, validate, bench, audit, suggest (heuristic)

**Reasoning:** Mirror src/logmind/core/skill_cli.py at v0.6.16 in a dedicated Go package (internal/skill/). Each function has a 1:1 Python counterpart so the byte-identical parity snapshot tests can pin them. Splitting CLI wiring (internal/cli/skill.go) from logic (internal/skill/) keeps cobra-isolated layers thin.

**Alternatives considered:** Inline everything inside internal/cli/skill.go, Reuse src/logmind via cgo or external subprocess shim

**Implications:**
- Future tooling (B5b logmind sync) can import internal/skill without dragging the cobra tree. Test surface stays manageable — package-level tests vs CLI snapshot tests share clear boundaries.

---
## 2026-06-02 16:35 - B5 review fixes: address clud-bug PR #124 findings (LimitReader, JSON brace-balance, audit comment)

**Reasoning:** clud-bug-review on PR #124 surfaced 2 critical + 1 minor finding. Addressed all three: (1) net/http body read now uses io.LimitReader(1 MB) so a misbehaving Anthropic endpoint can't exhaust process memory; (2) extractJSONBlob() now walks the brace stack honoring quoted strings + escapes so a trailing '}' in chat-style suffix or a '}' inside a draft_description string doesn't break the parse; (3) audit DecisionCount substring-match retained for byte-identical parity with Python v0.6.16 but documented with explicit trade-off comment + pointer to the word-boundary fix when v1.0 spec accepts the change. Added two new tests: TestAnthropicSuggester_LimitReader pins the cap behaviour, TestExtractJSONBlob extended with 6 cases including brace-inside-string + escaped-quote scenarios.

**Alternatives considered:** Whole-word regex match for audit DecisionCount: rejected as parity-breaking; deferred to v1.0 spec accept, Stream-decoder for the LLM response instead of LimitReader: rejected — adds complexity without solving the core memory bound that LimitReader handles in one line

**Implications:**
- Critical findings closed before merge. Audit DecisionCount stays substring-match (matches Python v0.6.16); future spec change unlocks the whole-word fix. LLM transport now defensive against runaway responses + malformed JSON; clud-bug will not re-flag these on the next round.

---
