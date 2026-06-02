# Installing logmind

logmind ships as a single self-contained binary. Pick the install method
that matches your platform and preferences.

> **Heads up:** the v1.0 binary release lives on the `v1-go-rewrite`
> branch and ships when the cutover PR merges to `main`. Until then the
> `brew install thrillmade/tap/logmind` and `curl logmind.dev/install.sh`
> paths described below are wired up but the v1.0 tag hasn't been pushed
> yet. v0.6.x users — see the [Deprecated: Python install](#deprecated-python-install)
> section.

## Homebrew (recommended on macOS and Linux)

```bash
brew install thrillmade/tap/logmind
```

This installs the latest signed + notarized release from
[`thrillmade/homebrew-tap`](https://github.com/thrillmade/homebrew-tap),
keyed to your CPU architecture (Apple Silicon, Intel, ARM64 Linux, x86_64
Linux). Brew handles `$PATH` setup for you.

To upgrade later:

```bash
brew upgrade thrillmade/tap/logmind
```

## curl one-liner

```bash
curl -fsSL logmind.dev/install.sh | sh
```

Detects your OS + architecture, downloads the matching tarball from the
latest [GitHub Release](https://github.com/thrillmade/logmind/releases),
verifies the SHA256 against the published `SHA256SUMS` file, and drops
the binary into `~/.local/bin/logmind`. No sudo required.

Override defaults:

```bash
# Install system-wide
curl -fsSL logmind.dev/install.sh | sh -s -- --prefix=/usr/local

# Pin to a specific version
curl -fsSL logmind.dev/install.sh | sh -s -- --version=v1.0.0
```

If `~/.local/bin` isn't on your `$PATH`, the installer prints the exact
line to add to your shell rc. We deliberately don't auto-edit dotfiles —
too many shells, too easy to clobber the wrong thing.

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
go install github.com/thrillmade/logmind/cmd/logmind@v1.0.0
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
| Linux      | x86_64       | brew, curl, manual  | No*      |
| Linux      | ARM64        | brew, curl, manual  | No*      |
| Windows    | x86_64       | manual              | No*      |

*Code signing for Linux/Windows isn't established yet — the releases
ship plain binaries with SHA256 checksums for integrity verification.
Linux distros' package manager signatures (Homebrew on Linux uses the
same path as macOS) cover the trust gap.

## Verifying your install

```bash
logmind --version
# logmind 1.0.0 (spec 0.1.0)
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

A full consumer-repo migration recipe lands alongside the v1.0 cutover
PR (Phase 3) — it will live in `docs/migrate-from-pip.md` once published.

## Troubleshooting

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
