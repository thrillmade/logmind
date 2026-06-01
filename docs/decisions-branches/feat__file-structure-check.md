## 2026-06-01 17:06 - feat: `logmind file-structure --check` (v0.6.9 — symmetric CI gate with timeline)

**Reasoning:** logmind timeline --check has shipped since v0.5.13; the mirror command file-structure has been missing the same flag. Adding it closes the asymmetry and gives CI / pre-commit / doctor a single scriptable verification primitive for the second derived doc. Also closes issue #93 (the v0.5.14 'always-fire merge driver' candidate, which turned out to be infeasible by git design — drivers only invoke on conflict, pre-merge-commit hooks don't fire on GitHub squash-merge). Mitigations (v0.5.11/12/13 + v0.6.7 + PAT-driven auto-fix in regen-timeline.yml) cover the practical cases end-to-end.

**Alternatives considered:** Skip ship — leave file-structure asymmetric with timeline (consumer-product wart), Always-fire merge driver — infeasible per git design (rejected), Pre-merge-commit hook — doesn't help squash-merge flow (rejected)

**Implications:**
- Closes issue #93. v0.6.9 ships as a small symmetric-completeness PR.
- Validated 804 pytest pass / 1 skip. Editable install needed re-running pip install -e . to pick up source edits — caught during pytest run when the installed cli.py was stale (worth noting for any contributor running tests after a CLI edit).

---
