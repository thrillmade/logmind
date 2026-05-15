# Contributing to logmind

Thanks for considering a contribution! logmind is small and the bar to land
useful improvements is low. This file covers the dev loop, the conventions
we keep, and how to ship a release.

## Quick start

```bash
git clone https://github.com/thrillmot/logmind
cd logmind
python3 -m venv venv
source venv/bin/activate
pip install -e ".[dev]"
pytest -q                  # full test suite (~430 tests)
logmind check-links        # docs link integrity (also runs in CI)
```

## Repo layout

- `src/logmind/` — installable package (`pip install logmind`)
- `src/logmind/core/` — core modules (`logger`, `git_handler`, `inserter`,
  `tree_gen`, `gitignore`, `skill_install`, `analytics`, `aggregator`, ...)
- `src/logmind/actions/` — entry points designed to run inside CI
  (`python -m logmind.actions.aggregate`, `python -m logmind.actions.link_check`)
- `src/logmind/templates/` — files written into target projects by `logmind init`
- `src/logmind/templates/github/` — workflow templates installed into
  `<target>/.github/workflows/`
- `tests/` — pytest tests with fixtures in `tests/conftest.py`
  (`temp_dir`, `git_repo`, `docs_dir`)
- `skill/` — content for the standalone `logmind-skill` repo published
  via skills.sh; not shipped in the wheel

## Conventions

### Decision logging

We dogfood logmind. Open a feature branch, then for any non-trivial
choice run:

```bash
logmind log "Short summary" -r "why" -a "alt 1" -i "implication"
```

Branch-aware: feature branches write to
`docs/decisions-branches/<branch>.md`; the default branch writes to
`docs/decisions.md`. The PR-merge GH Action then appends a one-line
summary linking your PR.

### Tests

- Use the existing fixtures (`temp_dir`, `git_repo`, `docs_dir`).
- For anything that touches branches, init with `git init -b main` so
  the test is independent of `init.defaultBranch` config.
- Don't depend on the caller's cwd — pass paths explicitly. (See
  `_resolve_decisions_path` in `src/logmind/core/logger.py` for the
  pattern that fixed a real bug.)

### Style

- `black` and `ruff` enforce formatting; `mypy` runs in lax mode for
  now (we'll tighten over time). All three are wired into
  `.pre-commit-config.yaml`:
  ```bash
  pip install pre-commit && pre-commit install
  ```

### Markdown links

`logmind check-links` runs in CI on every PR touching `*.md` files.
Broken links and orphaned `docs/*.md` files fail the build. Keep
relative links relative; allowlist intentional orphans via
`linkcheck.allow_orphans` in `.logmind/config.yml`.

## Pull request checklist

- [ ] Tests added or updated (`pytest -q` is green locally)
- [ ] `logmind check-links` is green
- [ ] A decision logged (`logmind log "..."`) for any architectural change
- [ ] CHANGELOG updated under `[Unreleased]`
- [ ] No committed `dist/`, `__pycache__/`, or other generated artefacts

## Release process

logmind releases are tag-driven. To cut `vX.Y.Z`:

1. Bump `version` in `pyproject.toml` to `X.Y.Z`.
2. Move `[Unreleased]` content in `CHANGELOG.md` to a new `[X.Y.Z] - <date>`
   section. Re-create an empty `[Unreleased]` block at the top.
3. Commit and merge to `main`.
4. Tag and push:
   ```bash
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```
5. The `publish.yml` workflow builds the sdist + wheel and pushes to PyPI
   via OIDC (no API token in CI), then creates a GitHub Release with the
   artifacts and the matching `CHANGELOG.md` excerpt.
6. Update the Homebrew tap in the separate `homebrew-logmind` repo with
   the real PyPI tarball SHA256.

## Reporting issues / proposing features

Use the issue templates in `.github/ISSUE_TEMPLATE/`. For security
vulnerabilities please follow `SECURITY.md` (private disclosure first).

## Branch protection

After the first green CI run on `main`, enable branch protection requiring:
- The `test` workflow to pass
- The `doc link integrity` workflow to pass
- At least one approving review

(GitHub UI → Settings → Branches; not automated yet.)
