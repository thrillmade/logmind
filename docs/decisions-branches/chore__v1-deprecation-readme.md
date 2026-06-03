## 2026-06-03 13:18 - Lead README install with Go binary; mark Python wheel frozen at v0.6.16

**Reasoning:** v1.0.0 ships as Go binary via brew/curl; new installs should land on the Go path, not silently get the obsolete v0.6.16 pip wheel that PyPI still serves. The previous README still treated brew/curl + pip as parallel channels and described v1.0 as in-progress.

**Alternatives considered:** Remove PyPI badges and pyproject references entirely (too disruptive; breaks links from old consumer repos that pinned 0.6.x)., Yank the PyPI package (would break old pinning; user spec says keep it on PyPI for honour-old-pin reasons).

**Implications:**
- README front-loads brew/curl install + verification command; explicit deprecation callout warns against 'pip install logmind'. Quick Start drops the Python API import example. Contributing section flipped to Go build. Custom-integrations doc link dropped (Python-API only). Legacy section near bottom documents migration path with one-line install swap.

---
