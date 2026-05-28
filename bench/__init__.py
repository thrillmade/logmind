"""logmind-bench — Q7-logmind enforcement (internal tool).

NOT shipped to PyPI. Lives in the repo as a bench/ directory + a nightly
.github/workflows/bench.yml that runs it.

The Phase 0.5 plan defines Q7-logmind: logmind must be a net token
saver, measured along 4 angles (per-call, per-session amortization, org
cumulative, worst-case). This package implements each angle as a
self-contained module that emits a structured result; `python -m bench`
runs all four and aggregates.
"""

__version__ = "0.1.0"
