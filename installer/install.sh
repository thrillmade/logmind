#!/usr/bin/env bash
# installer/install.sh — one-shot logmind binary installer.
#
# Distributed at https://logmind.dev/install.sh (hosted by the
# logmind-site Vercel project — separate repo; this PR ships the
# script, deployment to the CDN URL is a follow-up).
#
# Usage:
#   curl -fsSL logmind.dev/install.sh | bash
#   curl -fsSL logmind.dev/install.sh | bash -s -- --prefix=$HOME/.local
#   curl -fsSL logmind.dev/install.sh | bash -s -- --version=v1.0.0
#
# What it does:
#   1. Detect OS (darwin | linux) + arch (x86_64 | arm64).
#   2. Resolve target version: default is the latest GitHub Release,
#      override via --version=vX.Y.Z.
#   3. Download the matching archive + SHA256SUMS file from the GitHub
#      Release assets.
#   4. Verify the archive against the SHA256SUMS line.
#   5. Extract, chmod +x, move to $PREFIX/bin/logmind.
#   6. Run `logmind --version` as a self-check.
#
# Default install prefix:
#   - $HOME/.local — no sudo required. Users with `~/.local/bin` already
#     in $PATH get a working binary immediately. We deliberately don't
#     default to /usr/local because every paste-curl-install script that
#     requires sudo trips a security-conscious user's flag — and
#     ~/.local/bin is the XDG-compatible spot.
#   - Override with --prefix=/usr/local for system-wide install. If
#     /usr/local/bin isn't user-writable the script will print a
#     `sudo mv` hint instead of silently failing.
#
# Windows: not supported by this installer. Use scoop / winget / direct
# download from the GitHub Release page. (Bash on Git Bash WOULD work,
# but the archive extraction logic targets POSIX tar.)
#
# This script is intentionally one file with no external deps beyond
# coreutils + curl/wget + tar + (sha256sum | shasum). Maintainability
# wins out over modularity at this size; longer is fine.

# Bash-only — the script uses [[ ]], $'...' ANSI-C quoting, and
# `set -o pipefail`. When piped through `sh` (Debian/Ubuntu point `sh`
# at `dash`), those constructs are no-ops or hard errors — most
# importantly the [[ ]] in the checksum verification block below would
# silently evaluate as false, letting a tampered archive install. So we
# detect dash/ash/posh up front and refuse with a corrective message
# BEFORE the first set/pipefail line that would error on those shells.
#
# BASH_VERSION is unset under any non-bash shell; the POSIX `[ -z ... ]`
# probe works across dash, ash, posh, and the POSIX sh in Solaris/BSDs.
if [ -z "${BASH_VERSION:-}" ]; then
  printf 'logmind installer needs bash, not sh.\n' >&2
  printf 'Re-run with: curl -fsSL logmind.dev/install.sh | bash\n' >&2
  exit 1
fi

set -euo pipefail

# ----------------------------------------------------------------------
# Config + flag parsing
# ----------------------------------------------------------------------
REPO="thrillmade/logmind"
DEFAULT_PREFIX="${HOME}/.local"
PREFIX=""
TARGET_VERSION=""

# Color helpers — disabled when stdout isn't a TTY (e.g. piped to less).
if [[ -t 1 ]] && [[ -t 2 ]]; then
  C_BOLD=$'\033[1m'
  C_DIM=$'\033[2m'
  C_RED=$'\033[31m'
  C_GREEN=$'\033[32m'
  C_RESET=$'\033[0m'
else
  C_BOLD=""; C_DIM=""; C_RED=""; C_GREEN=""; C_RESET=""
fi

info()  { printf "%s==>%s %s\n" "${C_GREEN}${C_BOLD}" "${C_RESET}" "$*"; }
warn()  { printf "%s==>%s %s\n" "${C_RED}${C_BOLD}" "${C_RESET}" "$*" >&2; }
fatal() { warn "$*"; exit 1; }
dim()   { printf "%s%s%s\n" "${C_DIM}" "$*" "${C_RESET}"; }

