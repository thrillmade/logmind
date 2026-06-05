## 2026-06-05 12:57 - feat(skill push): privacy gate layers 3+4 — content scanner + repo-visibility (§8.2 wave-2)

**Reasoning:** Belt-and-braces privacy model needs the content-scanner heuristic + visibility check on top of layers 1+2 so unmarked SKILL.md bodies + private→public catalog leaks both fail closed by default. Hardcoded baseline (credential prefixes, NDA-class keywords) is never bypassable from config; user config can only WIDEN the deny set. Test fixtures embed a 'DUMMYFIXTURE' sentinel infix in every credential-shaped test string so the scanner regexes still match while commercial secret detectors (GitHub push-protection, etc.) don't flag the test files.

**Alternatives considered:** Single-layer entropy-based credential detection (too FP-prone on UUIDs/SHAs), Inverting opt-out to opt-in for cross-visibility (would break first-time skill authors with private repos), Use the GitHub per-secret push-protection unblock URLs (manual web action; doesn't scale to CI)

**Implications:**
- PushOptions gains Stderr + ScannerConfig + AllowPromoteFromPrivate fields; .logmind/config.yml gains privacy_scanner + allow_promote_from_private sections; new sentinels ErrPrivacyScannerHit + ErrCrossVisibilityPush; gh runner now called on dry-run for visibility lookup (existing test assertion adjusted); credential test fixtures use 'DUMMYFIXTURE' infix to dodge commercial secret detectors while preserving regex contract (8+ alnum for Stripe, 36+ for ghp_, 60+ for github_pat_, 16 for AKIA).

---
