← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-fix-audit-batch-crlf-determinism-push-timeout-atomic-config- -->
- **2026-07-17** — fix: audit batch - CRLF determinism, push timeout, atomic config writes, quiet help, spec 1.2.1/3.7 conformance
<!-- logmind-entry-end -->

## 2026-07-17 09:12 - fix: audit batch - CRLF determinism, push timeout, atomic config writes, quiet help, spec 1.2.1/3.7 conformance

**Reasoning:** Six adversarially-confirmed defects found ahead of the v2.0.0 tag: (1) extractEntryBlocks left a trailing CR on CRLF-sourced body lines, breaking docs/timeline.md's own byte-determinism invariant and the dedupeAndSuffix same-body carve-out; (2) gitcli.Push had no context timeout/WaitDelay so a wedged remote could hang every 'logmind log' (auto_push defaults true); (3) claudehook/hooks/inserter each overwrote an EXISTING file with a bare os.WriteFile (O_TRUNC then write), risking a truncated file on a crash mid-write; (4) the --quiet persistent flag's help text promised 'one ok line per verb' but only 9 of 22 subcommands honor it; (5) DefaultConfig's ignore_patterns omitted SPEC section-1.2.1's .next, .turbo, .DS_Store; (6) self-update had no pinVersion field at all, so SPEC section-3.7's 'no-op when pinned' requirement was unimplemented.

**Alternatives considered:** Wire --quiet through all 13 remaining verbs now instead of just fixing the help text - rejected, explicitly out of scope and a much larger surface, Skip a shared atomic-write helper and patch each of the 3 call sites ad hoc - rejected, the codebase already has two near-identical helpers (writeAtomic, realAtomicWriteFile); a third bespoke copy would be the same drift risk this fix is closing, Copy the SPEC prose's trailing-slash pattern format (.next/, .turbo/) verbatim into the Go defaults - rejected, internal/tree's matcher does a literal filepath.Match against the basename/component so a trailing slash would silently never match; used the no-trailing-slash form every sibling default already uses

**Implications:**
- New internal/atomicio package (temp-sibling + rename, mode preserved) now backs claudehook.EnsurePreToolUseGuard, hooks.installHook, and inserter.InsertLogmindSection
- gitcli.Push gained package-level pushTimeout (45s) / pushWaitDelay (2s) vars so tests can shrink them for a hermetic hang test
- Config gained a top-level PinVersion field (deliberately omitted from DefaultMap to preserve config-list Python-parity, same precedent as root_label/context.repomap/context.spec_file)
- DefaultConfig/DefaultMap ignore_patterns and the config_test.go golden both grew the three new entries; TestConfigList_DefaultsMatchPython is a spot-check so it was untouched

---

