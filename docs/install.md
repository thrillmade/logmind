# Installing logmind

logmind ships as a single self-contained binary. Pick the install method
that matches your platform and preferences.

> **Heads up:** v2.0.0 is the current release line (Go binary, main
> branch). v0.6.x Python users — see the
> [Deprecated: Python install](#deprecated-python-install) section.

## Homebrew (recommended on macOS)

```bash
brew install thrillmade/tap/logmind
```

This installs the latest signed + notarized release from
[`thrillmade/homebrew-tap`](https://github.com/thrillmade/homebrew-tap),
keyed to your CPU architecture (Apple Silicon, Intel). Brew handles
`$PATH` setup for you. The tap ships a macOS-only cask — on Linux, use the
[curl one-liner](#curl-one-liner) below.

The fully-qualified name (`thrillmade/tap/logmind`) auto-trusts the cask
under Homebrew 6.0.0's [tap trust](https://docs.brew.sh/Tap-Trust) rules,
so no extra `brew trust` step is needed.

To upgrade later:

```bash
brew upgrade thrillmade/tap/logmind
```

> **Seeing a "tap trust is required" warning about `thrillmot/logmind`?**
> That's a stale personal tap from before the `thrillmade` org migration.
> Run `brew untap thrillmot/logmind` — see
> [Troubleshooting](#troubleshooting).

## curl one-liner

```bash
curl -fsSL logmind.dev/install.sh | bash
```

Detects your OS + architecture, downloads the matching tarball from the
latest [GitHub Release](https://github.com/thrillmade/logmind/releases),
verifies the SHA256 against the published `SHA256SUMS` file, and drops
the binary into `~/.local/bin/logmind`. No sudo required.

Override defaults:

```bash
# Install system-wide
curl -fsSL logmind.dev/install.sh | bash -s -- --prefix=/usr/local

# Pin to a specific version (flag form)
curl -fsSL logmind.dev/install.sh | bash -s -- --version=v2.0.0

# Pin to a specific version (env form — equivalent, lower precedence than --version)
LOGMIND_VERSION=v2.0.0 curl -fsSL logmind.dev/install.sh | bash
```

Re-running the installer when the same version is already installed
exits with a fast "already installed" message — idempotent, safe to
drop into bootstrap scripts.

If `~/.local/bin` isn't on your `$PATH`, the installer prints the exact
line to add to your shell rc. We deliberately don't auto-edit dotfiles —
too many shells, too easy to clobber the wrong thing.

## GitHub Actions

For CI use the [`thrillmade/setup-logmind`](https://github.com/thrillmade/setup-logmind)
action instead of curl-install:

```yaml
- uses: thrillmade/setup-logmind@v1
  with:
    token: ${{ github.token }}
- run: logmind check-links
```

The `token: ${{ github.token }}` line matters: composite actions can't
default an input to `github.token`, so without it setup-logmind's
release-lookup call is anonymous — shared GitHub-hosted-runner IP
ranges routinely exhaust the unauthenticated `api.github.com` rate
limit and 403 before logmind installs. Pass it explicitly.

`logmind init` (v1.1.0+) installs a `.github/dependabot.yml` block that
groups `thrillmade/*` action bumps, so Dependabot opens one PR per
logmind release. You pin once and the ecosystem keeps you current.

If `curl logmind.dev/install.sh | bash` ever slips into a workflow
file, the installer detects `GITHUB_ACTIONS=true` and prints a one-line
nudge at the end of the run pointing at `setup-logmind`.

## Manual download

Grab the matching archive from the
[Releases page](https://github.com/thrillmade/logmind/releases) and
extract the `logmind` binary into any directory on your `$PATH`.

Verify the checksum first:

```bash
shasum -a 256 logmind_VERSION_OS_ARCH.tar.gz
# Compare against the matching line in SHA256SUMS (also in the release assets)
```

## Build from source

If you have Go 1.22 or newer:

```bash
go install github.com/thrillmade/logmind/cmd/logmind@latest
```

Builds the latest tagged release into `$(go env GOPATH)/bin/logmind`.

For a specific tag:

```bash
go install github.com/thrillmade/logmind/cmd/logmind@v2.0.0
```

Or clone + build:

```bash
git clone https://github.com/thrillmade/logmind.git
cd logmind
make build           # writes ./bin/logmind
./bin/logmind --version
```

`go install` builds are unsigned. They run fine on Linux. On macOS
Gatekeeper may complain about an unsigned binary the first time you
launch it; right-click → Open in Finder once to grant permission, or
prefer the brew/curl paths above which deliver signed + notarized
binaries.

## Platform support matrix

| Platform   | Architecture | Install paths       | Signed   |
|------------|--------------|---------------------|----------|
| macOS      | Apple Silicon | brew, curl, manual | Yes      |
| macOS      | Intel        | brew, curl, manual  | Yes      |
| Linux      | x86_64       | curl, manual        | No*      |
| Linux      | ARM64        | curl, manual        | No*      |
| Windows    | x86_64       | manual              | No*      |

*Code signing for Linux/Windows isn't established yet — the releases
ship plain binaries with SHA256 checksums for integrity verification.
The curl installer verifies the download against `SHA256SUMS` before
installing, which covers the trust gap. Homebrew is a macOS-only
distribution path here — the `thrillmade/tap/logmind` cask does not
target Linux.

## Verifying your install

```bash
logmind --version
# logmind 2.0.0 (spec 1.5.0)
```

The `--version` line is the protocol contract — downstream tooling
(clud-bug, tokenomics, agent-skills) reads it to detect protocol skew.
If the line doesn't appear or the format differs, your install is broken
or running an older version.

## Deprecated: Python install

Prior to v1.0, logmind shipped as a Python package on PyPI. The Python
distribution is **deprecated** as of v1.0 but still works for v0.6.x:

```bash
# Deprecated — pinned to v0.6.x
pipx install logmind
```

The deprecated path is documented for users on long-lived consumer
repos that haven't yet swapped pip-install for brew/curl in their
workflow YAML. Migration is a one-line change in your CI: replace
`pip install logmind==0.6.X` with `brew install thrillmade/tap/logmind`.

## Troubleshooting

- **"Homebrew is currently ignoring formulae, casks and commands from
  these taps because tap trust is required" (mentions
  `thrillmot/logmind`)**
  Homebrew 6.0.0 (June 2026) added [tap
  trust](https://docs.brew.sh/Tap-Trust): non-official third-party taps
  must be explicitly trusted before their Ruby code runs, and any tap
  that isn't trusted is ignored with this warning. The offender here is
  the **stale personal tap** `thrillmot/logmind` — a pre-v0.3.1 install
  path that predates the org migration to `thrillmade`. It was never the
  canonical tap and nothing installs from it anymore. Remove it:

  ```bash
  brew untap thrillmot/logmind
  ```

  Then install (or reinstall) from the canonical tap:

  ```bash
  brew install thrillmade/tap/logmind
  ```

  **You do not need to run `brew trust` for the canonical path.** A
  fully-qualified install (`brew install thrillmade/tap/logmind`)
  auto-trusts that cask before installing — that's why the command above
  Just Works under the new rules, and it's the form we document
  everywhere. Tap trust is entirely client-side: the tap publisher signs
  or attests nothing, so there is nothing for `thrillmade/homebrew-tap`
  to change on its end.

  Only if you prefer to tap first and install by the short name do you
  need an explicit trust step:

  ```bash
  brew tap thrillmade/tap
  brew trust thrillmade/tap   # required for the UNqualified `brew install logmind`
  brew install logmind
  ```

  (`export HOMEBREW_NO_REQUIRE_TAP_TRUST=1` disables the check globally,
  but Homebrew warns it's unsafe and slated for removal — untap the stale
  tap instead.)

- **"command not found: logmind" after install**
  Your install prefix's `bin/` directory isn't on `$PATH`. The curl
  installer prints the exact `export PATH=...` line to add. For `go
  install` builds, ensure `$(go env GOPATH)/bin` is on your `$PATH`.

- **"developer cannot be verified" Gatekeeper dialog on macOS**
  Only happens with the `go install` / manual unsigned build. Use brew
  or curl install paths, which ship signed + notarized binaries.

- **"checksum mismatch" from the curl installer**
  Either the release was malformed (file an issue) or your download was
  corrupted in transit (try again). The installer refuses to install in
  either case — that's the point.

- **Brew install fails on an old macOS (< Big Sur)**
  The signed binary requires Hardened Runtime support, which requires
  macOS 11+. On older macOS, use `go install` or manual download.
