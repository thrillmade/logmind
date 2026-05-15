<!-- Thanks for the PR! Brief context first, then the checklist. -->

## What this changes

<!-- One paragraph. What does the diff actually do, and why? -->

## How to verify

<!-- The exact commands a reviewer can run locally to see it work. -->

## Checklist

- [ ] Tests added / updated; `pytest -q` is green locally
- [ ] `logmind check-links` is green (run it before pushing)
- [ ] Architectural change? Logged via `logmind log "..."` on this branch
- [ ] CHANGELOG updated under `[Unreleased]` if user-visible
- [ ] No committed `dist/`, `__pycache__/`, or other generated artefacts
