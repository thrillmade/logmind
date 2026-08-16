← back to [docs/timeline.md](../timeline.md)

## 2026-08-16 05:33 - site: pin the areas mirror to the Go constant that owns it, and gate the areas line on its own release rather than on NEXT_VERSION

**Reasoning:** site/app/page.tsx and internal/version/version.go both spelled the SPEC §7.3 areas string, and nothing checked they agreed — zero test files and zero workflow files referenced page.tsx or AREAS (control: page.tsx IS referenced by 7 files under docs/, so the search finds things when they exist). A hand-kept second copy diverges at exactly the wrong moment: a hand-edit at tag time, when the site is claiming what the new binary prints and nobody re-runs the binary. Pinning it surfaced a second defect in the code it pins to. The areas line was gated on IS_NEXT_RELEASED (CURRENT_VERSION === NEXT_VERSION), which answers 'is the next release out' — but the question the site asks is 'does the CURRENT release print this line'. Those coincide only because 2.0.0 happens to be the first release that prints it, and they diverge the moment NEXT_VERSION moves on. Worse, the new pin forces NEXT_VERSION to track version.Version, which becomes 2.1.0-dev right after the tag, so the pin would have SCHEDULED the regression for the first commit of the next cycle.

**Alternatives considered:** Pin AREAS and leave the predicate alone — rejected, that ships a known-wrong render on a known date. Pin CURRENT_VERSION/CURRENT_SPEC to the Go version too — rejected, those deliberately name the released 1.2.0 build while the binary is 2.0.0-dev, so equality there would be wrong today.

**Implications:**
- Gating is now SHOWS_AREAS_LINE = compareVersions(CURRENT_VERSION, AREAS_SINCE) >= 0, compared numerically rather than lexically because '2.10.0' < '2.9.0' as strings and that is a real future value. IS_NEXT_RELEASED is unchanged and still gates the four 'not yet released' caveats, each of which genuinely asks whether NEXT_VERSION itself shipped. Verified by extracting the real constants from page.tsx and evaluating them, not by reading: 1.2.0 hides the line, 2.0.0 shows it whether NEXT is 2.0.0 or 2.1.0, 2.10.0 shows it. An always-true comparator turns the first row red. Still uncovered: no CI-level test guards the TS render logic itself — only the two Go tests guard the raw string constants.

---

