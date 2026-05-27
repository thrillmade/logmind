## 2026-05-18 14:02 - docs: fix stale skills.sh badge URL in README

**Reasoning:** The skills.sh badge in README.md line 6 still pointed at thrillmade/logmind-skill — that repo was renamed to thrillmade/agent-skills in v0.1.2 and reorganized into a collection layout. Badge image was 404'ing or showing 'custom badge invalid'. Updated to the collection badge URL (thrillmade/agent-skills) and the link to the logmind skill's page within the collection.

**Implications:**
- README badge resolves to the collection's Skills count badge
- Click-through lands on the logmind skill's skills.sh page within the agent-skills collection

---
