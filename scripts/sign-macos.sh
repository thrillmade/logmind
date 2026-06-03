#!/usr/bin/env bash
# scripts/sign-macos.sh — codesign + notarize a single darwin binary.
#
# Invoked by GoReleaser as a per-build `hooks.post` step. Receives the
# binary path as argument 1. GOOS is exported by GoReleaser via the hook
# env block so we can no-op on non-darwin builds without parsing the
# path.
#
# Behavior:
#   - GOOS != darwin             → no-op (silent).
#   - MACOS_CERTIFICATE unset    → no-op with a one-line note. This is
#                                  the "local snapshot build" path; it
#                                  lets `goreleaser release --snapshot`
#                                  work offline.
#   - All env present + darwin   → run codesign + notarytool submit,
#                                  fail loudly on either.
#
# Required env (only when actually signing):
#   - MACOS_CERTIFICATE         base64-encoded .p12 (provisioned secret;
#                               imported into a temp keychain by
#                               Apple-Actions/import-codesign-certs@v3
#                               BEFORE this script runs).
#   - MACOS_CERTIFICATE_PWD     .p12 password (provisioned secret).
#   - MACOS_NOTARY_USER         Apple ID email (provisioned secret).
#   - MACOS_NOTARY_PWD          App-specific password — NOT iCloud
#                               password (provisioned secret).
#   - MACOS_TEAM_ID             10-char Team ID from Apple Developer
#                               portal (provisioned secret).
#   - MACOS_SIGNING_IDENTITY    optional override for the identity
#                               string; defaults to "Developer ID
#                               Application" (codesign picks the first
#                               matching identity in the keychain
#                               search list).
#
# Notarization model: we codesign the binary, zip it (notarytool requires
# a zip/dmg/pkg, never a loose binary), submit the zip, WAIT for
# approval, then exit. The signed binary is left in place. We don't
# staple a notarization ticket to the binary because plain Mach-O
# binaries can't be stapled — only .dmg, .pkg, and .app bundles support
# staples. Apple's Gatekeeper looks up the notarization status by hash
# online instead, which is the supported model for distributing
# notarized binaries inside .tar.gz archives.
set -euo pipefail

BINARY_PATH="${1:?usage: sign-macos.sh <binary-path>}"

# Only act on darwin builds. GoReleaser fires this hook for ALL goos
# values (one binary per goos/goarch pair), so we filter here. The
# `env:` block in .goreleaser.yaml exports GOOS to this hook.
if [[ "${GOOS:-}" != "darwin" ]]; then
  exit 0
fi

# Local-build escape hatch: if the cert secret is missing (developer
# laptop, PR-validation CI, snapshot mode), skip signing instead of
# failing. The release workflow ensures the secrets are present in the
# real release path; this script doesn't enforce it.
if [[ -z "${MACOS_CERTIFICATE:-}" ]]; then
  echo "[sign-macos] MACOS_CERTIFICATE unset; skipping codesign+notarize for ${BINARY_PATH}" >&2
  exit 0
fi

# All 5 secrets are required once we've decided to sign. Fail fast if
# any are missing so a half-configured workflow doesn't produce an
# unsigned-but-claimed-signed release.
: "${MACOS_CERTIFICATE_PWD:?MACOS_CERTIFICATE_PWD is required when signing}"
: "${MACOS_NOTARY_USER:?MACOS_NOTARY_USER is required when signing}"
: "${MACOS_NOTARY_PWD:?MACOS_NOTARY_PWD is required when signing}"
: "${MACOS_TEAM_ID:?MACOS_TEAM_ID is required when signing}"

IDENTITY="${MACOS_SIGNING_IDENTITY:-Developer ID Application}"

# ----------------------------------------------------------------------
# 1. codesign
# ----------------------------------------------------------------------
echo "[sign-macos] codesign --sign \"${IDENTITY}\" ${BINARY_PATH}"
# --options runtime enables Hardened Runtime, which Apple's notary
# service REQUIRES. Without it notarytool rejects the submission with
# "The executable does not have the hardened runtime enabled."
# --timestamp embeds an Apple timestamp authority signature; also
# required for notarization.
# --force overwrites any preexisting signature (matters when a previous
# snapshot build ran on the same machine).
codesign \
  --sign "${IDENTITY}" \
  --options runtime \
  --timestamp \
  --force \
  --verbose=2 \
  "${BINARY_PATH}"

# Verify codesign actually attached a valid signature. codesign --sign
# can silently fall back to ad-hoc signing if the identity isn't in the
# keychain; --verify --strict catches that.
codesign --verify --strict --verbose=2 "${BINARY_PATH}"

echo "[sign-macos] signed: ${BINARY_PATH}"

# ----------------------------------------------------------------------
# 2. notarize via xcrun notarytool
# ----------------------------------------------------------------------
# notarytool wants a zip/dmg/pkg input, never a raw binary. Build a
# zip in a temp dir so we don't pollute the build output directory
# (goreleaser archives the binary directly, not the zip we make here).
ZIP_DIR="$(mktemp -d -t logmind-notarize.XXXXXX)"
trap 'rm -rf "${ZIP_DIR}"' EXIT

BINARY_NAME="$(basename "${BINARY_PATH}")"
ZIP_PATH="${ZIP_DIR}/${BINARY_NAME}.zip"

# `ditto` is Apple's recommended tool for creating notary-compatible
# zip archives — preserves metadata and avoids the rare zip-format
# quirks that have caused notary rejections in the past. Same tool
# Apple's own documentation uses.
ditto -c -k --keepParent "${BINARY_PATH}" "${ZIP_PATH}"

echo "[sign-macos] xcrun notarytool submit (binary: ${BINARY_NAME})"
# --wait blocks until notarization completes (or times out at 20m,
# notarytool's default). On approval, the notarization ticket is
# registered with Apple keyed to the binary's hash; Gatekeeper looks
# it up online at first run.
#
# We pass credentials via flags rather than `notarytool store-credentials`
# (keychain-stored profile) because the CI runner is ephemeral — no
# point storing a profile that vanishes after the job. The Apple ID
# password MUST be an app-specific password generated at
# appleid.apple.com/account/manage; full Apple ID passwords are
# rejected with "Bad credentials".
xcrun notarytool submit "${ZIP_PATH}" \
  --apple-id "${MACOS_NOTARY_USER}" \
  --password "${MACOS_NOTARY_PWD}" \
  --team-id "${MACOS_TEAM_ID}" \
  --wait \
  --timeout 20m

echo "[sign-macos] notarized: ${BINARY_PATH}"
# We deliberately DON'T `xcrun stapler staple` here. stapler only works
# on .dmg/.pkg/.app — running it against a Mach-O binary fails with
# "Could not validate ticket: the staple to a binary is not supported".
# Apple's notarization service registers the ticket by hash; Gatekeeper
# fetches it online when the user first runs the binary, which is the
# canonical model for binary distribution.
