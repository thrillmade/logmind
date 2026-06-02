## 2026-06-02 17:34 - B7: choose GoReleaser for cross-platform Go binary distribution

**Reasoning:** GoReleaser handles the full release pipeline declaratively in one .goreleaser.yaml: cross-compile (darwin amd64+arm64, linux amd64+arm64, windows amd64), archive .tar.gz/.zip, SHA256SUMS, GitHub Release publication, and Homebrew tap auto-bump. Hand-rolled cross-compile scripts would re-implement all of this poorly. GoReleaser also gracefully handles the snapshot-vs-release modes we need for PR-validation CI.

**Alternatives considered:** Hand-rolled GitHub Actions matrix build + manual archive + manual SHA256SUMS + manual brew formula write, cargo-make / Taskfile-style cross-compile orchestration, Build per-platform from per-platform runners (matrix of ubuntu + macos + windows)

**Implications:**
- Single .goreleaser.yaml is the source of truth for the release shape
- Pinned to GoReleaser v2.x; major-version bumps may require config migration
- Brews block deprecated in v2.16 → migrated to homebrew_casks (works for CLI binaries despite the name)

---
## 2026-06-02 17:37 - B7: macOS codesign + notarize via per-build hook script (notarytool, no gon)

**Reasoning:** scripts/sign-macos.sh is invoked by goreleaser builds.hooks.post per darwin binary. It codesigns with --options runtime --timestamp --force (all 3 required for notarization), then zips the signed binary with ditto and submits to xcrun notarytool with --wait. The 5 provisioned secrets (MACOS_CERTIFICATE, MACOS_CERTIFICATE_PWD, MACOS_NOTARY_USER, MACOS_NOTARY_PWD, MACOS_TEAM_ID) flow through release.yml env block straight to the script — no certificate import in the script (Apple-Actions/import-codesign-certs@v3 handles that in CI before goreleaser runs). Script no-ops when MACOS_CERTIFICATE is unset so local snapshot builds work offline. We do NOT staple — plain Mach-O binaries don't support stapling; Gatekeeper checks notarization status by hash online which is the supported model for binary tarballs.

**Alternatives considered:** gon (archived Sep 2022, no maintenance, predates notarytool migration), GoReleaser native notarize: block (Pro-only OR OSS quill, but OSS quill needs App Store Connect API key flow + .p8 file — incompatible with the legacy Apple-ID + app-specific-password secrets already provisioned), Sign via codesign hook + notarize via separate after-archive hook (more script files, no real benefit; per-build is simpler)

**Implications:**
- Per-darwin-binary notarization: each tag release submits 2 notarytool jobs (amd64 + arm64), each waits up to 20m
- Local snapshot builds skip signing automatically (escape hatch via MACOS_CERTIFICATE unset)
- Signed binary inside .tar.gz means Gatekeeper queries Apple's notary service by hash on first run — no staple needed

---
## 2026-06-02 17:37 - B7: Homebrew tap auto-bump via homebrew_casks block (not brews; not separate publish job)

**Reasoning:** GoReleaser's homebrew_casks block opens a PR on thrillmade/homebrew-tap with the new tag URL + per-arch SHA256 baked in. brews block is deprecated in v2.10 and fully removed in v2.16 (we installed 2.16 locally; CI uses 'latest'). homebrew_casks works fine for CLI binaries via the binaries: [] field — the cask form gives us a cleaner Gatekeeper quarantine attr strip via postflight hook than a Formula would. Cross-repo write requires HOMEBREW_TAP_PAT (fine-grained PAT on thrillmade/homebrew-tap with Contents+PullRequests write) — already provisioned and surfaced from release.yml env.

**Alternatives considered:** Hand-rolled brew-bump workflow (mirrors existing v0.x homebrew-bump.yml but for the Go binary tap; adds maintenance burden, no payoff), Use the deprecated brews: block (would have worked on v2.15 but breaks on v2.16+; unstable choice), Open a manual PR per release (defeats the automation purpose of the wave)

**Implications:**
- Cask file path: thrillmade/homebrew-tap/Casks/logmind.rb (was Formula/logmind.rb on the old v0.x tap)
- User-facing install command: brew install thrillmade/tap/logmind (was brew install thrillmade/logmind/logmind on the old tap)
- Old homebrew-bump.yml (v0.x PyPI path) and new release.yml coexist via tag-pattern gating: v0.* fires homebrew-bump, v1.* fires release.yml

---
