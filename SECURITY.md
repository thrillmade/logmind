# Security Policy

## Supported versions

logmind follows semantic versioning. Until 1.0, only the latest minor
version on the `main` branch receives security fixes.

| Version | Supported          |
|---------|--------------------|
| 0.1.x   | :white_check_mark: |
| < 0.1   | :x:                |

## Reporting a vulnerability

Please report security issues privately so we can ship a fix before public
disclosure.

**Preferred:** open a private security advisory via GitHub:
<https://github.com/thrillmade/logmind/security/advisories/new>

**Alternate:** email **security@logmind.dev** with:
- A description of the issue and its impact
- Steps to reproduce (a minimal repro is best)
- The logmind version and Python version

We will acknowledge within 3 business days, target an initial assessment
within 7 days, and aim to publish a fixed release plus advisory within 30
days for high-severity issues.

## What's in scope

- The `logmind` package itself (CLI, library, GitHub Actions installed by
  `logmind init`).
- Default `.github/workflows/*.yml.template` content shipped in the wheel.

## What's out of scope

- The user's own code that calls `logmind.log()`.
- Third-party dependencies (`click`, `pyyaml`, `langchain-core`) — please
  report those upstream; we will pull the fix in once it's released.
- Vulnerabilities only reproducible with `auto_commit: false` and no git
  remote configured (logmind never makes network calls in that mode).
