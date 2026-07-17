← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-16-routes-the-non-interactive-link-advisory-to-stderr-closing-t -->
- **2026-07-16** — Routes the non-interactive link advisory to stderr, closing the 206a section 3.1.1 stdout-exactness gap before the v2.0.0 tag
<!-- logmind-entry-end -->

## 2026-07-16 22:42 - Route the non-interactive link advisory to stderr for section 3.1.1 exactness

**Reasoning:** Under no-interactive or non-TTY the link-health advisory printed to stdout, breaking the byte-exact three-line contract for agents and CI in repos with broken doc links; v2 made that contract a headline so shipping with a known violation of it is wrong

**Alternatives considered:** Defer to a post-release patch, which would ship v2.0.0 knowingly violating the clause it just polished

**Implications:**
- Fixes half of issue 206; the TTY nudge ordering half stays open as a product call since the headline answer must precede the commit line by design

---

