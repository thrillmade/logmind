← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-tree-union-the-ignore-sources-instead-of-letting-config-repl -->
- **2026-08-15** — tree: union the ignore sources instead of letting config replace the defaults
<!-- logmind-entry-end -->

## 2026-08-15 09:08 - tree: union the ignore sources instead of letting config replace the defaults

**Reasoning:** Setting a single file_structure.ignore_patterns entry replaced the built-in list wholesale, so a repo that ignored one temp file suddenly rendered node_modules, .venv and dist into its file structure. Measured against real binaries on a scratch repo whose config sets only *.tmp: before, the tree carried .venv, dist and node_modules at 535 bytes; after, 405 bytes with only .logmind and docs. The four consumers all funnel through tree.ResolveRules, so the merge lands in one function rather than at four call sites.

**Alternatives considered:** Resolve overlapping patterns by letting any matching negation win, wherever it sits in the merged list. That is what this branch shipped first, and an adversarial panel blocked it: inside a single .gitignore reading `!important.log` then `*.log`, real git answers `.gitignore:2:*.log` — ignored, last match wins — while negation-wins answered not-ignored. The divergence was far broader than the cross-source case that justified it, and it left config no way to re-exclude what .gitignore had negated. The evidence originally offered for it was real but misdiagnosed: a repo with no config did receive DefaultConfig's sixteen patterns at the config source's position, so positional resolution did let a re-appended dist override a !dist. The defect was the mis-ranked defaults, not the positional rule.

**Implications:**
- The built-in defaults become a source of their own, config.DefaultIgnorePatterns, and DefaultConfig leaves FileStructure.IgnorePatterns nil — so a non-empty value there means the user's config.yml set the key and nothing else can, which makes the mis-ranking unrepresentable rather than corrected. Resolution is then strictly positional, defaults then gitignore then config then extras, last match wins, per SPEC §1.4 and git alike.
- dedup keeps the last occurrence of a repeated rule rather than the first: under last-match-wins, dropping the later copy moves a decision earlier and changes verdicts, which `*.log` / `!important.log` / `*.log` demonstrates against git.
- Seven fixtures are now compared against `git check-ignore` directly rather than against hand-written expectations; the oracle needs two invocations because `-v` redefines the exit code as "a pattern matched", so a path re-included by a trailing negation still exits 0.
- DefaultMap renders DefaultIgnorePatterns instead of keeping a second hand-maintained copy, so what `logmind config list` prints and what `config set` writes back come from one owner. The remaining #304 gap is unchanged and still filed separately: `config set` materialises the defaults into the user file, and under positional resolution that materialised copy now ranks as config — correct per §1.4, but a reason to fix #304 rather than to live with it.

---

## 2026-08-15 13:54 - tree: give the built-in ignore defaults their own source, then resolve positionally like git

**Reasoning:** The panel refuted the negation-wins resolution I approved a round earlier. Its justification covered only collisions across sources, but the implementation applied everywhere, so a single gitignore containing a negation followed by a broader pattern disagreed with git itself. Real git ignores important.log given bang-important.log then star-dot-log, because the last matching rule wins; logmind did not. The actual defect was never that positional resolution is wrong. It is that DefaultConfig seeded the ignore list, so a repository with no config file still reached the resolver carrying sixteen patterns wearing the config source's identity, in last position, where any positional rule mis-ranks them.

**Alternatives considered:** Keep negation-wins and document the divergence for users. Rejected: it diverges from git in cases the justification never covered, leaves config no way to re-exclude what gitignore negated, and asks every reader to learn a second set of ignore semantics. Also rejected: a wasSet sentinel alongside the seeded defaults, because it leaves a third state that can still be mis-ranked. Unseeding removes the state rather than detecting it.

**Implications:**
- Verified against git check-ignore as an oracle across seven fixtures, all agreeing, including the three orderings of a negation among broader patterns. The oracle itself needed care: check-ignore -v exits zero whenever any pattern matched, including a path a trailing negation re-included, so reading its exit code as the verdict inverts every negation row and would have agreed with a broken resolver. Fifteen mutations, all compiled, all died; one from the previous round had survived because ResolveRules' variadic extras had no production caller and no test at all. Dedup now keeps the last occurrence rather than the first, which is verdict-preserving under last-match-wins where keep-first is not.

---

