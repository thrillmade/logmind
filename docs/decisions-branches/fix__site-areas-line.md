← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-add-the-spec-7-3-areas-line-to-the-site-gated-on-actual-rele -->
- **2026-08-15** — Add the SPEC §7.3 areas line to the site, gated on actual release
<!-- logmind-entry-end -->

## 2026-08-15 08:47 - Add the SPEC §7.3 areas line to the site, gated on actual release

**Reasoning:** site/app/page.tsx rendered the --version example from a single-line template. SPEC §7.3 requires two lines. Nothing is wrong on the live site today — it correctly advertises the released v1.2.0, whose binary genuinely prints one line, because the areas line shipped after it. The defect is forward-looking: the file header says releasing v2.0.0 is a three-constant flip, and after that flip the page would have shown the version line and stopped — a --version example missing the line §7.3 makes mandatory, on the page whose whole job is telling people what a correct install prints.

**Alternatives considered:** Hardcode the areas line now. Rejected: that makes the page claim something the downloadable binary does not do, which is exactly the class of error #272 fixed when it stopped the site advertising an unreleased version.

**Implications:**
- A fourth constant, AREAS, copied verbatim from internal/version/version.go rather than retyped, and gated on the existing IS_NEXT_RELEASED derivation. It is deliberately decoupled from the three release constants because it tracks the declared areas, not the version number — so the tag-time flip stays exactly three lines, and the header comment now says AREAS is only touched if the binary Areas value itself changes. Verified by BUILDING the site and grepping the rendered HTML, not by reading source: today one line unchanged; with the constants temporarily flipped, two lines with a real newline and the exact Areas string. npm run lint fails for want of an eslint config, confirmed pre-existing on origin/dev and unrelated.

---

