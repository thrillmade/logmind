# Decision Log

This file contains the 20 most recent decisions. Older decisions are archived in [decisions-archive.md](decisions-archive.md).

---
## 2025-10-19 18:42 - Use oauth authentication

**Reasoning:** Security for /api/data

---
## 2025-10-19 18:42 - Connect to db.example.com:3306

---
## 2025-10-19 18:42 - Cache backend: redis

---
## 2025-10-19 18:42 - Use FastAPI

**Reasoning:** Best for our use case

**Alternatives considered:** Django, Flask, Tornado

---
## 2025-10-19 18:42 - Deploy to AWS

**Alternatives considered:** AWS eu-west-1, GCP, Azure

---
## 2025-10-19 18:42 - Enable dark_mode

**Implications:**
- Users will see new UI
- Backend load may increase

---
## 2025-10-19 18:42 - Process item1

---
## 2025-10-19 18:42 - Process item2

---
## 2025-10-19 18:42 - Process item3

---
## 2025-10-19 18:42 - Use Redis for caching

---
## 2025-10-19 18:42 - Use PostgreSQL

**Reasoning:** Selected postgres based on requirements

---
## 2025-10-19 18:42 - Unknown choice: memcached

---
## 2025-10-19 18:42 - The answer

---
## 2025-10-19 18:42 - Attempt risky operation

---
## 2025-10-19 20:26 - Implement Phase 3 decorators for automatic decision logging

**Reasoning:** Enable developers to log decisions automatically using @log_decision and @log_choice decorators. Supports template strings with function arguments, alternatives, implications, and all standard logmind features.

**Alternatives considered:** Manual logging only, Aspect-oriented programming approach

**Implications:**
- Users can decorate functions to auto-log decisions
- Template strings support {arg_name} placeholders
- @log_choice decorator for return-value-based decisions
- 15 comprehensive tests, 110 total tests passing

---
## 2025-10-19 20:28 - Add decorator documentation to README and update plan

**Reasoning:** Users need clear examples of how to use the new @log_decision and @log_choice decorators. README should showcase Phase 3 features.

**Implications:**
- README shows decorator usage examples
- Plan.md updated to show Phase 3 progress (33% complete)
- Clear path forward for remaining Phase 3 features

---
## 2025-10-19 20:46 - Prepare package for PyPI publishing

**Reasoning:** Package built and validated. Added LICENSE, MANIFEST.in, updated pyproject.toml with proper metadata, created CHANGELOG. Ready for upload to PyPI.

**Implications:**
- Built packages in dist/ folder (wheel and tar.gz)
- All twine checks pass
- Templates included correctly
- Package name: logmind v0.1.0

---
## 2025-10-20 07:07 - Add logmind-readme.md to docs folder for AI agents

**Reasoning:** AI agents need easy access to logmind documentation without navigating to repository root. Creates docs/logmind-readme.md during init by copying README.md content.

**Implications:**
- CLAUDE.md and other AI instruction files now link to docs/logmind-readme.md
- AI agents can read complete logmind instructions directly from docs folder
- Template files updated: logmind-section.md and CLAUDE.md.template

---
## 2025-10-20 07:12 - Make decisions-archive.md optional reference, not required reading

**Reasoning:** Archive is a searchable reference for historical context, not something AI agents must read upfront. Only recent decisions (docs/decisions.md) are required.

**Implications:**
- decisions-archive.md moved to 'Additional Reference' section
- Reduces cognitive load for AI agents - focus on recent 20 decisions
- Template files updated: logmind-section.md and CLAUDE.md.template

---
## 2026-01-17 14:28 - Add Cline and OpenAI Codex (AGENTS.md) to supported agents

**Reasoning:** Cline is a major VS Code agent using .clinerules; AGENTS.md is an emerging universal standard supported by OpenAI Codex, Cursor, Windsurf, and others under the Linux Foundation

**Alternatives considered:** Only add Cline, Only add AGENTS.md, Wait for more adoption

**Implications:**
- Now support 11 AI agents total
- AGENTS.md provides cross-tool compatibility

---
