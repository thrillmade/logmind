# Security Policy

## Supported versions

logmind follows semantic versioning. Only the 2.x line (the Go binary)
receives security fixes.

| Version                | Supported          |
|-------------------------|--------------------|
| 2.x                     | :white_check_mark: |
| 1.x                     | :x:                |
| 0.6.x (Python, frozen)  | :x:                |
| < 0.6                   | :x:                |

The Python wheel is frozen at v0.6.16 — the last published Python release
before the Go rewrite. It receives no further updates, including security
backports. New installs should use the Go binary; see
[docs/install.md](docs/install.md) for the migration path.

## Reporting a vulnerability

Please report security issues privately so we can ship a fix before public
disclosure.

**Preferred:** open a private security advisory via GitHub:
<https://github.com/thrillmade/logmind/security/advisories/new>

**Alternate:** email **security@logmind.dev** with:
- A description of the issue and its impact
- Steps to reproduce (a minimal repro is best)
- The `logmind --version` output (`logmind X.Y.Z (spec A.B.C)`) and your
  OS/architecture

We will acknowledge within 3 business days, target an initial assessment
within 7 days, and aim to publish a fixed release plus advisory within 30
days for high-severity issues.

## What's in scope

- The `logmind` Go binary (CLI + the packages under `internal/`).
- The GitHub Actions workflows, git hooks, and `.gitattributes` /
  merge-driver config installed by `logmind init` / `logmind doctor --fix`.
- The curl installer (`installer/install.sh`, served from
  `logmind.dev/install.sh`) and its checksum verification.

## What's out of scope

- The user's own code and decision content logged via `logmind log`.
- Third-party Go module dependencies — please report those to the
  upstream project; we will pull the fix in once it's released.
- The frozen Python wheel (`pip install logmind==0.6.16`) — unsupported,
  see [Supported versions](#supported-versions) above.
- Vulnerabilities only reproducible with `git.auto_commit: false` and no
  git remote configured (logmind never makes network calls in that mode).
