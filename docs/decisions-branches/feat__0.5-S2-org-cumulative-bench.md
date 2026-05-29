## 2026-05-29 16:31 - Implement bench/org_cumulative real impl (Phase 0.5 §2, v0.5.7)

**Reasoning:** Closes the last Q7-logmind bench stub. The 4-angle frame is now fully populated (per-call ✅, worst-case ✅, per-session ✅ [v0.5.6], org-cumulative ✅ [this]). Walks the same session JSONLs as per_session.py via shared helpers (_session_paths, _session_cwd, _is_logmind_repo, _walk_reads, _bucket, _git_equivalent_bytes), then aggregates DIFFERENTLY — sums bytes across sessions+repos for one global net_pct + per_repo_share, instead of per-session avg. Same informational-only treatment as per-session (shared thin git baseline; sign isn't a quality signal). Load-bearing data is per_repo_share, used by Step 4 validation to spot per-consumer cache-key regressions (>2× median share).

**Alternatives considered:** Different baseline for org-cumulative (e.g., session-aware reconstruction) — deferred: needs a defined no-logmind-world comparator before sign becomes interpretable; informational treatment matches per_session's same constraint, Walk consuming-repo git history for logmind-log frequency — rejected as primary signal: requires multi-repo enumeration + auth; session-log aggregation is the same fidelity at zero infrastructure cost

**Implications:**
- Bumps to v0.5.7. CHANGELOG entry. 3 new tests in tests/test_bench.py (no-sessions stub, fixture aggregation across 2 repos, per-repo outlier detection at 3:1 ratio) + 1 pin-test (stub shim must delegate to real impl, not silently revert to placeholder shape). bench/__main__.py informational set now {per-session, org-cumulative}; verdict label updated to Step 4 / 0.B.5 / 0.B.6 inputs to reflect dual consumer.

---
