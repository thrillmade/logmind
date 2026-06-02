## 2026-06-02 16:26 - B6: config + doctor + init + self-update (Go rewrite)

**Reasoning:** Wave B6 lands the foundation that makes brew install logmind && logmind init a working install. Implements the four core commands replacing pip-distribution path. PATH-resolution probe is v0.6.16 carry-forward — surfaces tokenomics-recurrence root cause at doctor time. Init writes byte-equivalent files modulo version-pin + timestamp + dirname.

**Alternatives considered:** Wait for SPEC v0.2 (overkill for B6 surface), Skip doctor entirely (regresses CI signal already shipped in Python)

**Implications:**
- Downstream consumers can brew install + logmind init from this binary once B7 ships the homebrew tap. config list YAML indent differs from PyYAML by sequence-item depth (documented delta — both parse equivalently). --configure-github, --with-skdd, --install-hook deferred to follow-up PRs (require external auth + tooling integration not in scope).

---