usage() {
  cat <<EOF
${C_BOLD}logmind installer${C_RESET}

Usage: install.sh [--prefix=PATH] [--version=vX.Y.Z]

Options:
  --prefix=PATH      Install binary under PATH/bin/. Default: ${DEFAULT_PREFIX}
  --version=vX.Y.Z   Specific release version. Default: latest stable.
  --help, -h         Show this help.

Examples:
  curl -fsSL logmind.dev/install.sh | bash
  curl -fsSL logmind.dev/install.sh | bash -s -- --prefix=/usr/local
  curl -fsSL logmind.dev/install.sh | bash -s -- --version=v1.0.0

Report install bugs to https://github.com/${REPO}/issues
EOF
}

for arg in "$@"; do
  case "${arg}" in
    --prefix=*)  PREFIX="${arg#*=}";;
    --version=*) TARGET_VERSION="${arg#*=}";;
    -h|--help)   usage; exit 0;;
    *)
      warn "unknown option: ${arg}"
      usage
      exit 64
      ;;
  esac
done

PREFIX="${PREFIX:-${DEFAULT_PREFIX}}"
BIN_DIR="${PREFIX}/bin"

# ----------------------------------------------------------------------
# OS + arch detection
# ----------------------------------------------------------------------
detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin";;
    Linux)  echo "linux";;
    *)      fatal "unsupported OS: $(uname -s). See https://github.com/${REPO}#install for alternatives.";;
  esac
}

detect_arch() {
  # The archive name embeds x86_64 / arm64 (matches goreleaser's
  # name_template in .goreleaser.yaml). We normalise uname output to
  # those exact strings.
  local raw
  raw="$(uname -m)"
  case "${raw}" in
    x86_64|amd64)     echo "x86_64";;
    arm64|aarch64)    echo "arm64";;
    *) fatal "unsupported arch: ${raw}";;
  esac
}

OS="$(detect_os)"
ARCH="$(detect_arch)"

# ----------------------------------------------------------------------
# HTTP helpers — prefer curl, fall back to wget
# ----------------------------------------------------------------------
have() { command -v "$1" >/dev/null 2>&1; }

if have curl; then
  fetch() { curl -fsSL "$1" -o "$2"; }
  fetch_stdout() { curl -fsSL "$1"; }
elif have wget; then
  fetch() { wget -q -O "$2" "$1"; }
  fetch_stdout() { wget -q -O - "$1"; }
else
  fatal "need curl or wget to download. Install one and retry."
fi

# ----------------------------------------------------------------------
# Resolve target tag
# ----------------------------------------------------------------------
if [[ -z "${TARGET_VERSION}" ]]; then
  info "resolving latest logmind release..."
  # GitHub's /latest endpoint redirects to /tag/<latest>. We follow the
  # Location header and pull the tag out of the final URL — avoids JSON
  # parsing (no jq dep) and works even if the JSON shape changes.
  if have curl; then
    LATEST_URL="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest")"
  else
    # wget doesn't expose the final URL cleanly; fall back to parsing
    # the API. We accept the minor jq-free risk here because curl is
    # available on virtually every modern Mac + Linux distro.
    LATEST_URL="$(wget -q --max-redirect 0 -S "https://github.com/${REPO}/releases/latest" 2>&1 | sed -n 's/^[[:space:]]*[Ll]ocation: //p' | head -n1)"
  fi
  TARGET_VERSION="${LATEST_URL##*/}"
  if [[ -z "${TARGET_VERSION}" || "${TARGET_VERSION}" == "latest" ]]; then
    fatal "could not resolve latest release. Pass --version=vX.Y.Z to override."
  fi
fi

# Strip leading "v" for the archive name (matches goreleaser's
# .Version template which omits the "v" prefix).
VERSION_BARE="${TARGET_VERSION#v}"

info "logmind ${C_BOLD}${TARGET_VERSION}${C_RESET} for ${OS}/${ARCH}"

# ----------------------------------------------------------------------
# Compose download URLs + paths
# ----------------------------------------------------------------------
ARCHIVE_NAME="logmind_${VERSION_BARE}_${OS}_${ARCH}.tar.gz"
ARCHIVE_URL="https://github.com/${REPO}/releases/download/${TARGET_VERSION}/${ARCHIVE_NAME}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${TARGET_VERSION}/SHA256SUMS"

