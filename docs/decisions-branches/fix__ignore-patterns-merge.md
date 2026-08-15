← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-tree-union-the-ignore-sources-instead-of-letting-config-repl -->
- **2026-08-15** — tree: union the ignore sources instead of letting config replace the defaults
<!-- logmind-entry-end -->

## 2026-08-15 09:08 - tree: union the ignore sources instead of letting config replace the defaults

**Reasoning:** Setting a single file_structure.ignore_patterns entry replaced the built-in list wholesale, so a repo that ignored one temp file suddenly rendered node_modules, .venv and dist into its file structure. Measured against real binaries on a scratch repo whose config sets only *.tmp: before, the tree carried .venv, dist and node_modules at 535 bytes; after, 405 bytes with only .logmind and docs. The four consumers all funnel through tree.ResolveRules, so the merge lands in one function rather than at four call sites.

**Alternatives considered:** Resolve overlapping patterns positionally, last match wins, the way gitignore does. Rejected on evidence: a repo with no config still receives DefaultConfig's sixteen patterns, and they arrive at ResolveRules as the config source, so a positional resolver lets a re-appended dist override a !dist the repository's own gitignore already re-included. Probing the resolved list showed rule[0]={dist true} against rule[11]={dist false}. Negation wins instead, and the reasoning sits in the IgnoreRules.Matches doc comment.

**Implications:**
- IgnoreRules becomes a slice of Rule so the ! prefix is parsed in exactly one place for defaults, gitignore and config alike; nine mutations were applied, all nine compiled and all nine died. logmind config list still prints only the configured patterns while the effective set is seventeen, which is filed separately rather than fixed here, because merging there would bake the defaults into the user file on the next config set.

---

