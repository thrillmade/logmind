## 2026-05-26 15:20 - fix: v0.2.5 — refresh-mode also updates stale version pins

**Reasoning:** Bug caught by another agent in clud-bug: regen-timeline.yml still pinned to logmind==0.2.1 after we shipped 0.2.4. Cause: refresh-mode only re-renders a workflow when the template-version marker differs, but the pip install pin can drift independently — releases like 0.2.3 and 0.2.4 didn't touch any template body, so refresh-mode left old pins behind. Fix: when marker matches AND a pin line exists, surgically rewrite ONLY the pin line if its version != current __version__. Markerless workflows (dogfood/customized) still left alone, respecting the same v0.2.1 heuristic.

**Alternatives considered:** Always re-render when pin is stale — would clobber legitimate body customizations users kept alongside a current marker, Bump every template marker on every release — defeats the idempotent-refresh design and over-refreshes

**Implications:**
- logmind init in any repo with stale pin will pick up the current version on next run, even when no template body changed
- Two regression tests added: stale-pin-with-current-marker (refreshes) and markerless-stale-pin (left alone)
- Net effect: logmind doctor's 'STALE installed version' suggestion now actually heals via the printed command

---
