# `logmind-bench` — Q7-logmind enforcement (internal)

> **NOT** released to PyPI. Internal QA gate that proves logmind is a
> net token saver, not an expander. Per the Phase 0.5 plan.

## What Q7-logmind requires

Four angles, all must be green:

| Angle | What it proves | Implementation |
|---|---|---|
| **Per-call** | `logmind <cmd>` emits ≤ bytes of manual git equivalent | `bench/per_call.py` — tempdir harness, run each logmind command vs its git-equivalent, count UTF-8 bytes |
| **Per-session amortization** | Future agent reads of generated artifacts save more than per-call cost | `bench/per_session.py` — sample 10 real sessions, count reads of `docs/decisions.md` / `timeline.md` / `file-structure.md` |
| **Org cumulative** | Across 5 repos over 30 days, bytes-saved > bytes-spent | `bench/org_cumulative.py` — aggregate per-call + per-session, roll up |
| **Worst-case** | Fresh session, single `logmind log`, never reads back is still ≤ break-even | `bench/worst_case.py` — synthetic stress test |

## Running

```bash
# All four angles (default)
python -m bench

# Single angle
python -m bench per-call
python -m bench worst-case

# Output JSON for downstream
python -m bench --json > bench/last-run.json

# Compare to a previous run (detect regressions)
python -m bench --baseline bench/last-run.json
```

## Output

```
$ python -m bench
ok: 4-angle Q7-logmind compliance
  per-call:     -88% bytes vs git equivalent  ✅ saver
  per-session:  -41% bytes amortized          ✅ saver
  org 30d:      -64% net (cumulative)         ✅ saver
  worst-case:   -12% even on never-read       ✅ saver
  all angles green
  trend: per-session amortization slightly UP since v0.5.4 timeline brief
```

If any angle flips negative, `python -m bench` exits non-zero — CI uses
this as the gate.

## CI integration

`.github/workflows/bench.yml` runs nightly + on PRs that touch
`src/logmind/` or `bench/`. Posts a summary comment if the trend
changes meaningfully.

## Why internal-only

Users don't need this. It's our QA gate. Keeps logmind's user-facing
CLI surface clean.
