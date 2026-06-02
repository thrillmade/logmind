## 2026-06-02 16:27 - B5 wave 1/5: skill core package — scaffold, validate, bench, audit, suggest (heuristic)

**Reasoning:** Mirror src/logmind/core/skill_cli.py at v0.6.16 in a dedicated Go package (internal/skill/). Each function has a 1:1 Python counterpart so the byte-identical parity snapshot tests can pin them. Splitting CLI wiring (internal/cli/skill.go) from logic (internal/skill/) keeps cobra-isolated layers thin.

**Alternatives considered:** Inline everything inside internal/cli/skill.go, Reuse src/logmind via cgo or external subprocess shim

**Implications:**
- Future tooling (B5b logmind sync) can import internal/skill without dragging the cobra tree. Test surface stays manageable — package-level tests vs CLI snapshot tests share clear boundaries.

---
