## 2026-05-15 09:57 - site: tighten copy, drop pseudo-numbering, remove ornament side-rules

**Reasoning:** user pointed out the '01 brief' marginalia felt out of place (no section 1, only 2 and 3), the three principles section had a divider on top plus ornament side-rules on either side of its label (visual doubling), the body copy ran a bit long, and 'ADR backlog' was jargon they didn't recognize. Restructured hero to drop the 01/02/03 pseudo-numbering, replaced ornament+label with a proper h2 'principles.', tightened each principle to ~25 words, and swapped ADR language for plainer phrasing ('no after-the-fact writeup', 'decisions that lived in someone's head').

**Alternatives considered:** keep ornament rules but make them less visible on mobile only, skip the copy edits, just remove the divider duplication

**Implications:**
- page reads more cohesively: one section divider per section, no decorative double-rules
- still passes mobile + desktop visual QA at 375/1440 viewports
- removes jargon that wasn't speaking the audience's language

---
