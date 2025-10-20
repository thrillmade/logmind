# Decision Log

This file contains the 20 most recent decisions. Older decisions are archived in [decisions-archive.md](decisions-archive.md).

---
## 2025-10-19 16:57 - Implement search command for decision logs

**Reasoning:** Users need to be able to search through past decisions to find relevant context quickly. Supports regex patterns, case-sensitive/insensitive search, and includes archived decisions by default.

**Alternatives considered:** grep-based approach, Full-text search engine

**Implications:**
- Search includes both decisions.md and decisions-archive.md
- Context lines shown around matches for better understanding
- CLI command: logmind search 'query'

---
## 2025-10-19 17:16 - Enhance CLAUDE.md templates with comprehensive CLI examples and search documentation

**Reasoning:** AI agents need clear, practical examples of both Python API and CLI commands. Previous templates only showed Python usage, but agents often work in shell contexts where CLI is more appropriate.

**Alternatives considered:** Keep only Python API examples, Create separate CLI-only documentation

**Implications:**
- All new logmind projects will get improved instructions with both API styles
- Agents will know about search, show, and log commands
- Better discoverability of logmind features for AI agents

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
