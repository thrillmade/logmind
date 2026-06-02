## 2026-06-01 23:09 - feat(v0.6.14): propagation-pipeline polish — doctor content-diff + [skip-logmind] auto-title + PyPI CDN retry

**Reasoning:** Three follow-up fixes from v0.6.13 propagation friction. (a) doctor.py: content-diff against bundled hook body when marker matches — closes the #112-addendum 'too cheap a signal' meta-concern; catches drift where bytes differ but marker line is preserved (manual edits, clobber-then-restore, marker-propagation bugs). (b) self-update template v7: prefixes both git commit -m and gh pr create --title with [skip-logmind] — bot-generated propagation PRs are by-definition not decision commits; eliminates the manual title-edit + close+reopen ritual that v0.6.13 required for all 5/5 consumers. (c) self-update template v7: 3-attempt CDN-aware retry on the PyPI version-check (0/30/60s backoff) — closes the silent-skip class where workflow polls during PyPI CDN propagation lag and sees stale 'latest' (hit v0.6.12 1/5 + v0.6.13 1/5). Per the plan's v0.6.14 NEXT slot.

**Alternatives considered:** Fold logmind upgrade self-dispatch into v0.6.14 — deferred to v0.6.15 alongside brew-first README and v1.0-TS-rewrite plan, Wait for D.10 orchestrator App to subsume entire propagation friction class — multi-week vs 1 day

**Implications:**
- Doctor's signal becomes a TRUE staleness check, not just version marker — addresses 'too cheap' user feedback
- Next propagation cycle (v0.6.15 → 5 consumers) should be 5/5 first-attempt with no manual title edits, no close+reopen, no silent CDN-race skips — first pure self-heal cycle since LOGMIND_AUTO_REGEN_PAT scope upgrade
- v6 template's gh pr create PAT fix (shipped in v0.6.13) + v7 template's [skip-logmind] + CDN retry compose at v0.6.15 propagation

---
