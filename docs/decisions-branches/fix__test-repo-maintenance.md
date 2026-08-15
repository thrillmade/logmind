← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-testgit-one-helper-creates-every-test-repository-so-the-main -->
- **2026-08-15** — testgit: one helper creates every test repository, so the maintenance race cannot be reintroduced a file at a time
<!-- logmind-entry-end -->

## 2026-08-15 14:01 - testgit: one helper creates every test repository, so the maintenance race cannot be reintroduced a file at a time

**Reasoning:** Git commit calls run_auto_maintenance, gated by maintenance.auto rather than gc.auto, and the spawned process daemonises and is still writing into the object store when a test returns, so TempDir cleanup fails with directory not empty. It surfaces as a cleanup error on whichever test loses the race, which is why it looked like a different bug each time. Four files carried the fix inline and the rest did not. Reproduced independently on this machine before writing the test: five of five commits spawned git maintenance with gc.auto alone, zero of five with both keys.

**Alternatives considered:** Paste the two config lines into each remaining file. Rejected: that is the arrangement that already failed once, since the next person adding a test repository has nothing telling them the lines exist. A leaf package depending only on os, exec and testing means any package can import it without a cycle, so there is no reason left to hand-roll one.

**Implications:**
- The issue counted fourteen exposed files; the sweep found eight genuinely unguarded initialisations, several of the listed files being false positives that already routed through a guarded helper or never touched real git. It also found three exposures the issue did not list at all: two clone destinations and one bare push remote, none of which inherit configuration from the repository they came from. Cloning is the gap a file-by-file audit misses, because the search term is init. Green count went from 880 to 882, the two additions being the helper's own tests, one reading the keys back and one capturing trace output across five commits to assert zero spawns.

---

