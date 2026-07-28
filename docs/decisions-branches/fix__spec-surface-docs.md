← back to [docs/timeline.md](../timeline.md)

## 2026-07-28 00:31 - Close three SPEC-vs-docs gaps: list the two git.* config keys, scope the LOGMIND_QUIET claim in the full AGENTS template, and re-base the contract framing on the SPEC

**Reasoning:** A 2026-07-27 feature-set audit against SPEC.md found the plan was complete on behavior and incomplete on surface. These are the three cheapest confirmed gaps, all verified against SPEC text rather than an agent report. (1) SPEC 1.2.1 documents git.enforce_commits (true) and git.commit_line_threshold (20) and places them after auto_rebase in its example config; both were parsed and honored by guard-commit but absent from config.DefaultMap(), so 'config get git.enforce_commits' answered 'not found' for a key with a documented default. (2) PR #247 fixed the LOGMIND_QUIET overclaim in skill/SKILL.md and the slim template but missed the sibling full AGENTS.md.template, which ships into every consumer repo claiming every verb honors the flag when only 9 of ~23 do. (3) docs/plan.md still named byte-identical output against frozen Python v0.6.16 as the hard contract, contradicting the ruling that the SPEC is the contract and Python is dead history.

**Alternatives considered:** Scaffold the two config keys into config.yml.template instead of DefaultMap -- rejected: DefaultMap is only the merge base for LoadAsMap, so adding them fixes introspection without changing the bytes written into a new repo's config.yml. Reword the quiet claim from scratch -- rejected in favour of reusing #247's exact wording in skill/SKILL.md, so the two surfaces cannot drift apart again. Delete the Python sentence outright -- rejected: the port history is real and worth keeping, it just must not read as the live constraint.

**Implications:**
- 'logmind config get' now answers for both git.* keys with SPEC-documented defaults; verified before/after with a control key that already worked and a nonexistent key that must still exit 1. New repos scaffolded from the full AGENTS template no longer ship a false claim about quiet coverage. Separately: the plan's rule to run the suite under init.defaultBranch=master is now known to be wrong -- CI does not set it, and it manufactures two internal/cli failures that do not reproduce without it. A scratch-repo probe confirmed decision routing is correct either way, because gitcli.DefaultBranch resolves refs/heads/main at step 2 and never reaches init.defaultBranch at step 4.

---

