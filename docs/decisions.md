# Decision Log

This file contains the 20 most recent decisions. Older decisions are archived in [decisions-archive.md](decisions-archive.md).

---
## 2025-10-19 17:41 - Use pipx for global logmind installation like standard CLI tools

**Reasoning:** logmind is a CLI tool like git/npm/docker - should be globally installed, not repo-local. Users expect to just type 'logmind' without prefixes or activation.

**Alternatives considered:** Repo-local venv with wrapper script, Makefile commands

**Implications:**
- Contributors run 'pipx install -e .' once after cloning
- Clean CLI experience: just type 'logmind'
- Works from any directory
- Need to document setup in README

---
## 2025-10-19 17:41 - Update README with pipx setup instructions and CLI examples

**Reasoning:** Contributors need clear setup instructions. README should show both Python API and CLI usage, and explain the pipx approach.

**Implications:**
- Clear onboarding for new contributors
- README shows complete feature set (log, show, search)
- Explains why pipx (CLI tool philosophy)

---
## 2025-10-19 17:44 - Make logmind commit all changed files, not just docs

**Reasoning:** When logging a decision, users expect ALL their changes to be committed together with that decision - like a normal git workflow. Previously only docs/ files were committed.

**Alternatives considered:** Keep selective file commits

**Implications:**
- logmind now does 'git add .' before committing
- All changes committed together with decision
- More intuitive git workflow

---
## 2025-10-19 18:42 - Test decision

**Reasoning:** Test reasoning

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
