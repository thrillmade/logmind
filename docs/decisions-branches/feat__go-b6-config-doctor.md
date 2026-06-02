## 2026-06-02 16:26 - B6: config + doctor + init + self-update (Go rewrite)

**Reasoning:** Wave B6 lands the foundation that makes brew install logmind && logmind init a working install. Implements the four core commands replacing pip-distribution path. PATH-resolution probe is v0.6.16 carry-forward — surfaces tokenomics-recurrence root cause at doctor time. Init writes byte-equivalent files modulo version-pin + timestamp + dirname.

**Alternatives considered:** Wait for SPEC v0.2 (overkill for B6 surface), Skip doctor entirely (regresses CI signal already shipped in Python)

**Implications:**
- Downstream consumers can brew install + logmind init from this binary once B7 ships the homebrew tap. config list YAML indent differs from PyYAML by sequence-item depth (documented delta — both parse equivalently). --configure-github, --with-skdd, --install-hook deferred to follow-up PRs (require external auth + tooling integration not in scope).

---
## 2026-06-02 16:29 - B6 cleanup: remove internal/skill/ that leaked from B5 worktree

**Reasoning:** Previous B6 commit accidentally bundled internal/skill/ (the B5 wave's untracked scaffold) because logmind log --stage all picked them up. The skill package is not imported anywhere — dead code in B6 surface and confuses reviewers about B6 scope.

**Alternatives considered:** Leave the skill package in (acknowledged as dead code in PR description)

**Implications:**
- B6 PR diff now focused only on config, doctor, init, self-update. B5 skill work resumes from its own branch.

---
