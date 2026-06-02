## 2026-06-02 17:34 - B7: choose GoReleaser for cross-platform Go binary distribution

**Reasoning:** GoReleaser handles the full release pipeline declaratively in one .goreleaser.yaml: cross-compile (darwin amd64+arm64, linux amd64+arm64, windows amd64), archive .tar.gz/.zip, SHA256SUMS, GitHub Release publication, and Homebrew tap auto-bump. Hand-rolled cross-compile scripts would re-implement all of this poorly. GoReleaser also gracefully handles the snapshot-vs-release modes we need for PR-validation CI.

**Alternatives considered:** Hand-rolled GitHub Actions matrix build + manual archive + manual SHA256SUMS + manual brew formula write, cargo-make / Taskfile-style cross-compile orchestration, Build per-platform from per-platform runners (matrix of ubuntu + macos + windows)

**Implications:**
- Single .goreleaser.yaml is the source of truth for the release shape
- Pinned to GoReleaser v2.x; major-version bumps may require config migration
- Brews block deprecated in v2.16 → migrated to homebrew_casks (works for CLI binaries despite the name)

---
