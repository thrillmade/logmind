## 2026-05-15 09:40 - Site: responsive fixes (mobile 375 + desktop wide-screen)

**Reasoning:** User audit surfaced 5 real layout bugs: page allowed horizontal scroll on real-phone widths; quickstart code block was wider than viewport; hero CTAs broke mid-button with arrows on new lines; dividers looked doubled; nav side margins didn't match section gutters at >=1440px viewports. Root cause for most overflow: the section grids declared  — on mobile (no sm) CSS Grid auto-sized the implicit column to the widest content (pre with whitespace-pre), making children 370px on a 327px container. Added explicit  for the mobile state; wrapped nav in the same max-w-6xl container the sections use; clamped html/body with overflow-x:clip; switched hero CTAs to flex-col sm:flex-row stacking; dropped the ornament side-rules under 640px; added a right-edge fade on the quickstart pre so the scroll-within-block reads as intentional.

**Implications:**
- Verified at 375/414/768/1440/1920 via chrome-devtools-mcp emulate + JS overflow audit: htmlScrollWidth === viewport and 0 overflowing elements at every viewport
- Nav inner and first section's max-w-6xl wrapper align to identical x/right at every desktop width (e.g. at 1920: both x=384 right=1536)
- Hero CTAs now stack vertically on mobile (full-width) and side-by-side from sm: breakpoint

---