TMP_DIR="$(mktemp -d -t logmind-install.XXXXXX)"
trap 'rm -rf "${TMP_DIR}"' EXIT

ARCHIVE_PATH="${TMP_DIR}/${ARCHIVE_NAME}"
CHECKSUMS_PATH="${TMP_DIR}/SHA256SUMS"

# ----------------------------------------------------------------------
# Download
# ----------------------------------------------------------------------
info "downloading ${ARCHIVE_NAME}"
fetch "${ARCHIVE_URL}" "${ARCHIVE_PATH}"

info "downloading SHA256SUMS"
fetch "${CHECKSUMS_URL}" "${CHECKSUMS_PATH}"

# ----------------------------------------------------------------------
# Verify checksum
# ----------------------------------------------------------------------
# Prefer sha256sum (Linux coreutils), fall back to shasum (BSD/Mac).
if have sha256sum; then
  SHA_CMD="sha256sum"
elif have shasum; then
  SHA_CMD="shasum -a 256"
else
  fatal "need sha256sum or shasum to verify download. Install coreutils and retry."
fi

EXPECTED_LINE="$(grep "  ${ARCHIVE_NAME}\$" "${CHECKSUMS_PATH}" || true)"
if [[ -z "${EXPECTED_LINE}" ]]; then
  fatal "no checksum line for ${ARCHIVE_NAME} in SHA256SUMS. Release malformed?"
fi
EXPECTED_SHA="${EXPECTED_LINE%%  *}"

ACTUAL_SHA="$(${SHA_CMD} "${ARCHIVE_PATH}" | awk '{print $1}')"

if [[ "${EXPECTED_SHA}" != "${ACTUAL_SHA}" ]]; then
  warn "checksum mismatch!"
  dim "  expected: ${EXPECTED_SHA}"
  dim "  actual:   ${ACTUAL_SHA}"
  fatal "refusing to install. File the bug at https://github.com/${REPO}/issues"
fi

info "checksum verified"

# ----------------------------------------------------------------------
# Extract + install
# ----------------------------------------------------------------------
info "extracting"
tar -xzf "${ARCHIVE_PATH}" -C "${TMP_DIR}"

EXTRACTED_BIN="${TMP_DIR}/logmind"
if [[ ! -x "${EXTRACTED_BIN}" ]]; then
  # Some tarballs put the binary inside a subdir; check there too.
  EXTRACTED_BIN="$(find "${TMP_DIR}" -name logmind -type f -perm -u+x | head -n1)"
  if [[ -z "${EXTRACTED_BIN}" ]]; then
    fatal "no executable named 'logmind' in archive"
  fi
fi

mkdir -p "${BIN_DIR}" || true

DEST="${BIN_DIR}/logmind"
if mv "${EXTRACTED_BIN}" "${DEST}" 2>/dev/null; then
  info "installed: ${C_BOLD}${DEST}${C_RESET}"
else
  warn "could not write to ${DEST} (permission denied)"
  dim "  try: sudo install -m 0755 \"${EXTRACTED_BIN}\" \"${DEST}\""
  dim "  or:  install.sh --prefix=\$HOME/.local"
  exit 1
fi

chmod +x "${DEST}"

# ----------------------------------------------------------------------
# Self-check + PATH advisory
# ----------------------------------------------------------------------
if ! "${DEST}" --version >/dev/null 2>&1; then
  fatal "${DEST} --version failed. Binary may be corrupted; please re-run installer."
fi

VERSION_OUTPUT="$(${DEST} --version)"
info "${VERSION_OUTPUT}"

# Check if BIN_DIR is in PATH; nag the user once if not. We don't
# auto-edit shell rcs (too many shells, too much surface area for the
# user's expectations). Just point at the line they need to add.
if ! echo ":${PATH}:" | grep -q ":${BIN_DIR}:"; then
  echo ""
  warn "${BIN_DIR} is not in your \$PATH"
  dim "  add this to your shell rc file (~/.zshrc, ~/.bashrc, ~/.config/fish/config.fish):"
  dim "    export PATH=\"${BIN_DIR}:\$PATH\""
fi

echo ""
info "${C_BOLD}logmind${C_RESET} installed. Run ${C_BOLD}logmind --help${C_RESET} to get started."
