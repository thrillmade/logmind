← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-03-document-homebrew-6-0-0-tap-trust-cleanup-for-the-stale-thri -->
- **2026-07-03** — Document Homebrew 6.0.0 tap-trust cleanup for the stale thrillmot/logmind personal tap
<!-- logmind-entry-end -->

## 2026-07-03 22:52 - Document Homebrew 6.0.0 tap-trust cleanup for the stale thrillmot/logmind personal tap

**Reasoning:** Homebrew 6.0.0 (Jun 2026) added client-side tap trust; the pre-migration personal tap thrillmot/logmind is now flagged untrusted and ignored, tripping users with a warning. The canonical thrillmade/tap is already correct in README, install.md, and .goreleaser.yaml (owner thrillmade/homebrew-tap), and a fully-qualified 'brew install thrillmade/tap/logmind' auto-trusts the cask, so no repo/goreleaser change is needed — only user-facing cleanup docs.

**Implications:**
- docs/install.md gains a Troubleshooting entry (brew untap thrillmot/logmind + canonical install auto-trusts, optional brew trust thrillmade/tap for unqualified installs) plus an inline pointer in the Homebrew section. No goreleaser/attestation change: tap trust is purely client-side. The two remaining thrillmot mentions are historical decision-log/changelog records and are intentionally left untouched.

---

